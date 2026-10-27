<!-- An issue's whole history (the panel's History mode): every change in
     order with who and when, as diffs, and the issue as it was after any of
     them (the slider, or a click on a change). Built from the thread
     (replay.ts): git-bug keeps every operation, so nothing is lost. -->
<template>
  <section class="hv" aria-label="History">
    <p class="hv-intro"><span>The same flow, with a cursor: pick a moment, read the issue as it was then.</span><span class="hv-muted">◆ a version · ◇ a tick · ■ a text</span></p>
    <IssueTimeline :steps="all" :current="pos" :fields="fields" :segments="segs" @select="select" />
    <div class="hv-scrub">
      <button type="button" class="gb-icon-btn" aria-label="Previous change" :disabled="pos <= 0" @click="pos--"><GbIcon name="back" small /></button>
      <input v-model.number="pos" class="hv-range" type="range" min="0" :max="all.length - 1" aria-label="As of" />
      <button type="button" class="gb-icon-btn hv-fwd" aria-label="Next change" :disabled="pos >= all.length - 1" @click="pos++"><GbIcon name="back" small /></button>
      <p class="hv-asof-line">
        As of <b>{{ when(step.at, true) }}</b> <span class="hv-muted">· change {{ pos + 1 }} of {{ all.length }}</span>
        <button v-if="pos < all.length - 1" type="button" class="hv-latest" @click="pos = all.length - 1">Now</button>
      </p>
    </div>

    <article class="hv-asof" :aria-label="`The issue as of ${when(step.at, true)}`">
      <span class="gb-eyebrow">As of {{ when(step.at, true) }} · what the change touched is outlined</span>
      <div class="hv-asof-head">
        <span :class="['epfl-badge', state.status === 'open' ? 'epfl-badge-tertiary' : 'epfl-badge-dark']">
          <GbIcon :name="state.status === 'open' ? 'open' : 'closed'" small />{{ state.status === 'open' ? 'Open' : 'Closed' }}
        </span>
        <h3 class="hv-asof-title" :class="{ 'hv-hit': step.kind === 'title' }">{{ state.title }}</h3>
      </div>
      <ul v-if="state.labels.length" class="hv-labels">
        <li v-for="l in state.labels" :key="l" class="gb-val" :class="[colorOf(l), { 'hv-hit-label': step.added?.includes(l) }]">{{ labelText(l) }}</li>
      </ul>
      <div class="hv-boxes">
        <div v-if="desc" class="hv-box" :class="{ 'hv-hit': step.version !== undefined }">
          <span class="gb-eyebrow">The description, as of then</span>
          <span>v{{ desc.v }}, published {{ when(desc.at, true) }} · {{ desc.criteria }}</span>
          <span v-if="desc.threads" class="hv-muted">threads: {{ desc.threads }}</span>
        </div>
        <div class="hv-box">
          <span class="gb-eyebrow">Texts as of then</span>
          <span>{{ texts.comments + texts.versions }} · {{ texts.comments }} comment{{ texts.comments === 1 ? '' : 's' }} and {{ texts.versions }} version{{ texts.versions === 1 ? '' : 's' }}</span>
          <span class="hv-muted">the log below is cut at this change</span>
        </div>
        <div class="hv-box">
          <span class="gb-eyebrow">This change</span>
          <span><b>{{ step.author.name }}</b> {{ whatOf(step) }}<template v-if="step.kind === 'labels'"> · {{ labelChanges(step).map((c) => `${c.field === 'label' ? 'label' : displayName(c.field)}: ${[c.before.join(', '), c.after.join(', ')].filter(Boolean).join(' → ')}`).join(' · ') }}</template></span>
        </div>
      </div>
      <div v-if="touched" class="hv-comment hv-hit">
        <p class="hv-comment-head"><b>{{ touched.author.name }}</b> {{ touched.kind === 'created' ? 'opened' : 'commented' }} {{ on(touched.at) }}<span v-if="touched.edits" class="hv-muted"> · edited {{ touched.edits }}×</span></p>
        <p class="hv-text">{{ touched.body || '(no text)' }}</p>
      </div>
      <details v-if="state.comments.length" class="hv-all">
        <summary>All {{ state.comments.length }} {{ state.comments.length === 1 ? 'text' : 'texts' }} as of then</summary>
        <div v-for="c in state.comments" :key="c.id" class="hv-comment">
          <p class="hv-comment-head"><b>{{ c.author.name }}</b> {{ c.kind === 'created' ? 'opened' : 'commented' }} {{ on(c.at) }}<span v-if="c.edits" class="hv-muted"> · edited {{ c.edits }}×</span></p>
          <p class="hv-text">{{ c.body || '(no text)' }}</p>
        </div>
      </details>
    </article>

    <div class="hv-log">
      <div class="hv-log-bar">
        <span class="gb-seg gb-seg-sm" role="group" aria-label="Changes">
          <button v-for="c in chips" :key="c.key" type="button" :aria-pressed="filter === c.key" @click="filter = c.key">{{ c.label }} {{ c.n }}</button>
        </span>
        <span class="gb-seg gb-seg-sm" role="group" aria-label="Order">
          <button type="button" :aria-pressed="!newest" @click="newest = false">Oldest first</button>
          <button type="button" :aria-pressed="newest" @click="newest = true">Newest first</button>
        </span>
      </div>
      <div class="hv-table">
        <table ref="list">
          <thead><tr><th>#</th><th>Time</th><th>Who</th><th>Operation</th><th>What</th><th>Op id</th><th v-if="forge">{{ forge.name }}{{ forge.number ? ` #${forge.number}` : '' }}</th></tr></thead>
          <tbody>
            <template v-for="r in rows" :key="r.key">
              <tr v-if="r.fold" class="hv-fold"><td :colspan="cols">{{ r.fold }} later change{{ r.fold === 1 ? '' : 's' }}, after the cursor · <button type="button" class="hv-link" @click="later = !later">{{ later ? 'hide' : 'show' }}</button></td></tr>
              <tr v-else-if="r.day" class="hv-day"><td :colspan="cols">{{ r.day }}</td></tr>
              <template v-else-if="r.s">
                <tr :data-step="r.i" :class="{ 'hv-at': r.i === pos, 'hv-dim': r.i > pos }" :aria-current="r.i === pos ? 'step' : undefined">
                  <td><button type="button" class="hv-n" :aria-label="`Move the cursor to change ${r.i + 1}`" @click="pos = r.i">{{ r.i + 1 }}</button></td>
                  <td class="hv-t"><time :datetime="r.s.at">{{ hhmm(r.s.at) }}</time></td>
                  <td>{{ r.s.author.name }}</td>
                  <td class="hv-op">{{ r.s.op }}</td>
                  <td>{{ whatOf(r.s) }}<template v-if="r.s.kind === 'meta'">{{ onWhat(r.s) }} <span class="hv-mono">{{ Object.keys(r.s.meta ?? {}).join(', ') }}</span></template></td>
                  <td class="hv-mono" :title="r.s.id">{{ short(r.s.id) }}</td>
                  <td v-if="forge" :class="{ 'hv-muted': r.s.pending.length || r.s.kind === 'meta' }">{{ forgeCell(r.s) }}</td>
                </tr>
                <tr v-if="r.i === pos && hasDetail(r.s)" class="hv-at-detail">
                  <td :colspan="cols">
                    <p v-if="r.s.kind === 'title'" class="hv-words">
                      <span v-for="(t, j) in wordDiff(r.s.before ?? '', r.s.after ?? '')" :key="j" :class="wordClass(t.op)">{{ t.text }}</span>
                    </p>
                    <ul v-else-if="r.s.kind === 'labels'" class="hv-changes">
                      <li v-for="(c, j) in labelChanges(r.s)" :key="j">
                        <b>{{ c.field === 'label' ? 'Label' : displayName(c.field) }}</b>
                        <template v-if="c.before.length">
                          <span v-for="v in c.before" :key="'b' + v" class="gb-val hv-out" :class="colorClass(c.field, v)">{{ valueText(c.field, v) }}</span>
                        </template>
                        <span v-if="c.before.length && c.after.length" class="hv-arrow" aria-label="becomes">→</span>
                        <span v-for="v in c.after" :key="'a' + v" class="gb-val" :class="colorClass(c.field, v)">{{ valueText(c.field, v) }}</span>
                        <span v-if="!c.after.length" class="gb-hint">removed</span>
                      </li>
                    </ul>
                    <p v-else-if="r.s.kind === 'created' || r.s.kind === 'comment'" class="hv-text hv-excerpt">{{ r.s.after || '(no text)' }}</p>
                    <div v-else-if="r.s.kind === 'edit'" class="hv-diff" aria-label="What the edit changed">
                      <div v-for="(l, j) in folded(r.s)" :key="j" :class="`hv-d hv-d-${l.kind}`">
                        <span class="hv-d-op" aria-hidden="true">{{ l.kind === 'add' ? '+' : l.kind === 'del' ? '−' : '' }}</span>
                        <span v-if="l.words"><span v-for="(t, k) in l.words" :key="k" :class="{ 'hv-wchg': t.op !== ' ' }">{{ t.text }}</span></span>
                        <span v-else>{{ l.text || ' ' }}</span>
                      </div>
                    </div>
                  </td>
                </tr>
              </template>
            </template>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import type { Cycle, Thread } from '@/api/board';
