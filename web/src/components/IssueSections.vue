<!-- The description as sections (docs/design/time-flow, 6a): the text cut
     at its headings, each section with a gutter holding its anchor and a
     chip for its threads. The rules are sections.ts's and flow.ts's. -->
<template>
  <div ref="root" class="gb-secs">
    <section v-for="s in sections" :key="s.slug" :data-section="s.slug" class="gb-sec" :aria-label="`§${s.n} ${s.heading || 'intro'}`">
      <MarkdownBody class="gb-sec-md" :html="s.html" :forges="forges" @ref="(n, e) => emit('ref', n, e)" />
      <div class="gb-gut">
        <span class="gb-anchor">§{{ s.n }} {{ s.heading || 'intro' }}</span>
        <button v-if="!threadsIn(s).length" type="button" class="gb-tchip is-ghost" :aria-expanded="composing === s.slug" @click="composing = s.slug">Comment on §{{ s.n }}</button>
        <button v-else type="button" class="gb-tchip" :class="{ 'is-open': threadsIn(s).some((t) => !t.resolved) }" :aria-expanded="open.has(s.slug)" @click="toggle(s.slug)">{{ chip(s) }}</button>
      </div>
      <div v-if="open.has(s.slug)" class="gb-sec-row gb-sec-threads">
        <ThreadCard
          v-for="t in threadsIn(s)"
          :key="t.event.id"
          :thread="t"
          :events="events"
          :sections="sections"
          :me-id="meId"
          :fields="fields"
          :forges="forges"
          :busy="busy"
          :save="save"
          :reply="reply"
          :resolve="resolve"
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
import { computed, nextTick, reactive, ref, useTemplateRef } from 'vue';
import type { Forge, IssueEvent } from '@/api/board';
import { bodyThreads, resolvedBy, type Item } from '@/flow';
import { anchorOf, sectionsOf, version, type Section } from '@/sections';
import type { Fields } from '@/fields';
import IssueComposer from '@/components/IssueComposer.vue';
import MarkdownBody from '@/components/MarkdownBody.vue';
import ThreadCard from '@/components/ThreadCard.vue';

const props = defineProps<{
  html: string;
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
}>();
const emit = defineEmits<{ ref: [n: string, event: MouseEvent] }>();

const sections = computed(() => sectionsOf(props.html));
// a section's threads: those whose anchor names its slug, any version
// (ponytail: M3 tells a carried thread from an outdated one)
const threads = computed(() => bodyThreads(props.events));
const threadsIn = (s: Section): Item[] => threads.value.filter((t) => anchorOf(t.event.meta?.['thread:anchor'])?.slug === s.slug);

const open = reactive(new Set<string>()); // the sections whose threads are unfolded
const composing = ref<string>(); // the section whose composer is open: one at a time
// the body's version, which a new anchor names
const v = computed(() => version(props.events.find((e) => e.kind === 'created')!));
async function post(anchor: string, body: string) {
  const ok = await props.comment(anchor, body);
  if (ok) composing.value = undefined;
  return ok;
}
const toggle = (slug: string) => (open.has(slug) ? open.delete(slug) : open.add(slug));

const root = useTemplateRef<HTMLElement>('root');
const scrollTo = (slug: string) => root.value?.querySelector(`[data-section="${CSS.escape(slug)}"]`)?.scrollIntoView({ block: 'start' });
// opens the section of a thread and scrolls to its root (the flow's
// pointer lines, the inbox)
async function unfold(rootId: string) {
  const t = threads.value.find((t) => t.event.id === rootId);
  const slug = t && anchorOf(t.event.meta?.['thread:anchor'])?.slug;
  if (!slug || !sections.value.some((s) => s.slug === slug)) return false;
  open.add(slug);
  await nextTick();
  root.value?.querySelector(`[data-event="${CSS.escape(rootId)}"]`)?.scrollIntoView({ block: 'center' });
  return true;
}
defineExpose({ unfold });

const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;
// "2 comments · open ▸", "· resolved by Person B ▸"; unfolded, "› 3 · 2 comments, 1 suggestion ▾",
// the item when every thread of the section is on the same one
function chip(s: Section): string {
  const ts = threadsIn(s);
  const all = ts.flatMap((t) => [t.event, ...t.replies]);
  // "resolved by v2" when the section has the one thread
  const state = ts.some((t) => !t.resolved) ? 'open' : ts.length === 1 ? resolvedBy(ts[0]!, props.events) : 'resolved';
  if (!open.has(s.slug)) return `${plural(all.length, 'comment')} · ${state} ▸`;
  const items = new Set(ts.map((t) => anchorOf(t.event.meta?.['thread:anchor'])?.item));
  const item = items.size === 1 ? [...items][0] : undefined;
  const sug = all.filter((e) => e.meta?.['thread:kind'] === 'suggestion').length;
  return `${item ? `› ${item} · ` : ''}${plural(all.length - sug, 'comment')}${sug ? `, ${plural(sug, 'suggestion')}` : ''} ▾`;
}
</script>

<style scoped>
.gb-secs { container-type: inline-size; display: flex; flex-direction: column; padding: 1rem 1.25rem 1.25rem; }
.gb-sec { display: grid; grid-template-columns: minmax(0, 1fr) 164px; gap: 10px 20px; padding: 16px 0; border-top: 1px solid var(--border-subtle); }
.gb-sec-row { grid-column: 1 / -1; }
.gb-sec-threads { display: flex; flex-direction: column; gap: 12px; }
.gb-sec:first-child { border-top: 0; padding-top: 0; }
.gb-gut { display: flex; flex-direction: column; align-items: flex-start; gap: 6px; font-size: .75rem; color: var(--fg-muted); }
.gb-sec :deep(.gb-md > :first-child) { margin-top: 0; }
@container (max-width: 720px) {
  .gb-sec { grid-template-columns: minmax(0, 1fr); }
  .gb-gut { flex-direction: row; flex-wrap: wrap; align-items: center; }
}
</style>
