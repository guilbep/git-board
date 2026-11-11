// The sync's state (GET /api/sync): the forges, the git remotes and the
// run, shared by the header and Settings › Sync. A run's steps come as
// progress events (live.ts): onStep keeps the last count (current), adds
// the steps a person reads whole, and takes the tallies as they stand: a
// check of 1,000 issues is a few tallies, never 5,000 rows.
import { ref } from 'vue';
import { getSync, type SyncStatus } from '@/api';
import type { LiveEvent } from '@/live';

export const syncStatus = ref<SyncStatus | null>(null);

export async function loadSync(): Promise<void> {
  try {
    syncStatus.value = await getSync();
  } catch {
    syncStatus.value = null;
  }
}

export function onStep(e: LiveEvent): void {
  const run = syncStatus.value?.run;
  if (e.kind !== 'progress' || run?.state !== 'running' || (e.id && e.id !== run.id)) return;
  if (e.tallies) run.tallies = e.tallies;
  if (!e.step) return;
  if (e.step.what === 'progress') run.current = e.step;
  else run.steps.push(e.step);
}
