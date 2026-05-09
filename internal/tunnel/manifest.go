package tunnel

import (
	"fmt"
	"runtime"
)

// PinnedVersion is the cloudflared release we download when no binary is on
// PATH. Bump this periodically; see scripts/pin-cloudflared.sh (TODO).
//
// Release page: https://github.com/cloudflare/cloudflared/releases
const PinnedVersion = "2024.11.1"

// asset describes one platform's cloudflared download.
type asset struct {
	// URL is the absolute https://github.com/.../releases/download/<v>/<file>
	// URL for this platform's binary.
	URL string
	// Tarred is true when URL points at a .tgz archive that contains a single
	// file named "cloudflared". Currently only macOS releases are tarred.
	Tarred bool
	// SHA256 is the hex-encoded sha256 of the downloaded file (the tarball
	// itself when Tarred). Empty until we pin via scripts/pin-cloudflared.sh.
	// When empty, downloads succeed but we log a warning — never silently skip.
	SHA256 string
}

// assets maps GOOS/GOARCH pairs to their cloudflared release asset. Entries
// missing from this table fall back to PATH lookup or error out.
var assets = map[string]asset{
	"linux/amd64":   {URL: urlFor("cloudflared-linux-amd64")},
	"linux/arm64":   {URL: urlFor("cloudflared-linux-arm64")},
	"linux/arm":     {URL: urlFor("cloudflared-linux-armhf")},
	"linux/386":     {URL: urlFor("cloudflared-linux-386")},
	"darwin/amd64":  {URL: urlFor("cloudflared-darwin-amd64.tgz"), Tarred: true},
	"darwin/arm64":  {URL: urlFor("cloudflared-darwin-arm64.tgz"), Tarred: true},
	"windows/amd64": {URL: urlFor("cloudflared-windows-amd64.exe")},
	"windows/386":   {URL: urlFor("cloudflared-windows-386.exe")},
}

func urlFor(file string) string {
	return fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/download/%s/%s", PinnedVersion, file)
}

// platformKey returns the GOOS/GOARCH key used to look up assets.
func platformKey() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// assetForCurrent returns the cloudflared asset for this build, or false if
// the platform isn't in our table (caller should fall back to PATH or error).
func assetForCurrent() (asset, bool) {
	a, ok := assets[platformKey()]
	return a, ok
}
