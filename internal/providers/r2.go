package providers

import "fmt"

// R2 represents a Cloudflare R2 bucket. R2 is the default provider because
// of its 10 GB free tier with zero egress fees.
type R2 struct {
	config Config
}

func NewR2(accountID, bucket, accessKeyID, secretAccessKey string) *R2 {
	return &R2{config: Config{
		Kind:            KindR2,
		Bucket:          bucket,
		AccountID:       accountID,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}}
}

func (r *R2) Kind() Kind     { return KindR2 }
func (r *R2) Bucket() string { return r.config.Bucket }

func (r *R2) Endpoint() string {
	if r.config.Endpoint != "" {
		return r.config.Endpoint
	}
	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", r.config.AccountID)
}

func (r *R2) Region() string              { return "auto" }
func (r *R2) ForcePathStyle() bool        { return true }
func (r *R2) AccessKeyID() string         { return r.config.AccessKeyID }
func (r *R2) SecretAccessKey() string     { return r.config.SecretAccessKey }
