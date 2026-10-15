package markdown

import (
	"strings"
	"testing"
)

func viaProxy(u string) string { return "/api/media?url=" + u }

func TestRenderKeepsWhatGitHubShows(t *testing.T) {
	src := "line one\nline two\n\n- [x] done\n- [ ] todo\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n" +
		"```go\nfmt.Println(1)\n```\n\n<details><summary>More</summary>hidden</details>\n\n~~old~~ https://epfl.ch"
	got := Render(src, viaProxy)
	for _, want := range []string{
		"line one<br", `type="checkbox"`, "checked", "<table>", "<td>1</td>",
		`class="language-go"`, "<details>", "<summary>More</summary>", "<del>old</del>",
		`href="https://epfl.ch"`, `target="_blank"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderRemovesWhatCouldRunCode(t *testing.T) {
	src := "<script>alert(1)</script>\n\n<img src=x onerror=alert(2)>\n\n[click](javascript:alert(3))\n\n" +
		"<a href=\"javascript:alert(4)\">a</a> <iframe src=\"https://evil.example\"></iframe> <div style=\"position:fixed\">x</div>"
	got := Render(src, viaProxy)
	for _, bad := range []string{"<script", "onerror", "javascript:", "<iframe", "position:fixed"} {
		if strings.Contains(strings.ToLower(got), bad) {
			t.Errorf("%q left in:\n%s", bad, got)
		}
	}
}

func TestRenderSendsImagesThroughTheServer(t *testing.T) {
	src := "![shot](https://github.com/user-attachments/assets/abc)\n\n<img width=\"300\" src=\"https://gitlab.example.com/uploads/x.png\">"
	got := Render(src, viaProxy)
	for _, want := range []string{
		`src="/api/media?url=https://github.com/user-attachments/assets/abc"`,
		`src="/api/media?url=https://gitlab.example.com/uploads/x.png"`,
		`width="300"`, `alt="shot"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
