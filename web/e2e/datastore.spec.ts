// A board kept elsewhere than git (serve.mjs's Markdown board,
// board.datastore = markdown:issues): Settings › Sync offers none of what
// the server refuses with 409 missing, each saying why (issue #82, ADR
// 0002, 3). On a git board, the same controls work as before. (Start over
// is the CLI's in v2: git board reset.)
import { expect, test } from '@playwright/test';
import { markdownBoard } from './board';

const md = markdownBoard;
const why = (what: string) => `${what} needs a board kept in git; this board is kept in Markdown files`;

test('a Markdown board: pull, push, fetch and ahead/behind are disabled, each saying why', async ({ page, request }) => {
  const errs: Error[] = [];
  page.on('pageerror', (e) => errs.push(e));
  await expect.poll(() => request.get(`${md}/api/board`).then((r) => r.ok(), () => false), { timeout: 30_000 }).toBe(true);
  expect((await (await request.get(`${md}/api/datastore`)).json()).replica).toBe(false);
  await page.goto(`${md}/#/settings/sync`);
  const origin = page.getByRole('row').filter({ has: page.getByText('origin', { exact: true }) });
  for (const [label, what] of [['Fetch', 'fetching the board from a remote'], ['Pull', 'pulling the board from a remote'], ['Push', 'pushing the board to a remote']] as const) {
    const b = origin.getByRole('button', { name: label, exact: true });
    await expect(b).toBeDisabled();
    await expect(b).toHaveAttribute('title', why(what));
  }
  const line = origin.getByText('Not compared: the board is kept in Markdown files');
  await expect(line).toHaveAttribute('aria-disabled', 'true');
  await expect(line).toHaveAttribute('title', why('comparing the board with a remote'));
  expect(errs).toEqual([]);
});

test('a git board: the same controls are enabled', async ({ page }) => {
  await page.goto('/#/settings/sync');
  const origin = page.getByRole('row').filter({ has: page.getByText('origin', { exact: true }) });
  // each says what it does, never a command (the web app names no terminal)
  for (const [label, title] of [
    ['Fetch', 'Compare the board with this remote: nothing changes here'],
    ['Pull', 'Take this remote’s changes into the board'],
    ['Push', 'Send the board’s changes to this remote'],
  ] as const) {
    const b = origin.getByRole('button', { name: label, exact: true });
    await expect(b).toBeEnabled();
    await expect(b).toHaveAttribute('title', title);
  }
});
