<!-- The board's Filter box (docs/prd/header-help.md, 5): a combobox,
     examples from this board when empty, the completions of its last word
     with their counts as you type, the filters' ? at its end. -->
<template>
  <!-- a combobox (docs/prd/header-help.md, 5): examples from this
       board when empty, completions with their counts as you type,
       the filters' ? at its end -->
  <div class="fbox" @focusout="onBoxLeave">
    <input
      id="filter"
      ref="filterInput"
      class="epfl-input"
      type="search"
      role="combobox"
      aria-autocomplete="list"
      :aria-expanded="boxOpen && boxOptions.length > 0"
      aria-controls="filter-list"
      :aria-activedescendant="boxActive >= 0 ? `filter-opt-${boxActive}` : undefined"
      title="Words in titles, descriptions, comments; #number; field:value"
      spellcheck="false"
      placeholder="Words, #12, status:ready, assignee:@me, is:closed…"
      autocomplete="off"
      :value="modelValue"
      @focus="boxOpen = true"
      @click="boxOpen = true"
      @input="onBoxInput"
      @keydown="onBoxKey"
    />
    <FilterHelp ref="boxHelp" button-class="filters-help fbox-help" align="right" @try="tryWord" />
    <div v-if="boxOpen && boxOptions.length" class="fbox-pop" @mousedown.prevent>
      <p v-if="!completing" class="fbox-head">{{ modelValue.trim() ? 'Add' : 'Try' }}</p>
      <ul id="filter-list" role="listbox" aria-label="Filter suggestions">
        <li
          v-for="(o, i) in boxOptions"
          :id="`filter-opt-${i}`"
          :key="o.value"
          role="option"
          :aria-selected="i === boxActive"
          :class="{ on: i === boxActive }"
          @mousemove="boxActive = i"
          @click="takeOption(i)"
        >
          <code>{{ o.label }}</code>
          <span>{{ o.note }}</span>
        </li>
      </ul>
      <p class="fbox-foot">
        <span><kbd>↑</kbd><kbd>↓</kbd> choose</span>
        <span v-if="completing"><kbd>Tab</kbd> complete</span>
        <span v-if="trick" class="fbox-trick">{{ trick }}</span>
        <button type="button" class="gb-linkbtn fbox-how" @click="boxOpen = false, boxHelp?.show()">How filtering works</button>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import type { Facet } from '@/facets';
import type { Fields } from '@/fields';
import { examples, nextTrick } from '@/filterhelp';
import { me } from '@/me';
import { suggest } from '@/viewfilter';
import FilterHelp from '@/components/FilterHelp.vue';

const props = defineProps<{ modelValue: string; fields: Fields; values: Record<string, string[]>; facets: Facet[] }>();
const emit = defineEmits<{ 'update:modelValue': [filter: string] }>();

// status: → status:ready, … (the values in use)
const suggestions = computed(() => suggest(props.modelValue, props.values));

