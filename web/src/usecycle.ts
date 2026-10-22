// The issue's step, read and written (api/board.ts getCycle, cycleAction),
// for the issue's frame (components/IssueStep.vue).
import { ref, watch } from 'vue';
import { ApiError, cycleAction, editComment, getCycle, patchIssue, type Cycle } from '@/api/board';
import { message } from '@/issue';
import { patches, type CardButton } from '@/cycle';

// description: the issue's, its criteria; with the labels, what the step is read from here
export function useCycle(props: { id: string; labels: string[]; description: string }, changed: () => void) {
  const cycle = ref<Cycle | null>(null);
  const error = ref('');
  const busy = ref(false);
  const needsWhy = ref(false);

  const has = (a: string) => !!cycle.value?.actions?.includes(a);

  async function load() {
    try {
      cycle.value = await getCycle(props.id);
      error.value = '';
    } catch (e) {
      error.value = `The step: ${message(e)}`;
    }
  }
  // the panel re-reads the thread after every write: labels moving or the
  // description edited re-reads the step
  watch(() => [props.id, props.labels.join('\n'), props.description], load, { immediate: true });

  async function act(action: string, body: Record<string, unknown> = {}) {
    busy.value = true;
    error.value = '';
    try {
      cycle.value = await cycleAction(props.id, action, body);
      needsWhy.value = false;
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

  // the risk and the status are fields: a PATCH, the thread re-read
  const patch = (body: Record<string, string>) => write(() => patchIssue(props.id, body));

  // the description (the criteria): the thread re-read re-reads the step;
  // a failure re-reads the thread too, so a retry after a 409 has the new base.
  // ponytail: a 409 is shown, no "save anyway" here; the description's pencil has it
  async function edit(commentId: string, body: string, base: string) {
    const ok = await write(() => editComment(props.id, commentId, body, base));
    if (!ok) changed();
    return ok;
  }

  async function press(b: CardButton) {
    if (patches.includes(b.action)) return patch(b.body as Record<string, string>);
    return act(b.action, b.body);
  }

  return { cycle, error, busy, needsWhy, has, act, patch, edit, press };
}
