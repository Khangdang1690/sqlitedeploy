//go:build darwin && arm64

package litestream

import _ "embed"

//go:embed bin/litestream-darwin-arm64
var bundledBinary []byte

const bundledExt = ""
