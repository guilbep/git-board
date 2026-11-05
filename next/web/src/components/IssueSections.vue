<!-- The description as sections (docs/design/time-flow, 6a): the text cut
     at its headings, each section with a gutter holding its anchor and a
     chip for its threads. The rules are sections.ts's and flow.ts's. -->
<template>
  <div ref="root" class="gb-secs">
    <section v-for="s in sections" :key="s.slug" :data-section="s.slug" class="gb-sec" :aria-label="`§${s.n} ${s.heading || 'intro'}`">
      <MarkdownBody class="gb-sec-md" :html="s.html" :forges="forges" @ref="(n, e) => emit('ref', n, e)" />
      <div class="gb-gut">
        <span class="gb-anchor">§{{ s.n }} {{ s.heading || 'intro' }}</span>
        <span v-if="changed(s)">{{ changed(s) }}</span>
        <button v-if="!threadsIn(s).length" type="button" class="gb-tchip is-ghost" :aria-expanded="composing === s.slug" @click="composing = s.slug">Comment on §{{ s.n }}</button>
        <button v-else-if="inFlow" type="button" class="gb-tchip" :class="chipClass(s)" @click="emit('flow', (threadsIn(s).find((t) => !t.resolved) ?? threadsIn(s)[0]!).event.id)">{{ chip(s) }}</button>
        <button v-else type="button" class="gb-tchip" :class="chipClass(s)" :aria-expanded="open.has(s.slug)" @click="toggle(s.slug)">{{ chip(s) }}</button>
      </div>
      <!-- every thread resolved: one line each, Show opens them -->
      <template v-if="!inFlow && !open.has(s.slug) && threadsIn(s).every((t) => t.resolved)">
        <div v-for="t in threadsIn(s)" :key="t.event.id" class="gb-sec-row gb-tdone">
          <span>Thread on <span class="gb-anchor">{{ label(t) }}</span> · {{ [...new Set([t.event, ...t.replies].map((e) => e.author.name))].join(', ') }} · {{ plural(1 + t.replies.length, 'comment') }} · <b>{{ resolvedBy(t, events) }}</b><template v-if="appliedOf(t)">: the suggested edit of {{ appliedOf(t) }} was applied</template></span>
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="open.add(s.slug)">Show</button>
        </div>
      </template>
      <div v-if="!inFlow && open.has(s.slug)" class="gb-sec-row gb-sec-threads">
        <ThreadCard
          v-for="t in threadsIn(s)"
          :key="t.event.id"
          :thread="t"
          :events="events"
          :sections="current"
          :me-id="meId"
          :fields="fields"
          :forges="forges"
          :busy="busy"
          :save="save"
          :reply="reply"
          :resolve="resolve"
          :apply="apply"
          @anchor="scrollTo"
          @ref="(n, e) => emit('ref', n, e)"
        />
      </div>
      <div v-if="composing === s.slug" class="gb-sec-row">
        <IssueComposer
          :status="status"
          :forges="forges"
          :busy="busy"
          :waiting="waiting"
          :error="error"
          :carries="[carried, `v${v}`, `§${s.n} ${s.heading || 'intro'}`].filter(Boolean)"
          :comment="(b: string) => post(`body@v${v}/${s.slug}`, b)"
          :cancel="() => (composing = undefined)"
        />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, reactive, ref, useTemplateRef, watch } from 'vue';
import { renderMarkdown, type Forge, type IssueEvent } from '@/api/board';
import { bodyThreads, resolvedBy, type Item } from '@/flow';
import { anchorLabel, anchorOf, changeOf, diffMarkdown, sectionsOf, stateOf, type BodyVersion, type Section } from '@/sections';
import type { Fields } from '@/fields';
import IssueComposer from '@/components/IssueComposer.vue';
import MarkdownBody from '@/components/MarkdownBody.vue';
import ThreadCard from '@/components/ThreadCard.vue';

