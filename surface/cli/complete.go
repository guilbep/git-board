package cli

import (
	"context"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/domain/issues"
)

// The shell completion of ids, fields, values and filters, v1's: "git
// board completion bash|zsh|fish|powershell" prints the script, and the
// shell asks the binary for each word (__complete). A completion never
// fails: on any error, or a panic, it offers nothing. It reads the
// issues from the view.

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

// completing opens the board for a completion, saying nothing of what it
// leaves aside: nil when it doesn't open.
func completing(cmd *cobra.Command) *app.Board {
	ctx := ctxOf(cmd)
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	b, err := app.Open(ctx, cwd, app.Options{Notice: func(string, ...any) {}})
	if err != nil {
		return nil
	}
	return b
}

// allIssues are the board's issues, open and closed, from the view.
func allIssues(cmd *cobra.Command, b *app.Board) []issues.Issue {
	all, err := b.Issues.List(ctxOf(cmd), issues.ListReq{Filter: "is:open,closed"})
	if err != nil {
		return nil
	}
	return all
}

// ctxOf is cmd's context; a completion may have none.
func ctxOf(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// firstIssue completes a command's <id>, its first argument.
func firstIssue(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFiles
	}
	return issueCompletions(cmd, toComplete), noFiles
}

// issueCompletions are the issues whose id or project number starts with
// prefix: the short id, as ls shows it, and the project number, each
// with the title; the open ones first.
func issueCompletions(cmd *cobra.Command, prefix string) []cobra.Completion {
	b := completing(cmd)
	if b == nil {
		return nil
	}
	defer b.Close()
	all := allIssues(cmd, b)
	slices.SortStableFunc(all, func(x, y issues.Issue) int { return strings.Compare(string(y.Status), string(x.Status)) })
	var res []cobra.Completion
	for _, is := range all {
		desc := is.ShownTitle()
		if is.Status == string(issues.Closed) {
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
func completeMove(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return issueCompletions(cmd, toComplete), noFiles
	case 1:
		b := completing(cmd)
		if b == nil {
			return nil, noFiles
		}
		defer b.Close()
		return withPrefix(toComplete, "", b.Issues.Schema.Fields["status"].Options), noFiles
	}
	return nil, noFiles
}

// completeSet is set <id> field=value..., or set --filter f field=value...
func completeSet(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) == 0 && !cmd.Flags().Changed("filter") {
		return issueCompletions(cmd, toComplete), noFiles
	}
	return assignmentCompletions(cmd, toComplete)
}

// completeNew is new [title] [field=value...]: the fields once the title
// is there, or after a "=".
func completeNew(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) == 0 && !strings.Contains(toComplete, "=") {
		return nil, noFiles
	}
	return assignmentCompletions(cmd, toComplete)
}

// assignmentCompletions complete a field=value: the field's name, then
// its values, after a + or a - too.
func assignmentCompletions(cmd *cobra.Command, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	b := completing(cmd)
	if b == nil {
		return nil, noFiles
	}
	defer b.Close()
	name, value, ok := strings.Cut(toComplete, "=")
	if !ok {
		names := append(slices.Clone(b.Issues.Schema.FieldOrder()), "label")
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
	return withPrefix(value, name+"="+sign, fieldValues(cmd, b, name)), noFiles
}

// fieldValues are what a field may hold: its options; @current, @next,
// @previous and the sprints for a sprint field; @me for a person; else
// the values on the issues. label is the [labels] list, else the plain
// labels on the issues.
func fieldValues(cmd *cobra.Command, b *app.Board, name string) []string {
	s := b.Issues.Schema
	if name == "label" {
		if s.Labels.Defined {
			return s.Labels.List
		}
		return labelValues(cmd, b, "")
	}
	if opts := s.Fields[name].Options; len(opts) > 0 {
		return opts
	}
	switch s.TypeOf(name, nil) {
	case issues.TypeIteration:
		vals := []string{"@current", "@next", "@previous"}
		for _, sp := range s.Sprints.List {
			vals = append(vals, sp.Slug)
		}
		return vals
	case issues.TypePerson:
		return append([]string{"@me"}, labelValues(cmd, b, name)...)
	}
	return labelValues(cmd, b, name)
}

// labelValues are the values of field on the issues, sorted: the plain
// labels for "".
func labelValues(cmd *cobra.Command, b *app.Board, field string) []string {
	seen := map[string]bool{}
	for _, is := range allIssues(cmd, b) {
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
	return slices.Sorted(maps.Keys(seen))
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
func completeFilter(cmd *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	b := completing(cmd)
	if b == nil {
		return nil, noFiles
	}
	defer b.Close()
	order := b.Issues.Schema.FieldOrder()
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
		for _, f := range order {
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
		values = order
	case "title":
		return nil, noFiles
	default:
		values = fieldValues(cmd, b, key)
	}
	return withPrefix(value, before+key+":", values), noFiles
}

// completeView completes view's <name>.
func completeView(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFiles
	}
	b := completing(cmd)
	if b == nil {
		return nil, noFiles
	}
	defer b.Close()
	var res []cobra.Completion
	for _, name := range slices.Sorted(maps.Keys(b.Views)) {
		if strings.HasPrefix(name, toComplete) {
			res = append(res, cobra.CompletionWithDesc(name, b.Views[name].Filter))
		}
	}
	return res, noFiles
}

// completeMe completes me's [person-id]: the people on the board.
func completeMe(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFiles
	}
	b := completing(cmd)
	if b == nil {
		return nil, noFiles
	}
	defer b.Close()
	all, err := b.People.List(ctxOf(cmd))
	if err != nil {
		return nil, noFiles
	}
	var res []cobra.Completion
	for _, p := range all {
		if strings.HasPrefix(string(p.ID), toComplete) {
			res = append(res, cobra.CompletionWithDesc(shortID(string(p.ID)), personLine(p)))
		}
	}
	return res, noFiles
}

// completeFieldNames completes a flag that takes a field: --group,
// --field.
func completeFieldNames(cmd *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	b := completing(cmd)
	if b == nil {
		return nil, noFiles
	}
	defer b.Close()
	return withPrefix(toComplete, "", b.Issues.Schema.FieldOrder()), noFiles
}

// completeSortKeys completes --sort: created, updated, title, author, id, is or a field.
func completeSortKeys(cmd *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	b := completing(cmd)
	if b == nil {
		return nil, noFiles
	}
	defer b.Close()
	keys := append([]string{"created", "updated", "title", "author", "id", "is"}, b.Issues.Schema.FieldOrder()...)
	return withPrefix(toComplete, "", keys), noFiles
}
