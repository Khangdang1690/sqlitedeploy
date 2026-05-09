//go:build !((linux || darwin) && (amd64 || arm64))

// Fallback for platforms we don't ship a sqld binary for — notably Windows
// (libsql-server does not compile on Windows; see internal/sqld/bin/README.md).
// The empty slice tricks Resolve into the PATH fallback path, where the user
// gets an instructive error if sqld isn't installed.

package sqld

var bundledBinary []byte

const bundledExt = ""
