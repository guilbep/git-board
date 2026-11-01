// Settings regressions, on a board whose forge is found in the git remotes
// (no [forges] table): the model once answered "forges": null, which broke
// the Fields page, and the Forge fields page said "No forge declared".
import { expect, test, type Page } from '@playwright/test';

// errors collects the page's uncaught errors: a page that throws is broken
// even when it shows something
function errors(page: Page): Error[] {
  const out: Error[] = [];
  page.on('pageerror', (e) => out.push(e));
  return out;
}

test('a field of Fields opens its editor', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/fields/prio');
  await expect(page.getByRole('textbox', { name: "The field's name" })).toHaveValue('prio');
  await expect(page.getByRole('button', { name: 'Remove p0' })).toBeVisible();
  // the forge found in the remotes is named, its row a link away
  await expect(page.locator('a[href="#/settings/forge-fields/github"]').first()).toBeVisible();
  expect(errs).toEqual([]);
});

test('Sync is in the project file group, with each forge’s fields a link away', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/sync');
  const nav = page.getByRole('navigation', { name: 'Settings' });
  await expect(nav.locator('.set-group').first()).toHaveText('The project file');
  await expect(nav.locator('.set-group').first().locator('+ a')).toContainText('Sync');
  await page.getByRole('link', { name: 'its fields' }).click();
  await expect(page).toHaveURL(/#\/settings\/forge-fields\/github$/);
  expect(errs).toEqual([]);
});

test('Forge fields shows the forge found in the remotes, and a save writes its table', async ({ page, request }) => {
  const errs = errors(page);
  await page.goto('/#/settings/forge-fields');
  await expect(page.getByText('No forge')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'github · github · o/r' })).toBeVisible();
  await expect(page.getByText('found in the git remotes')).toBeVisible();

  // one override, saved: the file gets the forge's table and the mapping
  const select = page.getByLabel('prio lives in');
  const options = await select.locator('option').evaluateAll((os) => os.map((o) => (o as HTMLOptionElement).value));
  const current = await select.inputValue();
  const other = options.find((v) => v !== current && v !== 'project');
  expect(other, `a target other than ${current} among ${options}`).toBeTruthy();
  await select.selectOption(other!);
  await page.getByRole('button', { name: /^Save/ }).click();
  await expect(page.getByText('found in the git remotes')).toHaveCount(0);

  const model = await (await request.get('/api/config/model')).json();
  expect(model.forges).toHaveLength(1);
  expect(model.forges[0]).toMatchObject({ name: 'github', kind: 'github', project: 'o/r' });
  expect(model.forges[0].discovered).toBeFalsy();
  expect(model.forges[0].fields?.prio?.to).toBe(other);
  expect(model.error ?? '').toBe('');
  expect(errs).toEqual([]);
});
