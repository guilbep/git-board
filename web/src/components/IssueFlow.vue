<!-- The issue's flow (docs/prd/time-flow.md, F3): its whole life in one
     column, a segment per step under its divider, read through the
     person's filters, order and grouping. The rules are flow.ts's. -->
<template>
  <div ref="root" class="gb-flow">
    <div class="gb-flow-controls">
      <div class="gb-flow-chips" role="group" aria-label="Flow">
        <span class="gb-eyebrow">Flow</span>
        <button v-if="step" type="button" class="gb-fchip" :aria-pressed="reading.show === 'step'" @click="reading.show = 'step'">This step {{ count({ show: 'step' }) }}</button>
        <button type="button" class="gb-fchip" :aria-pressed="reading.show === 'all'" @click="reading.show = 'all'">All {{ count({ show: 'all' }) }}</button>
        <button type="button" class="gb-fchip" :aria-pressed="reading.show === 'open'" @click="reading.show = 'open'">Open threads {{ count({ show: 'open' }) }}</button>
        <button type="button" class="gb-fchip" :aria-pressed="reading.mine" @click="reading.mine = !reading.mine">Mine {{ count(only('mine')) }}</button>
        <button type="button" class="gb-fchip" :aria-pressed="reading.agent" @click="reading.agent = !reading.agent">Agent {{ count(only('agent')) }}</button>
        <button type="button" class="gb-fchip" :aria-pressed="reading.milestones" @click="reading.milestones = !reading.milestones">Milestones {{ count(only('milestones')) }}</button>
      </div>
      <div class="gb-flow-segs">
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
    <p v-if="step" class="gb-note">{{ segs.length }} segments, one per step: a comment carries the step it was written in. Threads fold to their first line; the composer carries the current step.</p>

    <p v-if="!total" class="gb-note">{{ narrowed ? 'Nothing in the flow under these filters.' : 'Nothing said yet.' }}</p>
    <template v-for="s in shown" :key="s.n">
      <p v-if="reading.show === 'step' && folded.length" class="gb-flow-ptr">
        {{ foldedWords }} · {{ foldedCount }} {{ foldedCount === 1 ? 'entry' : 'entries' }} ·
        <button type="button" class="gb-linkbtn" @click="reading.show = 'all'">show all</button>
      </p>
      <h3 v-if="s.n" :id="`step-${s.n}`" tabindex="-1" class="gb-fdiv" :class="{ 'is-current': s.n === step }">{{ divider(s, s.n === step) }}</h3>
      <p v-if="s.n === lastHidden" class="gb-flow-ptr">
        {{ foldedWords }} · <button type="button" class="gb-linkbtn" @click="unfolded = true">show all</button>
      </p>
      <template v-if="!hidden.has(s.n)">
        <ol v-if="s.entries.length" class="gb-fl" :aria-labelledby="s.n ? `step-${s.n}` : undefined" :aria-label="s.n ? undefined : 'The whole conversation'">
          <template v-for="(it, i) in items(s)" :key="it.event.id">
            <li v-if="it.event.id === newId" class="gb-fnew" role="separator" :aria-label="`New since your last visit, ${on(sinceIso!, true)}`">new since your last visit · {{ when(sinceIso!, true) }}</li>
            <TimelineEvent v-if="it.event.kind !== 'comment'" class="gb-fe" :data-n="gutter(s, i)" :data-event="it.event.id" :event="it.event" :fields="fields" />
            <li v-else class="gb-fe" :data-n="gutter(s, i)" :data-event="it.event.id">
              <div v-if="it.resolved !== undefined && !open.has(it.event.id)" class="gb-fthread">
                <span>
                  <b>{{ it.event.author.name }}</b> on
                  <button type="button" class="gb-anchor" @click="emit('line', it.event.review!.path!)">{{ where(it.event) }}</button>
                  <span class="gb-fthread-q"> · “{{ firstWords(it.event) }}”</span>
                </span>
                <button type="button" class="gb-tchip" :class="{ 'is-open': !it.resolved }" aria-expanded="false" @click="open.add(it.event.id)">
                  {{ it.replies.length + 1 }} comment{{ it.replies.length ? 's' : '' }} · {{ it.resolved ? 'resolved' : 'open' }} ▸
                </button>
              </div>
              <div v-else class="gb-fthread-open" :class="{ 'is-thread': it.resolved !== undefined }">
                <CommentCard v-for="c in [it.event, ...it.replies]" :key="c.id" :data-event="c.id" :event="c" :forges="forges" :mine="c.author.id === meId" :save="save" @ref="(n, ev) => emit('ref', n, ev)" @line="(p) => emit('line', p)" />
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
import { divider, firstNew, fold, pick, segments, type Item, type Reading, type Segment } from '@/flow';
import { on, when } from '@/me';
import CommentCard from '@/components/CommentCard.vue';
import TimelineEvent from '@/components/TimelineEvent.vue';

const props = defineProps<{
  events: IssueEvent[];
  cycle: Cycle | null;
  meId: string;
  since: number | null; // the last visit, for the "new since" line
  forges?: Forge[] | undefined;
  fields: Fields;
  save: (id: string, body: string, base: string) => Promise<unknown>;
  reading: Reading; // the panel's, the flow writes its picks into it
}>();
const reading = toRef(props, 'reading');
const emit = defineEmits<{ line: [path: string]; ref: [n: string, event: MouseEvent] }>();

