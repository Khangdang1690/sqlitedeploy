// Package auth handles sqlitedeploy's JWT-based authentication for sqld.
//
// On primary nodes, `sqlitedeploy up` generates an Ed25519 keypair, writes
// it under .sqlitedeploy/auth/, and mints a long-lived JWT for replicas to
// authenticate to the primary's gRPC endpoint. Sqld verifies inbound JWTs
// against the public key passed via `--auth-jwt-key-file`.
//
// On replica nodes, only the public key + replica JWT are needed — the user
// copies them over from the primary via scp/secure-transfer (never via
// chat or unencrypted channels).
package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// GenerateKeypair returns a fresh Ed25519 keypair using crypto/rand.
func GenerateKeypair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("ed25519 keygen: %w", err)
	}
	return pub, priv, nil
}

// WritePrivatePEM writes priv to path in PKCS#8 PEM form, mode 0600.
// The directory is created if it doesn't exist (mode 0700).
func WritePrivatePEM(path string, priv ed25519.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}
	return writePEM(path, &pem.Block{Type: "PRIVATE KEY", Bytes: der}, 0o600)
}

// WritePublicPEM writes pub to path in PKIX PEM form, mode 0644.
func WritePublicPEM(path string, pub ed25519.PublicKey) error {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return fmt.Errorf("marshal public key: %w", err)
	}
	return writePEM(path, &pem.Block{Type: "PUBLIC KEY", Bytes: der}, 0o644)
}

// ReadPrivatePEM reads an Ed25519 PKCS#8 PEM file from path.
func ReadPrivatePEM(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS8 in %s: %w", path, err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s is not an Ed25519 private key (got %T)", path, key)
	}
	return priv, nil
}

func writePEM(path string, block *pem.Block, mode os.FileMode) error {
	if err := os.MkdirAll(dirOf(path), 0o700); err != nil {
		return fmt.Errorf("create %s parent: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	if err := pem.Encode(f, block); err != nil {
		return fmt.Errorf("write PEM to %s: %w", path, err)
	}
	return nil
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
