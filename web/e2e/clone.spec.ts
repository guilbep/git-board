// A clone shows its board (docs/prd/clone-shows-board.md): the clone
// board, serve.mjs's second, is a fresh clone of a local origin holding two
// issues; the e2e board's origin can't be read.
import { expect, test, type Page } from '@playwright/test';

const clone = `http://127.0.0.1:${Number(process.env.E2E_PORT ?? '18321') + 2}`;

function errors(page: Page): Error[] {
  const errs: Error[] = [];
  page.on('pageerror', (e) => errs.push(e));
  return errs;
}

test('a fresh clone opens on the issues pulled from origin, and says so', async ({ page }) => {
  const errs = errors(page);
  await page.goto(`${clone}/#/`);
  const note = page.getByRole('status').filter({ hasText: 'This clone had no board' });
  await expect(note).toContainText('2 issues pulled from origin');
  await expect(page.getByText('Seeded alpha').filter({ visible: true })).toBeVisible();
  await expect(page.getByText('Seeded beta').filter({ visible: true })).toBeVisible();
  await note.getByRole('button', { name: 'Hide' }).click();
  await expect(note).toHaveCount(0);
  expect(errs).toEqual([]);
});

test('an empty board whose origin can’t be read says so', async ({ page }) => {
  await page.goto('/#/');
  await expect(page.getByRole('status').filter({ hasText: 'This clone has no board yet' })).toContainText("origin can't be read");
});
