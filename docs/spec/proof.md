# Proof notes

A check's result is one signed line in a git note
([ADR 0001](../adr/0001-check-results-are-signed-git-notes.md)). This page
gives the line, the content key, the signature and the refs, precise
enough to make sure that a proof holds, with git and `ssh-keygen` alone. The code is
`domain/releases/proof`. The configuration is
[Configuration › Tests and checks](../guide/configuration.md#tests-and-checks).

## The line

```text
v1 go-tests PASSED key=3f1c…e9 platform=darwin/arm64 tools=sha256:5d2e… commit=9440305… at=2026-10-22T14:03:11Z dur=48s log=sha256:7a0b… sig=U1NIU0lH…
```

| Field | Meaning |
| --- | --- |
| `v1` | the format's version |
| `go-tests` | the check: a `[cycle.tests.*]` or `[cycle.checks.*]` name |
| `PASSED` or `FAILED` | the result, in the in-toto test-result vocabulary |
| `key=` | the content key, the note's address |
| `platform=` | the runner's OS and architecture |
| `tools=` | the sha256 of the output of the check's version commands, in order |
| `commit=` | the commit that the run checked out, for people to read. The key is what counts |
| `at=`, `dur=` | when the run started and how long it took, by the runner's clock. For audit, not trusted |
| `log=` | the sha256 of the run's output. The log stays with the runner |
| `sig=` | the SSH signature, base64 on one line |

- The fields come in this order, and no value holds a space.
- A reader ignores a line that it cannot parse.
- A reader never depends on the order of lines from different refs. These
  are the in-toto bundle rules.
- A standard in-toto statement can come from a line later. The same fact
  in a DSSE envelope is 930 bytes of base64, against 375 readable bytes.

## The content key

The key is `git hash-object --stdin`, without `-w`, over a canonical text.
So it has the repository's own hash format.

```text
git-board-ci/key/v1
check go-tests
run ["go","test","./..."]
env CGO_ENABLED=0
file 100644 6e1f… go.mod
file 100644 b2c4… go.sum
file 100644 91d0… domain/cycle/cycle.go
…
```

- The `run` line is the check's command as an argv, in JSON.
- A `dir <path>` line follows when the check runs in a directory of its
  own. A check run elsewhere is another check.
- The `env` lines are the variables that the file sets for the check, as
  `env NAME=value`, sorted by name.
- The `file` lines are every tracked file of the commit that the check's
  `files` globs match, as mode, blob id and path, sorted by path.
- A check with no `files` covers the whole tree.

The key holds only what anyone can compute again from the commit and the
file. The machine's side, its platform and its tools, is in the signed
line.

`git notes` accepts an address with no object behind it. Such a note
travels, merges and survives `gc`. `git notes prune` deletes it, so
git-board never runs prune on these refs.

## The signature

1. The payload is `git-board-ci/v1`, a newline, then the line up to
   ` sig=`.
2. Sign it with `ssh-keygen -Y sign -n git-board-ci`.
3. Write the signature blob's base64, unwrapped, after `sig=`.

The signer is the principal that `ssh-keygen -Y find-principals` names.
Nothing written in the line names the signer.

The signers file is in the repository, in OpenSSH's `allowed_signers`
format, with `namespaces="git-board-ci"`. Its path is `[cycle] signers`,
`.allowed_signers` by default. A developer's own
`gpg.ssh.allowedSignersFile` plays no part.

## The refs

- A clone writes only `refs/notes/ci/<writer>`. `git config
  git-board.writer` names the writer, set on the first run to
  `<user.name>-<host>-<4 hex>`.
- A clone fetches the other writers' refs into
  `refs/notes/remotes/<remote>/ci/*`, never with a `+` into its own
  namespace.
- A write is a compare-and-swap. git-board copies the ref to a private
  ref under `refs/notes/ci-tmp/`, appends there, and runs
  `git update-ref refs/notes/ci/<writer> <new> <old>`. On a failure, it
  starts again.
- `notes.rewriteRef` is never set for these refs.

## A proof read by hand

1. Read the check's `run`, `env` and `files` from the base branch's
   `.git-board.toml`.
2. Compute the key from the commit, as [The content key](#the-content-key)
   says.
3. Read the note at that key from every writer's ref.
4. Verify each line's signature against the base branch's signers file.
   Ignore a line that fails.
5. Apply the rule of ADR 0001, section 6.

`git board verify [commit]` runs the same steps.

## Evidence

Two reviews on 2026-10-22 ran experiments on git 2.50.1 and OpenSSH 10.2,
in throwaway repositories.

- Parallel writes lose lines with no error. Two worktrees each ran 100
  `git notes append` calls. 141 of 200 lines stayed, and every exit code
  was 0. With compare-and-swap, 200 of 200 stayed, after 107 retries.
- A note on a hash with no object behind it shows in a clone, survives
  `gc --prune=now`, and merges. `git notes prune` lists it for deletion.
- `notes.rewriteRef` copied a pass onto a rebased commit whose covered
  file changed: a false proof.
- 2,000 notes took 904 KiB once packed, with a 19 ms lookup.
- The one-line SSH signature verified. A changed payload, the wrong
  namespace, and a principal that someone else claimed were each
  refused. `-Overify-time` lets a signer choose the time, so the time is
  not trusted. A key rotation is a revocation and a new run of the checks.
- GitHub keeps `refs/notes/*`, shows none of them since 2014, and its
  rulesets do not cover them. GitLab shows nothing, and its pull mirroring
  drops them. Gitea and Forgejo show only `refs/notes/commits`.

## Prior art

- mhagger's git-test keys results by tree in `refs/notes/tests/<name>`,
  and records nothing from a dirty tree.
- git-appraise keeps CI results in `refs/notes/devtools/ci`, unsigned.
- gittuf signs in-toto attestations with SSH under `refs/gittuf/`.
- SLSA's source-tool stores one signed attestation per line of a git
  note. It is the closest precedent.
- Bazel's action key and Turborepo's hash also cover the command and the
  toolchain, as the key and the line do here.

Sources:

- https://git-scm.com/docs/git-notes
- https://man.openbsd.org/ssh-keygen
- https://github.com/mhagger/git-test
- https://github.com/google/git-appraise
- https://github.com/gittuf/gittuf/blob/main/docs/design-document.md
- https://github.com/slsa-framework/source-tool/blob/main/docs/DESIGN.md
- https://github.com/in-toto/attestation/blob/main/spec/predicates/test-result.md
- https://github.com/in-toto/attestation/blob/main/spec/v1/bundle.md
- https://github.com/basecamp/gh-signoff
- https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets
- https://docs.github.com/en/repositories/creating-and-managing-repositories/repository-limits
- https://github.blog/news-insights/git-notes-display
- https://gitlab.com/gitlab-org/gitlab/-/issues/15029
- https://gitlab.com/gitlab-org/gitlab/-/issues/23519
- https://github.com/go-gitea/gitea/blob/main/modules/git/notes.go
- https://bazel.build/remote/caching
