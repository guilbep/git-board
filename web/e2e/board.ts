// The e2e board's file, as serve.mjs wrote it and the saves changed it.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

export const boardFile = (): string => readFileSync(join(process.env.E2E_DIR ?? '', 'repo', '.git-board.toml'), 'utf8');
