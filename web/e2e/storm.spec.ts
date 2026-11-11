// A long table, is:closed past the table's windowing (about 1,000 rows on
// serve.mjs's long board), as on a real board of 1,000 closed issues:
// idle, it does no work; while the board's issues change under it (a
// sync), "more" and scrolling keep every row once, on screen, and never
// throw. A table that re-measured its rows after each render looped
// (~180 layouts in 2 s idle, then "Maximum call stack size exceeded"),
// and "more" appended ids already shown (Vue crashed on duplicate keys:
// a blank table).
import { expect, request as api, test, type APIRequestContext, type Page } from '@playwright/test';
import { errors, identity, longBoard } from './board';

const ROWS = 'tbody tr:not(.row-spacer):not(.row-more)';

async function closedIDs(r: APIRequestContext, limit: number): Promise<string[]> {
  const res = await r.get('/api/issues/ids', { params: { f: 'is:closed', limit: String(limit) } });
  expect(res.ok(), await res.text()).toBeTruthy();
  return ((await res.json()) as { lists: string[][] }).lists[0] ?? [];
}

// patch writes over whatever version the issue is at
async function patch(r: APIRequestContext, id: string, data: object) {
  const res = await r.patch(`/api/issues/${id}`, { data });
  expect(res.ok(), await res.text()).toBeTruthy();
}

// remaining: what the list's "N more" says is left to load (rows shown
// are windowed: their count says nothing of how many are loaded)
const remaining = async (page: Page) => {
  const b = page.locator('.row-more button').last();
  return (await b.count()) ? Number((await b.textContent())!.replace(/\D/g, '')) : 0;
};

// past: "more" until over n rows are loaded
async function past(page: Page, n: number) {
  const total = await remaining(page) + (await page.locator(ROWS).count());
  for (let i = 0; i < 30 && (await remaining(page)) && total - (await remaining(page)) <= n; i++) {
    const left = await remaining(page);
    await page.locator('.row-more button').last().click();
    await expect.poll(() => remaining(page)).toBeLessThan(left);
  }
}

// onScreen: the rows that intersect the viewport, by their id cell
const onScreen = (page: Page) =>
  page.evaluate((sel) => [...document.querySelectorAll(sel)].filter((r) => { const b = r.getBoundingClientRect(); return b.bottom > 0 && b.top < innerHeight; }).length, ROWS);

// scrollTo moves whatever scrolls the table (the window or its own box)
const scrollTo = (page: Page, where: number) =>
  page.evaluate((f) => {
    const row = document.querySelector('tbody tr');
    for (let el = row?.parentElement; el; el = el.parentElement) {
      if (/(auto|scroll)/.test(getComputedStyle(el).overflowY) && el.scrollHeight > el.clientHeight) {
        el.scrollTop = (el.scrollHeight - el.clientHeight) * f;
        return;
      }
    }
    window.scrollTo(0, (document.documentElement.scrollHeight - innerHeight) * f);
  }, where);

let r: APIRequestContext;
test.beforeAll(async () => {
  r = await api.newContext({ baseURL: longBoard });
  await identity(r);
  // rows of different heights all through the list, as a real board's
  // long titles and two forge links make them
  const ids = (await closedIDs(r, 400)).filter((_, i) => i % 2);
  for (let i = 0; i < ids.length; i += 20) await Promise.all(ids.slice(i, i + 20).map((id) => patch(r, id, { title: `A long title ${'that goes on and on '.repeat(15)}${id}` })));
});
test.afterAll(() => r.dispose());

test('a long table, idle, does no work', async ({ page }) => {
  const errs = errors(page);
  await page.goto(`${longBoard}/#/?filter=is:closed`);
  await expect(page.locator(ROWS).first()).toBeVisible();
  await past(page, Infinity); // every page: none loads under the measure
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Performance.enable');
  const metric = async (name: string) => ((await cdp.send('Performance.getMetrics')).metrics.find((m) => m.name === name)?.value ?? 0);
  // wheeled down then back up, as a person reads it: a row of another
  // height first on screen set the loop off, which blanked the table
  // what the table itself does: other parts of the page animate (a tip
  // fading), which recalculates styles, never the table's rows
  await page.evaluate(() => {
    const w = window as unknown as { __muts: number };
    w.__muts = 0;
    new MutationObserver((l) => (w.__muts += l.length)).observe(document.querySelector('.issue-table')!, { childList: true, attributes: true, subtree: true });
  });
  const muts = () => page.evaluate(() => (window as unknown as { __muts: number }).__muts);
  for (let i = 0; i < 25; i++) {
    await page.mouse.move(700, 500);
    await page.mouse.wheel(0, i < 15 ? 1200 : -1600);
    await page.waitForTimeout(150);
    await page.mouse.move(5, 5); // off the rows: a row's hover fades in, styles a few frames
    await page.waitForTimeout(300);
    const [layouts, changes] = [await metric('LayoutCount'), await muts()];
    await page.waitForTimeout(500);
    expect(await metric('LayoutCount') - layouts, `layouts while idle, step ${i}`).toBeLessThanOrEqual(3);
    expect(await muts() - changes, `table changes while idle, step ${i}`).toBe(0);
    expect(await onScreen(page), `rows on screen, step ${i}`).toBeGreaterThan(0);
  }
  expect(await onScreen(page)).toBeGreaterThan(0);
  expect(errs).toEqual([]);
});

test('a long table keeps its rows while the board changes under it', async ({ page }) => {
  test.slow();
  const errs = errors(page);
  await page.goto(`${longBoard}/#/?filter=is:closed`);
  await expect(page.locator(ROWS).first()).toBeVisible();
  // the storm: issues of the first page reopened and closed again, each
  // shifting the pages after it, as a sync's writes do
  const first = await closedIDs(r, 40);
  let stop = false;
  const storm = (async () => {
    for (let i = 0; i < 150 && !stop; i++) {
      const id = first[i % first.length]!;
      await patch(r, id, { status: i % 2 ? 'closed' : 'open' });
    }
    for (const id of first) await patch(r, id, { status: 'closed' });
  })();
  try {
    let left = Infinity;
    for (let round = 0; round < 6; round++) {
      await scrollTo(page, 1);
      // the end coming near loads it too: the button may be gone as it's clicked
      await page.locator('.row-more button').last().click({ timeout: 2000 }).catch(() => {});
      await page.waitForTimeout(300);
      // a live reload keeps the pages loaded: what is left only shrinks
      // (give or take the issues the storm reopened meanwhile)
      // (no button: a list reloading, or loaded whole; nothing to read)
      if (await page.locator('.row-more button').count()) {
        const now = await remaining(page);
        expect(now, 'rows left to load never grow back').toBeLessThanOrEqual(left + 40);
        left = Math.min(left, now);
      }
      for (const where of [0.5, 0]) {
        await scrollTo(page, where);
        await expect.poll(() => onScreen(page), `rows on screen at ${where}`).toBeGreaterThan(0);
      }
    }
  } finally {
    stop = true;
    await storm;
  }
  const ids = await page.locator(`${ROWS} td.cell-id`).allTextContents();
  expect(ids.length - new Set(ids).size, 'rows shown twice').toBe(0);
  expect(errs).toEqual([]);
});
