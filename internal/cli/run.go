package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/config"
	"github.com/Khangdang1690/sqlitedeploy/internal/providers"
	"github.com/Khangdang1690/sqlitedeploy/internal/sqld"
	"github.com/Khangdang1690/sqlitedeploy/internal/sqlitex"
)

// NewRunCmd builds the `run` subcommand: starts the bundled sqld server as a
// foreground process, with bottomless replication writing to the user's
// bucket. Refuses on replica nodes.
func NewRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start sqld in primary mode with bottomless replication",
		Long: `Run the bundled libsql-server (sqld) binary in primary mode against the
config produced by ` + "`sqlitedeploy init`" + `.

sqld serves:
  - Hrana over HTTP for apps and edge clients (Workers / Lambda)
  - gRPC for replica nodes that stream WAL frames live
  - Continuous bottomless replication to your S3-compatible bucket

This is a foreground process intended to be supervised by your process manager
(systemd, docker, supervisord, etc.). On Ctrl-C it shuts sqld down cleanly.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(cmd)
			if err != nil {
				return err
			}
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			if cfg.Role != config.RolePrimary {
				return fmt.Errorf("`run` is only valid on primary nodes (this is a %s). Did you mean `sqlitedeploy attach`?", cfg.Role)
			}
			if err := sqlitex.VerifyWAL(absDB(dir, cfg.DBPath)); err != nil {
				return err
			}
			prov, err := providers.FromConfig(cfg.Provider)
			if err != nil {
				return err
			}

			runner, err := sqld.NewRunner()
			if err != nil {
				return err
			}

			opts := sqld.PrimaryOpts{
				DBPath:          absDB(dir, cfg.DBPath),
				HTTPListenAddr:  cfg.HTTPListenAddr,
				HranaListenAddr: cfg.HranaListenAddr,
				GRPCListenAddr:  cfg.GRPCListenAddr,
				AdminListenAddr: cfg.AdminListenAddr,
				AuthJWTKeyFile:  cfg.ResolveAuthPath(dir, cfg.JWTPublicKeyPath),
				BottomlessEnv:   sqld.BottomlessEnv(prov, cfg.BucketPrefix),
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			fmt.Printf("→ sqld --db-path %s --http-listen-addr %s --grpc-listen-addr %s --enable-bottomless-replication\n",
				opts.DBPath, opts.HTTPListenAddr, opts.GRPCListenAddr)
			return runner.Serve(ctx, opts)
		},
	}
	addProjectDirFlag(cmd)
	return cmd
}
