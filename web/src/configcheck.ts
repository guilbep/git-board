// The file editor's check of a text (Settings › The file): TOML first,
// here, with its line and column; then what the server refused of it (PUT
// /api/config?dry_run=1), its message; else what it holds. Pure: node runs
// configcheck.test.ts on it.
import { parse } from 'smol-toml';

export interface ConfigCheck {
  ok: boolean;
  error?: { line: number; col: number; message: string };
  message?: string;
  warnings: string[];
  summary: { fields: string[]; views: string[]; sprints: number };
}

const keys = (v: unknown) => (v && typeof v === 'object' && !Array.isArray(v) ? Object.keys(v) : []);

export function checkOf(text: string, refused: string): ConfigCheck {
  const empty = { fields: [], views: [], sprints: 0 };
  let doc: Record<string, unknown>;
  try {
    doc = parse(text) as Record<string, unknown>;
  } catch (e) {
    const err = e as { line?: number; column?: number; message?: string };
    const first = (err.message ?? String(e)).split('\n')[0]!.replace(/^Invalid TOML document: /, '');
    return { ok: false, error: { line: err.line ?? 1, col: err.column ?? 1, message: first }, warnings: [], summary: empty };
  }
  if (refused) return { ok: false, message: refused, warnings: [], summary: empty };
  const sprints = (doc.sprints as { list?: unknown[] } | undefined)?.list;
  return { ok: true, warnings: [], summary: { fields: keys(doc.fields), views: keys(doc.views), sprints: Array.isArray(sprints) ? sprints.length : 0 } };
}
