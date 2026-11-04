package main

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// The shell completion of ids, fields, values and filters: "git board
// completion bash|zsh|fish|powershell" prints the script, and the shell
// asks the binary for each word. A completion never fails: on any error,
// or a panic, it offers nothing.

const noFiles = cobra.ShellCompDirectiveNoFileComp

// addCompletions gives the commands their argument and flag completions,
// by name.
func addCompletions(root *cobra.Command) {
	byName := map[string]cobra.CompletionFunc{
		"show": firstIssue, "comment": firstIssue, "edit": firstIssue, "get": firstIssue,
		"close": firstIssue, "reopen": firstIssue, "log": firstIssue, "branch": firstIssue,
		"move": completeMove, "set": completeSet, "new": completeNew,
		"ls": completeFilterArg, "tui": completeFilterArg,
		"view": completeView, "me": completeMe,
	}
	flags := map[string]map[string]cobra.CompletionFunc{
		"ls":  {"group": completeFieldNames, "sort": completeSortKeys},
		"log": {"field": completeFieldNames},
		"set": {"filter": completeFilter},
	}
	for _, c := range root.Commands() {
		if f, ok := byName[c.Name()]; ok && c.ValidArgsFunction == nil {
			c.ValidArgsFunction = safe(f)
		}
		for name, f := range flags[c.Name()] {
			if c.Flags().Lookup(name) != nil {
				_ = c.RegisterFlagCompletionFunc(name, safe(f))
			}
		}
	}
}

// safe is f, offering nothing instead of a panic.
func safe(f cobra.CompletionFunc) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) (res []cobra.Completion, d cobra.ShellCompDirective) {
		defer func() {
			if recover() != nil {
				res, d = nil, noFiles
			}
		}()
		return f(cmd, args, toComplete)
	}
}

// firstIssue completes a command's <id>, its first argument.
func firstIssue(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFiles
	}
	return issueCompletions(toComplete), noFiles
}

// issueCompletions are the issues whose id or project number starts with
// prefix: the short id, as ls shows it, and the project number, each with
// the title.
func issueCompletions(prefix string) []cobra.Completion {
	b, err := store.OpenCwd()
	if err != nil {
		return nil
	}
	defer b.Close()
	issues, err := b.ListIssues()
	if err != nil {
		return nil
	}
	// the open ones first
	slices.SortStableFunc(issues, func(x, y store.Issue) int {
		return strings.Compare(y.Status, x.Status)
	})
	var res []cobra.Completion
	for _, is := range issues {
		desc := is.ShownTitle()
		if is.Status == "closed" {
			desc += " (closed)"
		}
		// a numbered issue goes by its number, by its id once one is typed
		if strings.HasPrefix(is.ID, prefix) && (prefix != "" || is.Number == "") {
			id := shortID(is.ID)
			if len(prefix) > len(id) {
				id = is.ID
			}
			res = append(res, cobra.CompletionWithDesc(id, desc))
		}
		if is.Number != "" && strings.HasPrefix(strings.ToLower(is.Number), strings.ToLower(prefix)) {
			res = append(res, cobra.CompletionWithDesc(is.Number, desc))
		}
	}
	return res
}

// completeMove is move <id> <status>.
func completeMove(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return issueCompletions(toComplete), noFiles
	case 1:
		cfg, err := loadConfig()
		if err != nil {
			return nil, noFiles
		}
		return withPrefix(toComplete, "", cfg.Fields["status"].Options), noFiles
	}
	return nil, noFiles
}

// completeSet is set <id> field=value..., or set --filter f field=value...
func completeSet(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) == 0 && !cmd.Flags().Changed("filter") {
		return issueCompletions(toComplete), noFiles
	}
	return assignmentCompletions(toComplete)
}

// completeNew is new [title] [field=value...]: the fields once the title
// is there, or after a "=".
func completeNew(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) == 0 && !strings.Contains(toComplete, "=") {
		return nil, noFiles
	}
	return assignmentCompletions(toComplete)
}

// assignmentCompletions complete a field=value: the field's name, then its
// values, after a + or a - too.
func assignmentCompletions(toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, noFiles
	}
	name, value, ok := strings.Cut(toComplete, "=")
	if !ok {
		names := append(slices.Clone(cfg.FieldOrder()), "label")
		var res []cobra.Completion
		for _, n := range names {
			if strings.HasPrefix(n, name) {
				res = append(res, n+"=")
			}
		}
		return res, noFiles | cobra.ShellCompDirectiveNoSpace
	}
	sign := ""
	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		sign, value = value[:1], value[1:]
	}
	return withPrefix(value, name+"="+sign, fieldValues(cfg, name)), noFiles
}

