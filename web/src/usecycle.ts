// The issue's step, read and written (api/board.ts getCycle, cycleAction):
// shared by the Issue view's card (IssueCycle.vue) and the Cycle view
// (IssueCycleView.vue). One of the two is on the page at a time.
import { computed, ref, watch } from 'vue';
import { ApiError, cycleAction, editComment, getCycle, patchIssue, type Cycle } from '@/api/board';
import { message } from '@/issue';
import type { CardButton } from '@/cycle';

export function useCycle(props: { id: string; status: string; labels: string[] }, changed: () => void) {
  const cycle = ref<Cycle | null>(null);
  const error = ref('');
  const busy = ref(false);
  const output = ref('');
  const commit = ref('');
  const why = ref('');
  const needsWhy = ref(false);

  const has = (a: string) => !!cycle.value?.actions?.includes(a);
  const checkedBy = (i: number) => cycle.value?.evidence.checks?.find((c) => c.index === i)?.by;
  const mine = computed(() => !!cycle.value?.mine);

  async function load() {
    try {
      cycle.value = await getCycle(props.id);
      error.value = '';
    } catch (e) {
      error.value = `The step: ${message(e)}`;
    }
  }
  // the panel re-reads the thread after every write: labels moving re-reads the step
  watch(() => [props.id, props.labels.join('\n')], load, { immediate: true });

  async function act(action: string, body: Record<string, unknown> = {}) {
    busy.value = true;
    error.value = '';
    try {
      cycle.value = await cycleAction(props.id, action, body);
      needsWhy.value = false;
      why.value = '';
      changed();
      return true;
    } catch (e) {
      // a sign-off without a run at the head takes a reason instead
      if (action === 'signoff' && e instanceof ApiError && e.status === 400 && e.message.startsWith('no run')) needsWhy.value = true;
      error.value = message(e);
      return false;
    } finally {
      busy.value = false;
    }
  }

  async function write(fn: () => Promise<unknown>) {
    busy.value = true;
    error.value = '';
    try {
      await fn();
      changed();
      return true;
    } catch (e) {
      error.value = message(e);
      return false;
    } finally {
      busy.value = false;
    }
  }

  // set-risk and the status are fields: a PATCH, the thread re-read
  const patch = (body: Record<string, string>) => write(() => patchIssue(props.id, body));

  // the description (the criteria): no label moves, so the step is re-read here;
  // a failure re-reads the thread too, so a retry after a 409 has the new base.
  // ponytail: a 409 is shown, no "save anyway" here; the Issue view's pencil has it
  async function edit(commentId: string, body: string, base: string) {
    const ok = await write(() => editComment(props.id, commentId, body, base));
    if (ok) await load();
    else changed();
    return ok;
  }

  async function press(b: CardButton) {
    if (b.action === 'set-risk') return patch(b.body as Record<string, string>);
    return act(b.action, b.body);
  }

  async function postRun() {
    if (await act('verify', { output: output.value, commit: commit.value.trim() })) output.value = commit.value = '';
  }

  return { cycle, error, busy, output, commit, why, needsWhy, has, checkedBy, mine, act, patch, edit, press, postRun };
}
