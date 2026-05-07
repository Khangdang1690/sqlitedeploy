package providers

// B2 represents a Backblaze B2 bucket via its S3-compatible API.
// 10 GB free, 1 GB/day download free.
type B2 struct {
	config Config
}

func NewB2(region, bucket, accessKeyID, secretAccessKey string) *B2 {
	return &B2{config: Config{
		Kind:            KindB2,
		Bucket:          bucket,
		Region:          region,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}}
}

func (b *B2) Kind() Kind     { return KindB2 }
func (b *B2) Bucket() string { return b.config.Bucket }

func (b *B2) Endpoint() string {
	if b.config.Endpoint != "" {
		return b.config.Endpoint
	}
	return "https://s3." + b.config.Region + ".backblazeb2.com"
}

func (b *B2) Region() string          { return b.config.Region }
func (b *B2) ForcePathStyle() bool    { return true }
func (b *B2) AccessKeyID() string     { return b.config.AccessKeyID }
func (b *B2) SecretAccessKey() string { return b.config.SecretAccessKey }
