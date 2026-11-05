// The e2e boards: this checkout's git-board-next, built with next/web
// inside, serving throwaway repositories. On E2E_PORT, the e2e board: a
// .git-board.toml with fields and no [forges] table, and an origin on
// GitHub, so its forge is found in the remotes, as on a fresh clone. A fake
// GitHub (E2E_PORT + 1) answers what the server asks of it, so nothing
// leaves this machine: git never reaches origin either, its first look
// fails at once. On E2E_PORT + 5, the Markdown board: board.datastore =
// markdown:issues, kept elsewhere than git, an origin all the same, and
// attachments on, to a fake S3 (E2E_PORT + 2) that keeps what it is put.
// On E2E_PORT + 3, the clone board: a fresh clone of a local origin that
// holds two issues, whose first open pulls them (clone.spec.ts).
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

// the fake GitHub: GET /repos/<owner>/<repo> is the repository, as named;
// POST /graphql, o's Project 1 (project.spec.ts): a Status field, and one
// item, an issue no board here has
const project = {
  id: 'PVT_o', title: 'The e2e Project',
  fields: { nodes: [{ id: 'F-status', name: 'Status', dataType: 'SINGLE_SELECT', options: [{ id: 'o-ready', name: 'Ready' }, { id: 'o-done', name: 'Done' }] }] },
  items: { pageInfo: { hasNextPage: false, endCursor: '1' }, nodes: [{ id: 'PVTI_1', content: { __typename: 'Issue', number: 77, url: 'https://github.com/o/elsewhere/issues/77', title: 'Elsewhere' },
    fieldValues: { nodes: [{ __typename: 'ProjectV2ItemFieldSingleSelectValue', name: 'Ready', field: { name: 'Status' } }] } }] },
};
// and o/r's pull requests (cycle.spec.ts): opened, edited and merged
// here, found by their head branch; the base's rules require none
const pulls = [];
const prNode = (p) => ({
  number: p.number, url: `https://github.com/o/r/pull/${p.number}`, state: p.state, isDraft: false, title: p.title, body: p.body,
  baseRefName: p.base, headRefOid: p.sha, mergeCommit: p.state === 'MERGED' ? { oid: p.sha } : null, headRepository: { nameWithOwner: 'o/r' },
  author: { login: 'e2e' }, reviewDecision: '', mergeable: 'MERGEABLE', mergeStateStatus: 'CLEAN',
  latestOpinionatedReviews: { nodes: [] }, reviewRequests: { nodes: [] }, commits: { nodes: [] },
});
const github = createServer((req, res) => {
  const url = (req.url ?? '').split('?')[0];
  res.setHeader('Content-Type', 'application/json');
  let body = '';
  req.on('data', (c) => (body += c));
  req.on('end', () => {
    const json = body ? JSON.parse(body) : {};
    if (req.method === 'POST' && url === '/graphql') {
      const { variables } = json;
      if (variables.head !== undefined) {
        const nodes = pulls.filter((p) => p.head === variables.head).reverse().map(prNode);
        return res.end(JSON.stringify({ data: { repository: { pullRequests: { nodes } } } }));
      }
      const p = variables.owner === 'o' && variables.n === 1 ? project : null;
      return res.end(JSON.stringify({ data: { repositoryOwner: { projectV2: p } } }));
    }
    if (url.startsWith('/repos/o/r/rules/branches/')) return res.end('[]');
    if (req.method === 'POST' && url === '/repos/o/r/pulls') {
      // the head's commit: the branch's, as git has it in the e2e repository
      const sha = execFileSync('git', ['rev-parse', `refs/remotes/origin/${json.head}`], { cwd: join(work, 'repo'), env }).toString().trim();
      const p = { number: pulls.length + 1, state: 'OPEN', title: json.title, body: json.body, base: json.base, head: json.head, sha };
      pulls.push(p);
      return res.end(JSON.stringify({ number: p.number, html_url: `https://github.com/o/r/pull/${p.number}` }));
    }
    const pm = /^\/repos\/o\/r\/pulls\/(\d+)(\/merge)?$/.exec(url);
    const pull = pm ? pulls[Number(pm[1]) - 1] : undefined;
    if (pull && req.method === 'PATCH') {
      Object.assign(pull, { title: json.title, body: json.body });
      return res.end('{}');
    }
    if (pull && pm[2] && req.method === 'PUT') {
      if (json.sha !== pull.sha) return res.writeHead(409).end('{"message":"Head branch was modified"}');
      pull.state = 'MERGED';
      return res.end('{"merged":true}');
    }
    route(url, res);
  });
});
function route(url, res) {
  const m = /^\/repos\/([^/]+)\/([^/]+)$/.exec(url);
  if (!m) return res.writeHead(404).end('{"message":"Not Found"}');
  res.end(JSON.stringify({ id: 1, full_name: `${m[1]}/${m[2]}`, private: false }));
}
const api = `http://127.0.0.1:${port + 1}`;
await new Promise((ok) => github.listen(port + 1, '127.0.0.1', ok));

