// Package config persists sqlitedeploy's per-project configuration.
//
// Layout on disk:
//
//	<project>/.sqlitedeploy/config.yml          — written by `init` / `attach`
//	<project>/.sqlitedeploy/auth/jwt_*.pem      — Ed25519 JWT keypair (init)
//	<project>/.sqlitedeploy/auth/replica.jwt    — long-lived replica token (init)
//	<project>/data/app.db                       — the SQLite file managed by sqld
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/Khangdang1690/sqlitedeploy/internal/providers"
)

const (
	// DirName is the per-project directory holding our config files.
	DirName = ".sqlitedeploy"

	// ConfigFile is the persisted user config inside DirName.
	ConfigFile = "config.yml"
)

// Role distinguishes a primary node (writes + replicates) from a read replica
// (pulls only). Persisted so `run` and `attach` can refuse to operate on the
// wrong kind of node.
type Role string

const (
	RolePrimary Role = "primary"
	RoleReplica Role = "replica"
)

// Config is what we serialize to .sqlitedeploy/config.yml.
type Config struct {
	Version  int              `yaml:"version"`
	Role     Role             `yaml:"role"`
	DBPath   string           `yaml:"db_path"`
	Provider providers.Config `yaml:"provider"`

	// BucketPrefix is the bottomless prefix used to namespace this DB's
	// data within the bucket. Lets multiple databases share one bucket.
	// Maps to LIBSQL_BOTTOMLESS_DB_NAME at runtime. Defaults to "db".
	BucketPrefix string `yaml:"bucket_prefix,omitempty"`

	// HTTPListenAddr exposes sqld's HTTP API to apps. On primaries this is
	// also where edge clients (Workers, Lambda) connect via Hrana-over-HTTP.
	// Default 127.0.0.1:8080; set to 0.0.0.0:8080 for production exposure.
	HTTPListenAddr string `yaml:"http_listen_addr,omitempty"`

	// HranaListenAddr exposes Hrana over WebSocket. Optional; if empty,
	// only HTTP is exposed.
	HranaListenAddr string `yaml:"hrana_listen_addr,omitempty"`

	// GRPCListenAddr (primary only) is where replica nodes connect to
	// stream WAL frames. Default 0.0.0.0:5001.
	GRPCListenAddr string `yaml:"grpc_listen_addr,omitempty"`

	// AdminListenAddr is sqld's admin HTTP API used by `sqlitedeploy
	// status`. Bind to localhost. Default 127.0.0.1:8081.
	AdminListenAddr string `yaml:"admin_listen_addr,omitempty"`

	// PrimaryGRPCURL (replica only) is the primary's gRPC endpoint, e.g.
	// "http://primary.example.com:5001". Required on replicas.
	PrimaryGRPCURL string `yaml:"primary_grpc_url,omitempty"`

	// PrimaryHranaURL (replica only) is informational — the primary's
	// Hrana endpoint, useful to display in status output and for direct
	// edge-client access without going through this replica.
	PrimaryHranaURL string `yaml:"primary_hrana_url,omitempty"`

	// JWTPublicKeyPath is the relative-to-DirName (or absolute) path to
	// the Ed25519 public key sqld uses to verify inbound JWTs. Required
	// on both primary and replicas. Default "auth/jwt_public.pem".
	JWTPublicKeyPath string `yaml:"jwt_public_key_path,omitempty"`

	// JWTPrivateKeyPath (primary only) is the path to the Ed25519 private
	// key used to mint replica tokens. Default "auth/jwt_private.pem".
	JWTPrivateKeyPath string `yaml:"jwt_private_key_path,omitempty"`
}

// CurrentVersion is bumped whenever the on-disk schema changes.
const CurrentVersion = 1

// Default values.
const (
	DefaultDBPath            = "data/app.db"
	DefaultBucketPrefix      = "db"
	DefaultHTTPListenAddr    = "127.0.0.1:8080"
	DefaultGRPCListenAddr    = "0.0.0.0:5001"
	DefaultAdminListenAddr   = "127.0.0.1:8081"
	DefaultJWTPublicKeyPath  = "auth/jwt_public.pem"
	DefaultJWTPrivateKeyPath = "auth/jwt_private.pem"
)

// Path returns the absolute path to the config file inside dir.
func Path(projectDir string) string {
	return filepath.Join(projectDir, DirName, ConfigFile)
}

// Load reads config from <projectDir>/.sqlitedeploy/config.yml.
func Load(projectDir string) (*Config, error) {
	p := Path(projectDir)
	raw, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no sqlitedeploy config found at %s — run `sqlitedeploy init` first", p)
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	c.applyDefaults()
	return &c, nil
}

// Save writes config to <projectDir>/.sqlitedeploy/config.yml with 0o600 perms,
// since it contains credentials.
func Save(projectDir string, c *Config) error {
	c.Version = CurrentVersion
	c.applyDefaults()

	dir := filepath.Join(projectDir, DirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.WriteFile(Path(projectDir), raw, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return ensureGitignore(projectDir)
}

func (c *Config) applyDefaults() {
	if c.DBPath == "" {
		c.DBPath = DefaultDBPath
	}
	if c.BucketPrefix == "" {
		c.BucketPrefix = DefaultBucketPrefix
	}
	if c.HTTPListenAddr == "" {
		c.HTTPListenAddr = DefaultHTTPListenAddr
	}
	if c.AdminListenAddr == "" {
		c.AdminListenAddr = DefaultAdminListenAddr
	}
	if c.JWTPublicKeyPath == "" {
		c.JWTPublicKeyPath = DefaultJWTPublicKeyPath
	}
	// GRPCListenAddr and JWTPrivateKeyPath default only on primaries; the
	// caller (init.go) is in a better position to set them since it knows
	// the role.
}

// ResolveAuthPath resolves a JWT key path that may be relative-to-DirName or
// absolute. Used by run/attach to convert config values into actual paths
// passed to sqld via --auth-jwt-key-file.
func (c *Config) ResolveAuthPath(projectDir, p string) string {
	if p == "" {
		return ""
	}
	// If absolute, use as-is.
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(projectDir, DirName, p)
}

// ensureGitignore appends ".sqlitedeploy/" to a project's .gitignore if a git
// repo is present. We never want credentials committed.
func ensureGitignore(projectDir string) error {
	if _, err := os.Stat(filepath.Join(projectDir, ".git")); err != nil {
		return nil
	}
	gi := filepath.Join(projectDir, ".gitignore")
	existing, err := os.ReadFile(gi)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	needle := "\n" + DirName + "/\n"
	combined := "\n" + string(existing) + "\n"
	if !contains(combined, needle) {
		f, err := os.OpenFile(gi, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.WriteString(DirName + "/\n"); err != nil {
			return err
		}
	}
	return nil
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
