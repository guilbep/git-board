// A write from a version someone moved from (412 stale, docs/rewrite/api.md,
// Versions): nothing is written, and the page asks: reload the issue, or
// overwrite with the change.
import { expect, test, type Page } from '@playwright/test';
import { errors, identity, issue } from './board';

// rename starts editing the title: while one types, a change from
// elsewhere is offered as a reload, not applied under the cursor
async function rename(page: Page, to: string) {
  await page.locator('.gb-panel').getByRole('button', { name: 'More' }).click();
  await page.getByRole('menuitem', { name: 'Edit the title' }).click();
  await page.locator('.gb-rename input').fill(to);
}

test('a title saved over a change made meanwhile: Overwrite writes it', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  const made = await issue(request, 'E2E stale one');
  await page.goto(`/#/?filter=&issue=${made.id}`);
  const panel = page.locator('.gb-panel');
  await expect(panel.locator('.gb-head-title')).toHaveText('E2E stale one');
  await rename(page, 'E2E stale mine');
  // someone else writes the issue meanwhile
  const r = await request.patch(`/api/issues/${made.id}`, { data: { set: ['prio=p1'] } });
  expect(r.ok(), await r.text()).toBeTruthy();
  await expect(panel.getByRole('status').filter({ hasText: 'Someone else changed this issue' })).toBeVisible({ timeout: 5000 });
  await panel.getByRole('button', { name: 'Save', exact: true }).click();
  const conflict = panel.getByRole('alert').filter({ hasText: 'This issue changed meanwhile' });
  await expect(conflict).toBeVisible();
  // not written: the server still has the old title
  expect(((await (await request.get(`/api/issues/${made.id}`)).json()) as { title: string }).title).toBe('E2E stale one');
  await conflict.getByRole('button', { name: 'Overwrite' }).click();
  await expect(conflict).toHaveCount(0);
  await expect(panel.locator('.gb-head-title')).toHaveText('E2E stale mine');
  const now = (await (await request.get(`/api/issues/${made.id}`)).json()) as { title: string; labels: string[] };
  expect(now.title).toBe('E2E stale mine');
  expect(now.labels).toContain('prio:p1'); // the other change kept: field by field
  expect(errs).toEqual([]);
});

test('Reload drops the change and shows the issue as it is', async ({ page, request }) => {
  await identity(request);
  const made = await issue(request, 'E2E stale two');
  await page.goto(`/#/?filter=&issue=${made.id}`);
  const panel = page.locator('.gb-panel');
  await expect(panel.locator('.gb-head-title')).toHaveText('E2E stale two');
  await rename(page, 'E2E stale lost');
  await request.patch(`/api/issues/${made.id}`, { data: { title: 'E2E stale theirs' } });
  await panel.getByRole('button', { name: 'Save', exact: true }).click();
  const conflict = panel.getByRole('alert').filter({ hasText: 'This issue changed meanwhile' });
  await expect(conflict).toBeVisible();
  await conflict.getByRole('button', { name: 'Reload' }).click();
  await expect(conflict).toHaveCount(0);
  await expect(panel.locator('.gb-head-title')).toHaveText('E2E stale theirs');
});

test('a settings form saved over a change to the file: the save bar offers both', async ({ page, request }) => {
  await page.goto('/#/settings/roles');
  await page.getByRole('button', { name: 'Add a role' }).click();
  await page.locator('.rs-input:visible').first().fill('qa');
  await page.locator('.rs-input:visible').nth(1).fill('tester');
  // the file moves meanwhile
  const r = await request.patch('/api/config', { data: { set: { 'repos.assignees': 'native' } } });
  expect(r.ok(), await r.text()).toBeTruthy();
  await page.getByRole('button', { name: /^Save/ }).click();
  await expect(page.getByRole('alert').filter({ hasText: /changed/ }).first()).toBeVisible();
});
