// One issue's thread and every write to it, for the panel and the full
// page. Each write sends the version the thread was read at, and answers
// the thread after it: the page never patches it by hand. A write the
// server refuses as stale (412: someone wrote in between) is kept, and the
// page asks: reload the issue and drop it, or overwrite (the same write
// without a version, the last writer wins, field by field).
import { ref, watch, type Ref } from 'vue';
import {
  addComment, ApiError, changesHeard, editComment, getCycle, getFollow, getThread, getThreadSince, isStale, message, patchIssue,
  type IssueChange, type Thread,
} from '@/api';
import { latestOnly } from '@/latest';
import { cycleOn } from '@/me';
import { loadUploadStatus } from '@/uploads';
import { apply as applyKept } from '@/local/store';

export { message };

// the threads last read, by issue: an issue opened again shows at once,
// then asks whether it moved (If-None-Match: a 304 when it didn't); a
// card hovered reads its thread before the click (prefetchThread)
// ponytail: the last 200 kept; an LRU if people open thousands a visit
const threads = new Map<string, Thread>();
const readAt = new Map<string, { at: number; heard: number }>(); // when, and at which change heard
function keepThread(t: Thread) {
  threads.delete(t.id);
  threads.set(t.id, t);
  readAt.set(t.id, { at: performance.now(), heard: changesHeard() });
  if (threads.size > 200) {
    const old = threads.keys().next().value!;
    threads.delete(old);
    readAt.delete(old);
  }
}
// fresh: read a moment ago, and nothing heard since: no need to ask again
function fresh(id: string): boolean {
  const r = readAt.get(id);
  return !!r && r.heard === changesHeard() && performance.now() - r.at < OPEN_KEEP;
}

// OPEN_KEEP is how long what an issue's opening read ahead serves it
export const OPEN_KEEP = 10_000;

// prefetchThread reads what opening an issue reads, at once rather than
// one after the other: its thread, its cycle (when on), whether one
// follows it, whether files attach; a card hovered, an issue opened
export function prefetchThread(id: string): Promise<void> {
  loadUploadStatus();
  void getFollow(id, OPEN_KEEP).catch(() => {});
  if (cycleOn.value) void getCycle(id, OPEN_KEEP).catch(() => {});
  if (threads.has(id)) return Promise.resolve();
  return getThread(id).then(keepThread, () => {}); // opened, it reads it again and says why
}

export function useIssue(id: Ref<string>, onChange: (t: Thread) => void = () => {}) {
  const thread = ref<Thread | null>(null);
  const loading = ref(false);
  const error = ref(''); // loading failed
  const saving = ref(false);
  const saveError = ref(''); // the last write failed
  const saveCode = ref(''); // its code: changed (a text edited meanwhile)
  // a write refused as stale, waiting for Reload or Overwrite
  const conflict = ref<null | (() => Promise<boolean>)>(null);

  const loads = latestOnly();
  async function load() {
    const current = loads.start();
    loading.value = true;
    error.value = '';
    const kept = threads.get(id.value);
    if (kept && thread.value?.id !== kept.id) {
      thread.value = kept; // shown at once, then asked whether it moved
      loading.value = false;
    }
    if (kept && fresh(id.value)) {
      thread.value = kept;
      loading.value = false;
      return;
    }
    try {
      const t = kept ? await getThreadSince(id.value, kept.version) : await getThread(id.value);
      if (t) keepThread(t);
      if (current()) thread.value = t ?? kept ?? thread.value;
    } catch (e) {
      if (current()) error.value = message(e);
    } finally {
      if (current()) loading.value = false;
    }
  }

  // a write's answer: the thread after it, fresher than a load asked
  // before it answered, which is dropped
  function written(t: Thread) {
    keepThread(t);
    loads.skip();
    loading.value = false;
    error.value = '';
    thread.value = t;
  }

  // run a write from the version read; true when it went through
  async function write(fn: (version: string) => Promise<Thread>, rethrow = false): Promise<boolean> {
    saving.value = true;
    saveError.value = '';
    saveCode.value = '';
    try {
      written(await fn(thread.value?.version ?? ''));
      conflict.value = null;
      onChange(thread.value!);
      return true;
    } catch (e) {
      if (isStale(e)) conflict.value = () => write(() => fn(''), rethrow);
      if (rethrow) throw e; // the caller shows it (an edit: "changed meanwhile")
      saveError.value = message(e);
      saveCode.value = e instanceof ApiError ? e.code : '';
      return false;
    } finally {
      saving.value = false;
    }
  }

  watch(id, () => {
    conflict.value = null;
    void load();
  }, { immediate: true });

  const patch = (change: IssueChange) => write((v) => patchIssue(id.value, v, change));

  // shown: a write the page shows at once, here and on the board's card
  // (the board kept, ADR 0023), back as it was if the server refuses it
  async function shown(p: Partial<Thread>, run: () => Promise<boolean>): Promise<boolean> {
    const before = thread.value;
    if (before) thread.value = { ...before, ...p };
    const undo = applyKept(id.value, p);
    const ok = await run();
    if (!ok) {
      if (before && thread.value?.version === before.version) thread.value = before;
      undo();
    }
    return ok;
  }
  const relabeled = (add: string[], remove: string[]): Partial<Thread> => {
    const now = (thread.value?.labels ?? []).filter((l) => !remove.includes(l));
    return { labels: [...now, ...add.filter((l) => !now.includes(l))] };
  };

  return {
    thread,
    loading,
    error,
    saving,
    saveError,
    saveCode,
    conflict,
    load,
    // Reload after a 412: the thread as it is now, the write dropped
    reload: () => {
      conflict.value = null;
      saveError.value = '';
      return load();
    },
    // Overwrite after a 412: the write again, over whatever it is now
    overwrite: () => conflict.value?.() ?? Promise.resolve(false),
    // a comment, or a status with one: "Close with comment"
    comment: (body: string, status?: 'open' | 'closed') =>
      status ? patch({ status, comment: body }) : write((v) => addComment(id.value, v, body)),
    // throws: the comment card shows why (ApiError code "changed")
    edit: (commentId: string, body: string, base: string) => write((v) => editComment(id.value, v, commentId, body, base), true),
    rename: (title: string) => shown({ title, shown_title: title }, () => patch({ title })),
    setStatus: (status: 'open' | 'closed') => shown({ status }, () => patch({ status })),
    labels: (add: string[], remove: string[]) => shown(relabeled(add, remove), () => patch({ labels: { add, remove } })),
    // field=value, field=-value removing one, field= clearing it
    set: (assignments: string[]) => patch({ set: assignments }),
  };
}
