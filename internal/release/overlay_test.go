package release

import (
	"os"
	"sort"
	"strings"
	"testing"
)

const co2Chart = "oci://ghcr.io/epfl-enac/co2-calculator/helm"

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustParse(t *testing.T, p string) *Overlay {
	t.Helper()
	o, err := ParseOverlay([]byte(read(t, p)))
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func render(t *testing.T, o *Overlay) string {
	t.Helper()
	b, err := o.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// lineDiff returns the lines only in a ("-") and only in b ("+"), by LCS.
func lineDiff(a, b []string) []string {
	n, m := len(a), len(b)
	l := make([][]int, n+1)
	for i := range l {
		l[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				l[i][j] = l[i+1][j+1] + 1
			} else {
				l[i][j] = max(l[i+1][j], l[i][j+1])
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			i++
			j++
		case j < m && (i == n || l[i][j+1] >= l[i+1][j]):
			out = append(out, "+"+b[j])
			j++
		default:
			out = append(out, "-"+a[i])
			i++
		}
	}
	return out
}

func TestDeployedReadsCo2(t *testing.T) {
	d := mustParse(t, "testdata/co2-prod.yaml").Deployed(co2Chart)
	fe := d.Images["ghcr.io/epfl-enac/epfl/co2-calculator/frontend"]
	if d.Commit != "" || d.Chart != "1.0.1789" || fe.Tag != "v1.4.19" || !strings.HasPrefix(fe.Digest, "sha256:33b43e11") {
		t.Fatalf("got %+v", d)
	}
}

func TestSetKeepsEverythingElse(t *testing.T) {
	before := read(t, "testdata/co2-prod.yaml")
	o := mustParse(t, "testdata/co2-prod.yaml")
	commit := "5e09a71" + strings.Repeat("0", 33)
	changed, err := o.Set(Deployed{
		Commit: commit, Version: "v1.4.20",
		Images: map[string]Image{
			"ghcr.io/epfl-enac/epfl/co2-calculator/frontend": {Tag: "v1.4.20", Digest: "sha256:aaaa"},
			"ghcr.io/epfl-enac/epfl/co2-calculator/backend":  {Tag: "v1.4.20", Digest: "sha256:bbbb"},
		},
	}, co2Chart, []string{"backend.env.APP_VERSION"})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	got := lineDiff(strings.Split(before, "\n"), strings.Split(render(t, o), "\n"))
	want := []string{
		"-    newTag: v1.4.19", "-    newTag: v1.4.19",
		"+    newTag: v1.4.20", "+    newTag: v1.4.20",
		"-    digest: sha256:33b43e1158bfc41fbda59eccb96db5d9f535dc92d081e5043355df877053ea13",
		"-    digest: sha256:64939de9917ed2bf1d5cb88740f6e7992fe1346535c5b67ed2c8c2f098df9503",
		"+    digest: sha256:aaaa", "+    digest: sha256:bbbb",
		"-    version: 1.0.1789", "+    version: v1.4.20",
		`-          APP_VERSION: "sprint-1-prod-2025-11-28"`, `+          APP_VERSION: "v1.4.20"`,
		"+commonAnnotations:",
		"+  git-board/commit: " + commit,
		"+  git-board/version: v1.4.20",
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("changed lines differ\n got: %q\nwant: %q", got, want)
	}
}

func TestSetUnknownImage(t *testing.T) {
	o := mustParse(t, "testdata/co2-prod.yaml")
	_, err := o.Set(Deployed{Images: map[string]Image{"ghcr.io/x/docs": {Tag: "v1"}}}, "", nil)
	if err == nil || err.Error() != "overlay has no image ghcr.io/x/docs" {
		t.Fatalf("err = %v", err)
	}
}

func TestSetUnknownChart(t *testing.T) {
	o := mustParse(t, "testdata/co2-prod.yaml")
	if _, err := o.Set(Deployed{}, "oci://nope", nil); err == nil || err.Error() != "overlay has no chart oci://nope" {
		t.Fatalf("err = %v", err)
	}
}

func TestSetTwiceIsNoChange(t *testing.T) {
	o := mustParse(t, "testdata/co2-prod.yaml")
	d := Deployed{Commit: strings.Repeat("a", 40), Version: "v1.4.20", Images: map[string]Image{
		"ghcr.io/epfl-enac/epfl/co2-calculator/frontend": {Tag: "v1.4.20", Digest: "sha256:aaaa"},
	}}
	vv := []string{"backend.env.APP_VERSION"}
	if c, err := o.Set(d, co2Chart, vv); err != nil || !c {
		t.Fatalf("first: %v %v", c, err)
	}
	if c, err := o.Set(d, co2Chart, vv); err != nil || c {
		t.Fatalf("second: changed=%v err=%v", c, err)
	}
}
