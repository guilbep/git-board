// Who writes from git-board, shared by the header, the composer and the
// new-issue dialog; and the board this page shows (GET /api/board): where
// it is kept, the experimental features on.
import { computed, ref } from 'vue';
import { getBoard, getMe, getPeople, type BoardInfo, type MeResponse, type Person } from '@/api';
import { featureOn } from '@/experimental';

export const me = ref<MeResponse | null>(null);
export const board = ref<BoardInfo | null>(null);
// the board's people (GET /api/people): whom a person field names
export const people = ref<Person[]>([]);
// meLoaded: /api/me answered, or failed
export const meLoaded = ref(false);
// the work-in-progress features on (experimental.ts): off, their tabs and
// controls stay hidden; a failed /api/board has none
export const cycleOn = computed(() => featureOn(board.value?.experimental, 'cycle'));

export async function loadMe(): Promise<void> {
  const [m, b, p] = await Promise.allSettled([getMe(), getBoard(), getPeople()]);
  me.value = m.status === 'fulfilled' ? m.value : null;
  if (b.status === 'fulfilled') board.value = b.value;
  if (p.status === 'fulfilled') people.value = p.value;
  meLoaded.value = true;
}

export { initials } from '@/initials';

export { on, when } from '@/when';
