// Package tunnel provides Cloudflare Tunnel integration so users get a public
// HTTPS URL for their local sqld without owning a domain, opening ports, or
// running a TLS terminator. We lean on TryCloudflare quick tunnels — free,
// ephemeral, no account required — for the default `up` flow.
package tunnel

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Resolve returns the path to a runnable cloudflared binary, downloading and
// caching the pinned release if necessary. Resolution order:
//
//  1. CLOUDFLARED env var (absolute path; lets users pin their own build)
//  2. `cloudflared` on PATH (homebrew / apt installs land here)
//  3. Cached download at <UserCacheDir>/sqlitedeploy/cloudflared[.exe]
//  4. Fresh download from github.com/cloudflare/cloudflared releases
func Resolve() (string, error) {
	if p := os.Getenv("CLOUDFLARED"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p, nil
	}

	cacheDir, err := cloudflaredCacheDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(cacheDir, cloudflaredFilename())
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}

	return downloadCloudflared(dst)
}

// cloudflaredFilename returns "cloudflared.exe" on Windows, "cloudflared"
// elsewhere.
func cloudflaredFilename() string {
	if runtime.GOOS == "windows" {
		return "cloudflared.exe"
	}
	return "cloudflared"
}

func cloudflaredCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "sqlitedeploy"), nil
}

// downloadCloudflared fetches the pinned release into dst and makes it
// executable. Returns the path on success.
func downloadCloudflared(dst string) (string, error) {
	a, ok := assetForCurrent()
	if !ok {
		return "", fmt.Errorf("no cloudflared release for %s/%s — install it manually (https://github.com/cloudflare/cloudflared) and put it on PATH, or set CLOUDFLARED=/abs/path", runtime.GOOS, runtime.GOARCH)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}

	fmt.Fprintf(os.Stderr, "→ Downloading cloudflared %s for %s/%s…\n", PinnedVersion, runtime.GOOS, runtime.GOARCH)
	resp, err := http.Get(a.URL)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", a.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", a.URL, resp.StatusCode)
	}

	// Buffer the whole download into memory so we can hash + (optionally)
	// untar in one pass. cloudflared binaries are ~30 MB — fine.
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	gotSum := sha256.Sum256(buf)
	gotHex := hex.EncodeToString(gotSum[:])
	switch {
	case a.SHA256 == "":
		fmt.Fprintf(os.Stderr, "  (no pinned sha256 yet; verify manually if needed: %s)\n", gotHex)
	case !strings.EqualFold(a.SHA256, gotHex):
		return "", fmt.Errorf("cloudflared sha256 mismatch: want %s, got %s — refusing to install", a.SHA256, gotHex)
	}

	body := buf
	if a.Tarred {
		body, err = extractCloudflaredFromTgz(buf)
		if err != nil {
			return "", err
		}
	}

	if err := os.WriteFile(dst, body, 0o755); err != nil {
		return "", fmt.Errorf("write %s: %w", dst, err)
	}
	return dst, nil
}

// extractCloudflaredFromTgz pulls the "cloudflared" file out of a gzipped tar
// archive. Used for the macOS releases which ship as .tgz.
func extractCloudflaredFromTgz(buf []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("cloudflared not found in tarball")
		}
		if err != nil {
			return nil, fmt.Errorf("untar: %w", err)
		}
		if filepath.Base(h.Name) == "cloudflared" {
			return io.ReadAll(tr)
		}
	}
}
