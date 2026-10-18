// The git remotes, shared by the header (what waits to send and get) and
// the sync page, which writes every answer of its fetches, pulls and
// pushes here.
import { ref } from 'vue';
import { getRemotes, type Remote } from '@/api/board';

export const remotes = ref<Remote[] | null>(null);

export async function loadRemotes(): Promise<void> {
  try {
    remotes.value = (await getRemotes()).remotes;
  } catch {
    remotes.value = null; // the header just counts nothing for them
  }
}
