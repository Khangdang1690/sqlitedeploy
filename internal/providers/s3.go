package providers

// S3 is a generic S3-compatible provider. Use this for AWS S3, MinIO, Tigris,
// or any other S3-API-speaking service that doesn't need provider-specific
// endpoint logic.
type S3 struct {
	config Config
}

func NewS3(region, bucket, endpoint, accessKeyID, secretAccessKey string) *S3 {
	return &S3{config: Config{
		Kind:            KindS3,
		Bucket:          bucket,
		Region:          region,
		Endpoint:        endpoint,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}}
}

func (s *S3) Kind() Kind          { return KindS3 }
func (s *S3) Bucket() string      { return s.config.Bucket }
func (s *S3) Endpoint() string    { return s.config.Endpoint }
func (s *S3) Region() string      { return s.config.Region }

// Custom-endpoint S3-compatible services almost always need path-style.
// AWS S3 (no custom endpoint) accepts it too, so default true is safe.
func (s *S3) ForcePathStyle() bool    { return true }
func (s *S3) AccessKeyID() string     { return s.config.AccessKeyID }
func (s *S3) SecretAccessKey() string { return s.config.SecretAccessKey }
