package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/guilbep/git-board/conf"
)

// AppendSprint appends a [[sprints.list]] entry to the .git-board.toml of
// dir's repository, after the last one, the rest of the file as it was,
// and answers the file's path (v1's config.AppendSprint). A slug the file
// lists already is refused.
func AppendSprint(dir, slug, title string, start time.Time) (string, error) {
	path := configPath(dir)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return path, fmt.Errorf("reading %s: %w", path, err)
	}
	f, err := conf.Parse(path, data)
	if err != nil {
		return path, fmt.Errorf("reading %s: %w", path, err)
	}
	cfg, err := ReadConfig(f, nil)
	if err != nil {
		return path, fmt.Errorf("reading %s: %w", path, err)
	}
	for _, s := range cfg.Schema.Sprints.List {
		if s.Slug == slug {
			return path, fmt.Errorf("sprint %q already exists in %s", slug, path)
		}
	}
	block := fmt.Sprintf("[[sprints.list]]\nslug = %q\ntitle = %q\nstart = %s\n", slug, title, start.Format("2006-01-02"))
	if err := os.WriteFile(path, []byte(appendAfterLastMatch(string(data), "[[sprints.list]]", block)), 0o644); err != nil {
		return path, fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
}

// appendAfterLastMatch inserts block after the table that the last line
// equal to header starts (up to the next header, or the end), else at
// the end, a blank line before it.
func appendAfterLastMatch(content, header, block string) string {
	lines := strings.Split(content, "\n")
	end := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != header {
			continue
		}
		end = len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "[") {
				end = j
				break
			}
		}
	}
	if end < 0 {
		if strings.TrimSpace(content) == "" {
			return block
		}
		return strings.TrimRight(content, "\n") + "\n\n" + block
	}
	before := strings.TrimRight(strings.Join(lines[:end], "\n"), "\n")
	after := strings.Join(lines[end:], "\n")
	var b strings.Builder
	b.WriteString(before)
	b.WriteString("\n\n")
	b.WriteString(block)
	if strings.TrimSpace(after) != "" {
		b.WriteString("\n")
		b.WriteString(after)
	}
	return b.String()
}
