package app

import (
	"context"
	"encoding/json"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/kernel/pack"
	"github.com/guilbep/git-board/next/store"
)

// forges are each issue's forge issues, from the sync entities' links
// (data-model.md, Sync: the first link of an issue, and of a forge issue,
// wins): what numbering reads, a GitHub number below the counter's start
// being the issue's own. The sync (M4) reads the rest of the records.
// ponytail: links alone, for the numbers; ls and show take theirs with M4
func forges(ctx context.Context, ds store.Datastore) (map[string][]issues.Forge, error) {
	heads, err := ds.Heads(ctx, "sync")
	if err != nil || len(heads) == 0 {
		return nil, err
	}
	refs := make([]store.Ref, len(heads))
	for i, h := range heads {
		refs[i] = store.Ref{Kind: h.Kind, ID: h.ID, Version: h.Version}
	}
	var mu sync.Mutex
	byEntity := map[string][]pack.Entry{}
	err = ds.Load(ctx, refs, func(r store.Ref, packs [][]byte, err error) error {
		if err != nil {
			return nil // gone meanwhile: no links
		}
		ops, _, err := pack.Order(packs)
		if err != nil {
			return nil // broken: no links, as the sync reads none
		}
		mu.Lock()
		byEntity[r.ID] = ops
		mu.Unlock()
		return nil
	})
	if err != nil {
		return nil, err
	}
	res := map[string][]issues.Forge{}
	for _, h := range heads {
		linked, forgeIDs := map[string]bool{}, map[string]bool{}
		for _, op := range byEntity[h.ID] {
			var l struct {
				Issue   string `json:"issue"`
				ForgeID string `json:"forge_id"`
				Number  string `json:"number"`
				URL     string `json:"url"`
			}
			if op.Kind != "link" || op.Author == "" || json.Unmarshal(op.Data, &l) != nil || linked[l.Issue] || forgeIDs[l.ForgeID] {
				continue
			}
			linked[l.Issue], forgeIDs[l.ForgeID] = true, true
			n := l.Number
			if n == "" {
				n = lastNumber(l.URL)
			}
			res[l.Issue] = append(res[l.Issue], issues.Forge{Name: forgeName(l.URL), Number: n})
		}
	}
	return res, nil
}

func forgeName(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	switch host := parsed.Hostname(); {
	case host == "github.com":
		return "GitHub"
	case strings.HasPrefix(host, "gitlab."):
		return "GitLab"
	default:
		return host
	}
}

// lastNumber is the issue number ending a forge URL, if it ends with one.
func lastNumber(u string) string {
	last := path.Base(u)
	if last == "" || strings.Trim(last, "0123456789") != "" {
		return ""
	}
	return last
}
