// The page's interactions: what a person does (open an issue, save a
// field), timed from the act to the paint after its last request. Each
// request it makes carries its trace (traceparent), so the server's spans
// line up under it; one over BUDGET_MS is sent to POST /api/slow, which
// logs it and, with tracing on, records it as its trace's root
// (docs/guide/performance.md, Traces). Pure but for the clock and the
// frames: node runs interaction.test.ts on it.

export const BUDGET_MS = 200;

export interface Interaction {
  name: string;
  trace: string; // 32 hex
  span: string; // 16 hex: the interaction's own span, its requests' parent
  start: number; // performance.now()
  asked?: boolean; // a request of its own started
  painted?: number; // its first frame
}

// QUIET_MS is how long an act may wait before its first request (a filter
// asked once typing pauses): one that asks nothing by then ends at its
// first frame
export const QUIET_MS = 120;

const hex = (bytes: number) => {
  const a = new Uint8Array(bytes);
  crypto.getRandomValues(a);
  return [...a].map((b) => b.toString(16).padStart(2, '0')).join('');
};

let current: Interaction | null = null;

// the interactions ended, the last 50, as the page timed them: the
// nightly reads them (web/perf/interactions.perf.ts: window.__gbActs)
export const settled: { name: string; ms: number }[] = [];
if (typeof window !== 'undefined') {
  const w = window as unknown as { __gbActs: typeof settled; __gbNow: () => unknown };
  w.__gbActs = settled;
  w.__gbNow = () => ({ current: current?.name ?? null, inflight }); // what goes on: the nightly's checks
}
let inflight = 0;
let report: (i: Interaction, ms: number) => void = () => {};

// onSlow sets where a slow interaction goes (the API, from http.ts)
export function onSlow(fn: (i: Interaction, ms: number) => void): void {
  report = fn;
}

// active is the interaction going on, if one is
export const active = (): Interaction | null => current;

// begin starts an interaction, ending the one before: a person acts once
export function begin(name: string): Interaction {
  const i: Interaction = { name, trace: hex(16), span: hex(8), start: performance.now() };
  current = i;
  frame(() => (i.painted ??= performance.now()));
  setTimeout(settleSoon, QUIET_MS); // one that asks nothing
  return i;
}

// traceparent is a request's header: the interaction's trace, its span as
// the parent; a request outside one is a trace of its own
export function traceparent(): string {
  return current ? `00-${current.trace}-${current.span}-01` : `00-${hex(16)}-${hex(8)}-01`;
}

// started and finished count a request in flight: the interaction ends
// once none is, at the next paint
export function started(): void {
  inflight++;
  if (current) current.asked = true;
}
export function finished(): void {
  inflight = Math.max(0, inflight - 1);
  if (!inflight) settleSoon();
}

const frame = (fn: () => void) => (typeof requestAnimationFrame === 'function' ? requestAnimationFrame(() => setTimeout(fn, 0)) : setTimeout(fn, 0));

// settleSoon ends the act at the next paint once nothing is in flight:
// after its requests; one that asked nothing, at its first frame, once
// QUIET_MS showed it won't
function settleSoon() {
  const i = current;
  if (!i) return;
  if (!i.asked && performance.now() - i.start < QUIET_MS) return; // its first request may come
  frame(() => {
    if (current !== i || inflight) return; // another began, or a request followed
    current = null;
    const end = i.asked ? performance.now() : (i.painted ?? performance.now());
    const ms = end - i.start;
    settled.push({ name: i.name, ms });
    if (settled.length > 50) settled.shift();
    if (ms > BUDGET_MS) report(i, ms);
  });
}

// routeName is a page's address as a pattern: an issue's id, a number,
// a tag become :id
export function routeName(path: string): string {
  return path.split('/').map((p) => (/^[0-9a-f]{7,}$/.test(p) || /^\d+$/.test(p) || /^[A-Z]+-\d+$/.test(p) ? ':id' : p)).join('/') || '/';
}
