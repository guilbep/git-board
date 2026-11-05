// The Markdown board (serve.mjs: board.datastore = markdown:issues): the
// same pages on a board kept in files. An issue made from the board, its
// fields set, a comment, closed: each write lands in its file, and the
// board shows it.
import { expect, test } from '@playwright/test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { errors } from './board';

const md = `http://127.0.0.1:${Number(process.env.E2E_PORT ?? '18421') + 5}`;
const files = () => readdirSync(join(process.env.E2E_DIR ?? '', 'markdown', 'issues')).filter((f) => f.endsWith('.md'));

test('a Markdown board: an issue made, set, commented and closed, in its file', async ({ page, request }) => {
  const errs = errors(page);
  if (!((await (await request.get(`${md}/api/me`)).json()) as { me: unknown }).me) {
    const r = await request.put(`${md}/api/me`, { data: { name: 'E2E', email: 'e2e@example.com' } });
    expect(r.ok(), await r.text()).toBeTruthy();
  }
  await page.goto(`${md}/#/`);
  await page.getByRole('button', { name: /New issue/ }).first().click();
  const dialog = page.getByRole('dialog', { name: /New issue/ });
  await dialog.getByLabel('Title (required)').fill('E2E kept in a file');
  await dialog.getByRole('button', { name: 'Create', exact: true }).click();
  const panel = page.locator('.gb-panel');
  await expect(panel.locator('.gb-head-title')).toHaveText('E2E kept in a file');

  // a field from the head, a comment, then Close with comment
  await panel.locator('.gb-stepper-box').click();
  await page.getByRole('menuitemradio', { name: /todo/i }).click();
  await expect(panel.locator('.gb-hline')).toContainText('todo');
  const box = panel.getByPlaceholder('Leave a comment');
  await box.fill('Written in Markdown.');
  await panel.getByRole('button', { name: 'Close with comment' }).click();
  await expect(panel.getByText('Written in Markdown.')).toBeVisible();
  await expect(panel.locator('.gb-ptop')).toContainText('Closed');

  // the file says it all
  const file = files().map((f) => readFileSync(join(process.env.E2E_DIR ?? '', 'markdown', 'issues', f), 'utf8')).find((t) => t.includes('E2E kept in a file'));
  expect(file).toBeTruthy();
  expect(file).toContain('Written in Markdown.');
  expect(file).toContain('state: closed');
  expect(file).not.toContain('closed-as'); // closed as completed
  expect(file).toMatch(/status:\n\s+- done/); // closing takes the status to its last option
  expect(errs).toEqual([]);
});
