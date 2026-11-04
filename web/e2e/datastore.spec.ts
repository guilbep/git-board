// A board kept elsewhere than git (serve.mjs's Markdown board,
// board.datastore = markdown:issues): Settings › Sync offers none of what
// the server refuses with 409 missing, each saying why (issue #82, ADR
// 0002, 3). On a git board, the same controls work as before.
import { expect, test } from '@playwright/test';

const md = `http://127.0.0.1:${Number(process.env.E2E_PORT ?? '18321') + 5}`;
const why = (what: string) => `${what} needs a board kept in git; this board is kept in Markdown files`;

test('a Markdown board: pull, push, fetch, ahead/behind and Start over are disabled, each saying why', async ({ page, request }) => {
  const errs: Error[] = [];
  page.on('pageerror', (e) => errs.push(e));
  await expect.poll(() => request.get(`${md}/api/config/model`).then((r) => r.ok(), () => false), { timeout: 30_000 }).toBe(true);
  expect((await (await request.get(`${md}/api/datastore`)).json()).replica).toBe(false);
  await page.goto(`${md}/#/settings/sync`);
  const origin = page.getByRole('row').filter({ has: page.getByText('origin', { exact: true }) });
  for (const [label, what] of [['Fetch', 'fetching the board from a remote'], ['Pull', 'git board pull'], ['Push', 'git board push']] as const) {
    const b = origin.getByRole('button', { name: label, exact: true });
    await expect(b).toBeDisabled();
    await expect(b).toHaveAttribute('title', why(what));
  }
  const line = origin.getByText('Not compared: the board is kept in Markdown files');
  await expect(line).toHaveAttribute('aria-disabled', 'true');
  await expect(line).toHaveAttribute('title', why('comparing the board with a remote'));
  const start = page.locator('summary').filter({ hasText: 'Start over' });
  await expect(start).toHaveAttribute('aria-disabled', 'true');
  await expect(start).toHaveAttribute('title', why('Starting over'));
  await start.click();
  await expect(page.locator('details.so')).not.toHaveAttribute('open', '');
  expect(errs).toEqual([]);
});

test('a git board: the same controls are enabled', async ({ page }) => {
  await page.goto('/#/settings/sync');
  const origin = page.getByRole('row').filter({ has: page.getByText('origin', { exact: true }) });
  for (const label of ['Fetch', 'Pull', 'Push']) {
    const b = origin.getByRole('button', { name: label, exact: true });
    await expect(b).toBeEnabled();
    await expect(b).toHaveAttribute('title', `git board ${label.toLowerCase()}`);
  }
  const start = page.locator('summary').filter({ hasText: 'Start over' });
  await expect(start).not.toHaveAttribute('aria-disabled');
  await start.click();
  await expect(page.locator('details.so')).toHaveAttribute('open', '');
});
