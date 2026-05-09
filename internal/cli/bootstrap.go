package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Khangdang1690/sqlitedeploy/internal/cloudflare"
	"github.com/Khangdang1690/sqlitedeploy/internal/credentials"
	"github.com/Khangdang1690/sqlitedeploy/internal/providers"
)

// providerInputs is the user-supplied raw flags routed to either the managed
// (Cloudflare-onboarded) or manual provider construction paths.
type providerInputs struct {
	kindStr, bucket, accountID, region, endpoint, accessKey, secretKey string
	// forceManual skips the managed flow even when no creds are given. Set by
	// `up --byo-storage` for users who want the old prompt-for-creds UX.
	forceManual bool
}

// buildProvider routes between the managed and manual flows. Managed kicks in
// only for R2 when none of access-key / secret-key / account-id were supplied
// and forceManual isn't set.
func buildProvider(projectDir string, in providerInputs) (providers.Provider, error) {
	kind, err := providers.ParseKind(in.kindStr)
	if err != nil {
		return nil, err
	}

	manualR2 := in.accessKey != "" || in.secretKey != "" || in.accountID != ""
	if kind == providers.KindR2 && !manualR2 && !in.forceManual {
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
			"Cloudflare requires a one-time activation (free; the 10 GB free tier\n"+
			"means no charges unless you exceed it). To enable:\n"+
			"\n"+
			"  1. Open: %s\n"+
			"  2. Click `Purchase R2 Plan` and accept the terms.\n"+
			"  3. Re-run `sqlitedeploy up`.",
		url)
}

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
// short human reason otherwise.
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

// buildProviderManual is the explicit-flags path: prompts for any missing
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
