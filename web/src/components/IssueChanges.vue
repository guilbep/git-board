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
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Thread } from '@/api/board';
import { isRoot } from '@/flow';
import { on, when } from '@/me';
import { changeOf, criteriaOf, sectionsOf, ticksOf, versions } from '@/sections';
import { verdicts } from '@/spec';

const props = defineProps<{ thread: Thread }>();

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
</style>
