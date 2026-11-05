// The git remotes, shared by the header (what waits to send and get) and
// the sync page, which writes every answer of its fetches, pulls and
// pushes here.
import { computed, ref } from 'vue';
import { getRemotes, remoteAction, type Remote } from '@/api/board';
import { toCheck } from '@/inboxcheck';
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

// checkOrigin: on opening the page, origin is fetched when this server
// hasn't in five minutes (the inbox's FRESH), so that Sync's badge says
// what waits to get. A fetch changes nothing local: never a pull, Pull
// stays a click (docs/prd/clone-shows-board.md, 2). A refused fetch
// leaves the badge as it was: the sync page says why.
export async function checkOrigin(): Promise<void> {
  await loadRemotes();
  if (!toCheck(remotes.value ?? [], Date.now(), false).includes('origin')) return;
  try {
    remotes.value = (await remoteAction('origin', 'fetch')).remotes;
  } catch {
    // offline, a forge sync running, origin unreadable: nothing to count
  }
}
