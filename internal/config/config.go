// Package config persists sqlitedeploy's per-project configuration.
//
// Layout on disk:
//
//	<project>/.sqlitedeploy/config.yml      — written by `init` / `attach`
//	<project>/.sqlitedeploy/litestream.yml  — generated; passed to litestream
//	<project>/data/app.db                   — the SQLite file
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

	// LitestreamFile is the generated litestream config inside DirName.
	LitestreamFile = "litestream.yml"
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

	// ReplicaPath is the optional sub-path within the bucket where this DB's
	// WAL lives. Lets multiple databases share one bucket. Defaults to "db".
	ReplicaPath string `yaml:"replica_path,omitempty"`

	// PullIntervalSeconds is the periodic restore cadence on replica nodes.
	// Ignored on primaries.
	PullIntervalSeconds int `yaml:"pull_interval_seconds,omitempty"`
}

// CurrentVersion is bumped whenever the on-disk schema changes.
const CurrentVersion = 1

// Default values.
const (
	DefaultDBPath              = "data/app.db"
	DefaultReplicaPath         = "db"
	DefaultPullIntervalSeconds = 5
)

// Path returns the absolute path to the config file inside dir.
func Path(projectDir string) string {
	return filepath.Join(projectDir, DirName, ConfigFile)
}

// LitestreamPath returns the absolute path to the generated litestream.yml.
func LitestreamPath(projectDir string) string {
	return filepath.Join(projectDir, DirName, LitestreamFile)
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
	if c.ReplicaPath == "" {
		c.ReplicaPath = DefaultReplicaPath
	}
	if c.PullIntervalSeconds == 0 {
		c.PullIntervalSeconds = DefaultPullIntervalSeconds
	}
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