import { colorClass } from '@/colors';
import { displayName, formatValue, type Fields } from '@/fields';
import { describeLabels } from '@/history';
import { bodyThreads, resolvedBy, segments } from '@/flow';
import { anchorLabel, anchorOf, criteriaOf, sectionsOf, stateOf, versions } from '@/sections';
import { on, when } from '@/me';
import { asOf, metaWhat, stateAfter, steps, textDiff, wordDiff, type Step, type WordOp } from '@/replay';
import GbIcon from '@/components/GbIcon.vue';
import IssueTimeline from '@/components/IssueTimeline.vue';

const props = defineProps<{ thread: Thread; fields: Fields; cycle?: Cycle | null }>();

const all = computed(() => steps(props.thread.events));
// the step bars: the flow's segments, with no cycle none
const segs = computed(() => (props.cycle ? segments(props.thread.events, props.cycle.stages, props.cycle.step, props.cycle.skipped) : []));
const pos = ref(0);
// the latest change when the history opens or grows
watch(() => all.value.length, (n) => (pos.value = n - 1), { immediate: true });
const step = computed(() => all.value[Math.min(pos.value, all.value.length - 1)]!);

// a marker of the timeline: that change, and its place in the list
const list = ref<HTMLElement | null>(null);
async function select(i: number) {
  pos.value = i;
  await nextTick();
  list.value?.querySelector(`[data-step="${i}"]`)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
}
// the log: chips and order are component state (ruling 4); the rows
// after the cursor fold to one row until shown
type Filter = 'all' | 'texts' | 'versions' | 'fields' | 'pending';
const filter = ref<Filter>('all');
const newest = ref(false);
const later = ref(false);
const forge = computed(() => props.thread.forges?.[0]); // ponytail: the first forge's column only
const cols = computed(() => (forge.value ? 7 : 6));
const groupOf = (s: Step): Filter | undefined =>
  s.version !== undefined ? 'versions' : s.kind === 'comment' || s.kind === 'edit' ? 'texts' : s.kind === 'meta' ? undefined : 'fields';
