<!-- The Changes tab (docs/design/time-flow, 6e): the description's
     versions, one card each with what changed, the threads it settled and
     its verdict, then its ticks. The rules are sections.ts's and spec.ts's. -->
<template>
  <section v-if="created" class="ic" aria-label="Changes">
    <div class="ic-head">
      <h2 class="ic-title">The description · {{ plural(vs.length, 'version') }}<template v-if="ticks.length">, {{ plural(ticks.length, 'tick') }}</template></h2>
      <span class="ic-muted">A version is an edit that changes text. An edit that only flips checkboxes is a tick, listed, not numbered.</span>
    </div>
    <div class="ic-vers">
      <template v-for="(c, i) in cards" :key="c.n">
        <span v-if="i" class="ic-arrow" aria-hidden="true">→</span>
        <div class="ic-ver" :class="{ 'is-on': c.n === vs.length }">
          <span><b>v{{ c.n }}</b> <span class="ic-muted">· {{ when(c.at, true) }} · {{ c.author }}</span></span>
          <span class="ic-muted ic-small">{{ c.what }}</span>
          <span class="ic-small" :class="c.verdict?.verdict === 'approve' ? 'ic-ok' : c.verdict ? 'ic-ko' : 'ic-muted'">{{ c.said }}<template v-if="c.n === vs.length"> · the current text</template></span>
        </div>
      </template>
      <div v-if="ticks.length" class="ic-ver is-ticks">
        <span><b>ticks</b> <span class="ic-muted">· {{ on(ticks.at(-1)!.at) }} · {{ [...new Set(ticks.map((t) => t.author.name))].join(', ') }}</span></span>
        <span class="ic-muted ic-small">{{ tickWords }}</span>
        <span class="ic-muted ic-small">{{ plural(ticks.length, 'edit') }} in History · not a version</span>
      </div>
    </div>

    <template v-if="vs.length > 1">
      <div class="ic-bar">
        <span class="gb-seg gb-seg-sm" role="group" aria-label="Compare">
          <button v-for="v in vs.slice(1)" :key="v.n" type="button" :aria-pressed="to === v.n" @click="to = v.n">v{{ v.n - 1 }} → v{{ v.n }}</button>
        </span>
        <span class="gb-seg gb-seg-sm" role="group" aria-label="Layout">
          <button type="button" :aria-pressed="!side" @click="side = false">Inline</button>
          <button type="button" :aria-pressed="side" @click="side = true">Side by side</button>
        </span>
        <label class="ic-small ic-check"><input v-model="withThreads" type="checkbox" /> Threads written on these lines</label>
      </div>
      <article class="ic-diff" :class="{ 'is-side': side }" :aria-label="`v${to - 1} to v${to}`">
        <section v-for="s in diffSecs" :key="s.slug" class="ic-sec" :class="{ 'is-changed': changed(s) !== 'unchanged' }" :aria-label="`§${s.n} ${s.heading || 'intro'}`">
          <div class="ic-cols">
            <div v-if="side" class="ic-md ic-before"><MarkdownBody :html="s.html" :forges="thread.forges" /></div>
            <div class="ic-md ic-after"><MarkdownBody :html="s.html" :forges="thread.forges" /></div>
          </div>
          <div class="ic-gut">
            <span class="gb-anchor">§{{ s.n }} {{ s.heading || 'intro' }}</span>
            <span>{{ changed(s) }}</span>
          </div>
          <template v-if="withThreads && changed(s) !== 'unchanged'">
            <div v-for="t in threadsOn(s)" :key="t.event.id" class="ic-row">
              <div class="ic-thread">
                <span><b>{{ t.event.author.name }}</b> on <span class="gb-anchor">{{ label(t) }}</span> · "{{ firstWords(t.event.body) }}" · {{ plural(1 + t.replies.length, 'comment') }} · {{ resolvedBy(t, thread.events) || 'open' }}</span>
                <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :aria-expanded="shown.has(t.event.id)" @click="shown.has(t.event.id) ? shown.delete(t.event.id) : shown.add(t.event.id)">{{ shown.has(t.event.id) ? 'Hide' : 'Show' }}</button>
              </div>
              <ThreadCard
                v-if="shown.has(t.event.id)"
                :thread="t"
                :events="thread.events"
                :sections="current"
                :me-id="meId"
                :fields="fields"
                :forges="thread.forges"
                :busy="busy"
                :save="save"
                :reply="reply"
                :resolve="resolve"
                :apply="apply"
              />
            </div>
          </template>
        </section>
      </article>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { renderMarkdown, type Thread } from '@/api/board';
