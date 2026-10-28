<!-- The issue's flow (docs/prd/time-flow.md, F3): its whole life in one
     column, a segment per step under its divider, read through the
     person's filters, order and grouping. The rules are flow.ts's. -->
<template>
  <div ref="root" class="gb-flow">
    <div class="gb-flow-controls">
      <div class="gb-flow-chips" role="group" aria-label="Flow">
        <span class="gb-eyebrow">Flow</span>
        <button v-if="cycle" type="button" class="gb-fchip" :aria-pressed="reading.show === 'step'" @click="reading.show = 'step'">This step {{ count({ show: 'step' }) }}</button>
        <button type="button" class="gb-fchip" :aria-pressed="reading.show === 'all' || (reading.show === 'version' && !cur)" @click="reading.show = 'all'">All {{ count({ show: 'all' }) }}</button>
        <button v-if="cur" type="button" class="gb-fchip" :aria-pressed="reading.show === 'version'" @click="reading.show = 'version'">On v{{ cur.v }} {{ count({ show: 'version' }) }}</button>
        <button type="button" class="gb-fchip" :aria-pressed="reading.show === 'open'" @click="reading.show = 'open'">Open threads {{ count({ show: 'open' }) }}</button>
        <button type="button" class="gb-fchip" :aria-pressed="reading.mine" @click="reading.mine = !reading.mine">Mine {{ count(only('mine')) }}</button>
        <button type="button" class="gb-fchip" :aria-pressed="reading.agent" @click="reading.agent = !reading.agent">Agent {{ count(only('agent')) }}</button>
        <button type="button" class="gb-fchip" :aria-pressed="reading.milestones" @click="reading.milestones = !reading.milestones">Milestones {{ count(only('milestones')) }}</button>
      </div>
      <div class="gb-flow-segs">
        <span class="gb-flow-where">
          Threads
          <span class="gb-seg gb-seg-sm" role="group" aria-label="Where threads show">
            <button type="button" :aria-pressed="reading.where === 'section'" @click="reading.where = 'section'">under their section</button>
            <button type="button" :aria-pressed="reading.where === 'flow'" @click="reading.where = 'flow'">in the flow</button>
          </span>
        </span>
        <div class="gb-seg gb-seg-sm" role="group" aria-label="Order">
          <button type="button" :aria-pressed="!reading.newest" @click="reading.newest = false">Oldest first</button>
          <button type="button" :aria-pressed="reading.newest" @click="reading.newest = true">Newest first</button>
        </div>
        <div class="gb-seg gb-seg-sm" role="group" aria-label="Grouping">
          <button type="button" :aria-pressed="!reading.threads" @click="reading.threads = false">Flat</button>
          <button type="button" :aria-pressed="reading.threads" @click="reading.threads = true">Threads</button>
        </div>
      </div>
    </div>
    <p v-if="cycle" class="gb-note">{{ segs.length }} segments, one per step: a comment carries the step it was written in. Threads fold to their first line; the composer carries the current step.</p>

    <p v-if="!total" class="gb-note">{{ narrowed ? 'Nothing in the flow under these filters.' : 'Nothing said yet.' }}</p>
    <p v-if="reading.show === 'version' && cur && offV" class="gb-flow-ptr">
      {{ offV }} {{ offV === 1 ? 'entry' : 'entries' }} on {{ cur.to ? 'other versions' : cur.v === 2 ? 'v1' : `v1–v${cur.v - 1}` }} {{ offV === 1 ? 'is' : 'are' }} hidden ·
      <button type="button" class="gb-linkbtn" @click="reading.show = 'all'">show all</button>
    </p>
    <template v-for="s in shown" :key="s.id">
      <p v-if="reading.show === 'step' && folded.length" class="gb-flow-ptr">
        {{ foldedWords }} · {{ foldedCount }} {{ foldedCount === 1 ? 'entry' : 'entries' }} ·
        <button type="button" class="gb-linkbtn" @click="reading.show = 'all'">show all</button>
      </p>
      <div v-if="s.n" class="gb-fdiv-row">
        <h3 :id="`step-${s.id}`" tabindex="-1" class="gb-fdiv" :class="{ 'is-current': s.id === step }">{{ divider(s, s.id === step) }}</h3>
        <button v-if="s.n < pos || (cycle?.done && s.id !== step)" type="button" class="gb-linkbtn" @click="emit('open', s.id)">Open step {{ s.n }} · {{ s.name }}</button>
      </div>
      <p v-if="s.n === lastHidden" class="gb-flow-ptr">
        {{ foldedWords }} · <button type="button" class="gb-linkbtn" @click="unfolded = true">show all</button>
      </p>
      <template v-if="!hidden.has(s.n)">
        <ol v-if="s.entries.length" class="gb-fl" :aria-labelledby="s.n ? `step-${s.id}` : undefined" :aria-label="s.n ? undefined : 'The whole conversation'">
          <template v-for="(it, i) in items(s)" :key="it.event.id">
            <li v-if="it.event.id === newId" class="gb-fnew" role="separator" :aria-label="`New since your last visit, ${on(sinceIso!, true)}`">new since your last visit · {{ when(sinceIso!, true) }}</li>
            <!-- a version of the description published: the flow's own milestone -->
            <li v-if="it.event.kind === 'version'" class="gb-fe gb-fver" :data-n="gutter(s, i)">
              <span class="gb-fver-text">
                <span><b>{{ it.event.author.name }}</b> published <b>v{{ pub(it.event).n }}</b> of the description <span class="gb-fver-muted">· {{ when(it.event.at, true) }}</span></span>
                <span class="gb-fver-muted">
                  <template v-for="c in pub(it.event).changed" :key="c"><span class="gb-anchor">{{ c }}</span>{{ ' ' }}</template>
                  <template v-if="pub(it.event).changed.length">changed · </template>
                  <template v-if="pub(it.event).applied.length">applied {{ pub(it.event).applied.length }} suggested edit{{ pub(it.event).applied.length === 1 ? '' : 's' }} of {{ [...new Set(pub(it.event).applied)].join(', ') }} · </template>
                  <template v-if="pub(it.event).resolved">resolved {{ pub(it.event).resolved }} thread{{ pub(it.event).resolved === 1 ? '' : 's' }} · </template>
                  <button type="button" class="gb-linkbtn" @click="emit('compare', pub(it.event).n)">Compare v{{ pub(it.event).n - 1 }}…v{{ pub(it.event).n }}</button>
                </span>
              </span>
            </li>
            <TimelineEvent v-else-if="it.event.kind !== 'comment'" class="gb-fe" :data-n="gutter(s, i)" :data-event="it.event.id" :event="it.event" :fields="fields" />
            <!-- a comment on the description, its thread under the section: a line pointing at it -->
            <li v-else-if="pointers && it.event.meta?.['thread:anchor']" class="gb-fe gb-fptr" :data-n="gutter(s, i)">
              <span>
                {{ when(it.event.at, true) }} · {{ it.event.author.name }} {{ verb(it.event) }}
                <button type="button" class="gb-anchor" @click="emit('thread', rootOf(it.event) ?? it.event.id)">{{ label(it.event) }}</button>
                <template v-if="it.event.id === firstPtr"> · under the section</template> ↑
              </span>
            </li>
            <li v-else class="gb-fe" :data-n="gutter(s, i)" :data-event="it.event.id">
              <div v-if="it.resolved !== undefined && !open.has(it.event.id)" class="gb-fthread">
                <span>
                  <b>{{ it.event.author.name }}</b> on
                  <button type="button" class="gb-anchor" @click="it.event.review ? emit('line', it.event.review.path!) : emit('thread', it.event.id)">{{ where(it.event) }}</button>
                  <span class="gb-fthread-q"> · “{{ firstWords(it.event) }}”</span>
                </span>
                <button type="button" class="gb-tchip" :class="{ 'is-open': !it.resolved }" aria-expanded="false" @click="open.add(it.event.id)">
                  {{ it.replies.length + 1 }} comment{{ it.replies.length ? 's' : '' }} · {{ it.resolved ? 'resolved' : 'open' }} ▸
                </button>
              </div>
              <div v-else class="gb-fthread-open" :class="{ 'is-thread': it.resolved !== undefined }">
                <ThreadCard
                  v-if="it.resolved !== undefined && it.event.meta?.['thread:anchor']"
                  :thread="it"
                  :events="events"
                  :sections="sections"
                  :me-id="meId"
                  :fields="fields"
                  :forges="forges"
                  :list="list"
                  :busy="busy"
                  :save="save"
                  :reply="reply"
                  :resolve="resolve"
                  :apply="apply"
                  @anchor="emit('thread', it.event.id)"
                  @ref="(n, ev) => emit('ref', n, ev)"
                />
                <CommentCard v-else v-for="c in [it.event, ...it.replies]" :key="c.id" :data-event="c.id" :event="c" :forges="forges" :list="list" :mine="c.author.id === meId" :save="save" :role="roleOf(fields, c.author.login)" @ref="(n, ev) => emit('ref', n, ev)" @line="(p) => emit('line', p)" />
                <p v-if="it.resolved !== undefined && threads"><button type="button" class="gb-linkbtn" aria-expanded="true" @click="open.delete(it.event.id)">Fold the thread</button></p>
              </div>
            </li>
          </template>
        </ol>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, reactive, ref, toRef, watch } from 'vue';
