// How many requests the page makes: the parts that hear one change event
// share what they ask (api/http.ts), the header keeps the board's state
// across pages (me.ts), and a hidden tab lets its event stream go
// (live.ts), so that open tabs don't take the browser's six connections.
import { expect, test, type Page } from '@playwright/test';
import { errors, identity } from './board';

// asked counts the page's GETs of path from now on
function asked(page: Page, path: string): () => number {
  let n = 0;
  page.on('request', (r) => {
    if (r.method() === 'GET' && new URL(r.url()).pathname === path) n++;
  });
  return () => n;
}

test('a config changed elsewhere asks who writes once, though several parts reload it', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  await page.goto('/#/settings');
  await expect(page.getByRole('banner')).toBeVisible();
  await page.waitForLoadState('networkidle');
  const me = asked(page, '/api/me');
  const before = (await (await request.get('/api/config')).json()) as { text: string; version: string };
  const put = await request.put('/api/config', { data: { text: before.text + '\n# e2e: requests\n' }, headers: { 'If-Match': `"${before.version}"` } });
  expect(put.ok(), await put.text()).toBeTruthy();
  try {
    await expect.poll(me, { timeout: 5000 }).toBeGreaterThan(0);
    await page.waitForLoadState('networkidle');
    expect(me()).toBe(1); // LiveNotice and the Settings page both reload it
  } finally {
    const now = (await (await request.get('/api/config')).json()) as { version: string };
    await request.put('/api/config', { data: { text: before.text }, headers: { 'If-Match': `"${now.version}"` } });
  }
  expect(errs).toEqual([]);
});

test("another page's header doesn't ask the board's state again", async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  await page.goto('/#/inbox');
  await expect(page.getByRole('banner')).toBeVisible();
  await page.waitForLoadState('networkidle');
  const state = asked(page, '/api/state');
  await page.goto('/#/releases');
  await expect(page.getByRole('banner')).toBeVisible();
  await page.waitForLoadState('networkidle');
  expect(state()).toBe(0);
  expect(errs).toEqual([]);
});

test('a hidden tab lets its event stream go, and takes it again when shown', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  await page.clock.install();
  const streams = asked(page, '/api/events');
  let ended = 0;
  page.on('requestfailed', (r) => new URL(r.url()).pathname === '/api/events' && ended++);
  page.on('requestfinished', (r) => new URL(r.url()).pathname === '/api/events' && ended++);
  await page.goto('/#/inbox');
  await expect.poll(streams).toBe(1);

  const show = (hidden: boolean) =>
    page.evaluate((h) => {
      Object.defineProperty(document, 'hidden', { configurable: true, get: () => h });
      document.dispatchEvent(new Event('visibilitychange'));
    }, hidden);
  await show(true);
  await page.clock.runFor(11_000);
  await expect.poll(() => ended).toBe(1);
  await show(false);
  await expect.poll(streams).toBe(2);
  expect(errs).toEqual([]);
});

test("a phone's table asks its first page, not every page", async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  for (let i = 0; i < 120; i++) {
    const r = await request.post('/api/issues', { data: { title: `E2E phone page ${i}`, body: '', labels: ['e2ephone'], set: [] } });
    expect(r.ok(), await r.text()).toBeTruthy();
  }
  await page.setViewportSize({ width: 412, height: 823 });
  let pages = 0;
  page.on('request', (r) => {
    const u = new URL(r.url());
    if (u.pathname === '/api/issues' && u.searchParams.get('cursor')) pages++;
  });
  await page.goto('/#/?layout=table&filter=' + encodeURIComponent('label:e2ephone'));
  await expect(page.locator('.row-more')).toBeVisible();
  await page.waitForLoadState('networkidle');
  expect(pages).toBe(0); // the next page waits for the reader to come near it
  expect(errs).toEqual([]);
});
