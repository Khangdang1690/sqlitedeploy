//go:build linux && arm64

package sqld

import _ "embed"

//go:embed bin/sqld-linux-arm64
var bundledBinary []byte

const bundledExt = ""
