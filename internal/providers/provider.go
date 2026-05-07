package providers

import "fmt"

// Kind identifies an object-storage provider supported by sqlitedeploy.
type Kind string

const (
	KindR2 Kind = "r2"
	KindB2 Kind = "b2"
	KindS3 Kind = "s3"
)

// Provider describes the minimum surface needed to render a Litestream replica
// block and validate that credentials work.
type Provider interface {
	Kind() Kind
	Bucket() string

	// Endpoint is the S3-compatible endpoint URL Litestream should talk to.
	// Empty string means "use the default AWS S3 endpoint".
	Endpoint() string

	// Region is the bucket's region. Some providers (R2) accept "auto".
	Region() string

	// ForcePathStyle controls whether the S3 client uses path-style addressing.
	// Required for R2 and most S3-compatible services.
	ForcePathStyle() bool

	// AccessKeyID and SecretAccessKey are the user's credentials.
	AccessKeyID() string
	SecretAccessKey() string
}

// Config is the persisted form of a provider — what we read back from
// .sqlitedeploy/config.yml on subsequent runs.
type Config struct {
	Kind            Kind   `yaml:"kind"`
	Bucket          string `yaml:"bucket"`
	AccountID       string `yaml:"account_id,omitempty"`
	Region          string `yaml:"region,omitempty"`
	Endpoint        string `yaml:"endpoint,omitempty"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
}

// FromConfig rehydrates a Provider from its persisted form.
func FromConfig(c Config) (Provider, error) {
	switch c.Kind {
	case KindR2:
		return &R2{config: c}, nil
	case KindB2:
		return &B2{config: c}, nil
	case KindS3:
		return &S3{config: c}, nil
	default:
		return nil, fmt.Errorf("unknown provider kind %q", c.Kind)
	}
}
