package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// MintToken issues an EdDSA-signed JWT with the given claims, signed by priv.
// Sqld accepts EdDSA JWTs when --auth-jwt-key-file points at the matching
// public key.
//
// We mint by hand (3 base64url segments joined with '.') rather than pulling
// in a JWT library — keeps dep surface small and the format is trivial.
func MintToken(priv ed25519.PrivateKey, claims map[string]any) (string, error) {
	header := map[string]string{"alg": "EdDSA", "typ": "JWT"}
	hdrJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	clmJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signing := b64(hdrJSON) + "." + b64(clmJSON)
	sig := ed25519.Sign(priv, []byte(signing))
	return signing + "." + b64(sig), nil
}

// MintReplicaToken returns a long-lived JWT (10y by default) that authorizes
// a replica node to stream from the primary's gRPC endpoint.
func MintReplicaToken(priv ed25519.PrivateKey, lifetime time.Duration) (string, error) {
	if lifetime <= 0 {
		lifetime = 10 * 365 * 24 * time.Hour
	}
	now := time.Now().UTC()
	claims := map[string]any{
		"iss": "sqlitedeploy",
		"sub": "replica",
		"iat": now.Unix(),
		"exp": now.Add(lifetime).Unix(),
	}
	return MintToken(priv, claims)
}

// b64 is base64url-without-padding (RFC 7515 §2).
func b64(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// Sanity-check that the JWT we just minted parses cleanly. Used as a smoke
// test in init so we fail fast if something's off, before printing the token.
func ParseUnverified(tok string) (header, claims map[string]any, err error) {
	parts := splitDot(tok)
	if len(parts) != 3 {
		return nil, nil, fmt.Errorf("expected 3 dot-separated segments, got %d", len(parts))
	}
	hdr, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf("decode header: %w", err)
	}
	clm, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf("decode claims: %w", err)
	}
	if err := json.Unmarshal(hdr, &header); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(clm, &claims); err != nil {
		return nil, nil, err
	}
	return header, claims, nil
}

func splitDot(s string) []string {
	var out []string
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			out = append(out, s[last:i])
			last = i + 1
		}
	}
	out = append(out, s[last:])
	return out
}
