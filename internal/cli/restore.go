package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewRestoreCmd is a stub for v2: the previous Litestream-based one-shot
// restore is gone, but the sqld-based replacement (`sqld --sync-from-storage`
// then exit, or `bottomless-cli restore`) hasn't been implemented yet.
//
// Today the closest thing is `sqlitedeploy attach` on a fresh host: that
// already cold-starts from the bucket via bottomless on first run. For pure
// disaster-recovery on the *primary* host (local DB lost, want to restore
// from bucket), implementation is tracked as Phase 4.5 in the plan file.
func NewRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "(v2) Stub — disaster recovery via bottomless not yet wired",
		Long: `In v1 (Litestream-based), this command did a one-shot
` + "`litestream restore`" + ` from the bucket into the local DB file.

In v2 (sqld-based), the equivalent is to start sqld with --sync-from-storage,
which seeds the local DB from bottomless. For now:

  - On a fresh replica host: just run ` + "`sqlitedeploy attach`" + ` — the first
    attach already does the bottomless cold-start seed automatically.
  - On a primary that lost its local DB: this command will eventually run
    sqld --sync-from-storage --no-listen until the seed completes, then exit.
    Tracked as Phase 4.5 in the plan.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("`sqlitedeploy restore` is not yet implemented in v2 (sqld-based). For replica cold-start, use `sqlitedeploy attach`. See the long help for details.")
		},
	}
	addProjectDirFlag(cmd)
	return cmd
}
