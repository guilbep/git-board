// The e2e boards: this checkout's git-board-next, built with next/web
// inside, serving throwaway repositories. On E2E_PORT, the e2e board: a
// .git-board.toml with fields and no [forges] table, and an origin on
// GitHub, so its forge is found in the remotes, as on a fresh clone. A fake
// GitHub (E2E_PORT + 1) answers what the server asks of it, so nothing
// leaves this machine: git never reaches origin either, its first look
// fails at once. On E2E_PORT + 5, the Markdown board: board.datastore =
// markdown:issues, kept elsewhere than git, an origin all the same.
// Playwright's webServer runs this; it lives until killed. E2E_SKIP_BUILD=1
// reuses next/web/dist and the binary.
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const root = resolve(import.meta.dirname, '../../..');
const port = Number(process.env.E2E_PORT ?? '18421');
// E2E_DIR: the tests read the file there (playwright.config.ts sets it)
const work = process.env.E2E_DIR ?? mkdtempSync(join(tmpdir(), 'git-board-next-e2e-'));
rmSync(work, { recursive: true, force: true });
const bin = join(root, 'next/web/e2e/.bin/git-board-next');

// the fake GitHub: GET /repos/<owner>/<repo> is the repository, as named
const github = createServer((req, res) => {
  const url = (req.url ?? '').split('?')[0];
  res.setHeader('Content-Type', 'application/json');
  const m = /^\/repos\/([^/]+)\/([^/]+)$/.exec(url);
  if (!m) return res.writeHead(404).end('{"message":"Not Found"}');
  res.end(JSON.stringify({ id: 1, full_name: `${m[1]}/${m[2]}`, private: false }));
});
const api = `http://127.0.0.1:${port + 1}`;
await new Promise((ok) => github.listen(port + 1, '127.0.0.1', ok));

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

if (!process.env.E2E_SKIP_BUILD) {
  sh('npm', ['run', 'build'], join(root, 'next/web'), process.env);
  sh('go', ['build', '-ldflags', `-X github.com/guilbep/git-board/next/app.GitHubAPI=${api}`, '-o', bin, './next/cmd/git-board-next'], root, process.env);
}

// a repository at dir with its file and an origin, committed
function repo(dir, origin, toml, config = {}) {
  mkdirSync(dir, { recursive: true });
  sh('git', ['init', '-q', '-b', 'main'], dir);
  sh('git', ['config', 'commit.gpgsign', 'false'], dir);
  for (const [k, v] of Object.entries(config)) sh('git', ['config', k, v], dir);
  sh('git', ['remote', 'add', 'origin', origin], dir);
  writeFileSync(join(dir, '.git-board.toml'), toml);
  sh('git', ['add', '.'], dir);
  sh('git', ['commit', '-q', '-m', 'the e2e board'], dir);
}

mkdirSync(env.HOME, { recursive: true });
// E2E_TOML: another file instead, a real board's, for a one-off smoke run
repo(join(work, 'repo'), 'git@github.com:o/r.git', process.env.E2E_TOML ? readFileSync(process.env.E2E_TOML, 'utf8') : readFileSync(join(import.meta.dirname, 'board.toml'), 'utf8'));
repo(join(work, 'markdown'), 'git@github.com:o/md.git', '[fields.status]\nsingle = true\noptions = ["todo", "done"]\n', { 'board.datastore': 'markdown:issues' });

// a board served on p, once it answers
const boards = [];
async function serve(dir, p) {
  const b = spawn(bin, ['serve', '--addr', `127.0.0.1:${p}`, '--log-level', 'warn'], { cwd: dir, env, stdio: 'inherit' });
  boards.push(b);
  for (let i = 0; i < 100; i++) {
    if (await fetch(`http://127.0.0.1:${p}/api/board`).then((r) => r.ok, () => false)) return b;
    await new Promise((ok) => setTimeout(ok, 100));
  }
  throw new Error(`the board on :${p} didn't answer`);
}
await serve(join(work, 'markdown'), port + 5);
await serve(join(work, 'repo'), port);
console.log(`e2e board: ${join(work, 'repo')} on :${port}, GitHub at ${api}; Markdown board on :${port + 5}`);
// one board gone, or a signal: both go
process.on('exit', () => boards.forEach((b) => b.kill()));
for (const b of boards) b.on('exit', (code) => process.exit(code ?? 1));
for (const sig of ['SIGINT', 'SIGTERM']) process.on(sig, () => process.exit(0));
