// Command flowgen draws how the data flows through git-board v2, from the
// code: docs/spec/data-flow.html.
//
// The map is read from the code: the packages the binary links
// (cmd/git-board and what it imports), their imports and their
// package comments. The stories are docs/spec/flows.json: each step names
// the packages a call goes through and the functions it calls. flowgen
// checks every one of them against the code, for every value of the
// page's selectors (the datastore, the forge): a hop that is no import, a
// function that isn't there, a route no handler serves, and nothing is
// written. "What lives where" is the table of docs/spec/data-flow.md.
//
//	go run ./tools/flowgen           write docs/spec/data-flow.html
//	go run ./tools/flowgen -check    fail if it isn't up to date
//
// It imports nothing of git-board's, and nothing outside the standard
// library (ADR 0017): it reads the code as text, as layers_test.go does.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const module = "github.com/guilbep/git-board/"

//go:embed page.html
var page string

func main() {
	root := flag.String("root", ".", "the repository")
	flows := flag.String("flows", "docs/spec/flows.json", "the stories")
	out := flag.String("out", "docs/spec/data-flow.html", "the page")
	check := flag.Bool("check", false, "fail if the page isn't what the code and the stories draw")
	flag.Parse()
	html, err := Generate(*root, *flows)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	path := *out
	if !filepath.IsAbs(path) {
		path = filepath.Join(*root, path)
	}
	if *check {
		cur, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(cur, html) {
			fmt.Fprintf(os.Stderr, "error: %s is not up to date: run make flow\n", *out)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(path, html, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// Flows is docs/spec/flows.json.
type Flows struct {
	Binary    string     `json:"binary"`    // the main package: what it links is the map
	Hide      []string   `json:"hide"`      // packages left off the map
	Externals []External `json:"externals"` // what isn't a package: the browser, the forges
	Params    []Param    `json:"params"`    // the page's selectors
	Where     *Where     `json:"where"`     // the spec's "what lives where"
	Stories   []Story    `json:"stories"`
}

type External struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Sub   string   `json:"sub,omitempty"`
	Side  string   `json:"side"` // left or right of the row of Near
	Near  string   `json:"near"`
	Links []string `json:"links"` // the packages it talks to
	Doc   string   `json:"doc,omitempty"`
}

type Param struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Options []Option `json:"options"`
}

type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Pkg  string `json:"pkg"`            // $<param> in a hop or a call
	Type string `json:"type,omitempty"` // $<param>Type in a call: the package's type
}

type Where struct {
	File    string         `json:"file"`
	Section string         `json:"section"`
	Param   string         `json:"param"`
	Columns map[string]int `json:"columns"` // option id → the table's column
	Rows    [][]string     `json:"rows"`    // filled by flowgen: label, then a cell per option, in Params order
	Heading []string       `json:"heading"` // filled by flowgen
}

type Story struct {
	ID      string   `json:"id"`
	Tab     string   `json:"tab"`
	Title   string   `json:"title"`
	Intro   string   `json:"intro"`
	Toggles []Toggle `json:"toggles,omitempty"`
	Steps   []Step   `json:"steps"`
}

type Toggle struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Tag   string `json:"tag,omitempty"`
}

// Step is one move of the packet: along Hops ("a -> b -> c", each pair an
// import, either way), or a pulse on Node. Calls are the functions the
// step names ("pkg.Func", "pkg.Type.Method", "$store.$storeType.Add"),
// Routes the HTTP patterns ("surface/http:POST /api/…") a package serves.
type Step struct {
	Hops   string            `json:"hops,omitempty"`
	Node   string            `json:"node,omitempty"`
	Calls  []string          `json:"calls,omitempty"`
	Routes []string          `json:"routes,omitempty"`
	Label  string            `json:"label"`
	Text   string            `json:"text,omitempty"`
	By     map[string]string `json:"by,omitempty"` // a text per value of the step's param: "store=md"
	When   string            `json:"when,omitempty"`
	Warn   bool              `json:"warn,omitempty"`
	Fan    bool              `json:"fan,omitempty"` // an event to every page
}

// Pkg is one package of next/, as the map draws it.
type Pkg struct {
	ID      string   `json:"id"` // its path in the module
	Doc     string   `json:"doc"`
	Imports []string `json:"imports"`
	Files   int      `json:"files"`

	syms   map[string]bool
	embeds map[string][]string // a type → the types it embeds, "pkg.T"
	source []byte
}

