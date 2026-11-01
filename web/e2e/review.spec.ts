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
