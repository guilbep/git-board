// Review fixes the browser shows: a new issue a field refused is not
// created, and the dialog says which field; j/k go on after the open issue
// leaves the view.
import { expect, test } from '@playwright/test';
import { errors, identity, issue } from './board';

async function count(request: import('@playwright/test').APIRequestContext, title: string): Promise<number> {
  const r = await request.get(`/api/issues?${new URLSearchParams({ filter: `"${title}"`, limit: '0' })}`);
  return ((await r.json()) as { items: { title: string }[] }).items.filter((i) => i.title === title).length;
}

test('a new issue with a refused field is not created, and the dialog says which field', async ({ page, request }) => {
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
  // the server refuses the field: the issue and its fields are one request
  await page.route('**/api/issues', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ code: 'invalid', message: 'size: refused here', field: 'set' }) })
      : route.fallback(),
  );
  await dialog.getByRole('button', { name: 'Create', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('size: refused here');
  await page.unroute('**/api/issues');
  // nothing was created, and the form keeps what was typed, to fix
  expect(await count(request, 'E2E refused field')).toBe(0);
  await expect(dialog.getByLabel('Title (required)')).toHaveValue('E2E refused field');
  await dialog.getByLabel('Size', { exact: true }).fill('');
  await dialog.getByRole('button', { name: 'Create', exact: true }).click();
  await expect(page.locator('.gb-panel .gb-head-title')).toHaveText('E2E refused field');
  expect(await count(request, 'E2E refused field')).toBe(1);
  expect(errs).toEqual([]);
});

test('j/k go on after the open issue is closed out of the view', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  const titles = ['E2E nav one', 'E2E nav two', 'E2E nav three'];
  const made = [];
  for (const t of titles) made.push(await issue(request, t, [], ['e2enav']));
  const filter = encodeURIComponent('is:open label:e2enav');
  await page.goto(`/#/?filter=${filter}&issue=${made[1]!.id}`);
  const panel = page.locator('.gb-panel');
  await expect(panel.locator('.gb-head-title')).toHaveText(titles[1]!);
  // closed elsewhere: the board hears it (the change events) and reloads
  const r = await request.patch(`/api/issues/${made[1]!.id}`, { data: { status: 'closed' } });
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
