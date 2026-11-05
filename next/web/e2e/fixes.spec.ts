// Review fixes the browser shows: a new issue a field refused is created
// once, and j/k go on after the open issue leaves the view.
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

function errors(page: Page): Error[] {
  const out: Error[] = [];
  page.on('pageerror', (e) => out.push(e));
  return out;
}

async function identity(request: APIRequestContext) {
  const r = await request.post('/api/me', { data: { name: 'E2E', email: 'e2e@example.com' } });
  expect(r.ok()).toBeTruthy();
}

async function issue(request: APIRequestContext, title: string, labels: string[] = []): Promise<string> {
  const r = await request.post('/api/issues', { data: { title, body: '', labels } });
  expect(r.ok(), await r.text()).toBeTruthy();
  return ((await r.json()) as { id: string }).id;
}

async function count(request: APIRequestContext, title: string): Promise<number> {
  const r = await request.get(`/api/issues?${new URLSearchParams({ filter: 'is:open,closed' })}`);
  const buckets = (await r.json()) as { issues: { title: string }[] }[];
  return buckets.flatMap((b) => b.issues).filter((i) => i.title === title).length;
}

test('a new issue with a refused field is created once, and the dialog says which field', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  await page.goto('/#/');
  const dialog = page.getByRole('dialog', { name: /New issue/ });
  // c works once the board has mounted its keys: a press before is lost
  await expect(async () => {
    await page.keyboard.press('c');
    await expect(dialog).toBeVisible({ timeout: 1000 });
  }).toPass();
  await dialog.getByLabel('Title (required)').fill('E2E refused field');
  await dialog.getByLabel('Size', { exact: true }).fill('m');
  // the server refuses the field once the issue exists (a value it can't
  // take, a forge sync starting): every PATCH, here
  await page.route('**/api/issues/*', (route) =>
    route.request().method() === 'PATCH'
      ? route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ error: 'size: refused here' }) })
      : route.fallback(),
  );
  await dialog.getByRole('button', { name: 'Create', exact: true }).click();
  const alert = dialog.getByRole('alert');
  await expect(alert).toContainText('without these fields');
  await expect(alert).toContainText('size: refused here');
  await page.unroute('**/api/issues/*');
  // the form started over: a second Create can't make the same issue again
  await expect(dialog.getByLabel('Title (required)')).toHaveValue('');
  await expect(dialog.getByRole('button', { name: 'Create', exact: true })).toBeDisabled();
  expect(await count(request, 'E2E refused field')).toBe(1);
  await dialog.getByRole('button', { name: 'Open it to set them' }).click();
  await expect(page.getByRole('dialog', { name: /New issue/ })).toHaveCount(0);
  await expect(page.locator('.gb-panel .gb-head-title')).toHaveText('E2E refused field');
  expect(errs).toEqual([]);
});

test('j/k go on after the open issue is closed out of the view', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  const titles = ['E2E nav one', 'E2E nav two', 'E2E nav three'];
  const ids: string[] = [];
  for (const t of titles) ids.push(await issue(request, t, ['e2enav']));
  const filter = encodeURIComponent('is:open label:e2enav');
  await page.goto(`/#/?filter=${filter}&issue=${ids[1]}`);
  const panel = page.locator('.gb-panel');
  await expect(panel.locator('.gb-head-title')).toHaveText(titles[1]!);
  // closed elsewhere: the board reloads without it
  const r = await request.put(`/api/issues/${ids[1]}/status`, { data: { status: 'closed' } });
  expect(r.ok(), await r.text()).toBeTruthy();
  const board = page.locator('main');
  await expect(board.getByText(titles[0]!, { exact: true })).toBeVisible();
  await expect(board.getByText(titles[1]!, { exact: true })).toHaveCount(0);
  const prev = panel.getByRole('button', { name: 'Previous issue (k)' });
  const next = panel.getByRole('button', { name: 'Next issue (j)' });
  // two issues left around where it was: one of j and k at least goes on
  await expect(async () => expect((await prev.isEnabled()) || (await next.isEnabled())).toBe(true)).toPass();
  await ((await next.isEnabled()) ? next : prev).click();
  await expect(panel.locator('.gb-head-title')).toHaveText(new RegExp(`^(${titles[0]}|${titles[2]})$`));
  expect(errs).toEqual([]);
});
