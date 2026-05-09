package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/config"
)

// NewStatusCmd builds the `status` subcommand: prints the configured paths,
// endpoints, and local DB size. The richer "what's in the bucket" view that
// the v1 `litestream ltx` integration provided is replaced in v2 by hitting
// sqld's admin API at cfg.AdminListenAddr — wired up as a follow-up.
func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show configured endpoints, paths, and local DB size",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(cmd)
			if err != nil {
				return err
			}
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			fmt.Printf("Role:              %s\n", cfg.Role)
			fmt.Printf("DB path:           %s\n", absDB(dir, cfg.DBPath))
			fmt.Printf("Provider:          %s\n", cfg.Provider.Kind)
			fmt.Printf("Bucket:            %s\n", cfg.Provider.Bucket)
			fmt.Printf("Bucket prefix:     %s\n", cfg.BucketPrefix)
			fmt.Println()
			switch cfg.Role {
			case config.RolePrimary:
				fmt.Printf("HTTP (Hrana):      http://%s\n", cfg.HTTPListenAddr)
				fmt.Printf("gRPC (replicas):   %s\n", cfg.GRPCListenAddr)
				fmt.Printf("Admin API:         http://%s\n", cfg.AdminListenAddr)
			case config.RoleReplica:
				fmt.Printf("HTTP (Hrana):      http://%s\n", cfg.HTTPListenAddr)
				fmt.Printf("Streaming from:    %s\n", cfg.PrimaryGRPCURL)
				if cfg.PrimaryHranaURL != "" {
					fmt.Printf("Primary Hrana URL: %s\n", cfg.PrimaryHranaURL)
				}
			}
			fmt.Println()

			if st, err := os.Stat(absDB(dir, cfg.DBPath)); err == nil {
				fmt.Printf("DB size (local):   %d bytes\n", st.Size())
			} else {
				fmt.Printf("DB size (local):   n/a (%v)\n", err)
			}
			return nil
		},
	}
	addProjectDirFlag(cmd)
	return cmd
}
