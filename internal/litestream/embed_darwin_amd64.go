//go:build darwin && amd64

package litestream

import _ "embed"

//go:embed bin/litestream-darwin-amd64
var bundledBinary []byte

const bundledExt = ""
