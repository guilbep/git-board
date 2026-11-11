// The GitHub Project on the Sync page (Settings › Sync) and where the
// fields it holds live (Settings › Fields): with [github-project] naming
// o's Project 1, which the fake GitHub of serve.mjs answers, the whole
// check reads it and the fields list says the Project's column. The
// table is taken out after.
import { expect, test } from '@playwright/test';
import { errors } from './board';

test('the GitHub Project: a check, and the fields it holds', async ({ page, request }) => {
  const errs = errors(page);
  const set = async (table: unknown) => {
    const cfg = (await (await request.get('/api/config')).json()) as { version: string };
    const r = await request.patch('/api/config', { data: { set: { 'github-project': table } }, headers: { 'If-Match': `"${cfg.version}"` } });
    expect(r.ok(), await r.text()).toBeTruthy();
  };
  await page.goto('/#/settings/sync');
  await expect(page.getByText('No GitHub Project is configured')).toBeVisible();
  await set({ owner: 'o', number: 1 });
  try {
    await page.reload();
    await expect(page.getByText('Not checked yet')).toBeVisible();
    await page.getByRole('button', { name: 'Check', exact: true }).click();
    await expect(page.locator('.pj-meta').first()).toContainText('The e2e Project');
    await expect(page.locator('.pj-meta').first()).toContainText('0 issues matched, 1 unmatched');
    await expect(page.getByRole('heading', { name: 'No conflicts' })).toBeVisible();
    await expect(page.getByText('1 item of the Project match no issue here')).toBeVisible();
    await expect(page.getByText("This board doesn't write to the Project")).toBeVisible();
    // write = true from the page, and off again: no hand edit of the file
    await page.getByRole('button', { name: 'Let the board write to the Project' }).click();
    await expect(page.getByText('The board writes its values to the Project.')).toBeVisible();
    await page.getByRole('button', { name: 'Stop writing to it' }).click();
    await expect(page.getByText("This board doesn't write to the Project")).toBeVisible();

    // status lives in the Project, whose last check found its column; prio
    // too (the Project's own columns), not found there
    await page.goto('/#/settings/fields');
    await expect(page.locator('.fl-place', { hasText: 'Project › Status' }).first()).toBeVisible();
    await expect(page.locator('.fl-place-missing', { hasText: 'Project › Priority' }).first()).toBeVisible();
  } finally {
    await set(null);
  }
  expect(errs).toEqual([]);
});

test('v1’s /conflicts address opens Settings › Sync, with the Project', async ({ page }) => {
  await page.goto('/#/conflicts');
  await expect(page).toHaveURL(/#\/settings\/sync$/);
  await expect(page.getByText('No GitHub Project is configured')).toBeVisible();
});
