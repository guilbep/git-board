package issuesync

import "strings"

// What git-board's comment keys mean on a forge (sync.md, Markers): the
// keys a comment's marker carries, what a forge edit restores, and a
// reply's root. As v1's internal/store/body.go.
// ponytail: v1's Quote (the line over a thread comment) and Published
// (the note after a new version of the body) read the body's sections,
// which come with the cycle (M6): v2 posts neither yet.

// carried are the keys a comment's marker carries, by their short names,
// in that order (docs/design/time-flow/protocol.md).
var carried = [][2]string{
	{"step", "cycle:step"}, {"anchor", "thread:anchor"}, {"reply-to", "thread:reply-to"}, {"kind", "thread:kind"},
	{"resolved", "thread:resolved"}, {"applied", "thread:applied"}, {"verdict", "spec:verdict"}, {"on", "spec:on"},
}

// carriable: a marker's value has no space and no ">".
func carriable(v string) bool { return v != "" && !strings.ContainsAny(v, " \t\n>") }

// Carry is what a comment's marker carries: its keys, "k=v", by their
// short names (Options.Carry).
func Carry(meta map[string]string) string {
	var kv []string
	for _, k := range carried {
		if v := meta[k[1]]; carriable(v) {
			kv = append(kv, k[0]+"="+v)
		}
	}
	return strings.Join(kv, " ")
}

// Restore is the metadata a forge edit of a comment sync posted sets,
// from the keys its marker carries; with the marker deleted, the
// thread's and the spec's keys cleared (Options.Restore).
func Restore(keys string, marked bool, meta map[string]string) map[string]string {
	out := map[string]string{}
	if !marked {
		for k, v := range meta {
			if v != "" && (strings.HasPrefix(k, "thread:") || strings.HasPrefix(k, "spec:")) {
				out[k] = ""
			}
		}
		return out
	}
	got := map[string]string{}
	for _, kv := range strings.Fields(keys) {
		k, v, _ := strings.Cut(kv, "=")
		got[k] = v
	}
	for _, k := range carried {
		if v := meta[k[1]]; got[k[0]] != v && (v == "" || carriable(v)) {
			out[k[1]] = got[k[0]]
		}
	}
	return out
}

// ReplyTo is the root a reply is posted under (Options.ReplyTo).
func ReplyTo(meta map[string]string) string { return meta["thread:reply-to"] }
