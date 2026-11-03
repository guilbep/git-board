// A board git board init started from its forge (serve.mjs: a clone of
// EPFL-ENAC/test, ENAC's labels and Project on the fake GitHub) agrees
// with it on the first day: Settings › Labels has nothing to adopt
// (docs/prd/board-template.md, issue #75).
import { expect, test } from '@playwright/test';

const enac = `http://127.0.0.1:${Number(process.env.E2E_PORT ?? '18321') + 4}`;

test('a board from its forge: no label differs, none to adopt', async ({ page, request }) => {
  const errs: Error[] = [];
  page.on('pageerror', (e) => errs.push(e));
  await expect.poll(() => request.get(`${enac}/api/config/model`).then((r) => r.ok(), () => false), { timeout: 30_000 }).toBe(true);
  await page.goto(`${enac}/#/settings/labels`);
  const table = page.locator('.gb-table').first();
  await expect(table.getByText('perf', { exact: true })).toBeVisible();
  await expect(table.getByText('Same', { exact: true })).toHaveCount(12);
  await expect(table.getByText('In the Project').first()).toBeVisible();
  await expect(page.getByText('Color differs')).toHaveCount(0);
  await expect(page.getByText('Description differs')).toHaveCount(0);
  await expect(page.getByText('Missing', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Adopt' })).toHaveCount(0);
  expect(errs).toEqual([]);
});
