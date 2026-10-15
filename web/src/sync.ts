// The sync status, shared by the header (its Conflicts count) and the
// conflicts page, which updates it after every check, sync or resolve.
import { ref } from 'vue';
import { getSync, type SyncStatus } from '@/api/board';

export const syncStatus = ref<SyncStatus | null>(null);

export async function loadSyncStatus(): Promise<void> {
  try {
    syncStatus.value = await getSync();
  } catch {
    syncStatus.value = null; // the header just shows no Conflicts link
  }
}
