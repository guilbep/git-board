package app

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/guilbep/git-board/next/conf"
)

// git board init, as v1's: the project file a repository starts from.

// DefaultConfig is the project file a repository starts from: what init
// writes.
//
//go:embed starter/default.git-board.toml
var DefaultConfig string

//go:embed starter/*.git-board.toml
var starters embed.FS

// starter is a built-in template by name (enac).
func starter(name string) (string, error) {
	b, err := starters.ReadFile("starter/" + name + ".git-board.toml")
	if err != nil || strings.ContainsAny(name, "/.") || name == "default" {
		return "", fmt.Errorf("no built-in template %q (there is enac)", name)
	}
	return string(b), nil
}

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

// globalTemplate is the global file's template: what a plain init starts
// from; "" with no file.
func globalTemplate() (string, error) {
	path := GlobalPath()
	data, err := os.ReadFile(path)
	if path == "" || errors.Is(err, fs.ErrNotExist) {
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

// StartConfig is the file a new board starts as, from the template spec
// names: a built-in one (enac), an http(s) URL, or a path; "" is the
// global file's template, else DefaultConfig. It must read: an error
// writes nothing.
func StartConfig(spec string) (string, error) {
	if spec == "" {
		g, err := globalTemplate()
		if err != nil || g == "" {
			return DefaultConfig, err
		}
		spec = g
	}
	var s string
	var err error
	switch {
	case strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "http://"):
		s, err = fetchTemplate(spec)
	case !strings.ContainsAny(spec, `/\.`):
		s, err = starter(spec)
	default:
		var b []byte
		b, err = os.ReadFile(spec)
		s = string(b)
	}
	if err != nil {
		return "", err
	}
	f, err := conf.Parse(conf.FileName, []byte(s))
	if err == nil {
		_, err = readConfig(f, nil)
	}
	if err != nil {
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

// InitConfig writes the project file at the root of the repository
// holding dir, from the template spec names (StartConfig), and answers
// its path. It never overwrites one.
func InitConfig(dir, spec string) (string, error) {
	path := configPath(dir)
	if path == conf.FileName {
		return "", errors.New("not in a git repository")
	}
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	}
	content, err := StartConfig(spec)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("%s already exists", path)
	}
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return "", err
	}
	return path, f.Close()
}
