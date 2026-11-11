// Releases (routes 43 to 48): a release cut from the page, as a job; its
// page; its notes, a comment and a yank on its record. The board's
// [release] for the test: dev and prod, no overlay, its authority a bare
// repository beside the board (git reaches no other), put back after.
import { expect, test, type APIRequestContext } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { errors, identity } from './board';

const work = process.env.E2E_DIR ?? '';
const repo = join(work, 'repo');
const git = (...args: string[]) => execFileSync('git', args, { cwd: repo, stdio: 'pipe' }).toString();

// own is the board's own page's Origin: a [release] write passes the
// code gate (ADR 0013) from it alone.
const own = (baseURL: string | undefined) => ({ Origin: baseURL ?? '' });

async function config(request: APIRequestContext): Promise<{ text: string; version: string }> {
  return (await (await request.get('/api/config')).json()) as { text: string; version: string };
}

test('a release cut from the page, its page, its notes, a comment and a yank', async ({ page, request, baseURL }) => {
  const errs = errors(page);
  await identity(request);
  const bare = join(work, 'rel.git');
  if (!existsSync(bare)) {
    execFileSync('git', ['init', '-q', '--bare', '-b', 'dev', bare]);
    git('remote', 'add', 'rel', bare);
  }
  git('push', '-q', 'rel', 'HEAD:refs/heads/dev');
  git('fetch', '-q', 'rel');
  const before = await config(request);
  const set = await request.patch('/api/config', { data: { set: { release: { environments: ['dev', 'prod'], remote: 'rel' } } }, headers: { ...own(baseURL), 'If-Match': `"${before.version}"` } });
  expect(set.ok(), await set.text()).toBeTruthy();
  try {
    await page.goto('/#/releases');
    await expect(page.getByRole('link', { name: 'Releases' })).toHaveClass(/active/);
    await expect(page.locator('.gb-env')).toHaveCount(2);
    await expect(page.getByText('No release tag yet.')).toBeVisible();

    await page.getByRole('button', { name: 'Cut a release' }).click();
    const cut = page.locator('dialog[open]').getByRole('button', { name: /^Cut v/ });
    await expect(cut).toBeEnabled({ timeout: 10_000 });
    const tag = ((await cut.textContent()) ?? '').trim().replace(/^Cut /, '').split(' ')[0]!;
    await cut.click();
    const link = page.getByRole('link', { name: tag, exact: true });
    await expect(link.first()).toBeVisible({ timeout: 20_000 });
    await expect(page.locator('.gb-job')).toContainText(tag);

    await page.locator('a.gb-rel-name', { hasText: tag }).click();
    await expect(page.locator('h1')).toHaveText(tag);
    await page.getByRole('button', { name: 'Edit notes' }).click();
    await page.getByRole('textbox', { name: 'Release notes' }).fill('The e2e release, **notes** edited.');
    await page.getByRole('button', { name: 'Save notes' }).click();
    await expect(page.locator('main').getByText('notes edited')).toBeVisible();

    await page.getByRole('textbox', { name: 'Comment' }).fill('Seen on prod.');
    await page.getByRole('button', { name: 'Comment', exact: true }).click();
    await expect(page.locator('main').getByText('Seen on prod.')).toBeVisible();

    await page.getByRole('button', { name: 'Yank…' }).click();
    await page.getByLabel('Why, for the record (required)').fill('breaks the e2e');
    await page.getByRole('button', { name: `Yank ${tag}` }).click();
    await expect(page.locator('.epfl-badge', { hasText: 'Yanked' })).toBeVisible();

    const list = (await (await request.get('/api/releases?local=1')).json()) as { releases: { tag: { name: string }; record: { yanked: string; notes: string } | null }[] };
    expect(list.releases[0]).toMatchObject({ tag: { name: tag }, record: { yanked: 'breaks the e2e', notes: 'The e2e release, **notes** edited.' } });
  } finally {
    const now = await config(request);
    const back = await request.put('/api/config', { data: { text: before.text }, headers: { ...own(baseURL), 'If-Match': `"${now.version}"` } });
    expect(back.ok(), await back.text()).toBeTruthy();
  }
  expect(errs).toEqual([]);
});

test('Releases without [release] says how to set it up', async ({ page, request, baseURL }) => {
  const errs = errors(page);
  const before = await config(request);
  const off = await request.patch('/api/config', { data: { set: { release: null } }, headers: { ...own(baseURL), 'If-Match': `"${before.version}"` } });
  expect(off.ok(), await off.text()).toBeTruthy();
  try {
    await page.goto('/#/releases');
    await expect(page.getByRole('heading', { name: 'No releases here yet' })).toBeVisible();
    await page.getByRole('link', { name: 'Set up the release in Settings' }).click();
    await expect(page).toHaveURL(/#\/settings\/release/);
  } finally {
    const now = await config(request);
    const back = await request.put('/api/config', { data: { text: before.text }, headers: { ...own(baseURL), 'If-Match': `"${now.version}"` } });
    expect(back.ok(), await back.text()).toBeTruthy();
  }
  expect(errs).toEqual([]);
});
