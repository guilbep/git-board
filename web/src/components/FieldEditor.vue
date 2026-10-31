<!-- One field, edited in place from the fields line (docs/design/issue-head,
     16a): a field with options is a menu, one click sets it ("Not set"
     clears it), several values are checkboxes; a person field lists the
     people to pick, and takes a login; a date, a number, a text or an
     issue is one small input, saved on Enter. Dense: rows of 2rem, no
     input takes the focus by itself. -->
<template>
  <div class="gb-fe" :aria-busy="busy">
    <p v-if="inherited" class="gb-fe-note">From the project: <b>{{ nameOf(inherited) }}</b></p>

    <ul v-if="kind === 'one'" class="gb-pop-list" role="menu" :aria-label="displayName(name)">
      <li v-for="o in choices" :key="o" role="none">
        <button type="button" role="menuitemradio" class="gb-pop-item" :aria-checked="o === values[0]" :disabled="busy" @click="setOne(o)">
          <ValueMark :field="name" :value="o" />
          <span>{{ formatValue(name, o, fields) }}</span>
          <GbIcon v-if="o === values[0]" name="check" small class="gb-pop-check" />
        </button>
      </li>
      <li v-if="values.length" role="none">
        <button type="button" role="menuitem" class="gb-pop-item is-quiet" :disabled="busy" @click="remove(values[0]!)">Not set</button>
      </li>
    </ul>

    <ul v-else-if="kind === 'many'" class="gb-pop-list" role="menu" :aria-label="displayName(name)">
      <li v-for="o in choices" :key="o" role="none">
        <button type="button" role="menuitemcheckbox" class="gb-pop-item" :aria-checked="values.includes(o)" :disabled="busy" @click="toggle(o)">
          <span class="gb-fe-box" :class="{ 'is-on': values.includes(o) }" aria-hidden="true"><GbIcon v-if="values.includes(o)" name="check" small /></span>
          <template v-if="kind === 'many' && type === 'person'">
            <span class="gb-avatar gb-avatar-sm" :class="{ 'gb-avatar-dark': o === myLogin }" aria-hidden="true">{{ initials(nameOf(o)) }}</span>
            <span>{{ nameOf(o) }}<span v-if="o === myLogin" class="gb-pop-muted"> · you</span></span>
          </template>
          <template v-else>
            <ValueMark :field="name" :value="o" />
            <span>{{ formatValue(name, o, fields) }}</span>
          </template>
        </button>
      </li>
    </ul>

    <ul v-if="kind === 'refs' && values.length" class="gb-pop-list">
      <li v-for="v in values" :key="v" class="gb-fe-ref">
        <button v-if="refOf(v)?.id" type="button" class="gb-fe-open" :title="`Open ${v}`" @click="emit('open', refOf(v)!.id!)">
          <GbIcon :name="refOf(v)!.status === 'closed' ? 'closed' : 'open'" small /> <span class="gb-mono">{{ v }}</span> {{ refOf(v)!.title }}
        </button>
        <span v-else>{{ v }}<span v-if="type === 'issue' && refsKnown" class="gb-pop-muted">: no such issue here</span></span>
        <button type="button" class="gb-fe-x" :aria-label="`Remove ${v}`" :title="`Remove ${v}`" :disabled="busy" @click="remove(v)"><GbIcon name="close" small /></button>
      </li>
    </ul>

    <form v-if="kind !== 'one' && entry" class="gb-fe-entry" @submit.prevent="submit">
      <input
        v-model="draft"
        class="epfl-input gb-fe-input"
        :type="inputType"
        :step="type === 'number' ? 'any' : undefined"
        :name="`gb-field-${name}`"
        :aria-label="entry"
        :placeholder="entry"
        autocomplete="off"
        autocapitalize="off"
        spellcheck="false"
        data-1p-ignore
        data-lpignore="true"
        data-form-type="other"
        :disabled="busy"
      />
      <button type="submit" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || !draft.trim()">{{ kind === 'value' ? 'Save' : 'Add' }}</button>
      <button v-if="kind === 'value' && values.length" type="button" class="gb-linkbtn" :disabled="busy" @click="remove(values[0]!)">Clear</button>
    </form>

    <p v-if="due" class="gb-fe-note" :class="{ 'is-error': due.endsWith('late') }">{{ due }}</p>
    <p v-if="held" class="gb-fe-note is-held">
      The Project says <b>{{ held.project || 'nothing' }}</b>; both values stay until someone picks one.
      <router-link to="/conflicts">Resolve in Conflicts</router-link>
    </p>
    <p v-if="waiting" class="gb-fe-note" role="status">Waits for the sync with the forges…</p>
    <p v-if="error" class="gb-fe-note is-error" role="alert">Not saved. {{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { IssueRef } from '@/api/board';
import { displayName, fieldValues, formatValue, typeOf, type Fields } from '@/fields';
import { dueNote } from '@/headfields';
import { patchField } from '@/fieldwrite';
import { message } from '@/issue';
import { initials, me } from '@/me';
import { syncStatus } from '@/sync';
import GbIcon from '@/components/GbIcon.vue';
import ValueMark from '@/components/ValueMark.vue';

// refs: the issues an issue field names, each to open, as the head read
// them; refsKnown: read for these values, so one with none is no issue here
const props = defineProps<{ id: string; name: string; fields: Fields; labels: string[]; refs: IssueRef[]; refsKnown: boolean }>();
const emit = defineEmits<{ changed: []; done: []; open: [id: string] }>();

const f = computed(() => props.fields[props.name]);
const type = computed(() => typeOf(props.name, props.fields));
const values = computed(() => fieldValues(props.labels, props.name));
// as configured; else one value, but for a person field and a field the
// issue already holds several values of, as the server decides (fields.Single)
const single = computed(() => (f.value ? f.value.single : type.value !== 'person' && values.value.length < 2));
const myLogin = computed(() => me.value?.me?.login ?? '');
const nameOf = (login: string) => me.value?.identities?.find((p) => p.login?.toLowerCase() === login.replace(/^@/, '').toLowerCase())?.name ?? login;

// one: a menu of options; many: checkboxes (options, or the people to
// pick); refs: the issues it names, and one to add; value: one input
const kind = computed<'one' | 'many' | 'refs' | 'value'>(() => {
  if (type.value === 'person') return 'many';
  if (type.value === 'issue') return 'refs';
  if (f.value?.options?.length) return single.value ? 'one' : 'many';
  return single.value ? 'value' : 'refs';
});
// the options, the values held outside them first; for a person, the
// viewer first, then the ones held, then whom the forges can assign
const choices = computed(() => {
  if (type.value === 'person') {
    const people = [...values.value, ...(f.value?.people ?? [])];
    if (myLogin.value) people.unshift(myLogin.value);
    return [...new Set(people.filter(Boolean))];
  }
  const opts = f.value?.options ?? [];
  return [...values.value.filter((v) => !opts.includes(v)), ...opts];
});
// the small input below: what it takes, or none
const entry = computed(() => {
  switch (kind.value) {
    case 'many':
      return type.value === 'person' ? 'Someone else: a login' : '';
    case 'refs':
      return type.value === 'issue' ? 'Add #12, or an issue id' : 'Add a value';
    case 'value':
      return type.value === 'date' ? 'A date' : type.value === 'number' ? `A number${f.value?.unit ? ` (${f.value.unit})` : ''}` : 'A value';
  }
  return '';
});
const inputType = computed(() => (type.value === 'date' ? 'date' : type.value === 'number' ? 'number' : 'search'));
const inherited = computed(() => (!values.value.length && f.value?.inherits ? f.value.inherits : ''));
const held = computed(() => syncStatus.value?.last?.conflicts.find((c) => c.bug_id === props.id && c.field === props.name));
// a date: late, today, in n days (the old side column's note)
const due = computed(() => (type.value === 'date' && values.value[0] ? dueNote(values.value[0], new Date()) : ''));

const refOf = (v: string) => props.refs.find((r) => r.value === v);

const draft = ref('');
watch(
  () => [props.name, values.value.join(' ')],
  () => (draft.value = kind.value === 'value' ? (values.value[0] ?? '') : ''),
  { immediate: true },
);

const busy = ref(false);
const waiting = ref(false);
const error = ref('');
async function write(value: string, close = false) {
  if (busy.value) return; // a double click writes once
  busy.value = true;
  error.value = '';
  try {
    await patchField(props.id, props.name, value, (w) => (waiting.value = w));
    emit('changed');
    if (close) emit('done');
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
    waiting.value = false;
  }
}
const setOne = (v: string) => (v === values.value[0] ? emit('done') : write(v, true));
const toggle = (v: string) => write(values.value.includes(v) ? `-${v}` : v);
const remove = (v: string) => write(`-${v}`, kind.value !== 'refs');
async function submit() {
  const v = draft.value.trim();
  if (!v || (kind.value === 'value' && v === values.value[0])) return;
  await write(v, kind.value === 'value');
  if (!error.value) draft.value = kind.value === 'value' ? v : '';
}
</script>