const keep = (s: Step, f: Filter) => f === 'all' || (f === 'pending' ? s.pending.length > 0 : groupOf(s) === f);
const chips = computed(() => {
  const c: { key: Filter; label: string; n: number }[] = [
    { key: 'all', label: 'All', n: 0 }, { key: 'texts', label: 'Texts', n: 0 },
    { key: 'versions', label: 'Versions and ticks', n: 0 }, { key: 'fields', label: 'Fields', n: 0 },
  ];
  if (forge.value) c.push({ key: 'pending', label: `Not on ${forge.value.name} yet`, n: 0 });
  for (const ch of c) ch.n = all.value.filter((s) => keep(s, ch.key)).length;
  return c;
});
type Row = { key: string; i: number; s?: Step; day?: string; fold?: number };
const day = (at: string) => new Date(at).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
const hhmm = (at: string) => new Date(at).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
const rows = computed(() => {
  const shown = all.value.map((s, i) => ({ s, i })).filter(({ s }) => keep(s, filter.value));
  const after = shown.filter(({ i }) => i > pos.value).length;
  const list = shown.filter(({ i }) => later.value || i <= pos.value);
  if (newest.value) list.reverse();
  const out: Row[] = [];
  let last = '';
  for (const { s, i } of list) {
    const d = day(s.at);
    if (d !== last) out.push({ key: `d${i}`, i, day: (last = d) });
    out.push({ key: s.key, i, s });
  }
  if (after) out[newest.value ? 'unshift' : 'push']({ key: 'fold', i: -1, fold: after });
  return out;
});
// an op id: the blob's first 7 and the index
const short = (id: string) => {
  const [h, n] = id.split(':');
  return n === undefined ? h!.slice(0, 7) : `${h!.slice(0, 7)}:${n}`;
};
// a set-meta's thread, by its anchor
function onWhat(s: Step): string {
  const a = anchorOf(s.event.meta?.['thread:anchor']);
  const created = props.thread.events.find((e) => e.kind === 'created');
  return a ? ` on ${anchorLabel(a, sectionsOf(created?.html ?? ''))} ·` : ' ·';
}
// the forge column (ruling 7): review:* stays local, the other keys are M5's
function forgeCell(s: Step): string {
  if (s.pending.length) return 'not yet';
  if (s.kind === 'meta') return Object.keys(s.meta ?? {}).every((k) => k.startsWith('review:')) ? 'local' : '—';
  return `on ${forge.value?.name}`;
}
const hasDetail = (s: Step) => s.kind !== 'status' && s.kind !== 'meta';
const state = computed(() => stateAfter(all.value, pos.value, props.thread.title));
// the description and its threads as of then: M2's and M3's rules on the
// events as they were (asOf)
const then = computed(() => asOf(all.value, pos.value));
const desc = computed(() => {
  const created = then.value.find((e) => e.kind === 'created');
  if (!created) return undefined;
  const vs = versions(created);
  const v = vs.at(-1)!;
  const { n, ticked } = criteriaOf(created.body ?? '');
  const secs = sectionsOf(created.html ?? '');
  const threads = bodyThreads(then.value).map((t) => {
    const a = anchorOf(t.event.meta?.['thread:anchor']);
    const st = stateOf(t.event, vs);
    const what = resolvedBy(t, then.value) || (st.state === 'carried' ? `open, carried from v${a!.v}` : st.state === 'outdated' ? `open, written on v${a!.v}` : 'open');
    return `${a ? anchorLabel(a, secs) : '§'} ${what}`;
  });
  return { v: v.n, at: v.at, criteria: `${n} criteri${n === 1 ? 'on' : 'a'}, ${ticked ? `${ticked} ticked` : 'none ticked'}`, threads: threads.join(' · ') };
});
const texts = computed(() => ({ comments: then.value.filter((e) => e.kind === 'comment').length, versions: desc.value?.v ?? 0 }));
// the text the current change wrote, as it was then
const touched = computed(() => (['created', 'comment', 'edit'].includes(step.value.kind) ? state.value.comments.find((c) => c.id === step.value.event.id) : undefined));

