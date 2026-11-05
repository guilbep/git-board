// Insights (GET /api/insights): the board's history replayed, each chart
// with its finding; the field and the period asked again; the sprint's
// report as Markdown.
import { expect, test } from '@playwright/test';
import { errors, identity, issue } from './board';

test('Insights replays the history: the flow, the tiles, another field, the sprint report', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  const a = await issue(request, 'E2E insights started', ['status=ready', 'sprint=s1']);
  expect((await request.patch(`/api/issues/${a.id}`, { data: { set: ['status=in-progress'] } })).ok()).toBeTruthy();
  expect((await request.patch(`/api/issues/${a.id}`, { data: { status: 'closed' } })).ok()).toBeTruthy();
  await issue(request, 'E2E insights waiting', ['status=backlog', 'prio=p1', 'sprint=s1']);

  await page.goto('/#/insights');
  await expect(page.getByRole('link', { name: 'Insights' })).toHaveClass(/active/);
  await expect(page.locator('#h-flow')).toContainText('open issues today');
  await expect(page.locator('.ins-tile').first()).toContainText('Open today');
  await expect(page.locator('#h-thr')).toContainText('closed a week');
  // the burn-up: the latest sprint started that has issues (other tests
  // rename and add sprints: its title is the API's)
  const rep = (await (await request.get('/api/insights?sprint=')).json()) as { title: string; markdown: string };
  await expect(page.locator('#h-bu')).toContainText(`${rep.title}:`);
  // the time machine's bars: one a value of status, in workflow order
  await expect(page.locator('.ins-bars li').first()).toContainText('backlog');

  // another field: the flow is drawn for it
  await page.getByRole('combobox', { name: 'Field' }).selectOption('prio');
  await expect(page.locator('.ins-bars')).toContainText('p1');
  await page.getByRole('combobox', { name: 'Period' }).selectOption('30');
  await expect(page.locator('#h-tm')).toContainText('open issues');

  // the sprint's report, rendered, then as Markdown
  await page.getByRole('button', { name: 'Sprint report' }).click();
  const dialog = page.locator('.srep');
  await expect(dialog.locator('#srep-title')).toContainText(rep.title);
  await expect(dialog.locator('.srep-body h1')).toContainText(rep.title);
  await dialog.getByRole('tab', { name: 'Markdown' }).click();
  await expect(dialog.getByLabel('The report in Markdown')).toHaveValue(rep.markdown);
  expect(errs).toEqual([]);
});

test('Insights on a view, and a view the file does not name', async ({ page, request }) => {
  const errs = errors(page);
  const r = await request.get('/api/insights?view=nope');
  expect(r.status()).toBe(400);
  expect(((await r.json()) as { code: string; field: string }).field).toBe('view');
  await page.goto('/#/insights');
  await expect(page.locator('#h-flow')).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Issues' }).locator('option')).toContainText(['All issues']);
  expect(errs).toEqual([]);
});
