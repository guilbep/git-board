// Who writes from git-board, shared by the header, the composer and the
// new-issue dialog; and whether the repository still waits for its switch.
import { ref } from 'vue';
import { getMe, type MeResponse } from '@/api/board';

export const me = ref<MeResponse | null>(null);

export async function loadMe(): Promise<void> {
  try {
    me.value = await getMe();
  } catch {
    me.value = null;
  }
}

export { initials } from '@/initials';

export { on, when } from '@/when';
