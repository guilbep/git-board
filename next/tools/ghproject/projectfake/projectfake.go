// Package projectfake is a GitHub Project (v2) for tests: GitHub's
// GraphQL answers for one Project's fields and items, and its REST
// answers for the assignees, served by httptest. It understands the
// queries next/forge/github's ProjectV2 sends, and nothing else.
package projectfake

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Field is a Project field.
type Field struct {
	ID, Name, DataType string
	Choices            []Choice // options or iterations
}

// Choice is an option or an iteration.
type Choice struct{ ID, Name string }

// Item is a Project item: an issue's content, and its values by field
// name, as names (an option's, an iteration's title), numbers, dates and
// texts.
type Item struct {
	ID        string
	Type      string // Issue, PullRequest, DraftIssue
	Number    int
	URL       string
	Title     string
	Assignees []string
	Labels    []string
	Milestone string
	Values    map[string]string
}

// Fake is one Project.
type Fake struct {
	Owner  string
	Number int
	ID     string
	Title  string

	mu         sync.Mutex
	fields     []Field
	items      []Item
	assignable map[string][]string // repo -> logins; nil: anyone
	writes     []string
}

// New is an empty Project number of owner.
func New(owner string, number int) *Fake {
	return &Fake{Owner: owner, Number: number, ID: "PVT_" + owner, Title: owner + "'s Project"}
}

// AddField adds a field.
func (f *Fake) AddField(fl Field) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fields = append(f.fields, fl)
}

// Add adds an item, its id made when it has none.
func (f *Fake) Add(it Item) Item {
	f.mu.Lock()
	defer f.mu.Unlock()
	if it.ID == "" {
		it.ID = fmt.Sprintf("PVTI_%d", len(f.items)+1)
	}
	if it.Type == "" {
		it.Type = "Issue"
	}
	if it.Values == nil {
		it.Values = map[string]string{}
	}
	f.items = append(f.items, it)
	return it
}

// SetAssignable says who can be assigned in repo.
func (f *Fake) SetAssignable(repo string, logins ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.assignable == nil {
		f.assignable = map[string][]string{}
	}
	f.assignable[repo] = logins
}

// Edit changes an item as a teammate on GitHub would.
func (f *Fake) Edit(itemID string, edit func(*Item)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.items {
		if f.items[i].ID == itemID {
			edit(&f.items[i])
		}
	}
}

// Items are the items as they are, copied.
func (f *Fake) Items() []Item {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Item, len(f.items))
	for i, it := range f.items {
		it.Values = clone(it.Values)
		it.Assignees = slices.Clone(it.Assignees)
		it.Labels = slices.Clone(it.Labels)
		out[i] = it
	}
	return out
}

func clone(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Fields are the Project's fields.
func (f *Fake) Fields() []Field {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.fields)
}

// Assignable is who can be assigned in repo: SetAssignable's, else who
// is assigned there now.
func (f *Fake) Assignable(repo string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l, ok := f.assignable[repo]; ok {
		return slices.Clone(l)
	}
	var out []string
	for _, it := range f.items {
		if strings.HasPrefix(it.URL, "https://github.com/"+repo+"/") {
			for _, a := range it.Assignees {
				if !slices.Contains(out, a) {
					out = append(out, a)
				}
			}
		}
	}
	return out
}

// Writes are the writes made through the API, one line each.
func (f *Fake) Writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.writes)
}

