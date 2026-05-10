package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/auth"
	"github.com/Khangdang1690/sqlitedeploy/internal/config"
	"github.com/Khangdang1690/sqlitedeploy/internal/ingress"
	"github.com/Khangdang1690/sqlitedeploy/internal/providers"
	"github.com/Khangdang1690/sqlitedeploy/internal/sqld"
)

// NewUpCmd builds the `up` subcommand: the headline free-tier command.
//
// First run: provisions a Cloudflare R2 bucket (10 GB free, $0 egress),
// generates a JWT keypair, starts sqld with bottomless replication, and opens
// a TryCloudflare quick tunnel so the database has a public HTTPS URL —
// no domain, no public IP, no TLS terminator, no port-forward needed.
//
// Subsequent runs: load the existing config and just resume sqld + tunnel.
//
// All defaults stay within free tiers: R2 free-tier limits, TryCloudflare
// quick tunnels (free, ephemeral), no SaaS subscription anywhere.
func NewUpCmd() *cobra.Command {
	var (
		providerKind   string
		bucket         string
		accountID      string
		region         string
		endpoint       string
		accessKey      string
		secretKey      string
		bucketPrefix   string
		httpListenAddr string
		grpcListenAddr string
		ingressMode    string
		publicURL      string
		noTunnel       bool // deprecated alias for --ingress=listen; kept for backwards compat
		byoStorage     bool
	)
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Provision storage, start sqld, and open a public Cloudflare Tunnel — your SQLite goes live",
		Long: `Bring sqlitedeploy live in one command.

First run:
  1. Authenticates against Cloudflare (uses creds from ` + "`sqlitedeploy auth login`" + `)
  2. Creates an R2 bucket (10 GB free tier, $0 egress) and a scoped access key
  3. Generates an Ed25519 JWT keypair for client + replica auth
  4. Starts the bundled sqld with bottomless replication to the bucket
  5. Opens a Cloudflare quick tunnel (free, ephemeral *.trycloudflare.com URL)
     so apps can reach sqld over HTTPS with no domain, no port-forward, no TLS

Subsequent runs reuse the existing config and just resume the stack.

Pass --ingress=listen to bind sqld on a public port and let your platform's
own HTTPS ingress (Fly auto-TLS, Render, Railway, Cloud Run, your VPS, etc.)
handle the public URL — combine with --public-url=<your-domain> so the
success banner shows the URL clients will actually use. Pass --byo-storage
plus --provider=s3 / b2 / r2 with --access-key / --secret-key / --endpoint
to point at any S3-compatible bucket on any cloud (AWS S3, Backblaze B2,
Tigris, Wasabi, MinIO, your own R2 token).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(cmd)
			if err != nil {
				return err
			}

			// Resolve ingress mode (with legacy --no-tunnel deprecation handling).
			mode := ingress.Mode(ingressMode)
			if noTunnel {
				fmt.Fprintln(os.Stderr, "warning: --no-tunnel is deprecated; use --ingress=listen instead")
				mode = ingress.ModeListen
			}
			switch mode {
			case ingress.ModeTunnel, ingress.ModeListen:
			default:
				return fmt.Errorf("unknown ingress mode %q (valid: tunnel, listen)", ingressMode)
			}

			// Listen mode needs a non-loopback bind so the platform's ingress
			// can reach sqld. Auto-promote when the user hasn't overridden
			// --http-listen-addr.
			userOverrodeBind := cmd.Flags().Changed("http-listen-addr")
			if mode == ingress.ModeListen && !userOverrodeBind && httpListenAddr == config.DefaultHTTPListenAddr {
				httpListenAddr = "0.0.0.0:8080"
				fmt.Fprintln(os.Stderr, "note: --ingress=listen — binding sqld on 0.0.0.0:8080 so your platform's ingress can reach it")
			}

			cfg, isFirstRun, err := loadOrBootstrapPrimary(dir, providerInputs{
				kindStr: providerKind, bucket: bucket, accountID: accountID,
				region: region, endpoint: endpoint,
				accessKey: accessKey, secretKey: secretKey,
				forceManual: byoStorage,
			}, primaryFlags{
				bucketPrefix:   bucketPrefix,
				httpListenAddr: httpListenAddr,
				grpcListenAddr: grpcListenAddr,
			})
			if err != nil {
				return err
			}

			// Subsequent-run safety net: a previously-saved config may still
			// have the loopback default if the prior run was tunnel-mode.
			// Override in-memory only — we don't rewrite config.yml here.
			if mode == ingress.ModeListen && !userOverrodeBind && cfg.HTTPListenAddr == config.DefaultHTTPListenAddr {
				cfg.HTTPListenAddr = "0.0.0.0:8080"
			}

			authFiles, err := auth.BootstrapPrimary(dir, config.DirName)
			if err != nil {
				return fmt.Errorf("bootstrap auth: %w", err)
			}

			// sqld treats DBPath as a directory it owns (creates dbs/<ns>/data
			// inside). We just ensure the parent exists; sqld handles the rest.
			absDBPath := absDB(dir, cfg.DBPath)
			if err := os.MkdirAll(absDBPath, 0o755); err != nil {
				return fmt.Errorf("create db dir: %w", err)
			}
			_ = isFirstRun // bootstrap already wrote config; nothing else to do here

			prov, err := providers.FromConfig(cfg.Provider)
			if err != nil {
				return err
			}
			runner, err := sqld.NewRunner()
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			sqldErr := make(chan error, 1)
			go func() {
				sqldErr <- runner.Serve(ctx, sqld.PrimaryOpts{
					DBPath:          absDBPath,
					HTTPListenAddr:  cfg.HTTPListenAddr,
					HranaListenAddr: cfg.HranaListenAddr,
					GRPCListenAddr:  cfg.GRPCListenAddr,
					AdminListenAddr: cfg.AdminListenAddr,
					AuthJWTKeyFile:  cfg.ResolveAuthPath(dir, cfg.JWTPublicKeyPath),
					BottomlessEnv:   sqld.BottomlessEnv(prov, cfg.BucketPrefix),
				})
			}()

			if err := waitForListener(ctx, cfg.HTTPListenAddr, 15*time.Second); err != nil {
				stop()
				<-sqldErr
				return fmt.Errorf("sqld didn't start listening on %s within 15s: %w", cfg.HTTPListenAddr, err)
			}

			upstream := "http://" + cfg.HTTPListenAddr
			if strings.HasPrefix(cfg.HTTPListenAddr, "0.0.0.0") {
				upstream = "http://" + strings.Replace(cfg.HTTPListenAddr, "0.0.0.0", "127.0.0.1", 1)
			}
			ing, err := ingress.New(ctx, mode, ingress.Opts{
				Upstream:  upstream,
				PublicURL: publicURL,
			})
			if err != nil {
				stop()
				<-sqldErr
				return fmt.Errorf("start ingress: %w", err)
			}

			tokenBytes, _ := os.ReadFile(authFiles.ReplicaToken)
			printUpSuccess(absDBPath, cfg, ing, strings.TrimSpace(string(tokenBytes)))

			select {
			case err := <-sqldErr:
				ing.Stop()
				return err
			case <-ctx.Done():
				ing.Stop()
				<-sqldErr
				return nil
			}
		},
	}

	addProjectDirFlag(cmd)
	cmd.Flags().StringVar(&providerKind, "provider", "r2", "object storage provider: r2 | b2 | s3")
	cmd.Flags().StringVar(&bucket, "bucket", "", "bucket name (managed-mode: optional, defaults derived from project dir)")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Cloudflare R2 account ID (manual mode only)")
	cmd.Flags().StringVar(&region, "region", "", "bucket region (B2/S3)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "S3 endpoint override (S3 only; e.g. for MinIO)")
	cmd.Flags().StringVar(&accessKey, "access-key", envOrDefault("SQLITEDEPLOY_ACCESS_KEY", ""), "access key (or env SQLITEDEPLOY_ACCESS_KEY) — supplying this triggers manual mode")
	cmd.Flags().StringVar(&secretKey, "secret-key", envOrDefault("SQLITEDEPLOY_SECRET_KEY", ""), "secret key (or env SQLITEDEPLOY_SECRET_KEY)")
	cmd.Flags().StringVar(&bucketPrefix, "bucket-prefix", config.DefaultBucketPrefix, "bottomless prefix within the bucket (lets multiple databases share one bucket)")
	cmd.Flags().StringVar(&httpListenAddr, "http-listen-addr", config.DefaultHTTPListenAddr, "where sqld serves Hrana-over-HTTP locally")
	cmd.Flags().StringVar(&grpcListenAddr, "grpc-listen-addr", config.DefaultGRPCListenAddr, "where sqld serves gRPC for replica nodes")
	cmd.Flags().StringVar(&ingressMode, "ingress", envOrDefault("SQLITEDEPLOY_INGRESS", string(ingress.ModeTunnel)), "public-URL strategy: tunnel (Cloudflare quick tunnel, $0, anywhere) | listen (bind a port, your platform's ingress handles HTTPS — Fly/Render/Railway/Cloud Run/your VPS)")
	cmd.Flags().StringVar(&publicURL, "public-url", envOrDefault("SQLITEDEPLOY_PUBLIC_URL", ""), "public HTTPS URL clients should use (listen mode only — e.g. https://db.example.com or your Fly/Render auto-domain)")
	cmd.Flags().BoolVar(&noTunnel, "no-tunnel", false, "deprecated: use --ingress=listen instead")
	_ = cmd.Flags().MarkHidden("no-tunnel")
	cmd.Flags().BoolVar(&byoStorage, "byo-storage", false, "skip the managed R2 flow; supply credentials via --access-key/--secret-key/--account-id")
	return cmd
}

type primaryFlags struct {
	bucketPrefix, httpListenAddr, grpcListenAddr string
}

// loadOrBootstrapPrimary returns the primary's config and whether this is the
// first run (no config existed). On first run it provisions storage, writes
// .sqlitedeploy/config.yml, and creates the SQLite file's parent dir.
func loadOrBootstrapPrimary(dir string, in providerInputs, pf primaryFlags) (*config.Config, bool, error) {
	if cfg, err := config.Load(dir); err == nil {
		if cfg.Role != config.RolePrimary {
			return nil, false, fmt.Errorf("`up` is only valid on primary nodes (this is a %s). Did you mean `sqlitedeploy attach`?", cfg.Role)
		}
		return cfg, false, nil
	}

	prov, err := buildProvider(dir, in)
	if err != nil {
		return nil, false, err
	}
	cfg := &config.Config{
		Role:              config.RolePrimary,
		DBPath:            config.DefaultDBPath,
		Provider:          providers.ToConfig(prov),
		BucketPrefix:      pf.bucketPrefix,
		HTTPListenAddr:    pf.httpListenAddr,
		GRPCListenAddr:    pf.grpcListenAddr,
		AdminListenAddr:   config.DefaultAdminListenAddr,
		JWTPublicKeyPath:  config.DefaultJWTPublicKeyPath,
		JWTPrivateKeyPath: config.DefaultJWTPrivateKeyPath,
	}
	if err := os.MkdirAll(absDB(dir, cfg.DBPath), 0o755); err != nil {
		return nil, false, err
	}
	if err := config.Save(dir, cfg); err != nil {
		return nil, false, err
	}
	return cfg, true, nil
}

// waitForListener polls a TCP address until it accepts a connection or
// timeout fires. Bind addresses like "0.0.0.0:8080" are dialed against
// 127.0.0.1 since dialing 0.0.0.0 is non-portable.
func waitForListener(ctx context.Context, addr string, timeout time.Duration) error {
	dialAddr := addr
	if strings.HasPrefix(addr, "0.0.0.0:") {
		dialAddr = "127.0.0.1:" + strings.TrimPrefix(addr, "0.0.0.0:")
	}
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c, err := net.DialTimeout("tcp", dialAddr, 500*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func printUpSuccess(absDBPath string, cfg *config.Config, ing ingress.Ingress, replicaToken string) {
	fmt.Println()
	fmt.Println("✓ Your SQLite is live!")
	fmt.Println()
	if u := ing.PublicURL(); u != "" {
		display := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
		fmt.Printf("  Public URL:  libsql://%s\n", display)
		if strings.HasPrefix(u, "https://") {
			fmt.Printf("               (HTTPS: %s)\n", u)
		}
	} else {
		// Listen mode without --public-url: point user at their platform's
		// ingress without inventing a URL we don't know.
		fmt.Printf("  Listening:   http://%s\n", cfg.HTTPListenAddr)
		fmt.Println("               (point your platform's HTTPS ingress at this port —")
		fmt.Println("                Fly auto-TLS, Render, Railway, Cloud Run, your reverse proxy, etc.)")
	}
	if replicaToken != "" {
		fmt.Printf("  Auth token:  %s\n", replicaToken)
	}
	fmt.Printf("  Local file:  %s\n", absDBPath)
	fmt.Printf("  Provider:    %s (bucket=%s, prefix=%s)\n", cfg.Provider.Kind, cfg.Provider.Bucket, cfg.BucketPrefix)
	fmt.Println()
	fmt.Println("Ctrl-C to stop · re-run `sqlitedeploy up` to resume · `sqlitedeploy down` to tear down")
	fmt.Println()
}

// absDB resolves cfg.DBPath against the project dir. Lives here so attach.go
// (the only other caller) keeps working after init.go was removed.
func absDB(projectDir, dbPath string) string {
	abs, _ := filepath.Abs(filepath.Join(projectDir, dbPath))
	return abs
}
