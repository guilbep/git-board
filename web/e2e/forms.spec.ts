// The project file's forms keep what they don't show: a save writes its
// section's keys, and a key the form left out of its body used to go
// (serve.mjs's file holds one of each). Each test edits one thing, saves,
// and reads the file back.
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

test('Repos keeps assignees', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/repos');
  await page.getByRole('button', { name: /Add a repository/ }).click();
  await page.getByRole('textbox', { name: 'A GitHub repository' }).last().fill('o/s');
  await save(page);
  expect(boardFile()).toContain('list = ["o/r", "o/s"]');
  expect(boardFile()).toContain('assignees = "labels"');
  expect(errs).toEqual([]);
});

test('Cycle keeps test-files, criteria and plans-in-pr', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/cycle');
  await page.getByRole('textbox', { name: 'Where the plans live' }).fill('docs/plans');
  await save(page);
  const file = boardFile();
  expect(file).toContain('plans = "docs/plans"');
  expect(file).toMatch(/test-files = \["\*_test\.go"\]/);
  expect(file).toContain('criteria = ["it works"]');
  expect(file).toContain('plans-in-pr = false');
  expect(errs).toEqual([]);
});

test('Cycle turned off can be saved', async ({ page }) => {
  await page.goto('/#/settings/cycle');
  await page.getByRole('checkbox').first().uncheck();
  await save(page);
  expect(boardFile()).not.toContain('[cycle]');
});

test('Release keeps platforms, build and an explicit follow = ""', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/release');
  await page.getByRole('textbox', { name: 'The checks command' }).fill('make ci');
  await save(page);
  const file = boardFile();
  expect(file).toContain('checks = "make ci"');
  expect(file).toContain('platforms = ["linux/amd64"]');
  expect(file).toContain('build = "make dist"');
  expect(file).toContain('follow = ""');
  expect(errs).toEqual([]);
});

test('Sprints shows a sprint’s first day and saves', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/sprints');
  await expect(page.getByRole('textbox', { name: 'First day' }).or(page.getByLabel('First day')).first()).toHaveValue('2026-10-05');
  await page.getByLabel('Title').first().fill('Sprint one');
  await save(page);
  expect(boardFile()).toContain('title = "Sprint one"');
  expect(boardFile()).toMatch(/start = 2026-10-05/);
  expect(errs).toEqual([]);
});

test('Fields leaves the flat kinds flat, and is clean once saved', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/fields/prio');
  await expect(page.getByRole('button', { name: 'Discard changes' })).toHaveCount(0); // nothing edited yet
  await page.getByRole('textbox', { name: 'Add a value to prio' }).fill('p4');
  await page.keyboard.press('Enter');
  await save(page);
  const file = boardFile();
  expect(file).toContain('options = ["p0", "p1", "p2", "p3", "p4"]');
  expect(file).not.toMatch(/^\[fields\.kind\]/m);
  expect(errs).toEqual([]);
});

test('Fields reorders beside the flat kinds', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/fields/prio');
  await page.getByRole('link', { name: /^prio / }).focus();
  await page.keyboard.press('Alt+ArrowUp');
  await save(page);
  const file = boardFile();
  // the file had status, prio, est: only prio moved, the kinds don't sort it
  const at = (n: string) => file.indexOf(`[fields.${n}]`);
  expect(at('prio')).toBeLessThan(at('status'));
  expect(at('status')).toBeLessThan(at('est'));
  expect(file).not.toMatch(/^\[fields\.kind\]/m);
  expect(errs).toEqual([]);
});
