// The e2e board: this checkout's git-board, built with the web app inside,
// serving a throwaway repository on E2E_PORT. The repository has a
// .git-board.toml with fields and no [forges] table, and an origin on
// GitHub, so its forge is found in the remotes, as on a fresh clone. A fake
// GitHub answers what the server asks of it (a found forge's current name),
// so nothing leaves this machine. Playwright's webServer runs this; it
// lives until killed. E2E_SKIP_BUILD=1 reuses web/dist and the binary.
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const root = resolve(import.meta.dirname, '../..');
const port = process.env.E2E_PORT ?? '18321';
// E2E_DIR: the tests read the file there (playwright.config.ts sets it)
const work = process.env.E2E_DIR ?? mkdtempSync(join(tmpdir(), 'git-board-e2e-'));
rmSync(work, { recursive: true, force: true });
const bin = join(root, 'web/e2e/.bin/git-board');

// the fake GitHub: GET /repos/<owner>/<repo> is the repository, as named
const github = createServer((req, res) => {
  const m = /^\/repos\/([^/]+)\/([^/]+)$/.exec(req.url ?? '');
  res.setHeader('Content-Type', 'application/json');
  if (!m) return res.writeHead(404).end('{"message":"Not Found"}');
  res.end(JSON.stringify({ id: 1, full_name: `${m[1]}/${m[2]}`, private: false }));
});
// on a fixed port: the binary has it built in, E2E_SKIP_BUILD reuses it
const api = `http://127.0.0.1:${Number(port) + 1}`;
await new Promise((ok) => github.listen(Number(port) + 1, '127.0.0.1', ok));

// the builds in the user's own environment, the board's commands in its own
const sh = (cmd, args, cwd, e = env) => execFileSync(cmd, args, { cwd, stdio: 'inherit', env: e });
const env = {
  ...process.env,
  HOME: join(work, 'home'), // no global git config, no credential helper
  GIT_CONFIG_NOSYSTEM: '1',
  GITHUB_TOKEN: 'e2e-token',
  GIT_AUTHOR_NAME: 'E2E', GIT_AUTHOR_EMAIL: 'e2e@example.com',
  GIT_COMMITTER_NAME: 'E2E', GIT_COMMITTER_EMAIL: 'e2e@example.com',
};

// the binary takes the fake GitHub's address at build time, as the Go
// tests' storetest.GitHub does at run time
if (!process.env.E2E_SKIP_BUILD) {
  sh('npm', ['run', 'build'], join(root, 'web'), process.env);
  sh('go', ['build', '-ldflags', `-X github.com/guilbep/git-board/internal/store.GitHubAPI=${api}`, '-o', bin, './cmd/git-board'], root, process.env);
}

const repo = join(work, 'repo');
mkdirSync(repo, { recursive: true });
mkdirSync(env.HOME, { recursive: true });
sh('git', ['init', '-q', '-b', 'main'], repo);
sh('git', ['config', 'commit.gpgsign', 'false'], repo); // signed commits break go-git reads
sh('git', ['remote', 'add', 'origin', 'git@github.com:o/r.git'], repo);
// E2E_TOML: another file instead, a real board's, for a one-off smoke run
writeFileSync(
  join(repo, '.git-board.toml'),
  process.env.E2E_TOML ? readFileSync(process.env.E2E_TOML, 'utf8') : `[fields.status]
single = true
options = ["backlog", "ready", "in-progress", "done"]

[fields.prio]
single = true
options = ["p0", "p1", "p2", "p3"]

[fields.est]
single = true
numeric = true
options = ["1", "2", "4", "8"]

# one field a type, for the type's own form
[fields.due]
type = "date"

[fields.sprint]
single = true

# a date by its values alone; a select a test turns into an issue
[fields.when]
options = ["2026-01-01", "2026-02-01"]

[fields.size]
options = ["s", "m", "l"]
colors = { s = "green" }

# the kinds are flat: plain labels, no [fields.kind]
[branches.kinds]
bug = "fix"
feat = "feat"

# what the forms don't show, and must keep
[cycle]
ready = "ready"
test-files = ["*_test.go"]
criteria = ["it works"]
plans-in-pr = false

[release]
environments = ["dev", "prod"]
follow = ""
binaries = ["./cmd/x"]
platforms = ["linux/amd64"]
build = "make dist"

[repos]
list = ["o/r"]
assignees = "labels"

[[sprints.list]]
slug = "s1"
title = "Sprint 1"
start = 2026-10-05
`,
);
sh('git', ['add', '.'], repo);
sh('git', ['commit', '-q', '-m', 'the e2e board'], repo);

console.log(`e2e board: ${repo} on :${port}, GitHub at ${api}`);
const board = spawn(bin, ['serve', '--addr', `127.0.0.1:${port}`], { cwd: repo, env, stdio: 'inherit' });
board.on('exit', (code) => process.exit(code ?? 1));
for (const sig of ['SIGINT', 'SIGTERM']) process.on(sig, () => board.kill(sig));
