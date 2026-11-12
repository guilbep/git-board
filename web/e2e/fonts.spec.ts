// The fonts: the system's by default, so a page off the EPFL network asks
// nothing of enac-it-fonts (which answers 403 there); EPFL's (Suisse
// Int'l) when chosen in Settings › You › Appearance, kept in this browser.
import { expect, test, type Page } from '@playwright/test';
import { errors } from './board';

const fontHost = 'enac-it-fonts.epfl.ch';

// fontRequests counts the page's requests to the fonts' host from now on;
// none leaves the machine (each is aborted)
async function fontRequests(page: Page): Promise<() => number> {
  let n = 0;
  page.on('request', (r) => {
    if (new URL(r.url()).hostname === fontHost) n++;
  });
  await page.route(`https://${fontHost}/**`, (r) => r.abort());
  return () => n;
}

test('by default the page asks nothing of the EPFL fonts host', async ({ page }) => {
  const errs = errors(page);
  const asked = await fontRequests(page);
  await page.goto('/#/');
  await expect(page.getByRole('banner')).toBeVisible();
  await page.goto('/#/settings/you');
  await expect(page.getByRole('radiogroup', { name: 'Fonts' })).toBeVisible();
  await page.waitForLoadState('networkidle');
  expect(asked()).toBe(0);
  // a preconnect makes no request the page sees, but opens a connection
  await expect(page.locator(`link[href*="${fontHost}"]`)).toHaveCount(0);
  expect(await page.evaluate(() => getComputedStyle(document.body).fontFamily)).not.toContain('Suisse');
  expect(errs).toEqual([]);
});

test('EPFL fonts, chosen in Settings, load from their host and stay chosen', async ({ page }) => {
  const errs = errors(page);
  const asked = await fontRequests(page);
  await page.goto('/#/settings/you');
  await page.getByRole('radiogroup', { name: 'Fonts' }).getByText('EPFL', { exact: true }).click();
  await expect(page.locator('html')).toHaveAttribute('data-fonts', 'epfl');
  await expect.poll(asked).toBeGreaterThan(0);
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-fonts', 'epfl');
  await expect(page.getByRole('radiogroup', { name: 'Fonts' }).getByRole('radio', { name: /EPFL/ })).toBeChecked();
  await page.getByRole('radiogroup', { name: 'Fonts' }).getByText('System', { exact: true }).click();
  await expect(page.locator('html')).not.toHaveAttribute('data-fonts', 'epfl');
  expect(errs).toEqual([]);
});