import type { Cycle, Forge, IssueEvent } from '@/api/board';
import type { Fields } from '@/fields';
import { bodyThreads, divider, firstNew, isRoot, items as itemsOf, pick, roleOf, rootOf, segments, type Item, type On, type Reading, type Segment } from '@/flow';
import { listOf } from '@/cycle';
import { on, when } from '@/me';
import { anchorLabel, anchorOf, changeOf, sectionsOf, versions } from '@/sections';
import CommentCard from '@/components/CommentCard.vue';
import ThreadCard from '@/components/ThreadCard.vue';
import TimelineEvent from '@/components/TimelineEvent.vue';

const props = defineProps<{
  events: IssueEvent[];
  cycle: Cycle | null;
  failed?: boolean; // the step could not be read: no cycle to wait for
  meId: string;
  since: number | null; // the last visit, for the "new since" line
  forges?: Forge[] | undefined;
  fields: Fields;
  save: (id: string, body: string, base: string) => Promise<unknown>;
  reading: Reading; // the panel's, the flow writes its picks into it
  shownV: number; // the description's version shown: On vN's
  // the description's threads are under its sections: the flow points at
  // them; else they show here, as review threads do, with ThreadCard's
  // reply, Resolve and Apply
  pointers: boolean;
  busy: boolean;
  reply: (root: string, body: string) => Promise<boolean>;
  resolve: (root: string, value: string) => Promise<boolean>;
  apply: (root: string, body: string, base: string, version: number) => Promise<boolean>;
}>();
const reading = toRef(props, 'reading');
// thread: a body thread's anchor, its place in the description
const emit = defineEmits<{ line: [path: string]; ref: [n: string, event: MouseEvent]; open: [id: string]; thread: [root: string]; compare: [v: number] }>();

