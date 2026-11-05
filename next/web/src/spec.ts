// The description's verdicts (docs/design/time-flow/protocol.md, spec:*):
// one comment per verdict, on a version. Pure: read from the thread.
import type { IssueEvent } from './api/board.ts';
import { bodyThreads } from './flow.ts';
import type { BodyVersion } from './sections.ts';

export interface Verdict { verdict: 'approve' | 'request-changes'; v: number; event: IssueEvent }

// the spec:verdict comments, in time order
export const verdicts = (events: IssueEvent[]): Verdict[] => events.flatMap((e) => {
  const verdict = e.meta?.['spec:verdict'];
  const v = Number(/^body@v([1-9]\d*)$/.exec(e.meta?.['spec:on'] ?? '')?.[1]);
  return (verdict === 'approve' || verdict === 'request-changes') && v ? [{ verdict, v, event: e } as Verdict] : [];
}).sort((a, b) => a.event.at.localeCompare(b.event.at));

export interface SpecState {
  v: number; // the current version
  on?: Verdict | undefined; // the last verdict on it
  before?: Verdict | undefined; // the last verdict on an earlier version
  approved?: Verdict | undefined; // the last approval of an earlier version: "approved v2, v3 awaits"
  open: number; // body threads open
  resolvedBy: number; // threads the current version resolved (thread:resolved "vN")
}

export function specOf(events: IssueEvent[], vs: BodyVersion[]): SpecState {
  const v = vs.length;
  const all = verdicts(events);
  const earlier = all.filter((x) => x.v < v);
  const threads = bodyThreads(events);
  return {
    v,
    on: all.filter((x) => x.v === v).at(-1),
    before: earlier.at(-1),
    approved: earlier.filter((x) => x.verdict === 'approve').at(-1),
    open: threads.filter((t) => !t.resolved).length,
    resolvedBy: threads.filter((t) => t.event.meta?.['thread:resolved'] === `v${v}`).length,
  };
}

// who may give it: the spec role's holder when it has one, else anyone
// but the description's author (the server's rule, cycle.go)
export function mayGive(people: Record<string, string> | undefined, author: string | undefined, me: string | undefined): boolean {
  const eq = (a?: string, b?: string) => !!a && !!b && a.toLowerCase() === b.toLowerCase();
  const holder = people?.spec;
  return holder ? eq(holder, me) : !!me && !eq(author, me);
}
