//go:build linux && amd64

package litestream

import _ "embed"

//go:embed bin/litestream-linux-amd64
var bundledBinary []byte

const bundledExt = ""
