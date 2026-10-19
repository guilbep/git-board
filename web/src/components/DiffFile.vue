<!-- One file of the Changes tab: its diff, loaded when opened, the line
     comments on their lines (or above the diff when their line isn't in
     this view), and a comment box under a line number clicked. -->
<template>
  <section class="gb-df" :data-file="file.path">
    <header class="gb-df-head">
      <button type="button" class="gb-df-toggle" :aria-expanded="open" @click="toggle">
        <GbIcon name="down" small :class="{ 'gb-df-shut': !open }" />
        <span class="gb-mono gb-df-path">{{ file.from ? `${file.from} → ` : '' }}{{ file.path }}</span>
      </button>
      <span class="gb-df-stat"><span class="gb-df-add">+{{ file.added }}</span> <span class="gb-df-del">−{{ file.deleted }}</span></span>
      <span v-if="threads.length" class="gb-tag">{{ threads.length }} thread{{ threads.length === 1 ? '' : 's' }}</span>
      <span class="gb-grow" />
      <label v-if="!file.generated" class="epfl-check gb-df-viewed">
        <input type="checkbox" :checked="viewed" @change="emit('viewed', !viewed)" /> Viewed
      </label>
    </header>

    <div v-if="open" class="gb-df-body">
      <ReviewThreadView v-for="t in loose" :key="t.root.id" :thread="t" :busy="busy" :reply="reply" :resolve="resolve" loose />
      <p v-if="file.binary" class="gb-hint gb-df-note">Binary file.</p>
      <p v-else-if="error" class="gb-hint gb-df-note" role="alert"><b>Couldn’t read the diff.</b> {{ error }}</p>
      <p v-else-if="!loaded" class="gb-hint gb-df-note" role="status">Loading…</p>
      <table v-else class="gb-df-rows">
        <tbody>
          <template v-for="(r, i) in rows" :key="i">
            <tr v-if="r.op === '@'" class="gb-df-hunk"><td colspan="3">{{ r.text }}</td></tr>
            <tr v-else :class="r.op === '+' ? 'gb-df-radd' : r.op === '-' ? 'gb-df-rdel' : ''">
              <td class="gb-df-num">
                <button v-if="r.old !== undefined && oldClickable" type="button" :aria-label="`Comment on line ${r.old} before`" @click="compose(i, 'old', r.old)">{{ r.old }}</button>
                <span v-else>{{ r.old }}</span>
              </td>
              <td class="gb-df-num">
                <button v-if="r.new !== undefined" type="button" :aria-label="`Comment on line ${r.new}`" @click="compose(i, 'new', r.new)">{{ r.new }}</button>
              </td>
              <td class="gb-df-text"><span class="gb-df-op" aria-hidden="true">{{ r.op }}</span>{{ r.text }}</td>
            </tr>
            <tr v-for="t in placed.get(i) ?? []" :key="t.root.id" class="gb-df-thread">
              <td colspan="3"><ReviewThreadView :thread="t" :busy="busy" :reply="reply" :resolve="resolve" /></td>
            </tr>
            <tr v-if="draftAt?.row === i" class="gb-df-thread">
              <td colspan="3">
                <div class="gb-df-draft">
                  <MarkdownEditor v-model="draft" :label="`Comment on line ${draftAt.line}`" :disabled="busy" @submit="send" />
                  <div class="gb-composer-foot">
                    <span class="gb-hint">On the pull request at the next sync.</span>
                    <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="draftAt = null">Cancel</button>
                    <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !draft.trim()" @click="send">Comment</button>
                  </div>
                </div>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
      <div v-if="truncated || (!autoOpens(file) && !file.binary)" class="gb-df-note">
        <span v-if="truncated" class="gb-hint">The first {{ LARGE_LIMIT.toLocaleString('en') }} lines.</span>
        <pre class="gb-dev-cmd"><code>{{ command }}</code></pre>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="copy">
          <GbIcon v-if="copied" name="check" small /> {{ copied ? 'Copied' : 'Copy' }}
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { getFileDiff, type ChangeRange, type FileStat, type ReviewRequest } from '@/api/board';
import { message } from '@/issue';
import { autoOpens, diffCommand, LARGE_LIMIT, lineIn, parseUnified, type ReviewThread } from '@/review';
import GbIcon from '@/components/GbIcon.vue';
import MarkdownEditor from '@/components/MarkdownEditor.vue';
import ReviewThreadView from '@/components/ReviewThread.vue';

const props = defineProps<{
  id: string;
  file: FileStat;
  range: Required<ChangeRange>; // the view: from…to of a branch
  base: string; // where the branch left the base
  head: string;
  threads: ReviewThread[]; // this file's
  viewed: boolean;
  busy: boolean;
  comment: (req: ReviewRequest) => Promise<boolean>;
  reply: (to: string, body: string) => Promise<boolean>;
  resolve: (root: string, resolved: boolean) => Promise<boolean>;
}>();
const emit = defineEmits<{ viewed: [on: boolean] }>();

