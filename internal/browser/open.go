// Package browser opens a URL in the user's default web browser.
//
// Best-effort: if the OS doesn't have a default browser configured (e.g.
// headless servers, CI), Open returns the underlying error and the caller
// should fall back to printing the URL.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open launches the default browser pointed at url. The call returns as soon
// as the launcher process exits — it does not wait for the browser to load.
func Open(url string) error {
	switch runtime.GOOS {
	case "windows":
		// `rundll32 url.dll,FileProtocolHandler <url>` is the standard Windows
		// shell entry point for opening URLs. We deliberately do NOT use
		// `cmd /c start "" <url>`: Go's exec.Command on Windows only quotes
		// args that contain spaces, so `start` would receive an unquoted URL
		// and cmd.exe would interpret `&` as a command separator — silently
		// truncating the URL at the first query param. rundll32 takes the URL
		// as a single argv entry with no shell parsing in between.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux", "freebsd", "openbsd", "netbsd":
		return exec.Command("xdg-open", url).Start()
	default:
		return fmt.Errorf("don't know how to open URLs on %s", runtime.GOOS)
	}
}