// Set writes a field's value on an item by their ids, as GitHub's
// mutations do: kind is option, iteration, number, date, text or clear.
func (f *Fake) Set(itemID, fieldID, kind, arg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var fl *Field
	for i := range f.fields {
		if f.fields[i].ID == fieldID {
			fl = &f.fields[i]
		}
	}
	if fl == nil {
		return fmt.Errorf("no field %s", fieldID)
	}
	var it *Item
	for i := range f.items {
		if f.items[i].ID == itemID {
			it = &f.items[i]
		}
	}
	if it == nil {
		return fmt.Errorf("no item %s", itemID)
	}
	value := arg
	switch kind {
	case "option", "iteration":
		value = ""
		for _, c := range fl.Choices {
			if c.ID == arg {
				value = c.Name
			}
		}
		if value == "" {
			return fmt.Errorf("no option %s on %s", arg, fl.Name)
		}
	case "number":
		x, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return err
		}
		value = strconv.FormatFloat(x, 'f', -1, 64)
	case "clear":
		delete(it.Values, fl.Name)
		f.writes = append(f.writes, fmt.Sprintf("#%d %s cleared", it.Number, fl.Name))
		return nil
	case "date", "text":
	default:
		return fmt.Errorf("a value of kind %q", kind)
	}
	it.Values[fl.Name] = value
	f.writes = append(f.writes, fmt.Sprintf("#%d %s = %s", it.Number, fl.Name, value))
	return nil
}

// Assign adds and removes the assignees of the issue at url.
func (f *Fake) Assign(url string, add, remove []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.items {
		it := &f.items[i]
		if it.URL != url {
			continue
		}
		for _, a := range add {
			if !slices.Contains(it.Assignees, a) {
				it.Assignees = append(it.Assignees, a)
			}
		}
		it.Assignees = slices.DeleteFunc(it.Assignees, func(a string) bool { return slices.Contains(remove, a) })
		f.writes = append(f.writes, fmt.Sprintf("#%d assignees +%v -%v", it.Number, add, remove))
	}
}

// Server serves the Project's API until the test ends: its URL is the
// REST API's base, and GraphQL is at /graphql under it.
func (f *Fake) Server(t testing.TB) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return srv
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/graphql" {
		f.graphql(w, body)
		return
	}
	// /repos/{o}/{r}/assignees, /repos/{o}/{r}/issues/{n}/assignees
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) == 4 && parts[0] == "repos" && parts[3] == "assignees" && r.Method == http.MethodGet:
		var out []map[string]any
		if r.URL.Query().Get("page") == "1" || r.URL.Query().Get("page") == "" {
			for i, l := range f.Assignable(parts[1] + "/" + parts[2]) {
				out = append(out, map[string]any{"id": i + 1, "login": l})
			}
		}
		writeJSON(w, out)
	case len(parts) == 6 && parts[0] == "repos" && parts[3] == "issues" && parts[5] == "assignees":
		var in struct {
			Assignees []string `json:"assignees"`
		}
		_ = json.Unmarshal(body, &in)
		url := "https://github.com/" + parts[1] + "/" + parts[2] + "/issues/" + parts[4]
		switch r.Method {
		case http.MethodPost:
			f.Assign(url, in.Assignees, nil)
		case http.MethodDelete:
			f.Assign(url, nil, in.Assignees)
		}
		writeJSON(w, map[string]any{})
	default:
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func gqlError(w http.ResponseWriter, msg string) {
	writeJSON(w, map[string]any{"errors": []map[string]string{{"message": msg}}})
}

func (f *Fake) graphql(w http.ResponseWriter, body []byte) {
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		gqlError(w, err.Error())
		return
	}
	v := req.Variables
	str := func(k string) string { s, _ := v[k].(string); return s }
	switch {
	case strings.Contains(req.Query, "updateProjectV2ItemFieldValue"), strings.Contains(req.Query, "clearProjectV2ItemFieldValue"):
		if str("p") != f.ID {
			gqlError(w, "no Project "+str("p"))
			return
		}
		kind, arg := "clear", ""
		if val, ok := v["v"].(map[string]any); ok {
			for k, x := range val {
				switch k {
				case "singleSelectOptionId":
					kind, arg = "option", fmt.Sprint(x)
				case "iterationId":
					kind, arg = "iteration", fmt.Sprint(x)
				case "number":
					kind, arg = "number", strconv.FormatFloat(x.(float64), 'f', -1, 64)
				case "date":
					kind, arg = "date", fmt.Sprint(x)
				case "text":
					kind, arg = "text", fmt.Sprint(x)
				}
			}
		}
		if err := f.Set(str("i"), str("f"), kind, arg); err != nil {
			gqlError(w, err.Error())
			return
		}
		writeJSON(w, map[string]any{"data": map[string]any{}})
	case !strings.Contains(req.Query, "projectV2(number"):
		gqlError(w, "the fake doesn't answer this query")
	case str("owner") != f.Owner || fmt.Sprint(v["n"]) != strconv.Itoa(f.Number):
		writeJSON(w, map[string]any{"data": map[string]any{"repositoryOwner": map[string]any{"projectV2": nil}}})
	case strings.Contains(req.Query, "items(first"):
		f.itemsPage(w, v)
	default:
		f.meta(w)
	}
}