// the issue's step, by id, and its place in the list; '' and 0 before the
// first step opens
const step = computed(() => props.cycle?.at ?? '');
const list = computed(() => listOf(props.cycle?.stages));
const pos = computed(() => list.value.findIndex((s) => s.id === step.value) + 1);
// the description's versions: a milestone for each past v1, sorted into
// its step as any event; On vN follows the version shown
const vs = computed(() => {
  const c = props.events.find((e) => e.kind === 'created');
  return c ? versions(c) : [];
});
const milestones = computed(() => vs.value.slice(1).map((b): IssueEvent => ({ kind: 'version', id: `version-${b.n}`, at: b.at, author: b.author })));
const cur = computed((): On | undefined => {
  const k = Math.min(Math.max(props.shownV, 1), vs.value.length);
  const next = vs.value[k];
  return vs.value.length > 1 ? { v: k, from: vs.value[k - 1]!.at, ...(next ? { to: next.at } : {}) } : undefined;
});
const segs = computed(() => segments([...props.events, ...milestones.value], props.cycle?.stages, step.value, props.cycle?.skipped));
const read = (r: Reading) => pick(segs.value, props.events, r, props.meId, step.value, cur.value);
const sum = (ss: Segment[]) => ss.reduce((n, s) => n + s.entries.length, 0);
const count = (r: Partial<Reading>) => sum(read({ ...reading.value, ...r }));
// a toggle's count: what it alone would show
const only = (k: 'mine' | 'agent' | 'milestones') => ({ mine: false, agent: false, milestones: false, [k]: true });
const shown = computed(() => read(reading.value));
const offV = computed(() => count({ show: 'all' }) - count({ show: 'version' }));
// a milestone's line: the sections changed since the version before, the
// suggestions applied in it and the threads it resolved
function pub(e: IssueEvent) {
  const n = Number(e.id.slice(8));
  const before = sectionsOf(vs.value[n - 2]?.html ?? '');
  const changed = sectionsOf(vs.value[n - 1]?.html ?? '').flatMap((s) => {
    const c = changeOf(before, s);
    return c === 'unchanged' ? [] : [c === 'changed' ? `§${s.n}` : `§${s.n} › ${c}`];
  });
  const by = bodyThreads(props.events).filter((t) => t.event.meta?.['thread:resolved'] === `v${n}`);
  const applied = by.filter((t) => t.event.meta?.['thread:applied']).flatMap((t) => [t.event, ...t.replies].findLast((x) => x.meta?.['thread:kind'] === 'suggestion')?.author.name ?? []);
  return { n, changed, applied, resolved: by.length };
}
const total = computed(() => sum(shown.value));
const narrowed = computed(() => reading.value.show !== 'all' || reading.value.mine || reading.value.agent || reading.value.milestones);
const threads = computed(() => reading.value.threads);
const items = (s: Segment): Item[] => itemsOf(s, reading.value.threads, props.pointers);

