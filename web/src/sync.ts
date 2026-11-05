// The sync's state (GET /api/sync): the forges, the git remotes and the
// run, shared by the header and Settings › Sync. A run's steps come as
// progress events (live.ts): onStep keeps the last one on the run.
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
  if (e.kind === 'progress' && e.step && run?.state === 'running' && (!e.id || e.id === run.id)) run.steps = [...run.steps, e.step];
}
