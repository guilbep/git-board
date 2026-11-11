// Each act a person does (e2e/interactions.ts), timed on the 5,000-issue
// board in two setups (ADR 0023):
//
//   Desk:  the CPU as it is, the server on this machine
//   Worst: the CPU 6x slower (a low-end phone), slow 4G (150 ms a round
//          trip, 1.6 Mbps down, 750 kbps up)
//
// An act's time is measured in the page, never by the test's clock (its
// clicks and polls add tens of ms): the longest of the browser's Event
// Timing entries of the act (input to the next paint, as INP measures it)
// and the page's own timing of what it started (interaction.ts: from the
// act to the paint after its last request); an optimistic act, its Event
// Timing alone: it shows before its answer, which the page waits for. PERF_RUNS runs each, a fresh
// page each time; each a Go benchmark line in $PERF_OUT
// (BenchmarkInteraction<Name><Setup>), held by testdata/perf/budgets.txt.
import { test, type BrowserContext, type Page } from '@playwright/test';
import { appendFileSync } from 'node:fs';
import { interactions, type Given } from '../e2e/interactions';

const out = process.env.PERF_OUT ?? '/dev/stdout';
const runs = Number(process.env.PERF_RUNS ?? '5');
const only = process.env.PERF_ONLY; // one act, by name: while working on it

interface Setup {
  name: string;
  cpu: number;
  network?: { latency: number; downloadThroughput: number; uploadThroughput: number };
}
const setups: Setup[] = [
  { name: 'Desk', cpu: 1 },
  { name: 'Worst', cpu: 6, network: { latency: 150, downloadThroughput: (1.6 * 1024 * 1024) / 8, uploadThroughput: (750 * 1024) / 8 } },
];

async function throttled(ctx: BrowserContext, page: Page, s: Setup) {
  const cdp = await ctx.newCDPSession(page);
  await cdp.send('Emulation.setCPUThrottlingRate', { rate: s.cpu });
  if (s.network) await cdp.send('Network.emulateNetworkConditions', { offline: false, ...s.network });
}

// watch starts the page's record of the act: Event Timing entries from now,
// and how many interactions interaction.ts timed before it
const watch = (page: Page) =>
  page.evaluate(() => {
    const w = window as unknown as { __perfEvents: number[]; __perfActsBefore: number; __gbActs?: { ms: number }[] };
    w.__perfEvents = [];
    w.__perfActsBefore = w.__gbActs?.length ?? 0;
    new PerformanceObserver((l) => {
      for (const e of l.getEntries()) w.__perfEvents.push(e.duration);
    }).observe({ type: 'event', durationThreshold: 16 } as PerformanceObserverInit);
  });

// took is the act's time: its longest event, or what it started, timed in
// the page, once the page is still
async function took(page: Page, shownOnly: boolean): Promise<number> {
  await page.waitForLoadState('networkidle');
  // the page's own word that its act ended (interaction.ts): a throttled
  // network's requests outlast Playwright's networkidle
  await page.waitForFunction(() => (window as unknown as { __gbNow?: () => { current: string | null } }).__gbNow?.().current === null, null, { timeout: 30_000, polling: 100 });
  const [ms, seen] = await page.evaluate(
    (shown) =>
      new Promise<[number, string]>((done) =>
        requestAnimationFrame(() =>
          requestAnimationFrame(() => {
            const w = window as unknown as { __perfEvents: number[]; __perfActsBefore: number; __gbActs?: { ms: number }[] };
            const acts = (w.__gbActs ?? []).slice(w.__perfActsBefore).map((a) => a.ms);
            done([Math.max(0, ...w.__perfEvents, ...(shown ? [] : acts)), JSON.stringify({ events: w.__perfEvents, acts: (w.__gbActs ?? []).slice(w.__perfActsBefore), all: w.__gbActs, before: w.__perfActsBefore, now: (window as unknown as { __gbNow?: () => unknown }).__gbNow?.() })]);
          }),
        ),
      ),
    shownOnly,
  );
  if (process.env.PERF_DEBUG) console.log(seen);
  return ms;
}

test('each interaction, timed', async ({ browser, baseURL }) => {
  const one = async (): Promise<Given> => {
    const [first] = (await (await fetch(`${baseURL}/api/issues?limit=1&sort=created+desc`)).json()).items;
    return { issue: { id: first.id, title: first.title } };
  };
  const lines: string[] = [];
  for (const s of setups) {
    for (const it of interactions.filter((x) => !only || x.name === only)) {
      for (let r = 0; r < runs; r++) {
        const given = await one(); // read each time: a rename moved its title
        const ctx = await browser.newContext({ baseURL: baseURL ?? '' });
        const page = await ctx.newPage();
        await throttled(ctx, page, s);
        await it.arrange(page, given);
        await page.waitForLoadState('networkidle');
        await watch(page);
        const t0 = Date.now();
        if (process.env.PERF_DEBUG) {
          page.on('request', (q) => q.url().includes('/api/') && console.log(`  > ${Date.now() - t0}ms ${q.method()} ${new URL(q.url()).pathname}`));
          page.on('requestfinished', (q) => q.url().includes('/api/') && console.log(`  < ${Date.now() - t0}ms ${q.method()} ${new URL(q.url()).pathname}`));
        }
        await it.act(page, given);
        await it.done(page, given);
        // an optimistic act: what the person sees, the paint after the act;
        // its write's answer comes after, and is the page's to wait for
        const ms = await took(page, !!it.optimistic);
        lines.push(`BenchmarkInteraction${it.name}${s.name}\t1\t${Math.round(ms * 1e6)} ns/op`);
        await ctx.close();
      }
    }
  }
  appendFileSync(out, `pkg: github.com/guilbep/git-board/web\n${lines.join('\n')}\n`);
});
