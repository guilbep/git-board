<!-- The issue's head, v2 (docs/design/issue-head/README.md, rows 16 to
     19): the title alone, on one line where it can; the fields line, the
     status and its next step first, "by #N" for what blocks it, the top
     fields (a person's as faces, an empty one to fill), "+N more" or
     "More fields", the one way to the others; the branch line; then Spec
     and Linked
     issues while they have something to say. The identity (number,
     forges, bell) is the bar's. A value opens its own editor, dense
     (FieldEditor); "+N more" lists every field (FieldPanel). -->
<template>
  <div class="gb-head">
    <slot v-if="renaming" name="rename" />
    <h2 v-else :id="titleId" class="gb-head-title">{{ thread.shown_title }}</h2>

    <div class="gb-hline" role="group" aria-label="Fields">
      <StatusStepper :id="thread.id" :current="status" :closed="thread.status === 'closed'" :fields="fields" :busy="saving" :flag="statusFlag" :set-status="setStatus" @changed="emit('changed')" />
      <template v-for="b in line.blockedBy" :key="b">
        <button v-if="blocker(b)?.id" type="button" class="gb-hby" :title="blocker(b)?.title" @click="openBlocker(b)">by {{ b }}</button>
        <span v-else class="gb-hby">by {{ b }}</span>
      </template>
      <GbPopover v-for="f in slots" :key="f.name" :label="f.values.length ? `${displayName(f.name)}: ${said(f.name, f.values)}. Change it` : `${displayName(f.name)}: nobody yet. Choose someone`" :title="displayName(f.name)" button-class="gb-hv">
        <template #button>
          <span v-if="!f.values.length" class="gb-hassign" aria-hidden="true"><GbIcon name="user" small /></span>
          <span v-else-if="typeOf(f.name, fields) === 'person'" class="gb-hfaces">
            <span v-for="v in f.values.slice(0, 4)" :key="v" class="gb-avatar gb-avatar-sm" :class="{ 'gb-avatar-dark': v === myLogin }" :title="`${nameOf(v)}${v === myLogin ? ' (you)' : ''}`">{{ initials(nameOf(v)) }}</span>
            <span v-if="f.values.length > 4" class="gb-hfaces-more">+{{ f.values.length - 4 }}</span>
          </span>
          <template v-else>
            <ValueMark v-if="f.values.length === 1" :field="f.name" :value="f.values[0]!" />
            <span>{{ said(f.name, f.values) }}</span>
          </template>
          <span v-if="held(f.name)" class="gb-flag is-held">held</span>
        </template>
        <template #default="{ close }">
          <FieldPanel :id="thread.id" :names="[f.name]" :fields="fields" :labels="thread.labels" @changed="emit('changed')" @done="close()" />
        </template>
      </GbPopover>
      <GbPopover v-if="allFields.length" :label="line.more.length ? `${line.more.length} more field${line.more.length === 1 ? '' : 's'} set: ${line.more.map(displayName).join(', ')}. Every field` : 'Every field, to read or set'" title="Fields" button-class="gb-hv is-quiet">
        <template #button>{{ line.more.length ? `+${line.more.length} more` : 'More fields' }}</template>
        <template #default="{ close }">
          <FieldPanel :id="thread.id" :names="allFields" :fields="fields" :labels="thread.labels" @changed="emit('changed')" @done="close()" />
        </template>
      </GbPopover>
      <GbPopover v-if="shownLabels.length || listed" :label="shownLabels.length ? `Labels: ${shownLabels.join(', ')}${outside ? ', some outside the project’s list' : ''}` : 'Add a label'" title="Labels" :button-class="shownLabels.length ? 'gb-hv' : 'gb-hv is-quiet'">
        <template #button>
          <template v-if="shownLabels.length"><GbIcon :name="outside ? 'warn' : 'tag'" small :class="{ 'gb-hv-warn': outside }" /> {{ shownLabels[0] }}<template v-if="shownLabels.length > 1"> +{{ shownLabels.length - 1 }}</template></template>
          <template v-else>Label…</template>
        </template>
        <LabelPicker :labels="thread.labels" :busy="saving" @labels="(a, r) => emit('labels', a, r)" />
      </GbPopover>
    </div>

    <BranchLine :id="thread.id" :labels="thread.labels" :title="thread.title" />

    <IssueSide class="gb-head-extra" v-bind="side" :only="['spec', 'linked']" :spec="spec" :reviewer="reviewer" @changed="emit('changed')" @labels="(a, r) => emit('labels', a, r)" @reveal="(id) => emit('reveal', id)" @open="(id) => emit('open', id)" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { getRefs, type IssueRefs, type Thread } from '@/api/board';
import { displayName, fieldValues, formatValue, typeOf, type Fields } from '@/fields';
import { fieldLine, labelLine } from '@/headfields';
import { labelList, loadLabelList } from '@/labellist';
import { initials, me } from '@/me';
import type { SpecState } from '@/spec';
import { syncStatus } from '@/sync';
import BranchLine from '@/components/BranchLine.vue';
import FieldPanel from '@/components/FieldPanel.vue';
import LabelPicker from '@/components/LabelPicker.vue';
import GbIcon from '@/components/GbIcon.vue';
import GbPopover from '@/components/GbPopover.vue';
import IssueSide from '@/components/IssueSide.vue';
import StatusStepper from '@/components/StatusStepper.vue';
import ValueMark from '@/components/ValueMark.vue';

const props = defineProps<{
  thread: Thread;
  fields: Fields;
  names: string[]; // the board's fields, and the issue's own namespaces
  saving: boolean;
  titleId: string;
  renaming: boolean;
  setStatus: (s: 'open' | 'closed') => Promise<boolean>;
  spec?: SpecState | undefined;
  reviewer?: { name: string; role?: string | undefined } | undefined;
}>();
const emit = defineEmits<{ changed: []; labels: [add: string[], remove: string[]]; reveal: [id: string]; open: [id: string] }>();

const status = computed(() => fieldValues(props.thread.labels, 'status')[0] ?? '');
const line = computed(() => fieldLine(props.names, props.thread.labels));
// the labels: [labels] list's picked, then those outside it, warned
const labelsNow = computed(() => labelLine(props.thread.labels, labelList.value?.list ?? []));
const shownLabels = computed(() => [...labelsNow.value.picked, ...labelsNow.value.outside]);
const outside = computed(() => labelsNow.value.outside.length > 0);
const listed = computed(() => !!labelList.value?.defined && labelList.value.list.length > 0);
onMounted(() => void loadLabelList());
// the top fields in their order: those set, and an unset person field as
// an empty face to fill, in its place
const slots = computed(() =>
  line.value.order.flatMap((name) => {
    const set = line.value.top.find((t) => t.name === name);
    if (set) return [set];
    return typeOf(name, props.fields) === 'person' ? [{ name, values: [] as string[] }] : [];
  }),
);
// every field but the status, the stepper's: what +N more lists
const allFields = computed(() => props.names.filter((n) => n !== 'status'));
const said = (name: string, values: string[]) => values.map((v) => (typeOf(name, props.fields) === 'person' ? nameOf(v) : formatValue(name, v, props.fields))).join(', ');
// a person field holds logins: their names, as the identities say them
const nameOf = (login: string) => me.value?.identities?.find((p) => p.login?.toLowerCase() === login.replace(/^@/, '').toLowerCase())?.name ?? login;
const myLogin = computed(() => me.value?.me?.login ?? '');

// a conflict with the GitHub Project the last check held on this field
const held = (name: string) => !!syncStatus.value?.last?.conflicts.some((c) => c.bug_id === props.thread.id && c.field === name);
const statusFlag = computed(() => (held('status') ? 'held' : line.value.blockedBy.length ? 'blocked' : ''));

// what blocks it, to open: the issue named in blocked-by
const refs = ref<IssueRefs>({ out: {}, in: {} });
watch(
  () => [props.thread.id, line.value.blockedBy.join(' ')],
  async () => {
    if (!line.value.blockedBy.length) return;
    try {
      refs.value = await getRefs(props.thread.id);
    } catch {
      refs.value = { out: {}, in: {} };
    }
  },
  { immediate: true },
);
const blocker = (v: string) => refs.value.out['blocked-by']?.find((r) => r.value === v);
function openBlocker(v: string) {
  const id = blocker(v)?.id;
  if (id) emit('open', id);
}

// what every IssueSide here shares: today's editors, one section each
const side = computed(() => ({
  id: props.thread.id,
  title: props.thread.title,
  labels: props.thread.labels,
  names: props.names,
  fields: props.fields,
  forges: props.thread.forges,
  participants: props.thread.participants,
  events: props.thread.events,
  saving: props.saving,
}));
</script>
