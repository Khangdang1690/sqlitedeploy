//go:build linux && amd64

package sqld

import _ "embed"

//go:embed bin/sqld-linux-amd64
var bundledBinary []byte

const bundledExt = ""
