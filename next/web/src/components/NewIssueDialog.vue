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
        <MarkdownEditor v-model="body" v-model:uploading="uploading" label="Description" @submit="create(false)" />
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
          <input autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" v-else :id="`new-${name}`" v-model="values[name]" class="epfl-input" :type="inputType(name)" :placeholder="hint(name)" :list="fields[name]?.people?.length ? `new-people-${name}` : undefined" />
          <datalist v-if="fields[name]?.people?.length" :id="`new-people-${name}`"><option v-for="p in fields[name]!.people" :key="p" :value="p" /></datalist>
          <small v-if="from[name]" class="gb-from-view">from the view: {{ name }}:{{ from[name] }}</small>
        </div>
        <div class="gb-field">
          <label class="epfl-label" for="new-labels">Labels</label>
          <input autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" id="new-labels" v-model="plain" class="epfl-input" placeholder="comma-separated" />
        </div>
      </div>
      <p v-if="error" class="gb-save-error" role="alert"><GbIcon name="error" small /> {{ error }}</p>
      <div v-if="created" class="gb-save-error gb-created" role="alert">
        <p><GbIcon name="error" small /> Created <b>{{ created.title }}</b>, without these fields:</p>
        <ul><li v-for="f in created.failed" :key="f">{{ f }}</li></ul>
        <p><button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="emit('created', created.id, false)">Open it to set them</button></p>
      </div>
    </form>

    <footer v-if="!(me && !me.me)" class="gb-dialog-foot">
      <p class="gb-hint gb-grow">Created as <b>{{ me?.me?.name }}</b>, saved in git-board at once. The forges get it at the next sync. <kbd>Ctrl</kbd> <kbd>Enter</kbd> creates.</p>
      <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy || uploading || !title.trim()" @click="create(true)">Create and open another</button>
      <button type="button" class="epfl-btn epfl-btn-primary" :disabled="busy || uploading || !title.trim()" @click="create(false)">{{ busy ? 'Creating…' : 'Create' }}</button>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { createIssue } from '@/api/board';
import { displayName, formatValue, type Fields } from '@/fields';
import { patchField } from '@/fieldwrite';
import { message } from '@/issue';
import { fieldWrites } from '@/newissue';
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
const uploading = ref(false); // a file uploads in the editor: sending waits for it (MarkdownEditor)
const error = ref('');
const titleInput = ref<HTMLInputElement | null>(null);
// an issue created with fields the server refused: said, and opened from here
const created = ref<{ id: string; title: string; failed: string[] } | null>(null);

// the fields the board's filter picks one value of (the server's facets)
const from = props.prefill ?? {};
const values = reactive<Record<string, string>>({});
for (const name of props.names) {
  // a configured single field with options starts at its first one (backlog)
  const first = props.fields[name]?.single ? props.fields[name]?.options?.[0] : undefined;
  values[name] = from[name] ?? (name === 'status' ? (first ?? '') : '');
}

const isMulti = (name: string) => (props.fields[name] ? !props.fields[name]!.single : name === 'assignee');
// the input follows the field's type: a date picker for a date, the right
// example for the rest (an issue is #12, a person a login or @me)
const inputType = (name: string) => (props.fields[name]?.type === 'date' && !isMulti(name) ? 'date' : 'text');
function hint(name: string): string {
  const t = props.fields[name]?.type;
  const many = isMulti(name) ? ', …' : '';
  if (t === 'issue') return `#12${many}`;
  if (t === 'person' || name === 'assignee') return `login or @me${many}`;
  if (t === 'date') return `YYYY-MM-DD${many}`;
  if (t === 'number') return `a number${many}`;
  if (t === 'iteration') return `a sprint or @current${many}`;
  return isMulti(name) ? 'a, b, …' : '';
}

async function create(another: boolean) {
  if (!title.value.trim() || busy.value || uploading.value) return;
  busy.value = true;
  error.value = '';
  created.value = null;
  let issue: { id: string; title: string };
  try {
    const labels = plain.value.split(',').map((l) => l.trim()).filter(Boolean);
    issue = await createIssue(title.value, body.value, labels);
  } catch (e) {
    error.value = `Not created: ${message(e)}`;
    busy.value = false;
    return;
  }
  // The issue exists from here: whatever a field write says, the board
  // gets it, and the form starts over, so a second Create is a new issue
  // on purpose, never the same one again. The fields as the panel sets
  // them (aliases resolve, single/multi apply), each on its own: a
  // refused one names itself, the others still go in, and a forge sync
  // running makes them wait for its end (fieldwrite.ts).
  const failed: string[] = [];
  for (const w of fieldWrites(values, isMulti)) {
    try {
      await patchField(issue.id, w.name, w.value);
    } catch (e) {
      failed.push(`${displayName(w.name)} ${w.value}: ${message(e)}`);
    }
  }
  busy.value = false;
  if (failed.length) {
    // stay open on what went wrong, the board reloaded behind
    emit('created', issue.id, true);
    created.value = { id: issue.id, title: issue.title, failed };
  } else emit('created', issue.id, another);
  if (another || failed.length) {
    title.value = '';
    body.value = '';
    titleInput.value?.focus();
  }
}

onMounted(() => titleInput.value?.focus());
</script>

<style scoped>
.gb-dialog { max-height: calc(100vh - 2rem); overflow-y: auto; }
.gb-dialog-body { display: grid; gap: .375rem; }
.gb-gap { margin-top: 1rem; }
.gb-save-error { display: flex; gap: .375rem; align-items: center; margin: .75rem 0 0; padding: .5rem .75rem; color: var(--danger-fg); background: var(--danger-bg); font-size: .875rem; }
.gb-created { display: block; }
.gb-created p, .gb-created ul { margin: 0; }
.gb-created ul { padding-left: 1.25rem; }
.gb-created p + ul, .gb-created ul + p { margin-top: .375rem; }
@media (max-width: 47.99rem) { .gb-fieldgrid { grid-template-columns: minmax(0, 1fr); } }
</style>