const step = computed(() => props.cycle?.step ?? 0);
const segs = computed(() => segments(props.events, props.cycle?.stages, step.value, props.cycle?.skipped));
const read = (r: Reading) => pick(segs.value, props.events, r, props.meId, step.value);
const sum = (ss: Segment[]) => ss.reduce((n, s) => n + s.entries.length, 0);
const count = (r: Partial<Reading>) => sum(read({ ...reading.value, ...r }));
// a toggle's count: what it alone would show
const only = (k: 'mine' | 'agent' | 'milestones') => ({ mine: false, agent: false, milestones: false, [k]: true });
const shown = computed(() => read(reading.value));
const total = computed(() => sum(shown.value));
const narrowed = computed(() => reading.value.show !== 'all' || reading.value.mine || reading.value.agent || reading.value.milestones);
const threads = computed(() => reading.value.threads);
const items = (s: Segment): Item[] => (reading.value.threads ? fold(s.entries) : s.entries.map((event) => ({ event, replies: [] })));

// What is folded: under This step the segments before the current one,
// pointed at above it; under All, a long flow's segments before the
// current one show their divider only.
// ponytail: 100 entries is a guess at where the page gets slow to read
const LONG = 100;
const unfolded = ref(false);
const folded = computed((): Segment[] => {
  const before = (ss: Segment[]) => ss.filter((s) => s.n && s.n < step.value);
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
const where = (e: IssueEvent) => `${e.review!.path}${e.review!.line ? `:${e.review!.line}` : ''}`;
const firstWords = (e: IssueEvent) => {
  const line = (e.body ?? '').trim().split('\n')[0] ?? '';
  return line.length > 60 ? `${line.slice(0, 60)}…` : line;
};
const open = reactive(new Set<string>()); // the threads unfolded

const sinceIso = computed(() => (props.since === null ? undefined : new Date(props.since).toISOString()));
const newId = computed(() => firstNew(shown.value, props.since, props.meId));

const root = ref<HTMLElement | null>(null);
// jump goes to a segment, the rail's: shown first if it is folded. Asked
// before the cycle has loaded, it waits for the segment.
let pending: number | undefined;
async function jump(n: number) {
  pending = undefined;
  if (!segs.value.some((s) => s.n === n)) {
    if (!props.cycle) pending = n;
    return;
  }
  if (!shown.value.some((s) => s.n === n)) reading.value.show = 'all';
  unfolded.value = true;
  await nextTick();
  const el = root.value?.querySelector<HTMLElement>(`#step-${n}`);
  el?.scrollIntoView({ block: 'start', behavior: 'smooth' });
  el?.focus({ preventScroll: true });
}
watch(segs, () => pending !== undefined && void jump(pending));
// unfold renders the whole flow, and shows what id is (a thread holding
// it unfolded, the filters let go if they hide it): what the inbox or
// the sidebar scrolls to is there
function unfold(id?: string) {
  unfolded.value = true;
  const e = id ? props.events.find((x) => x.id === id) : undefined;
  if (!e || e.kind === 'created') return;
  if (!shown.value.some((s) => s.entries.includes(e))) Object.assign(reading.value, { show: 'all', mine: false, agent: false, milestones: false });
  if (e.review?.path) open.add(e.review.reply_to ?? e.id);
}
defineExpose({ jump, unfold });
</script>

<style scoped>
.gb-flow { display: flex; flex-direction: column; }
.gb-flow-controls { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 10px 16px; margin-bottom: 10px; }
.gb-flow-chips, .gb-flow-segs { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.gb-flow-chips .gb-eyebrow { margin-right: 6px; }
.gb-flow-segs { gap: 8px 14px; }
.gb-fchip, .gb-tchip { font: inherit; font-size: .75rem; padding: 4px 10px; border: 1px solid var(--border-strong); border-radius: var(--radius); background: var(--bg); color: var(--fg); white-space: nowrap; cursor: pointer; }
.gb-fchip[aria-pressed="true"] { border-color: var(--fg); background: var(--bg-dark); color: var(--fg-inverse); }
.gb-tchip { padding: 4px 8px; text-align: left; }
.gb-tchip.is-open { border-color: var(--gb-done); color: var(--gb-done); }
.gb-flow-ptr { margin: 12px 0 0; font-size: .875rem; color: var(--fg-muted); }
.gb-fdiv { display: flex; align-items: center; gap: 12px; margin: 22px 0 4px; font-size: .75rem; letter-spacing: .12em; text-transform: uppercase; color: var(--gb-done); font-weight: 700; }
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
.gb-fthread-q { color: var(--fg-muted); font-style: italic; }
.gb-anchor { padding: 0; border: 0; background: none; font-family: var(--font-mono); font-size: .75rem; color: var(--gb-done); white-space: nowrap; cursor: pointer; }
.gb-anchor:hover { text-decoration: underline; }
.gb-fthread-open { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
.gb-fthread-open.is-thread { animation: gb-unfold 120ms ease-out; }
@keyframes gb-unfold { from { opacity: 0; transform: translateY(-4px); } }
@media (prefers-reduced-motion: reduce) { .gb-fthread-open.is-thread { animation: none; } }
.gb-fnew { display: flex; align-items: center; gap: 12px; margin: 12px 0 2px; font-size: .75rem; font-weight: 700; color: var(--primary-hover); }
.gb-fnew::before, .gb-fnew::after { content: ""; flex: 1 1 auto; border-top: 2px solid var(--primary-hover); }
</style>
