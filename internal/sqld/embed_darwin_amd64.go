//go:build darwin && amd64

package sqld

import _ "embed"

//go:embed bin/sqld-darwin-amd64
var bundledBinary []byte

const bundledExt = ""
