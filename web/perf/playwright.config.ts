// The interactions timed (make perf-web): web/perf/interactions.perf.ts on
// a 5,000-issue board that git-board serves; its benchmark lines go to
// $PERF_OUT. Not the e2e tests' config: one long test, no e2e board.
import { defineConfig } from '@playwright/test';

const port = process.env.PERF_WEB_PORT ?? '18492';

export default defineConfig({
  testDir: '.',
  testMatch: 'interactions.perf.ts',
  workers: 1,
  timeout: 20 * 60_000,
  reporter: 'line',
  use: { baseURL: `http://127.0.0.1:${port}`, browserName: 'chromium' },
  webServer: {
    command: `"${process.env.PERF_BIN}" serve --addr 127.0.0.1:${port} --log-level warn`,
    cwd: process.env.PERF_DIR ?? '.',
    url: `http://127.0.0.1:${port}/api/board`,
    timeout: 120_000,
    reuseExistingServer: false,
  },
});
