// A board's lanes are lists of their own, paged (boardlists.ts): each
// counts every issue it holds, shows its first page of cards, and the
// next as its end comes near.
import { expect, test } from '@playwright/test';
import { errors, identity, issue } from './board';

// fresh: a label of this run's own, for its issues alone: the board is the
// whole run's, and --repeat-each or a retry would count the last run's too
const fresh = (base: string): string => `${base}${Date.now().toString(36)}`;

test('a lane counts its issues, shows 40 cards, and the rest on demand', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  const label = fresh('e2elane');
  for (let i = 0; i < 45; i++) await issue(request, `E2E lane ${i}`, ['status=ready'], [label]);
  await issue(request, 'E2E lane backlog', ['status=backlog'], [label]);
  await page.goto(`/#/?filter=${encodeURIComponent(`label:${label}`)}`);
  // the switch to a board groups it by status
  await page.getByRole('group', { name: 'Layout' }).getByRole('button', { name: 'Board' }).click();
  // a lane by its name: the cards go by id, so which 40 come first, and
  // whether "E2E lane 0" is among them, changes from run to run
  const ready = page.getByRole('region', { name: /^ready, / });
  await expect(ready.locator('.column-count')).toHaveText('45');
  await expect(page.getByRole('region', { name: /^backlog, / }).locator('.column-count')).toHaveText('1');
  // the first page, then the rest as the lane's end comes near
  await expect(ready.locator('.issue-card')).toHaveCount(40);
  await expect(ready.locator('.cards-more')).toHaveText('5 more');
  await ready.locator('.cards-more').scrollIntoViewIfNeeded();
  await expect(ready.locator('.issue-card')).toHaveCount(45);
  await expect(ready.locator('.cards-more')).toHaveCount(0);
  await expect(page.locator('.status')).toContainText('46');
  expect(errs).toEqual([]);
});

test('a table shows its first 100 rows, and the next page as its end comes near', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  const label = fresh('e2etable');
  for (let i = 0; i < 105; i++) await issue(request, `E2E row ${i}`, [], [label]);
  await page.goto(`/#/?filter=${encodeURIComponent(`label:${label}`)}`);
  const rows = page.locator('table.issue-table tbody tr:not(.row-spacer):not(.row-more)');
  await expect(page.locator('.group-count, .status').first()).toContainText('105');
  await expect(rows).toHaveCount(100);
  await expect(page.locator('.row-more')).toContainText('5 more');
  await page.locator('.row-more').scrollIntoViewIfNeeded();
  await expect(rows).toHaveCount(105);
  await expect(page.locator('.row-more')).toHaveCount(0);
  expect(errs).toEqual([]);
});
