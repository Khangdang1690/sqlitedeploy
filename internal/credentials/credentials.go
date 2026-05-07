// Package credentials manages sqlitedeploy's machine-level credential file —
// the place where `sqlitedeploy auth login` stores the user's Cloudflare API
// token so subsequent `sqlitedeploy init` commands can use it without
// re-prompting.
//
// File location:
//
//	Windows: %APPDATA%\sqlitedeploy\credentials.yml
//	macOS:   ~/Library/Application Support/sqlitedeploy/credentials.yml
//	Linux:   ~/.config/sqlitedeploy/credentials.yml
//
// File mode is 0600 on Unix; Windows uses ACLs (we set the user-only flag is
// not portable, so we accept the default — the directory is per-user already).
package credentials

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// File is the persisted credential payload.
type File struct {
	// Cloudflare holds the user's Cloudflare API token, persisted after the
	// `auth login` flow validates it. Optional — if absent, managed-mode init
	// will refuse to run.
	Cloudflare *CloudflareCreds `yaml:"cloudflare,omitempty"`
}

// CloudflareCreds is what we store for a Cloudflare-authenticated user.
type CloudflareCreds struct {
	APIToken    string `yaml:"api_token"`
	AccountID   string `yaml:"account_id,omitempty"`   // cached from first ListAccounts
	AccountName string `yaml:"account_name,omitempty"` // shown in `auth status`
}

// dirName is the per-app config directory inside os.UserConfigDir().
const dirName = "sqlitedeploy"
const fileName = "credentials.yml"

// Path returns the absolute path to the credentials file. Created lazily.
func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config dir: %w", err)
	}
	return filepath.Join(base, dirName, fileName), nil
}

// Load reads the credentials file, returning a zero-valued File when no file
// exists yet. Other I/O errors propagate.
func Load() (*File, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &File{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &f, nil
}

// Save writes f to the credentials file with mode 0600.
func Save(f *File) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(p), err)
	}
	raw, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", p, err)
	}
	return nil
}

// Delete removes the credentials file. Returns nil if it didn't exist.
func Delete() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Cloudflare loads the credentials file and returns the Cloudflare block.
// Returns ErrNoCloudflare if the user hasn't run `auth login` yet.
func Cloudflare() (*CloudflareCreds, error) {
	f, err := Load()
	if err != nil {
		return nil, err
	}
	if f.Cloudflare == nil || f.Cloudflare.APIToken == "" {
		return nil, ErrNoCloudflare
	}
	return f.Cloudflare, nil
}

// SaveCloudflare upserts the Cloudflare block in the credentials file,
// preserving any future non-Cloudflare blocks we add later.
func SaveCloudflare(creds *CloudflareCreds) error {
	f, err := Load()
	if err != nil {
		return err
	}
	f.Cloudflare = creds
	return Save(f)
}

// ErrNoCloudflare signals that no Cloudflare token has been stored yet —
// callers should print an instructive message pointing at `auth login`.
var ErrNoCloudflare = errors.New("no Cloudflare credentials saved; run `sqlitedeploy auth login` first")
