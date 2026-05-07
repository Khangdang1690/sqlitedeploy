package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/config"
	"github.com/Khangdang1690/sqlitedeploy/internal/litestream"
	"github.com/Khangdang1690/sqlitedeploy/internal/providers"
)

// NewRestoreCmd builds the `restore` subcommand: pulls the latest (or
// point-in-time) snapshot from object storage into the configured DB path.
// Used for disaster recovery and on-demand replica refreshes.
func NewRestoreCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore the local SQLite file from the object-storage master",
		Long: `Pulls the latest snapshot+WAL from object storage into the configured
local DB path. Use this for disaster recovery after losing the local DB, or
to refresh a replica on demand.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(cmd)
			if err != nil {
				return err
			}
			cfg, err := config.Load(dir)
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
			prov, err := providers.FromConfig(cfg.Provider)
			if err != nil {
				return err
			}
			replicaURL := litestream.ReplicaURL(prov, cfg.ReplicaPath)
			ifNotExists := !force
			fmt.Printf("→ Restoring %s from %s\n", absDB(dir, cfg.DBPath), replicaURL)
			return runEnvWrappedRestore(runner, absDB(dir, cfg.DBPath), replicaURL, ifNotExists, prov)
		},
	}
	addProjectDirFlag(cmd)
	cmd.Flags().BoolVar(&force, "force", false, "overwrite the local DB if it exists")
	return cmd
}
