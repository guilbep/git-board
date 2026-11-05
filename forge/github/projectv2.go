package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/forge"
)

// A GitHub Project (v2): an owner's board of issues, whose fields (single
// select, iteration, number, date, text) hold a team's priorities. The
// Project bridge (tools/ghproject) reads and writes them here, through
// GraphQL, with the token of the host's posting account; the assignees,
// the issue's own, through REST.

// ProjectV2 is one Project, an owner's by its number.
type ProjectV2 struct {
	f      *Forge
	Owner  string
	Number int
}

// OpenProject is the Project number of owner on o's host, through o's
// account's token. o.Project is not read.
func OpenProject(ctx context.Context, o Options, owner string, number int) (*ProjectV2, error) {
	if owner == "" || strings.ContainsAny(owner, "/?#@ \r\n") || number <= 0 {
		return nil, fmt.Errorf("github: Project %q %d: an owner and a number", owner, number)
	}
	f, err := open(ctx, o)
	if err != nil {
		return nil, err
	}
	f.s.project = owner + "/projects/" + strconv.Itoa(number)
	return &ProjectV2{f: f, Owner: owner, Number: number}, nil
}

// Key is the host, the owner and the number: github.com/EPFL-ENAC/projects/40.
func (p *ProjectV2) Key() string { return p.f.Key() }

// ProjectMeta is a Project's id, title and fields.
type ProjectMeta struct {
	ID, Title string
	Fields    []ProjectV2Field
}

// ProjectV2Field is one of a Project's fields: its id, name and type
// (SINGLE_SELECT, ITERATION, NUMBER, DATE, TEXT, or one of GitHub's own:
// TITLE, ASSIGNEES, MILESTONE…), and a single select's options or an
// iteration's iterations, completed ones too.
type ProjectV2Field struct {
	ID, Name, DataType string
	Choices            []ProjectChoice
}

// ProjectChoice is an option or an iteration: its id and its name.
type ProjectChoice struct{ ID, Name string }

// ProjectItem is one row of a Project: an issue, a pull request or a draft,
// and its fields' values.
type ProjectItem struct {
	ID string
	// Type is the content's: Issue, PullRequest or DraftIssue.
	Type      string
	Number    int
	URL       string
	Title     string
	Assignees []string // the issue's logins
	Labels    []string // the issue's labels on GitHub
	Milestone string   // the issue's milestone, its title
	// Values are the item's field values by the field's name: an option's
	// name, an iteration's title, a number as Go formats it shortest, a
	// date (2026-11-05), a text.
	Values map[string]string
}

const projectMetaQuery = `query($owner: String!, $n: Int!) {
  repositoryOwner(login: $owner) { ... on ProjectV2Owner { projectV2(number: $n) { id title
    fields(first: 100) { nodes {
      ... on ProjectV2FieldCommon { id name dataType }
      ... on ProjectV2SingleSelectField { options { id name } }
      ... on ProjectV2IterationField { configuration { iterations { id title } completedIterations { id title } } }
    } } } } }
}`

type gqlError struct {
	Message string `json:"message"`
}

func gqlErr(errs []gqlError) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("github: graphql: %s", errs[0].Message)
}

// Meta reads the Project's id, title and fields. A token without the
// read:project scope gets GitHub's error.
func (p *ProjectV2) Meta(ctx context.Context) (ProjectMeta, error) {
	type named struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	var r struct {
		Data struct {
			RepositoryOwner *struct {
				ProjectV2 *struct {
					ID     string `json:"id"`
					Title  string `json:"title"`
					Fields struct {
						Nodes []struct {
							ID            string  `json:"id"`
							Name          string  `json:"name"`
							DataType      string  `json:"dataType"`
							Options       []named `json:"options"`
							Configuration struct {
								Iterations          []named `json:"iterations"`
								CompletedIterations []named `json:"completedIterations"`
							} `json:"configuration"`
						} `json:"nodes"`
					} `json:"fields"`
				} `json:"projectV2"`
			} `json:"repositoryOwner"`
		} `json:"data"`
		Errors []gqlError `json:"errors"`
	}
	in := map[string]any{"query": projectMetaQuery, "variables": map[string]any{"owner": p.Owner, "n": p.Number}}
	if err := p.f.call(ctx, http.MethodPost, p.f.s.graphql, in, &r); err != nil {
		return ProjectMeta{}, err
	}
	if err := gqlErr(r.Errors); err != nil {
		return ProjectMeta{}, err
	}
	if r.Data.RepositoryOwner == nil || r.Data.RepositoryOwner.ProjectV2 == nil {
		return ProjectMeta{}, fmt.Errorf("github: no Project %d for %s: %w", p.Number, p.Owner, forge.ErrNotFound)
	}
	pr := r.Data.RepositoryOwner.ProjectV2
	meta := ProjectMeta{ID: pr.ID, Title: pr.Title}
	for _, n := range pr.Fields.Nodes {
		if n.ID == "" {
			continue
		}
		f := ProjectV2Field{ID: n.ID, Name: n.Name, DataType: n.DataType}
		for _, o := range n.Options {
			f.Choices = append(f.Choices, ProjectChoice{o.ID, o.Name})
		}
		for _, it := range append(n.Configuration.Iterations, n.Configuration.CompletedIterations...) {
			f.Choices = append(f.Choices, ProjectChoice{it.ID, it.Title})
		}
		meta.Fields = append(meta.Fields, f)
	}
	return meta, nil
}

