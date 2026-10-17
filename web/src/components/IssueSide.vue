<!-- The panel's side column: fields (one PATCH each, as before), the
     "Held" marker of a field in conflict with the GitHub Project, plain
     labels, the forges and what waits for them, participants. -->
<template>
  <div class="gb-side">
    <section class="gb-side-sec" aria-labelledby="side-fields">
      <div class="gb-side-head">
        <h3 id="side-fields" class="gb-section-title">Fields</h3>
        <span class="gb-saved" role="status">{{ saving ? 'Saving…' : saved ? '✓ Saved' : '' }}</span>
      </div>
      <p v-if="!names.length" class="gb-hint">No fields yet. A label like <code>status:ready</code> or <code>prio:p1</code> is one.</p>
      <div class="gb-fields">
        <div v-for="name in names" :key="name" class="gb-field">
          <div class="gb-field-head">
            <label class="epfl-label gb-field-label" :for="`f-${name}`"><ValueMark v-if="isSingle(name) && single(name)" :field="name" :value="single(name)" />{{ displayName(name) }}</label>
            <router-link v-if="held(name)" to="/conflicts" class="gb-held-tag"><GbIcon name="warn" small /> Held</router-link>
          </div>
          <select v-if="isSingle(name) && fields[name]?.options?.length" :id="`f-${name}`" class="epfl-select" :value="single(name)" :disabled="saving" @change="onSelect(name, $event)">
            <option value="" disabled>Not set</option>
            <option v-for="opt in selectOptions(name)" :key="opt" :value="opt">{{ formatValue(name, opt, fields) }}</option>
          </select>
          <template v-else-if="isSingle(name)">
            <input
              :id="`f-${name}`"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other"
              class="epfl-input"
              :class="{ 'gb-date-input': typeOf(name, fields) === 'date' }"
              :type="typeOf(name, fields) === 'date' ? 'date' : typeOf(name, fields) === 'number' ? 'number' : 'text'"
              :step="typeOf(name, fields) === 'number' ? 'any' : undefined"
              :placeholder="typeOf(name, fields) === 'issue' ? '#12, or a git-bug id' : undefined"
              :value="single(name)"
              :disabled="saving"
              @change="onText(name, $event)"
            />
            <p v-if="typeOf(name, fields) === 'date' && single(name)" class="gb-field-note" :class="colorClass(name, single(name))">{{ dueNote(single(name)) }}</p>
            <button v-for="r in linked(name)" :key="r.value" type="button" class="gb-ref" :disabled="!r.id" @click="r.id && emit('open', r.id)">
              <GbIcon :name="r.status === 'closed' ? 'closed' : 'open'" small /> {{ r.title || `${r.value}: no such issue here` }}
            </button>
          </template>
          <template v-else>
            <ul v-if="values(name).length" class="gb-chips" :aria-label="displayName(name)">
              <li v-for="v in values(name)" :key="v" class="gb-chip" :class="colorClass(name, v)">
                <button v-if="typeOf(name, fields) === 'issue'" type="button" class="gb-chip-ref" :disabled="!refOf(name, v)?.id" @click="openRef(name, v)">{{ v }} {{ refOf(name, v)?.title ?? '' }}</button>
                <template v-else>{{ formatValue(name, v, fields) }}</template>
                <button type="button" class="gb-chip-x" :aria-label="`Remove ${v} from ${name}`" :disabled="saving" @click="save(name, `-${v}`)">×</button>
              </li>
            </ul>
            <form class="gb-addrow" @submit.prevent="onAdd(name)">
              <input autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" :id="`f-${name}`" v-model="drafts[name]" class="epfl-input" :placeholder="typeOf(name, fields) === 'issue' ? 'Add #12, or a git-bug id' : typeOf(name, fields) === 'person' ? 'Add, or @me' : 'Add'" :disabled="saving" />
              <button type="submit" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="saving"><GbIcon name="plus" small /> Add</button>
            </form>
          </template>
          <p v-if="held(name)" class="gb-held">
            The Project says <b>{{ held(name)!.project || 'nothing' }}</b>. Both values stay until someone picks one.
            <router-link to="/conflicts">Resolve in Conflicts</router-link>
          </p>
        </div>
      </div>
      <p v-if="error" class="gb-hint" role="alert"><b>Not saved.</b> {{ error }}</p>
    </section>

    <section v-if="incoming.length" class="gb-side-sec" aria-labelledby="side-linked">
      <h3 id="side-linked" class="gb-section-title">Linked issues</h3>
      <div v-for="[field, list] in incoming" :key="field" class="gb-linked">
        <p class="gb-linked-name">{{ inverseName(field) }} <span class="gb-hint">{{ list.length }}</span></p>
        <button v-for="r in list" :key="r.id" type="button" class="gb-ref" @click="r.id && emit('open', r.id)">
          <GbIcon :name="r.status === 'closed' ? 'closed' : 'open'" small /> {{ r.title }}
        </button>
      </div>
    </section>

    <section class="gb-side-sec" aria-labelledby="side-labels">
      <h3 id="side-labels" class="gb-section-title">Labels</h3>
      <ul class="gb-chips">
        <li v-for="l in plain" :key="l" class="gb-chip">
          {{ l }}
          <button type="button" class="gb-chip-x" :aria-label="`Remove label ${l}`" :disabled="saving" @click="emit('labels', [], [l])">×</button>
        </li>
      </ul>
      <form class="gb-addrow" @submit.prevent="addLabel">
        <input autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" v-model="newLabel" class="epfl-input" aria-label="New label" placeholder="Add label" :disabled="saving" />
        <button type="submit" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="saving || !newLabel.trim()"><GbIcon name="plus" small /> Add</button>
      </form>
    </section>

    <section v-if="forges?.length" class="gb-side-sec" aria-labelledby="side-forges">
      <h3 id="side-forges" class="gb-section-title">On the forges</h3>
      <ul class="gb-plain-list">
        <li v-for="f in forges" :key="f.url" class="gb-forgerow">
          <a :href="f.url" target="_blank" rel="noopener noreferrer">{{ f.name }}{{ f.number ? ` #${f.number}` : '' }} <GbIcon name="external" small /></a>
          <span class="gb-hint">{{ waitingFor(f.name) ? `${waitingFor(f.name)} change${waitingFor(f.name) === 1 ? '' : 's'} waiting` : 'in sync' }}</span>
        </li>
      </ul>
      <template v-if="waiting.length">
        <button type="button" class="gb-waiting-toggle" :aria-expanded="showWaiting" @click="showWaiting = !showWaiting">
          <GbIcon :name="showWaiting ? 'up' : 'down'" small />
          {{ showWaiting ? 'Hide' : 'See' }} the {{ waiting.length }} waiting change{{ waiting.length === 1 ? '' : 's' }}
        </button>
        <WaitingChanges v-if="showWaiting" :changes="waiting" :forges="forges" @reveal="(id) => emit('reveal', id)" />
      </template>
      <p class="gb-hint">Waiting changes go out at the next <router-link to="/sync#forges">sync with the forges</router-link>, or now:</p>
      <IssueSync :id="id" :waiting="waiting.length" on-forges @synced="emit('changed')" />
    </section>
    <section v-else-if="forgeStatus?.bridges.length" class="gb-side-sec" aria-labelledby="side-forges">
      <h3 id="side-forges" class="gb-section-title">On the forges</h3>
      <p class="gb-hint">Not on the forges yet. The next <router-link to="/sync#forges">sync with the forges</router-link> puts it there, or now:</p>
      <IssueSync :id="id" :waiting="waiting.length" :on-forges="false" @synced="emit('changed')" />
    </section>

    <section v-if="participants?.length" class="gb-side-sec" aria-labelledby="side-people">
      <h3 id="side-people" class="gb-section-title">Participants</h3>
      <ul class="gb-plain-list">
        <li v-for="p in participants" :key="p.id" class="gb-person">
          <span class="gb-avatar gb-avatar-sm" :class="{ 'gb-avatar-dark': p.id === meId }" aria-hidden="true">{{ initials(p.name) }}</span>
          {{ p.name }} <span v-if="p.id === meId" class="gb-hint">(you)</span>
        </li>
      </ul>
    </section>
  </div>
