package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/config"
	"github.com/Khangdang1690/sqlitedeploy/internal/litestream"
	"github.com/Khangdang1690/sqlitedeploy/internal/providers"
)

// NewAttachCmd builds the `attach` subcommand: bootstraps a read-replica node.
// Pulls the latest snapshot from object storage and then runs a periodic
// restore loop so the local SQLite file stays near-real-time.
func NewAttachCmd() *cobra.Command {
	var (
		providerKind  string
		bucket        string
		accountID     string
		region        string
		endpoint      string
		accessKey     string
		secretKey     string
		dbPath        string
		replicaPath   string
		pullInterval  int
		oneShot       bool
	)

	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Attach this node as a read replica",
		Long: `Pull the latest snapshot from the object-storage master into a local
SQLite file and keep it near-real-time by re-running ` + "`litestream restore`" + ` on
an interval.

This node is read-only. Writes from a replica are not supported and will not
be replicated back to the master.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(cmd)
			if err != nil {
				return err
			}

			cfg, err := loadOrInitReplicaConfig(dir, &replicaInputs{
				providerKind: providerKind, bucket: bucket, accountID: accountID,
				region: region, endpoint: endpoint, accessKey: accessKey, secretKey: secretKey,
				dbPath: dbPath, replicaPath: replicaPath, pullInterval: pullInterval,
			})
			if err != nil {
				return err
			}

			lsPath, err := litestream.Render(dir, cfg)
			if err != nil {
				return err
			}
			runner, err := litestream.NewRunner(lsPath)
			if err != nil {
				return err
			}

			absDBPath := absDB(dir, cfg.DBPath)
			prov, err := providers.FromConfig(cfg.Provider)
			if err != nil {
				return err
			}
			replicaURL := litestream.ReplicaURL(prov, cfg.ReplicaPath)

			fmt.Printf("→ Initial restore from %s\n", replicaURL)
			if err := runEnvWrappedRestore(runner, absDBPath, replicaURL, true /* ifNotExists */, prov); err != nil {
				return fmt.Errorf("initial restore: %w", err)
			}

			fmt.Println()
			fmt.Println("✓ sqlitedeploy replica attached")
			fmt.Printf("  Database file (read-only): %s\n", absDBPath)
			fmt.Printf("  Connection (URI):          sqlite:///%s?mode=ro\n", filepath.ToSlash(absDBPath))
			fmt.Printf("  Pull interval:             %ds\n", cfg.PullIntervalSeconds)
			fmt.Println()

			if oneShot {
				return nil
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return restoreLoop(ctx, runner, absDBPath, replicaURL, prov, time.Duration(cfg.PullIntervalSeconds)*time.Second)
		},
	}
	addProjectDirFlag(cmd)
	cmd.Flags().StringVar(&providerKind, "provider", "r2", "object storage provider: r2 | b2 | s3")
	cmd.Flags().StringVar(&bucket, "bucket", "", "bucket name (required if no existing config)")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Cloudflare R2 account ID (R2 only)")
	cmd.Flags().StringVar(&region, "region", "", "bucket region (B2/S3)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "S3 endpoint override")
	cmd.Flags().StringVar(&accessKey, "access-key", envOrDefault("SQLITEDEPLOY_ACCESS_KEY", ""), "access key (or env SQLITEDEPLOY_ACCESS_KEY) — read-only key recommended")
	cmd.Flags().StringVar(&secretKey, "secret-key", envOrDefault("SQLITEDEPLOY_SECRET_KEY", ""), "secret key (or env SQLITEDEPLOY_SECRET_KEY)")
	cmd.Flags().StringVar(&dbPath, "db-path", config.DefaultDBPath, "where to put the local replica SQLite file")
	cmd.Flags().StringVar(&replicaPath, "replica-path", config.DefaultReplicaPath, "sub-path within the bucket (must match primary's --replica-path)")
	cmd.Flags().IntVar(&pullInterval, "pull-interval", config.DefaultPullIntervalSeconds, "seconds between restore pulls")
	cmd.Flags().BoolVar(&oneShot, "one-shot", false, "perform initial restore then exit (skip the periodic refresh loop)")
	return cmd
}

type replicaInputs struct {
	providerKind, bucket, accountID, region, endpoint, accessKey, secretKey string
	dbPath, replicaPath                                                     string
	pullInterval                                                            int
}

func loadOrInitReplicaConfig(dir string, in *replicaInputs) (*config.Config, error) {
	if cfg, err := config.Load(dir); err == nil {
		// Existing config — reuse, ignore CLI flags so we don't silently change creds.
		if cfg.Role == "" {
			cfg.Role = config.RoleReplica
		}
		return cfg, nil
	}
	// No config yet — build from flags. Replicas always use the manual path:
	// they need to point at an *existing* primary's bucket, so there's nothing
	// for the managed (auto-create) flow to do here.
	kind, err := providers.ParseKind(in.providerKind)
	if err != nil {
		return nil, err
	}
	prov, err := buildProviderManual(kind, providerInputs{
		kindStr: in.providerKind, bucket: in.bucket, accountID: in.accountID,
		region: in.region, endpoint: in.endpoint,
		accessKey: in.accessKey, secretKey: in.secretKey,
	})
	if err != nil {
		return nil, err
	}
	cfg := &config.Config{
		Role:                config.RoleReplica,
		DBPath:              in.dbPath,
		Provider:            providers.ToConfig(prov),
		ReplicaPath:         in.replicaPath,
		PullIntervalSeconds: in.pullInterval,
	}
	if err := os.MkdirAll(filepath.Dir(absDB(dir, cfg.DBPath)), 0o755); err != nil {
		return nil, err
	}
	if err := config.Save(dir, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// restoreLoop re-runs `litestream restore` on the configured cadence so the
// replica DB stays near-real-time. We tolerate transient errors (network blips
// happen) and only abort on context cancellation.
func restoreLoop(ctx context.Context, r *litestream.Runner, dbPath, replicaURL string, prov providers.Provider, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := runEnvWrappedRestore(r, dbPath, replicaURL, false, prov); err != nil {
				fmt.Fprintf(os.Stderr, "warn: restore tick failed: %v\n", err)
			}
		}
	}
}

// runEnvWrappedRestore invokes Runner.Restore with provider credentials
// exported as AWS-style env vars, so the litestream subprocess picks them up
// even when restoring from a URL (the URL form doesn't carry credentials).
func runEnvWrappedRestore(r *litestream.Runner, dbPath, replicaURL string, ifNotExists bool, prov providers.Provider) error {
	prevAK, hadAK := os.LookupEnv("LITESTREAM_ACCESS_KEY_ID")
	prevSK, hadSK := os.LookupEnv("LITESTREAM_SECRET_ACCESS_KEY")
	os.Setenv("LITESTREAM_ACCESS_KEY_ID", prov.AccessKeyID())
	os.Setenv("LITESTREAM_SECRET_ACCESS_KEY", prov.SecretAccessKey())
	defer func() {
		if hadAK {
			os.Setenv("LITESTREAM_ACCESS_KEY_ID", prevAK)
		} else {
			os.Unsetenv("LITESTREAM_ACCESS_KEY_ID")
		}
		if hadSK {
			os.Setenv("LITESTREAM_SECRET_ACCESS_KEY", prevSK)
		} else {
			os.Unsetenv("LITESTREAM_SECRET_ACCESS_KEY")
		}
	}()

	return r.Restore(context.Background(), dbPath, replicaURL, ifNotExists)
}

func absDB(projectDir, dbPath string) string {
	abs, _ := filepath.Abs(filepath.Join(projectDir, dbPath))
	return abs
}
