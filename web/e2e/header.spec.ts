// The header and the keys (docs/prd/header-help.md, docs/prd/avatars.md):
// Settings as a cog, the account card, pictures, the search's two keys,
// the key line, the tips. The e2e board has no identity: the tests that
// need one answer /api/me themselves.
import { expect, test, type Page } from '@playwright/test';

const PIC = 'https://avatars.githubusercontent.com/u/1?s=88';
async function asPierre(page: Page, avatar = PIC) {
  const pierre = { id: 'p1', name: 'Pierre Guilbert', login: 'guilbep', avatar_url: avatar };
  await page.route('**/api/me', (r) =>
    r.request().method() === 'GET'
      ? r.fulfill({ json: { me: pierre, identities: [pierre], view: 'classic', view_from: 'default', avatars: 'forge' } })
      : r.fallback(),
  );
}
// a 1×1 PNG, for a picture that loads
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=', 'base64');

test('Settings is the nav’s cog, current on every Settings page', async ({ page }) => {
  await page.goto('/#/settings/labels');
  const cog = page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Settings' });
  await expect(cog).toHaveAttribute('aria-current', 'page');
  await page.goto('/#/');
  await expect(cog).not.toHaveAttribute('aria-current', 'page');
});

test('the avatar opens your account card: who, profile, identity, appearance', async ({ page }) => {
  await asPierre(page);
  await page.route('https://avatars.githubusercontent.com/**', (r) => r.fulfill({ body: png, contentType: 'image/png' }));
  await page.goto('/#/');
  const button = page.getByLabel('Pierre Guilbert: your account');
  await button.click();
  const card = page.locator('.ac');
  await expect(card.getByText('Pierre Guilbert')).toBeVisible();
  await expect(card.getByText('guilbep')).toBeVisible();
  await expect(card.getByRole('link', { name: 'Your profile' })).toHaveAttribute('href', '#/settings/you');
  await expect(card.getByRole('link', { name: 'Switch identity' })).toBeVisible();
  await expect(card.getByText(/Keyboard shortcuts|checked|Not checked/)).toHaveCount(0);
  await card.getByRole('button', { name: 'Dark' }).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await card.getByRole('button', { name: 'Light' }).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await page.keyboard.press('Escape');
  await expect(card).toBeHidden();
  await expect(button).toBeFocused();
});

test('avatar falls back to initials when the picture can’t load', async ({ page }) => {
  await asPierre(page);
  await page.route('https://avatars.githubusercontent.com/**', (r) => r.abort());
  await page.goto('/#/');
  const button = page.getByLabel('Pierre Guilbert: your account');
  await expect(button.getByText('PG')).toBeVisible();
  await expect(button.locator('img')).toHaveCount(0);
});

test('a picture that loads is shown', async ({ page }) => {
  await asPierre(page);
  await page.route('https://avatars.githubusercontent.com/**', (r) => r.fulfill({ body: png, contentType: 'image/png' }));
  await page.goto('/#/');
  await expect(page.getByLabel('Pierre Guilbert: your account').locator('img')).toBeVisible();
});

test('the search shows both keys; its ? opens every shortcut', async ({ page }) => {
  await page.goto('/#/');
  const group = page.getByRole('group', { name: 'Search and shortcuts' });
  await expect(group.getByRole('button', { name: 'Search: filter or jump to' })).toBeVisible();
  await group.getByRole('button', { name: 'Keyboard shortcuts' }).click();
  await expect(page.getByRole('heading', { name: 'Keyboard shortcuts' })).toBeVisible();
});

test('the key line: the board’s keys, ? last; none on a phone', async ({ page }) => {
  await page.goto('/#/');
  const line = page.getByLabel('Keys here');
  await expect(line).toContainText('new issue');
  await expect(line).toContainText('search');
  await expect(line.locator('.gb-keyline-k').last()).toContainText('every shortcut');
  await page.setViewportSize({ width: 375, height: 700 });
  await expect(line).toBeHidden();
});

test('the palette shows each command’s key and its own keys', async ({ page }) => {
  await page.goto('/#/');
  await page.getByRole('button', { name: 'Search: filter or jump to' }).click();
  const dialog = page.getByRole('dialog', { name: 'Filter the issues, jump to one, or run a command' });
  await expect(dialog.getByRole('option', { name: /New issue/ }).locator('kbd')).toHaveText('c');
  await expect(dialog.getByRole('button', { name: /every shortcut/ })).toBeVisible();
});

test('a tip: in the header when wide, the strip when not, none on a phone; × retires it', async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 900 });
  await page.goto('/#/');
  const inHeader = page.locator('.app-tip [role=note]');
  await expect(inHeader).toBeVisible();
  await expect(inHeader).not.toHaveAttribute('aria-live', /.+/);
  await page.setViewportSize({ width: 1100, height: 800 });
  await expect(page.locator('.app-tipstrip [role=note]')).toBeVisible();
  await expect(inHeader).toHaveCount(0);
  await page.setViewportSize({ width: 375, height: 700 });
  await expect(page.locator('[role=note].gb-tip')).toHaveCount(0);

  await page.setViewportSize({ width: 1100, height: 800 });
  const tip = page.locator('.gb-tip');
  const first = await tip.locator('.gb-tip-text').textContent();
  await tip.getByRole('button', { name: /Hide this tip/ }).click();
  await page.reload();
  await expect(page.locator('.gb-tip-text')).not.toHaveText(first ?? '');
});

test('with reduced motion, the tip doesn’t change', async ({ browser }) => {
  const ctx = await browser.newContext({ reducedMotion: 'reduce', viewport: { width: 1100, height: 800 } });
  const page = await ctx.newPage();
  await page.clock.install();
  await page.goto('/#/');
  const text = page.locator('.gb-tip-text');
  const first = await text.textContent();
  await page.clock.runFor(30_000);
  await expect(text).toHaveText(first ?? '');
  await ctx.close();
});

test('the tips turn off from the list, and back on from Settings', async ({ page }) => {
  await page.setViewportSize({ width: 1100, height: 800 });
  await page.goto('/#/');
  await page.keyboard.press('?');
  await page.getByRole('button', { name: 'Hide tips' }).click();
  await page.keyboard.press('Escape');
  await expect(page.locator('.gb-tip')).toHaveCount(0);
  await page.goto('/#/settings/you');
  await page.getByRole('button', { name: 'Show every tip again' }).click();
  await expect(page.locator('.gb-tip')).toBeVisible();
});
