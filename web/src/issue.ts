// One issue's history and every write to it, for the panel and the full
// page. Each write answers with the whole history, re-read from git-bug:
// the page never patches it by hand.
import { ref, watch, type Ref } from 'vue';
import {
  addComment, ApiError, changeLabels, editComment, getThread, postReview, resolveReview, setStatus, setTitle, threadMeta,
  type ReviewRequest, type Thread, type ThreadOn,
} from '@/api/board';
import { untilSyncEnds } from '@/forges';

export function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

export function useIssue(id: Ref<string>, onChange: (t: Thread) => void = () => {}) {
  const thread = ref<Thread | null>(null);
  const loading = ref(false);
  const error = ref(''); // loading failed
  const saving = ref(false);
  const waiting = ref(false); // a sync with the forges runs: the change is saved after it
  const saveError = ref(''); // the last write failed
  const saveCode = ref(''); // its code: changed (someone edited meanwhile)

  let latest = 0;
  async function load() {
    const mine = ++latest;
    loading.value = true;
    error.value = '';
    try {
      const t = await getThread(id.value);
      if (mine === latest) thread.value = t;
    } catch (e) {
      if (mine === latest) error.value = message(e);
    } finally {
      if (mine === latest) loading.value = false;
    }
  }

  // run a write; true when it went through. Refused because a forge sync
  // runs, it waits for the end of the sync and goes again.
  async function write(fn: () => Promise<Thread>, rethrow = false): Promise<boolean> {
    saving.value = true;
    saveError.value = '';
    saveCode.value = '';
    try {
      for (;;) {
        try {
          thread.value = await fn();
          onChange(thread.value);
          return true;
        } catch (e) {
          if (!(e instanceof ApiError && e.code === 'syncing')) throw e;
          waiting.value = true;
          await untilSyncEnds();
          waiting.value = false;
        }
      }
    } catch (e) {
      if (rethrow) throw e; // the caller shows it (an edit: "changed meanwhile")
      saveError.value = message(e);
      saveCode.value = e instanceof ApiError ? e.code : '';
      return false;
    } finally {
      saving.value = false;
      waiting.value = false;
    }
  }

  watch(id, () => void load(), { immediate: true });

  return {
    thread,
    loading,
    error,
    saving,
    waiting,
    saveError,
    saveCode,
    load,
    comment: (body: string, status?: 'open' | 'closed', step = '', on?: ThreadOn) => write(() => addComment(id.value, body, status, step, on)),
    // "" reopens; a person's id, or "v2" once applied
    resolveThread: (root: string, value: string) => write(() => threadMeta(id.value, root, { 'thread:resolved': value })),
    // Apply: the description's edit, then the root's keys with the new
    // version's id. Throws as edit does; a refused edit writes nothing on
    // the root. ponytail: two calls, not atomic: a failed second one
    // leaves the edit and an open thread to resolve by hand
    apply: async (root: string, body: string, base: string, version: number) => {
      const created = () => thread.value?.events.find((e) => e.kind === 'created');
      if (!(await write(() => editComment(id.value, created()!.id, body, base), true))) return false;
      const edit = created()?.history?.at(-1)?.id ?? '';
      return write(() => threadMeta(id.value, root, { 'thread:resolved': `v${version}`, 'thread:applied': edit }));
    },
    // throws: the comment card shows why (ApiError code "changed")
    edit: (commentId: string, body: string, base: string) => write(() => editComment(id.value, commentId, body, base), true),
    rename: (title: string) => write(() => setTitle(id.value, title)),
    setStatus: (status: 'open' | 'closed') => write(() => setStatus(id.value, status)),
    labels: (add: string[], remove: string[]) => write(() => changeLabels(id.value, add, remove)),
    review: (req: ReviewRequest) => write(() => postReview(id.value, req)),
    resolve: (commentId: string, resolved: boolean) => write(() => resolveReview(id.value, commentId, resolved)),
  };
}
