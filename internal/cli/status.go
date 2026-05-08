package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/config"
	"github.com/Khangdang1690/sqlitedeploy/internal/litestream"
)

// NewStatusCmd builds the `status` subcommand: dumps the configured paths,
// local DB size, and litestream's view of replicated LTX files.
func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show config, local DB stats, and replicated LTX files from object storage",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(cmd)
			if err != nil {
				return err
			}
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			fmt.Printf("Role:            %s\n", cfg.Role)
			fmt.Printf("DB path:         %s\n", absDB(dir, cfg.DBPath))
			fmt.Printf("Provider:        %s\n", cfg.Provider.Kind)
			fmt.Printf("Bucket:          %s\n", cfg.Provider.Bucket)
			fmt.Printf("Replica path:    %s\n", cfg.ReplicaPath)
			if st, err := os.Stat(absDB(dir, cfg.DBPath)); err == nil {
				fmt.Printf("DB size (local): %d bytes\n", st.Size())
			} else {
				fmt.Printf("DB size (local): n/a (%v)\n", err)
			}

			lsPath, err := litestream.Render(dir, cfg)
			if err != nil {
				return err
			}
			runner, err := litestream.NewRunner(lsPath)
			if err != nil {
				return err
			}
			out, err := runner.LTXFiles(context.Background(), absDB(dir, cfg.DBPath))
			fmt.Println()
			fmt.Println("Replicated LTX files:")
			if err != nil {
				fmt.Printf("  (failed to list: %v)\n", err)
				fmt.Println("  Note: this usually means replication hasn't started yet —")
				fmt.Println("        run `sqlitedeploy run` to begin replicating to your bucket.")
			} else if len(out) == 0 {
				fmt.Println("  (none yet — nothing has been replicated to your bucket)")
			} else {
				fmt.Print(string(out))
			}
			return nil
		},
	}
	addProjectDirFlag(cmd)
	return cmd
}
