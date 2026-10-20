// The sync with the forges, shared by the header (its running state)
// and the sync page. While a run goes on it's polled; edits refused
// meanwhile wait for its end (issue.ts).
import { ref } from 'vue';
import { getForges, type ForgeStatus, type Issue } from '@/api/board';
import { forgeNumbers } from '@/issuesync';
import type { LiveEvent } from '@/live';

export const forgeStatus = ref<ForgeStatus | null>(null);

// onStep keeps the running job's step from a progress event, as it comes:
// the poll would show it up to 1.5 s later. One issue's own sync (e.id)
// is its drawer's, not the job's.
export function onStep(e: LiveEvent): void {
  const job = forgeStatus.value?.job;
  if (e.kind === 'progress' && !e.id && e.step && job?.state === 'running') job.step = e.step;
}

const pollMs = 1500;
let polling: ReturnType<typeof setTimeout> | undefined;
let waiters: (() => void)[] = [];

export async function loadForges(): Promise<void> {
  let reached = true;
  try {
    forgeStatus.value = await getForges();
  } catch {
    reached = false; // unreachable for now: not "ended", ask again
  }
  const running = !reached || forgeStatus.value?.job?.state === 'running';
  if (running && !polling && (reached ? true : waiters.length > 0)) {
    polling = setTimeout(() => {
      polling = undefined;
      void loadForges();
    }, pollMs);
  }
  if (!running) {
    const done = waiters;
    waiters = [];
    done.forEach((w) => w());
  }
}

// untilSyncEnds resolves when no forge sync runs, asked one poll later at
// the soonest: a write refused for a running sync doesn't retry at once.
export function untilSyncEnds(): Promise<void> {
  return new Promise((resolve) => {
    waiters.push(resolve);
    setTimeout(() => void loadForges(), pollMs);
  });
}

// idLabel names an issue on a card: its project number (CO2-3075), else
// its number on the first forge it is still on (#2518), which its shown
// title leaves out, and its git-bug id.
export function idLabel(i: Issue, n = 7): string {
  const forge = forgeNumbers(i)[0];
  const number = i.number ?? (forge ? `#${forge}` : '');
  return number ? `${number} · ${i.id.slice(0, n)}` : i.id.slice(0, n);
}
