// Each act a person does (e2e/interactions.ts), timed on the 5,000-issue
// board: from the act to what the person sees, the CPU 4 times slower (as
// Lighthouse's mobile run), PERF_RUNS times each, a fresh page each time.
// Each run is a Go benchmark line in $PERF_OUT, which tools/perfcheck holds
// to testdata/perf/budgets.txt (5k/web/Interaction<Name>).
import { test } from '@playwright/test';
import { appendFileSync } from 'node:fs';
import { interactions, type Given } from '../e2e/interactions';

const out = process.env.PERF_OUT ?? '/dev/stdout';
const runs = Number(process.env.PERF_RUNS ?? '5');

test('each interaction, timed', async ({ browser, baseURL }) => {
  const one = async (): Promise<Given> => {
    const [first] = (await (await fetch(`${baseURL}/api/issues?limit=1&sort=created+desc`)).json()).items;
    return { issue: { id: first.id, title: first.title } };
  };
  const lines: string[] = [];
  for (const it of interactions) {
    for (let r = 0; r < runs; r++) {
      const given = await one(); // read each time: a rename moved its title
      const ctx = await browser.newContext({ baseURL: baseURL ?? '' });
      const page = await ctx.newPage();
      const cdp = await ctx.newCDPSession(page);
      await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 });
      await it.arrange(page, given);
      await page.waitForLoadState('networkidle');
      const t0 = performance.now();
      await it.act(page, given);
      await it.done(page, given);
      const ms = performance.now() - t0;
      lines.push(`BenchmarkInteraction${it.name}\t1\t${Math.round(ms * 1e6)} ns/op`);
      await ctx.close();
    }
  }
  appendFileSync(out, `pkg: github.com/guilbep/git-board/web\n${lines.join('\n')}\n`);
});
