// Who writes from git-board: the repository's git-bug identity, shared by
// the header, the composer and the new-issue dialog.
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

const day = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
const dayTime = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
const time = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' });

// on: "on 2 Oct" or "today 10:12", for "opened on 2 Oct"
export function on(iso: string, withTime = false): string {
  const w = when(iso, withTime);
  return w.startsWith('today') ? w : `on ${w}`;
}

// when: "today 10:12", "2 Oct", or with the time
export function when(iso: string, withTime = false): string {
  const d = new Date(iso);
  const today = new Date();
  if (d.toDateString() === today.toDateString()) return `today ${time.format(d)}`;
  return withTime ? dayTime.format(d) : day.format(d);
}
