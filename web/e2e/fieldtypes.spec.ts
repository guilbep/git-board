// The Fields editor's form follows the field's type: a select or a number
// lists its values, a date, a person, an issue or a sprint has none of its
// own, and a save takes out the values such a field had in the file.
import { expect, test, type Page } from '@playwright/test';
import { boardFile } from './board';

function errors(page: Page): Error[] {
  const out: Error[] = [];
  page.on('pageerror', (e) => out.push(e));
  return out;
}

async function save(page: Page) {
  await page.getByRole('button', { name: /^Save/ }).click();
  await expect(page.getByRole('button', { name: 'Discard changes' })).toHaveCount(0);
}

// the [fields.<name>] table, up to the next header
const table = (name: string) => new RegExp(`^\\[fields\\.${name}\\]\\n((?:[^\\[].*\\n|\\n)*)`, 'm').exec(boardFile())?.[1] ?? '';

test('a select lists its values, a date, a person, an issue and a sprint do not', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/fields/prio');
  await expect(page.getByRole('textbox', { name: 'Add a value to prio' })).toBeVisible();
  await expect(page.getByText('Per-value details')).toBeVisible();

  await page.goto('/#/settings/fields/due');
  await expect(page.getByText(/A date per issue/)).toBeVisible();
  await expect(page.getByRole('textbox', { name: 'Add a value to due' })).toHaveCount(0);
  await expect(page.getByText('Per-value details')).toHaveCount(0);

  await page.goto('/#/settings/fields/sprint');
  await expect(page.getByText(/A sprint: the values are the sprints/)).toBeVisible();
  await expect(page.getByRole('link', { name: 'Sprints', exact: true }).last()).toHaveAttribute('href', '#/settings/sprints');
  await expect(page.getByRole('textbox', { name: 'Add a value to sprint' })).toHaveCount(0);

  await page.getByRole('combobox', { name: 'Type' }).selectOption('person');
  await expect(page.getByText(/Forge logins, @me/)).toBeVisible();
  await page.getByRole('combobox', { name: 'Type' }).selectOption('issue');
  await expect(page.getByText(/Another issue, #12/)).toBeVisible();
  expect(errs).toEqual([]);
});

test('a select turned into an issue drops its values and their colors on save', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/fields/size');
  await page.getByRole('combobox', { name: 'Type' }).selectOption('issue');
  await expect(page.getByText(/saving takes them out/)).toBeVisible();
  await save(page);
  const t = table('size');
  expect(t).toContain('type = "issue"');
  expect(t).not.toContain('options');
  expect(boardFile()).not.toContain('[fields.size.colors]');
  expect(boardFile()).not.toMatch(/colors = \{ s = "green" \}/);
  expect(errs).toEqual([]);
});

test('a date known by its values alone keeps its type once the values go', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/fields/when');
  await expect(page.getByText(/A date per issue/)).toBeVisible();
  await page.getByRole('radio', { name: 'one value' }).check();
  await save(page);
  const t = table('when');
  expect(t).toContain('type = "date"');
  expect(t).toContain('single = true');
  expect(t).not.toContain('options');
  expect(errs).toEqual([]);
});
