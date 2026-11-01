// Every settings tab opens without a page error: the page keeps them all
// mounted, so one tab's throw breaks the others (the model's null forges
// broke Fields). E2E_TOML=<a board's file> runs it on that board.
import { expect, test } from '@playwright/test';
import { settingsTabs } from '../src/settingstabs';

for (const t of settingsTabs) {
  test(`the ${t.label} tab opens`, async ({ page }) => {
    const errs: Error[] = [];
    page.on('pageerror', (e) => errs.push(e));
    await page.goto(`/#/settings/${t.id}`);
    await expect(page.locator('.set-tab[aria-current="page"]')).toContainText(t.label);
    await page.waitForLoadState('networkidle');
    expect(errs).toEqual([]);
  });
}
