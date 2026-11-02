// The filters' help (docs/prd/header-help.md, 5): the ?s explain
// filtering, and the Filter box offers examples and completions.
import { expect, test } from '@playwright/test';

const how = (page: import('@playwright/test').Page) => page.getByRole('dialog', { name: 'How filtering works' });

test('the panel’s ? explains filtering, not every shortcut; Try writes the word', async ({ page }) => {
  await page.goto('/#/');
  await page.getByRole('complementary', { name: 'Filters' }).getByRole('button', { name: 'How filtering works' }).click();
  await expect(how(page)).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Keyboard shortcuts' })).toHaveCount(0);
  await expect(how(page).getByText('-status:done')).toBeVisible();
  await how(page).getByRole('button', { name: 'Try is:closed' }).click();
  await expect(how(page)).toBeHidden();
  await expect(page.locator('#filter')).toHaveValue(/is:closed/);
  await expect(page.locator('#filter')).toBeFocused();
});

test('the Ticks tab says the modifier as this keyboard does', async ({ page }) => {
  await page.goto('/#/');
  await page.getByRole('complementary', { name: 'Filters' }).getByRole('button', { name: 'How filtering works' }).click();
  await how(page).getByRole('tab', { name: 'Ticks' }).click();
  await expect(how(page).getByText('Shift')).toBeVisible();
  await expect(how(page).locator('kbd', { hasText: /^(Alt|⌥)$/ })).toHaveCount(1);
});

test('the Filter box and the folded rail have the same ?', async ({ page }) => {
  await page.goto('/#/');
  await page.locator('.fbox').getByRole('button', { name: 'How filtering works' }).click();
  await expect(how(page)).toBeVisible();
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Hide filters' }).click();
  await page.locator('.filters-rail').getByRole('button', { name: 'How filtering works' }).click();
  await expect(how(page)).toBeVisible();
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: /Show filters/ }).click();
});

test('the empty Filter box offers examples; ↓ ↵ takes one', async ({ page }) => {
  await page.goto('/#/');
  const box = page.locator('#filter');
  await box.fill('');
  await box.focus();
  const list = page.getByRole('listbox', { name: 'Filter suggestions' });
  await expect(list.getByRole('option', { name: /is:closed/ })).toBeVisible();
  await expect(list.getByRole('option')).toHaveCount(await list.getByRole('option').count());
  expect(await list.getByRole('option').count()).toBeLessThanOrEqual(5);
  await box.press('ArrowDown');
  const first = (await list.getByRole('option').first().locator('code').textContent()) ?? '';
  await box.press('Enter');
  await expect(box).toHaveValue(first);
});

test('typing completes; Tab takes it; Esc closes the list first', async ({ page }) => {
  await page.goto('/#/');
  const box = page.locator('#filter');
  await box.fill('');
  await box.pressSequentially('status:re');
  const list = page.getByRole('listbox', { name: 'Filter suggestions' });
  await expect(list.getByRole('option', { name: /status:ready/ })).toBeVisible();
  await expect(page.locator('.fbox-trick')).toContainText('-status:');
  await box.press('Tab');
  await expect(box).toHaveValue('status:ready');
  await expect(box).toBeFocused();
  await box.press('Escape');
  await expect(list).toBeHidden();
  await expect(box).toHaveValue('status:ready');
});

test('the list’s link opens How filtering works', async ({ page }) => {
  await page.goto('/#/');
  const box = page.locator('#filter');
  await box.fill('');
  await box.focus();
  await page.getByRole('button', { name: 'How filtering works', exact: true }).filter({ hasText: 'How filtering works' }).click();
  await expect(how(page)).toBeVisible();
});
