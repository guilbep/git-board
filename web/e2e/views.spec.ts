// A view saved from the board's bar (ViewBar): the filter as typed, a new
// view in .git-board.toml, its tab, and the board on it.
import { expect, test } from '@playwright/test';
import { boardFile, errors } from './board';

test('Save as new view writes its table, and its tab opens it', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/');
  const box = page.getByRole('combobox', { name: 'Filter' });
  await box.fill('is:open prio:p1');
  const bar = page.getByRole('region', { name: 'Unsaved changes to the view' });
  await expect(bar).toBeVisible();
  await bar.getByRole('button', { name: 'Save as new view…' }).click();
  await bar.getByLabel('New view name').fill('e2e-urgent');
  await bar.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(bar).toHaveCount(0);
  await expect(page).toHaveURL(/view=e2e-urgent/);
  await expect(page.getByRole('navigation', { name: 'Views' }).getByRole('link', { name: 'E2e urgent' })).toHaveAttribute('aria-current', 'page');
  expect(boardFile()).toMatch(/\[views\.e2e-urgent\]\nfilter = "is:open prio:p1"\nlayout = "table"/);
  expect(errs).toEqual([]);
});
