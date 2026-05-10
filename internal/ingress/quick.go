package ingress

import (
	"context"

	"github.com/Khangdang1690/sqlitedeploy/internal/tunnel"
)

// quickTunnel adapts tunnel.QuickTunnel to the Ingress interface. The actual
// cloudflared subprocess lifecycle stays in the tunnel package; this is just
// a thin wrapper so the cli layer doesn't need to know which driver it has.
type quickTunnel struct {
	qt *tunnel.QuickTunnel
}

func newQuick(ctx context.Context, upstream string) (*quickTunnel, error) {
	qt, err := tunnel.RunQuick(ctx, upstream)
	if err != nil {
		return nil, err
	}
	return &quickTunnel{qt: qt}, nil
}

func (q *quickTunnel) PublicURL() string { return q.qt.PublicURL }
func (q *quickTunnel) Stop()              { q.qt.Stop() }
