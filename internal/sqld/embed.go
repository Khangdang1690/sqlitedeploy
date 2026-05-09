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

// Resolve returns the absolute path to a runnable `sqld` binary.
//
// Strategy:
//  1. If a real binary was embedded for this platform, extract it to the user
//     cache dir and run it from there.
//  2. Otherwise fall back to looking for `sqld` on PATH.
//  3. If neither works, return an instructive error.
func Resolve() (string, error) {
	if path, err := extractBundled(); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("sqld"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf(
		"no sqld binary available for %s/%s. Either build sqlitedeploy with bundled binaries (run `make build-sqld` then rebuild) or install sqld separately and put it on PATH. On Windows, run sqlitedeploy under WSL — libsql-server does not support Windows natively.",
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
