// A clone shows its board (docs/prd/clone-shows-board.md): the clone
// board, serve.mjs's on E2E_PORT + 3, is a fresh clone of a local origin
// holding two issues; the e2e board's origin can't be read.
import { expect, test, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const clone = `http://127.0.0.1:${Number(process.env.E2E_PORT ?? '18421') + 3}`;

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
  await expect(page.getByText('Seeded alpha').filter({ visible: true }).first()).toBeVisible();
  await expect(page.getByText('Seeded beta').filter({ visible: true }).first()).toBeVisible();
  await note.getByRole('button', { name: 'Hide' }).click();
  await expect(note).toHaveCount(0);
  expect(errs).toEqual([]);
});

test('an empty board whose origin can’t be read says so', async ({ page }) => {
  await page.goto('/#/');
  await expect(page.getByRole('status').filter({ hasText: 'This clone has no board yet' })).toContainText("origin can't be read");
});

// Add a forge takes a token again (api.md, 33): its table in the file,
// the token given to git's credential helper, never written there. On the
// clone board, whose origin names no forge: the e2e board's found one
// stays as the other tests find it.
test('Add a forge writes its table, and the token goes to the credential helper', async ({ page }) => {
  const errs = errors(page);
  await page.goto(`${clone}/#/settings/sync`);
  await page.getByRole('button', { name: '+ Add a forge' }).click();
  await page.getByRole('button', { name: 'Next' }).click();
  await page.getByLabel('The project’s address on GitHub').or(page.getByLabel("The project's address on GitHub")).fill('https://github.com/o/clone');
  await page.getByLabel(/A token/).fill('e2e-secret-token');
  await page.getByRole('button', { name: 'Add the forge' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'is added to' })).toContainText('gh is added to .git-board.toml');
  const file = readFileSync(join(process.env.E2E_DIR!, 'clone', '.git-board.toml'), 'utf8');
  expect(file).toContain('[forges.gh]');
  expect(file).toContain('project = "o/clone"');
  expect(file).not.toContain('e2e-secret-token');
  expect(errs).toEqual([]);
});
