// The git remotes, shared by the header (what waits to send and get) and
// the sync page, which writes every answer of its fetches, pulls and
// pushes here.
import { computed, ref } from 'vue';
import { getRemotes, type Remote } from '@/api/board';
import { repoName } from '@/releases';

export const remotes = ref<Remote[] | null>(null);

// the repository, as origin (else the first remote) names it: the header's
// name, the tab's title
export const repo = computed(() => {
  const r = remotes.value?.find((x) => x.name === 'origin') ?? remotes.value?.[0];
  return r ? repoName(r.url) : '';
});

export async function loadRemotes(): Promise<void> {
  try {
    remotes.value = (await getRemotes()).remotes;
  } catch {
    remotes.value = null; // the header just counts nothing for them
  }
}
