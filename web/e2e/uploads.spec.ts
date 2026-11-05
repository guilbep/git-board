// Attachments (routes 40 and 41, ADR 0015): on the Markdown board, whose
// server has a bucket (serve.mjs: a fake S3), a file picked or dropped in
// a comment is uploaded, and the comment gets its Markdown; a kind no
// issue carries is refused, said under the box.
import { expect, test } from '@playwright/test';
import { errors, markdownBoard } from './board';

const md = markdownBoard;
const png = Buffer.concat([Buffer.from('\x89PNG\r\n\x1a\n', 'latin1'), Buffer.alloc(64)]);

test('a file picked, then one dropped, in a comment: uploaded, the comment gets their Markdown', async ({ page, request }) => {
  const errs = errors(page);
  if (!((await (await request.get(`${md}/api/me`)).json()) as { me: unknown }).me) {
    const r = await request.put(`${md}/api/me`, { data: { name: 'E2E', email: 'e2e@example.com' } });
    expect(r.ok(), await r.text()).toBeTruthy();
  }
  const status = (await (await request.get(`${md}/api/uploads`)).json()) as { enabled: boolean; where: string };
  expect(status.enabled).toBe(true);
  const made = await request.post(`${md}/api/issues`, { data: { title: 'E2E with a picture', body: '', labels: [], set: [] } });
  expect(made.ok(), await made.text()).toBeTruthy();
  const { id } = (await made.json()) as { id: string };

  await page.goto(`${md}/#/issues/${id}`);
  const box = page.getByRole('textbox', { name: 'Comment' });
  await box.fill('See: ');
  await page.locator('input[type=file]').setInputFiles({ name: 'shot.png', mimeType: 'image/png', buffer: png });
  await expect(box).toHaveValue(/^See:\s*!\[shot\.png\]\(https:\/\/\S+\/shot\.png\)/);

  // a file dropped on the box
  await box.evaluate((el, bytes) => {
    const dt = new DataTransfer();
    dt.items.add(new File([new Uint8Array(bytes)], 'notes.txt', { type: 'text/plain' }));
    el.dispatchEvent(new DragEvent('drop', { dataTransfer: dt, bubbles: true, cancelable: true }));
  }, [...Buffer.from('some notes\n')]);
  await expect(box).toHaveValue(/\[notes\.txt\]\(https:\/\/\S+\/notes\.txt\)/);

  // a kind no issue carries: refused, said under the box
  await page.locator('input[type=file]').setInputFiles({ name: 'run.exe', mimeType: 'application/octet-stream', buffer: Buffer.from('MZ\x90\x00binary', 'latin1') });
  await expect(page.locator('.gb-attach-error')).toContainText("run.exe wasn't attached");
  expect(errs).toEqual([]);
});

test('without a bucket, Settings › Attachments says they are off', async ({ page }) => {
  const errs = errors(page);
  await page.goto('/#/settings/attachments');
  await expect(page.getByRole('status').filter({ hasText: 'Off on this server' })).toBeVisible();
  expect(errs).toEqual([]);
});
