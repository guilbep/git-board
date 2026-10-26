package store

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/core/syncer"
)

// The switch from the git-bug fork (core.md's M6): the core pulls the
// repository's issues from its forges, the fork's mirrors joined to their
// originals (sync.md's "Switching from the fork"), the project's numbers
// keyed again by the core's ids, and the fork's release records carried
// into the release kind. It never deletes refs/bugs, and never pushes.

// Fork is what the switch reads of the fork's data: its issues and its
// people, through its binary.
type Fork struct {
	Bugs  []ForkBug
	Users []ForkUser
	// Texts are an issue's description and comments, read when needed.
	// git-bug's JSON gives a comment's text, never its metadata.
	Texts func(id string) ([]string, error)
}

// ForkBug is an issue as the fork lists it.
type ForkBug struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Metadata map[string]string `json:"metadata"`
}

// ForkUser is one of the fork's identities.
type ForkUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Login string `json:"login"`
}

// ReadFork reads the fork's data with its binary, gitBug, in the
// repository at dir: read-only commands, though they build git-bug's cache
// there, so the switch runs them on its copy.
func ReadFork(ctx context.Context, gitBug, dir string) (Fork, error) {
	run := func(out any, args ...string) error {
		cmd := exec.CommandContext(ctx, gitBug, args...)
		cmd.Dir = dir
		data, err := cmd.Output()
		if ee, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%s %s: %w: %s", gitBug, strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		if err != nil {
			return fmt.Errorf("%s: %w", gitBug, err)
		}
		return json.Unmarshal(data, out)
	}
	var f Fork
	if err := run(&f.Bugs, "bug", "--format", "json"); err != nil {
		return f, err
	}
	if err := run(&f.Users, "user", "--format", "json"); err != nil {
		return f, err
	}
	f.Texts = func(id string) ([]string, error) {
		var d struct {
			Comments []struct {
				Message string `json:"message"`
			} `json:"comments"`
		}
		err := run(&d, "bug", "show", "--format", "json", id)
		var out []string
		for _, c := range d.Comments {
			out = append(out, c.Message)
		}
		return out, err
	}
	return f, nil
}

// SwitchRun is a switch: its forges, read only, and how it goes.
type SwitchRun struct {
	Forges         []NamedForge
	PublishPrivate bool
	GitBug         string // the fork's binary: git-bug when empty
	Fork           *Fork  // the fork's data, instead of reading it with GitBug
	// Apply writes refs/board/* here, unless the report has a blocker and
	// Force isn't set. Otherwise the switch is a dry run.
	Apply, Force bool
	// AllowCreate accepts what the next sync would create on the forges:
	// that blocker alone.
	AllowCreate bool
}

// SwitchReport is what a switch found, and did.
type SwitchReport struct {
	Forges    map[string]int // forge name -> the issues linked there
	Pulled    syncer.Report
	Joined    int // mirrors the fork posted, joined to their original's issue
	OnSeveral int // issues on two forges or more
	Numbers   int // numbers keyed again
	// NoIssue are the numbers left without an issue, as "GB-7 (fork's id)":
	// kept in the counter, so that none is handed out again.
	NoIssue []string
	// Lost are the fork's issues the switch doesn't bring: on no forge, or
	// only on forges it doesn't pull.
	Lost []string
	// Split are the fork's issues the pull made two issues or more.
	Split []string
	// LostComments are the fork's comments no forge has: its review
	// comments, which never went to one, among them.
	LostComments []string
	Releases     int      // the fork's release records
	Carried      int      // of them, carried into the release kind
	ReleaseNotes []string // what about them didn't carry over
	// WouldCreate is what the next sync would create on a forge: a mirror
	// not joined, or what the fork never sent. Writes are its other writes.
	// Each names its forge issue by forge and number, and its title.
	WouldCreate []string
	Writes      []string
	Blockers    []string
	Applied     bool
}

// ErrNoForge: a switch pulls the issues from the forges, and has none.
var ErrNoForge = errors.New("no forge to pull from: add a git remote on GitHub or GitLab, or a [forges.<name>] table with sync = true to .git-board.toml")

