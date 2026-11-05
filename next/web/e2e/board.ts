// What the e2e tests share: the e2e board's file, as serve.mjs wrote it
// and the saves changed it; the page's uncaught errors; an identity and
// issues made through the API.
import { expect, type APIRequestContext, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

export const boardFile = (): string => readFileSync(join(process.env.E2E_DIR ?? '', 'repo', '.git-board.toml'), 'utf8');

// errors collects the page's uncaught errors: a page that throws is broken
// even when it shows something
export function errors(page: Page): Error[] {
  const out: Error[] = [];
  page.on('pageerror', (e) => out.push(e));
  return out;
}

// identity: someone to write as, made once (a second PUT of the same
// person is refused: they are on the board already)
export async function identity(request: APIRequestContext) {
  if (((await (await request.get('/api/me')).json()) as { me: unknown }).me) return;
  const r = await request.put('/api/me', { data: { name: 'E2E', email: 'e2e@example.com' } });
  expect(r.ok(), await r.text()).toBeTruthy();
}

// issue makes one, answering its id and version
export async function issue(request: APIRequestContext, title: string, set: string[] = [], labels: string[] = []): Promise<{ id: string; version: string }> {
  const r = await request.post('/api/issues', { data: { title, body: '', labels, set } });
  expect(r.ok(), await r.text()).toBeTruthy();
  return (await r.json()) as { id: string; version: string };
}
