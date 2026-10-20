<!-- A field's cell in the table, edited in place as in GitHub Projects: a
     click (or Enter) opens a select of the field's options, a number or
     date input, a text box, or for a field of several values (assignees)
     a small list to add to and remove from. Saved at once, shown before
     the server answers, undone if it refuses; Esc leaves it as it was. -->
<template>
  <td
    class="cell-field"
    :class="{ 'is-editing': editing }"
    :tabindex="editing ? -1 : 0"
    :aria-label="`${displayName(name)}: ${values.join(', ') || 'not set'}. Enter to edit`"
    @click.stop="open"
    @keydown.enter.self.prevent="open"
  >
    <span v-if="!editing" class="cell-tags">
      <span v-for="v in values" :key="v" class="gb-val" :class="colorClass(name, v)"><ValueMark chip :field="name" :value="v" />{{ formatValue(name, v, fields) }}</span>
      <span v-if="!values.length && inherited" class="cell-inherited">{{ formatValue(name, inherited, fields) }}</span>
      <span v-else-if="!values.length" class="cell-empty" aria-hidden="true">+</span>
    </span>

    <select
      v-else-if="kind === 'select'"
      ref="control"
      class="cell-control epfl-select"
      :aria-label="displayName(name)"
      :value="values[0] ?? ''"
      @change="setSingle(($event.target as HTMLSelectElement).value)"
      @keydown.esc.stop.prevent="close"
      @blur="close"
    >
      <option value="">Not set</option>
      <option v-for="o in options" :key="o" :value="o">{{ formatValue(name, o, fields) }}</option>
    </select>

    <input
      v-else-if="kind !== 'multi'"
      ref="control"
      class="cell-control epfl-input"
      autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other"
      :type="kind === 'number' ? 'number' : kind === 'date' ? 'date' : 'text'"
      :step="kind === 'number' ? 'any' : undefined"
      :aria-label="displayName(name)"
      :value="values[0] ?? ''"
      @keydown.enter.prevent="setSingle(($event.target as HTMLInputElement).value.trim())"
      @keydown.esc.stop.prevent="close"
      @change="kind === 'date' && setSingle(($event.target as HTMLInputElement).value)"
      @blur="setSingle(($event.target as HTMLInputElement).value.trim())"
    />

    <div v-else class="cell-multi" @focusout="onFocusOut" @keydown.esc.stop.prevent="close">
      <span v-for="v in values" :key="v" class="gb-val" :class="colorClass(name, v)">
        {{ formatValue(name, v, fields) }}
        <button type="button" class="cell-x" :aria-label="`Remove ${v}`" @click.stop="removeValue(v)">×</button>
      </span>
      <input
        ref="control"
        v-model="draft"
        class="cell-control epfl-input"
        autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other"
        :aria-label="`Add to ${displayName(name)}`"
        :placeholder="typeOf(name, fields) === 'person' ? 'Add, or @me' : 'Add'"
        :list="people.length ? `cell-people-${name}` : undefined"
        @keydown.enter.prevent="addValue"
      />
      <datalist v-if="people.length" :id="`cell-people-${name}`"><option v-for="p in people" :key="p" :value="p" /></datalist>
    </div>

    <p v-if="error" class="cell-error" role="alert">{{ error }}</p>
  </td>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue';
import { changeLabels, patchIssue, type Issue } from '@/api/board';
import { colorClass } from '@/colors';
import { displayName, fieldValues, formatValue, typeOf, type Fields } from '@/fields';
import { message } from '@/issue';
import { me } from '@/me';
import ValueMark from '@/components/ValueMark.vue';

const props = defineProps<{ issue: Issue; name: string; fields: Fields }>();
const emit = defineEmits<{ saved: [] }>();

const editing = ref(false);
const error = ref('');
const draft = ref('');
const control = ref<HTMLInputElement | HTMLSelectElement | null>(null);

