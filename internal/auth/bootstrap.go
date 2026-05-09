package auth

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
)

// Standard layout under <project>/.sqlitedeploy/auth/.
const (
	DirName          = "auth"
	PublicKeyFile    = "jwt_public.pem"
	PrivateKeyFile   = "jwt_private.pem"
	ReplicaTokenFile = "replica.jwt"
)

// Files are the absolute paths produced by Bootstrap.
type Files struct {
	PublicKey    string
	PrivateKey   string
	ReplicaToken string
}

// AuthDir returns <projectDir>/.sqlitedeploy/auth.
func AuthDir(projectDir, configDirName string) string {
	return filepath.Join(projectDir, configDirName, DirName)
}

// BootstrapPrimary creates the auth/ directory under .sqlitedeploy/, generates
// an Ed25519 keypair, writes both halves to disk, mints a long-lived replica
// JWT, and returns the absolute paths.
//
// configDirName is typically config.DirName (".sqlitedeploy"); accepting it
// as a parameter avoids a circular import between auth and config.
//
// Idempotent: if the keypair files already exist, we re-use them and only
// re-mint the replica token.
func BootstrapPrimary(projectDir, configDirName string) (*Files, error) {
	authDir := AuthDir(projectDir, configDirName)
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", authDir, err)
	}
	files := &Files{
		PublicKey:    filepath.Join(authDir, PublicKeyFile),
		PrivateKey:   filepath.Join(authDir, PrivateKeyFile),
		ReplicaToken: filepath.Join(authDir, ReplicaTokenFile),
	}

	priv, err := loadOrCreateKeypair(files)
	if err != nil {
		return nil, err
	}

	tok, err := MintReplicaToken(priv, 0)
	if err != nil {
		return nil, fmt.Errorf("mint replica token: %w", err)
	}
	if _, _, err := ParseUnverified(tok); err != nil {
		return nil, fmt.Errorf("self-check minted token: %w", err)
	}
	if err := os.WriteFile(files.ReplicaToken, []byte(tok+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("write %s: %w", files.ReplicaToken, err)
	}
	return files, nil
}

func loadOrCreateKeypair(files *Files) (ed25519.PrivateKey, error) {
	if _, err := os.Stat(files.PrivateKey); err == nil {
		return ReadPrivatePEM(files.PrivateKey)
	}
	pub, priv, err := GenerateKeypair()
	if err != nil {
		return nil, err
	}
	if err := WritePrivatePEM(files.PrivateKey, priv); err != nil {
		return nil, err
	}
	if err := WritePublicPEM(files.PublicKey, pub); err != nil {
		return nil, err
	}
	return priv, nil
}
