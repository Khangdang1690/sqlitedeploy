package litestream

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// bundledBinary is the litestream binary for the current GOOS/GOARCH, embedded
// at build time. It's populated by exactly one of the build-tagged files in
// this package (embed_<os>_<arch>.go). Platforms we don't ship a binary for
// fall through to embed_unsupported.go and end up with an empty slice.
//
// bundledExt is "" on Unix and ".exe" on Windows — used to name the extracted
// file in the user cache dir so callers can re-execute it directly.

// placeholderPrefix marks an in-tree placeholder file (used before
// `make fetch-litestream` / `scripts\fetch-litestream.ps1` has run) so we
// can detect it at runtime and fall back to PATH.
var placeholderPrefix = []byte("PLACEHOLDER")

// minBinarySize is a lower bound for what we'll consider a real binary. Real
// litestream releases are tens of MB.
const minBinarySize = 1 << 20 // 1 MiB

// Resolve returns the absolute path to a runnable `litestream` binary.
//
// Strategy:
//  1. If a real binary was embedded for this platform, extract it to the user
//     cache dir and run it from there.
//  2. Otherwise fall back to looking for `litestream` on PATH.
//  3. If neither works, return an instructive error.
func Resolve() (string, error) {
	if path, err := extractBundled(); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("litestream"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf(
		"no litestream binary available. Either build sqlitedeploy with bundled binaries (run `make fetch-litestream` or `scripts\\fetch-litestream.ps1` then rebuild), or install litestream from https://litestream.io/install/")
}

func extractBundled() (string, error) {
	if isPlaceholder(bundledBinary) {
		return "", fmt.Errorf("no bundled litestream binary for %s/%s", runtime.GOOS, runtime.GOARCH)
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
	return fmt.Sprintf("litestream-%s-%s%s", runtime.GOOS, runtime.GOARCH, bundledExt)
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
