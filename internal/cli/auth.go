package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/browser"
	"github.com/Khangdang1690/sqlitedeploy/internal/cloudflare"
	"github.com/Khangdang1690/sqlitedeploy/internal/credentials"
)

// tokenCreateURL is the Cloudflare API tokens dashboard URL we open during
// `auth login`. We deliberately do NOT include `permissionGroupKeys`/`name`
// query params:
//
// Cloudflare's docs at
//   https://developers.cloudflare.com/fundamentals/api/how-to/account-owned-token-template/
// describe a pre-fill mechanism via these query params, but it does not work
// against the current (May 2026) dashboard SPA. Verified empirically: the
// dashboard strips the query string via history.replaceState and lands on the
// bare list page. Sending the user there with broken-promise pre-fill params
// would just confuse them — better to land on the same page they'd reach by
// clicking around manually, with clear in-terminal instructions for what to
// click next.
//
// If Cloudflare ever fixes pre-fill, we can re-enable the params here.
func tokenCreateURL() string {
	return "https://dash.cloudflare.com/profile/api-tokens"
}

// NewAuthCmd builds the `auth` subcommand group: login, logout, status.
func NewAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Cloudflare credentials for managed R2 onboarding",
	}
	cmd.AddCommand(newAuthLoginCmd(), newAuthLogoutCmd(), newAuthStatusCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var (
		tokenFlag string
		noBrowser bool
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a Cloudflare API token (one paste, persisted)",
		Long: `Opens your browser to Cloudflare's API tokens page, walks you through creating
a token with the right permissions, then validates and saves it locally.

Required token permissions:
  - Account / Workers R2 Storage / Edit   (so we can list and create R2 buckets)
  - User / API Tokens / Edit              (so we can create per-bucket scoped tokens)

The pasted token is stored at the path printed below with mode 0600.
Subsequent ` + "`sqlitedeploy up`" + ` commands reuse it across projects.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			deeplink := tokenCreateURL()

			fmt.Println("Cloudflare login")
			fmt.Println()
			fmt.Println("We'll open your browser to dash.cloudflare.com/profile/api-tokens.")
			fmt.Println("Once it loads, do these steps in the dashboard:")
			fmt.Println()
			fmt.Println("  1. Click the blue `Create Token` button (top right).")
			fmt.Println("  2. Scroll to the bottom and pick `Custom Token` -> `Get Started`.")
			fmt.Println("  3. Token name: anything you'll recognize (e.g. `sqlitedeploy`).")
			fmt.Println("  4. Permissions -- click `+ Add more` and add BOTH of these rows:")
			fmt.Println("       Row 1:  Account  ->  Workers R2 Storage  ->  Edit")
			fmt.Println("       Row 2:  User     ->  API Tokens          ->  Edit")
			fmt.Println("  5. Account Resources: `Include` -> pick the account you'll use for R2")
			fmt.Println("     (or `Include` -> `All accounts` if you want to defer that choice).")
			fmt.Println("  6. Leave Client IP Filtering and TTL empty.")
			fmt.Println("  7. Click `Continue to summary` -> `Create Token` -> copy the token value.")
			fmt.Println("  8. Switch back here and paste it.")
			fmt.Println()

			if !noBrowser {
				if err := browser.Open(deeplink); err != nil {
					fmt.Fprintf(os.Stderr, "(couldn't auto-open browser: %v)\n", err)
				}
			}
			fmt.Printf("URL: %s\n\n", deeplink)

			token := strings.TrimSpace(tokenFlag)
			if token == "" {
				v, err := promptSecret("Cloudflare API token")
				if err != nil {
					return err
				}
				token = strings.TrimSpace(v)
			}
			if token == "" {
				return errors.New("no token provided")
			}

			fmt.Println()
			fmt.Println("Validating token...")
			c := cloudflare.New(token)
			accts, err := c.ListAccounts(context.Background())
			if err != nil {
				if cloudflare.IsUnauthorized(err) {
					return fmt.Errorf("the pasted token couldn't authenticate against Cloudflare. Original error: %w", err)
				}
				return err
			}
			if len(accts) == 0 {
				return errors.New("token authenticated, but no accounts are visible to it. Re-check the `Account Resources` step in the token wizard")
			}

			selected := accts[0]
			if len(accts) > 1 {
				fmt.Println()
				fmt.Println("This token sees multiple accounts. Pick which one to use as the default:")
				for i, a := range accts {
					fmt.Printf("  %d. %s (%s)\n", i+1, a.Name, a.ID)
				}
				idxStr, err := promptString("Choice", "1")
				if err != nil {
					return err
				}
				idx, err := strconv.Atoi(strings.TrimSpace(idxStr))
				if err != nil || idx < 1 || idx > len(accts) {
					return fmt.Errorf("invalid choice %q", idxStr)
				}
				selected = accts[idx-1]
			}

			if err := credentials.SaveCloudflare(&credentials.CloudflareCreds{
				APIToken:    token,
				AccountID:   selected.ID,
				AccountName: selected.Name,
			}); err != nil {
				return err
			}

			path, _ := credentials.Path()
			fmt.Println()
			fmt.Printf("Authenticated as %s (account id: %s)\n", selected.Name, selected.ID)
			fmt.Printf("Saved to %s\n", path)
			fmt.Println()
			fmt.Println("Next: run `sqlitedeploy up` in any project to bring SQLite live.")
			return nil
		},
	}
	cmd.Flags().StringVar(&tokenFlag, "token", "", "Cloudflare API token (skip the interactive prompt; CI-friendly)")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "skip auto-opening the browser; just print the URL")
	return cmd
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Forget the saved Cloudflare API token",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := credentials.Delete(); err != nil {
				return err
			}
			path, _ := credentials.Path()
			fmt.Printf("Removed %s\n", path)
			return nil
		},
	}
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which Cloudflare account the saved token authenticates as",
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, err := credentials.Cloudflare()
			if err != nil {
				if errors.Is(err, credentials.ErrNoCloudflare) {
					path, _ := credentials.Path()
					fmt.Printf("Not logged in (no credentials at %s)\n", path)
					fmt.Println("Run `sqlitedeploy auth login` to set up.")
					return nil
				}
				return err
			}

			// Live-validate so we don't lie about staleness.
			c := cloudflare.New(creds.APIToken)
			accts, err := c.ListAccounts(context.Background())
			if err != nil {
				return fmt.Errorf("token validation failed: %w (run `sqlitedeploy auth login` to refresh)", err)
			}

			path, _ := credentials.Path()
			fmt.Printf("Logged in. Credentials: %s\n", path)
			fmt.Printf("Default account: %s (%s)\n", creds.AccountName, creds.AccountID)
			if len(accts) > 1 {
				fmt.Println("All visible accounts:")
				for _, a := range accts {
					marker := ""
					if a.ID == creds.AccountID {
						marker = " (default)"
					}
					fmt.Printf("  - %s (%s)%s\n", a.Name, a.ID, marker)
				}
			}
			return nil
		},
	}
}
