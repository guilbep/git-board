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
      <div class="hv-asof-head">
        <span :class="['epfl-badge', state.status === 'open' ? 'epfl-badge-tertiary' : 'epfl-badge-dark']">
          <GbIcon :name="state.status === 'open' ? 'open' : 'closed'" small />{{ state.status === 'open' ? 'Open' : 'Closed' }}
        </span>
        <h3 class="hv-asof-title" :class="{ 'hv-hit': step.kind === 'title' }">{{ state.title }}</h3>
      </div>
      <ul v-if="state.labels.length" class="hv-labels">
        <li v-for="l in state.labels" :key="l" class="gb-val" :class="[colorOf(l), { 'hv-hit-label': step.added?.includes(l) }]">{{ labelText(l) }}</li>
      </ul>
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

    <ol ref="list" class="hv-steps">
      <li
        v-for="(s, i) in all"
        :key="s.key"
        class="hv-step"
        :data-step="i"
        :class="{ 'hv-current': i === pos, 'hv-future': i > pos }"
        :aria-current="i === pos ? 'step' : undefined"
      >
        <button type="button" class="hv-step-head" @click="pos = i">
          <span class="hv-icon"><GbIcon :name="iconOf(s)" small /></span>
          <span class="hv-what"><b>{{ s.author.name }}</b> {{ whatOf(s) }}</span>
          <time class="gb-hint" :datetime="s.at">{{ when(s.at, true) }}</time>
        </button>
        <p v-if="s.pending.length" class="gb-fstate gb-fstate-local hv-pending"><GbIcon name="clock" small /> not on {{ s.pending.join(' or ') }} yet: next sync</p>

        <p v-if="s.kind === 'title'" class="hv-words">
          <span v-for="(t, j) in wordDiff(s.before ?? '', s.after ?? '')" :key="j" :class="wordClass(t.op)">{{ t.text }}</span>
        </p>
        <ul v-else-if="s.kind === 'labels'" class="hv-changes">
          <li v-for="(c, j) in labelChanges(s)" :key="j">
            <b>{{ c.field === 'label' ? 'Label' : displayName(c.field) }}</b>
            <template v-if="c.before.length">
              <span v-for="v in c.before" :key="'b' + v" class="gb-val hv-out" :class="colorClass(c.field, v)">{{ valueText(c.field, v) }}</span>
            </template>
            <span v-if="c.before.length && c.after.length" class="hv-arrow" aria-label="becomes">→</span>
            <span v-for="v in c.after" :key="'a' + v" class="gb-val" :class="colorClass(c.field, v)">{{ valueText(c.field, v) }}</span>
            <span v-if="!c.after.length" class="gb-hint">removed</span>
          </li>
        </ul>
        <p v-else-if="s.kind === 'created' || s.kind === 'comment'" class="hv-text hv-excerpt">{{ s.after || '(no text)' }}</p>
        <div v-else-if="s.kind === 'edit'" class="hv-diff" :aria-label="`What the edit changed`">
          <div v-for="(l, j) in folded(s)" :key="j" :class="`hv-d hv-d-${l.kind}`">
            <span class="hv-d-op" aria-hidden="true">{{ l.kind === 'add' ? '+' : l.kind === 'del' ? '−' : '' }}</span>
            <span v-if="l.words"><span v-for="(t, k) in l.words" :key="k" :class="{ 'hv-wchg': t.op !== ' ' }">{{ t.text }}</span></span>
            <span v-else>{{ l.text || ' ' }}</span>
          </div>
        </div>
      </li>
    </ol>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import type { Cycle, Thread } from '@/api/board';
import { colorClass } from '@/colors';
import { displayName, formatValue, type Fields } from '@/fields';
import { describeLabels } from '@/history';
import { segments } from '@/flow';
import { on, when } from '@/me';
import { metaWhat, stateAfter, steps, textDiff, wordDiff, type Step, type WordOp } from '@/replay';
import GbIcon, { type IconName } from '@/components/GbIcon.vue';
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
const state = computed(() => stateAfter(all.value, pos.value, props.thread.title));
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
function iconOf(s: Step): IconName {
  switch (s.kind) {
    case 'created': return 'plus';
    case 'comment': case 'edit': return 'edit';
    case 'labels': return (s.added ?? []).concat(s.removed ?? []).every((l) => l.startsWith('assignee:')) ? 'user' : 'tag';
    case 'status': return s.status === 'closed' ? 'closed' : 'open';
    case 'title': return 'title';
    case 'meta': return 'check';
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
.hv-comment { padding: .5rem .75rem; background: var(--bg); border: 1px solid var(--border-subtle); }
.hv-comment-head { margin: 0 0 .25rem; font-size: .8125rem; }
.hv-text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: .875rem; line-height: 1.55; max-height: 14rem; overflow: auto; }
.hv-hit { outline: 2px solid var(--primary); outline-offset: 2px; }
.hv-muted { color: var(--fg-muted); font-weight: 400; }
.hv-all summary { cursor: pointer; font-size: .875rem; font-weight: 700; }
.hv-all[open] { display: grid; gap: .5rem; }
.hv-hit-label { outline: 2px solid var(--primary); outline-offset: 1px; }

.hv-steps { display: grid; gap: 0; margin: 0 0 0 6px; padding: 0; list-style: none; border-left: 2px solid var(--border); }
.hv-step { position: relative; display: grid; gap: .375rem; padding: .625rem 0 .875rem 1.25rem; }
.hv-step::before { content: ''; position: absolute; left: -6px; top: 1.05rem; width: 10px; height: 10px; border-radius: 50%; background: var(--bg); border: 2px solid var(--border-strong); }
.hv-current::before { background: var(--primary); border-color: var(--primary); }
.hv-future { opacity: .45; }
.hv-step-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: .375rem .5rem; padding: 0; border: 0; background: none; color: var(--fg); font: inherit; font-size: .9375rem; text-align: left; cursor: pointer; }
.hv-step-head:hover .hv-what { text-decoration: underline; }
.hv-current .hv-step-head { font-weight: 400; }
.hv-icon { align-self: center; display: inline-flex; color: var(--fg-muted); }
.hv-what { flex: 1 1 auto; }
.hv-pending { margin: 0; }
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
