// Who writes from git-board, shared by the header, the composer and the
// new-issue dialog; and whether the repository still waits for its switch.
import { computed, ref } from 'vue';
import { getMe, type MeResponse } from '@/api/board';

export const me = ref<MeResponse | null>(null);
// meLoaded: /api/me answered, or failed; until then the issue page doesn't
// know which layout to draw
export const meLoaded = ref(false);
// the cycle's page, not the classic one: a failed /api/me is classic
export const cycleView = computed(() => me.value?.view === 'cycle');
// classicPeek: someone on the cycle's page looks at issues as the classic
// page shows them, from the issue's switch: in this tab, until switched back
export const classicPeek = ref(false);

export async function loadMe(): Promise<void> {
  try {
    me.value = await getMe();
  } catch {
    me.value = null;
  } finally {
    meLoaded.value = true;
  }
}

export { initials } from '@/initials';

export { on, when } from '@/when';
