//go:build linux && arm64

package litestream

import _ "embed"

//go:embed bin/litestream-linux-arm64
var bundledBinary []byte

const bundledExt = ""
