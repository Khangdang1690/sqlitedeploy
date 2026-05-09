package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Khangdang1690/sqlitedeploy/internal/auth"
	"github.com/Khangdang1690/sqlitedeploy/internal/config"
	"github.com/Khangdang1690/sqlitedeploy/internal/providers"
	"github.com/Khangdang1690/sqlitedeploy/internal/sqld"
)

// NewAttachCmd builds the `attach` subcommand: bootstraps a read-replica node.
// Streams WAL frames live from the primary's gRPC endpoint (sub-second
// freshness), and on first run seeds the local DB from the bucket via
// bottomless to avoid replaying everything over the network.
func NewAttachCmd() *cobra.Command {
	var (
		providerKind   string
		bucket         string
		accountID      string
		region         string
		endpoint       string
		accessKey      string
		secretKey      string
		dbPath         string
		bucketPrefix   string
		primaryGRPCURL string
		primaryHTTPURL string
		authTokenFile  string
		jwtPublicFile  string
		httpListenAddr string
		oneShot        bool
	)

	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Attach this node as a sqld read replica",
		Long: `Attach this host as a read-only replica: starts sqld in replica mode,
streaming WAL frames from the primary's gRPC endpoint live (sub-second
freshness) and serving Hrana locally for app reads.

On first attach, the local DB is seeded from the bucket via bottomless before
the live stream catches up — much faster than replaying everything from gRPC
for large databases.

You'll need three things from the primary host (transferred via scp / secure
channel — never paste secrets into chat):
  - the JWT public key:    .sqlitedeploy/auth/jwt_public.pem
  - the replica JWT token: .sqlitedeploy/auth/replica.jwt
  - the primary's gRPC URL (--primary-grpc-url)

Plus the same bucket credentials the primary uses, for the cold-start seed.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := projectDir(cmd)
			if err != nil {
				return err
			}

			cfg, isFirstAttach, err := loadOrInitReplicaConfig(dir, &replicaInputs{
				providerKind:   providerKind,
				bucket:         bucket,
				accountID:      accountID,
				region:         region,
				endpoint:       endpoint,
				accessKey:      accessKey,
				secretKey:      secretKey,
				dbPath:         dbPath,
				bucketPrefix:   bucketPrefix,
				primaryGRPCURL: primaryGRPCURL,
				primaryHTTPURL: primaryHTTPURL,
				jwtPublicFile:  jwtPublicFile,
				httpListenAddr: httpListenAddr,
			})
			if err != nil {
				return err
			}

			prov, err := providers.FromConfig(cfg.Provider)
			if err != nil {
				return err
			}

			runner, err := sqld.NewRunner()
			if err != nil {
				return err
			}

			absDBPath := absDB(dir, cfg.DBPath)
			// sqld treats DBPath as a directory it owns; create the dir itself.
			if err := os.MkdirAll(absDBPath, 0o755); err != nil {
				return err
			}

			tokenPath := authTokenFile
			if tokenPath == "" {
				tokenPath = filepath.Join(dir, config.DirName, auth.DirName, auth.ReplicaTokenFile)
			}
			if _, err := os.Stat(tokenPath); err != nil {
				return fmt.Errorf("replica JWT not found at %s — copy it from the primary's .sqlitedeploy/auth/replica.jwt over scp/secure channel, then re-run", tokenPath)
			}

			pubKeyPath := cfg.ResolveAuthPath(dir, cfg.JWTPublicKeyPath)
			if _, err := os.Stat(pubKeyPath); err != nil {
				return fmt.Errorf("JWT public key not found at %s — copy it from the primary's .sqlitedeploy/auth/jwt_public.pem", pubKeyPath)
			}

			opts := sqld.ReplicaOpts{
				DBPath:          absDBPath,
				HTTPListenAddr:  cfg.HTTPListenAddr,
				HranaListenAddr: cfg.HranaListenAddr,
				PrimaryGRPCURL:  cfg.PrimaryGRPCURL,
				AuthJWTKeyFile:  pubKeyPath,
				BottomlessEnv:   sqld.BottomlessEnv(prov, cfg.BucketPrefix),
				SyncFromStorage: isFirstAttach,
			}

			fmt.Println()
			fmt.Println("✓ sqlitedeploy replica attached")
			fmt.Printf("  Local DB:                  %s\n", absDBPath)
			fmt.Printf("  Local read endpoint:       http://%s\n", cfg.HTTPListenAddr)
			fmt.Printf("  Streaming from primary:    %s\n", cfg.PrimaryGRPCURL)
			if isFirstAttach {
				fmt.Println("  Cold-start seed:           bottomless (bucket → local DB)")
			}
			fmt.Println()

			if oneShot {
				return nil
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return runner.ServeReplica(ctx, opts)
		},
	}
	addProjectDirFlag(cmd)
	cmd.Flags().StringVar(&providerKind, "provider", "r2", "object storage provider: r2 | b2 | s3")
	cmd.Flags().StringVar(&bucket, "bucket", "", "bucket name (required if no existing config)")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Cloudflare R2 account ID (R2 only)")
	cmd.Flags().StringVar(&region, "region", "", "bucket region (B2/S3)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "S3 endpoint override")
	cmd.Flags().StringVar(&accessKey, "access-key", envOrDefault("SQLITEDEPLOY_ACCESS_KEY", ""), "access key (or env SQLITEDEPLOY_ACCESS_KEY) — read-only key recommended")
	cmd.Flags().StringVar(&secretKey, "secret-key", envOrDefault("SQLITEDEPLOY_SECRET_KEY", ""), "secret key (or env SQLITEDEPLOY_SECRET_KEY)")
	cmd.Flags().StringVar(&dbPath, "db-path", config.DefaultDBPath, "where to put the local replica SQLite file")
	cmd.Flags().StringVar(&bucketPrefix, "bucket-prefix", config.DefaultBucketPrefix, "bottomless prefix within the bucket (must match primary's --bucket-prefix)")
	cmd.Flags().StringVar(&primaryGRPCURL, "primary-grpc-url", "", "primary's gRPC URL, e.g. http://primary.example.com:5001 (required)")
	cmd.Flags().StringVar(&primaryHTTPURL, "primary-http-url", "", "primary's Hrana HTTP URL (informational; defaults to derived from --primary-grpc-url)")
	cmd.Flags().StringVar(&authTokenFile, "auth-token-file", "", "path to replica.jwt copied from primary (default: .sqlitedeploy/auth/replica.jwt under -C)")
	cmd.Flags().StringVar(&jwtPublicFile, "jwt-public-key-file", "", "path to jwt_public.pem copied from primary (default: .sqlitedeploy/auth/jwt_public.pem under -C)")
	cmd.Flags().StringVar(&httpListenAddr, "http-listen-addr", config.DefaultHTTPListenAddr, "where this replica's local Hrana endpoint listens")
	cmd.Flags().BoolVar(&oneShot, "one-shot", false, "validate config and exit without starting the replica daemon")
	return cmd
}

type replicaInputs struct {
	providerKind, bucket, accountID, region, endpoint, accessKey, secretKey string
	dbPath, bucketPrefix                                                    string
	primaryGRPCURL, primaryHTTPURL                                          string
	jwtPublicFile, httpListenAddr                                           string
}

// loadOrInitReplicaConfig returns the replica's config (loading existing or
// building from flags) and a flag indicating whether this is the first attach
// on this host (so we know to set --sync-from-storage for bottomless seed).
func loadOrInitReplicaConfig(dir string, in *replicaInputs) (*config.Config, bool, error) {
	if cfg, err := config.Load(dir); err == nil {
		// Existing config — reuse, ignore CLI flags (don't silently change creds).
		if cfg.Role == "" {
			cfg.Role = config.RoleReplica
		}
		// Decide first-attach by whether the local DB already exists.
		dbAbs := absDB(dir, cfg.DBPath)
		_, statErr := os.Stat(dbAbs)
		isFirst := os.IsNotExist(statErr)
		return cfg, isFirst, nil
	}

	if in.primaryGRPCURL == "" {
		return nil, false, fmt.Errorf("--primary-grpc-url is required when bootstrapping a new replica config")
	}

	kind, err := providers.ParseKind(in.providerKind)
	if err != nil {
		return nil, false, err
	}
	prov, err := buildProviderManual(kind, providerInputs{
		kindStr: in.providerKind, bucket: in.bucket, accountID: in.accountID,
		region: in.region, endpoint: in.endpoint,
		accessKey: in.accessKey, secretKey: in.secretKey,
	})
	if err != nil {
		return nil, false, err
	}
	cfg := &config.Config{
		Role:             config.RoleReplica,
		DBPath:           in.dbPath,
		Provider:         providers.ToConfig(prov),
		BucketPrefix:     in.bucketPrefix,
		HTTPListenAddr:   in.httpListenAddr,
		PrimaryGRPCURL:   in.primaryGRPCURL,
		PrimaryHranaURL:  in.primaryHTTPURL,
		JWTPublicKeyPath: config.DefaultJWTPublicKeyPath,
	}
	if err := os.MkdirAll(absDB(dir, cfg.DBPath), 0o755); err != nil {
		return nil, false, err
	}
	if err := config.Save(dir, cfg); err != nil {
		return nil, false, err
	}
	// Fresh replica config + no DB yet → first attach.
	return cfg, true, nil
}