const open = ref(autoOpens(props.file) && !props.viewed);
const diff = ref('');
const loaded = ref(false);
const truncated = ref(false);
const error = ref('');
async function load() {
  error.value = '';
  try {
    const d = await getFileDiff(props.id, props.range, props.file.path, props.file.from, LARGE_LIMIT);
    [diff.value, truncated.value, loaded.value] = [d.diff, d.truncated, true];
  } catch (e) {
    error.value = message(e);
  }
}
function toggle() {
  open.value = !open.value;
}
// by value: a re-read of the changes makes a new, equal range
watch([open, () => props.range.from, () => props.range.to, () => props.file.blob], () => {
  loaded.value = false;
  if (open.value && !props.file.binary) void load();
}, { immediate: true });
// a file marked viewed folds
watch(() => props.viewed, (v) => v && (open.value = false));

const rows = computed(() => parseUnified(diff.value));
// the old side is the base only in a view from it: elsewhere its numbers
// aren't what a comment on the old side means
const oldClickable = computed(() => props.range.from === props.base);

// each thread under its line's row, or above the diff (loose)
const placement = computed(() => {
  const at = new Map<number, ReviewThread[]>();
  const loose: ReviewThread[] = [];
  for (const t of props.threads) {
    const r = t.root.review!;
    const line = r.side === 'old' && !oldClickable.value ? null : lineIn(r, props.range.to, props.head);
    const i = line === null ? -1 : rows.value.findIndex((row) => (r.side === 'old' ? row.op !== '+' && row.old === line : row.op !== '-' && row.new === line));
    if (i < 0) loose.push(t);
    else at.set(i, [...(at.get(i) ?? []), t]);
  }
  return { at, loose };
});
const placed = computed(() => placement.value.at);
const loose = computed(() => placement.value.loose);

const draftAt = ref<{ row: number; side: 'old' | 'new'; line: number } | null>(null);
const draft = ref('');
function compose(row: number, side: 'old' | 'new', line: number) {
  draftAt.value = { row, side, line };
  draft.value = '';
}
async function send() {
  const at = draftAt.value;
  if (!at || !draft.value.trim()) return;
  if (await props.comment({ body: draft.value, path: props.file.path, line: at.line, side: at.side, commit: props.range.to })) draftAt.value = null;
}

const command = computed(() => diffCommand(props.range.from, props.range.to, props.file.path));
const copied = ref(false);
async function copy() {
  try {
    await navigator.clipboard.writeText(command.value);
    copied.value = true;
    setTimeout(() => (copied.value = false), 2000);
  } catch {
    window.prompt('Copy the command', command.value);
  }
}
</script>

<style scoped>
.gb-df { border: 1px solid var(--border); background: var(--bg); }
.gb-df + .gb-df { margin-top: .5rem; }
.gb-df-head { display: flex; align-items: center; gap: .5rem; padding: .375rem .75rem; background: var(--bg-subtle); font-size: .875rem; }
.gb-df-toggle { display: inline-flex; align-items: center; gap: .375rem; min-width: 0; padding: 0; border: 0; background: none; color: var(--fg); font: inherit; cursor: pointer; text-align: left; }
.gb-df-shut { transform: rotate(-90deg); }
.gb-df-path { overflow-wrap: anywhere; font-weight: 700; }
.gb-df-stat { white-space: nowrap; font-size: .8125rem; }
.gb-df-add { color: var(--success-fg); }
.gb-df-del { color: var(--danger-fg); }
.gb-df-viewed { margin: 0; font-size: .8125rem; white-space: nowrap; }
.gb-df-body { overflow-x: auto; }
.gb-df-note { display: flex; flex-direction: column; align-items: flex-start; gap: .375rem; margin: 0; padding: .5rem .75rem; }
.gb-df-rows { width: 100%; border-collapse: collapse; font-family: var(--font-mono); font-size: .8125rem; line-height: 1.25rem; }
.gb-df-rows td { padding: 0; border: 0; vertical-align: top; }
.gb-df-num { width: 1%; padding: 0 .375rem !important; color: var(--fg-subtle); text-align: right; white-space: nowrap; user-select: none; }
.gb-df-num button { padding: 0; border: 0; background: none; color: inherit; font: inherit; cursor: pointer; }
.gb-df-num button:hover, .gb-df-num button:focus-visible { color: var(--primary); text-decoration: underline; }
.gb-df-text { padding-right: .75rem !important; white-space: pre-wrap; overflow-wrap: anywhere; }
.gb-df-op { display: inline-block; width: 1.25rem; padding-left: .25rem; color: var(--fg-subtle); }
.gb-df-radd { background: var(--success-bg); }
.gb-df-rdel { background: var(--danger-bg); }
.gb-df-hunk td { padding: .125rem .75rem !important; background: var(--bg-subtle); color: var(--fg-muted); }
.gb-df-draft { display: flex; flex-direction: column; gap: .5rem; padding: .5rem .75rem; border: 1px solid var(--border); background: var(--bg); }
.gb-df-thread td { padding: .5rem .75rem !important; background: var(--bg-subtle); font-family: var(--font-sans, inherit); }
</style>
