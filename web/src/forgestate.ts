// A forge's icon in the bar (docs/design/issue-head/README.md, 16e and 18;
// docs/prd/issue-head.md, forge badges and new on the forge): GitHub's or
// GitLab's shape; nothing waiting, the bare icon; changes to send, a count
// with an up arrow; new there since the last sync (after the issue's own
// check), a count with a down arrow; a run going, out of reach, changed on
// both sides: a small badge each, said in words in its name and tooltip.
// Pure: ForgeIcons reads the server and counts the waiting changes from
// the events, as the side column's waitingFor did.
import type { BridgeTally, Forge, ForgeJob, IssueSyncResult, SyncBridge } from './api/board';

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
// a run going, then the counts (to send, new there: one pill). kind: its
// bridge's target, when one is found; else the link's name and URL say.
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

// The bridge an issue's forge link syncs through, from the link's URL: on
// its host, the bridge whose project the link is in; else the one there
// whose path no link spells (a GitLab table names its project by id).
// Two that can't be told apart name none: no badge rather than the wrong one.
export function bridgeFor(f: Forge, bridges: SyncBridge[]): SyncBridge | undefined {
  let u: URL;
  try {
    u = new URL(f.url);
  } catch {
    return undefined;
  }
  const path = u.pathname.toLowerCase();
  const on = bridges.filter((b) => b.host.toLowerCase() === u.host);
  const inside = on.filter((b) => b.path && path.startsWith(`/${b.path.toLowerCase()}/`));
  const pick = inside.length ? inside : on.filter((b) => !b.path);
  return pick.length === 1 ? pick[0] : undefined;
}

// What a check would bring in from a forge: every operation it would pull.
// Not the issues changed besides: changes counts their operations already,
// and an issue is "changed" for what it would send too.
export function incomingOf(t: BridgeTally | undefined): number {
  return t?.from.changes ?? 0;
}

// Each of an issue's forges with its state: syncing while a real run goes
// on; out of reach from the newer of the issue's own last run and the
// server's job (a run of the board's, running or done), by its bridge's
// tally; changed on both sides and new there from the issue's own last
// check, which ForgeIcons keeps while it holds. Its own real run brought in
// what was new and merged both sides: it says only which forges it reached.
export function forgeStates(
  forges: Forge[],
  waitingFor: (forge: string) => number,
  ctx: { bridges: SyncBridge[]; job: ForgeJob | null; check: IssueSyncResult | null },
): { forge: Forge; state: ForgeState }[] {
  const running = ctx.job?.state === 'running' && !ctx.job.dry_run;
  const jobNewer = !!ctx.job?.ended && (!ctx.check || Date.parse(ctx.job.ended) > Date.parse(ctx.check.ended));
  const dry = !!ctx.check?.dry_run;
  return forges.map((forge) => {
    const bridge = bridgeFor(forge, ctx.bridges);
    const last = bridge && ctx.job?.bridges[bridge.name];
    const checked = bridge && ctx.check?.bridges[bridge.name];
    return {
      forge,
      state: forgeState(forge, waitingFor(forge.name), {
        kind: bridge?.target,
        running,
        left: checked && !jobNewer ? checked.left : last?.left,
        conflict: dry && (checked?.conflicts ?? 0) > 0,
        incoming: dry ? incomingOf(checked) : 0,
      }),
    };
  });
}

// The issue's last run, while it still says something: until a real run
// of the board's is done after it (a whole sync from Settings, which the
// server's job shows), both times by the server's clock. A run that failed
// went part of the way or none: the check stays until a Refresh.
export function checkHolds(check: { ended: string } | undefined, job: ForgeJob | null): boolean {
  if (!check) return false;
  if (!job || job.dry_run || job.state !== 'done' || !job.ended) return true;
  return Date.parse(job.ended) < Date.parse(check.ended);
}
