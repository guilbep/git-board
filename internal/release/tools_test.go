package release_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
)

// fakeBin puts an executable shell script called name first on PATH.
func fakeBin(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestExecRevisionNoImage(t *testing.T) {
	fakeBin(t, "crane", `echo "MANIFEST_UNKNOWN: manifest unknown" >&2; exit 1`)
	_, _, err := release.Exec().Revision(context.Background(), "reg/img:abc")
	if !errors.Is(err, release.ErrNoImage) {
		t.Fatalf("err = %v, want ErrNoImage", err)
	}
}

func TestExecRevisionReadsLabel(t *testing.T) {
	fakeBin(t, "crane", `case "$1" in digest) echo sha256:abc;; config) echo '{"config":{"Labels":{"org.opencontainers.image.revision":"deadbeef"}}}';; esac`)
	d, r, err := release.Exec().Revision(context.Background(), "reg/img:abc")
	if err != nil || d != "sha256:abc" || r != "deadbeef" {
		t.Fatalf("got %q %q %v", d, r, err)
	}
}

func TestExecBuildReadsMetadata(t *testing.T) {
	fakeBin(t, "docker", `while [ $# -gt 0 ]; do [ "$1" = --metadata-file ] && echo '{"containerimage.digest":"sha256:abc"}' > "$2"; shift; done`)
	d, err := release.Exec().Build(context.Background(), release.Build{Context: ".", Ref: "reg/img:abc", Revision: "abc"})
	if err != nil || d != "sha256:abc" {
		t.Fatalf("got %q %v", d, err)
	}
}

func TestExecPushChartDigest(t *testing.T) {
	fakeBin(t, "helm", `printf 'Pushed: reg/chart:1.0.0\nDigest: sha256:def\n' >&2`) // as helm 4 does
	d, err := release.Exec().PushChart(context.Background(), "chart-1.0.0.tgz", "oci://reg")
	if err != nil || d != "sha256:def" {
		t.Fatalf("got %q %v", d, err)
	}
}

func TestExecPackageChartFile(t *testing.T) {
	fakeBin(t, "helm", `echo "Successfully packaged chart and saved it to: /out/chart-1.0.0.tgz"`)
	f, err := release.Exec().PackageChart(context.Background(), "chart", "1.0.0", "/out")
	if err != nil || f != "/out/chart-1.0.0.tgz" {
		t.Fatalf("got %q %v", f, err)
	}
}

func TestExecScanFails(t *testing.T) {
	fakeBin(t, "trivy", `echo "CVE-1 HIGH" >&2; exit 1`)
	err := release.Exec().Scan(context.Background(), "reg/img:abc")
	if err == nil || !strings.Contains(err.Error(), "reg/img:abc") {
		t.Fatalf("err = %v, want it to name the ref", err)
	}
}

func TestExecSaysHowToInstall(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := release.Exec().Scan(context.Background(), "reg/img:abc")
	if err == nil || !strings.Contains(err.Error(), "trivy isn't installed") || !strings.Contains(err.Error(), "brew install trivy") {
		t.Fatalf("err = %v, want it to say how to install trivy", err)
	}
}

func TestFakeToolsDigestRefsAndTags(t *testing.T) {
	ctx := context.Background()
	f := releasetest.NewTools()
	d, _ := f.Build(ctx, release.Build{Ref: "reg/img:abc", Revision: "abc"})
	if err := f.Tag(ctx, "reg/img:abc", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"reg/img@" + d, "reg/img:1.0.0"} {
		if got, rev, err := f.Revision(ctx, ref); err != nil || got != d || rev != "abc" {
			t.Fatalf("%s: %q %q %v", ref, got, rev, err)
		}
	}
	if err := f.Copy(ctx, "reg/none:x", "o/none:x"); !errors.Is(err, release.ErrNoImage) {
		t.Fatalf("err = %v", err)
	}
}
