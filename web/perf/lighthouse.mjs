// The pages' timings (make perf-web): serves a board with git-board, runs
// Lighthouse's mobile run (a slow 4G and a 4x slower CPU, simulated) on
// the board's page and an issue's, and prints each run as a Go benchmark
// line, which tools/perfcheck holds to testdata/perf/budgets.txt:
//   node web/perf/lighthouse.mjs <git-board> <board dir> <runs> > web.txt
// Chrome: $CHROME_PATH, else Playwright's Chromium, else the one installed.
import { spawn, execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { chromium } from '@playwright/test';

const [bin, dir, runs = '3'] = process.argv.slice(2);
if (!bin || !dir) {
  console.error('usage: lighthouse.mjs <git-board> <board dir> [runs]');
  process.exit(2);
}
const port = process.env.PERF_WEB_PORT ?? '18491';
const base = `http://127.0.0.1:${port}`;
const env = { ...process.env };
if (!env.CHROME_PATH && existsSync(chromium.executablePath())) env.CHROME_PATH = chromium.executablePath();

const server = spawn(bin, ['serve', '--addr', `127.0.0.1:${port}`, '--log-level', 'warn'], { cwd: dir, stdio: ['ignore', 'ignore', 'inherit'] });
const stop = () => server.kill();
process.on('exit', stop);
try {
  for (let i = 0; ; i++) {
    try {
      if ((await fetch(`${base}/api/board`)).ok) break;
    } catch {}
    if (i > 600) throw new Error(`git-board serve didn't answer on ${base}`);
    await new Promise((r) => setTimeout(r, 100));
  }
  const [first] = (await (await fetch(`${base}/api/issues?limit=1`)).json()).items;
  const pages = { Board: '/', Issue: `/#/issues/${first.id}` };
  const out = mkdtempSync(join(tmpdir(), 'perf-web-'));
  console.log(`pkg: github.com/guilbep/git-board/web`);
  for (const [name, path] of Object.entries(pages)) {
    for (let r = 0; r < Number(runs); r++) {
      const file = join(out, `${name}-${r}.json`);
      execFileSync('npx', ['-y', 'lighthouse@12.6.1', base + path, '--only-categories=performance', '--output=json', `--output-path=${file}`, '--chrome-flags=--headless=new', '--quiet'], { env, stdio: ['ignore', 'ignore', 'inherit'] });
      const a = JSON.parse(readFileSync(file, 'utf8')).audits;
      const ms = { LCP: a['largest-contentful-paint'], FCP: a['first-contentful-paint'], TBT: a['total-blocking-time'] };
      for (const [m, audit] of Object.entries(ms)) console.log(`Benchmark${name}${m}\t1\t${Math.round(audit.numericValue * 1e6)} ns/op`);
    }
  }
  rmSync(out, { recursive: true, force: true });
} finally {
  stop();
}