// What is folded: under This step the segments before the current one,
// pointed at above it; under All, a long flow's segments before the
// current one show their divider only.
// ponytail: 100 entries is a guess at where the page gets slow to read
const LONG = 100;
const unfolded = ref(false);
const folded = computed((): Segment[] => {
  const before = (ss: Segment[]) => ss.filter((s) => s.n && s.n < pos.value);
  if (reading.value.show === 'step') return before(read({ ...reading.value, show: 'all' }));
  return reading.value.show === 'all' && !unfolded.value && total.value > LONG ? before(shown.value) : [];
});
const hidden = computed(() => new Set(reading.value.show === 'step' ? [] : folded.value.map((s) => s.n)));
const lastHidden = computed(() => (hidden.value.size ? shown.value.filter((s) => hidden.value.has(s.n)).at(-1)!.n : undefined));
const foldedCount = computed(() => sum(folded.value));
const pad = (n: number) => String(n).padStart(2, '0');
const foldedWords = computed(() => {
  const ns = folded.value.map((s) => s.n).sort((a, b) => a - b);
  if (ns.length === 1) return `Segment ${pad(ns[0]!)} is folded`;
  return `Segments ${pad(ns[0]!)} ${ns.length === 2 ? 'and' : 'to'} ${pad(ns.at(-1)!)} are folded`;
});

// the gutter: the segment's number on its first entry
const gutter = (s: Segment, i: number) => (s.n && i === 0 ? pad(s.n) : undefined);
// a body thread's anchor, "§3 › 3"; a review line's path
const sections = computed(() => sectionsOf(props.events.find((e) => e.kind === 'created')?.html ?? ''));
const label = (e: IssueEvent) => anchorLabel(anchorOf(e.meta?.['thread:anchor'])!, sections.value);
const where = (e: IssueEvent) => (e.review ? `${e.review.path}${e.review.line ? `:${e.review.line}` : ''}` : label(e));
// a pointer line's verb, and the first one, which says where they are
const verb = (e: IssueEvent) => (e.meta?.['thread:kind'] === 'suggestion' ? 'suggested an edit on' : rootOf(e) ? 'replied on' : 'commented on');
const firstPtr = computed(() => (props.pointers ? shown.value.flatMap((s) => s.entries).find((e) => e.meta?.['thread:anchor'])?.id : undefined));
const firstWords = (e: IssueEvent) => {
  const line = (e.body ?? '').trim().split('\n')[0] ?? '';
  return line.length > 60 ? `${line.slice(0, 60)}…` : line;
};
const open = reactive(new Set<string>()); // the threads unfolded

const sinceIso = computed(() => (props.since === null ? undefined : new Date(props.since).toISOString()));
const newId = computed(() => firstNew(shown.value, props.since, props.meId, reading.value.threads));

