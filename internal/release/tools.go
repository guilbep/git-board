package release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoImage means the registry has no such image.
var ErrNoImage = errors.New("no such image")

// RevisionLabel is the label that says which commit an image was built from.
const RevisionLabel = "org.opencontainers.image.revision"

// Build is one image build; the platform is always linux/amd64.
type Build struct{ Context, Ref, Revision, Builder string }

// Tools is every external program a release runs besides git.
type Tools interface {
	// Revision returns the digest and revision label of ref; ErrNoImage when absent.
	Revision(ctx context.Context, ref string) (digest, revision string, err error)
	Build(ctx context.Context, b Build) (digest string, err error)
	Scan(ctx context.Context, ref string) error
	Copy(ctx context.Context, src, dst string) error
	Tag(ctx context.Context, ref, tag string) error
	PackageChart(ctx context.Context, path, version, dir string) (file string, err error)
	PushChart(ctx context.Context, file, registry string) (digest string, err error)
}

// Exec runs the real docker, crane, trivy and helm found on PATH.
func Exec() Tools { return execTools{} }

type execTools struct{}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
	}
	return strings.TrimSpace(out.String()), nil
}

func (execTools) Revision(ctx context.Context, ref string) (string, string, error) {
	digest, err := run(ctx, "crane", "digest", ref)
	if err != nil {
		if strings.Contains(err.Error(), "MANIFEST_UNKNOWN") || strings.Contains(err.Error(), "NAME_UNKNOWN") {
			return "", "", fmt.Errorf("%s: %w", ref, ErrNoImage)
		}
		return "", "", err
	}
	cfg, err := run(ctx, "crane", "config", ref)
	if err != nil {
		return "", "", err
	}
	var c struct {
		Config struct{ Labels map[string]string }
	}
	if err := json.Unmarshal([]byte(cfg), &c); err != nil {
		return "", "", fmt.Errorf("crane config %s: %w", ref, err)
	}
	return digest, c.Config.Labels[RevisionLabel], nil
}

func (execTools) Build(ctx context.Context, b Build) (string, error) {
	dir, err := os.MkdirTemp("", "release-build")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	meta := filepath.Join(dir, "metadata.json")
	args := []string{"buildx", "build"}
	if b.Builder != "" {
		args = append(args, "--builder", b.Builder)
	}
	args = append(args, "--platform", "linux/amd64", "--label", RevisionLabel+"="+b.Revision,
		"--provenance=mode=max", "--sbom=true", "--push", "--metadata-file", meta, "-t", b.Ref, b.Context)
	if _, err := run(ctx, "docker", args...); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(meta)
	if err != nil {
		return "", err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("docker metadata for %s: %w", b.Ref, err)
	}
	d, _ := m["containerimage.digest"].(string)
	if d == "" {
		return "", fmt.Errorf("docker metadata for %s has no containerimage.digest", b.Ref)
	}
	return d, nil
}

func (execTools) Scan(ctx context.Context, ref string) error {
	_, err := run(ctx, "trivy", "image", "--exit-code", "1", "--severity", "HIGH,CRITICAL", "--ignore-unfixed", ref)
	return err
}

func (execTools) Copy(ctx context.Context, src, dst string) error {
	_, err := run(ctx, "crane", "copy", src, dst)
	return err
}

func (execTools) Tag(ctx context.Context, ref, tag string) error {
	_, err := run(ctx, "crane", "tag", ref, tag)
	return err
}

func (execTools) PackageChart(ctx context.Context, path, version, dir string) (string, error) {
	out, err := run(ctx, "helm", "package", path, "--version", version, "--app-version", version, "-d", dir)
	if err != nil {
		return "", err
	}
	f := strings.Fields(out)
	if len(f) == 0 {
		return "", fmt.Errorf("helm package %s: no output", path)
	}
	return f[len(f)-1], nil
}

func (execTools) PushChart(ctx context.Context, file, registry string) (string, error) {
	// helm push says what it pushed on stderr
	out, err := exec.CommandContext(ctx, "helm", "push", file, registry).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("helm push %s %s: %w: %s", file, registry, err, strings.TrimSpace(string(out)))
	}
	for _, l := range strings.Split(string(out), "\n") {
		if d, ok := strings.CutPrefix(strings.TrimSpace(l), "Digest:"); ok {
			return strings.TrimSpace(d), nil
		}
	}
	return "", fmt.Errorf("helm push %s: no Digest line in %q", file, out)
}
