// Sync › Check on a big board: what a sync would do comes as counts by
// forge and way, warnings grouped by reason, never one row a write. A
// check of a forge never synced listed 5,199 rows, one a comment, and the
// page scrolled for minutes. The run is the server's answer as it stands
// at its end (GET /api/sync); the tallies themselves are surface/http's
// TestRunTallies.
import { expect, test } from '@playwright/test';
import { errors } from './board';

const step = (i: number, line: string) => ({ forge: 'gitlab', way: 'push', what: 'comment', issue: i.toString(16).padStart(40, '0'), line });

test('a check of 1,000 would-be writes is a few counted lines', async ({ page }) => {
  const errs = errors(page);
  await page.route('**/api/sync', async (route) => {
    if (route.request().method() !== 'GET') return route.continue();
    const res = await route.fetch();
    const st = await res.json();
    st.run = {
      id: '9', scope: 'forges', way: 'sync', dry_run: true, started_at: new Date().toISOString(), ended_at: new Date().toISOString(), state: 'done',
      steps: [{ forge: 'gitlab', way: 'push', what: 'held', line: 'the first sync with this forge' }],
      tallies: [
        { forge: 'gitlab', way: 'push', what: 'create-comment', n: 4200, examples: [1, 2, 3, 4, 5].map((i) => step(i, 'create-comment')) },
        { forge: 'gitlab', way: 'push', what: 'set-status', n: 1000, examples: [1, 2, 3, 4, 5].map((i) => step(i, 'set-status')) },
        { forge: 'gitlab', what: 'warning: gitlab.example.org/1 refused assignees "…" of issue …: no account there', n: 900, examples: [] },
        { forge: 'github', way: 'pull', what: 'updated', n: 12, examples: [] },
      ],
    };
    await route.fulfill({ response: res, json: st });
  });
  await page.goto('/#/settings/sync');
  const report = page.getByRole('region', { name: /The check finished/ });
  await expect(report.getByText(`${(4200).toLocaleString()} comments posted`)).toBeVisible();
  await expect(report.getByText('would push')).toBeVisible();
  await expect(report.getByText(/900 warnings, 1 kinds/)).toBeVisible();
  // every row of the report, examples and notes included: a handful
  expect(await report.locator('li').count()).toBeLessThan(50);
  expect(errs).toEqual([]);
});