// fieldValues are what a field may hold: its options; @current, @next,
// @previous and the sprints for a sprint field; @me for a person; else the
// values on the issues. label is the [labels] list, else the plain labels
// on the issues.
func fieldValues(cfg *config.Config, name string) []string {
	if name == "label" {
		if cfg.Labels.Defined {
			return cfg.Labels.List
		}
		return labelValues("")
	}
	if opts := cfg.Fields[name].Options; len(opts) > 0 {
		return opts
	}
	switch cfg.TypeOf(name, nil) {
	case config.TypeIteration:
		vals := []string{"@current", "@next", "@previous"}
		for _, s := range cfg.Sprints.List {
			vals = append(vals, s.Slug)
		}
		return vals
	case config.TypePerson:
		return append([]string{"@me"}, labelValues(name)...)
	}
	return labelValues(name)
}

// labelValues are the values of field on the issues, sorted: the plain
// labels for "".
func labelValues(field string) []string {
	b, err := store.OpenCwd()
	if err != nil {
		return nil
	}
	defer b.Close()
	issues, err := b.ListIssues()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, is := range issues {
		for _, l := range is.Labels {
			k, v, ok := strings.Cut(l, ":")
			switch {
			case field == "" && !ok:
				seen[l] = true
			case field != "" && ok && k == field:
				seen[v] = true
			}
		}
	}
	vals := make([]string, 0, len(seen))
	for v := range seen {
		vals = append(vals, v)
	}
	slices.Sort(vals)
	return vals
}

// withPrefix are the values starting with typed, each after prefix.
func withPrefix(typed, prefix string, values []string) []cobra.Completion {
	var res []cobra.Completion
	for _, v := range values {
		if strings.HasPrefix(v, typed) {
			res = append(res, prefix+v)
		}
	}
	return res
}

// completeFilterArg completes the [filter] of ls and tui.
func completeFilterArg(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFiles
	}
	return completeFilter(cmd, args, toComplete)
}

// completeFilter completes the last word of a filter: a qualifier, then
// its values. The words before it stay, for a filter in quotes.
func completeFilter(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, noFiles
	}
	before, word := cutLast(toComplete, " ")
	if before != "" {
		before += " "
	}
	if strings.HasPrefix(word, "-") {
		before, word = before+"-", word[1:]
	}
	key, value, ok := strings.Cut(word, ":")
	if !ok {
		quals := []string{"is:", "author:", "involves:", "follows:", "label:", "no:", "title:"}
		for _, f := range cfg.FieldOrder() {
			quals = append(quals, f+":")
		}
		return withPrefix(word, before, quals), noFiles | cobra.ShellCompDirectiveNoSpace
	}
	var values []string
	switch key {
	case "is":
		values = []string{"open", "closed"}
	case "author", "involves", "follows":
		values = []string{"@me"}
	case "no":
		values = cfg.FieldOrder()
	case "title":
		return nil, noFiles
	default:
		values = fieldValues(cfg, key)
	}
	return withPrefix(value, before+key+":", values), noFiles
}

// completeView completes view's <name>.
func completeView(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFiles
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, noFiles
	}
	var res []cobra.Completion
	for _, name := range cfg.ViewNames() {
		if strings.HasPrefix(name, toComplete) {
			res = append(res, cobra.CompletionWithDesc(name, cfg.Views[name].Filter))
		}
	}
	return res, noFiles
}

// completeMe completes me's [person-id]: the people on the board.
func completeMe(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFiles
	}
	b, err := store.OpenCwd()
	if err != nil {
		return nil, noFiles
	}
	defer b.Close()
	persons, err := b.Identities()
	if err != nil {
		return nil, noFiles
	}
	var res []cobra.Completion
	for _, p := range persons {
		if strings.HasPrefix(p.ID, toComplete) {
			res = append(res, cobra.CompletionWithDesc(shortID(p.ID), personLine(p)))
		}
	}
	return res, noFiles
}

// completeFieldNames completes a flag that takes a field: --group, --field.
func completeFieldNames(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, noFiles
	}
	return withPrefix(toComplete, "", cfg.FieldOrder()), noFiles
}

// completeSortKeys completes --sort: created, updated, title or a field.
func completeSortKeys(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, noFiles
	}
	keys := append([]string{"created", "updated", "title"}, cfg.FieldOrder()...)
	return withPrefix(toComplete, "", keys), noFiles
}
