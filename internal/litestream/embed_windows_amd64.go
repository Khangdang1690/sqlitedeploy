//go:build windows && amd64

package litestream

import _ "embed"

//go:embed bin/litestream-windows-amd64.exe
var bundledBinary []byte

const bundledExt = ".exe"
