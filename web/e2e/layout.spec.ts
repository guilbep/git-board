// The settings pages' layout at the widths people use: nothing cut, nothing
// over its neighbour. Each test measures the page, as a screenshot would
// show it.
import { expect, test, type Page } from '@playwright/test';

test.use({ viewport: { width: 1440, height: 900 } });

// right edge of a box
// (every settings tab stays mounted: the visible one's)
const right = (page: Page, sel: string) => page.locator(`${sel}:visible`).first().evaluate((e) => e.getBoundingClientRect().right);

test('the Fields list shows each name whole', async ({ page }) => {
  await page.goto('/#/settings/fields/due');
  await expect(page.locator('.fl-nm').first()).toBeVisible();
  const cut = await page.locator('.fl-nm').evaluateAll((els) => els.filter((e) => e.scrollWidth > e.clientWidth).map((e) => e.textContent));
  expect(cut).toEqual([]);
});

test('the Fields editor keeps Remove inside its card', async ({ page }) => {
  await page.goto('/#/settings/fields/due');
  const card = await right(page, '.fl-ed .cc');
  expect(await right(page, '.fl-ed .cc-remove')).toBeLessThanOrEqual(card);
  // the summary is one line
  const h = await page.locator('.fs-sum').evaluate((e) => e.getBoundingClientRect().height);
  expect(h).toBeLessThan(24);
});

test('a role row stays in its column', async ({ page }) => {
  await page.goto('/#/settings/roles');
  await page.getByRole('button', { name: 'Add a role' }).click();
  await page.locator('.rs-input:visible').first().fill('lead-dev');
  await page.locator('.rs-input:visible').nth(1).fill('guilbep');
  const forms = await right(page, '.set-forms');
  const del = await page.locator('.set-forms:visible .cr-del').last().evaluate((e) => e.getBoundingClientRect().right);
  expect(del).toBeLessThanOrEqual(forms);
});

test('a hint reads as a sentence: a code chip shares the line of the text after it', async ({ page }) => {
  await page.goto('/#/settings/roles');
  const same = await page.locator('.set-forms .gb-hint', { hasText: 'digits and' }).evaluate((p) => {
    const code = [...p.querySelectorAll('code')].find((c) => c.textContent === '-')!;
    const next = code.nextSibling!; // ". One login each; …"
    const r = document.createRange();
    r.setStart(next, 0);
    r.setEnd(next, 1);
    return Math.abs(r.getBoundingClientRect().top - code.getBoundingClientRect().top) < 8;
  });
  expect(same).toBe(true);
});

test('Forge fields: the written-to panel wraps, and the legend waits for an override', async ({ page }) => {
  await page.goto('/#/settings/forge-fields/github');
  const pre = page.locator('.ff-toml');
  await expect(pre).toContainText('nothing');
  expect(await pre.evaluate((e) => e.scrollWidth <= e.clientWidth)).toBe(true);
  await expect(page.getByText('blue = an override')).toHaveCount(0);
});

test.describe('on a phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });
  test('the Sync table scrolls sideways instead of breaking words', async ({ page }) => {
    await page.goto('/#/settings/sync');
    const cell = page.locator('.gb-table td.sy-wrap').first();
    await expect(cell).toContainText('git@github.com:o/r.git');
    // the remote's address on one line, not one syllable a line
    const lines = await cell.evaluate((e) => {
      const r = document.createRange();
      r.selectNodeContents(e.firstElementChild ?? e);
      return new Set([...r.getClientRects()].map((x) => Math.round(x.top))).size;
    });
    expect(lines).toBeLessThanOrEqual(2);
  });
});