const split = (l: string): [string, string] => {
  const i = l.indexOf(':');
  return i < 0 ? ['label', l] : [l.slice(0, i), l.slice(i + 1)];
};
const valueText = (field: string, v: string) => formatValue(field, v, props.fields);
const labelText = (l: string) => {
  const [f, v] = split(l);
  return f === 'label' ? v : `${displayName(f)}: ${valueText(f, v)}`;
};
const colorOf = (l: string) => {
  const [f, v] = split(l);
  return f === 'label' ? '' : colorClass(f, v);
};

// a label change as each field before → after
function labelChanges(s: Step): { field: string; before: string[]; after: string[] }[] {
  return describeLabels(s.added ?? [], s.removed ?? []).reduce<{ field: string; before: string[]; after: string[] }[]>((out, c) => {
    let row = out.find((r) => r.field === c.field);
    if (!row) out.push((row = { field: c.field, before: [], after: [] }));
    if (c.verb === 'set') {
      row.after.push(...c.values);
      row.before.push(...(s.removed ?? []).filter((l) => split(l)[0] === c.field).map((l) => split(l)[1]));
    } else if (c.verb === 'added') row.after.push(...c.values);
    else row.before.push(...c.values);
    return out;
  }, []);
}

function whatOf(s: Step): string {
  switch (s.kind) {
    case 'created': return 'opened the issue';
    case 'comment': return 'commented';
    case 'edit': return s.event.kind === 'created' ? 'edited the description' : `edited ${s.event.author.id === s.author.id ? 'their' : `${s.event.author.name}'s`} comment`;
    case 'labels': return 'changed fields';
    case 'status': return s.status === 'closed' ? 'closed the issue' : 'reopened the issue';
    case 'title': return 'renamed the issue';
    case 'meta': return metaWhat(s.meta);
  }
}
const wordClass = (op: WordOp['op']) => (op === '+' ? 'hv-ins' : op === '-' ? 'hv-del' : '');