const projectItemsQuery = `query($owner: String!, $n: Int!, $first: Int!, $after: String) {
  repositoryOwner(login: $owner) { ... on ProjectV2Owner { projectV2(number: $n) {
    items(first: $first, after: $after) { pageInfo { hasNextPage endCursor } nodes { id
      content { __typename
        ... on Issue { number url title assignees(first: 20) { nodes { login } } labels(first: 50) { nodes { name } } milestone { title } }
        ... on PullRequest { number url title }
        ... on DraftIssue { title } }
      fieldValues(first: 50) { nodes { __typename
        ... on ProjectV2ItemFieldSingleSelectValue { name field { ... on ProjectV2FieldCommon { name } } }
        ... on ProjectV2ItemFieldIterationValue { title field { ... on ProjectV2FieldCommon { name } } }
        ... on ProjectV2ItemFieldNumberValue { number field { ... on ProjectV2FieldCommon { name } } }
        ... on ProjectV2ItemFieldDateValue { date field { ... on ProjectV2FieldCommon { name } } }
        ... on ProjectV2ItemFieldTextValue { text field { ... on ProjectV2FieldCommon { name } } }
      } } } } } } }
}`

// Items reads the Project's items, limit at most (0: every one), and says
// whether it left some out.
func (p *ProjectV2) Items(ctx context.Context, limit int) (items []ProjectItem, more bool, err error) {
	type names struct {
		Nodes []struct {
			Login string `json:"login"`
			Name  string `json:"name"`
		} `json:"nodes"`
	}
	var after *string
	for {
		first := 100
		if limit > 0 {
			first = min(first, limit-len(items))
		}
		var r struct {
			Data struct {
				RepositoryOwner *struct {
					ProjectV2 *struct {
						Items struct {
							PageInfo struct {
								HasNextPage bool   `json:"hasNextPage"`
								EndCursor   string `json:"endCursor"`
							} `json:"pageInfo"`
							Nodes []struct {
								ID      string `json:"id"`
								Content *struct {
									Typename  string `json:"__typename"`
									Number    int    `json:"number"`
									URL       string `json:"url"`
									Title     string `json:"title"`
									Assignees names  `json:"assignees"`
									Labels    names  `json:"labels"`
									Milestone *struct {
										Title string `json:"title"`
									} `json:"milestone"`
								} `json:"content"`
								FieldValues struct {
									Nodes []struct {
										Typename string   `json:"__typename"`
										Name     *string  `json:"name"`
										Title    *string  `json:"title"`
										Number   *float64 `json:"number"`
										Date     *string  `json:"date"`
										Text     *string  `json:"text"`
										Field    struct {
											Name string `json:"name"`
										} `json:"field"`
									} `json:"nodes"`
								} `json:"fieldValues"`
							} `json:"nodes"`
						} `json:"items"`
					} `json:"projectV2"`
				} `json:"repositoryOwner"`
			} `json:"data"`
			Errors []gqlError `json:"errors"`
		}
		vars := map[string]any{"owner": p.Owner, "n": p.Number, "first": first, "after": after}
		if err := p.f.call(ctx, http.MethodPost, p.f.s.graphql, map[string]any{"query": projectItemsQuery, "variables": vars}, &r); err != nil {
			return nil, false, err
		}
		if err := gqlErr(r.Errors); err != nil {
			return nil, false, err
		}
		if r.Data.RepositoryOwner == nil || r.Data.RepositoryOwner.ProjectV2 == nil {
			return nil, false, fmt.Errorf("github: no Project %d for %s: %w", p.Number, p.Owner, forge.ErrNotFound)
		}
		page := r.Data.RepositoryOwner.ProjectV2.Items
		for _, n := range page.Nodes {
			it := ProjectItem{ID: n.ID, Values: map[string]string{}}
			if c := n.Content; c != nil {
				it.Type, it.Number, it.URL, it.Title = c.Typename, c.Number, c.URL, c.Title
				for _, a := range c.Assignees.Nodes {
					it.Assignees = append(it.Assignees, a.Login)
				}
				for _, l := range c.Labels.Nodes {
					it.Labels = append(it.Labels, l.Name)
				}
				if c.Milestone != nil {
					it.Milestone = c.Milestone.Title
				}
			}
			for _, v := range n.FieldValues.Nodes {
				var s *string
				switch {
				case v.Name != nil:
					s = v.Name
				case v.Title != nil:
					s = v.Title
				case v.Date != nil:
					s = v.Date
				case v.Text != nil:
					s = v.Text
				case v.Number != nil:
					x := strconv.FormatFloat(*v.Number, 'f', -1, 64)
					s = &x
				}
				if s != nil && v.Field.Name != "" {
					it.Values[v.Field.Name] = *s
				}
			}
			items = append(items, it)
		}
		if !page.PageInfo.HasNextPage {
			return items, false, nil
		}
		if limit > 0 && len(items) >= limit {
			return items, true, nil
		}
		cursor := page.PageInfo.EndCursor
		after = &cursor
	}
}

