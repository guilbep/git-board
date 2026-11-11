// What the e2e tests share: the e2e board's file, as serve.mjs wrote it
// and the saves changed it; the page's uncaught errors; an identity and
// issues made through the API.
import { expect, type APIRequestContext, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

// markdownBoard is serve.mjs's Markdown board, on E2E_PORT + 5: the port
// playwright.config.ts serves on, one default for every spec
export const markdownBoard = `http://127.0.0.1:${Number(process.env.E2E_PORT ?? '18421') + 5}`;

export const boardFile = (): string => readFileSync(join(process.env.E2E_DIR ?? '', 'repo', '.git-board.toml'), 'utf8');

// errors collects the page's uncaught errors: a page that throws is broken
// even when it shows something
export function errors(page: Page): Error[] {
  const out: Error[] = [];
  page.on('pageerror', (e) => out.push(e));
  // a filter the page answered itself that the server answers otherwise
  // (local/query.ts) is a bug too, though the page then shows the server's
  page.on('console', (m) => m.text().includes("differs from the server's") && out.push(new Error(m.text())));
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