// an edit's diff, unchanged runs folded to two lines around the changes
function folded(s: Step): { kind: 'same' | 'add' | 'del' | 'skip'; text: string; words?: WordOp[] | undefined }[] {
  const lines = textDiff(s.before ?? '', s.after ?? '');
  const keep = lines.map((l, i) => l.op !== ' ' || lines.slice(Math.max(0, i - 2), i + 3).some((x) => x.op !== ' '));
  const out: { kind: 'same' | 'add' | 'del' | 'skip'; text: string; words?: WordOp[] | undefined }[] = [];
  let skipped = 0;
  lines.forEach((l, i) => {
    if (!keep[i]) {
      skipped++;
      return;
    }
    if (skipped) out.push({ kind: 'skip', text: `… ${skipped} unchanged line${skipped === 1 ? '' : 's'}` });
    skipped = 0;
    out.push({ kind: l.op === '+' ? 'add' : l.op === '-' ? 'del' : 'same', text: l.text, words: l.words });
  });
  if (skipped) out.push({ kind: 'skip', text: `… ${skipped} unchanged line${skipped === 1 ? '' : 's'}` });
  return out;
}
</script>

<style scoped>
.hv { display: grid; gap: 1rem; }
.hv-scrub { position: sticky; top: 0; z-index: 2; display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; padding: .625rem .75rem; background: var(--bg); border: 1px solid var(--border); box-shadow: 0 2px 6px rgba(0, 0, 0, .04); }
.hv-range { flex: 1 1 12rem; accent-color: var(--primary); }
.hv-fwd :deep(svg) { transform: scaleX(-1); }
.hv-asof-line { flex-basis: 100%; margin: 0; font-size: .875rem; }
.hv-intro { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: baseline; gap: 8px 16px; margin: 0; font-size: .9375rem; }
.hv-intro .hv-muted { font-size: .8125rem; }
.hv-latest { margin-left: .5rem; padding: 0; border: 0; background: none; color: var(--fg); font: inherit; font-weight: 700; text-decoration: underline; cursor: pointer; }

.hv-asof { display: grid; gap: .625rem; padding: 1rem 1.25rem; border: 1px dashed var(--border-strong); background: var(--bg-subtle); }
.hv-asof-head { display: flex; flex-wrap: wrap; align-items: center; gap: .625rem; }
.hv-asof-title { margin: 0; font-size: 1.125rem; font-weight: 700; }
.hv-labels { display: flex; flex-wrap: wrap; gap: .375rem; margin: 0; padding: 0; list-style: none; }
.hv-boxes { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(280px, 100%), 1fr)); gap: .75rem; font-size: .875rem; }
.hv-box { display: flex; flex-direction: column; gap: 4px; padding: .75rem .875rem; background: var(--bg); border: 1px solid var(--border-subtle); }
.hv-box .hv-muted { font-size: .8125rem; }
.hv-comment { padding: .5rem .75rem; background: var(--bg); border: 1px solid var(--border-subtle); }
.hv-comment-head { margin: 0 0 .25rem; font-size: .8125rem; }
.hv-text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: .875rem; line-height: 1.55; max-height: 14rem; overflow: auto; }
.hv-hit { outline: 2px solid var(--primary); outline-offset: 2px; }
.hv-muted { color: var(--fg-muted); font-weight: 400; }
.hv-all summary { cursor: pointer; font-size: .875rem; font-weight: 700; }
.hv-all[open] { display: grid; gap: .5rem; }
.hv-hit-label { outline: 2px solid var(--primary); outline-offset: 1px; }