// Switch runs a switch on a copy of the repository, and applies it when
// asked, by fetching the copy's refs/board/* here. No write reaches a
// forge: every one is answered in the process.
func (b *Backend) Switch(ctx context.Context, run SwitchRun) (SwitchReport, error) {
	rep := SwitchReport{Forges: map[string]int{}}
	block := func(format string, a ...any) { rep.Blockers = append(rep.Blockers, fmt.Sprintf(format, a...)) }
	if len(run.Forges) == 0 { // a switch would carry no issue: not a blocker --force lifts
		return rep, ErrNoForge
	}
	if _, err := os.Stat(filepath.Join(b.s.CommonDir(), "board", "sync.lock")); err == nil {
		block("a forge sync runs, or left board/sync.lock in the git directory")
	}
	// the numbers are keyed again on origin's: this clone's may be behind,
	// or hold an earlier --apply's that never reached origin
	theirs := gitstore.ZeroHash
	if b.checkRemote(NumbersRemote) == nil {
		var err error
		if theirs, err = b.remoteSeq(NumbersRemote); err != nil {
			return rep, err
		}
	}
	scratch, done, err := b.scratch(ctx)
	if err != nil {
		return rep, err
	}
	defer done()
	if theirs != gitstore.ZeroHash {
		ours, err := scratch.Ref(seqRef)
		if err == nil && ours != theirs {
			err = scratch.UpdateRef(ctx, seqRef, theirs, ours)
		}
		if err != nil {
			return rep, err
		}
	}

	fork := Fork{Texts: func(string) ([]string, error) { return nil, nil }}
	if bugs, err := b.s.Refs("refs/bugs/"); err != nil {
		return rep, err
	} else if run.Fork != nil {
		fork = *run.Fork
	} else if len(bugs) > 0 {
		f, err := ReadFork(ctx, cmp.Or(run.GitBug, "git-bug"), scratch.CommonDir())
		if err != nil {
			block("the fork's issues can't be read, so neither its links nor what is lost: %v", err)
		} else {
			fork = f
		}
	}

	paths := map[string]string{} // forge key -> the path of its project, named by id
	for _, f := range run.Forges {
		g, ok := f.Forge.(interface {
			Path(context.Context) (string, error)
		})
		if _, err := strconv.Atoi(strings.TrimPrefix(f.Key(), f.Server()+"/")); ok && err == nil {
			if p, err := g.Path(ctx); err == nil && p != "" {
				paths[f.Key()] = p
			}
		}
	}
	places := map[string][]string{} // fork id -> its places on the run's forges
	var groups [][]string
	for _, bug := range fork.Bugs {
		ps, linked, unplaced := bug.places(run.Forges, paths)
		if unplaced != "" {
			rep.Lost = append(rep.Lost, short(bug.ID)+" "+bug.Title+": unplaced, as "+unplaced+" may be on a forge of [forges] named by id")
			continue
		}
		places[bug.ID] = ps
		switch {
		case !linked:
			rep.Lost = append(rep.Lost, short(bug.ID)+" "+bug.Title+": on no forge")
		case len(ps) == 0:
			rep.Lost = append(rep.Lost, short(bug.ID)+" "+bug.Title+": on no forge of [forges]")
		case len(ps) > 1:
			groups = append(groups, ps)
		}
	}

	var fs []forge.Forge
	ids := new(int)
	for _, f := range run.Forges {
		fs = append(fs, &counted{f: f.Forge, name: f.Name, dry: true, ids: ids, see: func(SyncEvent) {}})
	}
	opt := syncer.Options{PublishPrivate: run.PublishPrivate, Local: isLocal, Mirrors: groups,
		Did: func(_, what, _ string, _ int) {
			if what == "joined" {
				rep.Joined++
			}
		}}
	if rep.Pulled, err = syncer.Pull(ctx, scratch, opt, fs...); err != nil {
		return rep, fmt.Errorf("pulling from the forges: %w", err)
	}

	records, err := syncer.Records(scratch)
	if err != nil {
		return rep, err
	}
	names := map[string]string{} // sync entity id -> forge name
	for _, f := range run.Forges {
		names[syncer.StateID(f.Key())] = f.Name
	}
	at := map[string]string{} // place, in lower case -> issue
	for id, rs := range records {
		for _, r := range rs {
			if names[r.Forge] != "" {
				rep.Forges[names[r.Forge]]++
			}
			for _, f := range run.Forges {
				if syncer.StateID(f.Key()) == r.Forge {
					at[strings.ToLower(f.Key()+"#"+r.Number)] = id
				}
			}
		}
		if len(rs) > 1 {
			rep.OnSeveral++
		}
	}
	issueOf := map[string]string{}     // fork id -> issue
	unlisted := map[[2]string]string{} // issue, forge name -> the place git-bug links it to there, not listed
	for _, bug := range fork.Bugs {
		got := map[string]bool{}
		for _, p := range places[bug.ID] {
			if id := at[strings.ToLower(p)]; id != "" {
				got[id] = true
				issueOf[bug.ID] = cmp.Or(issueOf[bug.ID], id)
			}
		}
		for _, p := range places[bug.ID] {
			key, n, _ := strings.Cut(p, "#")
			if id := issueOf[bug.ID]; id != "" && at[strings.ToLower(p)] == "" {
				unlisted[[2]string{id, names[syncer.StateID(key)]}] = names[syncer.StateID(key)] + "#" + n
			}
		}
		switch {
		case len(got) > 1:
			rep.Split = append(rep.Split, short(bug.ID)+" "+bug.Title)
		case len(got) == 0 && len(places[bug.ID]) > 0:
			rep.Lost = append(rep.Lost, short(bug.ID)+" "+bug.Title+": not pulled from "+strings.Join(places[bug.ID], ", "))
		}
	}

	if err := rekey(scratch, issueOf, records, &rep); err != nil {
		block("the numbers can't be keyed again: %v", err)
	}
	if err := lostComments(scratch, fork, issueOf, &rep); err != nil {
		return rep, err
	}
	if err := carryReleases(ctx, scratch, fork.Users, &rep); err != nil {
		return rep, err
	}
	if err := wouldPush(ctx, scratch, run, opt, records, unlisted, &rep); err != nil {
		return rep, err
	}

	if len(rep.Lost) > 0 {
		block("%d of the fork's issues aren't on the forges pulled: run the fork's sync first, or lose them", len(rep.Lost))
	}
	if len(rep.Split) > 0 {
		block("%d of the fork's issues were pulled as two issues or more", len(rep.Split))
	}
	if len(rep.WouldCreate) > 0 && !run.AllowCreate {
		block("the next sync would create %d issues or comments on the forges (--allow-create accepts them)", len(rep.WouldCreate))
	}
	if !run.Apply || len(rep.Blockers) > 0 && !run.Force {
		return rep, nil
	}
	if theirs != gitstore.ZeroHash {
		if err := b.setSeq(theirs); err != nil { // the copy's numbers follow on from origin's
			return rep, err
		}
	}
	if _, err := b.run(nil, "fetch", "--no-write-fetch-head", "--quiet", scratch.CommonDir(), "refs/board/*:refs/board/*"); err != nil {
		return rep, fmt.Errorf("applying the switch: %w", err)
	}
	if err := b.markSwitched(); err != nil {
		return rep, fmt.Errorf("applying the switch: %w", err)
	}
	rep.Applied = true
	return rep, nil
}

