package cloudflare

import "context"

// Account is the trimmed-down view of /accounts that we actually use.
type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListAccounts returns every account the parent token can see. The first
// account is typically the user's personal account.
func (c *Client) ListAccounts(ctx context.Context) ([]Account, error) {
	return do[[]Account](ctx, c, "GET", "/accounts", nil)
}
