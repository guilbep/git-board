package cli

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

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/conf"
)

// gitKey is a git config key git-board reads, this clone's own (ADR
// 0011): what a value may be, and where to set it instead when config set
// shouldn't.
type gitKey struct {
	check   func(string) error // nil: any value
	instead string             // set elsewhere: the command that does
	note    string             // said on stderr once it is set
}

var gitKeys = map[string]gitKey{
	"board.me": {instead: "git board me <id> chooses who you write as"},
	"board.view": {check: func(v string) error { return oneOf(v, "classic", "cycle") },
		note: "board.view is v1's: v2 reads it nowhere. A [cycle] table in .git-board.toml turns the cycle on (Settings › Cycle)"},
	"board.avatars": {check: func(v string) error {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "forge", "gravatar", "none":
			return nil
		}
		return oneOf(v, "forge", "gravatar", "none")
	}},
	app.DatastoreKey: {check: func(v string) error {
		if v == "git" || v == "forge" || strings.HasPrefix(v, "markdown:") || isPostgres(v) {
			return nil
		}
		return fmt.Errorf("%q: git, forge, markdown:<dir> or postgres://…", v)
	}},
	app.LocalExperimental: {check: func(v string) error {
		for _, f := range strings.Split(v, ",") {
			if !slices.Contains(app.Experimental, strings.TrimSpace(f)) {
				return fmt.Errorf("%q: not an experimental feature (cycle, agents, worktrees)", f)
			}
		}
		return nil
	}},
}

func isPostgres(v string) bool {
	return strings.HasPrefix(v, "postgres://") || strings.HasPrefix(v, "postgresql://")
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
	if key == app.DatastoreKey && isPostgres(v) {
		if u, err := url.Parse(v); err == nil {
			return u.Redacted()
		}
		return "postgres://…"
	}
	return v
}

func newConfig() *cobra.Command {
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
board.avatars, board.datastore, board.experimental (board.view is v1's,
and v2 reads it nowhere). set
writes it in this repository's config (--local), get reads it at every
level git does. A datastore URL is printed without its password.

.git-board.toml has a schema version (docs/guide/config-schema.md):
schema shows it, upgrade and downgrade move the file between versions.
An older file is read as if upgraded, and written upgraded by set and
unset, which say so.`,
		Example: `  git board config list
  git board config get views.backlog.filter
  git board config set views.backlog.filter "is:open prio:p1"
  git board config set sprints.length_days 14
  git board config unset github-project.write
  git board config set board.avatars gravatar
  git board config schema
  git board config upgrade --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	var js *jsonOut
	list := &cobra.Command{
		Use:   "list",
		Short: "List the keys set: the file's, then this clone's",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return configList(cmd.OutOrStdout(), js)
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
				return configSet(cmd.ErrOrStderr(), args[0], args[1], false)
			},
		},
		&cobra.Command{
			Use:   "unset <key>",
			Short: "Remove a key",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return configSet(cmd.ErrOrStderr(), args[0], "", true)
			},
		},
		newConfigSchema(), newConfigMigrate(true), newConfigMigrate(false))
	return cmd
}

// knownGitKey is a board.* key's rules, or a usage error.
func knownGitKey(key string) (gitKey, error) {
	k, ok := gitKeys[key]
	if !ok {
		var names []string
		for n := range gitKeys {
			names = append(names, n)
		}
		slices.Sort(names)
		return k, usageError{fmt.Errorf("%s: git-board doesn't know this key; this clone's keys are %s", key, strings.Join(names, ", "))}
	}
	return k, nil
}

func gitConfigGet(key string) string {
	out, err := exec.Command("git", "config", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func configGet(key string) (string, error) {
	if strings.HasPrefix(key, "board.") {
		if _, err := knownGitKey(key); err != nil {
			return "", err
		}
		v := gitConfigGet(key)
		if v == "" {
			return "", &app.Error{Class: app.NotFound, Err: fmt.Errorf("not found: %s is not set", key)}
		}
		return shownValue(key, v), nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	v, err := app.GetConfig(dir, key)
	if err != nil {
		return "", err
	}
	var s struct{ V string }
	if _, err := toml.Decode("v = "+v, &s); err == nil && strings.HasPrefix(v, `"`) {
		return s.V, nil // a string, as git config prints one: unquoted
	}
	return v, nil
}

func configSet(stderr io.Writer, key, value string, unset bool) error {
	if strings.HasPrefix(key, "board.") {
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
		if k.note != "" && !unset {
			fmt.Fprintln(stderr, "note: "+k.note)
		}
		return nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	lit := ""
	if !unset {
		lit = conf.Value(value)
	}
	w, err := app.SetConfig(dir, key, lit)
	if n := w.Notice(); n != "" && err == nil {
		fmt.Fprintln(stderr, n)
	}
	return err
}

// configEntry is a key config list prints.
type configEntry struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"` // .git-board.toml, or git config's scope
}

func configList(out io.Writer, js *jsonOut) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	var entries []configEntry
	if data, err := os.ReadFile(app.ConfigPath(dir)); err == nil {
		kvs, err := conf.ListKeys(string(data))
		if err != nil {
			return fmt.Errorf("parsing %s: %w", conf.FileName, err)
		}
		for _, kv := range kvs {
			entries = append(entries, configEntry{kv.Key, kv.Value, conf.FileName})
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
		if e.Source != conf.FileName {
			src = "  (" + e.Source + ")"
		}
		fmt.Fprintf(out, "%s = %s%s\n", e.Key, e.Value, src)
	}
	return nil
}