func short(id string) string { return id[:min(len(id), 7)] }

// places are where a fork issue is on the run's forges, as "<key>#<n>"
// with their keys, from its sync metadata and its forge URLs; paths are
// the projects' paths of the forges named by id. linked says it has a link
// at all; unplaced is a URL that may be on such a forge, whose path isn't
// known.
func (bug ForkBug) places(forges []NamedForge, paths map[string]string) (out []string, linked bool, unplaced string) {
	add := func(p string) {
		if p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	synced := map[string]bool{} // forge key, in lower case -> the bug has sync:<key>: metadata
	for k := range bug.Metadata {
		if key, ok := strings.CutPrefix(k, "sync:"); ok {
			if i := strings.LastIndex(key, ":"); i > 0 {
				synced[strings.ToLower(key[:i])] = true
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(bug.Metadata)) {
		v := bug.Metadata[k]
		if key, ok := strings.CutPrefix(k, "sync:"); ok && strings.HasSuffix(key, ":number") {
			linked = true
			key = strings.TrimSuffix(key, ":number")
			for _, f := range forges {
				if strings.EqualFold(f.Key(), key) {
					add(f.Key() + "#" + v)
				}
			}
		} else if strings.HasSuffix(k, "-url") || strings.HasPrefix(k, "sync:") && strings.HasSuffix(k, ":url") {
			if v != "" && k != "gitlab-base-url" {
				linked = true
				p, unknown := placeOf(v, forges, paths, synced)
				add(p)
				if unknown {
					unplaced = v
				}
			}
		}
	}
	return out, linked, unplaced
}

// placeOf is the place of a forge issue's URL on the run's forges: the
// forge whose key the URL's project is; else, for a GitLab URL, which
// names its project by path, a forge of its host named by id whose path
// it is, when the bug has no sync:<key>: metadata for it. unknown says
// such a forge's path isn't known.
func placeOf(u string, forges []NamedForge, paths map[string]string, synced map[string]bool) (place string, unknown bool) {
	p, err := url.Parse(u)
	n := lastNumber(u)
	if err != nil || n == "" {
		return "", false
	}
	project, _, gitlab := strings.Cut(strings.TrimPrefix(p.Path, "/"), "/-/")
	project, _, _ = strings.Cut(project, "/issues/")
	for _, f := range forges {
		if strings.EqualFold(f.Key(), p.Host+"/"+project) {
			return f.Key() + "#" + n, false
		}
	}
	for _, f := range forges {
		_, err := strconv.Atoi(strings.TrimPrefix(f.Key(), f.Server()+"/"))
		if !gitlab || f.Server() != p.Host || err != nil || synced[strings.ToLower(f.Key())] {
			continue
		}
		path, ok := paths[f.Key()]
		if !ok {
			unknown = true
		} else if strings.EqualFold(path, project) {
			return f.Key() + "#" + n, false
		}
	}
	return "", unknown
}

// rekey keys the project's numbers again by the core's issue ids, in a
// commit on refs/board/seq's head. A number whose issue the switch didn't
// bring keeps its line, so that it is never handed out again; so does one
// keyed by an issue already.
func rekey(s *gitstore.Store, issueOf map[string]string, issues map[string][]syncer.Record, rep *SwitchReport) error {
	c, ok, err := readCounter(s)
	if err != nil || !ok {
		return err
	}
	head, err := s.Ref(seqRef)
	if err != nil {
		return err
	}
	next := Counter{Prefix: c.Prefix, Start: c.Start, ByID: map[string]int{}}
	for _, old := range slices.SortedFunc(maps.Keys(c.ByID), func(x, y string) int { return cmp.Compare(c.ByID[x], c.ByID[y]) }) {
		n := c.ByID[old]
		id := issueOf[old]
		if _, ok := issues[old]; ok && id == "" {
			id = old
		}
		if _, taken := next.ByID[id]; id == "" || taken {
			rep.NoIssue = append(rep.NoIssue, fmt.Sprintf("%s (%s)", c.Format(n), short(old)))
			next.ByID[old] = n
			continue
		}
		next.ByID[id] = n
		if id != old {
			rep.Numbers++
		}
	}
	if rep.Numbers == 0 {
		return nil
	}
	_, err = commitCounter(s, next, head)
	return err
}

// lostComments lists the fork's comments the pulled issue has no copy of,
// each, though a comment only the forge has levels the counts.
// ponytail: one git-bug bug show per issue pulled, once per switch
func lostComments(s *gitstore.Store, fork Fork, issueOf map[string]string, rep *SwitchReport) error {
	for _, bug := range fork.Bugs {
		id := issueOf[bug.ID]
		if id == "" {
			continue // lost whole, and listed so
		}
		is, err := issue.Load(s, id)
		if err != nil {
			return err
		}
		texts, err := fork.Texts(bug.ID)
		if err != nil {
			return err
		}
		for _, t := range texts[min(1, len(texts)):] {
			if !slices.ContainsFunc(is.Comments, func(c issue.Comment) bool { return syncer.SameText(t, c.Body) }) {
				line, _, _ := strings.Cut(strings.TrimSpace(t), "\n")
				rep.LostComments = append(rep.LostComments, short(bug.ID)+": "+line[:min(len(line), 60)])
			}
		}
	}
	return nil
}

// wouldPush runs the next sync's push on a copy of the switched copy, its
// writes answered in the process, and lists them. records are the copy's;
// unlisted, by issue and forge, a place git-bug links an issue to there
// that the forge didn't list.
func wouldPush(ctx context.Context, from *gitstore.Store, run SwitchRun, opt syncer.Options,
	records map[string][]syncer.Record, unlisted map[[2]string]string, rep *SwitchReport) error {
	s, done, err := scratchOf(ctx, from.CommonDir(), from.CommonDir())
	if err != nil {
		return err
	}
	defer done()
	names := map[string]string{} // forge key, and sync entity id -> forge name
	for _, f := range run.Forges {
		names[f.Key()], names[syncer.StateID(f.Key())] = f.Name, f.Name
	}
	at := map[string]string{} // forge name and its issue's id -> issue
	links := func(id, but string) []string {
		var out []string
		for _, r := range records[id] {
			if names[r.Forge] != but {
				out = append(out, names[r.Forge]+"#"+r.Number)
			}
		}
		return out
	}
	for id, rs := range records {
		for _, r := range rs {
			at[names[r.Forge]+" "+r.ForgeID] = id
		}
	}
	title := func(id string) string {
		is, err := issue.Load(from, id)
		if err != nil {
			return ""
		}
		t := []rune(is.Title)
		if len(t) > 40 {
			return fmt.Sprintf(" %q", string(t[:40])+"…")
		}
		return fmt.Sprintf(" %q", is.Title)
	}
	// on names a forge issue by forge and number, else, for one the push
	// creates, by where its issue already is
	created := map[string]string{} // forge name and its made-up id -> issue
	var made [2]string             // the forge name and made-up id of the last issue created
	on := func(name, forgeID string) string {
		if id := created[name+" "+forgeID]; id != "" {
			return "the issue it creates for " + strings.Join(links(id, name), ", ") + title(id)
		}
		id := at[name+" "+forgeID]
		for _, r := range records[id] {
			if names[r.Forge] == name {
				return name + "#" + r.Number + title(id)
			}
		}
		return name + " issue " + forgeID
	}
	var fs []forge.Forge
	ids := new(int)
	for _, f := range run.Forges {
		name := f.Name
		fs = append(fs, &counted{f: f.Forge, name: f.Name, dry: true, ids: ids, made: func(id string) { made = [2]string{name, id} }, see: func(ev SyncEvent) {
			switch ev.What {
			case "comment":
				rep.WouldCreate = append(rep.WouldCreate, ev.Forge+": a comment on "+on(ev.Forge, ev.Issue))
			default:
				rep.Writes = append(rep.Writes, ev.Forge+": "+ev.Line+" on "+on(ev.Forge, ev.Issue))
			}
		}})
	}
	opt.Mirrors, opt.Did = nil, func(key, what, id string, _ int) {
		if what != "posted" {
			return
		}
		name := names[key]
		created[made[0]+" "+made[1]] = id
		line := name + ": issue for " + strings.Join(links(id, name), ", ") + title(id)
		if why := unlisted[[2]string{id, name}]; why != "" {
			line += ": git-bug links it to " + why + ", which the forge didn't list"
		}
		rep.WouldCreate = append(rep.WouldCreate, line)
	}
	_, err = syncer.Push(ctx, s, opt, fs...)
	return err
}

// The fork's release records, git-board's own in git-bug's format: a chain
// of commits under refs/releases/<id>, each tree an ops file, {"author":
// {"id"}, "ops": [{"type", "timestamp", fields}]}, and an edit-clock-<n>.
var forkReleaseKinds = map[int]string{1: "create", 2: "set-notes", 3: "comment", 4: "yank"}

type forkOp struct {
	author, kind string
	time         int64
	data         releaseData
}

// readForkRelease is a record's operations, its packs by edit clock.
func readForkRelease(s *gitstore.Store, head gitstore.Hash) ([]forkOp, error) {
	type pack struct {
		clock int
		hash  string
		ops   []forkOp
	}
	var packs []pack
	seen := map[gitstore.Hash]bool{}
	for todo := []gitstore.Hash{head}; len(todo) > 0; {
		h := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if seen[h] {
			continue
		}
		seen[h] = true
		c, err := s.ReadCommit(h)
		if err != nil {
			return nil, err
		}
		todo = append(todo, c.Parents...)
		entries, err := s.ReadTree(c.Tree)
		if err != nil {
			return nil, err
		}
		pk := pack{hash: h.String()}
		var raw []byte
		for _, e := range entries {
			if n, ok := strings.CutPrefix(e.Name, "edit-clock-"); ok {
				pk.clock, _ = strconv.Atoi(n)
			}
			if e.Name == "ops" {
				if raw, err = s.ReadBlob(e.Blob); err != nil {
					return nil, err
				}
			}
		}
		var blob struct {
			Author struct {
				ID string `json:"id"`
			} `json:"author"`
			Ops []json.RawMessage `json:"ops"`
		}
		if err := json.Unmarshal(raw, &blob); err != nil {
			return nil, fmt.Errorf("commit %s: %w", h, err)
		}
		for _, r := range blob.Ops {
			var o struct {
				Type      int   `json:"type"`
				Timestamp int64 `json:"timestamp"`
			}
			var d releaseData
			if err := errors.Join(json.Unmarshal(r, &o), json.Unmarshal(r, &d)); err != nil {
				return nil, fmt.Errorf("commit %s: %w", h, err)
			}
			kind := forkReleaseKinds[o.Type]
			if kind == "" {
				return nil, fmt.Errorf("commit %s: an operation of type %d", h, o.Type)
			}
			pk.ops = append(pk.ops, forkOp{blob.Author.ID, kind, o.Timestamp, d})
		}
		packs = append(packs, pk)
	}
	slices.SortFunc(packs, func(x, y pack) int { return cmp.Or(cmp.Compare(x.clock, y.clock), strings.Compare(x.hash, y.hash)) })
	var out []forkOp
	for _, p := range packs {
		out = append(out, p.ops...)
	}
	if len(out) == 0 || out[0].kind != "create" || out[0].data.Commit == "" {
		return nil, errors.New("it doesn't start with a create naming its commit")
	}
	return out, nil
}

// carryReleases copies each fork record into the release kind, unless its
// commit has a record there already, its authors as persons: by login,
// else by name, else a new one.
func carryReleases(ctx context.Context, s *gitstore.Store, users []ForkUser, rep *SwitchReport) error {
	heads, err := s.Refs("refs/releases/")
	if err != nil {
		return err
	}
	rep.Releases = len(heads)
	have := map[string]bool{}
	ids, err := oplog.List(s, "release")
	if err != nil {
		return err
	}
	for _, id := range ids {
		l, err := oplog.Load(s, "release", id)
		if err != nil {
			return err
		}
		var d releaseData
		if len(l.Entries) > 0 && json.Unmarshal(l.Entries[0].Data, &d) == nil {
			have[d.Commit] = true
		}
	}
	persons := map[string]string{} // fork identity -> person
	person := func(forkID string) (string, error) {
		if p, ok := persons[forkID]; ok {
			return p, nil
		}
		u := ForkUser{ID: forkID, Name: short(forkID)}
		if i := slices.IndexFunc(users, func(u ForkUser) bool { return u.ID == forkID }); i >= 0 {
			u = users[i]
		}
		all, err := people.List(s)
		if err != nil {
			return "", err
		}
		var byName string
		for _, id := range all {
			pe, err := people.Load(s, id)
			if err != nil {
				return "", err
			}
			if u.Login != "" && slices.ContainsFunc(pe.Accounts, func(a people.Account) bool { return strings.EqualFold(a.Login, u.Login) }) {
				persons[forkID] = pe.ID
				return pe.ID, nil
			}
			if pe.Name == u.Name && byName == "" {
				byName = pe.ID
			}
		}
		if byName == "" {
			pe, err := people.Create(ctx, s, people.NewCreate("", u.Name, ""))
			if err != nil {
				return "", err
			}
			byName = pe.ID
		}
		persons[forkID] = byName
		return byName, nil
	}
	for _, ref := range slices.Sorted(maps.Keys(heads)) {
		name := "release " + short(strings.TrimPrefix(ref, "refs/releases/"))
		ops, err := readForkRelease(s, heads[ref])
		if err != nil {
			rep.ReleaseNotes = append(rep.ReleaseNotes, name+": "+err.Error())
			continue
		}
		if have[ops[0].data.Commit] {
			continue
		}
		var out []oplog.Op
		for _, o := range ops {
			who, err := person(o.author)
			if err != nil {
				return err
			}
			raw, _ := json.Marshal(o.data)
			out = append(out, oplog.Op{Kind: o.kind, Author: who, Time: o.time, Data: raw})
		}
		if _, err := oplog.Create(ctx, s, "release", "", out); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		have[ops[0].data.Commit] = true
		rep.Carried++
	}
	return nil
}
