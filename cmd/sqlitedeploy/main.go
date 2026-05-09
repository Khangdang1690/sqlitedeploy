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
		Short:         "Distributed SQLite (sqld + bottomless) over your own object-storage bucket",
		Long: `sqlitedeploy bootstraps a SQLite database whose durable backup lives in
your own object-storage bucket (Cloudflare R2 / Backblaze B2 / S3). It bundles
sqld (libsql-server) so apps + edge clients connect over Hrana HTTP, replicas
stream WAL frames live from the primary's gRPC endpoint, and any language with
a SQLite driver can still open the local file directly.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		cli.NewAuthCmd(),
		cli.NewDevCmd(),
		cli.NewUpCmd(),
		cli.NewDownCmd(),
		cli.NewAttachCmd(),
		cli.NewStatusCmd(),
	)
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
