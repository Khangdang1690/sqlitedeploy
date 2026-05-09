package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/config"
	"github.com/Khangdang1690/sqlitedeploy/internal/sqld"
)

// NewDevCmd builds the `dev` subcommand: the zero-signup, $0, no-cloud entry
// point. Spins up the bundled sqld against a local SQLite file with no auth
// and no bottomless replication. Meant as the first 60 seconds of the user's
// experience — try it locally before deciding whether to go live with `up`.
func NewDevCmd() *cobra.Command {
	var (
		dbPath         string
		httpListenAddr string
		reset          bool
	)
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Run sqld locally with no cloud, no auth — instant SQLite-as-a-service",
		Long: `Spins up the bundled sqld against a local SQLite file. No bucket, no auth,
no Cloudflare account, no signup — everything runs on this machine and costs
$0. The DB persists between runs; pass --reset to wipe.

Use this to try out sqlitedeploy before running ` + "`sqlitedeploy up`" + `, or for
local development where you don't want a real cloud-replicated database yet.

Apps connect over Hrana-over-HTTP at the URL printed below, or open the local
SQLite file directly with any sqlite3 driver.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(cmd)
			if err != nil {
				return err
			}

			absDBPath := filepath.Join(dir, dbPath)
			if reset {
				if err := os.RemoveAll(absDBPath); err != nil {
					return fmt.Errorf("reset: remove %s: %w", absDBPath, err)
				}
			}
			if err := os.MkdirAll(absDBPath, 0o755); err != nil {
				return fmt.Errorf("create db dir: %w", err)
			}

			runner, err := sqld.NewRunner()
			if err != nil {
				return err
			}

			fmt.Println()
			fmt.Println("sqlitedeploy dev (no auth, no bucket, $0)")
			fmt.Printf("  Database dir: %s (sqld manages it, persists between runs)\n", absDBPath)
			fmt.Printf("  Hrana HTTP:   http://%s\n", httpListenAddr)
			fmt.Printf("  Connect:      libsql://%s\n", httpListenAddr)
			fmt.Println("  Ctrl-C to stop · `--reset` to wipe")
			fmt.Println()

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return runner.Serve(ctx, sqld.PrimaryOpts{
				DBPath:         absDBPath,
				HTTPListenAddr: httpListenAddr,
			})
		},
	}
	addProjectDirFlag(cmd)
	cmd.Flags().StringVar(&dbPath, "db", config.DefaultDBPath, "where to put the SQLite file (relative to project dir)")
	cmd.Flags().StringVar(&httpListenAddr, "http-listen-addr", config.DefaultHTTPListenAddr, "where sqld serves Hrana-over-HTTP")
	cmd.Flags().BoolVar(&reset, "reset", false, "delete the local DB before starting")
	return cmd
}
