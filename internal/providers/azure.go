package providers

import "fmt"

// Azure is an Azure Blob Storage provider accessed via Microsoft's
// S3-compatible endpoint (https://<account>.s3.blob.storage.azure.net).
// Requires hierarchical namespace (ADLS Gen2) to be enabled on the storage
// account. Auth uses AWS Signature V4 with the storage account name as the
// access key ID and the account key as the secret.
type Azure struct{ config Config }

// NewAzure constructs an Azure provider from a storage account name,
// container name (equivalent to a bucket), and storage account key.
func NewAzure(accountName, container, accountKey string) *Azure {
	return &Azure{config: Config{
		Kind:            KindAzure,
		Bucket:          container,
		AccountID:       accountName,
		AccessKeyID:     accountName,
		SecretAccessKey: accountKey,
	}}
}

func (a *Azure) Kind() Kind     { return KindAzure }
func (a *Azure) Bucket() string { return a.config.Bucket }

func (a *Azure) Endpoint() string {
	if a.config.Endpoint != "" {
		return a.config.Endpoint
	}
	return fmt.Sprintf("https://%s.s3.blob.storage.azure.net", a.config.AccountID)
}

// Region returns a fixed placeholder region string. Azure's S3-compatible
// endpoint accepts any valid AWS region string for Signature V4 purposes; the
// actual bucket location is determined by the storage account's region.
func (a *Azure) Region() string              { return "us-east-1" }
func (a *Azure) ForcePathStyle() bool        { return true }
func (a *Azure) AccessKeyID() string         { return a.config.AccountID }
func (a *Azure) SecretAccessKey() string     { return a.config.SecretAccessKey }
