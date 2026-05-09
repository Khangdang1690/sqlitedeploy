// Package sqld bundles and runs the libsql-server `sqld` binary that powers
// sqlitedeploy's runtime. Replaces the previous Litestream integration; the
// engine swap is documented in the project plan.
//
// Resolution strategy mirrors internal/litestream: prefer a binary embedded
// at build time (one per supported GOOS/GOARCH), else fall back to `sqld` on
// $PATH, else error with an instructive message.
package sqld

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// bundledBinary is the sqld binary for the current GOOS/GOARCH, embedded
// at build time. Populated by exactly one of the build-tagged files in this
// package (embed_<os>_<arch>.go). Platforms we don't ship a binary for —
// notably Windows, which libsql-server doesn't compile on — fall through to
// embed_unsupported.go and end up with an empty slice.
//
// bundledExt is "" on Unix; we don't currently produce Windows binaries so
// it's never ".exe", but the variable is kept for symmetry with the previous
// Litestream package.

// placeholderPrefix marks an in-tree placeholder file (used before
// `make build-sqld` / CI release builds have run) so we can detect it at
// runtime and fall back to PATH.
var placeholderPrefix = []byte("PLACEHOLDER")

// minBinarySize is a lower bound for what we'll consider a real binary. Real
// sqld release builds are tens of MB.
const minBinarySize = 1 << 20 // 1 MiB

// downloadVersion is the GitHub release tag (without leading "v") used to
// fetch a standalone sqld binary as a last-resort fallback when neither the
// embedded binary nor PATH yields a runnable sqld. Stamped at build time via
// `-X internal/sqld.downloadVersion=<ver>` by `make release` and CI.
//
// Default "dev" disables the fallback so locally-built binaries (no
// `make build-sqld` first) still surface the loud "no sqld available" error
// — silent network fetch from a dev tree would mask configuration mistakes.
var downloadVersion = "dev"

// downloadBaseURL is the GitHub Releases base. Variable so tests can point it
// at a local server.
var downloadBaseURL = "https://github.com/Khangdang1690/sqlitedeploy/releases/download"

// Resolve returns the absolute path to a runnable `sqld` binary.
//
// Strategy:
//  1. If a real binary was embedded for this platform, extract it to the user
//     cache dir and run it from there.
//  2. Otherwise fall back to looking for `sqld` on PATH.
//  3. Otherwise (tagged release builds only) download the standalone sqld
//     binary attached to this repo's GitHub Release matching downloadVersion.
//  4. If none works, return an instructive error.
func Resolve() (string, error) {
	if path, err := extractBundled(); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("sqld"); err == nil {
		return path, nil
	}
	if path, err := downloadSqld(); err == nil {
		return path, nil
	}
	return "", fmt.Errorf(
		"no sqld binary available for %s/%s. Tried: bundled binary (placeholder in this build), `sqld` on PATH (not found), and download from GitHub Releases (disabled in dev builds or unreachable). Either build sqlitedeploy with bundled binaries (run `make build-sqld` then rebuild), install sqld separately and put it on PATH, or use a tagged release (`go install …@v0.5.1` or `npm install -g sqlitedeploy`). On Windows, run sqlitedeploy under WSL — libsql-server does not support Windows natively.",
		runtime.GOOS, runtime.GOARCH)
}

func extractBundled() (string, error) {
	if isPlaceholder(bundledBinary) {
		return "", fmt.Errorf("no bundled sqld binary for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	cacheDir, err := userCacheDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(cacheDir, bundledName())

	// Skip rewrite if already extracted with same content (cheap content check
	// via size — embed bytes are stable per build).
	if st, err := os.Stat(dst); err == nil && st.Size() == int64(len(bundledBinary)) {
		return dst, nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, bundledBinary, 0o755); err != nil {
		return "", err
	}
	return dst, nil
}

func bundledName() string {
	return fmt.Sprintf("sqld-%s-%s%s", runtime.GOOS, runtime.GOARCH, bundledExt)
}

// downloadSqld fetches the standalone sqld binary attached to this repo's
// GitHub Release for the running platform, caches it under userCacheDir, and
// returns the absolute path. Used only when both the embedded binary and the
// PATH lookup have failed (see Resolve). Disabled in dev builds.
//
// The cache key includes downloadVersion so an upgrade doesn't reuse the
// previous version's binary.
func downloadSqld() (string, error) {
	if downloadVersion == "dev" {
		return "", fmt.Errorf("download fallback disabled in dev builds; run `make build-sqld` or install sqld on PATH")
	}
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("no sqld for windows; run sqlitedeploy under WSL2")
	}

	cacheDir, err := userCacheDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(cacheDir, fmt.Sprintf("sqld-v%s-%s-%s%s", downloadVersion, runtime.GOOS, runtime.GOARCH, bundledExt))

	// Reuse cached binary if a previous run already downloaded it.
	if st, err := os.Stat(dst); err == nil && st.Size() >= minBinarySize {
		return dst, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/v%s/sqld-%s-%s%s",
		downloadBaseURL, downloadVersion, runtime.GOOS, runtime.GOARCH, bundledExt)
	fmt.Fprintf(os.Stderr, "[sqlitedeploy] downloading sqld from %s …\n", url)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", url, resp.Status)
	}

	tmp := dst + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}

	st, err := os.Stat(tmp)
	if err != nil || st.Size() < minBinarySize {
		os.Remove(tmp)
		return "", fmt.Errorf("downloaded sqld too small or missing")
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}

func isPlaceholder(data []byte) bool {
	if len(data) < minBinarySize {
		return true
	}
	if bytes.HasPrefix(data, placeholderPrefix) {
		return true
	}
	return false
}

func userCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "sqlitedeploy"), nil
}