</template>

<script setup lang="ts">
import IssueSync from '@/components/IssueSync.vue';
import { forgeStatus } from '@/forges';
import ValueMark from '@/components/ValueMark.vue';
import { colorClass } from '@/colors';
import { computed, reactive, ref, watch } from 'vue';
import { getRefs, patchIssue, type Forge, type IssueEvent, type IssueRefs, type Person } from '@/api/board';
import { displayName, fieldValues, formatValue, typeOf, type Fields } from '@/fields';
import { message } from '@/issue';
import { initials, me } from '@/me';
import { syncStatus } from '@/sync';
import { waitingChanges } from '@/history';
import GbIcon from '@/components/GbIcon.vue';
import WaitingChanges from '@/components/WaitingChanges.vue';

const props = defineProps<{
  id: string;
  labels: string[];
  names: string[];
  fields: Fields;
  forges?: Forge[] | null | undefined;
  participants: Person[] | null;
  events: IssueEvent[];
  saving: boolean;
}>();
const emit = defineEmits<{ changed: []; labels: [add: string[], remove: string[]]; reveal: [id: string]; open: [id: string] }>();

// issue fields, both ways: the issues this one names, and those naming it
const refs = ref<IssueRefs>({ out: {}, in: {} });
async function loadRefs() {
  try {
    refs.value = await getRefs(props.id);
  } catch {
    refs.value = { out: {}, in: {} };
  }
}
watch(() => [props.id, props.labels.join(' ')], loadRefs, { immediate: true });
const linked = (name: string) => (typeOf(name, props.fields) === 'issue' ? (refs.value.out[name] ?? []) : []);
const refOf = (name: string, v: string) => refs.value.out[name]?.find((r) => r.value === v);
function openRef(name: string, v: string) {
  const id = refOf(name, v)?.id;
  if (id) emit('open', id);
}
const incoming = computed(() => Object.entries(refs.value.in).sort(([a], [b]) => a.localeCompare(b)));
// what an issue is to the issues naming it in that field
const inverses: Record<string, string> = { parent: 'Sub-issues', 'blocked-by': 'Blocks', blocks: 'Blocked by', 'depends-on': 'Needed by', 'duplicate-of': 'Duplicates' };
const inverseName = (field: string) => inverses[field] ?? `Named in ${displayName(field)}`;

