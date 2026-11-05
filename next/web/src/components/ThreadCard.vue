<!-- A thread on the description (docs/design/time-flow, 6a): its comments,
     the root first with its anchor, a suggested edit as a two-line diff
     with Apply and Decline for the description's author, then the reply
     row: Reply, and Resolve while it is open, Unresolve once it is
     resolved. -->
<template>
  <div class="gb-tcard">
    <p v-if="state.state === 'outdated'" class="gb-tout">Written on v{{ anchor!.v }}. {{ anchorLabel({ text: 'body', v: anchor!.v, slug: anchor!.slug }, sections) }} changed in v{{ state.since }}, so this thread may be answered by the text itself; it stays open until someone resolves it.</p>
    <CommentCard
      v-for="(c, i) in [thread.event, ...thread.replies]"
      :key="c.id"
      :data-event="c.id"
      :event="c"
      :forges="forges"
      :list="list"
      :mine="c.author.id === meId"
      :save="save"
      :role="roleOf(fields, c.author.login)"
      :anchor="i === 0 && anchor ? anchorLabel(anchor, sections) + (anchor.v < v ? ` · v${anchor.v}` : '') : undefined"
      @anchor="emit('anchor', anchor!.slug)"
      @ref="(n, e) => emit('ref', n, e)"
    >
      <template v-if="sug(c) !== undefined">
        <div class="gb-sugbody"><MarkdownBody class="gb-comment-body" :html="c.html ?? ''" :forges="forges" @ref="(n, e) => emit('ref', n, e)" /></div>
        <div class="gb-sugwrap">
          <div class="gb-sug">
            <div v-if="line" class="gb-sug-d">{{ line }}</div>
            <div class="gb-sug-a">{{ sug(c) }}</div>
          </div>
          <p v-if="applied" class="gb-hint">applied in {{ thread.event.meta?.['thread:resolved'] }}</p>
          <template v-else>
            <p v-if="declined(c)" class="gb-hint">declined</p>
            <div v-if="author && !thread.resolved" class="gb-sug-act">
              <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="applyIt(c)">Apply in v{{ v + 1 }}</button>
              <button v-if="!declined(c)" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="decline">Decline</button>
              <span class="gb-hint">Applying publishes v{{ v + 1 }} with this line replaced.</span>
            </div>
          </template>
          <div v-if="changed" class="epfl-alert epfl-alert-warning gb-alert-row" role="alert">
            <GbIcon name="warn" small />
            <span><b>Someone changed this text meanwhile.</b> Nothing was applied: reload, then apply again.</span>
          </div>
          <p v-else-if="error" class="gb-save-error" role="alert"><GbIcon name="error" small /> <b>Not applied.</b> {{ error }}</p>
        </div>
      </template>
    </CommentCard>
    <div class="gb-treply">
      <label :for="`r-${uid}`" class="gb-hint">Reply</label>
      <input :id="`r-${uid}`" ref="input" v-model="text" class="epfl-input" :disabled="busy" @keydown.enter.prevent="send" />
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || !text.trim()" @click="send">Reply</button>
      <button v-if="!thread.resolved" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="resolve(thread.event.id, meId)">Resolve</button>
      <template v-else>
        <span class="gb-hint">{{ resolvedBy(thread, events) }}</span>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="resolve(thread.event.id, '')">Unresolve</button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useId, useTemplateRef } from 'vue';
import { ApiError, type Forge, type IssueEvent, type StageStep } from '@/api/board';
import type { Fields } from '@/fields';
import { resolvedBy, roleOf, type Item } from '@/flow';
import { anchorLabel, anchorOf, applySuggestion, lineAt, stateOf, suggestionOf, versions, type Section } from '@/sections';
import CommentCard from '@/components/CommentCard.vue';
import GbIcon from '@/components/GbIcon.vue';
import MarkdownBody from '@/components/MarkdownBody.vue';

