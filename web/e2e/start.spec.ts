// A new board's first run (docs/design/first-run/proposal.md): the new
// board, serve.mjs's on E2E_PORT + 6, has no file, no origin, no identity
// and no issue. Its empty state offers the Start page, whose steps write
// through the API and never show a terminal command.
import { expect, test, type Page } from '@playwright/test';

const board = `http://127.0.0.1:${Number(process.env.E2E_PORT ?? '18421') + 6}`;

function errors(page: Page): Error[] {
  const errs: Error[] = [];
  page.on('pageerror', (e) => errs.push(e));
  return errs;
}

test('a new board offers the Start page, and its steps move the board on', async ({ page, request }) => {
  const errs = errors(page);
  const before = await (await request.get(`${board}/api/state`)).json();
  expect(before.phase).toBe('new');
  expect(before.next.map((a: { id: string }) => a.id)).toEqual(['me', 'forge']);

  await page.goto(`${board}/#/`);
  await expect(page.getByText('This board is new.')).toBeVisible();
  await page.getByRole('link', { name: 'Set up this board' }).click();
  await expect(page.getByRole('heading', { name: 'Set up this board' })).toBeVisible();

  // 1. who you are: created here, then named
  await page.getByLabel('Name').fill('Ada');
  await page.getByRole('button', { name: 'Create and use' }).click();
  await expect(page.getByTestId('me-done')).toContainText('Ada');

  // the file: none yet, written on demand
  await page.getByRole('button', { name: 'Write the default file now' }).click();
  await expect(page.getByRole('button', { name: 'Write the default file now' })).toHaveCount(0);

  // 2. a goal shows what it does; with no forge, it adds one
  await page.getByText('Keep a backup on another forge').click();
  await expect(page.getByTestId('goal-panel').getByRole('button', { name: 'Add the backup forge' })).toBeVisible();
  await page.getByText('Keep the issues as Markdown files').click();
  await expect(page.getByTestId('goal-panel').getByRole('link', { name: 'Open Settings › Datastore' })).toBeVisible();

  // no command anywhere on the page
  await expect(page.locator('main')).not.toContainText('git board');

  const after = await (await request.get(`${board}/api/state`)).json();
  expect(after.me.state).toBe('chosen');
  expect(after.file.state).not.toBe('none');
  expect(after.next.map((a: { id: string }) => a.id)).toEqual(['forge', 'new']);

  await page.getByTestId('open-board').click();
  await expect(page).toHaveURL(/#\/$/);
  expect(errs).toEqual([]);
});
