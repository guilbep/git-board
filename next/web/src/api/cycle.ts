// The development cycle (experimental; docs/rewrite/api.md, routes 49 and
// 50): an issue's step and its evidence, its actions, and the Work page's
// rows. Off, each answers 404 experimental.
import { enc, get, send } from './http.ts';
import type { Cycle, CycleActBody, CycleStages, WorkGroups } from './cycletypes.ts';

type Read = Cycle & { evidence: Cycle['evidence'] & { stages?: CycleStages } };
// the server keeps the stages in the evidence; the page reads them beside
const withStages = (c: Read): Cycle => ({ ...c, stages: c.stages ?? c.evidence.stages! });

export async function getCycle(id: string): Promise<Cycle> {
  return withStages(await get<Read>(`/api/issues/${enc(id)}/cycle`));
}

// cycleAct does one of a step's actions from the version read (If-Match:
// a 412 stale writes nothing), and answers the cycle after it.
export async function cycleAct(id: string, action: string, version: string | undefined, body: CycleActBody = {}): Promise<Cycle> {
  return withStages(await send<Read>('POST', `/api/issues/${enc(id)}/cycle/${enc(action)}`, { body, ifMatch: version }));
}

export async function getWork(who: 'me' | 'team'): Promise<WorkGroups> {
  const g = await get<WorkGroups>('/api/cycle', { query: { who: who === 'team' ? 'team' : undefined } });
  return { needs: g.needs ?? [], waiting: g.waiting ?? [] };
}