func (f *Fake) meta(w http.ResponseWriter) {
	var nodes []map[string]any
	for _, fl := range f.Fields() {
		n := map[string]any{"id": fl.ID, "name": fl.Name, "dataType": fl.DataType}
		var cs []map[string]any
		for _, c := range fl.Choices {
			cs = append(cs, map[string]any{"id": c.ID, "name": c.Name, "title": c.Name})
		}
		switch fl.DataType {
		case "SINGLE_SELECT":
			n["options"] = cs
		case "ITERATION":
			n["configuration"] = map[string]any{"iterations": cs, "completedIterations": []any{}}
		}
		nodes = append(nodes, n)
	}
	writeJSON(w, map[string]any{"data": map[string]any{"repositoryOwner": map[string]any{"projectV2": map[string]any{
		"id": f.ID, "title": f.Title, "fields": map[string]any{"nodes": nodes}}}}})
}

func (f *Fake) itemsPage(w http.ResponseWriter, v map[string]any) {
	first := 100
	if x, ok := v["first"].(float64); ok {
		first = int(x)
	}
	start := 0
	if s, ok := v["after"].(string); ok {
		start, _ = strconv.Atoi(s)
	}
	types := map[string]string{}
	for _, fl := range f.Fields() {
		types[fl.Name] = fl.DataType
	}
	items := f.Items()
	end := min(start+first, len(items))
	var nodes []map[string]any
	for _, it := range items[start:end] {
		content := map[string]any{"__typename": it.Type, "number": it.Number, "url": it.URL, "title": it.Title}
		if it.Type == "Issue" {
			var as, ls []map[string]string
			for _, a := range it.Assignees {
				as = append(as, map[string]string{"login": a})
			}
			for _, l := range it.Labels {
				ls = append(ls, map[string]string{"name": l})
			}
			content["assignees"] = map[string]any{"nodes": as}
			content["labels"] = map[string]any{"nodes": ls}
			if it.Milestone != "" {
				content["milestone"] = map[string]string{"title": it.Milestone}
			}
		}
		var vals []map[string]any
		for name, val := range it.Values {
			n := map[string]any{"field": map[string]string{"name": name}}
			switch types[name] {
			case "SINGLE_SELECT":
				n["__typename"], n["name"] = "ProjectV2ItemFieldSingleSelectValue", val
			case "ITERATION":
				n["__typename"], n["title"] = "ProjectV2ItemFieldIterationValue", val
			case "NUMBER":
				x, _ := strconv.ParseFloat(val, 64)
				n["__typename"], n["number"] = "ProjectV2ItemFieldNumberValue", x
			case "DATE":
				n["__typename"], n["date"] = "ProjectV2ItemFieldDateValue", val
			default:
				n["__typename"], n["text"] = "ProjectV2ItemFieldTextValue", val
			}
			vals = append(vals, n)
		}
		nodes = append(nodes, map[string]any{"id": it.ID, "content": content, "fieldValues": map[string]any{"nodes": vals}})
	}
	writeJSON(w, map[string]any{"data": map[string]any{"repositoryOwner": map[string]any{"projectV2": map[string]any{
		"items": map[string]any{"pageInfo": map[string]any{"hasNextPage": end < len(items), "endCursor": strconv.Itoa(end)}, "nodes": nodes}}}}})
}
