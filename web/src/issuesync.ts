// What one issue's sync did, or would do, on each forge, in words:
// "sent 1 comment", "created the issue with 2 comments", "got 3 changes".
import type { BridgeTally, SyncDirection } from './api/board';

const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`;

function rest(d: SyncDirection): string[] {
  const out: string[] = [];
  if (d.comments) out.push(plural(d.comments, 'comment'));
  if (d.edits) out.push(plural(d.edits, 'edit'));
  if (d.changes) out.push(plural(d.changes, 'change'));
  return out;
}

// one way: the issue created (or imported) with its texts, else the
// changes sent (or got)
function way(d: SyncDirection, dryRun: boolean, create: [string, string], change: [string, string]): string {
  const [done, would] = d.issues ? create : change;
  const verb = dryRun ? would : done;
  const what = rest(d);
  if (d.issues) return what.length ? `${verb} with ${what.join(', ')}` : verb;
  return what.length ? `${verb} ${what.join(', ')}` : '';
}

export function tallyWords(t: BridgeTally, dryRun: boolean): string {
  const words = [
    way(t.to, dryRun, ['created the issue', 'would create the issue'], ['sent', 'would send']),
    way(t.from, dryRun, ['imported the issue', 'would import the issue'], ['got', 'would get']),
  ].filter(Boolean);
  if (t.errors) words.push(plural(t.errors, 'error'));
  else if (t.warnings) words.push(plural(t.warnings, 'warning'));
  return words.join('; ') || 'up to date';
}
