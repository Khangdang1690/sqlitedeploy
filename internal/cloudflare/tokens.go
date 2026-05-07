package cloudflare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Permission group NAMES (not UUIDs — we resolve UUIDs at runtime via the
// permission-groups catalog endpoint so Cloudflare can rotate IDs without
// breaking us). Litestream needs both reads and writes against the bucket:
// list segments, get snapshots/segments, put new segments.
const (
	permNameR2BucketItemRead  = "Workers R2 Storage Bucket Item Read"
	permNameR2BucketItemWrite = "Workers R2 Storage Bucket Item Write"
)

// CreateTokenResult is what the user gets back from a successful token-create:
// the parts we need to derive S3-compatible credentials.
type CreateTokenResult struct {
	AccessKeyID     string // = Cloudflare token "id"
	SecretAccessKey string // = sha256-hex(Cloudflare token "value")
	TokenID         string // same as AccessKeyID; kept for clarity in logs
}

// rawCreateTokenResp is the API response shape for POST /user/tokens.
type rawCreateTokenResp struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

// CreateR2APIToken creates a Cloudflare user-owned API token scoped to a
// single R2 bucket and derives the corresponding S3-compatible Access Key ID
// + Secret Access Key. The user will see this token in their dashboard under
// `My Profile -> API Tokens` and can revoke it at any time.
//
// We use the user-tokens endpoint (`/user/tokens`) rather than the account-
// tokens endpoint because:
//
//   - The parent permission needed is `api_tokens:edit` (user-scoped), which
//     pre-fills cleanly via Cloudflare's deeplink — `account_api_tokens:edit`
//     does not appear in any verified-working real-world deeplink.
//   - For a personal CLI like sqlitedeploy, a user-owned token is the natural
//     ownership: it follows the user, not an org/account.
//   - The child token's bucket scope is unchanged — it's enforced by the
//     `resources` field in the policy, not by which endpoint created it.
//
// tokenName shows up in the Cloudflare dashboard, so include enough info that
// the user can identify what created it (e.g. "sqlitedeploy-mybucket-laptop").
func (c *Client) CreateR2APIToken(ctx context.Context, accountID, bucketName, tokenName string) (CreateTokenResult, error) {
	// Resolve the two permission group UUIDs by name from Cloudflare's
	// user-token catalog. This avoids hardcoding UUIDs that Cloudflare could
	// rotate, and gives us a clear error if a permission ever disappears.
	// We use the user-token catalog (not account-token) to match the namespace
	// of POST /user/tokens below; see ListUserTokenPermissionGroups.
	groups, err := c.ListUserTokenPermissionGroups(ctx)
	if err != nil {
		return CreateTokenResult{}, fmt.Errorf("list permission groups: %w", err)
	}
	readID := FindPermissionGroupID(groups, permNameR2BucketItemRead)
	writeID := FindPermissionGroupID(groups, permNameR2BucketItemWrite)
	if readID == "" || writeID == "" {
		return CreateTokenResult{}, fmt.Errorf(
			"required R2 permission groups not found in account catalog (read=%q write=%q). "+
				"This usually means Cloudflare changed the permission group names; "+
				"please file an issue.",
			permNameR2BucketItemRead, permNameR2BucketItemWrite)
	}

	resourceKey := fmt.Sprintf("com.cloudflare.edge.r2.bucket.%s_default_%s", accountID, bucketName)

	body := map[string]any{
		"name": tokenName,
		"policies": []map[string]any{{
			"effect": "allow",
			"resources": map[string]string{
				resourceKey: "*",
			},
			"permission_groups": []map[string]string{
				{"id": readID},
				{"id": writeID},
			},
		}},
	}

	raw, err := do[rawCreateTokenResp](ctx, c, "POST", "/user/tokens", body)
	if err != nil {
		return CreateTokenResult{}, err
	}
	if raw.ID == "" || raw.Value == "" {
		return CreateTokenResult{}, fmt.Errorf("Cloudflare returned an empty token; cannot derive S3 credentials")
	}

	// S3 secret key derivation per Cloudflare docs:
	//   "Access Key ID = the id of the API token"
	//   "Secret Access Key = the SHA-256 hash of the API token value"
	// Encoding: hex-lowercase. If R2 ever changes this, smoke tests will surface it.
	sum := sha256.Sum256([]byte(raw.Value))
	return CreateTokenResult{
		AccessKeyID:     raw.ID,
		SecretAccessKey: hex.EncodeToString(sum[:]),
		TokenID:         raw.ID,
	}, nil
}
