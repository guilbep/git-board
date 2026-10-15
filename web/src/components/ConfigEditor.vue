<!-- .git-board.toml, edited in place: colored, checked as you type (where it
     breaks, what git-board won't use), reviewed as a diff, then saved; the
     server takes it at once. The textarea lies over its colored copy. -->
<template>
  <section class="cfg" aria-labelledby="set-file">
    <div class="cfg-head">
      <h2 id="set-file" class="h5">Project configuration</h2>
      <code class="cfg-path">{{ file?.path }}</code>
    </div>
    <p class="gb-hint">Fields, colors, views, sprints, the GitHub Project, attachments. The reference beside the editor follows your cursor; changes apply as soon as you save.</p>

    <div class="cfg-tools" role="toolbar" aria-label="Add to the file">
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="snippet(addView)">+ View</button>
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="snippet(addSprint)">+ Sprint</button>
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="snippet(addField)">+ Field</button>
      <span class="gb-grow" />
      <span class="cfg-state" :class="stateClass" role="status">{{ stateText }}</span>
    </div>

    <div class="cfg-body">
      <div class="cfg-main">
      <div class="cfg-editor" :class="{ 'cfg-editor-err': errLine }">
        <div ref="gutter" class="cfg-gutter" aria-hidden="true">
          <div v-for="n in lines.length" :key="n" :class="{ 'cfg-err': n === errLine }">{{ n }}</div>
        </div>
        <div class="cfg-code">
          <div ref="hl" class="cfg-hl" aria-hidden="true">
            <div v-for="(line, i) in lines" :key="i" class="cfg-line" :class="{ 'cfg-err': i + 1 === errLine }">
              <span v-for="(t, j) in tomlTokens(line)" :key="j" :class="t.cls && `tk-${t.cls}`">{{ t.text }}</span>
            </div>
          </div>
          <textarea
            ref="area"
            v-model="text"
            class="cfg-area"
            spellcheck="false"
            autocapitalize="off"
            autocomplete="off"
            aria-label=".git-board.toml"
            @scroll="syncScroll"
          @click="followCursor"
          @keyup="followCursor"
          @focus="followCursor"
            @keydown.meta.s.prevent="review"
            @keydown.ctrl.s.prevent="review"
          />
        </div>
      </div>

      <p v-if="check?.error" class="cfg-msg cfg-msg-err" role="alert">
        <button type="button" class="gb-linkbtn" @click="goTo(check.error.line, check.error.col)">Line {{ check.error.line }}</button>: {{ check.error.message }}
      </p>
      <p v-else-if="check?.message" class="cfg-msg cfg-msg-err" role="alert">{{ check.message }}</p>
      <ul v-if="check?.warnings.length" class="cfg-msg cfg-msg-warn">
        <li v-for="w in check.warnings" :key="w">{{ w }}</li>
      </ul>

      <div v-if="reviewing && dirty" class="cfg-diff" aria-label="Changes to save">
        <div v-for="(h, i) in hunks" :key="i" :class="`cfg-d cfg-d${h.op === ' ' ? 'same' : h.op === '+' ? 'add' : h.op === '-' ? 'del' : 'skip'}`">
          <span class="cfg-d-op">{{ h.op === '~' ? '' : h.op }}</span>{{ h.text }}
        </div>
      </div>

      <p v-if="saveError" class="epfl-alert epfl-alert-danger" role="alert">
        {{ saveError }}
        <button v-if="conflict" type="button" class="gb-linkbtn" @click="load()">Load the file as it is now (your edits are dropped)</button>
      </p>
      <p v-if="saved" class="cfg-msg cfg-msg-ok" role="status">Saved. Views, fields and sprints are live.</p>

      <div class="cfg-actions">
        <button v-if="!reviewing" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!dirty || !check?.ok" @click="review">Review changes</button>
        <template v-else>
          <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!dirty || !check?.ok || saving" @click="save">{{ saving ? 'Saving…' : 'Save and apply' }}</button>
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="reviewing = false">Keep editing</button>
        </template>
        <button v-if="dirty" type="button" class="gb-linkbtn" @click="discard">Discard changes</button>
      </div>
      </div>
      <aside class="cfg-doc" aria-labelledby="cfg-doc-title">
        <div class="cfg-doc-head">
          <h3 id="cfg-doc-title" class="cfg-doc-title">Reference</h3>
          <span class="gb-hint">follows your cursor</span>
        </div>
        <nav class="cfg-doc-nav" aria-label="Reference sections">
          <button v-for="d in docs" :key="d.id" type="button" class="cfg-doc-tab" :aria-current="d.id === docId ? 'true' : undefined" @click="docId = d.id">{{ d.title }}</button>
        </nav>
        <div v-if="doc" class="cfg-doc-body">
          <h4 class="cfg-doc-section">{{ doc.title }}</h4>
          <div v-if="doc.id === 'colors'" class="cfg-swatches" aria-label="The colors">
            <span v-for="c in palette" :key="c" class="gb-val" :class="`gb-c-${c}`">{{ c }}</span>
          </div>
          <div class="gb-md" v-html="doc.html" />
        </div>
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { ApiError, checkConfig, getConfigDocs, getConfigFile, saveConfig, type ConfigCheck, type ConfigFile, type DocSection } from '@/api/board';
import { addField, addSprint, addView, docSectionAt, type Snippet } from '@/configsnippets';
import { message } from '@/issue';
import { lineDiff } from '@/linediff';
import { tomlTokens } from '@/toml';

const emit = defineEmits<{ saved: [] }>();

const file = ref<ConfigFile | null>(null);
const text = ref('');
const check = ref<ConfigCheck | null>(null);
const reviewing = ref(false);
const saving = ref(false);
const saved = ref(false);
const saveError = ref('');
const conflict = ref(false);
const area = ref<HTMLTextAreaElement | null>(null);
const hl = ref<HTMLElement | null>(null);
const gutter = ref<HTMLElement | null>(null);

const lines = computed(() => text.value.split('\n'));

// the reference: the section of the table under the cursor, until a tab
// is picked by hand (kept until the cursor moves to another table)
const docs = ref<DocSection[]>([]);
const docId = ref('overview');
let cursorDoc = 'overview';
const doc = computed(() => docs.value.find((d) => d.id === docId.value) ?? docs.value[0]);
const palette = ['grey', 'blue', 'purple', 'amber', 'orange', 'green', 'red', 'teal', 'pink'];

function followCursor() {
  const a = area.value;
  if (!a) return;
  const id = docSectionAt(text.value, a.selectionStart);
  if (id !== cursorDoc) docId.value = cursorDoc = id;
}
const dirty = computed(() => file.value !== null && text.value !== file.value.text);
const errLine = computed(() => check.value?.error?.line ?? 0);

const stateClass = computed(() => (!check.value ? '' : check.value.ok ? (dirty.value ? 'cfg-state-dirty' : 'cfg-state-ok') : 'cfg-state-err'));
const stateText = computed(() => {
  const c = check.value;
  if (!c) return 'Checking…';
  if (!c.ok) return c.error ? `Error on line ${c.error.line}` : 'Not valid';
  const s = c.summary;
  const parts = [`${s.fields?.length ?? 0} fields`, `${s.views?.length ?? 0} views`, `${s.sprints} sprints`].join(' · ');
  return `${dirty.value ? 'Unsaved · ' : 'Valid · '}${parts}`;
});

// the diff to save, unchanged runs folded to two lines of context
const hunks = computed(() => {
  const d = lineDiff(file.value?.text ?? '', text.value);
  const keep = d.map((l, i) => l.op !== ' ' || d.slice(Math.max(0, i - 2), i + 3).some((x) => x.op !== ' '));
  const out: { op: ' ' | '+' | '-' | '~'; text: string }[] = [];
  let skipped = 0;
  d.forEach((l, i) => {
    if (keep[i]) {
      if (skipped) out.push({ op: '~', text: `… ${skipped} unchanged line${skipped === 1 ? '' : 's'}` });
      skipped = 0;
      out.push(l);
    } else skipped++;
  });
  if (skipped) out.push({ op: '~', text: `… ${skipped} unchanged line${skipped === 1 ? '' : 's'}` });
  return out;
});

let timer: ReturnType<typeof setTimeout> | undefined;
watch(text, () => {
  saved.value = false;
  clearTimeout(timer);
  timer = setTimeout(runCheck, 300);
});

async function runCheck() {
  const t = text.value;
  try {
    const c = await checkConfig(t);
    if (t === text.value) check.value = c;
  } catch (e) {
    check.value = { ok: false, message: message(e), warnings: [], summary: { fields: [], views: [], sprints: 0 } };
  }
}

function syncScroll() {
  const a = area.value;
  if (!a) return;
  if (hl.value) hl.value.style.transform = `translate(${-a.scrollLeft}px, ${-a.scrollTop}px)`;
  if (gutter.value) gutter.value.scrollTop = a.scrollTop;
}

async function select(start: number, end: number) {
  await nextTick();
  const a = area.value;
  if (!a) return;
  a.focus();
  a.setSelectionRange(start, end);
  // bring the selection into view: a line is 20px
  const line = text.value.slice(0, start).split('\n').length;
  a.scrollTop = Math.max(0, (line - 5) * 20);
  syncScroll();
  followCursor();
}

function goTo(line: number, col: number) {
  const before = lines.value.slice(0, line - 1).join('\n').length + (line > 1 ? 1 : 0);
  void select(before + col - 1, before + col - 1);
}

function snippet(fn: (t: string) => Snippet) {
  const s = fn(text.value);
  text.value = s.text;
  reviewing.value = false;
  void select(...s.select);
}

function review() {
  if (dirty.value && check.value?.ok) reviewing.value = true;
}

function discard() {
  text.value = file.value?.text ?? '';
  reviewing.value = false;
}

async function save() {
  if (!file.value) return;
  saving.value = true;
  saveError.value = '';
  conflict.value = false;
  try {
    check.value = await saveConfig(text.value, file.value.hash);
    await load(true);
    saved.value = true;
    emit('saved');
  } catch (e) {
    conflict.value = e instanceof ApiError && e.code === 'changed';
    saveError.value = `Not saved: ${message(e)}`;
  } finally {
    saving.value = false;
  }
}

async function load(keepSaved = false) {
  try {
    file.value = await getConfigFile();
    text.value = file.value.text;
    reviewing.value = false;
    saveError.value = '';
    conflict.value = false;
    await nextTick();
    saved.value = keepSaved;
    void runCheck();
  } catch (e) {
    saveError.value = message(e);
  }
}

onMounted(() => {
  void load();
  getConfigDocs().then(
    (d) => (docs.value = d),
    () => (docs.value = []),
  );
});
</script>

<style scoped>
.cfg { display: grid; gap: .5rem; margin-top: 2rem; }
.cfg h2, .cfg p { margin: 0; }
.cfg-head { display: flex; align-items: baseline; gap: 1rem; flex-wrap: wrap; }
.cfg-path { font-size: .8125rem; color: var(--fg-muted); overflow-wrap: anywhere; }
.cfg-tools { display: flex; align-items: center; gap: .5rem; flex-wrap: wrap; }
.gb-grow { flex: 1; }
.cfg-state { font-size: .8125rem; font-weight: 700; padding: .25rem .625rem; border: 1px solid var(--border); color: var(--fg-muted); }
.cfg-state-ok { color: var(--success-fg); border-color: var(--success-border); background: var(--success-bg); }
.cfg-state-dirty { color: var(--warning-fg); border-color: var(--warning-border); background: var(--warning-bg); }
.cfg-state-err { color: var(--danger-fg); border-color: var(--danger-border); background: var(--danger-bg); }

/* the editor: gutter + code; the textarea's text is transparent over the
   colored copy, both with the same font, line height and padding */
.cfg-body { display: grid; grid-template-columns: minmax(0, 1fr) 24rem; gap: 1rem; align-items: start; }
.cfg-main { display: grid; gap: .5rem; min-width: 0; }
@media (max-width: 68rem) { .cfg-body { grid-template-columns: minmax(0, 1fr); } }
.cfg-doc { display: grid; gap: .5rem; max-height: min(32rem, 60vh); min-height: 0; padding: .75rem 1rem; border: 1px solid var(--border); background: var(--bg-subtle); overflow: auto; }
@media (max-width: 68rem) { .cfg-doc { max-height: none; } }
.cfg-doc-head { display: flex; align-items: baseline; gap: .5rem; }
.cfg-doc-title { margin: 0; font-size: .8125rem; font-weight: 700; text-transform: uppercase; letter-spacing: .06em; color: var(--primary); }
.cfg-doc-nav { display: flex; flex-wrap: wrap; gap: .25rem; }
.cfg-doc-tab { padding: .125rem .5rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; font-size: .75rem; cursor: pointer; }
.cfg-doc-tab:hover { border-color: var(--fg); }
.cfg-doc-tab[aria-current] { border-color: var(--fg); background: var(--fg); color: var(--bg); font-weight: 700; }
.cfg-doc-section { margin: .5rem 0 0; font-size: 1.0625rem; font-weight: 700; }
.cfg-doc-body .gb-md { font-size: .875rem; }
.cfg-doc-body :deep(table) { width: 100%; border-collapse: collapse; font-size: .8125rem; }
.cfg-doc-body :deep(th), .cfg-doc-body :deep(td) { padding: .25rem .375rem; border-bottom: 1px solid var(--border); text-align: left; vertical-align: top; }
.cfg-doc-body :deep(pre) { padding: .5rem .75rem; background: var(--bg); border: 1px solid var(--border); overflow-x: auto; font-size: .8125rem; }
.cfg-doc-body :deep(code) { font-size: .8125rem; }
.cfg-swatches { display: flex; flex-wrap: wrap; gap: .25rem; }
.cfg-editor { display: flex; height: min(32rem, 60vh); border: 1px solid var(--border-strong); background: var(--bg); overflow: hidden; }
.cfg-editor:focus-within { border-color: var(--fg); }
.cfg-editor-err { border-color: var(--danger-border); }
.cfg-gutter, .cfg-hl, .cfg-area { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 13px; line-height: 20px; tab-size: 2; }
.cfg-gutter { flex: none; min-width: 3rem; padding: 12px 8px 12px 0; overflow: hidden; text-align: right; color: var(--fg-subtle); background: var(--bg-subtle); border-right: 1px solid var(--border); user-select: none; }
.cfg-code { position: relative; flex: 1; overflow: hidden; }
.cfg-hl { position: absolute; top: 0; left: 0; min-width: 100%; padding: 12px; white-space: pre; pointer-events: none; color: var(--fg); }
.cfg-line { min-height: 20px; }
.cfg-gutter .cfg-err { color: var(--danger-fg); font-weight: 700; }
.cfg-hl .cfg-err { background: var(--danger-bg); box-shadow: inset 3px 0 0 var(--danger); margin: 0 -12px; padding: 0 12px; }
.cfg-area { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; padding: 12px; border: 0; outline: none; resize: none; white-space: pre; overflow: auto; overflow-wrap: normal; color: transparent; background: transparent; caret-color: var(--fg); }
.cfg-area::selection { background: color-mix(in srgb, var(--info) 30%, transparent); }
.tk-comment { color: var(--fg-subtle); font-style: italic; }
.tk-table { color: var(--primary); font-weight: 700; }
.tk-key { color: var(--fg); font-weight: 600; }
.tk-string { color: var(--success-fg); }
.tk-value { color: var(--info-fg); }
.tk-punct { color: var(--fg-muted); }

.cfg-msg { font-size: .875rem; padding: .5rem .75rem; border-left: 3px solid; }
.cfg-msg-err { color: var(--danger-fg); background: var(--danger-bg); border-color: var(--danger); }
.cfg-msg-warn { margin: 0; padding-left: 1.75rem; color: var(--warning-fg); background: var(--warning-bg); border-color: var(--warning); }
.cfg-msg-ok { color: var(--success-fg); background: var(--success-bg); border-color: var(--success); }
.cfg-msg .gb-linkbtn { color: inherit; font-weight: 700; }

.cfg-diff { max-height: 20rem; overflow: auto; padding: .5rem 0; border: 1px solid var(--border); font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 12.5px; line-height: 19px; white-space: pre; }
.cfg-d { padding: 0 .75rem; }
.cfg-d-op { display: inline-block; width: 1.25rem; color: var(--fg-subtle); }
.cfg-dadd { background: var(--success-bg); color: var(--success-fg); }
.cfg-ddel { background: var(--danger-bg); color: var(--danger-fg); text-decoration: line-through; text-decoration-color: color-mix(in srgb, currentColor 40%, transparent); }
.cfg-dskip { color: var(--fg-subtle); font-style: italic; }

.cfg-actions { display: flex; align-items: center; gap: .75rem; flex-wrap: wrap; }
.gb-linkbtn { padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
</style>
