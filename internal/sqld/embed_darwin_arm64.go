//go:build darwin && arm64

package sqld

import _ "embed"

//go:embed bin/sqld-darwin-arm64
var bundledBinary []byte

const bundledExt = ""
