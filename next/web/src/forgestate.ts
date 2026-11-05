// A forge's icon in the bar (docs/design/issue-head/README.md, 16e and 18;
// docs/prd/issue-head.md, forge badges and new on the forge): GitHub's or
// GitLab's shape; nothing waiting, the bare icon; changes to send, a count
// with an up arrow; new there since the last sync (after the issue's own
// check), a count with a down arrow; a run going, out of reach, changed on
// both sides: a small badge each, said in words in its name and tooltip.
// Pure: ForgeIcons counts the waiting changes from the events. (v1's
// one-issue sync and its check, which said what was new there, wait for
// v2's: api.md, POST /api/sync with issue, later.)
import type { Forge } from './api/types.ts';

export type ForgeBadge = 'count' | 'syncing' | 'unreachable' | 'conflict' | null;
export interface ForgeState {
  kind: 'github' | 'gitlab' | 'other';
  badge: ForgeBadge;
  n: number; // changes waiting to be sent
  incoming: number; // new there, as the issue's last check found
  label: string;
}

export function forgeKind(f: Forge): ForgeState['kind'] {
  const s = `${f.name} ${f.url}`.toLowerCase();
  if (s.includes('github')) return 'github';
  if (s.includes('gitlab')) return 'gitlab';
  return 'other';
}

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

// The order when two apply: out of reach, then changed on both sides, then
// a run going, then the counts (to send, new there: one pill). kind: the
// forge's, when known; else the link's name and URL say.
export function forgeState(f: Forge, waiting: number, o: { kind?: string | undefined; running?: boolean | undefined; left?: string | undefined; conflict?: boolean | undefined; incoming?: number | undefined } = {}): ForgeState {
  const name = `${f.name}${f.number ? ` #${f.number}` : ''}`;
  const kind = o.kind === 'github' || o.kind === 'gitlab' ? o.kind : forgeKind(f);
  const incoming = o.incoming ?? 0;
  const base = { kind, n: waiting, incoming };
  if (o.left) return { ...base, badge: 'unreachable', label: `${name}: out of reach (${o.left})` };
  if (o.conflict) return { ...base, badge: 'conflict', label: `${name}: changed here and there since the last sync` };
  if (o.running) return { ...base, badge: 'syncing', label: `${name}: syncing now` };
  const said = [
    waiting > 0 && `${plural(waiting, 'change')} wait${waiting === 1 ? 's' : ''} to be sent`,
    incoming > 0 && `${incoming} new there since the last sync`,
  ].filter(Boolean);
  if (said.length) return { ...base, badge: 'count', label: `${name}: ${said.join(', ')}` };
  return { ...base, badge: null, label: `${name}: in sync` };
}
