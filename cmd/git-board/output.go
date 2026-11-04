package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
)

// jsonOut is a command's --json [fields], --jq and --template: what it
// prints instead of its text, and how.
type jsonOut struct {
	fields string   // --json's value: "true" when it came alone
	jq     string   // --jq
	tmpl   string   // --template
	valid  []string // the fields the command prints

	keep  []string // the fields --json named, nil for all
	query *gojq.Code
	text  *template.Template
}

// jsonFieldsKey is the annotation that names a command's JSON fields, for
// jsonFieldArgs.
const jsonFieldsKey = "json-fields"

// addJSON gives cmd --json, --jq and, withTemplate, --template. fields
// are the names --json may select. Each is checked before the command
// runs, so a wrong one writes nothing.
func addJSON(cmd *cobra.Command, usage string, fields []string, withTemplate bool) *jsonOut {
	o := &jsonOut{valid: fields}
	f := cmd.Flags().VarPF(jsonFlag{&o.fields}, "json", "", usage+"; --json id,title keeps only those fields")
	f.NoOptDefVal = "true"
	cmd.Flags().StringVar(&o.jq, "jq", "", "filter the JSON with a jq `expression`")
	if withTemplate {
		cmd.Flags().StringVar(&o.tmpl, "template", "", "print the JSON through a Go `template`")
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[jsonFieldsKey] = strings.Join(fields, ",")
	_ = cmd.RegisterFlagCompletionFunc("json", completeJSONFields(fields))
	pre := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if err := o.prepare(); err != nil {
			return usageError{err}
		}
		if pre != nil {
			return pre(c, args)
		}
		return nil
	}
	return o
}

// jsonFlag is --json: alone, it reads as a bool flag does ("true"), and
// it takes a list of fields after an "=".
type jsonFlag struct{ v *string }

func (f jsonFlag) String() string     { return *f.v }
func (f jsonFlag) Set(s string) error { *f.v = s; return nil }
func (f jsonFlag) Type() string       { return "bool" } // no value shown in the usage

// on says the command prints JSON, or what --jq or --template make of it.
func (o *jsonOut) on() bool {
	return o != nil && (o.fields != "" && o.fields != "false" || o.jq != "" || o.tmpl != "")
}

// prepare checks the fields, the jq expression and the template.
func (o *jsonOut) prepare() error {
	if o.jq != "" && o.tmpl != "" {
		return errors.New("--jq or --template, not both")
	}
	if o.fields != "" && o.fields != "true" && o.fields != "false" {
		o.keep = nil
		for _, f := range strings.Split(o.fields, ",") {
			f = strings.TrimSpace(f)
			if !slices.Contains(o.valid, f) {
				return fmt.Errorf("--json: unknown field %q; the fields are %s", f, strings.Join(o.valid, ", "))
			}
			if !slices.Contains(o.keep, f) {
				o.keep = append(o.keep, f)
			}
		}
	}
	if o.jq != "" {
		q, err := gojq.Parse(o.jq)
		if err != nil {
			return fmt.Errorf("--jq: %w", err)
		}
		if o.query, err = gojq.Compile(q); err != nil {
			return fmt.Errorf("--jq: %w", err)
		}
	}
	if o.tmpl != "" {
		t, err := template.New("template").Funcs(templateFuncs).Parse(o.tmpl)
		if err != nil {
			return fmt.Errorf("--template: %w", err)
		}
		o.text = t
	}
	return nil
}

