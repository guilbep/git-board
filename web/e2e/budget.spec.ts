// What each page asks for when it opens, held to a budget: how many API
// requests, none asked twice, and the web app's files gzipped. Their size
// is held loosely (MAX_KB): the lazy chunks a page prefetches once idle
// vary from run to run (77 to 206 KB on the board). Counted, not timed, so it holds on any machine (the timings are
// the nightly's: make perf-web, testdata/perf/budgets.txt). A budget
// moves down when a page asks for less; up only with a reason written
// here, never to let a page through that asks for more.
import { expect, test, type Browser } from '@playwright/test';
import { identity, issue } from './board';

type Opened = { api: string[]; assets: number; plain: string[] };

// open loads url in a fresh browser (nothing cached) and tells what it asked
async function open(browser: Browser, baseURL: string, url: string): Promise<Opened> {
  const ctx = await browser.newContext({ baseURL });
  const page = await ctx.newPage();
  const out: Opened = { api: [], assets: 0, plain: [] };
  const sizes: Promise<void>[] = [];
  page.on('requestfinished', (r) => {
    const u = new URL(r.url());
    if (u.origin !== baseURL) return;
    if (u.pathname.startsWith('/api/')) out.api.push(`${r.method()} ${u.pathname}${u.search}`);
    else if (/\.(js|css)$/.test(u.pathname))
      sizes.push(
        (async () => {
          const res = await r.response();
          const size = (await r.sizes()).responseBodySize;
          out.assets += size;
          if (size > 1024 && !['br', 'gzip'].includes(res?.headers()['content-encoding'] ?? '')) out.plain.push(u.pathname);
        })(),
      );
  });
  await page.goto(url);
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(800); // the header's inbox count comes after the page (AppHeader)
  await Promise.all(sizes);
  await ctx.close();
  return out;
}

// a page's JS and CSS at most, KB as sent: a library imported whole shows.
// 300 with Quasar's CSS (281 KB raw); without it (ADR 0023) the entry is
// 185 KB raw, under 60 sent
const MAX_KB = 200;

// page, its API requests at most
const budgets: [string, number][] = [
  ['/#/', 12], // 8 every page asks (board, config, fields, me, people, state, sync, inbox), the list, the filters' 2 label counts, the closed count
  ['/#/issues/{id}', 13], // the issue, its cycle, follow, the uploads' settings
  ['/#/inbox', 8],
  ['/#/work?who=team', 9],
  ['/#/settings', 16], // its sections' sources: datastore, forges, project, the guide (one Markdown), 2 dry runs of the file
  ['/#/releases', 9], // local first, then fetched: 2
];

test('each page asks for no more than its budget, nothing twice, its files compressed', async ({ browser, request, baseURL }) => {
  await identity(request);
  const made = await issue(request, 'Budget issue');
  for (const [path, maxAPI] of budgets) {
    const got = await open(browser, baseURL!, path.replace('{id}', made.id));
    const gets = got.api.filter((a) => a.startsWith('GET '));
    const twice = gets.filter((a, i) => gets.indexOf(a) !== i);
    expect.soft(twice, `${path}: asked twice`).toEqual([]);
    expect.soft(got.api.length, `${path}: API requests\n  ${got.api.sort().join('\n  ')}`).toBeLessThanOrEqual(maxAPI);
    expect.soft(got.plain, `${path}: files sent uncompressed`).toEqual([]);
    expect.soft(Math.round(got.assets / 1024), `${path}: JS and CSS, KB`).toBeLessThanOrEqual(MAX_KB);
  }
});
