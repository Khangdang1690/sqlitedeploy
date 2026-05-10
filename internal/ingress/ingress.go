// Package ingress abstracts "how does sqld get a public URL?" so sqlitedeploy
// can deploy to any cloud, not just Cloudflare.
//
// Two drivers ship today:
//
//   - tunnel: outbound-only Cloudflare quick tunnel. Works anywhere with
//     outbound HTTPS (laptop, home server, container with no public IP).
//     Free, ephemeral *.trycloudflare.com hostname. The default.
//   - listen: bind sqld on a public port and let the user's existing platform
//     ingress (Fly auto-TLS, Render, Railway, Cloud Run, ECS+ALB, your own
//     reverse proxy, etc.) terminate TLS and route traffic. sqlitedeploy
//     provisions nothing — the cloud-portable mode.
package ingress

import (
	"context"
	"fmt"
)

// Mode selects an ingress driver.
type Mode string

const (
	ModeTunnel Mode = "tunnel"
	ModeListen Mode = "listen"
)

// Ingress is a running public-endpoint strategy. The caller's lifecycle is:
// New → use PublicURL → Stop on shutdown.
type Ingress interface {
	// PublicURL returns the URL clients should use to reach sqld. May be empty
	// when the listen driver was started without a --public-url hint, in which
	// case the CLI prints local-port guidance instead.
	PublicURL() string
	// Stop tears down any resources the driver holds (e.g. the cloudflared
	// subprocess for tunnel mode). Safe to call multiple times. Listen mode
	// is a noop here since there's nothing to tear down.
	Stop()
}

// Opts is the union of fields the drivers might need. Each driver ignores
// fields that don't apply to it — keeping the constructor signature flat
// avoids per-driver Opts types just to express two flags.
type Opts struct {
	// Upstream is the local HTTP endpoint cloudflared (tunnel mode) proxies to,
	// e.g. "http://127.0.0.1:8080". Ignored by listen mode.
	Upstream string
	// PublicURL is the URL the listen driver echoes back to the user (e.g.
	// their Fly app URL or custom domain). Ignored by tunnel mode, where the
	// URL is auto-detected from cloudflared's output.
	PublicURL string
}

// New starts an ingress driver. Tunnel mode blocks until cloudflared reports
// its hostname (or times out, ~30s). Listen mode returns immediately.
func New(ctx context.Context, mode Mode, opts Opts) (Ingress, error) {
	switch mode {
	case ModeTunnel:
		return newQuick(ctx, opts.Upstream)
	case ModeListen:
		return newListen(opts.PublicURL), nil
	default:
		return nil, fmt.Errorf("unknown ingress mode %q (valid: tunnel, listen)", mode)
	}
}
