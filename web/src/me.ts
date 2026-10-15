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

// initials for an avatar: "Pierre Guilbert" → "PG"
export function initials(name: string): string {
  const parts = name.trim().split(/[\s._-]+/).filter(Boolean);
  if (parts.length === 0) return '?';
  if (parts.length === 1) return parts[0]!.slice(0, 2).toUpperCase();
  return (parts[0]![0]! + parts[parts.length - 1]![0]!).toUpperCase();
}

const day = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
const dayTime = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
const time = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' });

// when: "today 10:12", "2 Oct", or with the time
export function when(iso: string, withTime = false): string {
  const d = new Date(iso);
  const today = new Date();
  if (d.toDateString() === today.toDateString()) return `today ${time.format(d)}`;
  return withTime ? dayTime.format(d) : day.format(d);
}
