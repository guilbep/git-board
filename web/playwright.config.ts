// The e2e tests: a browser on this checkout's git-board, serving
// throwaway boards (e2e/serve.mjs). make web-e2e; the first run needs
// npx playwright install chromium.
import { defineConfig } from '@playwright/test';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const port = process.env.E2E_PORT ?? '18421';
// the board's directory: the tests read its .git-board.toml (e2e/board.ts)
export const dir = process.env.E2E_DIR ?? join(tmpdir(), `git-board-e2e-${port}`);
process.env.E2E_DIR = dir;
process.env.E2E_PORT = port; // the tests reach the Markdown board beside it (e2e/board.ts)

export default defineConfig({
  testDir: 'e2e',
  workers: 1, // one board, one file: the tests run in turn
  use: { baseURL: `http://127.0.0.1:${port}`, browserName: 'chromium' },
  webServer: {
    command: 'node e2e/serve.mjs',
    url: `http://127.0.0.1:${port}/api/board`,
    timeout: 300_000, // the web build and the go build come first
    reuseExistingServer: false,
    env: { E2E_PORT: port, E2E_DIR: dir, E2E_TOML: process.env.E2E_TOML ?? '' },
  },
});