import type { Fields } from '@/fields';
import { bodyThreads, isRoot, resolvedBy, type Item } from '@/flow';
import { on, when } from '@/me';
import { anchorLabel, anchorOf, changeOf, criteriaOf, diffMarkdown, sectionsOf, ticksOf, versions, type Section } from '@/sections';
import { verdicts } from '@/spec';
import MarkdownBody from '@/components/MarkdownBody.vue';
import ThreadCard from '@/components/ThreadCard.vue';

const props = defineProps<{
  thread: Thread;
  // a thread's card, as under a section: its edit, reply, Resolve, Apply
  meId: string;
  fields: Fields;
  busy: boolean;
  save: (id: string, body: string, base: string) => Promise<unknown>;
  reply: (root: string, body: string) => Promise<boolean>;
  resolve: (root: string, value: string) => Promise<boolean>;
  apply: (root: string, body: string, base: string, version: number) => Promise<boolean>;
}>();

const created = computed(() => props.thread.events.find((e) => e.kind === 'created'));
const vs = computed(() => (created.value ? versions(created.value) : []));
const ticks = computed(() => (created.value ? ticksOf(created.value) : []));
const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;
const and = (xs: (string | number)[]) => (xs.length < 2 ? `${xs[0] ?? ''}` : `${xs.slice(0, -1).join(', ')} and ${xs.at(-1)}`);

// each version: what it changed against the one before, the threads it
// resolved (and the suggestions it applied), its last verdict
const cards = computed(() => {
  const roots = props.thread.events.filter(isRoot);
  const said = verdicts(props.thread.events);
  return vs.value.map((v, i) => {
    const n = criteriaOf(v.body).n;
    let what = `the original · ${n} criteri${n === 1 ? 'on' : 'a'}`;
    if (i) {
      const before = sectionsOf(vs.value[i - 1]!.html);
      const changed = sectionsOf(v.html).flatMap((s) => {
        const c = changeOf(before, s);
        return c === 'unchanged' ? [] : [c === 'changed' ? `§${s.n}` : `§${s.n} › ${c}`];
      });
      const resolved = roots.filter((r) => r.meta?.['thread:resolved'] === `v${v.n}`);
      const applied = resolved.filter((r) => r.meta?.['thread:applied']).length;
      what = [
        changed.length ? `${changed.join(' ')} changed` : 'no section changed',
        applied && `${plural(applied, 'suggested edit')} applied`,
        resolved.length && `resolves ${plural(resolved.length, 'thread')}`,
      ].filter(Boolean).join(' · ');
    }
    const verdict = said.filter((x) => x.v === v.n).at(-1);
    const by = verdict && `by ${verdict.event.author.name}, ${when(verdict.event.at)}`;
    return { n: v.n, at: v.at, author: v.author.name, what, verdict, said: !verdict ? 'no verdict' : verdict.verdict === 'approve' ? `approved ${by}` : `changes requested ${by}` };
  });
});
// the diff of two consecutive versions, the last pair first: the plain
// text until the server answers, the last request's answer only
const to = ref(0);
watch(() => vs.value.length, (n) => (to.value = n), { immediate: true });
const side = ref(false);
const withThreads = ref(true);
const diff = ref<string>();
let asked = 0;
watch(() => [to.value, vs.value] as const, async ([k, v]) => {
  const n = ++asked;
  diff.value = undefined;
  if (k < 2 || !v[k - 1]) return;
  const { html } = await renderMarkdown(diffMarkdown(v[k - 2]!.body, v[k - 1]!.body));
  if (n === asked) diff.value = html;
}, { immediate: true });
const diffSecs = computed(() => sectionsOf(diff.value ?? vs.value[to.value - 1]?.html ?? ''));
const fromSecs = computed(() => sectionsOf(vs.value[to.value - 2]?.html ?? ''));
const toSecs = computed(() => sectionsOf(vs.value[to.value - 1]?.html ?? ''));
function changed(s: Section): string {
  const now = toSecs.value.find((x) => x.slug === s.slug);
  const c = now ? changeOf(fromSecs.value, now) : 'changed';
  return typeof c === 'number' ? `› ${c} changed` : c;
}
// a changed section's threads written on the from-version (ruling 2)
const current = computed(() => sectionsOf(created.value?.html ?? ''));
const threads = computed(() => bodyThreads(props.thread.events));
const threadsOn = (s: Section): Item[] => threads.value.filter((t) => {
  const a = anchorOf(t.event.meta?.['thread:anchor']);
  return a?.slug === s.slug && a.v === to.value - 1;
});
const label = (t: Item) => anchorLabel(anchorOf(t.event.meta?.['thread:anchor'])!, current.value);
const firstWords = (body?: string) => {
  const w = (body ?? '').replace(/```suggestion[\s\S]*?```/g, '').trim().split(/\s+/);
  return w.slice(0, 8).join(' ') + (w.length > 8 ? '…' : '');
};
const shown = reactive(new Set<string>()); // the threads unfolded

