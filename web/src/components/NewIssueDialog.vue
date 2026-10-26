<!-- New issue: title, description, fields prefilled from the board's
     filter (the fields it picks one value of, e.g. the sprint of
     sprint:@current), "Create" or "Create and open another". The issue is created, then its fields are set as the
     panel sets them (aliases like @current and @me resolve there). -->
<template>
  <section class="gb-dialog" role="dialog" aria-modal="true" aria-labelledby="new-issue-title">
    <header class="gb-dialog-head">
      <h2 id="new-issue-title" class="h5">New issue{{ repo ? ` in ${repo}` : '' }}</h2>
      <span class="gb-grow" />
      <span class="gb-keys" aria-hidden="true"><kbd>Esc</kbd></span>
      <button type="button" class="gb-icon-btn" aria-label="Close (Esc)" @click="emit('close')"><GbIcon name="close" /></button>
    </header>

    <IdentityChooser v-if="me && !me.me" class="gb-dialog-body" />
    <form v-else class="gb-dialog-body" @submit.prevent="create(false)">
      <label class="epfl-label" for="new-title">Title (required)</label>
      <input autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" id="new-title" ref="titleInput" v-model="title" class="epfl-input" required @keydown.enter.ctrl.prevent="create(false)" />

      <span class="epfl-label gb-gap">Description</span>
      <div class="gb-composer">
        <MarkdownEditor v-model="body" label="Description" @submit="create(false)" />
      </div>

      <h3 class="gb-section-title gb-gap">Fields</h3>
      <p v-if="view" class="gb-hint">Prefilled from the view <b>{{ view }}</b>. Change any of them now or later in the panel.</p>
      <div class="gb-fieldgrid">
        <div v-for="name in names" :key="name" class="gb-field">
          <label class="epfl-label" :for="`new-${name}`">{{ displayName(name) }}</label>
          <select v-if="fields[name]?.options?.length && fields[name]?.single" :id="`new-${name}`" v-model="values[name]" class="epfl-select">
            <option value="">Not set</option>
            <option v-if="values[name] && !fields[name]?.options?.includes(values[name]!)" :value="values[name]">{{ values[name] }}</option>
            <option v-for="o in fields[name]?.options ?? []" :key="o" :value="o">{{ formatValue(name, o, fields) }}</option>
          </select>
          <input autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" v-else :id="`new-${name}`" v-model="values[name]" class="epfl-input" :placeholder="isMulti(name) ? 'comma-separated, or @me' : ''" :list="fields[name]?.people?.length ? `new-people-${name}` : undefined" />
          <datalist v-if="fields[name]?.people?.length" :id="`new-people-${name}`"><option v-for="p in fields[name]!.people" :key="p" :value="p" /></datalist>
          <small v-if="from[name]" class="gb-from-view">from the view: {{ name }}:{{ from[name] }}</small>
        </div>
        <div class="gb-field">
          <label class="epfl-label" for="new-labels">Labels</label>
          <input autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" id="new-labels" v-model="plain" class="epfl-input" placeholder="comma-separated" />
        </div>
      </div>
      <p v-if="error" class="gb-save-error" role="alert"><GbIcon name="error" small /> {{ error }}</p>
    </form>

    <footer v-if="!(me && !me.me)" class="gb-dialog-foot">
      <p class="gb-hint gb-grow">Created as <b>{{ me?.me?.name }}</b>, saved in git-board at once. The forges get it at the next sync. <kbd>Ctrl</kbd> <kbd>Enter</kbd> creates.</p>
      <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy || !title.trim()" @click="create(true)">Create and open another</button>
      <button type="button" class="epfl-btn epfl-btn-primary" :disabled="busy || !title.trim()" @click="create(false)">{{ busy ? 'Creating…' : 'Create' }}</button>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { createIssue, patchIssue } from '@/api/board';
import { displayName, formatValue, type Fields } from '@/fields';
import { message } from '@/issue';
import { me } from '@/me';
import GbIcon from '@/components/GbIcon.vue';
import IdentityChooser from '@/components/IdentityChooser.vue';
import MarkdownEditor from '@/components/MarkdownEditor.vue';

const props = defineProps<{ names: string[]; fields: Fields; view?: string; prefill?: Record<string, string>; repo?: string }>();
const emit = defineEmits<{ close: []; created: [id: string, another: boolean] }>();

const title = ref('');
const body = ref('');
const plain = ref('');
const busy = ref(false);
const error = ref('');
const titleInput = ref<HTMLInputElement | null>(null);

// the fields the board's filter picks one value of (the server's facets)
const from = props.prefill ?? {};
const values = reactive<Record<string, string>>({});
for (const name of props.names) {
  // a configured single field with options starts at its first one (backlog)
  const first = props.fields[name]?.single ? props.fields[name]?.options?.[0] : undefined;
  values[name] = from[name] ?? (name === 'status' ? (first ?? '') : '');
}

const isMulti = (name: string) => (props.fields[name] ? !props.fields[name]!.single : name === 'assignee');

async function create(another: boolean) {
  if (!title.value.trim() || busy.value) return;
  busy.value = true;
  error.value = '';
  try {
    const labels = plain.value.split(',').map((l) => l.trim()).filter(Boolean);
    const issue = await createIssue(title.value, body.value, labels);
    // the fields as the panel sets them: aliases resolve, single/multi apply
    const sets: Record<string, string> = {};
    for (const [name, v] of Object.entries(values)) {
      if (!v.trim()) continue;
      if (isMulti(name)) {
        for (const one of v.split(',').map((s) => s.trim()).filter(Boolean)) {
          await patchIssue(issue.id, { [name]: one });
        }
      } else sets[name] = v.trim();
    }
    if (Object.keys(sets).length) await patchIssue(issue.id, sets);
    emit('created', issue.id, another);
    if (another) {
      title.value = '';
      body.value = '';
      titleInput.value?.focus();
    }
  } catch (e) {
    error.value = `Not created, or created without all its fields: ${message(e)}`;
  } finally {
    busy.value = false;
  }
}

onMounted(() => titleInput.value?.focus());
</script>

<style scoped>
.gb-dialog { max-height: calc(100vh - 2rem); overflow-y: auto; }
.gb-dialog-body { display: grid; gap: .375rem; }
.gb-gap { margin-top: 1rem; }
.gb-save-error { display: flex; gap: .375rem; align-items: center; margin: .75rem 0 0; padding: .5rem .75rem; color: var(--danger-fg); background: var(--danger-bg); font-size: .875rem; }
@media (max-width: 47.99rem) { .gb-fieldgrid { grid-template-columns: minmax(0, 1fr); } }
</style>
