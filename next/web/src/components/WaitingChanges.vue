<!-- What the next forge sync will take to each forge: one row per waiting
     change, newest first, with the change itself (a title old → new,
     labels ±, an edited text as a line diff, a new comment's first lines).
     A row's title shows it in the history. -->
<template>
  <ol class="gb-waiting" aria-label="Waiting changes">
    <li v-for="c in changes" :key="c.event.id + c.what" class="gb-waiting-row" @click="emit('reveal', c.event.id)">
      <div class="gb-waiting-head">
        <button type="button" class="gb-waiting-what" @click.stop="emit('reveal', c.event.id)">{{ c.what }}</button>
        <span class="gb-hint">{{ c.author.name }} · <time :datetime="c.at">{{ when(c.at, true) }}</time></span>
      </div>
      <div class="gb-waiting-forges">
        <span v-for="f in forgeNames" :key="f" class="gb-fstate" :class="{ 'gb-fstate-local': c.waiting.includes(f) }">
          <GbIcon :name="c.waiting.includes(f) ? 'clock' : 'check'" small /> {{ f }} {{ c.waiting.includes(f) ? 'waiting' : 'has it' }}
        </span>
      </div>
      <p v-if="c.event.kind === 'title'" class="gb-waiting-detail"><span class="gb-strike">{{ c.event.was }}</span> → <b>{{ c.event.title }}</b></p>
      <p v-else-if="c.event.kind === 'labels'" class="gb-waiting-detail">
        <span v-for="l in c.event.added" :key="`+${l}`" class="gb-diff-chip gb-diff-add">+ {{ l }}</span>
        <span v-for="l in c.event.removed" :key="`-${l}`" class="gb-diff-chip gb-diff-del">− {{ l }}</span>
      </p>
      <pre v-else-if="c.event.edit_pending?.length && !c.event.pending?.length" class="gb-diff" :aria-label="`Changes to the ${c.event.kind === 'created' ? 'description' : 'comment'}`"><span
        v-for="(d, i) in diffOf(c.event.edit_was ?? '', c.event.body ?? '')" :key="i" :class="{ 'gb-diff-add': d.op === '+', 'gb-diff-del': d.op === '-', 'gb-diff-gap': d.op === '…' }">{{ d.op === '…' ? d.text : `${d.op} ${d.text}` }}
</span></pre>
      <p v-else-if="c.event.body" class="gb-waiting-detail gb-waiting-body">{{ firstLines(c.event.body) }}</p>
    </li>
  </ol>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Forge } from '@/api/board';
import type { WaitingChange } from '@/history';
import { lineDiff } from '@/linediff';
import { when } from '@/me';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ changes: WaitingChange[]; forges: Forge[] }>();
const emit = defineEmits<{ reveal: [id: string] }>();

const forgeNames = computed(() => [...new Set(props.forges.map((f) => f.name))]);

// The changed lines with one line around them; the rest folds into "…".
function diffOf(was: string, now: string) {
  const d = lineDiff(was, now);
  const keep = d.map((l, i) => l.op !== ' ' || d[i - 1]?.op === '-' || d[i - 1]?.op === '+' || d[i + 1]?.op === '-' || d[i + 1]?.op === '+');
  const out: { op: string; text: string }[] = [];
  d.forEach((l, i) => {
    if (keep[i]) out.push(l);
    else if (out[out.length - 1]?.op !== '…') out.push({ op: '…', text: '…' });
  });
  return out.length > 16 ? [...out.slice(0, 15), { op: '…', text: `… ${out.length - 15} more lines` }] : out;
}

function firstLines(body: string) {
  const lines = body.split('\n').filter((l) => l.trim()).slice(0, 3).join('\n');
  return lines.length > 240 ? `${lines.slice(0, 240)}…` : lines;
}
</script>

<style scoped>
.gb-waiting { list-style: none; margin: .5rem 0 0; padding: 0; display: grid; gap: .5rem; }
.gb-waiting-row { display: grid; gap: .375rem; font-size: .875rem; padding: .625rem .75rem; border: 1px solid var(--border); border-radius: var(--radius); cursor: pointer; }
.gb-waiting-row:hover { border-color: var(--fg); }
.gb-waiting-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: .25rem .5rem; }
.gb-waiting-what { padding: 0; border: 0; background: none; font: inherit; font-size: .9375rem; line-height: 1.4; font-weight: 700; color: var(--fg); text-decoration: underline; text-underline-offset: 2px; cursor: pointer; }
.gb-waiting-forges { display: flex; flex-wrap: wrap; gap: .25rem .75rem; font-size: .8125rem; }
.gb-waiting-detail { margin: 0; font-size: .875rem; overflow-wrap: anywhere; }
.gb-waiting-body { white-space: pre-line; color: var(--fg-muted); }
.gb-diff { margin: 0; padding: .375rem 0; max-height: 16rem; overflow: auto; font-family: var(--font-mono); font-size: .75rem; line-height: 1.5; background: var(--bg-subtle); border-radius: var(--radius); white-space: pre-wrap; overflow-wrap: anywhere; }
.gb-diff span { display: block; padding: 0 .5rem; }
.gb-diff-add { background: var(--gb-green-bg); color: var(--gb-green-fg); }
.gb-diff-del { background: var(--gb-red-bg); color: var(--gb-red-fg); text-decoration: line-through; text-decoration-color: color-mix(in srgb, currentColor 40%, transparent); }
.gb-diff-gap { color: var(--fg-muted); }
.gb-diff-chip { display: inline-block; margin: 0 .25rem .25rem 0; padding: 0 .375rem; border-radius: var(--radius); font-weight: 700; font-size: .8125rem; }
</style>
