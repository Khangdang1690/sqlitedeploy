// Command sqlitedeploy bootstraps a free, distributed SQLite database whose
// durable master lives in user-owned object storage and whose working copy
// lives on the application's disk.
//
// See https://github.com/Khangdang1690/sqlitedeploy for the full architecture.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/cli"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:           "sqlitedeploy",
		Short:         "Free, distributed SQLite via object storage + Litestream",
		Long: `sqlitedeploy bootstraps a SQLite database whose durable master lives in
your own object-storage bucket (Cloudflare R2 / Backblaze B2 / S3) and whose
working copy lives next to your application.

The primary node owns writes; replica nodes pull near-real-time copies. Any
language with a SQLite driver can connect to the local file directly.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		cli.NewAuthCmd(),
		cli.NewInitCmd(),
		cli.NewRunCmd(),
		cli.NewAttachCmd(),
		cli.NewStatusCmd(),
		cli.NewRestoreCmd(),
		cli.NewDestroyCmd(),
	)
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