// "criteria 1, 2 and 3 ticked", the unticks after
const tickWords = computed(() => {
  const net = new Map<number, boolean>();
  for (const t of ticks.value) {
    t.ticked.forEach((k) => net.set(k, true));
    t.unticked.forEach((k) => net.set(k, false));
  }
  const on = [...net].filter(([, v]) => v).map(([k]) => k).sort((a, b) => a - b);
  const off = [...net].filter(([, v]) => !v).map(([k]) => k).sort((a, b) => a - b);
  return [on.length && `${on.length === 1 ? 'criterion' : 'criteria'} ${and(on)} ticked`, off.length && `${and(off)} unticked`].filter(Boolean).join(' · ');
});
</script>

<style scoped>
.ic { display: grid; gap: .875rem; }
.ic-head { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: baseline; gap: .5rem 1rem; }
.ic-title { margin: 0; font-size: 1.25rem; }
.ic-muted { color: var(--fg-muted); font-weight: 400; }
.ic-small { font-size: .8125rem; }
.ic-ok { color: var(--success-fg); }
.ic-ko { color: var(--danger-fg); }
.ic-vers { display: flex; flex-wrap: wrap; align-items: stretch; gap: .625rem; }
.ic-arrow { align-self: center; font-size: 1.25rem; color: var(--fg-muted); }
.ic-ver { flex: 1 1 260px; min-width: 0; display: flex; flex-direction: column; gap: 4px; padding: .75rem .875rem; border: 1px solid var(--border); font-size: .875rem; }
.ic-ver.is-on { border-color: var(--fg); box-shadow: inset 0 3px 0 var(--fg); }
.ic-ver.is-ticks { border-style: dashed; }
.ic-bar { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: .5rem 1rem; }
.ic-bar .gb-seg { flex-wrap: wrap; }
.ic-check { display: flex; gap: 6px; align-items: center; }
.ic-diff { display: flex; flex-direction: column; padding: 1rem 1.25rem 1.25rem; border: 1px solid var(--border); }
/* the section grid of the description (IssueSections) */
.ic-sec { display: grid; grid-template-columns: minmax(0, 1fr) 164px; gap: 10px 20px; padding: 16px 0; border-top: 1px solid var(--border-subtle); }
.ic-sec:first-child { border-top: 0; }
.ic-sec.is-changed { padding: 16px 16px 16px 20px; background: var(--gb-teal-bg); box-shadow: inset 3px 0 0 var(--gb-done); }
.ic-row { grid-column: 1 / -1; display: grid; gap: 10px; }
.ic-cols { display: grid; gap: 20px; min-width: 0; }
.is-side .ic-cols { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.ic-gut { display: flex; flex-direction: column; align-items: flex-start; gap: 6px; font-size: .75rem; color: var(--fg-muted); }
.ic-thread { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 6px 12px; padding: 10px 16px; border: 1px solid var(--border-subtle); background: var(--bg); font-size: .8125rem; color: var(--fg-muted); }
.ic-thread b { color: var(--fg); }
.ic-md :deep(.gb-md > :first-child) { margin-top: 0; }
.ic-md :deep(ins) { background: var(--gb-green-bg); color: var(--gb-green-fg); text-decoration: none; padding: 1px 4px; }
.ic-md :deep(del) { background: var(--gb-red-bg); color: var(--gb-red-fg); padding: 1px 4px; }
/* side by side: one diff twice, the before without what was added, the after without what was removed */
.ic-before :deep(ins), .is-side .ic-after :deep(del) { display: none; }
.ic-before :deep(:is(li, p):has(> ins)), .is-side .ic-after :deep(:is(li, p):has(> del)) { display: none; }
@media (max-width: 719.98px) {
  .ic-sec, .is-side .ic-cols { grid-template-columns: minmax(0, 1fr); }
  .ic-gut { flex-direction: row; flex-wrap: wrap; align-items: center; }
}
</style>
