// The e2e board: this checkout's git-board, built with the web app inside,
// serving a throwaway repository on E2E_PORT. The repository has a
// .git-board.toml with fields and no [forges] table, and an origin on
// GitHub, so its forge is found in the remotes, as on a fresh clone. A fake
// GitHub answers what the server asks of it (a found forge's current name),
// so nothing leaves this machine: git never reaches origin either, its
// first look fails at once, as an unreachable origin's does. Beside it, on
// E2E_PORT + 2, the clone board: a fresh clone of a local origin that holds
// two issues, whose first open pulls them (docs/prd/clone-shows-board.md).
// Playwright's webServer runs this; it lives until killed.
// E2E_SKIP_BUILD=1 reuses web/dist and the binary.
import { execFileSync, spawn } from 'node:child_process';
import { once } from 'node:events';
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
  GIT_ALLOW_PROTOCOL: 'file', // git reaches local paths only: origin, git@github.com:o/r.git, can't be read
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
bug = "bug"
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

// a board served on p, once it answers
const serve = async (dir, p) => {
  const b = spawn(bin, ['serve', '--addr', `127.0.0.1:${p}`], { cwd: dir, env, stdio: 'inherit' });
  for (let i = 0; i < 100; i++) {
    if (await fetch(`http://127.0.0.1:${p}/api/config/model`).then((r) => r.ok, () => false)) return b;
    await new Promise((ok) => setTimeout(ok, 100));
  }
  throw new Error(`the board on :${p} didn't answer`);
};

// the clone board: origin gets two issues from a seed clone (made through
// its server: no command writes an issue), then a fresh clone of it opens
const origin = join(work, 'origin.git');
const seed = join(work, 'seed');
const clone = join(work, 'clone');
sh('git', ['init', '-q', '--bare', '-b', 'main', origin], work);
mkdirSync(seed);
sh('git', ['init', '-q', '-b', 'main'], seed);
sh('git', ['config', 'commit.gpgsign', 'false'], seed);
sh('git', ['remote', 'add', 'origin', origin], seed);
sh('git', ['commit', '-q', '--allow-empty', '-m', 'the clone board'], seed);
sh('git', ['push', '-q', 'origin', 'main'], seed);
const seeder = await serve(seed, Number(port) + 3);
try {
  const post = async (path, body) => {
    const r = await fetch(`http://127.0.0.1:${Number(port) + 3}${path}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!r.ok) throw new Error(`the seed's ${path}: ${r.status} ${await r.text()}`);
  };
  await post('/api/me', { name: 'Seed', email: 'seed@example.com' });
  for (const title of ['Seeded alpha', 'Seeded beta']) await post('/api/issues', { title, body: '', labels: [] });
} finally {
  seeder.kill();
  await once(seeder, 'exit'); // its lock goes with it
}
sh(bin, ['push'], seed);
sh('git', ['clone', '-q', origin, clone], work);
const cloneBoard = await serve(clone, Number(port) + 2);

console.log(`e2e board: ${repo} on :${port}, GitHub at ${api}; clone board: ${clone} on :${Number(port) + 2}`);
const board = await serve(repo, port);
// one board gone, or a signal: both go
process.on('exit', () => [board, cloneBoard].forEach((b) => b.kill()));
for (const b of [board, cloneBoard]) b.on('exit', (code) => process.exit(code ?? 1));
for (const sig of ['SIGINT', 'SIGTERM']) process.on(sig, () => process.exit(0));