// Generate reads the code and the stories, checks one against the other,
// and gives the page.
func Generate(root, flowsPath string) ([]byte, error) {
	if !filepath.IsAbs(flowsPath) {
		flowsPath = filepath.Join(root, flowsPath)
	}
	raw, err := os.ReadFile(flowsPath)
	if err != nil {
		return nil, err
	}
	var f Flows
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", flowsPath, err)
	}
	pkgs, err := linked(root, f.Binary)
	if err != nil {
		return nil, err
	}
	for _, h := range f.Hide {
		if _, ok := pkgs[h]; !ok {
			return nil, fmt.Errorf("%s: hide names %s, which the binary doesn't link", flowsPath, h)
		}
		delete(pkgs, h)
	}
	for _, p := range pkgs {
		p.Imports = slices.DeleteFunc(p.Imports, func(i string) bool { _, ok := pkgs[i]; return !ok })
	}
	if err := verify(&f, pkgs); err != nil {
		return nil, fmt.Errorf("%s doesn't match the code:\n%w", flowsPath, err)
	}
	if f.Where != nil {
		if err := readWhere(root, &f); err != nil {
			return nil, err
		}
	}
	list := make([]*Pkg, 0, len(pkgs))
	for _, p := range pkgs {
		list = append(list, p)
	}
	slices.SortFunc(list, func(a, b *Pkg) int { return strings.Compare(a.ID, b.ID) })
	data, err := json.Marshal(map[string]any{
		"binary":    f.Binary,
		"packages":  list,
		"externals": f.Externals,
		"params":    f.Params,
		"where":     f.Where,
		"stories":   f.Stories,
	})
	if err != nil {
		return nil, err
	}
	// </script> can't appear in JSON's strings once < is escaped, which
	// encoding/json does.
	return []byte(strings.Replace(page, "/*FLOW-DATA*/null", string(data), 1)), nil
}

// linked is the binary's package and every package of the module it imports,
// read from their non-test files.
func linked(root, binary string) (map[string]*Pkg, error) {
	pkgs := map[string]*Pkg{}
	var visit func(id string) error
	visit = func(id string) error {
		if _, ok := pkgs[id]; ok {
			return nil
		}
		p, err := readPkg(root, id)
		if err != nil {
			return err
		}
		pkgs[id] = p
		for _, i := range p.Imports {
			if err := visit(i); err != nil {
				return err
			}
		}
		return nil
	}
	return pkgs, visit(binary)
}

func readPkg(root, id string) (*Pkg, error) {
	dir := filepath.Join(root, filepath.FromSlash(id))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("package %s: %w", id, err)
	}
	p := &Pkg{ID: id, Imports: []string{}, syms: map[string]bool{}, embeds: map[string][]string{}}
	imports := map[string]bool{}
	fset := token.NewFileSet()
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		p.source = append(p.source, src...)
		file, err := parser.ParseFile(fset, n, src, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		p.Files++
		if file.Doc != nil && p.Doc == "" {
			p.Doc = synopsis(file.Doc.Text())
		}
		alias := map[string]string{} // the file's names for the module's packages
		for _, imp := range file.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			rest, ok := strings.CutPrefix(path, module)
			if !ok {
				continue
			}
			imports[rest] = true
			name := rest[strings.LastIndex(rest, "/")+1:]
			if imp.Name != nil {
				name = imp.Name.Name
			}
			alias[name] = rest
		}
		symbols(p, file, alias)
	}
	if p.Files == 0 {
		return nil, fmt.Errorf("package %s has no Go files", id)
	}
	for i := range imports {
		p.Imports = append(p.Imports, i)
	}
	slices.Sort(p.Imports)
	return p, nil
}

// symbols records what a file declares: funcs, types, methods ("T.M",
// whatever the receiver's pointer or type parameters), an interface's
// methods, consts and vars; and what a struct embeds.
func symbols(p *Pkg, file *ast.File, alias map[string]string) {
	for _, d := range file.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil || len(d.Recv.List) == 0 {
				p.syms[d.Name.Name] = true
				continue
			}
			if t := typeName(d.Recv.List[0].Type); t != "" {
				p.syms[t+"."+d.Name.Name] = true
			}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.ValueSpec:
					for _, n := range s.Names {
						p.syms[n.Name] = true
					}
				case *ast.TypeSpec:
					p.syms[s.Name.Name] = true
					switch t := s.Type.(type) {
					case *ast.InterfaceType:
						for _, m := range t.Methods.List {
							for _, n := range m.Names {
								p.syms[s.Name.Name+"."+n.Name] = true
							}
						}
					case *ast.StructType:
						for _, fl := range t.Fields.List {
							if len(fl.Names) > 0 {
								continue
							}
							if e := embedded(fl.Type, p.ID, alias); e != "" {
								p.embeds[s.Name.Name] = append(p.embeds[s.Name.Name], e)
							}
						}
					}
				}
			}
		}
	}
}

