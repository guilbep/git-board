// The sync status, shared by the header (the conflicts its Sync counts),
// the sync page and the conflicts page, which update it after every check,
// sync or resolve.
import { ref } from 'vue';
import { getSync, type SyncStatus } from '@/api/board';

export const syncStatus = ref<SyncStatus | null>(null);

export async function loadSyncStatus(): Promise<void> {
  try {
    syncStatus.value = await getSync();
  } catch {
    syncStatus.value = null; // the header just counts no conflicts
  }
}
