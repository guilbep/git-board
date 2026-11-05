// The cycle's steps (experimental), as the thread's flow divides an issue's
// history by them (flow.ts): the project's list, else the default seven.
// The rest of v1's cycle words come back with the cycle's page and its
// route (M6: GET /api/issues/{id}/cycle). Pure: node runs cycle.test.ts.
import type { CycleStages, StageStep } from './api/cycletypes.ts';

export const STEPS = ['', 'Intent', 'Risk', 'Plan', 'Tests', 'Code', 'Review', 'Verify'];
// the default list's ids by number: "6" is review
export const DEFAULT_IDS: readonly string[] = ['', 'intent', 'risk', 'plan', 'tests', 'code', 'review', 'verify'];
// listOf is the project's list ([cycle] steps), the default seven when the
// server sends none (core/cycle's list)
export const listOf = (s: CycleStages | undefined): StageStep[] =>
  s?.steps?.length ? s.steps : DEFAULT_IDS.slice(1).map((id, i) => ({ id, kind: id === 'verify' ? 'signoff' : id, name: STEPS[i + 1]! }));
// kindOf is an entry's kind as the step's number, 1 to 7: what its gate,
// evidence and workspace are read by
export const kindOf = (s: StageStep): number => (s.kind === 'signoff' ? 7 : DEFAULT_IDS.indexOf(s.kind));