// a date against today: late, today, in n days
function dueNote(iso: string): string {
  const days = Math.round((new Date(`${iso}T12:00:00`).getTime() - new Date(new Date().toDateString() + ' 12:00').getTime()) / 864e5);
  if (days < 0) return `${-days} day${days === -1 ? '' : 's'} late`;
  if (days === 0) return 'Today';
  return `In ${days} day${days === 1 ? '' : 's'}`;
}

const showWaiting = ref(false);
const waiting = computed(() => waitingChanges(props.events));

const busy = ref(false);
const saving = computed(() => busy.value || props.saving);
const saved = ref(false);
const error = ref('');
const drafts = reactive<Record<string, string>>({});
const newLabel = ref('');
const meId = computed(() => me.value?.me?.id ?? '');

const values = (name: string) => fieldValues(props.labels, name);
const single = (name: string) => values(name)[0] ?? '';
const plain = computed(() => props.labels.filter((l) => !l.includes(':')));

// As configured; else one value, but for a person field and a field the
// issue already holds several values of — as the server decides
// (fields.Single).
function isSingle(name: string): boolean {
  const f = props.fields[name];
  return f ? f.single : typeOf(name, props.fields) !== 'person' && values(name).length < 2;
}

function selectOptions(name: string): string[] {
  const opts = props.fields[name]?.options ?? [];
  const current = single(name);
  return current && !opts.includes(current) ? [current, ...opts] : opts;
}

