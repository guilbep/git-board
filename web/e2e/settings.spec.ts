// Settings regressions, on a board whose forge is found in the git remotes
// (no [forges] table): the model once answered "forges": null, which broke
// the Fields page, and the forge's places said "No forge declared".
import { expect, test, type Page } from '@playwright/test';
import { boardFile } from './board';

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
  // the forge found in the remotes has its row in Where it lives
  await expect(page.getByLabel('prio lives on GitHub in')).toBeVisible();
  expect(errs).toEqual([]);
});

test('Sync is in the project file group; each forge says where its fields live, and links to Fields', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/sync');
  const nav = page.getByRole('navigation', { name: 'Settings' });
  await expect(nav.locator('.set-group').first()).toHaveText('The project file');
  await expect(nav.locator('.set-group').first().locator('+ a')).toContainText('Sync');
  await expect(page.getByText(/fields: .* as labels/).first()).toBeVisible();
  await page.getByRole('link', { name: 'change it in Fields' }).first().click();
  await expect(page).toHaveURL(/#\/settings\/fields/);
  // an old link to Forge fields opens Fields
  await page.goto('/#/settings/forge-fields/github');
  await expect(page.locator('.set-tab[aria-current="page"]')).toContainText('Fields');
  expect(errs).toEqual([]);
});

test('a place on a forge found in the remotes, saved with its field, writes the forge’s table', async ({ page, request }) => {
  const errs = errors(page);
  await page.goto('/#/settings/fields/prio');
  // one override, saved: the file gets the forge's table and the mapping
  const select = page.getByLabel('prio lives on GitHub in');
  await expect(select.locator('option')).not.toHaveCount(0); // the targets come with the model
  const options = await select.locator('option').evaluateAll((os) => os.map((o) => (o as HTMLOptionElement).value));
  const current = await select.inputValue();
  const other = options.find((v) => v !== current && v !== 'project');
  expect(other, `a target other than ${current} among ${options}`).toBeTruthy();
  await select.selectOption(other!);
  await page.getByRole('button', { name: /^Save/ }).click();
  await expect(page.getByRole('button', { name: 'Discard changes' })).toHaveCount(0);

  const model = await (await request.get('/api/config/model')).json();
  expect(model.forges).toHaveLength(1);
  expect(model.forges[0]).toMatchObject({ name: 'github', kind: 'github', project: 'o/r' });
  expect(model.forges[0].discovered).toBeFalsy();
  expect(model.forges[0].fields?.prio?.to).toBe(other);
  expect(model.error ?? '').toBe('');
  expect(errs).toEqual([]);
});

test('a Project column saved with its field, and the default again removes the override', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/fields/est');
  await page.getByLabel('est lives on GitHub in').selectOption('project');
  await expect(page.getByLabel('The Project column of est on GitHub')).toHaveValue('Est. hours'); // the draft's target shows its details
  await page.getByRole('button', { name: /^Save/ }).click();
  await expect(page.getByRole('button', { name: 'Discard changes' })).toHaveCount(0);
  expect(boardFile()).toMatch(/est = \{ to = "project", field = "Est\. hours" \}/);
  expect(boardFile()).toMatch(/\[fields\.est\]/);
  // a move is never run by the save: it waits for its button
  await expect(page.getByRole('button', { name: 'Run the migration' })).toBeVisible();

  await page.getByLabel('est lives on GitHub in').selectOption('label');
  await page.getByRole('button', { name: /^Save/ }).click();
  await expect(page.getByRole('button', { name: 'Discard changes' })).toHaveCount(0);
  expect(boardFile()).not.toMatch(/^est = /m);
  expect(errs).toEqual([]);
});
