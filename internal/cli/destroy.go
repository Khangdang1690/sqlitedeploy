package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/config"
)

// NewDestroyCmd builds the `destroy` subcommand: removes local sqlitedeploy
// state. Does not touch the bucket — that's the user's account, and any
// purge there should be done deliberately via their provider's tools.
func NewDestroyCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Remove local sqlitedeploy state (config + DB file)",
		Long: `Removes the .sqlitedeploy/ directory and the local SQLite file. Does NOT
delete anything in your object-storage bucket — your master remains intact and
can be recovered with ` + "`sqlitedeploy attach`" + ` or ` + "`restore`" + `.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(cmd)
			if err != nil {
				return err
			}
			cfg, _ := config.Load(dir)

			targets := []string{filepath.Join(dir, config.DirName)}
			if cfg != nil {
				targets = append(targets, absDB(dir, cfg.DBPath))
				targets = append(targets, absDB(dir, cfg.DBPath)+"-wal")
				targets = append(targets, absDB(dir, cfg.DBPath)+"-shm")
			}

			fmt.Println("About to remove:")
			for _, t := range targets {
				fmt.Printf("  - %s\n", t)
			}
			if !yes {
				fmt.Fprint(os.Stderr, "Proceed? [y/N]: ")
				var ans string
				fmt.Scanln(&ans)
				if !strings.EqualFold(strings.TrimSpace(ans), "y") {
					return fmt.Errorf("aborted")
				}
			}
			for _, t := range targets {
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
	return cmd
}
