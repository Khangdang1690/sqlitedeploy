package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// trycloudflareURL matches the public hostname cloudflared prints in its log
// output once a quick tunnel is up. cloudflared logs roughly:
//
//	... INF |  https://example-words-of-the-day.trycloudflare.com  |
var trycloudflareURL = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

// QuickTunnel represents a running TryCloudflare quick tunnel. The PublicURL
// is the HTTPS hostname users (and their app code) connect to.
type QuickTunnel struct {
	PublicURL string
	stop      func()
	done      chan error
}

// Wait blocks until the underlying cloudflared process exits or ctx is
// cancelled, returning the process error (if any).
func (q *QuickTunnel) Wait() error {
	return <-q.done
}

// Stop cancels the underlying process. Safe to call multiple times.
func (q *QuickTunnel) Stop() { q.stop() }

// RunQuick starts `cloudflared tunnel --url <upstream>` and waits up to ~30s
// for it to print the public *.trycloudflare.com hostname. Returns a handle
// the caller uses to wait on / stop the tunnel.
//
// upstream is the local HTTP endpoint cloudflared should proxy to, e.g.
// "http://127.0.0.1:8080".
func RunQuick(ctx context.Context, upstream string) (*QuickTunnel, error) {
	bin, err := Resolve()
	if err != nil {
		return nil, err
	}

	// cloudflared logs are noisy; --no-autoupdate avoids the daemon trying to
	// upgrade itself on every run, which it has no business doing here.
	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, bin,
		"tunnel",
		"--no-autoupdate",
		"--url", upstream,
	)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	// We don't expect anything useful on stdout, but pipe it so the buffer
	// doesn't fill and block.
	cmd.Stdout = os.Stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start cloudflared: %w", err)
	}

	urlCh := make(chan string, 1)
	done := make(chan error, 1)

	// Tee stderr: scan for the public URL, then forward subsequent lines so
	// users see cloudflared's heartbeat / connection logs in their terminal.
	go scanStderr(stderr, urlCh)

	go func() {
		err := cmd.Wait()
		if err != nil && runCtx.Err() == nil {
			done <- fmt.Errorf("cloudflared exited: %w", err)
		} else {
			done <- nil
		}
		// Belt-and-braces: unblock anyone still waiting on urlCh.
		select {
		case urlCh <- "":
		default:
		}
	}()

	stopOnce := sync.Once{}
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			// Give cloudflared a brief grace period; CommandContext SIGKILLs
			// it on cancellation otherwise.
			_ = cmd.Process.Signal(os.Interrupt)
		})
	}

	select {
	case u := <-urlCh:
		if u == "" {
			stop()
			return nil, fmt.Errorf("cloudflared exited before reporting a public URL — see logs above")
		}
		return &QuickTunnel{PublicURL: u, stop: stop, done: done}, nil
	case <-time.After(30 * time.Second):
		stop()
		return nil, fmt.Errorf("timed out waiting for cloudflared to report a public URL (30s) — check network connectivity")
	case <-ctx.Done():
		stop()
		return nil, ctx.Err()
	}
}

// scanStderr reads cloudflared's stderr line-by-line. The first matching
// trycloudflare URL is sent on urlCh; subsequent lines are forwarded to the
// parent process's stderr so users see live heartbeat logs.
func scanStderr(r io.Reader, urlCh chan<- string) {
	br := bufio.NewReader(r)
	sent := false
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			if !sent {
				if m := trycloudflareURL.FindString(line); m != "" {
					urlCh <- m
					sent = true
				}
			}
			// Forward (with a "cf: " prefix so it's distinguishable from sqld
			// output in interleaved logs).
			fmt.Fprint(os.Stderr, "cf: ", strings.TrimRight(line, "\n"), "\n")
		}
		if err != nil {
			if !sent {
				// Drop a sentinel so RunQuick's select can give up cleanly.
				select {
				case urlCh <- "":
				default:
				}
			}
			return
		}
	}
}
