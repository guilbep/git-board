// The change events (GET /api/events): what another page, a command or a
// sync writes shows here without a reload, and an open issue follows.
import { expect, test } from '@playwright/test';
import { errors, identity, issue } from './board';

test('an issue made elsewhere shows on the board, and a title changed elsewhere in its panel', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  await page.goto('/#/?filter=' + encodeURIComponent('label:e2elive'));
  await expect(page.locator('.status')).toContainText('0');
  const made = await issue(request, 'E2E live one', [], ['e2elive']);
  // no reload: the board hears the event and asks again
  await expect(page.locator('main').getByText('E2E live one', { exact: true })).toBeVisible({ timeout: 5000 });

  await page.goto(`/#/?filter=${encodeURIComponent('label:e2elive')}&issue=${made.id}`);
  const title = page.locator('.gb-panel .gb-head-title');
  await expect(title).toHaveText('E2E live one');
  const r = await request.patch(`/api/issues/${made.id}`, { data: { title: 'E2E live renamed' } });
  expect(r.ok(), await r.text()).toBeTruthy();
  await expect(title).toHaveText('E2E live renamed', { timeout: 5000 });
  await expect(page.locator('main').getByText('E2E live renamed', { exact: true })).toBeVisible();
  expect(errs).toEqual([]);
});