const props = defineProps<{
  thread: Item;
  events: IssueEvent[]; // the issue's: who resolved it, by name
  sections: Section[];
  meId: string;
  fields: Fields;
  forges?: Forge[] | null | undefined;
  list?: StageStep[] | undefined; // the project's steps, for the comments' chips
  busy: boolean;
  save: (id: string, body: string, base: string) => Promise<unknown>;
  reply: (root: string, body: string) => Promise<boolean>;
  resolve: (root: string, value: string) => Promise<boolean>; // "" reopens
  // the description's edit and the root's thread:resolved, thread:applied
  apply: (root: string, body: string, base: string, version: number) => Promise<boolean>;
}>();
const emit = defineEmits<{ anchor: [slug: string]; ref: [n: string, event: MouseEvent] }>();

const uid = useId();
const anchor = computed(() => anchorOf(props.thread.event.meta?.['thread:anchor']));
const text = ref('');
// the suggested edit: the anchored line struck, the suggestion added
const created = computed(() => props.events.find((e) => e.kind === 'created')!);
const vs = computed(() => versions(created.value));
const v = computed(() => vs.value.length);
const state = computed(() => stateOf(props.thread.event, vs.value));
const author = computed(() => created.value.author.id === props.meId);
const sug = (c: IssueEvent) => (c.meta?.['thread:kind'] === 'suggestion' ? suggestionOf(c.body ?? '') : undefined);
const applied = computed(() => props.thread.event.meta?.['thread:applied']);
// once applied, the line as it was before the edit (ponytail: the current
// sections pair its headings, as long as the edit kept them)
const before = computed(() => {
  const h = created.value.history ?? [];
  const i = h.findIndex((x) => x.id === applied.value);
  return i > 0 ? h[i - 1]!.body : (created.value.body ?? '');
});
const line = computed(() => anchor.value && lineAt(before.value, props.sections, anchor.value));
// declined: the description's author replied after the suggestion
const declined = (c: IssueEvent) => props.thread.replies.some((r) => r.author.id === created.value.author.id && r.at > c.at);
const changed = ref(false);
const error = ref('');
async function applyIt(c: IssueEvent) {
  changed.value = false;
  error.value = '';
  const body = anchor.value && applySuggestion(created.value.body ?? '', props.sections, anchor.value, sug(c)!);
  if (body === undefined) return void (error.value = 'The anchored line is gone; edit the description instead.');
  try {
    if (!(await props.apply(props.thread.event.id, body, created.value.body ?? '', v.value + 1))) error.value = `v${v.value + 1} is published, the thread stays open: resolve it by hand.`;
  } catch (e) {
    if (e instanceof ApiError && e.code === 'changed') changed.value = true;
    else error.value = e instanceof Error ? e.message : String(e);
  }
}
// a declined suggestion is the author's reply; the thread stays open
const input = useTemplateRef<HTMLInputElement>('input');
async function decline() {
  text.value = 'Declined: ';
  await nextTick();
  input.value?.focus();
}

async function send() {
  if (!text.value.trim()) return;
  if (await props.reply(props.thread.event.id, text.value)) text.value = '';
}
</script>

<style scoped>
.gb-tcard { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
.gb-tout { margin: 0; padding: 10px 14px; background: var(--warning-bg); color: var(--warning-fg); font-size: .8125rem; }
.gb-treply { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 10px 14px; background: var(--bg-subtle); border: 1px solid var(--border-subtle); }
.gb-treply .epfl-input { flex: 1 1 200px; width: auto; }
.gb-sugbody :deep(pre:has(> code.language-suggestion)) { display: none; }
.gb-sugwrap { display: flex; flex-direction: column; gap: 8px; padding: 0 1.25rem 1rem; }
.gb-sugwrap p { margin: 0; }
.gb-sug { font-family: var(--font-mono); font-size: 13px; line-height: 1.6; border: 1px solid var(--border-subtle); }
.gb-sug > div { padding: 3px 10px; white-space: pre-wrap; overflow-wrap: anywhere; }
.gb-sug-d { background: var(--gb-red-bg); color: var(--gb-red-fg); text-decoration: line-through; }
.gb-sug-a { background: var(--gb-green-bg); color: var(--gb-green-fg); }
.gb-sug-act { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.gb-save-error { display: flex; gap: .375rem; align-items: center; margin: 0; padding: .5rem 1rem; color: var(--danger-fg); background: var(--danger-bg); font-size: .875rem; }
.gb-alert-row { margin: 0; border-radius: 0; }
</style>