// a conflict with the GitHub Project the last check held on this field
const held = (name: string) => syncStatus.value?.last?.conflicts.find((c) => c.bug_id === props.id && c.field === name);

// texts and changes not on a forge yet
const waitingFor = (forge: string) =>
  props.events.filter((e) => e.pending?.includes(forge) || e.edit_pending?.includes(forge)).length;

async function save(name: string, value: string): Promise<boolean> {
  busy.value = true;
  saved.value = false;
  error.value = '';
  try {
    await patchIssue(props.id, { [name]: value });
    saved.value = true;
    emit('changed');
    return true;
  } catch (e) {
    error.value = message(e);
    return false;
  } finally {
    busy.value = false;
  }
}

async function onSelect(name: string, event: Event) {
  const el = event.target as HTMLSelectElement;
  if (!(await save(name, el.value))) el.value = single(name);
}

async function onText(name: string, event: Event) {
  const el = event.target as HTMLInputElement;
  const value = el.value.trim();
  if (!value || value === single(name)) {
    el.value = single(name);
    return;
  }
  if (!(await save(name, value))) el.value = single(name);
}

async function onAdd(name: string) {
  const value = (drafts[name] ?? '').trim();
  if (value && (await save(name, value))) drafts[name] = '';
}

function addLabel() {
  const l = newLabel.value.trim();
  if (!l) return;
  emit('labels', [l], []);
  newLabel.value = '';
}
</script>

<style scoped>
.gb-chip-x { padding: 0 .25rem; border: 0; background: none; color: var(--fg-muted); cursor: pointer; font: inherit; }
.gb-chip-x:hover { color: var(--fg); }
.gb-addrow { display: flex; gap: .5rem; margin-top: .5rem; }
.gb-addrow .epfl-input { flex: 1; min-width: 0; }
.gb-forgerow { display: flex; justify-content: space-between; gap: .5rem; padding: .25rem 0; }
.gb-forgerow a { display: inline-flex; align-items: center; gap: .25rem; font-weight: 700; color: var(--fg); }
.gb-person { display: flex; align-items: baseline; gap: .5rem; padding: .25rem 0; }
.gb-person .gb-avatar { align-self: center; }
.gb-held-tag { display: inline-flex; align-items: center; gap: .25rem; text-decoration: none; }
.gb-field-label { display: inline-flex; align-items: center; gap: var(--space-2); }
.gb-field-note { display: inline-block; margin: .375rem 0 0; padding: 0 .375rem; font-size: .8125rem; font-weight: 700; background: var(--c-bg, transparent); color: var(--c-fg, var(--fg-muted)); }
.gb-ref { display: flex; align-items: center; gap: .375rem; width: 100%; margin-top: .375rem; padding: .25rem 0; border: 0; background: none; color: var(--fg); font: inherit; font-size: .875rem; text-align: left; cursor: pointer; }
.gb-ref:hover:not(:disabled) { text-decoration: underline; }
.gb-ref:disabled { color: var(--fg-muted); cursor: default; }
.gb-chip-ref { padding: 0; border: 0; background: none; color: inherit; font: inherit; cursor: pointer; text-align: left; }
.gb-chip-ref:hover:not(:disabled) { text-decoration: underline; }
.gb-linked + .gb-linked { margin-top: .75rem; }
.gb-linked-name { margin: 0; font-size: .8125rem; font-weight: 700; }
.gb-linked-name .gb-hint { display: inline; margin-left: .25rem; font-weight: 400; }
.gb-waiting-toggle { display: inline-flex; align-items: center; gap: .375rem; margin-top: .5rem; padding: 0; border: 0; background: none; font: inherit; font-size: .875rem; font-weight: 700; color: var(--fg); text-decoration: underline; text-underline-offset: 2px; cursor: pointer; }
</style>
