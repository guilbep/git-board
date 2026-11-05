// The cycle on the issue page (experimental; api.md routes 49 and 50): its
// panel walks an issue through its steps with the server's actions, opens
// and merges a branch's pull request on the (fake) forge, and the Work
// page's Review and Sign off land on working controls.
import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { errors, identity, issue } from './board';

const repo = () => join(process.env.E2E_DIR ?? '', 'repo');
const git = (...args: string[]) =>
  execFileSync('git', args, { cwd: repo(), env: { ...process.env, GIT_AUTHOR_NAME: 'E2E', GIT_AUTHOR_EMAIL: 'e2e@example.com', GIT_COMMITTER_NAME: 'E2E', GIT_COMMITTER_EMAIL: 'e2e@example.com' } })
    .toString()
    .trim();

// a branch on origin with a test file, made with plumbing alone: the
// working tree (and the board's file there) is left as it is
function pushedBranch(name: string) {
  const blob = execFileSync('git', ['hash-object', '-w', '--stdin'], { cwd: repo(), input: 'package x\n' }).toString().trim();
  const tree = execFileSync('git', ['mktree'], { cwd: repo(), input: `${git('ls-tree', 'HEAD')}\n100644 blob ${blob}\tx_test.go\n` }).toString().trim();
  const commit = git('commit-tree', tree, '-p', 'HEAD', '-m', 'a test');
  git('update-ref', `refs/heads/${name}`, commit);
  git('update-ref', `refs/remotes/origin/${name}`, commit);
  return commit;
}

test('the issue page walks the cycle: no criteria, the risk, the start', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  const made = await issue(request, 'E2E cycle walk', ['status=ready']);
  await page.goto(`/#/issues/${made.id}`);
  const panel = page.getByRole('region', { name: 'The cycle' });
  await expect(panel.getByText(/Step 1 · Intent/)).toBeVisible();
  await panel.getByRole('button', { name: 'Accept without criteria' }).click();
  await expect(panel.getByText('Step 2 · Risk')).toBeVisible();
  await expect(page.getByText('Accepted without acceptance criteria')).toBeVisible();
  await panel.getByRole('button', { name: 'Low' }).click();
  await expect(panel.getByText(/Step 3 · Plan/)).toBeVisible();
  await panel.getByRole('button', { name: 'Start: move to in-progress' }).click();
  await expect(panel.getByText(/Step 4 · Tests/)).toBeVisible();
  await expect(panel.getByTestId('cycle-waits')).toHaveText('Anyone to write the tests.');
  // the status moved with it, on the issue itself
  const th = (await (await request.get(`/api/issues/${made.id}`)).json()) as { labels: string[] };
  expect(th.labels).toContain('status:in-progress');
  expect(errs).toEqual([]);
});

test('a branch’s pull request opened and merged from the issue page', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  const made = await issue(request, 'E2E cycle pull', ['status=in-progress']);
  const branch = `feat/${made.id.slice(0, 7)}-pull`;
  pushedBranch(branch);
  await page.goto(`/#/issues/${made.id}`);
  const panel = page.getByRole('region', { name: 'The cycle' });
  await expect(panel.getByText(/Step 5 · Code/)).toBeVisible();
  await panel.getByRole('button', { name: `Open a pull request from ${branch}` }).click();
  const pr = panel.getByRole('group', { name: 'Pull request' });
  await expect(pr.getByRole('link', { name: '#1 open' })).toBeVisible();
  await pr.getByRole('button', { name: 'Edit' }).click();
  await pr.getByLabel("The pull request's title").fill('E2E cycle pull, edited');
  await pr.getByRole('button', { name: 'Save' }).click();
  await expect(pr.getByText('E2E cycle pull, edited')).toBeVisible();
  await pr.getByRole('button', { name: 'Merge #1' }).click();
  await expect(pr.getByRole('link', { name: '#1 merged' })).toBeVisible();
  expect(errs).toEqual([]);
});

test('the Work page’s Review and Sign off land on working controls', async ({ page, request }) => {
  const errs = errors(page);
  await identity(request);
  const before = (await (await request.get('/api/config')).json()) as { text: string; version: string };
  const statuses = ['backlog', 'ready', 'in-progress', 'in-review', 'qa', 'done'];
  const on = await request.patch('/api/config', { data: { set: { 'fields.status.options': statuses } }, headers: { 'If-Match': `"${before.version}"` } });
  expect(on.ok(), await on.text()).toBeTruthy();
  try {
    const made = await issue(request, 'E2E cycle review', ['status=in-review']);
    const body = { body: '## it works\n- [ ] it saves\n' };
    expect((await request.patch(`/api/issues/${made.id}`, { data: body })).ok()).toBeTruthy();

    await page.goto('/#/work?who=team');
    await expect(page.getByRole('heading', { name: /^Needs you/ })).toBeVisible({ timeout: 30_000 });
    for (const more of await page.getByRole('button', { name: /^Show \d+ more$/ }).all()) await more.click();
    const row = page.locator('main tr', { hasText: 'E2E cycle review' });
    await row.getByRole('link', { name: 'Review', exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/issues/${made.id}`));
    const panel = page.getByRole('region', { name: 'The cycle' });
    await panel.getByLabel('Your review', { exact: true }).fill('Reads well: one save, one test.');
    await panel.getByRole('button', { name: 'Submit the review' }).click();
    await expect(page.getByText('Reads well: one save, one test.').last()).toBeVisible();

    // to validation: Work's Sign off, then the criterion, a run, the sign-off
    expect((await request.patch(`/api/issues/${made.id}`, { data: { set: ['status=qa'] } })).ok()).toBeTruthy();
    await page.goto('/#/work?who=team');
    await expect(page.getByRole('heading', { name: /^Needs you/ })).toBeVisible({ timeout: 30_000 });
    for (const more of await page.getByRole('button', { name: /^Show \d+ more$/ }).all()) await more.click();
    await page.locator('main tr', { hasText: 'E2E cycle review' }).getByRole('link', { name: 'Sign off qa', exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/issues/${made.id}`));
    await expect(panel.getByText(/Step 7/)).toBeVisible();
    await panel.getByRole('button', { name: 'Check: it saves' }).click();
    await expect(panel.getByText(/checked by/)).toBeVisible();
    await expect(panel.getByRole('button', { name: 'Sign off qa' })).toBeDisabled();
    await panel.getByLabel("A run's output").fill('ok  example 0.1s');
    await panel.getByRole('button', { name: 'Record the run' }).click();
    await expect(panel.getByRole('button', { name: 'Sign off qa' })).toBeEnabled();
    await panel.getByRole('button', { name: 'Sign off qa' }).click();
    await expect(page.locator('.gb-ptop-state', { hasText: 'Closed' })).toBeVisible();
  } finally {
    const now = (await (await request.get('/api/config')).json()) as { version: string };
    const back = await request.put('/api/config', { data: { text: before.text }, headers: { 'If-Match': `"${now.version}"` } });
    expect(back.ok(), await back.text()).toBeTruthy();
  }
  expect(errs).toEqual([]);
});
