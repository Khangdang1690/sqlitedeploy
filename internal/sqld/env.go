package sqld

import (
	"github.com/Khangdang1690/sqlitedeploy/internal/providers"
)

// Bottomless replication env vars consumed by sqld at runtime.
// These names come from libsql-server's bottomless integration; the
// `LIBSQL_BOTTOMLESS_AWS_*` keys are reused for any S3-compatible provider
// (R2, B2, Tigris, MinIO, AWS S3) since bottomless speaks the S3 API.
const (
	envBucket          = "LIBSQL_BOTTOMLESS_BUCKET"
	envEndpoint        = "LIBSQL_BOTTOMLESS_ENDPOINT"
	envRegion          = "LIBSQL_BOTTOMLESS_AWS_REGION"
	envAccessKeyID     = "LIBSQL_BOTTOMLESS_AWS_ACCESS_KEY_ID"
	envSecretAccessKey = "LIBSQL_BOTTOMLESS_AWS_SECRET_ACCESS_KEY"
	envBucketPrefix    = "LIBSQL_BOTTOMLESS_DB_NAME"
)

// BottomlessEnv translates the project's Provider abstraction into the env
// vars sqld expects for bottomless. Returns a map suitable for merging with
// os.Environ() before exec.Cmd.Env. The dbName is used as the bucket prefix
// (lets multiple databases share one bucket); pass "" to use bottomless's
// default ("" results in the prefix being unset, which sqld interprets as
// "use the bucket root").
func BottomlessEnv(p providers.Provider, dbName string) map[string]string {
	env := map[string]string{
		envBucket:          p.Bucket(),
		envAccessKeyID:     p.AccessKeyID(),
		envSecretAccessKey: p.SecretAccessKey(),
	}
	if e := p.Endpoint(); e != "" {
		env[envEndpoint] = e
	}
	if r := p.Region(); r != "" {
		env[envRegion] = r
	}
	if dbName != "" {
		env[envBucketPrefix] = dbName
	}
	return env
}