// typeName is a receiver's type: T, *T, T[A], *T[A, B].
func typeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.IndexExpr:
		return typeName(t.X)
	case *ast.IndexListExpr:
		return typeName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// embedded is an embedded field's type, as "pkg.T": its own package's, or
// another package of the module.
func embedded(e ast.Expr, self string, alias map[string]string) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return embedded(t.X, self, alias)
	case *ast.Ident:
		return self + "." + t.Name
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			if path, ok := alias[x.Name]; ok {
				return path + "." + t.Sel.Name
			}
		}
	}
	return ""
}

// synopsis is a package comment's first sentence, without "Package x is".
func synopsis(doc string) string {
	doc = strings.Join(strings.Fields(doc), " ")
	if i := strings.Index(doc, ". "); i >= 0 {
		doc = doc[:i+1]
	}
	return doc
}

var paramRef = regexp.MustCompile(`\$([a-z]+)(Type)?`)

// verify checks every story against the code, for every combination of
// the selectors a step names.
func verify(f *Flows, pkgs map[string]*Pkg) error {
	var errs []error
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf("  "+format, a...)) }
	params := map[string]Param{}
	for _, p := range f.Params {
		params[p.ID] = p
		for _, o := range p.Options {
			if _, ok := pkgs[o.Pkg]; !ok {
				bad("param %s: option %s names %s, which the binary doesn't link", p.ID, o.ID, o.Pkg)
			}
		}
	}
	ext := map[string]External{}
	for _, x := range f.Externals {
		ext[x.ID] = x
		if _, ok := pkgs[x.Near]; !ok {
			bad("external %s: near %s, which isn't on the map", x.ID, x.Near)
		}
		for _, l := range x.Links {
			if _, ok := pkgs[l]; !ok {
				bad("external %s: links %s, which isn't on the map", x.ID, l)
			}
		}
	}
	linkedTo := func(a, b string) bool {
		if pa, ok := pkgs[a]; ok && slices.Contains(pa.Imports, b) {
			return true
		}
		if pb, ok := pkgs[b]; ok && slices.Contains(pb.Imports, a) {
			return true
		}
		for _, pair := range [][2]string{{a, b}, {b, a}} {
			if x, ok := ext[pair[0]]; ok && slices.Contains(x.Links, pair[1]) {
				return true
			}
		}
		return false
	}
	ids := map[string]bool{}
	for _, s := range f.Stories {
		if ids[s.ID] {
			bad("story %s: twice", s.ID)
		}
		ids[s.ID] = true
		toggles := map[string]bool{}
		for _, t := range s.Toggles {
			toggles[t.ID] = true
		}
		for i, st := range s.Steps {
			where := fmt.Sprintf("story %s, step %d (%s)", s.ID, i+1, st.Label)
			if (st.Hops == "") == (st.Node == "") {
				bad("%s: a step has hops or a node, one of them", where)
				continue
			}
			if st.When != "" {
				if k, v, ok := strings.Cut(st.When, "="); ok {
					if p, known := params[k]; !known || !slices.ContainsFunc(p.Options, func(o Option) bool { return o.ID == v }) {
						bad("%s: when %s names no option", where, st.When)
					}
				} else if !toggles[st.When] {
					bad("%s: when %s names no toggle of the story", where, st.When)
				}
			}
			for k := range st.By {
				pk, pv, ok := strings.Cut(k, "=")
				p, known := params[pk]
				if !ok || !known || !slices.ContainsFunc(p.Options, func(o Option) bool { return o.ID == pv }) {
					bad("%s: by %s names no option", where, k)
				}
			}
			for _, combo := range combos(st, f.Params) {
				sub := func(s string) string {
					return paramRef.ReplaceAllStringFunc(s, func(m string) string {
						g := paramRef.FindStringSubmatch(m)
						o := combo[g[1]]
						if g[2] != "" {
							return o.Type
						}
						return o.Pkg
					})
				}
				label := where
				if len(combo) > 0 {
					var vs []string
					for k, o := range combo {
						vs = append(vs, k+"="+o.ID)
					}
					slices.Sort(vs)
					label += " [" + strings.Join(vs, " ") + "]"
				}
				if st.Node != "" {
					if _, ok := pkgs[sub(st.Node)]; !ok {
						if _, ok := ext[st.Node]; !ok {
							bad("%s: node %s isn't on the map", label, sub(st.Node))
						}
					}
				}
				if st.Hops != "" {
					hops := strings.Split(sub(st.Hops), "->")
					for j := range hops {
						hops[j] = strings.TrimSpace(hops[j])
					}
					for j := 0; j+1 < len(hops); j++ {
						if !linkedTo(hops[j], hops[j+1]) {
							bad("%s: %s and %s: neither imports the other", label, hops[j], hops[j+1])
						}
					}
				}
				for _, c := range st.Calls {
					if err := hasSym(pkgs, sub(c)); err != nil {
						bad("%s: %v", label, err)
					}
				}
				for _, r := range st.Routes {
					pkg, pattern, _ := strings.Cut(sub(r), ":")
					p, ok := pkgs[pkg]
					if !ok {
						bad("%s: route %s: no package %s", label, r, pkg)
					} else if !bytes.Contains(p.source, []byte(strconv.Quote(pattern))) {
						bad("%s: route %q isn't in %s", label, pattern, pkg)
					}
				}
			}
		}
	}
	return errors.Join(errs...)
}

