package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/cloudflare"
	"github.com/Khangdang1690/sqlitedeploy/internal/config"
	"github.com/Khangdang1690/sqlitedeploy/internal/credentials"
	"github.com/Khangdang1690/sqlitedeploy/internal/providers"
)

// NewDownCmd builds the `down` subcommand: tears down what `up` brought up.
//
// Default behavior is "stop" only — sqld and the tunnel exit cleanly when the
// `up` process is interrupted, so this command is mainly used after the fact
// to delete local state. Pass --wipe to also delete the R2 bucket.
//
// Buckets are NEVER deleted without --wipe — the bucket is the durable
// master copy of the database, and a stray `down` shouldn't lose data.
func NewDownCmd() *cobra.Command {
	var (
		yes        bool
		wipeBucket bool
	)
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Tear down local sqlitedeploy state (and optionally the R2 bucket)",
		Long: `Removes the .sqlitedeploy/ directory (config, JWT keypair, local DB, WAL).

By default the R2 bucket is left untouched — it's the durable master copy of
your database. Pass --wipe to also delete the bucket and all objects in it.

This command does NOT stop a running ` + "`sqlitedeploy up`" + `; Ctrl-C that process
first, then run ` + "`down`" + `.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(cmd)
			if err != nil {
				return err
			}
			cfg, _ := config.Load(dir)

			localTargets := []string{filepath.Join(dir, config.DirName)}

			fmt.Println("About to remove (local):")
			for _, t := range localTargets {
				fmt.Printf("  - %s\n", t)
			}
			if wipeBucket {
				if cfg == nil {
					return fmt.Errorf("--wipe needs a config to know which bucket to delete; nothing found at %s", config.Path(dir))
				}
				fmt.Printf("About to remove (cloud, --wipe):\n  - %s bucket: %s\n", cfg.Provider.Kind, cfg.Provider.Bucket)
			}
			if !yes {
				fmt.Fprint(os.Stderr, "Proceed? [y/N]: ")
				var ans string
				fmt.Scanln(&ans)
				if !strings.EqualFold(strings.TrimSpace(ans), "y") {
					return fmt.Errorf("aborted")
				}
			}

			if wipeBucket {
				if err := wipeR2Bucket(cfg); err != nil {
					return fmt.Errorf("wipe bucket: %w", err)
				}
				fmt.Printf("✓ deleted bucket %s\n", cfg.Provider.Bucket)
			}

			for _, t := range localTargets {
				if err := os.RemoveAll(t); err != nil && !os.IsNotExist(err) {
					fmt.Fprintf(os.Stderr, "warn: remove %s: %v\n", t, err)
				}
			}
			fmt.Println("✓ local sqlitedeploy state removed")
			return nil
		},
	}
	addProjectDirFlag(cmd)
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	cmd.Flags().BoolVar(&wipeBucket, "wipe", false, "also delete the R2 bucket and all objects in it")
	return cmd
}

// wipeR2Bucket deletes the R2 bucket referenced by cfg via the user's stored
// Cloudflare token. Returns a clear error for non-R2 providers since we don't
// (yet) automate B2/S3 wipes.
func wipeR2Bucket(cfg *config.Config) error {
	if providers.Kind(cfg.Provider.Kind) != providers.KindR2 {
		return fmt.Errorf("--wipe currently supports R2 only; for %s buckets, delete manually via your provider's console", cfg.Provider.Kind)
	}
	creds, err := credentials.Cloudflare()
	if err != nil {
		return fmt.Errorf("need Cloudflare credentials to wipe (run `sqlitedeploy auth login`): %w", err)
	}
	cf := cloudflare.New(creds.APIToken)
	accountID := creds.AccountID
	if accountID == "" {
		return fmt.Errorf("no account ID cached; re-run `sqlitedeploy auth login`")
	}
	return cf.DeleteBucket(context.Background(), accountID, cfg.Provider.Bucket)
}
