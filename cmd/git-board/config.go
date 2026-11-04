package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/avatar"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// gitKey is a git config key git-board reads, this clone's own (ADR 0011):
// what a value may be, and where to set it instead when config set
// shouldn't.
type gitKey struct {
	about   string
	check   func(string) error // nil: any value
	instead string             // set elsewhere: the command that does
}

var gitKeys = map[string]gitKey{
	"board.me": {about: "the person you write as", instead: "git board me <id> chooses who you write as"},
	"board.view": {about: "the issue page you open: classic or cycle", check: func(v string) error {
		return oneOf(v, "classic", "cycle")
	}},
	"board.avatars": {about: "people's pictures: forge, gravatar or none", check: func(v string) error {
		if _, ignored := avatar.ParseMode(v); ignored != "" || v == "" {
			return oneOf(v, "forge", "gravatar", "none")
		}
		return nil
	}},
	store.DatastoreKey: {about: "where this clone keeps its board (git board datastore)", check: func(v string) error {
		if v == "git" || v == store.ForgeOnly || store.IsMarkdown(v) || store.IsPostgres(v) {
			return nil
		}
		return fmt.Errorf("%q: git, forge, markdown:<dir> or postgres://…", v)
	}},
	config.LocalExperimental: {about: "the experimental features on in this clone, a comma list", check: func(v string) error {
		for _, f := range strings.Split(v, ",") {
			if !slices.ContainsFunc(config.ExperimentalFeatures, func(e config.ExperimentalFeature) bool { return e.Name == strings.TrimSpace(f) }) {
				return fmt.Errorf("%q: not an experimental feature (cycle, agents, worktrees)", f)
			}
		}
		return nil
	}},
}

func oneOf(v string, values ...string) error {
	if slices.Contains(values, v) {
		return nil
	}
	return fmt.Errorf("%q: one of %s", v, strings.Join(values, ", "))
}

// shownValue is a git config value as config prints it: a datastore URL
// without its password.
func shownValue(key, v string) string {
	if key == store.DatastoreKey && store.IsPostgres(v) {
		if u, err := url.Parse(v); err == nil {
			return u.Redacted()
		}
		return "postgres://…"
	}
	return v
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Get and set .git-board.toml keys and this clone's board.* git config",
		Long: `Read and write the board's settings from the command line.

A key with a dot path, like github-project.owner, sprints.length_days or
views.backlog.filter, is in .git-board.toml, which the team shares. set
changes that key's line alone: every other line and comment stays. It
refuses a key git-board doesn't read, and a value that leaves the file
broken. A value is taken as TOML when it is one (true, 14, ["a", "b"]),
else as a string: quote "14" for a string of digits.

A key board.<name> is this clone's own, in git config: board.me,
board.view, board.avatars, board.datastore, board.experimental. set
writes it in this repository's config (--local), get reads it at every
level git does. A datastore URL is printed without its password.`,
		Example: `  git board config list
  git board config get views.backlog.filter
  git board config set views.backlog.filter "is:open prio:p1"
  git board config set sprints.length_days 14
  git board config unset github-project.write
  git board config set board.view classic`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	var js *jsonOut
	list := &cobra.Command{
		Use:   "list",
		Short: "List the keys set: the file's, then this clone's",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigList(cmd.OutOrStdout(), js)
		},
	}
	js = addJSON(list, `print [{"key", "value", "source"}]`, jsonFields(configEntry{}), true)
	cmd.AddCommand(list,
		&cobra.Command{
			Use:   "get <key>",
			Short: "Print a key's value; exit code 3 when it isn't set",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				v, err := configGet(args[0])
				if err == nil {
					fmt.Fprintln(cmd.OutOrStdout(), v)
				}
				return err
			},
		},
		&cobra.Command{
			Use:   "set <key> <value>",
			Short: "Set a key",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return configSet(args[0], args[1], false)
			},
		},
		&cobra.Command{
			Use:   "unset <key>",
			Short: "Remove a key",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return configSet(args[0], "", true)
			},
		})
	return cmd
}