// combos is every assignment of the params a step names to an option.
func combos(st Step, params []Param) []map[string]Option {
	all := st.Hops + st.Node + strings.Join(st.Calls, " ") + strings.Join(st.Routes, " ")
	out := []map[string]Option{{}}
	for _, p := range params {
		if !strings.Contains(all, "$"+p.ID) {
			continue
		}
		var next []map[string]Option
		for _, c := range out {
			for _, o := range p.Options {
				m := map[string]Option{p.ID: o}
				for k, v := range c {
					m[k] = v
				}
				next = append(next, m)
			}
		}
		out = next
	}
	return out
}

// hasSym says whether pkg.Name or pkg.Type.Method is declared, a method
// also through the types the type embeds.
func hasSym(pkgs map[string]*Pkg, ref string) error {
	// a package's path has no dot: it ends at the first dot after the
	// last slash
	slash := strings.LastIndex(ref, "/")
	dot := strings.Index(ref[slash+1:], ".")
	if dot < 0 {
		return fmt.Errorf("call %s: want pkg.Name or pkg.Type.Method", ref)
	}
	dot += slash + 1
	pkg, ok := pkgs[ref[:dot]]
	if !ok {
		return fmt.Errorf("call %s: %s isn't on the map", ref, ref[:dot])
	}
	name := ref[dot+1:]
	seen := map[string]bool{}
	var find func(p *Pkg, name string) bool
	find = func(p *Pkg, name string) bool {
		if p.syms[name] {
			return true
		}
		t, m, ok := strings.Cut(name, ".")
		if !ok || seen[p.ID+"."+t] {
			return false
		}
		seen[p.ID+"."+t] = true
		for _, e := range p.embeds[t] {
			k := strings.LastIndex(e, ".")
			if q, ok := pkgs[e[:k]]; ok && find(q, e[k+1:]+"."+m) {
				return true
			}
		}
		return false
	}
	if !find(pkg, name) {
		return fmt.Errorf("call %s: %s declares no %s", ref, pkg.ID, name)
	}
	return nil
}

// readWhere fills the table from the spec's section: its first column,
// then a cell per option of the param.
func readWhere(root string, f *Flows) error {
	w := f.Where
	raw, err := os.ReadFile(filepath.Join(root, w.File))
	if err != nil {
		return err
	}
	var p *Param
	for i := range f.Params {
		if f.Params[i].ID == w.Param {
			p = &f.Params[i]
		}
	}
	if p == nil {
		return fmt.Errorf("where: no param %s", w.Param)
	}
	lines := strings.Split(string(raw), "\n")
	in := false
	var table [][]string
	for _, l := range lines {
		if strings.HasPrefix(l, "## ") {
			in = strings.TrimSpace(l[3:]) == w.Section
			continue
		}
		if !in || !strings.HasPrefix(strings.TrimSpace(l), "|") {
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(l), "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if strings.Trim(strings.Join(cells, ""), "-: ") == "" {
			continue // the separator
		}
		table = append(table, cells)
	}
	if len(table) < 2 {
		return fmt.Errorf("%s: no table under ## %s", w.File, w.Section)
	}
	col := func(row []string, o Option) (string, error) {
		c, ok := w.Columns[o.ID]
		if !ok || c >= len(row) {
			return "", fmt.Errorf("where: no column for %s", o.ID)
		}
		return row[c], nil
	}
	w.Heading = nil
	for _, o := range p.Options {
		h, err := col(table[0], o)
		if err != nil {
			return err
		}
		w.Heading = append(w.Heading, h)
	}
	w.Rows = nil
	for _, row := range table[1:] {
		out := []string{row[0]}
		for _, o := range p.Options {
			c, err := col(row, o)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		w.Rows = append(w.Rows, out)
	}
	return nil
}