// The kinds of a ProjectValue.
const (
	ValueOption    = "option"
	ValueIteration = "iteration"
	ValueNumber    = "number"
	ValueDate      = "date"
	ValueText      = "text"
	ValueClear     = "clear"
)

// ProjectValue is one field's value written to one item: an option's or an
// iteration's id, a number, a date, a text; or the field cleared.
type ProjectValue struct {
	Item, Field string // their ids
	Kind        string
	Arg         string
}

const (
	setValueMutation = `mutation($p: ID!, $i: ID!, $f: ID!, $v: ProjectV2FieldValue!) {
  updateProjectV2ItemFieldValue(input: {projectId: $p, itemId: $i, fieldId: $f, value: $v}) { projectV2Item { id } } }`
	clearValueMutation = `mutation($p: ID!, $i: ID!, $f: ID!) {
  clearProjectV2ItemFieldValue(input: {projectId: $p, itemId: $i, fieldId: $f}) { projectV2Item { id } } }`
)

// Set writes v on the Project, whose id is projectID (Meta's). Writes are
// paced as the issues' are.
func (p *ProjectV2) Set(ctx context.Context, projectID string, v ProjectValue) error {
	vars := map[string]any{"p": projectID, "i": v.Item, "f": v.Field}
	query := setValueMutation
	switch v.Kind {
	case ValueOption:
		vars["v"] = map[string]any{"singleSelectOptionId": v.Arg}
	case ValueIteration:
		vars["v"] = map[string]any{"iterationId": v.Arg}
	case ValueNumber:
		x, err := strconv.ParseFloat(v.Arg, 64)
		if err != nil {
			return fmt.Errorf("github: %q isn't a number", v.Arg)
		}
		vars["v"] = map[string]any{"number": x}
	case ValueDate:
		vars["v"] = map[string]any{"date": v.Arg}
	case ValueText:
		vars["v"] = map[string]any{"text": v.Arg}
	case ValueClear:
		query = clearValueMutation
	default:
		return fmt.Errorf("github: a Project value of kind %q", v.Kind)
	}
	if err := p.f.s.pace(ctx, p.Key()); err != nil {
		return err
	}
	var r struct {
		Errors []gqlError `json:"errors"`
	}
	if err := p.f.call(ctx, http.MethodPost, p.f.s.graphql, map[string]any{"query": query, "variables": vars}, &r); err != nil {
		return err
	}
	return gqlErr(r.Errors)
}

// repoURL is the REST URL of repo, owner/name.
func (p *ProjectV2) repoURL(repo string) (string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf("github: repository %q isn't owner/name", repo)
	}
	return p.f.s.api + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name), nil
}

// Assignable lists who can be assigned on repo, owner/name: GitHub drops
// anyone else silently.
func (p *ProjectV2) Assignable(ctx context.Context, repo string) ([]string, error) {
	u, err := p.repoURL(repo)
	if err != nil {
		return nil, err
	}
	us, err := pages(ctx, p.f, u+"/assignees", func(u user) int64 { return u.ID })
	if err != nil {
		return nil, err
	}
	var out []string
	for _, x := range us {
		out = append(out, x.Login)
	}
	return out, nil
}

// Assign adds and removes assignees of issue number of repo.
func (p *ProjectV2) Assign(ctx context.Context, repo string, number int, add, remove []string) error {
	u, err := p.repoURL(repo)
	if err != nil {
		return err
	}
	u += "/issues/" + strconv.Itoa(number) + "/assignees"
	if len(add) > 0 {
		if err := p.f.call(ctx, http.MethodPost, u, map[string]any{"assignees": add}, nil); err != nil {
			return err
		}
	}
	if len(remove) > 0 {
		return p.f.call(ctx, http.MethodDelete, u, map[string]any{"assignees": remove}, nil)
	}
	return nil
}