const props = defineProps<{
  html: string;
  // the body's versions, the one shown and whether its changes since the
  // one before are marked (the card head's seg and checkbox)
  versions: BodyVersion[];
  shown: number;
  since: boolean;
  events: IssueEvent[];
  forges?: Forge[] | null | undefined;
  // the composer under a section, as the flow's: the issue's status, the
  // save state, the step it carries ("01 Intent")
  status: string;
  busy: boolean;
  waiting: boolean;
  error: string;
  carried: string;
  comment: (anchor: string, body: string) => Promise<boolean>; // posts a comment with thread:anchor
  // the threads under a section: their cards' edit, reply and Resolve
  meId: string;
  fields: Fields;
  save: (id: string, body: string, base: string) => Promise<unknown>;
  reply: (root: string, body: string) => Promise<boolean>;
  resolve: (root: string, value: string) => Promise<boolean>;
  apply: (root: string, body: string, base: string, version: number) => Promise<boolean>;
  inFlow: boolean; // the threads show in the flow: a chip goes to its section's first open one there
}>();
const emit = defineEmits<{ ref: [n: string, event: MouseEvent]; flow: [root: string] }>();

// the version shown, or its diff against the one before once rendered:
// the plain text until the server answers, the last request's answer only
const diff = ref<string>();
let asked = 0;
watch(() => [props.shown, props.since, props.versions] as const, async ([k, since, vs]) => {
  const n = ++asked;
  diff.value = undefined;
  if (!since || k < 2) return;
  const { html } = await renderMarkdown(diffMarkdown(vs[k - 2]!.body, vs[k - 1]!.body));
  if (n === asked) diff.value = html;
}, { immediate: true });
const sections = computed(() => sectionsOf(diff.value ?? props.versions[props.shown - 1]?.html ?? props.html));
// a gutter's change against the version before: nothing on v1
const before = computed(() => (props.shown > 1 ? sectionsOf(props.versions[props.shown - 2]!.html) : undefined));
const shownSections = computed(() => sectionsOf(props.versions[props.shown - 1]?.html ?? props.html));
function changed(s: Section): string {
  const now = shownSections.value.find((x) => x.slug === s.slug);
  if (!before.value || !now) return '';
  const c = changeOf(before.value, now);
  return c === 'unchanged' ? 'unchanged' : c === 'changed' ? `changed in v${props.shown}` : `› ${c} changed in v${props.shown}`;
}
// the current text's sections: a thread's label, Apply's lines
const current = computed(() => sectionsOf(props.html));
// a section's threads: those whose anchor names its slug, any version;
// each one current, carried or outdated against the current version
const threads = computed(() => bodyThreads(props.events));
const threadsIn = (s: Section): Item[] => threads.value.filter((t) => anchorOf(t.event.meta?.['thread:anchor'])?.slug === s.slug);

const open = reactive(new Set<string>()); // the sections whose threads are unfolded
const composing = ref<string>(); // the section whose composer is open: one at a time
// the version a new anchor names: the one shown
const v = computed(() => props.shown || 1);
async function post(anchor: string, body: string) {
  const ok = await props.comment(anchor, body);
  if (ok) composing.value = undefined;
  return ok;
}
const toggle = (slug: string) => (open.has(slug) ? open.delete(slug) : open.add(slug));

const root = useTemplateRef<HTMLElement>('root');
const scrollTo = (slug: string) => root.value?.querySelector(`[data-section="${CSS.escape(slug)}"]`)?.scrollIntoView({ block: 'start' });
// opens the section of a thread and scrolls to its root (the flow's
// pointer lines, the inbox); with the threads in the flow, to the section
async function unfold(rootId: string) {
  const t = threads.value.find((t) => t.event.id === rootId);
  const slug = t && anchorOf(t.event.meta?.['thread:anchor'])?.slug;
  if (!slug || !sections.value.some((s) => s.slug === slug)) return false;
  if (!props.inFlow) open.add(slug);
  await nextTick();
  if (props.inFlow) scrollTo(slug);
  else root.value?.querySelector(`[data-event="${CSS.escape(rootId)}"]`)?.scrollIntoView({ block: 'center' });
  return true;
}
defineExpose({ unfold });

