//go:build windows && arm64

package litestream

import _ "embed"

//go:embed bin/litestream-windows-arm64.exe
var bundledBinary []byte

const bundledExt = ".exe"
