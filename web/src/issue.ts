// One issue's history and every write to it, for the panel and the full
// page. Each write answers with the whole history, re-read from git-bug:
// the page never patches it by hand.
import { ref, watch, type Ref } from 'vue';
import {
  addComment, changeLabels, editComment, getThread, setStatus, setTitle, type Thread,
} from '@/api/board';

export function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

export function useIssue(id: Ref<string>, onChange: (t: Thread) => void = () => {}) {
  const thread = ref<Thread | null>(null);
  const loading = ref(false);
  const error = ref(''); // loading failed
  const saving = ref(false);
  const saveError = ref(''); // the last write failed, e.g. a sync with GitHub/GitLab runs

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

  // run a write; true when it went through
  async function write(fn: () => Promise<Thread>): Promise<boolean> {
    saving.value = true;
    saveError.value = '';
    try {
      thread.value = await fn();
      onChange(thread.value);
      return true;
    } catch (e) {
      saveError.value = message(e);
      return false;
    } finally {
      saving.value = false;
    }
  }

  watch(id, () => void load(), { immediate: true });

  return {
    thread,
    loading,
    error,
    saving,
    saveError,
    load,
    comment: (body: string, status?: 'open' | 'closed') => write(() => addComment(id.value, body, status)),
    edit: (commentId: string, body: string) => write(() => editComment(id.value, commentId, body)),
    rename: (title: string) => write(() => setTitle(id.value, title)),
    setStatus: (status: 'open' | 'closed') => write(() => setStatus(id.value, status)),
    labels: (add: string[], remove: string[]) => write(() => changeLabels(id.value, add, remove)),
  };
}
