package cloudflare

import (
	"context"
)

// PermissionGroup is the trimmed view of an entry returned by the
// permission-groups catalog endpoint.
type PermissionGroup struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Label string   `json:"label,omitempty"`
	Scope []string `json:"scopes,omitempty"`
}

// ListUserTokenPermissionGroups returns every permission group that can be
// attached to a user-owned API token. Used to resolve canonical permission
// UUIDs by name at runtime so we never depend on hardcoded UUIDs that
// Cloudflare could rotate.
//
// We hit the user-token catalog (not the account-token one) because we create
// user-owned tokens via POST /user/tokens — the namespaces must match. The
// account-scoped catalog at /accounts/{id}/tokens/permission_groups requires
// an "Account API Tokens" permission that the standard "User → API Tokens →
// Edit" grant does not include, and would 9109 here.
func (c *Client) ListUserTokenPermissionGroups(ctx context.Context) ([]PermissionGroup, error) {
	return do[[]PermissionGroup](ctx, c, "GET", "/user/tokens/permission_groups", nil)
}

// FindPermissionGroupID returns the ID of the permission group whose Name
// (case-sensitive exact match) equals wantName. Returns an empty string and
// no error when not found, so callers can produce richer "couldn't find X"
// error messages with extra context.
func FindPermissionGroupID(groups []PermissionGroup, wantName string) string {
	for _, g := range groups {
		if g.Name == wantName {
			return g.ID
		}
	}
	return ""
}
