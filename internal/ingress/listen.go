package ingress

// listen is the "do nothing — your platform handles ingress" driver. By the
// time we get here sqld is already bound to its listen address; this driver
// just remembers what URL (if any) to echo back in the success banner so
// users on Fly/Render/Cloud Run/etc. see their actual platform domain.
type listen struct {
	publicURL string
}

func newListen(publicURL string) *listen {
	return &listen{publicURL: publicURL}
}

func (l *listen) PublicURL() string { return l.publicURL }
func (l *listen) Stop()              {} // nothing to tear down
