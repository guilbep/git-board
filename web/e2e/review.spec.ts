// The UI review's fixes (docs/prd/ui-review.md), one test each where the
// page shows the change: a count, a word, a state.
import { expect, test, type Page } from '@playwright/test';

function errors(page: Page): Error[] {
  const out: Error[] = [];
  page.on('pageerror', (e) => out.push(e));
  return out;
}

test('D-44: a focused field is outlined in the text color, not the error red', async ({ page }) => {
  // (the e2e board has no identity, so no form opens: the kit's own input class)
  await page.goto('/#/');
  const [outline, text] = await page.evaluate(() => {
    const e = document.createElement('input');
    e.className = 'epfl-input';
    document.body.append(e);
    e.focus();
    return [getComputedStyle(e).outlineColor, getComputedStyle(document.body).color];
  });
  expect(outline).toBe(text);
});

test('B-11: Attachments leads with the server switch, the file second', async ({ page }) => {
  await page.goto('/#/settings/attachments');
  await expect(page.getByRole('status').filter({ hasText: 'Off on this server' })).toBeVisible();
  await expect(page.getByText('no key set: the defaults')).toBeVisible();
});

test('B-10: the Release summary counts the binaries', async ({ page }) => {
  await page.goto('/#/settings/release');
  await expect(page.getByText('2 environments · 0 images · 1 binary')).toBeVisible();
});

test('C-22, D-33: Forge fields says the kind once, each field in words', async ({ page }) => {
  await page.goto('/#/settings/forge-fields/github');
  await expect(page.getByRole('heading', { name: 'GitHub · o/r' })).toBeVisible();
  await expect(page.getByRole('row', { name: /status/ }).getByText('values · one per issue')).toBeVisible();
  await expect(page.getByRole('row', { name: /due/ }).getByText('a date · several per issue')).toBeVisible();
});

test('D-26: a disabled Save says why', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/roles');
  const save = page.getByRole('button', { name: /^Save/ });
  await expect(save).toBeDisabled();
  // (every settings tab stays mounted: the visible one's)
  await expect(page.locator('.csb-why:visible')).toHaveText('No change to save');
  expect(errs).toEqual([]);
});

test('C-20: Settings opens on Sync, each menu item says where it stands', async ({ page }) => {
  await page.goto('/#/settings');
  await expect(page.getByRole('heading', { name: 'Sync', exact: true })).toBeVisible();
  await expect(page.locator('.set-state').filter({ hasText: /origin/ })).toBeVisible();
  await expect(page.getByRole('link', { name: /^Overview/ })).toHaveCount(0);
});

test('C-16: the reference beside a form drops its pointer back to the form', async ({ page }) => {
  await page.goto('/#/settings/fields');
  await expect(page.locator('.crp:visible')).toContainText('options');
  await expect(page.getByText(/These are the forms of Settings/)).toHaveCount(0);
});

test('C-21, B-15: the file tab is "The file", saved like the others, no quick-add buttons', async ({ page }) => {
  await page.goto('/#/settings/advanced');
  await expect(page.getByRole('heading', { name: 'The file' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Save…' })).toBeVisible();
  await expect(page.getByRole('button', { name: '+ View' })).toHaveCount(0);
});

test('C-43: Add a forge names the forge the remotes already give', async ({ page }) => {
  await page.goto('/#/settings/sync');
  await page.getByRole('button', { name: '+ Add a forge' }).click();
  await expect(page.getByText(/Already found in your git remotes/)).toContainText('GitHub · o/r');
  await expect(page.getByRole('heading', { name: 'How forges work' })).toHaveCount(0);
});

test('C-34: one way to the command palette, the header’s', async ({ page }) => {
  await page.goto('/#/');
  await expect(page.getByRole('button', { name: 'Filter or jump to' })).toHaveCount(1);
  await expect(page.getByRole('button', { name: /Filter or jump ⌘K|Filter or jump Ctrl/ })).toHaveCount(0);
});

test('D-23: Labels in three parts, lint a line per field', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/labels');
  await expect(page.getByRole('button', { name: 'Add a label' })).toBeVisible();
  await page.getByRole('tab', { name: 'Lint' }).click();
  await expect(page.getByRole('button', { name: 'Add a label' })).toBeHidden();
  // the e2e board has no issue: every option is dead, one line per field
  await expect(page.getByText('status: no issue carries backlog, ready, in-progress, done')).toBeVisible();
  await page.getByRole('tab', { name: 'On GitHub' }).click();
  await expect(page.getByRole('heading', { name: /outside the project's labels/ })).toBeVisible();
  expect(errs).toEqual([]);
});