// the fake S3: a signed PUT is kept, by its path; a GET answers it
const s3Objects = new Map();
const s3 = createServer((req, res) => {
  if (req.method === 'PUT' && req.headers.authorization) {
    const chunks = [];
    req.on('data', (c) => chunks.push(c));
    req.on('end', () => {
      s3Objects.set(req.url, Buffer.concat(chunks));
      res.end();
    });
    return;
  }
  const kept = s3Objects.get(req.url ?? '');
  if (req.method === 'GET' && kept) return res.end(kept);
  res.writeHead(403).end('<Error><Code>AccessDenied</Code></Error>');
});
await new Promise((ok) => s3.listen(port + 2, '127.0.0.1', ok));

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

// the clone board's origin: two issues, written and pushed by a seed
// clone, then a fresh clone of it, which git clone gives no board
const origin = join(work, 'origin.git');
const seed = join(work, 'seed');
sh('git', ['init', '-q', '--bare', '-b', 'main', origin], work);
mkdirSync(seed);
sh('git', ['init', '-q', '-b', 'main'], seed);
for (const [k, v] of [['commit.gpgsign', 'false'], ['user.name', 'Seed'], ['user.email', 'seed@example.com']]) sh('git', ['config', k, v], seed);
sh('git', ['remote', 'add', 'origin', origin], seed);
sh('git', ['commit', '-q', '--allow-empty', '-m', 'the clone board'], seed);
sh('git', ['push', '-q', 'origin', 'main'], seed);
sh(bin, ['me', '--new'], seed);
for (const title of ['Seeded alpha', 'Seeded beta']) sh(bin, ['new', title], seed);
sh(bin, ['push'], seed);
sh('git', ['clone', '-q', origin, join(work, 'clone')], work);

// a board served on p, once it answers
const boards = [];
async function serve(dir, p, more = {}) {
  const b = spawn(bin, ['serve', '--addr', `127.0.0.1:${p}`, '--log-level', 'warn'], { cwd: dir, env: { ...env, ...more }, stdio: 'inherit' });
  boards.push(b);
  for (let i = 0; i < 100; i++) {
    if (await fetch(`http://127.0.0.1:${p}/api/board`).then((r) => r.ok, () => false)) return b;
    await new Promise((ok) => setTimeout(ok, 100));
  }
  throw new Error(`the board on :${p} didn't answer`);
}
await serve(join(work, 'markdown'), port + 5, {
  ACCESS_KEY_ID: 'e2e', ACCESS_KEY_SECRET: 'e2e-secret', BUCKET_NAME: 'e2e', UPLOADS_ENDPOINT: `http://127.0.0.1:${port + 2}`,
});
await serve(join(work, 'clone'), port + 3);
await serve(join(work, 'repo'), port);
console.log(`e2e board: ${join(work, 'repo')} on :${port}, GitHub at ${api}; clone board on :${port + 3}; Markdown board on :${port + 5}`);
// one board gone, or a signal: both go
process.on('exit', () => boards.forEach((b) => b.kill()));
for (const b of boards) b.on('exit', (code) => process.exit(code ?? 1));
for (const sig of ['SIGINT', 'SIGTERM']) process.on(sig, () => process.exit(0));
