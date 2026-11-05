// The UI review's fixes (docs/prd/ui-review.md), one test each where the
// page shows the change: a count, a word, a state. (D-23, Labels' lint, is
// git board lint's, a tool v2 serves with M6.)
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

test('C-22, D-33: each field says where it lives, the forge named once, in words', async ({ page }) => {
  await page.goto('/#/settings/fields/status');
  // the list: a place per forge on each field
  await expect(page.locator('.fl-row', { hasText: 'status' }).first().locator('.fl-place')).toHaveText(/Label/);
  // the editor: the forge by its kind, once
  await expect(page.locator('.fp-forge')).toHaveCount(1);
  await expect(page.locator('.fp-forge')).toHaveText('GitHub');
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

test('D-39: a half-typed term neither filters the board nor offers to save the view', async ({ page }) => {
  await page.goto('/#/');
  const box = page.getByRole('combobox', { name: 'Filter' });
  await box.click();
  await page.keyboard.press('End');
  await page.keyboard.type(' prio:');
  await page.waitForTimeout(500);
  const bar = page.getByRole('region', { name: 'Unsaved changes to the view' });
  await expect(bar).toHaveCount(0);
  await page.keyboard.type('p1');
  await expect(bar).toBeVisible();
});

test('D-42: a field preset already declared opens that field', async ({ page }) => {
  await page.goto('/#/settings/fields/prio');
  await page.getByRole('button', { name: '+ Add a field' }).click();
  await page.getByRole('menuitem', { name: /status · already declared/ }).click();
  await expect(page).toHaveURL(/\/settings\/fields\/status$/);
  await expect(page.locator('.fl-nm', { hasText: /^status_2$|^status-2$/ })).toHaveCount(0);
});