.hv-log { display: grid; gap: .625rem; min-width: 0; }
.hv-log-bar { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: .625rem 1rem; }
.hv-log-bar .gb-seg { flex-wrap: wrap; }
.hv-table { overflow-x: auto; border-top: 1px solid var(--border); }
.hv-table table { width: 100%; border-collapse: collapse; font-size: .875rem; }
.hv-table th { padding: .5rem .625rem; text-align: left; font-size: .75rem; font-weight: 700; letter-spacing: .06em; text-transform: uppercase; color: var(--fg-muted); white-space: nowrap; }
.hv-table td { padding: .5rem .625rem; border-top: 1px solid var(--border-subtle); vertical-align: top; }
.hv-day td { padding-top: .875rem; font-weight: 700; border-top: 0; }
.hv-fold td { color: var(--fg-muted); text-align: center; background: var(--bg-subtle); }
.hv-t, .hv-op, .hv-mono { font-family: var(--font-mono); font-size: .8125rem; white-space: nowrap; }
.hv-n { min-width: 1.75rem; padding: 0 .25rem; border: 1px solid var(--border); background: var(--bg); font: inherit; font-family: var(--font-mono); cursor: pointer; }
.hv-n:hover { border-color: var(--fg); }
.hv-at td { background: var(--gb-teal-bg); }
.hv-at td:first-child { box-shadow: inset 3px 0 0 var(--gb-red-dot); }
.hv-at-detail td { border-top: 0; background: var(--gb-teal-bg); box-shadow: inset 3px 0 0 var(--gb-red-dot); }
.hv-dim { opacity: .5; }
.hv-link { padding: 0; border: 0; background: none; color: var(--fg); font: inherit; text-decoration: underline; cursor: pointer; }
.hv-words { margin: 0; font-size: .9375rem; }
.hv-ins { background: var(--success-bg); color: var(--success-fg); text-decoration: none; }
.hv-del { background: var(--danger-bg); color: var(--danger-fg); text-decoration: line-through; }
.hv-changes { display: grid; gap: .25rem; margin: 0; padding: 0; list-style: none; font-size: .875rem; }
.hv-changes li { display: flex; flex-wrap: wrap; align-items: center; gap: .375rem; }
.hv-out { opacity: .6; text-decoration: line-through; }
.hv-arrow { color: var(--fg-muted); }
.hv-excerpt { max-height: 6.5rem; color: var(--fg-muted); }
.hv-diff { padding: .375rem 0; border: 1px solid var(--border); background: var(--bg); font-family: var(--font-mono); font-size: .8125rem; line-height: 1.55; white-space: pre-wrap; overflow-wrap: anywhere; max-height: 22rem; overflow: auto; }
.hv-d { display: flex; gap: .25rem; padding: 0 .625rem; }
.hv-d-op { flex: none; width: 1rem; color: var(--fg-subtle); }
.hv-d-add { background: var(--success-bg); color: var(--success-fg); }
.hv-d-del { background: var(--danger-bg); color: var(--danger-fg); }
.hv-d-skip { color: var(--fg-subtle); font-style: italic; }
.hv-d-add .hv-wchg { background: color-mix(in srgb, var(--success) 28%, transparent); font-weight: 700; }
.hv-d-del .hv-wchg { background: color-mix(in srgb, var(--danger) 22%, transparent); text-decoration: line-through; }
</style>
