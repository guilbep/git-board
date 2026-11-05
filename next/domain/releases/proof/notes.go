package proof

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Ref is a writer's notes ref, the only one its clone writes.
func Ref(writer string) string { return "refs/notes/ci/" + writer }

var notWriter = regexp.MustCompile(`[^a-z0-9-]+`)

// WriterName is this clone's writer: git config git-board.writer, set on
// the first call to "<user.name>-<host>-<4 hex>", so two clones of one
// person never share a ref. Its worktrees share the clone's config, and so
// the writer.
func WriterName(ctx context.Context, dir string) (string, error) {
	if out, _ := git(ctx, dir, nil, "config", "--get", "git-board.writer"); strings.TrimSpace(string(out)) != "" {
		return strings.TrimSpace(string(out)), nil
	}
	user, _ := git(ctx, dir, nil, "config", "--get", "user.name")
	host, _ := os.Hostname()
	host, _, _ = strings.Cut(host, ".")
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	name := strings.Trim(notWriter.ReplaceAllString(strings.ToLower(strings.TrimSpace(string(user))+"-"+host), "-"), "-")
	name = cmp.Or(name, "writer") + "-" + hex.EncodeToString(b)
	if _, err := git(ctx, dir, nil, "config", "git-board.writer", name); err != nil {
		return "", err
	}
	return name, nil
}

// Append adds a line to the note at addr in the writer's ref, by
// compare-and-swap (ADR 0001, section 8): the ref copied to a private ref,
// the line appended there, then `git update-ref <ref> <new> <old>`; a
// write that lost the race starts again. `git notes append` alone loses
// lines under parallel writers. It retries until ctx ends.
// ponytail: a crash leaves a refs/notes/ci-tmp/ ref behind; harmless, a
// prune when one shows up.
func Append(ctx context.Context, dir, writer, addr, line string) error {
	ref := Ref(writer)
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("appending to %s: %w", ref, err)
		}
		out, _ := git(ctx, dir, nil, "rev-parse", "--verify", "--quiet", ref)
		old := strings.TrimSpace(string(out))
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		tmp := "refs/notes/ci-tmp/" + writer + "-" + hex.EncodeToString(b)
		if old != "" {
			if _, err := git(ctx, dir, nil, append(private, "update-ref", tmp, old)...); err != nil {
				return err
			}
		}
		if _, err := git(ctx, dir, nil, append(private, "notes", "--ref="+tmp, "append", "--no-separator", "-m", line, addr)...); err != nil {
			drop(dir, tmp)
			return err
		}
		out, err := git(ctx, dir, nil, "rev-parse", tmp)
		if err == nil {
			// old "" is "must not exist yet"
			_, err = git(ctx, dir, nil, "update-ref", ref, strings.TrimSpace(string(out)), old)
		}
		drop(dir, tmp)
		if err == nil {
			return nil
		}
	}
}

// private are the options of a command on Append's private ref. No
// reflog: parallel writers' deletes prune the logs/refs/notes/ci-tmp/
// directory as another writer's log is created there. A delete locks
// packed-refs, which the other deletes hold in turn: git's 1s wait for it
// runs out under load.
var private = []string{"-c", "core.logAllRefUpdates=false", "-c", "core.packedRefsTimeout=30000"}

func drop(dir, tmp string) {
	_, _ = git(context.Background(), dir, nil, append(private, "update-ref", "-d", tmp)...)
}

// Lines are the lines of the note at addr, by writer, from every writer's
// ref: this clone's refs/notes/ci/* and those fetched into
// refs/notes/remotes/<remote>/ci/*. A writer's own ref is ahead of any
// fetched copy of it (it only fast-forwards), so it wins.
func Lines(ctx context.Context, dir, addr string) (map[string][]string, error) {
	out, err := git(ctx, dir, nil, "for-each-ref", "--format=%(refname)", "refs/notes/remotes/", "refs/notes/ci/")
	if err != nil {
		return nil, err
	}
	res := map[string][]string{}
	for _, ref := range strings.Fields(string(out)) {
		i := strings.LastIndex(ref, "/ci/")
		if i < 0 {
			continue
		}
		note, err := git(ctx, dir, nil, "notes", "--ref="+ref, "show", addr)
		if err != nil {
			if strings.Contains(err.Error(), "no note found") {
				continue
			}
			return nil, err
		}
		res[ref[i+len("/ci/"):]] = strings.Split(strings.TrimSpace(string(note)), "\n")
	}
	return res, nil
}
