package cloudflare

import (
	"context"
	"fmt"
)

// Bucket is the subset of an R2 bucket record we care about.
type Bucket struct {
	Name         string `json:"name"`
	CreationDate string `json:"creation_date"`
	Location     string `json:"location,omitempty"`
}

// listBucketsResult matches the slightly nested shape of the buckets endpoint.
type listBucketsResult struct {
	Buckets []Bucket `json:"buckets"`
}

// ListBuckets returns every R2 bucket in the given account.
func (c *Client) ListBuckets(ctx context.Context, accountID string) ([]Bucket, error) {
	r, err := do[listBucketsResult](ctx, c, "GET", fmt.Sprintf("/accounts/%s/r2/buckets", accountID), nil)
	if err != nil {
		return nil, err
	}
	return r.Buckets, nil
}

// CreateBucket creates a new R2 bucket. Returns the bucket on success. If a
// bucket with the same name already exists, Cloudflare returns an error which
// we propagate verbatim — callers should distinguish "already exists" from
// other failures and offer to use the existing one.
func (c *Client) CreateBucket(ctx context.Context, accountID, name string) (Bucket, error) {
	body := map[string]string{"name": name}
	return do[Bucket](ctx, c, "POST", fmt.Sprintf("/accounts/%s/r2/buckets", accountID), body)
}

// DeleteBucket removes an R2 bucket. The bucket must be empty; Cloudflare
// returns 10004 ("bucket not empty") otherwise. Callers should surface a
// dashboard URL for manual cleanup in that case.
func (c *Client) DeleteBucket(ctx context.Context, accountID, name string) error {
	_, err := do[struct{}](ctx, c, "DELETE", fmt.Sprintf("/accounts/%s/r2/buckets/%s", accountID, name), nil)
	return err
}
