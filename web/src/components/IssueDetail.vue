<!-- Issue side panel (rendered inside a right-hand q-dialog, which traps
     focus, closes on Escape and gives focus back to the card or row).
     Every change is one PATCH; the parent gets the server's updated issue. -->
<template>
  <div class="issue-detail">
    <div class="detail-top">
      <div class="epfl-eyebrow">Issue {{ issue.id.slice(0, 8) }}</div>
      <button type="button" class="icon-button" aria-label="Close" @click="emit('close')">
        <svg viewBox="0 0 24 24" aria-hidden="true"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
      </button>
    </div>

    <h2 id="issue-detail-title" class="detail-title">{{ issue.title }}</h2>

    <div class="detail-meta">
      <span :class="['epfl-badge', issue.status === 'open' ? 'epfl-badge-tertiary' : 'epfl-badge-dark']">
        {{ issue.status === 'open' ? 'Open' : 'Closed' }}
      </span>
      <a
        v-if="issue.github_url"
        class="epfl-btn epfl-btn-secondary epfl-btn-sm github-link"
        :href="issue.github_url"
        target="_blank"
        rel="noopener noreferrer"
      >
        View on GitHub
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" /><polyline points="15 3 21 3 21 9" /><line x1="10" y1="14" x2="21" y2="3" /></svg>
        <span class="visually-hidden">(opens in a new tab)</span>
      </a>
    </div>

    <div class="detail-fields">
      <div v-for="name in names" :key="name" class="detail-field">
        <!-- single field with options: a select (no "unset": the API can't
             remove a single field's label, only swap it) -->
        <template v-if="fields[name]?.single && fields[name]?.options?.length">
          <label class="epfl-label" :for="`field-${name}`">{{ displayName(name) }}</label>
          <select
            :id="`field-${name}`"
            class="epfl-select"
            :value="single(name)"
            :disabled="saving"
            @change="onSelect(name, $event)"
          >
            <option v-if="!single(name)" value="" disabled>Not set</option>
            <option v-for="opt in selectOptions(name)" :key="opt" :value="opt">
              {{ formatValue(name, opt, fields) }}
            </option>
          </select>
        </template>

        <!-- single free-form field: saved on Enter or when leaving it -->
        <template v-else-if="isSingle(name)">
          <label class="epfl-label" :for="`field-${name}`">{{ displayName(name) }}</label>
          <input
            :id="`field-${name}`"
            class="epfl-input"
            type="text"
            :value="single(name)"
            :disabled="saving"
            @change="onText(name, $event)"
          />
        </template>

        <!-- multi field: removable chips plus an add box -->
        <template v-else>
          <span class="epfl-label" :id="`field-${name}-label`">{{ displayName(name) }}</span>
          <ul v-if="values(name).length" class="chips" :aria-labelledby="`field-${name}-label`">
            <li v-for="v in values(name)" :key="v" class="chip">
              {{ formatValue(name, v, fields) }}
              <button
                type="button"
                class="chip-remove"
                :aria-label="`Remove ${v} from ${name}`"
                :disabled="saving"
                @click="save(name, `-${v}`)"
              >
                <svg viewBox="0 0 24 24" aria-hidden="true"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
              </button>
            </li>
          </ul>
          <form class="chip-add" @submit.prevent="onAdd(name)">
            <input
              v-model="drafts[name]"
              class="epfl-input"
              type="text"
              :aria-label="`Add a value to ${name}`"
              placeholder="Add a value, or @me"
              :list="fields[name]?.options?.length ? `field-${name}-options` : undefined"
              :disabled="saving"
            />
            <datalist v-if="fields[name]?.options?.length" :id="`field-${name}-options`">
              <option v-for="opt in fields[name]?.options ?? []" :key="opt" :value="opt" />
            </datalist>
            <button type="submit" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="saving">Add</button>
          </form>
        </template>
      </div>
    </div>

    <div v-if="other.length" class="detail-other">
      <span class="epfl-label">Other labels</span>
      <ul class="chips">
        <li v-for="l in other" :key="l" class="chip">{{ l }}</li>
      </ul>
    </div>

    <p :class="['save-status', { done: !saving && saved }]" role="status">{{ saving ? 'Saving…' : saved }}</p>
    <div v-if="error" class="epfl-alert epfl-alert-danger" role="alert">
      <b>Not saved.</b> {{ error }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { patchIssue, type Issue } from '@/api/board';
import { displayName, fieldValues, formatValue, otherLabels, type Fields } from '@/fields';

const props = defineProps<{ issue: Issue; names: string[]; fields: Fields }>();
const emit = defineEmits<{ close: []; updated: [issue: Issue] }>();

const saving = ref(false);
const saved = ref(''); // what the last successful save did, in words
const error = ref('');
const drafts = reactive<Record<string, string>>({});

const other = computed(() => otherLabels(props.issue.labels, props.names));

const values = (name: string) => fieldValues(props.issue.labels, name);

// As configured; else one value, but for assignee and a field the issue
// already holds several values of — as the server decides (fields.Single).
function isSingle(name: string): boolean {
  const f = props.fields[name];
  return f ? f.single : name !== 'assignee' && values(name).length < 2;
}
const single = (name: string) => values(name)[0] ?? '';

// The configured options, plus the current value if it isn't one of them
// (set outside git-board), so the select never misreports it.
function selectOptions(name: string): string[] {
  const opts = props.fields[name]?.options ?? [];
  const current = single(name);
  return current && !opts.includes(current) ? [current, ...opts] : opts;
}

async function save(name: string, value: string): Promise<boolean> {
  saving.value = true;
  saved.value = '';
  error.value = '';
  try {
    emit('updated', await patchIssue(props.issue.id, { [name]: value }));
    saved.value = savedText(name, value);
    return true;
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : String(e);
    return false;
  } finally {
    saving.value = false;
  }
}

function savedText(name: string, value: string): string {
  const field = displayName(name);
  if (isSingle(name)) return `Saved: ${field} is now ${formatValue(name, value, props.fields)}.`;
  if (value.startsWith('-')) return `Saved: removed ${value.slice(1)} from ${field}.`;
  return `Saved: added ${value} to ${field}.`;
}

async function onSelect(name: string, event: Event) {
  const el = event.target as HTMLSelectElement;
  if (!(await save(name, el.value))) el.value = single(name); // show what's really stored
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
</script>

<style scoped>
.issue-detail {
  width: min(34rem, 100vw);
  height: 100%;
  overflow-y: auto;
  padding: var(--space-4);
  background: var(--bg);
  border-left: var(--border-w) solid var(--border);
  border-radius: 0;
  box-sizing: border-box;
}

.detail-top { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
.detail-top .epfl-eyebrow { margin: 0; }

.detail-title {
  margin: var(--space-2) 0 var(--space-3);
  font-size: var(--fs-h4);
  font-weight: var(--w-bold);
  line-height: var(--lh-normal);
  overflow-wrap: anywhere;
}

.detail-meta { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-3); }
.github-link { display: inline-flex; align-items: center; gap: var(--space-2); }

.detail-fields {
  display: grid;
  gap: var(--space-4);
  margin-top: var(--space-4);
  padding-top: var(--space-4);
  border-top: var(--border-w) solid var(--border-subtle);
}
.detail-other {
  margin-top: var(--space-4);
  padding-top: var(--space-4);
  border-top: var(--border-w) solid var(--border-subtle);
}

.chips { display: flex; flex-wrap: wrap; gap: var(--space-2); list-style: none; margin: 0 0 var(--space-2); padding: 0; }
.chip {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-1) var(--space-2);
  font-size: var(--fs-sm);
  border: var(--border-w) solid var(--epfl-gray-300);
  border-radius: var(--radius);
}
.chip-remove {
  display: inline-flex;
  padding: 0.125rem;
  color: var(--fg-muted);
  background: none;
  border: 0;
  border-radius: var(--radius);
  cursor: pointer;
  transition: color var(--t-fast);
}
.chip-remove:hover { color: var(--primary-hover); }
.chip-remove:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }
.chip-remove svg { width: 0.875rem; height: 0.875rem; }

.chip-add { display: flex; gap: var(--space-2); }
.chip-add .epfl-input { flex: 1; min-width: 0; }

.icon-button {
  display: inline-flex;
  padding: var(--space-1);
  color: var(--fg);
  background: none;
  border: var(--border-w) solid transparent;
  border-radius: var(--radius);
  cursor: pointer;
  transition: border-color var(--t-fast), color var(--t-fast);
}
.icon-button:hover { border-color: var(--border-strong); }
.icon-button:focus-visible { outline: var(--focus-w) solid var(--epfl-gray-600); outline-offset: 0.125rem; }

svg {
  width: 1.25rem;
  height: 1.25rem;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.github-link svg { width: 1rem; height: 1rem; }

.save-status { min-height: 1.5em; margin: var(--space-4) 0 var(--space-2); font-size: var(--fs-sm); color: var(--fg-muted); }
.save-status.done { color: var(--success-fg); }

.visually-hidden {
  position: absolute;
  width: 0.0625rem;
  height: 0.0625rem;
  overflow: hidden;
  clip-path: inset(50%);
  white-space: nowrap;
}
</style>
