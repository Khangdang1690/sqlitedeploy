package providers

import "fmt"

// ToConfig flattens any Provider back into its persistable Config form.
func ToConfig(p Provider) Config {
	c := Config{
		Kind:            p.Kind(),
		Bucket:          p.Bucket(),
		Region:          p.Region(),
		Endpoint:        p.Endpoint(),
		AccessKeyID:     p.AccessKeyID(),
		SecretAccessKey: p.SecretAccessKey(),
	}
	if r, ok := p.(*R2); ok {
		c.AccountID = r.config.AccountID
		// R2's endpoint is derived from AccountID; don't persist a redundant copy.
		c.Endpoint = ""
	}
	return c
}

// ParseKind validates that s names a supported provider.
func ParseKind(s string) (Kind, error) {
	switch Kind(s) {
	case KindR2, KindB2, KindS3:
		return Kind(s), nil
	default:
		return "", fmt.Errorf("unsupported provider %q (supported: r2, b2, s3)", s)
	}
}
