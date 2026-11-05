<!-- "How filtering works" (docs/prd/header-help.md, 5; the canvas's
     B1 + B2): what the panel's ticks do and what the Filter box's words
     mean, each word with Try, which writes it into the box. Opened by the
     filters panel's ?, the folded rail's, the one at the end of the Filter
     box, and the box's list. Never the app's shortcut list: that is ?. -->
<template>
  <!-- the panel floats on its own layer (q-menu, teleported): the filters
       panel scrolls, and would cut it -->
  <button ref="btn" type="button" :class="buttonClass ?? 'filters-help'" aria-label="How filtering works" title="How filtering works" aria-haspopup="dialog" :aria-expanded="open" @click="open = !open">?</button>
  <q-menu v-model="open" :target="btn ?? undefined" no-parent-event :anchor="align === 'right' ? 'bottom right' : 'bottom left'" :self="align === 'right' ? 'top right' : 'top left'" :offset="[0, 6]" class="fh-menu" @show="focusTab" @hide="tryPending">
    <div class="fh" role="dialog" aria-label="How filtering works">
        <p class="fh-title">How filtering works</p>
        <p class="fh-lead">Each tick in the filters panel writes words into the Filter box: both say the same thing.</p>
        <div class="fh-tabs" role="tablist" aria-label="How filtering works">
          <button v-for="t in tabs" :id="`fh-tab-${t}`" :key="t" type="button" role="tab" :aria-selected="tab === t" :aria-controls="`fh-${t}`" :tabindex="tab === t ? 0 : -1" @click="tab = t" @keydown.right.prevent="tab = other(t)" @keydown.left.prevent="tab = other(t)">{{ t }}</button>
        </div>
        <div v-if="tab === 'Words'" id="fh-Words" class="fh-words" role="tabpanel" aria-labelledby="fh-tab-Words">
          <ul v-for="(col, i) in columns" :key="i">
            <li v-for="w in col" :key="w.term">
              <code>{{ w.term }}</code>
              <span>{{ w.means }}</span>
              <button type="button" class="gb-linkbtn" :aria-label="`Try ${w.example}`" @click="tryWord(w.example)">Try</button>
            </li>
          </ul>
        </div>
        <ul v-else id="fh-Ticks" class="fh-ticks" role="tabpanel" aria-labelledby="fh-tab-Ticks">
          <li v-for="t in TICKS" :key="t.means">
            <span class="fh-keys"><kbd v-for="k in t.keys" :key="k">{{ keyLabel(k) }}</kbd></span>
            <span>{{ t.means }}</span>
          </li>
        </ul>
        <p class="fh-foot">
          Words are AND’ed: <code>status:ready assignee:@me</code> is both. <kbd>Tab</kbd> completes a value.
          <button type="button" class="gb-linkbtn" @click="close(), (helpOpen = true)">Every shortcut <kbd>?</kbd></button>
        </p>
    </div>
  </q-menu>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { GRAMMAR, TICKS } from '@/filterhelp';
import { keyLabel } from '@/keys';
import { helpOpen } from '@/shortcuts';

defineProps<{ buttonClass?: string; align?: 'left' | 'right' }>();
const emit = defineEmits<{ try: [term: string] }>();

const tabs = ['Words', 'Ticks'] as const;
type Tab = (typeof tabs)[number];
const tab = ref<Tab>('Words');
const other = (t: Tab): Tab => (t === 'Words' ? 'Ticks' : 'Words');
const half = Math.ceil(GRAMMAR.length / 2);
const columns = [GRAMMAR.slice(0, half), GRAMMAR.slice(half)];

const open = ref(false);
const btn = ref<HTMLButtonElement | null>(null);
const close = () => (open.value = false);
// the focus goes to the selected tab, so the arrows and Tab go on from there
function focusTab() {
  document.querySelector<HTMLElement>('.fh-menu [role=tab][aria-selected=true]')?.focus();
}
// Try closes the menu first and writes the word once it has hidden: the
// menu gives the focus back to its ? as it hides, and the page focuses the
// Filter box on try, so that focus must come last (issue #91)
let pending: string | null = null;
function tryWord(term: string) {
  pending = term;
  close();
}
function tryPending() {
  if (pending === null) return;
  const term = pending;
  pending = null;
  emit('try', term);
}
defineExpose({ show: () => (open.value = true) });
</script>

<style scoped>
.fh { width: min(46rem, calc(100vw - 2rem)); box-sizing: border-box; padding: .875rem 1rem 1rem; font-size: .84375rem; }
.fh-title { margin: 0 0 .25rem; font-size: 1rem; font-weight: var(--w-bold); }
.fh-lead { margin: 0 0 .75rem; color: var(--fg-muted); }
.fh-tabs { display: flex; gap: 1.25rem; border-bottom: var(--border-w) solid var(--border); }
.fh-tabs > button { padding: .5rem 0 .4375rem; border: 0; border-bottom: .125rem solid transparent; background: none; color: var(--fg); font: inherit; font-size: .875rem; cursor: pointer; }
.fh-tabs > button[aria-selected='true'] { border-bottom-color: var(--primary); font-weight: var(--w-bold); }
.fh-tabs > button:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: .125rem; }
.fh-words { display: grid; grid-template-columns: 1fr 1fr; gap: 0 1.75rem; }
.fh-words ul, .fh-ticks { margin: 0; padding: .25rem 0 0; list-style: none; }
.fh-words li { display: grid; grid-template-columns: 9.5rem 1fr auto; align-items: center; gap: .625rem; padding: .375rem 0; border-bottom: var(--border-w) solid var(--border-subtle); }
.fh-ticks li { display: grid; grid-template-columns: 7.5rem 1fr; align-items: center; gap: .625rem; padding: .375rem 0; border-bottom: var(--border-w) solid var(--border-subtle); }
.fh code { justify-self: start; padding: .0625rem .3125rem; border: var(--border-w) solid var(--border-subtle); background: var(--bg-subtle); font-size: .78125rem; white-space: nowrap; }
.fh-words li > span, .fh-ticks li > span:last-child { color: var(--fg-muted); }
.fh-keys { display: inline-flex; gap: .1875rem; }
.fh kbd { min-width: .75rem; padding: 0 .3125rem; border: var(--border-w) solid var(--border-strong); border-bottom-width: .125rem; border-radius: var(--radius); background: var(--bg); color: var(--fg); font-family: var(--font-sans); font-size: .75rem; line-height: 1.35; text-align: center; }
.fh-foot { margin: .75rem 0 0; color: var(--fg-muted); }
@media (max-width: 47.99rem) { .fh-words { grid-template-columns: 1fr; } }
</style>
