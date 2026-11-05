// One issue's thread and every write to it, for the panel and the full
// page. Each write sends the version the thread was read at, and answers
// the thread after it: the page never patches it by hand. A write the
// server refuses as stale (412: someone wrote in between) is kept, and the
// page asks: reload the issue and drop it, or overwrite (the same write
// without a version, the last writer wins, field by field).
import { ref, watch, type Ref } from 'vue';
import {
  addComment, ApiError, editComment, getThread, isStale, message, patchIssue,
  type IssueChange, type Thread,
} from '@/api';
import { latestOnly } from '@/latest';

export { message };

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
    try {
      const t = await getThread(id.value);
      if (current()) thread.value = t;
    } catch (e) {
      if (current()) error.value = message(e);
    } finally {
      if (current()) loading.value = false;
    }
  }

  // a write's answer: the thread after it, fresher than a load asked
  // before it answered, which is dropped
  function written(t: Thread) {
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
    rename: (title: string) => patch({ title }),
    setStatus: (status: 'open' | 'closed') => patch({ status }),
    labels: (add: string[], remove: string[]) => patch({ labels: { add, remove } }),
    // field=value, field=-value removing one, field= clearing it
    set: (assignments: string[]) => patch({ set: assignments }),
  };
}
