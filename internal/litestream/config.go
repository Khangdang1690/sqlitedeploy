// Package litestream renders Litestream's YAML config from our own
// internal/config representation, and runs the bundled `litestream` binary.
package litestream

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/Khangdang1690/sqlitedeploy/internal/config"
	"github.com/Khangdang1690/sqlitedeploy/internal/providers"
)

// litestreamYAML mirrors the subset of Litestream's config schema we use.
// See https://litestream.io/reference/config/ for the full spec.
type litestreamYAML struct {
	DBs []dbBlock `yaml:"dbs"`
}

type dbBlock struct {
	Path     string         `yaml:"path"`
	Replicas []replicaBlock `yaml:"replicas,omitempty"`
}

type replicaBlock struct {
	Type            string `yaml:"type"`
	Bucket          string `yaml:"bucket"`
	Path            string `yaml:"path"`
	Region          string `yaml:"region,omitempty"`
	Endpoint        string `yaml:"endpoint,omitempty"`
	ForcePathStyle  bool   `yaml:"force-path-style,omitempty"`
	AccessKeyID     string `yaml:"access-key-id"`
	SecretAccessKey string `yaml:"secret-access-key"`
}

// Render writes a litestream.yml derived from c to its canonical location.
// On replica nodes the file contains an empty `replicas:` block — replica
// nodes use the `litestream restore` command, not the `replicate` daemon.
func Render(projectDir string, c *config.Config) (string, error) {
	prov, err := providers.FromConfig(c.Provider)
	if err != nil {
		return "", err
	}
	dbAbs, err := filepath.Abs(filepath.Join(projectDir, c.DBPath))
	if err != nil {
		return "", err
	}

	doc := litestreamYAML{DBs: []dbBlock{{Path: dbAbs}}}
	if c.Role == config.RolePrimary {
		doc.DBs[0].Replicas = []replicaBlock{replicaFor(prov, c.ReplicaPath)}
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}

	out := config.LitestreamPath(projectDir)
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(out, raw, 0o600); err != nil {
		return "", fmt.Errorf("write litestream config: %w", err)
	}
	return out, nil
}

// ReplicaURL renders the litestream-style replica URL used by `litestream
// restore` on replica nodes that don't (yet) have a config.yml. Format:
//
//	s3://<bucket>/<path>?endpoint=<url>&region=<r>&force-path-style=true
func ReplicaURL(p providers.Provider, replicaPath string) string {
	u := fmt.Sprintf("s3://%s/%s", p.Bucket(), replicaPath)
	q := ""
	if e := p.Endpoint(); e != "" {
		q = appendQuery(q, "endpoint", e)
	}
	if r := p.Region(); r != "" {
		q = appendQuery(q, "region", r)
	}
	if p.ForcePathStyle() {
		q = appendQuery(q, "force-path-style", "true")
	}
	if q != "" {
		u += "?" + q
	}
	return u
}

func replicaFor(p providers.Provider, path string) replicaBlock {
	return replicaBlock{
		Type:            "s3",
		Bucket:          p.Bucket(),
		Path:            path,
		Region:          p.Region(),
		Endpoint:        p.Endpoint(),
		ForcePathStyle:  p.ForcePathStyle(),
		AccessKeyID:     p.AccessKeyID(),
		SecretAccessKey: p.SecretAccessKey(),
	}
}

func appendQuery(existing, k, v string) string {
	if existing == "" {
		return k + "=" + v
	}
	return existing + "&" + k + "=" + v
}
