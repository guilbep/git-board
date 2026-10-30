// A forge's icon in the bar (docs/design/issue-head/README.md, 16e and 18):
// GitHub's or GitLab's shape; nothing waiting, the bare icon; changes to
// send, a count with an up arrow; a run going, out of reach, changed on
// both sides: a small badge each, said in words in its name and tooltip.
// Pure: ForgeIcons counts the waiting changes from the events, as the side
// column's waitingFor does.
import type { Forge } from './api/board';

export type ForgeBadge = 'count' | 'syncing' | 'unreachable' | 'conflict' | null;
export interface ForgeState {
  kind: 'github' | 'gitlab' | 'other';
  badge: ForgeBadge;
  n: number;
  label: string;
}

export function forgeKind(f: Forge): ForgeState['kind'] {
  const s = `${f.name} ${f.url}`.toLowerCase();
  if (s.includes('github')) return 'github';
  if (s.includes('gitlab')) return 'gitlab';
  return 'other';
}

// The order when two apply: out of reach, then changed on both sides, then
// a run going, then the count.
export function forgeState(f: Forge, waiting: number, o: { running?: boolean; left?: string; conflict?: boolean } = {}): ForgeState {
  const name = `${f.name}${f.number ? ` #${f.number}` : ''}`;
  const kind = forgeKind(f);
  if (o.left) return { kind, badge: 'unreachable', n: waiting, label: `${name}: out of reach (${o.left})` };
  if (o.conflict) return { kind, badge: 'conflict', n: waiting, label: `${name}: changed here and there since the last sync` };
  if (o.running) return { kind, badge: 'syncing', n: waiting, label: `${name}: syncing now` };
  if (waiting > 0) return { kind, badge: 'count', n: waiting, label: `${name}: ${waiting} change${waiting === 1 ? '' : 's'} wait${waiting === 1 ? 's' : ''} to be sent` };
  return { kind, badge: null, n: 0, label: `${name}: in sync` };
}