const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;
const label = (t: Item) => anchorLabel(anchorOf(t.event.meta?.['thread:anchor'])!, current.value);
// who wrote the suggestion an Apply resolved the thread with
const appliedOf = (t: Item) => t.event.meta?.['thread:applied'] && [t.event, ...t.replies].findLast((e) => e.meta?.['thread:kind'] === 'suggestion')?.author.name;
// an open thread written on an older version: "carried from v1" while its
// section is unchanged since, "written on v1" (amber) once it changed
function older(s: Section): { word: string; warn: boolean } | undefined {
  const open = threadsIn(s).filter((t) => !t.resolved);
  const states = open.map((t) => ({ t, st: stateOf(t.event, props.versions) }));
  const out = states.find((x) => x.st.state === 'outdated') ?? states.find((x) => x.st.state === 'carried');
  if (!out) return undefined;
  const v = anchorOf(out.t.event.meta?.['thread:anchor'])!.v;
  return out.st.state === 'outdated' ? { word: `written on v${v}`, warn: true } : { word: `carried from v${v}`, warn: false };
}
const chipClass = (s: Section) => ({ 'is-open': threadsIn(s).some((t) => !t.resolved) && !older(s)?.warn, 'is-warn': older(s)?.warn });
// "2 comments · open ▸", "· resolved by Person B ▸"; unfolded, "› 3 · 2 comments, 1 suggestion ▾",
// the item when every thread of the section is on the same one
function chip(s: Section): string {
  const ts = threadsIn(s);
  const all = ts.flatMap((t) => [t.event, ...t.replies]);
  // "resolved by v2" when the section has the one thread
  const state = ts.some((t) => !t.resolved) ? 'open' : ts.length === 1 ? resolvedBy(ts[0]!, props.events) : 'resolved';
  const o = older(s);
  if (!open.has(s.slug) || o) return `${plural(all.length, 'comment')} · ${state}${o ? ` · ${o.word}` : ''} ${open.has(s.slug) ? '▾' : '▸'}`;
  const items = new Set(ts.map((t) => anchorOf(t.event.meta?.['thread:anchor'])?.item));
  const item = items.size === 1 ? [...items][0] : undefined;
  const sug = all.filter((e) => e.meta?.['thread:kind'] === 'suggestion').length;
  return `${item ? `› ${item} · ` : ''}${plural(all.length - sug, 'comment')}${sug ? `, ${plural(sug, 'suggestion')}` : ''} ▾`;
}
</script>

<style scoped>
.gb-secs { display: flex; flex-direction: column; padding: 1rem 1.25rem 1.25rem; }
.gb-sec { display: grid; grid-template-columns: minmax(0, 1fr) 164px; gap: 10px 20px; padding: 16px 0; border-top: 1px solid var(--border-subtle); }
.gb-sec-row { grid-column: 1 / -1; }
.gb-tdone { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 6px 12px; padding: 10px 16px; border: 1px solid var(--border-subtle); font-size: .8125rem; color: var(--fg-muted); }
.gb-tdone b { color: var(--gb-green-fg); }
.gb-sec-threads { display: flex; flex-direction: column; gap: 12px; }
.gb-sec:first-child { border-top: 0; padding-top: 0; }
.gb-gut { display: flex; flex-direction: column; align-items: flex-start; gap: 6px; font-size: .75rem; color: var(--fg-muted); }
.gb-gut .gb-tchip { white-space: normal; }
.gb-sec :deep(.gb-md > :first-child) { margin-top: 0; }
.gb-sec :deep(ins) { background: var(--gb-green-bg); color: var(--gb-green-fg); text-decoration: none; padding: 1px 4px; }
.gb-sec :deep(del) { background: var(--gb-red-bg); color: var(--gb-red-fg); padding: 1px 4px; }
/* the README's widths are the viewport's: the gutter keeps 164 px down to 720 */
@media (max-width: 719.98px) {
  .gb-sec { grid-template-columns: minmax(0, 1fr); }
  .gb-gut { flex-direction: row; flex-wrap: wrap; align-items: center; }
}
</style>