func isGitKey(key string) bool { return strings.HasPrefix(key, "board.") }

// knownGitKey is the git config key's rules, or a usage error.
func knownGitKey(key string) (gitKey, error) {
	k, ok := gitKeys[key]
	if !ok {
		var names []string
		for n := range gitKeys {
			names = append(names, n)
		}
		slices.Sort(names)
		return k, usageError{fmt.Errorf("%s: %w; this clone's keys are %s", key, config.ErrUnknownKey, strings.Join(names, ", "))}
	}
	return k, nil
}

func configGet(key string) (string, error) {
	if isGitKey(key) {
		if _, err := knownGitKey(key); err != nil {
			return "", err
		}
		v := gitConfig(key)
		if v == "" {
			return "", fmt.Errorf("%w: %s is not set", store.ErrNotFound, key)
		}
		return shownValue(key, v), nil
	}
	data, err := os.ReadFile(config.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: no %s here", store.ErrNotFound, config.FileName)
	}
	if err != nil {
		return "", err
	}
	v, err := config.GetKey(string(data), key)
	if errors.Is(err, config.ErrNoKey) {
		return "", fmt.Errorf("%w: %s is not set in %s", store.ErrNotFound, key, config.FileName)
	}
	if err != nil {
		return "", err
	}
	var s struct{ V string }
	if _, err := toml.Decode("v = "+v, &s); err == nil && strings.HasPrefix(v, `"`) {
		return s.V, nil // a string, as git config prints one: unquoted
	}
	return v, nil
}

func configSet(key, value string, unset bool) error {
	if isGitKey(key) {
		k, err := knownGitKey(key)
		if err != nil {
			return err
		}
		if k.instead != "" {
			return usageError{fmt.Errorf("%s: %s", key, k.instead)}
		}
		args := []string{"config", "--local", key, value}
		if unset {
			args = []string{"config", "--local", "--unset", key}
		} else if k.check != nil {
			if err := k.check(value); err != nil {
				return usageError{fmt.Errorf("%s: %w", key, err)}
			}
		}
		out, err := exec.Command("git", args...).CombinedOutput()
		var ee *exec.ExitError
		if unset && errors.As(err, &ee) && ee.ExitCode() == 5 {
			return nil // wasn't set
		}
		if err != nil {
			return fmt.Errorf("git config: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	path := config.Path()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no %s here: git board init writes one", config.FileName)
	}
	if err != nil {
		return err
	}
	lit := ""
	if !unset {
		lit = config.Value(value)
	}
	out, err := config.SetKeyText(string(data), key, lit)
	if err != nil {
		return usageError{err}
	}
	if out == string(data) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out), info.Mode().Perm())
}

// configEntry is a key config list prints.
type configEntry struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"` // .git-board.toml, or git config's scope: local, global, system
}

func runConfigList(out io.Writer, js *jsonOut) error {
	var entries []configEntry
	if data, err := os.ReadFile(config.Path()); err == nil {
		kvs, err := config.ListKeys(string(data))
		if err != nil {
			return fmt.Errorf("parsing %s: %w", config.FileName, err)
		}
		for _, kv := range kvs {
			entries = append(entries, configEntry{kv.Key, kv.Value, config.FileName})
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	got, _ := exec.Command("git", "config", "--show-scope", "--get-regexp", `^board\.`).Output()
	for _, line := range strings.Split(strings.TrimSpace(string(got)), "\n") {
		scope, rest, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		key, v, _ := strings.Cut(rest, " ")
		if _, known := gitKeys[key]; known {
			entries = append(entries, configEntry{key, shownValue(key, v), scope})
		}
	}
	if js.on() {
		if entries == nil {
			entries = []configEntry{}
		}
		return js.write(out, entries)
	}
	for _, e := range entries {
		src := ""
		if e.Source != config.FileName {
			src = "  (" + e.Source + ")"
		}
		fmt.Fprintf(out, "%s = %s%s\n", e.Key, e.Value, src)
	}
	return nil
}
