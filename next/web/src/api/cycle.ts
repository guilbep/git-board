// The development cycle (experimental; docs/rewrite/api.md, route 49): an
// issue's step and its evidence, and the Work page's rows. Off, both
// answer 404 experimental.
import { enc, get } from './http.ts';
import type { Cycle, CycleStages, WorkGroups } from './cycletypes.ts';

// the server keeps the stages in the evidence; the page reads them beside
export async function getCycle(id: string): Promise<Cycle> {
  const c = await get<Cycle & { evidence: Cycle['evidence'] & { stages?: CycleStages } }>(`/api/issues/${enc(id)}/cycle`);
  return { ...c, stages: c.stages ?? c.evidence.stages! };
}

export async function getWork(who: 'me' | 'team'): Promise<WorkGroups> {
  const g = await get<WorkGroups>('/api/cycle', { query: { who: who === 'team' ? 'team' : undefined } });
  return { needs: g.needs ?? [], waiting: g.waiting ?? [] };
}
