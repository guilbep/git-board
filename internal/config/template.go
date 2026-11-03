package config

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// GlobalPath is the user's own file, for every board:
// $XDG_CONFIG_HOME/git-board/config.toml, else ~/.config/git-board/config.toml.
func GlobalPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "git-board", "config.toml")
}

// GlobalTemplate is the global file's template: what a plain init starts
// from. "" with no file. Only a new board reads it: one that exists is its
// own .git-board.toml, whatever this says.
func GlobalTemplate() (string, error) {
	path := GlobalPath()
	data, err := os.ReadFile(path)
	if path == "" || os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var g struct {
		Template string `toml:"template"`
	}
	if _, err := toml.Decode(string(data), &g); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return g.Template, nil
}

// Start is the file a new board starts as, from the template spec names:
// a built-in one (enac), an http(s) URL, or a path; "" is the global
// file's template (GlobalTemplate), else Default. It must check: an error
// writes nothing.
func Start(spec string) (string, error) {
	if spec == "" {
		g, err := GlobalTemplate()
		if err != nil || g == "" {
			return Default, err
		}
		spec = g
	}
	var s string
	var err error
	switch {
	case strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "http://"):
		s, err = fetchTemplate(spec)
	case !strings.ContainsAny(spec, `/\.`):
		s, err = Template(spec)
	default:
		var b []byte
		b, err = os.ReadFile(spec)
		s = string(b)
	}
	if err != nil {
		return "", err
	}
	if _, _, err := Check([]byte(s)); err != nil {
		return "", fmt.Errorf("template %s: %w", spec, err)
	}
	return s, nil
}

func fetchTemplate(url string) (string, error) {
	c := http.Client{Timeout: 30 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("template %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(b), err
}
