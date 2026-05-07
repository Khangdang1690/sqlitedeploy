//go:build !((linux && amd64) || (linux && arm64) || (darwin && amd64) || (darwin && arm64) || (windows && amd64) || (windows && arm64))

package litestream

// bundledBinary is empty on platforms we don't ship a binary for. Resolve()
// will detect this via isPlaceholder() and fall back to looking for
// `litestream` on PATH.
var bundledBinary []byte

const bundledExt = ""
