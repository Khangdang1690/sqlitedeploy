package litestream

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Runner wraps the litestream binary so callers don't have to think about
// resolution, env, or stdio plumbing.
type Runner struct {
	binPath    string
	configPath string
}

// NewRunner resolves the litestream binary and binds a config file path.
func NewRunner(configPath string) (*Runner, error) {
	bin, err := Resolve()
	if err != nil {
		return nil, err
	}
	return &Runner{binPath: bin, configPath: configPath}, nil
}

// Replicate runs `litestream replicate -config <configPath>` until ctx is
// cancelled or the process exits. Stdio is plumbed to the parent so users see
// litestream's logs in their terminal.
func (r *Runner) Replicate(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, r.binPath, "replicate", "-config", r.configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// Restore runs `litestream restore` against the configured database, pulling
// the latest snapshot+WAL from object storage. dbPath is required; replicaURL
// is optional — when non-empty, restore reads from that URL directly instead
// of using the config file (used by `attach` before a config exists).
func (r *Runner) Restore(ctx context.Context, dbPath, replicaURL string, ifNotExists bool) error {
	args := []string{"restore"}
	if ifNotExists {
		args = append(args, "-if-db-not-exists", "-if-replica-exists")
	}
	args = append(args, "-o", dbPath)
	if replicaURL != "" {
		args = append(args, replicaURL)
	} else {
		args = append(args, "-config", r.configPath, dbPath)
	}

	cmd := exec.CommandContext(ctx, r.binPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("litestream restore: %w", err)
	}
	return nil
}

// LTXFiles lists the LTX (Litestream Transaction) files replicated for the
// configured database. Litestream 0.5+ uses LTX as the unit of replication;
// the older `snapshots` subcommand was removed.
func (r *Runner) LTXFiles(ctx context.Context, dbPath string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.binPath, "ltx", "-config", r.configPath, dbPath)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}