// write prints v: as JSON, its --json fields only when it named some,
// then through --jq or --template.
func (o *jsonOut) write(out io.Writer, v any) error {
	if o.keep == nil && o.query == nil && o.text == nil {
		return writeJSON(out, v)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var data any
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	if o.keep != nil {
		data = pick(data, o.keep)
	}
	switch {
	case o.query != nil:
		return runJQ(out, o.query, data)
	case o.text != nil:
		return o.text.Execute(out, data)
	}
	return writeJSON(out, data)
}

// pick keeps the fields of an object, or of each object in a list. A
// field an object lacks is null.
func pick(v any, keep []string) any {
	switch x := v.(type) {
	case []any:
		for i := range x {
			x[i] = pick(x[i], keep)
		}
		return x
	case map[string]any:
		m := make(map[string]any, len(keep))
		for _, k := range keep {
			m[k] = x[k]
		}
		return m
	}
	return v
}

// runJQ prints each value the query yields, a line each: a string as it
// is, anything else as compact JSON, as gh --jq does.
func runJQ(out io.Writer, code *gojq.Code, data any) error {
	iter := code.Run(data)
	for {
		v, ok := iter.Next()
		if !ok {
			return nil
		}
		if err, ok := v.(error); ok {
			var halt *gojq.HaltError
			if errors.As(err, &halt) && halt.Value() == nil {
				return nil
			}
			return fmt.Errorf("--jq: %w", err)
		}
		if s, ok := v.(string); ok {
			fmt.Fprintln(out, s)
			continue
		}
		b, err := gojq.Marshal(v)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s\n", b)
	}
}

// templateFuncs are --template's helpers, gh's names and argument order.
var templateFuncs = template.FuncMap{
	// join sep list: the list's items, sep between
	"join": func(sep string, list any) string {
		var parts []string
		for _, v := range asList(list) {
			parts = append(parts, fmt.Sprint(v))
		}
		return strings.Join(parts, sep)
	},
	// pluck field list: that field of each object in list
	"pluck": func(field string, list any) []any {
		var res []any
		for _, v := range asList(list) {
			if m, ok := v.(map[string]any); ok {
				res = append(res, m[field])
			}
		}
		return res
	},
	// timeago time: "3 days ago"
	"timeago": func(s string) (string, error) {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return "", err
		}
		return timeAgo(time.Since(t)), nil
	},
	// timefmt layout time: the time in a Go layout ("2006-01-02")
	"timefmt": func(layout, s string) (string, error) {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return "", err
		}
		return t.Local().Format(layout), nil
	},
	// truncate n text: at most n characters, "..." ending a cut one
	"truncate": func(n int, s string) string {
		r := []rune(s)
		if len(r) <= n {
			return s
		}
		if n <= 3 {
			return string(r[:n])
		}
		return string(r[:n-3]) + "..."
	},
}

func asList(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case nil:
		return nil
	}
	return []any{v}
}

// timeAgo is a while, as a person says it.
func timeAgo(d time.Duration) string {
	unit := func(n int, name string) string {
		if n == 1 {
			return "1 " + name + " ago"
		}
		return fmt.Sprintf("%d %ss ago", n, name)
	}
	switch {
	case d < time.Minute:
		return "less than a minute ago"
	case d < time.Hour:
		return unit(int(d/time.Minute), "minute")
	case d < 24*time.Hour:
		return unit(int(d/time.Hour), "hour")
	case d < 30*24*time.Hour:
		return unit(int(d/(24*time.Hour)), "day")
	case d < 365*24*time.Hour:
		return unit(int(d/(30*24*time.Hour)), "month")
	}
	return unit(int(d/(365*24*time.Hour)), "year")
}

// jsonFields are the names v prints as JSON, in its order: the struct's
// json tags, an embedded struct's in place.
func jsonFields(v any) []string {
	var names []string
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for i := range t.NumField() {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			name, _, _ := strings.Cut(tag, ",")
			switch {
			case name == "-" || !f.IsExported():
			case f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct:
				walk(f.Type)
			case name == "":
				names = append(names, f.Name)
			default:
				names = append(names, name)
			}
		}
	}
	walk(reflect.TypeOf(v))
	return names
}

var fieldList = regexp.MustCompile(`^[a-z_]+(,[a-z_]+)+$`)

// jsonFieldArgs reads "--json id,title" as "--json=id,title": --json
// alone takes no value, so without it the fields would be an argument.
// The word after --json is taken as fields when it is a list of them
// (a comma in it), or one of the command's fields: "ls --json title"
// prints the titles; "ls title --json" searches for "title".
func jsonFieldArgs(root *cobra.Command, args []string) []string {
	cmd, _, err := root.Find(args)
	if err != nil || cmd.Annotations[jsonFieldsKey] == "" {
		return args
	}
	fields := strings.Split(cmd.Annotations[jsonFieldsKey], ",")
	res := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return append(res, args[i:]...)
		}
		if a == "--json" && i+1 < len(args) {
			next := args[i+1]
			if fieldList.MatchString(next) || slices.Contains(fields, next) {
				res = append(res, "--json="+next)
				i++
				continue
			}
		}
		res = append(res, a)
	}
	return res
}

// runRoot runs the command line args, as main does.
func runRoot(root *cobra.Command, args []string) error {
	root.SetArgs(jsonFieldArgs(root, args))
	return root.Execute()
}

// completeJSONFields completes --json=a,b,<TAB>: the fields not named yet.
func completeJSONFields(fields []string) cobra.CompletionFunc {
	return func(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		done, typed := cutLast(toComplete, ",")
		named := strings.Split(done, ",")
		var res []cobra.Completion
		for _, f := range fields {
			if !strings.HasPrefix(f, typed) || done != "" && slices.Contains(named, f) {
				continue
			}
			if done == "" {
				res = append(res, f)
			} else {
				res = append(res, done+","+f)
			}
		}
		return res, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
	}
}

// cutLast is s before and after its last sep; all of s after when none.
func cutLast(s, sep string) (before, after string) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):]
	}
	return "", s
}
