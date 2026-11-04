// Settings › Datastore's pure part: which kind a board.datastore value is,
// and what the page says of each. No "@/" imports: node runs
// datastore.test.ts on it.

export type StoreKind = 'git' | 'postgres' | 'markdown' | 'forge';

// kindOf is the kind a board.datastore value names; anything else is git's.
export function kindOf(setting: string): StoreKind {
  if (setting === 'forge') return 'forge';
  if (setting.startsWith('markdown:')) return 'markdown';
  if (/^postgres(ql)?:\/\//.test(setting)) return 'postgres';
  return 'git';
}

// valueOf is the board.datastore value a kind and a URL (or, for
// Markdown, a folder) make.
export function valueOf(kind: StoreKind, url: string): string {
  if (kind === 'postgres') return url.trim();
  if (kind === 'markdown') return 'markdown:' + url.trim();
  return kind;
}

// folderOf is a markdown:<dir> value's folder.
export function folderOf(setting: string): string {
  return setting.startsWith('markdown:') ? setting.slice('markdown:'.length) : '';
}

// folderProblem says what's wrong with a Markdown folder, '' when none.
export function folderProblem(dir: string): string {
  const d = dir.trim();
  if (!d) return 'A folder in the checkout: issues';
  if (d.startsWith('/') || d.split('/').includes('..')) return 'A folder inside the checkout, from its top: issues, docs/issues';
  return '';
}

// urlProblem says what's wrong with a PostgreSQL URL, '' when none.
export function urlProblem(url: string): string {
  const u = url.trim();
  if (!u) return 'A URL: postgres://user@host:5432/database';
  if (!/^postgres(ql)?:\/\//.test(u)) return 'It starts with postgres://';
  if (!/^postgres(ql)?:\/\/[^/]+\/[^/?]+/.test(u)) return 'Name the database: postgres://user@host/database';
  return '';
}

// kinds are the choices, each with what it gives and gives up (ADR 0002).
export const kinds: { kind: StoreKind; label: string; gives: string; givesUp: string }[] = [
  {
    kind: 'git',
    label: 'Git',
    gives: 'The board in this repository (refs/board/*): offline work, pull and push with remotes, git log of the board.',
    givesUp: 'Every clone holds a copy; it moves with a pull and a push.',
  },
  {
    kind: 'postgres',
    label: 'PostgreSQL',
    gives: 'One shared server: every writer sees each write at once, and the index (lists, search) is kept there for every machine.',
    givesUp: 'No offline work unless the server is local; pull, push and start over don’t apply.',
  },
  {
    kind: 'markdown',
    label: 'Markdown files',
    gives: 'A folder in the checkout: one file per issue to read and edit, committed and reviewed with the code; the packs beside them in .board.',
    givesUp: 'No project numbers but the forges’; a hand edit goes in with the next write or git board import; branches merge their boards with git.',
  },
  {
    kind: 'forge',
    label: 'The forges alone',
    gives: 'No store of your own: a copy in .git/board that sync fills, each write sent to the forges at once.',
    givesUp: 'No project numbers but the forges’; the history is what the pulls saw; every write needs the network.',
  },
];

// datastoreSummary is the tab's line in the menu.
export function datastoreSummary(d: { datastore: string; restart: boolean }): string {
  return d.restart ? `${d.datastore}; a change waits for a restart` : d.datastore;
}