const root = ref<HTMLElement | null>(null);
// settled waits for the cycle: the segments and the step's head move
// everything when it lands, so a reveal asked before goes after,
// or right away if it failed to load. A newer ask drops an older one.
let pending: (() => void) | undefined;
const loaded = () => !!props.cycle || !!props.failed;
const settled = () => new Promise<void>((ok) => (loaded() ? ok() : (pending = ok)));
watch(loaded, (has) => {
  if (has) pending?.();
  pending = undefined;
});
// unfold renders the whole flow, and shows what id is (a thread holding
// it unfolded, the filters let go if they hide it): what the inbox or
// the sidebar scrolls to is there
function unfold(id?: string) {
  unfolded.value = true;
  const e = id ? props.events.find((x) => x.id === id) : undefined;
  if (!e || e.kind === 'created') return;
  if (!shown.value.some((s) => s.entries.includes(e))) Object.assign(reading.value, { show: 'all', mine: false, agent: false, milestones: false });
  const root = rootOf(e) ?? (isRoot(e) ? e.id : undefined);
  if (root) open.add(root);
}
defineExpose({ unfold, settled });
</script>

<style scoped>
.gb-flow { display: flex; flex-direction: column; }
.gb-flow-controls { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 10px 16px; margin-bottom: 10px; }
.gb-flow-chips, .gb-flow-segs { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.gb-flow-chips .gb-eyebrow { margin-right: 6px; }
.gb-flow-segs { gap: 8px 14px; }
.gb-fchip { font: inherit; font-size: .75rem; padding: 4px 10px; border: 1px solid var(--border-strong); border-radius: var(--radius); background: var(--bg); color: var(--fg); white-space: nowrap; cursor: pointer; }
.gb-fchip[aria-pressed="true"] { border-color: var(--fg); background: var(--bg-dark); color: var(--fg-inverse); }
.gb-flow-ptr { margin: 12px 0 0; font-size: .875rem; color: var(--fg-muted); }
.gb-fdiv-row { display: flex; align-items: center; gap: 12px; margin: 22px 0 4px; }
.gb-fdiv-row .gb-linkbtn { flex: none; font-size: .75rem; }
.gb-fdiv { flex: 1 1 auto; display: flex; align-items: center; gap: 12px; margin: 0; font-size: .75rem; letter-spacing: .12em; text-transform: uppercase; color: var(--gb-done); font-weight: 700; }
.gb-fdiv::after { content: ""; flex: 1 1 auto; border-top: 1px solid var(--border-subtle); }
.gb-fdiv.is-current { color: var(--fg); }
.gb-fdiv:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 2px; }
.gb-fl { list-style: none; margin: 0; padding: 0; }
.gb-fe { display: grid; grid-template-columns: 44px minmax(0, 1fr); column-gap: 12px; align-items: start; padding: 10px 0; border-top: 1px solid var(--border-subtle); font-size: .875rem; line-height: 1.45; }
.gb-fl > :first-child, .gb-fnew + .gb-fe { border-top: 0; }
.gb-fe::before { content: attr(data-n); font-family: var(--font-mono); font-size: .75rem; color: var(--gb-done); padding-top: 2px; }
/* an event's own li (TimelineEvent): the gutter, its icon, its line */
.gb-fe.gb-event { grid-template-columns: 44px auto minmax(0, 1fr); align-items: center; }
.gb-fthread { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 6px 12px; padding: 10px 14px; border: 1px solid var(--border-subtle); }
.gb-flow-where { display: flex; align-items: center; gap: 6px; font-size: .75rem; color: var(--fg-muted); }
.gb-fptr { padding: 7px 0; color: var(--fg-muted); }
.gb-fver-text { display: flex; flex-direction: column; gap: 2px; }
.gb-fver-muted { color: var(--fg-muted); }
.gb-fthread-q { color: var(--fg-muted); font-style: italic; }
.gb-fthread-open { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
.gb-fthread-open.is-thread { animation: gb-unfold 120ms ease-out; }
@keyframes gb-unfold { from { opacity: 0; transform: translateY(-4px); } }
@media (prefers-reduced-motion: reduce) { .gb-fthread-open.is-thread { animation: none; } }
.gb-fnew { display: flex; align-items: center; gap: 12px; margin: 12px 0 2px; font-size: .75rem; font-weight: 700; color: var(--primary-hover); }
.gb-fnew::before, .gb-fnew::after { content: ""; flex: 1 1 auto; border-top: 2px solid var(--primary-hover); }
</style>
