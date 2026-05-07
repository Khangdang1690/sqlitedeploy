package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/cloudflare"
	"github.com/Khangdang1690/sqlitedeploy/internal/config"
	"github.com/Khangdang1690/sqlitedeploy/internal/credentials"
	"github.com/Khangdang1690/sqlitedeploy/internal/litestream"
	"github.com/Khangdang1690/sqlitedeploy/internal/providers"
	"github.com/Khangdang1690/sqlitedeploy/internal/sqlitex"
)

// NewInitCmd builds the `init` subcommand: bootstraps a primary node — creates
// the local SQLite file in WAL mode, persists provider credentials, generates
// a Litestream config, and prints a connection string.
//
// Two modes:
//
//	Managed (default for r2): user has run `sqlitedeploy auth login`. The CLI
//	  uses their stored Cloudflare token to list/create a bucket and generate
//	  a per-bucket R2 access key automatically.
//
//	Manual: any of --access-key / --secret-key / --account-id supplied. The
//	  CLI takes those values verbatim and skips the Cloudflare API entirely.
//	  This is the path for B2, generic S3, and CI/automated R2 setups.
func NewInitCmd() *cobra.Command {
	var (
		providerKind string
		bucket       string
		accountID    string
		region       string
		endpoint     string
		accessKey    string
		secretKey    string
		dbPath       string
		replicaPath  string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap the primary node: create the SQLite DB and provider config",
		Long: `Bootstrap a sqlitedeploy primary node.

Creates ./data/app.db in WAL mode, writes ./.sqlitedeploy/config.yml with the
provider credentials, generates ./.sqlitedeploy/litestream.yml, and prints the
connection string your application can use.

Run this once on the node that owns writes. Other nodes use ` + "`sqlitedeploy attach`" + ` instead.

For Cloudflare R2: run ` + "`sqlitedeploy auth login`" + ` first; the CLI will then create
the bucket and a scoped access key for you. Or supply --access-key, --secret-key,
and --account-id to skip the Cloudflare API entirely.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(cmd)
			if err != nil {
				return err
			}

			if _, err := os.Stat(config.Path(dir)); err == nil {
				return fmt.Errorf("config already exists at %s — refusing to overwrite. Run `sqlitedeploy destroy` first or pick a different --project-dir", config.Path(dir))
			}

			prov, err := buildProvider(dir, providerInputs{
				kindStr: providerKind, bucket: bucket, accountID: accountID,
				region: region, endpoint: endpoint,
				accessKey: accessKey, secretKey: secretKey,
			})
			if err != nil {
				return err
			}

			if dbPath == "" {
				dbPath = config.DefaultDBPath
			}
			if replicaPath == "" {
				replicaPath = config.DefaultReplicaPath
			}

			cfg := &config.Config{
				Role:        config.RolePrimary,
				DBPath:      dbPath,
				Provider:    providers.ToConfig(prov),
				ReplicaPath: replicaPath,
			}

			if err := sqlitex.InitDB(filepath.Join(dir, dbPath)); err != nil {
				return fmt.Errorf("init sqlite: %w", err)
			}
			if err := config.Save(dir, cfg); err != nil {
				return err
			}
			lsPath, err := litestream.Render(dir, cfg)
			if err != nil {
				return err
			}

			absDB, _ := filepath.Abs(filepath.Join(dir, dbPath))
			fmt.Println()
			fmt.Println("sqlitedeploy primary initialized")
			fmt.Println()
			fmt.Printf("  Database file:     %s\n", absDB)
			fmt.Printf("  Connection (URI):  sqlite:///%s\n", filepath.ToSlash(absDB))
			fmt.Printf("  Provider:          %s (bucket=%s, path=%s)\n", prov.Kind(), prov.Bucket(), replicaPath)
			fmt.Printf("  Litestream config: %s\n", lsPath)
			fmt.Println()
			fmt.Println("Next steps:")
			fmt.Printf("  1. Start replication:    sqlitedeploy run -C %q\n", dir)
			fmt.Println("  2. Connect from your app using the connection URI above.")
			fmt.Printf("     Python: sqlite3.connect(%q)\n", absDB)
			fmt.Printf("     Node:   new Database(%q)\n", absDB)
			fmt.Printf("     Go:     sql.Open(\"sqlite3\", %q)\n", absDB)
			fmt.Println()
			return nil
		},
	}

	addProjectDirFlag(cmd)
	cmd.Flags().StringVar(&providerKind, "provider", "r2", "object storage provider: r2 | b2 | s3")
	cmd.Flags().StringVar(&bucket, "bucket", "", "bucket name (managed-mode: optional, defaults derived from project dir)")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Cloudflare R2 account ID (manual mode only)")
	cmd.Flags().StringVar(&region, "region", "", "bucket region (B2/S3)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "S3 endpoint override (S3 only; e.g. for MinIO)")
	cmd.Flags().StringVar(&accessKey, "access-key", envOrDefault("SQLITEDEPLOY_ACCESS_KEY", ""), "access key (or env SQLITEDEPLOY_ACCESS_KEY) — supplying this triggers manual mode")
	cmd.Flags().StringVar(&secretKey, "secret-key", envOrDefault("SQLITEDEPLOY_SECRET_KEY", ""), "secret key (or env SQLITEDEPLOY_SECRET_KEY)")
	cmd.Flags().StringVar(&dbPath, "db-path", config.DefaultDBPath, "where to put the SQLite file (relative to project dir)")
	cmd.Flags().StringVar(&replicaPath, "replica-path", config.DefaultReplicaPath, "sub-path within the bucket")
	return cmd
}

type providerInputs struct {
	kindStr, bucket, accountID, region, endpoint, accessKey, secretKey string
}

// buildProvider routes between the managed and manual flows. Managed kicks in
// only for R2 when none of access-key / secret-key / account-id were supplied.
func buildProvider(projectDir string, in providerInputs) (providers.Provider, error) {
	kind, err := providers.ParseKind(in.kindStr)
	if err != nil {
		return nil, err
	}

	manualR2 := in.accessKey != "" || in.secretKey != "" || in.accountID != ""
	if kind == providers.KindR2 && !manualR2 {
		return buildR2Managed(projectDir, in.bucket)
	}
	return buildProviderManual(kind, in)
}

// buildR2Managed runs the Cloudflare-managed onboarding: list/create a bucket
// in the user's account, generate a scoped R2 API token, and return a ready
// R2 Provider. Requires `sqlitedeploy auth login` to have been run.
func buildR2Managed(projectDir, bucketFlag string) (providers.Provider, error) {
	creds, err := credentials.Cloudflare()
	if err != nil {
		if errors.Is(err, credentials.ErrNoCloudflare) {
			return nil, fmt.Errorf(
				"no Cloudflare credentials saved.\n" +
					"\n" +
					"Either run `sqlitedeploy auth login` first (recommended), or supply\n" +
					"--access-key, --secret-key, and --account-id to skip the Cloudflare API")
		}
		return nil, err
	}

	ctx := context.Background()
	cf := cloudflare.New(creds.APIToken)

	// 1. Resolve account: use cached one, or list and pick.
	accountID, accountName := creds.AccountID, creds.AccountName
	if accountID == "" {
		accts, err := cf.ListAccounts(ctx)
		if err != nil {
			return nil, fmt.Errorf("list Cloudflare accounts: %w", err)
		}
		if len(accts) == 0 {
			return nil, fmt.Errorf("token sees no accounts; re-run `sqlitedeploy auth login`")
		}
		picked, err := pickAccount(accts)
		if err != nil {
			return nil, err
		}
		accountID, accountName = picked.ID, picked.Name
	}
	fmt.Printf("Using Cloudflare account: %s\n", accountName)

	// 2. Resolve bucket: list, then either pick existing, use --bucket, or create new.
	existing, err := cf.ListBuckets(ctx, accountID)
	if err != nil {
		if errors.Is(err, cloudflare.ErrR2NotEnabled) {
			return nil, r2NotEnabledMessage(accountID)
		}
		return nil, fmt.Errorf("list R2 buckets: %w", err)
	}
	bucketName, err := chooseBucket(ctx, cf, accountID, projectDir, bucketFlag, existing)
	if err != nil {
		if errors.Is(err, cloudflare.ErrR2NotEnabled) {
			return nil, r2NotEnabledMessage(accountID)
		}
		return nil, err
	}

	// 3. Create a scoped R2 API token for this bucket.
	hostname, _ := os.Hostname()
	tokenName := fmt.Sprintf("sqlitedeploy-%s-%s", bucketName, sanitizeForToken(hostname))
	fmt.Printf("Creating scoped R2 access key (%s)...\n", tokenName)
	tok, err := cf.CreateR2APIToken(ctx, accountID, bucketName, tokenName)
	if err != nil {
		return nil, fmt.Errorf("create R2 API token: %w", err)
	}
	fmt.Printf("  access key id: %s\n", tok.AccessKeyID)

	return providers.NewR2(accountID, bucketName, tok.AccessKeyID, tok.SecretAccessKey), nil
}

// r2NotEnabledMessage builds the friendly error users see when R2 hasn't been
// activated yet. R2 requires a one-time ToS click-through in the dashboard,
// even on accounts whose API tokens already have R2 permissions.
func r2NotEnabledMessage(accountID string) error {
	url := fmt.Sprintf("https://dash.cloudflare.com/%s/r2/overview", accountID)
	return fmt.Errorf(
		"R2 isn't enabled on this Cloudflare account yet.\n"+
			"\n"+
			"Cloudflare requires a one-time activation (free; the 10 GB free tier means\n"+
			"no charges unless you exceed it). To enable:\n"+
			"\n"+
			"  1. Open: %s\n"+
			"  2. Click `Purchase R2 Plan` and accept the terms.\n"+
			"  3. Re-run `sqlitedeploy init`.",
		url)
}

// chooseBucket implements the bucket selection rules:
//
//	bucketFlag != "":     use it; create if not in existing.
//	bucketFlag == "":     show menu of existing + "create new" option.
func chooseBucket(ctx context.Context, cf *cloudflare.Client, accountID, projectDir, bucketFlag string, existing []cloudflare.Bucket) (string, error) {
	if bucketFlag != "" {
		for _, b := range existing {
			if b.Name == bucketFlag {
				fmt.Printf("Reusing existing bucket: %s\n", b.Name)
				return b.Name, nil
			}
		}
		if reason := validateBucketName(bucketFlag); reason != "" {
			return "", fmt.Errorf("--bucket %q is not a valid R2 bucket name: %s", bucketFlag, reason)
		}
		fmt.Printf("Creating bucket: %s\n", bucketFlag)
		if _, err := cf.CreateBucket(ctx, accountID, bucketFlag); err != nil {
			return "", fmt.Errorf("create bucket %q: %w", bucketFlag, err)
		}
		return bucketFlag, nil
	}

	if len(existing) > 0 {
		fmt.Println()
		fmt.Println("Existing R2 buckets:")
		for i, b := range existing {
			fmt.Printf("  %d. %s\n", i+1, b.Name)
		}
		fmt.Printf("  %d. (create new)\n", len(existing)+1)
		choiceStr, err := promptString("Choice", strconv.Itoa(len(existing)+1))
		if err != nil {
			return "", err
		}
		choice, err := strconv.Atoi(strings.TrimSpace(choiceStr))
		if err != nil || choice < 1 || choice > len(existing)+1 {
			return "", fmt.Errorf("invalid choice %q", choiceStr)
		}
		if choice <= len(existing) {
			return existing[choice-1].Name, nil
		}
	}

	defaultName := defaultBucketName(projectDir)
	fmt.Println()
	fmt.Println("R2 bucket name rules: 3-63 chars, lowercase letters / digits / hyphens,")
	fmt.Println("must start and end with a letter or digit.")
	name, err := promptValidBucketName(defaultName)
	if err != nil {
		return "", err
	}
	fmt.Printf("Creating bucket: %s\n", name)
	if _, err := cf.CreateBucket(ctx, accountID, name); err != nil {
		return "", fmt.Errorf("create bucket %q: %w", name, err)
	}
	return name, nil
}

// promptValidBucketName loops until the user types a name that passes the R2
// naming rules (or hits enter to accept the default). Re-prompting locally is
// much friendlier than letting Cloudflare reject and forcing a full re-run.
func promptValidBucketName(defaultName string) (string, error) {
	for {
		raw, err := promptString("New bucket name", defaultName)
		if err != nil {
			return "", err
		}
		name := strings.TrimSpace(raw)
		if name == "" {
			fmt.Println("  bucket name is required")
			continue
		}
		if reason := validateBucketName(name); reason != "" {
			fmt.Printf("  %q is not a valid R2 bucket name: %s\n", name, reason)
			continue
		}
		return name, nil
	}
}

// validateBucketName returns "" when name is a valid R2 bucket name, or a
// short human reason otherwise. Rules cribbed from Cloudflare's R2 docs and
// the matching S3 conventions:
//   - length 3-63 chars
//   - only lowercase a-z, digits 0-9, and hyphens
//   - must start and end with a letter or digit (no leading/trailing hyphen)
func validateBucketName(name string) string {
	if n := len(name); n < 3 || n > 63 {
		return "must be 3-63 characters"
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-':
		case c >= 'A' && c <= 'Z':
			return "uppercase letters are not allowed (use lowercase)"
		default:
			return fmt.Sprintf("character %q is not allowed (only a-z, 0-9, hyphen)", c)
		}
	}
	if name[0] == '-' {
		return "cannot start with a hyphen"
	}
	if name[len(name)-1] == '-' {
		return "cannot end with a hyphen"
	}
	return ""
}

func pickAccount(accts []cloudflare.Account) (cloudflare.Account, error) {
	if len(accts) == 1 {
		return accts[0], nil
	}
	fmt.Println("Pick which Cloudflare account to use:")
	for i, a := range accts {
		fmt.Printf("  %d. %s (%s)\n", i+1, a.Name, a.ID)
	}
	idxStr, err := promptString("Choice", "1")
	if err != nil {
		return cloudflare.Account{}, err
	}
	idx, err := strconv.Atoi(strings.TrimSpace(idxStr))
	if err != nil || idx < 1 || idx > len(accts) {
		return cloudflare.Account{}, fmt.Errorf("invalid choice %q", idxStr)
	}
	return accts[idx-1], nil
}

// defaultBucketName derives "sqlitedeploy-<projectdir-basename>" sanitized for
// R2's bucket naming rules (lowercase, dashes only, 3-63 chars).
func defaultBucketName(projectDir string) string {
	base := filepath.Base(projectDir)
	base = strings.ToLower(base)
	clean := make([]byte, 0, len(base))
	for i := 0; i < len(base); i++ {
		c := base[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			clean = append(clean, c)
		case c == '-' || c == '_':
			clean = append(clean, '-')
		}
	}
	name := "sqlitedeploy-" + string(clean)
	if len(name) > 63 {
		name = name[:63]
	}
	return strings.Trim(name, "-")
}

// sanitizeForToken cleans up a hostname so it's safe to embed in a Cloudflare
// token name (printable, dash-separated).
func sanitizeForToken(s string) string {
	s = strings.ToLower(s)
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			out = append(out, c)
		default:
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "host"
	}
	return string(out)
}

// buildProviderManual is the old explicit-flags path: prompts for any missing
// values and constructs the right Provider. Used by B2, generic S3, and any
// R2 flow that supplied at least one of access-key / secret-key / account-id.
func buildProviderManual(kind providers.Kind, in providerInputs) (providers.Provider, error) {
	bucket := in.bucket
	var err error

	if bucket == "" {
		bucket, err = promptString("Bucket name", "")
		if err != nil {
			return nil, err
		}
	}
	if bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}

	accessKey, secretKey := in.accessKey, in.secretKey
	if accessKey == "" {
		accessKey, err = promptString("Access key ID", "")
		if err != nil {
			return nil, err
		}
	}
	if secretKey == "" {
		secretKey, err = promptSecret("Secret access key")
		if err != nil {
			return nil, err
		}
	}
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("access key and secret key are required")
	}

	switch kind {
	case providers.KindR2:
		accountID := in.accountID
		if accountID == "" {
			accountID, err = promptString("Cloudflare account ID", "")
			if err != nil {
				return nil, err
			}
		}
		if accountID == "" {
			return nil, fmt.Errorf("--account-id is required for R2 (find it at dash.cloudflare.com)")
		}
		return providers.NewR2(accountID, bucket, accessKey, secretKey), nil

	case providers.KindB2:
		region := in.region
		if region == "" {
			region, err = promptString("B2 region (e.g. us-west-001)", "")
			if err != nil {
				return nil, err
			}
		}
		if region == "" {
			return nil, fmt.Errorf("--region is required for B2 (e.g. us-west-001)")
		}
		return providers.NewB2(region, bucket, accessKey, secretKey), nil

	case providers.KindS3:
		region := in.region
		if region == "" {
			region, err = promptString("AWS region", "us-east-1")
			if err != nil {
				return nil, err
			}
		}
		return providers.NewS3(region, bucket, in.endpoint, accessKey, secretKey), nil
	}
	return nil, fmt.Errorf("unreachable: unknown provider kind %q", kind)
}
