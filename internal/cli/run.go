package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/config"
	"github.com/Khangdang1690/sqlitedeploy/internal/litestream"
	"github.com/Khangdang1690/sqlitedeploy/internal/sqlitex"
)

// NewRunCmd builds the `run` subcommand: starts the bundled litestream
// replicate daemon as a foreground process. Refuses on replica nodes.
func NewRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start continuous replication on the primary",
		Long: `Run the bundled litestream binary in `+"`replicate`"+` mode against the
config produced by ` + "`sqlitedeploy init`" + `.

This is a foreground process intended to be supervised by your process manager
(systemd, docker, supervisord, etc.). On Ctrl-C it shuts litestream down
cleanly.`,
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
			lsPath, err := litestream.Render(dir, cfg)
			if err != nil {
				return err
			}
			runner, err := litestream.NewRunner(lsPath)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			fmt.Printf("→ litestream replicate -config %s\n", lsPath)
			return runner.Replicate(ctx)
		},
	}
	addProjectDirFlag(cmd)
	return cmd
}
