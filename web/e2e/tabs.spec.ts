// Every settings tab opens without a page error: the page keeps them all
// mounted, so one tab's throw breaks the others (the model's null forges
// broke Fields). An experimental feature's tab opens while the board has it
// on (/api/me), and is the Sync while it is off (settingstabs.ts).
// E2E_TOML=<a board's file> runs it on that board.
import { expect, test } from '@playwright/test';
import { featureOn, type Feature } from '../src/experimental';
import { settingsTabs } from '../src/settingstabs';

for (const t of settingsTabs) {
  test(`the ${t.label} tab opens`, async ({ page, request }) => {
    const errs: Error[] = [];
    page.on('pageerror', (e) => errs.push(e));
    const me = (await (await request.get('/api/me')).json()) as { experimental?: string[] };
    const on = !('feature' in t) || featureOn(me.experimental, t.feature as Feature);
    await page.goto(`/#/settings/${t.id}`);
    await expect(page.locator('.set-tab[aria-current="page"]')).toContainText(on ? t.label : 'Sync');
    await page.waitForLoadState('networkidle');
    expect(errs).toEqual([]);
  });
}
