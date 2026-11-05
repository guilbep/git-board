<!-- The status and its next step, one control (docs/design/issue-head/
     README.md, 19d): the box opens the workflow's values; the arrow moves
     to the next one, named in its tooltip. The step onto the last value
     closes the issue, as the cycle's sign-off does (signedOff); a closed
     issue's arrow reopens it. A move fades the words out and in; nothing
     slides. A move during a forge sync waits for its end, as every write
     does (issue.ts). -->
<template>
  <span class="gb-stepper" role="group" aria-label="Status">
    <GbPopover :label="`Status: ${current ? formatValue('status', current, fields) : 'not set'}. Change it`" title="Status" focus=".gb-pop-item[aria-checked='true']" button-class="gb-stepper-box" :disabled="moving">
      <template #button>
        <ValueMark v-if="current" field="status" :value="current" />
        <span class="gb-stepper-word" :class="{ 'is-fading': moving }">{{ current ? formatValue('status', current, fields) : 'No status' }}</span>
        <span v-if="flag" class="gb-flag" :class="`is-${flag}`">{{ flag }}</span>
        <GbIcon name="down" small />
      </template>
      <template #default="{ close }">
        <ul class="gb-pop-list" role="menu" aria-label="Status">
          <li v-for="o in choices" :key="o" role="none">
            <button type="button" role="menuitemradio" class="gb-pop-item" :aria-checked="o === current" :disabled="moving" @click="pick(o, close)">
              <ValueMark field="status" :value="o" />
              <span>{{ formatValue('status', o, fields) }}</span>
              <GbIcon v-if="o === current" name="check" small class="gb-pop-check" />
            </button>
          </li>
        </ul>
        <p v-if="!options.length" class="gb-pop-note">No workflow in the project file: these are the values found.</p>
      </template>
    </GbPopover>
    <button v-if="next" type="button" class="gb-stepper-arrow" :class="`is-${next.icon}`" :aria-label="next.label" :title="next.label" :disabled="moving || busy" @click="step(next)">
      <GbIcon :name="next.icon" />
    </button>
    <span v-if="waiting" class="gb-stepper-note" role="status">Waits for the sync…</span>
    <span v-if="error" class="gb-stepper-error" role="alert"><GbIcon name="error" small /> Not saved. {{ error }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { ApiError, patchIssue } from '@/api/board';
import { formatValue, type Fields } from '@/fields';
import { untilSyncEnds } from '@/forges';
import { message } from '@/issue';
import { nextStep, type NextStep } from '@/nextstep';
import GbIcon from '@/components/GbIcon.vue';
import GbPopover from '@/components/GbPopover.vue';
import ValueMark from '@/components/ValueMark.vue';

const props = defineProps<{
  id: string;
  current: string; // the status value, '' when none
  closed: boolean;
  fields: Fields;
  busy: boolean; // the panel writes
  flag?: 'held' | 'blocked' | '' | undefined;
  setStatus: (s: 'open' | 'closed') => Promise<boolean>; // the panel's: open or close the issue
}>();
const emit = defineEmits<{ changed: [] }>();

const options = computed(() => props.fields.status?.options ?? []);
// the values offered: the workflow's, the current one first when it is not in it
const choices = computed(() => (props.current && !options.value.includes(props.current) ? [props.current, ...options.value] : options.value));
const next = computed(() => nextStep(options.value, props.current, props.closed));

const moving = ref(false);
const waiting = ref(false);
const error = ref('');

async function patch(value: string) {
  for (;;) {
    try {
      await patchIssue(props.id, { status: value });
      return;
    } catch (e) {
      if (!(e instanceof ApiError && e.code === 'syncing')) throw e;
      waiting.value = true;
      await untilSyncEnds();
      waiting.value = false;
    }
  }
}

async function run(fn: () => Promise<unknown>) {
  if (moving.value) return; // a double click moves once
  moving.value = true;
  error.value = '';
  try {
    await fn();
    emit('changed');
  } catch (e) {
    error.value = message(e);
  } finally {
    moving.value = false;
    waiting.value = false;
  }
}

function step(n: NextStep) {
  return run(async () => {
    if (n.reopens) return props.setStatus('open');
    if (n.target) await patch(n.target);
    if (n.closes) return props.setStatus('closed');
  });
}

function pick(value: string, close: (refocus?: boolean) => void) {
  close();
  if (value !== props.current) void run(() => patch(value));
}
</script>
