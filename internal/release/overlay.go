package release

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Deployed is what an overlay says is deployed.
type Deployed struct {
	Commit  string           `json:"commit"`  // git-board/commit, "" when not adopted
	Version string           `json:"version"` // git-board/version
	Images  map[string]Image `json:"images"`  // key: the images[] entry's name
	Chart   string           `json:"chart"`   // the chart entry's version, "" when none
}

// Image is one images[] entry's tag and digest.
type Image struct {
	Tag    string `json:"tag"`
	Digest string `json:"digest"`
}

// Overlay is a kustomization.yaml kept as a yaml.Node tree, so a write
// leaves everything it does not touch as it was.
type Overlay struct{ doc yaml.Node }

const (
	commitAnnotation  = "git-board/commit"
	versionAnnotation = "git-board/version"
)

// ParseOverlay parses a kustomization.yaml.
func ParseOverlay(data []byte) (*Overlay, error) {
	o := &Overlay{}
	if err := yaml.Unmarshal(data, &o.doc); err != nil {
		return nil, fmt.Errorf("parse overlay: %w", err)
	}
	if o.doc.Kind != yaml.DocumentNode || len(o.doc.Content) != 1 || o.doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("parse overlay: not a YAML mapping")
	}
	return o, nil
}

// Bytes encodes the overlay with a 2-space indent.
func (o *Overlay) Bytes() ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(&o.doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (o *Overlay) root() *yaml.Node { return o.doc.Content[0] }

// get returns the value node under key in a mapping, or nil.
func get(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// ensure returns the value node under key, appending it (of kind) when absent.
func ensure(m *yaml.Node, key string, kind yaml.Kind) *yaml.Node {
	if v := get(m, key); v != nil {
		return v
	}
	v := &yaml.Node{Kind: kind}
	if kind == yaml.ScalarNode {
		v.Tag = "!!str"
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
	return v
}

// setStr sets the string under key, keeping an existing scalar's style.
// It reports whether anything changed.
func setStr(m *yaml.Node, key, val string, style yaml.Style) bool {
	v := get(m, key)
	if v == nil {
		v = ensure(m, key, yaml.ScalarNode)
		v.Style = style
	} else if v.Kind == yaml.ScalarNode && v.Value == val {
		return false
	}
	v.Kind, v.Tag, v.Value = yaml.ScalarNode, "!!str", val
	return true
}

func scalar(m *yaml.Node, key string) string {
	if v := get(m, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}

func (o *Overlay) chart(repo string) *yaml.Node {
	for _, c := range items(get(o.root(), "helmCharts")) {
		if scalar(c, "repo") == repo {
			return c
		}
	}
	return nil
}

func items(n *yaml.Node) []*yaml.Node {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	return n.Content
}

// Deployed reads what the overlay says is deployed; chartRepo selects the chart entry.
func (o *Overlay) Deployed(chartRepo string) Deployed {
	d := Deployed{Images: map[string]Image{}}
	for _, e := range items(get(o.root(), "images")) {
		d.Images[scalar(e, "name")] = Image{Tag: scalar(e, "newTag"), Digest: scalar(e, "digest")}
	}
	ann := get(o.root(), "commonAnnotations")
	d.Commit, d.Version = scalar(ann, commitAnnotation), scalar(ann, versionAnnotation)
	if chartRepo != "" {
		d.Chart = scalar(o.chart(chartRepo), "version")
	}
	return d
}

// Set writes d into the overlay and reports whether anything changed.
func (o *Overlay) Set(d Deployed, chartRepo string, versionValues []string) (bool, error) {
	changed := false
	imgs := items(get(o.root(), "images"))
	for name, img := range d.Images {
		var e *yaml.Node
		for _, c := range imgs {
			if scalar(c, "name") == name {
				e = c
			}
		}
		if e == nil {
			return false, fmt.Errorf("overlay has no image %s", name)
		}
		changed = setStr(e, "newTag", img.Tag, 0) || changed
		changed = setStr(e, "digest", img.Digest, 0) || changed
	}
	if chartRepo != "" {
		c := o.chart(chartRepo)
		if c == nil {
			return false, fmt.Errorf("overlay has no chart %s", chartRepo)
		}
		changed = setStr(c, "version", d.Version, 0) || changed
		for _, path := range versionValues {
			m := ensure(c, "valuesInline", yaml.MappingNode)
			parts := strings.Split(path, ".")
			for _, p := range parts[:len(parts)-1] {
				m = ensure(m, p, yaml.MappingNode)
			}
			changed = setStr(m, parts[len(parts)-1], d.Version, yaml.DoubleQuotedStyle) || changed
		}
	}
	if d.Commit != "" {
		ann := ensure(o.root(), "commonAnnotations", yaml.MappingNode)
		changed = setStr(ann, commitAnnotation, d.Commit, 0) || changed
		changed = setStr(ann, versionAnnotation, d.Version, 0) || changed
	}
	return changed, nil
}