// the Filter box's list: examples from this board when it's empty, else
// the completions of its last word, each with its count from the facets
const filterInput = ref<HTMLInputElement | null>(null);
const boxHelp = ref<InstanceType<typeof FilterHelp> | null>(null);
const boxOpen = ref(false);
const boxActive = ref(-1);
// completing: the last word's values; else examples from this board, the
// ones the box doesn't hold yet, added after what it holds
const completing = computed(() => suggestions.value.length > 0);
const boxOptions = computed<{ value: string; label: string; note: string }[]>(() => {
  if (!completing.value) {
    const f = props.modelValue.trim();
    const held = f.split(/\s+/);
    return examples(props.fields, me.value?.me?.login ?? '')
      .filter((e) => !held.includes(e.term))
      .map((e) => ({ value: f ? `${f} ${e.term}` : e.term, label: e.term, note: e.means }));
  }
  return suggestions.value.slice(0, 8).map((full) => {
    const word = full.split(/\s+/).at(-1) ?? full;
    const [key, v] = [word.slice(0, word.indexOf(':')), word.slice(word.indexOf(':') + 1).split(',').at(-1) ?? ''];
    const n = props.facets.find((f) => f.key === key)?.options.find(([o]) => o === v)?.[1];
    return { value: full, label: word, note: n === undefined ? '' : `${n} ${n === 1 ? 'issue' : 'issues'}` };
  });
});
const trick = computed(() => (completing.value ? nextTrick(props.modelValue) : null));
watch(boxOptions, () => (boxActive.value = -1));
function onBoxInput(e: Event) {
  boxOpen.value = true;
  emit('update:modelValue', (e.target as HTMLInputElement).value);
}
function takeOption(i: number) {
  const o = boxOptions.value[i];
  if (!o) return;
  emit('update:modelValue', o.value);
  boxActive.value = -1;
  filterInput.value?.focus();
}
// ↑↓ choose; Tab and Enter take the chosen one (Tab the first completion
// when none is chosen); Esc closes the list first; else the keys do what
// they did
function onBoxKey(e: KeyboardEvent) {
  const n = boxOptions.value.length;
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    if (!n) return;
    e.preventDefault();
    boxOpen.value = true;
    boxActive.value = e.key === 'ArrowDown' ? (boxActive.value + 1) % n : (boxActive.value - 1 + n) % n;
    return;
  }
  if (!boxOpen.value || !n) return;
  if (e.key === 'Enter' && boxActive.value >= 0) {
    e.preventDefault();
    takeOption(boxActive.value);
  } else if (e.key === 'Tab' && !e.shiftKey && (boxActive.value >= 0 || completing.value)) {
    e.preventDefault();
    takeOption(Math.max(boxActive.value, 0));
  } else if (e.key === 'Escape') {
    e.preventDefault();
    e.stopPropagation();
    boxOpen.value = false;
  }
}
function onBoxLeave(e: FocusEvent) {
  const box = e.currentTarget as HTMLElement;
  if (!(e.relatedTarget instanceof Node && box.contains(e.relatedTarget))) boxOpen.value = false;
}

// Try, in "How filtering works": the word joins what the box holds
function tryWord(term: string) {
  const f = props.modelValue.trim();
  emit('update:modelValue', f ? `${f} ${term}` : term);
  boxOpen.value = false;
  void nextTick(() => filterInput.value?.focus());
}

// the box, focused: after a Try of the panel's How filtering works
defineExpose({ focus: () => filterInput.value?.focus() });
</script>

<style scoped>
.fbox { position: relative; }
.fbox > .epfl-input { width: 100%; padding-right: 2.5rem; }
.fbox > :deep(.fbox-help) { position: absolute; top: 50%; right: .625rem; transform: translateY(-50%); }
.fbox-pop { position: absolute; z-index: 25; top: calc(100% + .25rem); left: 0; width: max(100%, 30rem); border: var(--border-w) solid var(--border-strong); background: var(--bg); box-shadow: var(--shadow); font-size: .875rem; }
.fbox-head { margin: 0; padding: .625rem 1rem .25rem; color: var(--fg-muted); font-size: .75rem; font-weight: var(--w-bold); letter-spacing: .06em; text-transform: uppercase; }
.fbox-pop ul { margin: 0; padding: .25rem 0; list-style: none; }
.fbox-pop li { display: flex; align-items: center; gap: .75rem; padding: .4375rem 1rem; cursor: pointer; }
.fbox-pop li.on { background: var(--bg-muted); box-shadow: inset 3px 0 0 var(--primary); }
.fbox-pop li > span { margin-left: auto; color: var(--fg-muted); font-size: .8125rem; text-align: right; }
.fbox-pop code { padding: .0625rem .3125rem; border: var(--border-w) solid var(--border-subtle); background: var(--bg-subtle); font-size: .8125rem; }
.fbox-foot { display: flex; flex-wrap: wrap; align-items: center; gap: .375rem 1rem; margin: 0; padding: .5rem 1rem; border-top: var(--border-w) solid var(--border-subtle); color: var(--fg-muted); font-size: .78125rem; }
.fbox-foot > span { display: inline-flex; align-items: center; gap: .1875rem; }
.fbox-how { margin-left: auto; }
.fbox-pop kbd { min-width: .75rem; padding: 0 .3125rem; border: var(--border-w) solid var(--border-strong); border-bottom-width: .125rem; border-radius: var(--radius); background: var(--bg); color: var(--fg); font-family: var(--font-sans); font-size: .71875rem; line-height: 1.35; text-align: center; }
</style>
