// The Work page (experimental, the cycle's; GET /api/cycle): every open
// issue's step, what needs you and what waits on others; hidden, and
// saying how to turn it on, when the cycle is off.
import { expect, test } from '@playwright/test';
import { errors, identity, issue } from './board';

test('Work lists the open issues by their step, Mine and Team', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  const made = await issue(request, 'E2E work waiting', ['status=ready']);
  const c = await request.get(`/api/issues/${made.id}/cycle`);
  expect(c.ok(), await c.text()).toBeTruthy();
  const cycle = (await c.json()) as { step: number; name: string; done: boolean };
  expect(cycle.step).toBeGreaterThan(0);

  await page.goto('/#/');
  await page.getByRole('link', { name: 'Work', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'What needs a person' })).toBeVisible();
  // every open issue's evidence is read: a while on a busy machine
  await expect(page.getByRole('heading', { name: /^Needs you/ })).toBeVisible({ timeout: 30_000 });
  await page.getByRole('button', { name: 'Team' }).click();
  await expect(page).toHaveURL(/who=team/);
  // a group of Needs you shows five rows until opened: the other tests' issues are many
  await expect(page.getByRole('heading', { name: /^Waiting on others/ })).toBeVisible({ timeout: 30_000 });
  for (const more of await page.getByRole('button', { name: /^Show \d+ more$/ }).all()) await more.click();
  const row = page.locator('main tr', { hasText: 'E2E work waiting' });
  await expect(row).toBeVisible();
  // its issue opens with its cycle, read at once; History draws its steps
  const read = page.waitForResponse((r) => r.url().endsWith(`/api/issues/${made.id}/cycle`) && r.ok());
  await row.getByRole('link', { name: /E2E work waiting/ }).click();
  await expect(page).toHaveURL(new RegExp(`/issues/${made.id}`));
  await read;
  await expect(page.getByRole('region', { name: 'The cycle' })).toBeVisible();
  await page.getByRole('tab', { name: /^History/ }).click();
  expect(errs).toEqual([]);
});

test('with the cycle off, Work is hidden and says how to turn it on', async ({ page, request }) => {
  const errs = errors(page);
  const before = (await (await request.get('/api/config')).json()) as { text: string; version: string };
  const off = await request.patch('/api/config', { data: { set: { 'experimental.features': [] } }, headers: { 'If-Match': `"${before.version}"` } });
  expect(off.ok(), await off.text()).toBeTruthy();
  try {
    await page.goto('/#/');
    await expect(page.getByRole('link', { name: 'Insights' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Work', exact: true })).toHaveCount(0);
    await page.goto('/#/work');
    await expect(page.getByRole('link', { name: 'Turn it on in Settings › Experimental' })).toBeVisible();
  } finally {
    const now = (await (await request.get('/api/config')).json()) as { version: string };
    const back = await request.put('/api/config', { data: { text: before.text }, headers: { 'If-Match': `"${now.version}"` } });
    expect(back.ok(), await back.text()).toBeTruthy();
  }
  expect(errs).toEqual([]);
});