const values = computed(() => fieldValues(props.issue.labels, props.name));
// a role the issue takes from the project, shown greyed while it sets none
const inherited = computed(() => fieldValues(props.issue.inherited ?? [], props.name)[0]);
// one value, as the server decides (fields.Single): configured, else not
// a person and not already several
const single = computed(() => props.fields[props.name]?.single ?? (typeOf(props.name, props.fields) !== 'person' && values.value.length < 2));
// a person field's picker: a datalist, never the options (those make
// board columns)
const people = computed(() => props.fields[props.name]?.people ?? []);
const options = computed(() => {
  const opts = props.fields[props.name]?.options ?? [];
  const cur = values.value[0];
  return cur && !opts.includes(cur) ? [cur, ...opts] : opts;
});
const kind = computed(() => {
  if (!single.value) return 'multi';
  if (props.fields[props.name]?.options?.length) return 'select';
  const t = typeOf(props.name, props.fields);
  return t === 'number' ? 'number' : t === 'date' ? 'date' : 'text';
});

async function open() {
  if (editing.value) return;
  error.value = '';
  editing.value = true;
  await nextTick();
  control.value?.focus();
  if (control.value instanceof HTMLSelectElement) control.value.showPicker?.();
}

function close() {
  editing.value = false;
}

const fieldLabels = () => props.issue.labels.filter((l) => l.startsWith(`${props.name}:`));

// optimistic: the labels change at once, back if the server refuses
async function apply(add: string[], remove: string[], patch?: string) {
  const before = [...props.issue.labels];
  props.issue.labels = [...before.filter((l) => !remove.includes(l)), ...add.filter((l) => !before.includes(l))];
  editing.value = false;
  try {
    if (patch !== undefined) await patchIssue(props.issue.id, { [props.name]: patch });
    else await changeLabels(props.issue.id, add, remove);
    emit('saved');
  } catch (e) {
    props.issue.labels = before;
    error.value = message(e);
  }
}

function setSingle(v: string) {
  if (!editing.value) return;
  if (v === (values.value[0] ?? '')) return close();
  if (!v) return void apply([], fieldLabels());
  void apply([`${props.name}:${v}`], fieldLabels(), v); // the server swaps and resolves @current
}

function addValue() {
  const v = draft.value.trim();
  if (!v) return;
  draft.value = '';
  const shown = v === '@me' ? (me.value?.me?.login ?? v) : v;
  void apply([`${props.name}:${shown}`], [], v).then(() => void open());
}

function removeValue(v: string) {
  void apply([], [`${props.name}:${v}`]).then(() => void open());
}

function onFocusOut(e: FocusEvent) {
  const next = e.relatedTarget as Node | null;
  if (!next || !(e.currentTarget as HTMLElement).contains(next)) close();
}
</script>

<style scoped>
.cell-field { cursor: pointer; }
.cell-field:hover:not(.is-editing) { box-shadow: inset 0 0 0 1px var(--border-strong); }
.cell-field:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: -0.125rem; }
.cell-tags { display: inline-flex; flex-wrap: wrap; gap: var(--space-1); min-height: 1.25rem; }
.cell-empty { color: var(--fg-subtle); opacity: 0; transition: opacity var(--t-fast); }
.cell-inherited { color: var(--fg-muted); font-style: italic; }
.cell-field:hover .cell-empty, .cell-field:focus-visible .cell-empty { opacity: 1; }
.cell-control { min-height: 2rem; padding: .125rem .5rem; font-size: var(--fs-sm); min-width: 8rem; }
select.cell-control { padding-right: 2rem; }
.cell-multi { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1); min-width: 12rem; }
.cell-multi .cell-control { flex: 1; min-width: 7rem; }
.cell-x { padding: 0 0 0 .125rem; border: 0; background: none; color: inherit; font: inherit; cursor: pointer; }
.cell-error { margin: .25rem 0 0; font-size: .75rem; color: var(--danger-fg); }
</style>
