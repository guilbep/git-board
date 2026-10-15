<!-- The filter sidebar: the view's filter expression (sent to the server),
     then one collapsible checkbox list per facet, counted against the other
     facets (see facets.ts). Design from the EPFL masonry-walls database. -->
<template>
  <aside class="filters" aria-label="Filters">
    <!-- Folded on narrow screens, where the sidebar sits above the issues;
         always open beside them (the toggle is hidden there). -->
    <details class="filters-fold" :open="wide">
      <summary class="filters-toggle">
        <span class="filters-chev" aria-hidden="true" />
        Filters<span v-if="active" class="filters-badge">{{ active }}</span>
      </summary>
      <div class="filters-head">
        <h2 class="filters-title">Filters</h2>
        <span class="filters-head-r">
          <button type="button" class="filters-help" aria-label="Keyboard shortcuts for the filters" title="Keyboard shortcuts (?)" @click="helpOpen = true">?</button>
          <button type="button" class="filters-reset" :disabled="!modified" @click="emit('reset')">Reset</button>
          <button type="button" class="filters-hide" aria-label="Hide filters" title="Hide filters" @click="emit('hide')"><GbIcon name="collapse" small /></button>
        </span>
      </div>

      <details class="filters-item" :open="openAtStart.has('query')">
        <summary>
          <span class="filters-chev" aria-hidden="true" />
          <span class="filters-item-label">Filter expression</span>
          <span v-if="queryChanged" class="filters-badge">1</span>
        </summary>
        <form class="filters-query" @submit.prevent="emit('apply')">
          <input
            v-model="query"
            class="epfl-input"
            type="text"
            spellcheck="false"
            autocomplete="off"
            aria-label="Filter expression"
            aria-describedby="query-hint"
          />
          <small id="query-hint">For example: is:open status:ready prio:p1 -title:*spec*</small>
          <button type="submit" class="epfl-btn epfl-btn-secondary epfl-btn-sm">Apply</button>
        </form>
      </details>

      <details v-for="f in facets" :key="f.key" class="filters-item" :open="openAtStart.has(f.key)">
        <summary>
          <span class="filters-chev" aria-hidden="true" />
          <span class="filters-item-label">{{ facetLabel(f.key) }}</span>
          <span v-if="sel[f.key]?.length" class="filters-badge">{{ sel[f.key]!.length }}</span>
        </summary>
        <div class="filters-tools">
          <span class="filters-sort" role="group" :aria-label="`Sort ${facetLabel(f.key)}`">
            <button
              v-for="m in sortModes(f.key)"
              :key="m"
              type="button"
              :aria-pressed="sortOf(f.key) === m"
              @click="setSort(f.key, m)"
            >{{ modeLabel(f.key, m) }}</button>
          </span>
          <button v-if="sel[f.key]?.length" type="button" class="filters-clear" @click="emit('set', f.key, [])">Clear</button>
        </div>
        <input
          v-if="f.options.length > 8"
          v-model="find[f.key]"
          class="epfl-input filters-find"
          type="search"
          :placeholder="`Find a ${facetLabel(f.key).toLowerCase()}…`"
          :aria-label="`Find in ${facetLabel(f.key)}`"
        />
        <div class="filters-options">
          <label
            v-for="[value, count] in shown(f)"
            :key="value"
            class="filters-option"
            :class="{ on: sel[f.key]?.includes(value) }"
            @click.prevent="onPick(f, value, $event)"
          >
            <span class="filters-bar" :style="{ width: `${(count / maxCount(f)) * 100}%` }" aria-hidden="true" />
            <input type="checkbox" :checked="!!sel[f.key]?.includes(value)" />
            <span class="filters-option-label"><ValueMark :field="f.key" :value="value" /> {{ valueLabel(f.key, value, fields) }}<span v-if="value === fields[f.key]?.current" class="filters-current">current</span></span>
            <button
              type="button"
              class="filters-only"
              :title="onlyThis(f.key, value) ? 'Every value but this one (Alt-click)' : 'Only this value (Alt-click)'"
              @click.stop.prevent="emit('set', f.key, pick(sel[f.key] ?? [], value, order(f), { alt: true }))"
            >{{ onlyThis(f.key, value) ? 'all but' : 'only' }}</button>
            <span class="filters-count">{{ count }}</span>
          </label>
          <p v-if="!shown(f).length" class="filters-none">No value matches.</p>
        </div>
      </details>
    </details>
  </aside>
</template>

<script setup lang="ts">
import ValueMark from '@/components/ValueMark.vue';
import GbIcon from '@/components/GbIcon.vue';
import { helpOpen } from '@/shortcuts';
import { colorClass } from '@/colors';
import { facetLabel, pick, sortOptions, valueLabel, type Facet, type OptionSort, type Selection } from '@/facets';
import type { Fields } from '@/fields';

import { computed, reactive } from 'vue';

const props = defineProps<{ facets: Facet[]; sel: Selection; fields: Fields; modified: boolean; queryChanged: boolean }>();
const query = defineModel<string>('query', { required: true });
const emit = defineEmits<{ set: [key: string, values: string[]]; reset: []; apply: []; hide: [] }>();

// Each section's order: workflow when the field has configured options,
// else by count; a choice is kept in this browser.
const SORT_KEY = 'gb-facet-sort';
const sorts = reactive<Record<string, OptionSort>>(readSorts());
function readSorts(): Record<string, OptionSort> {
  try {
    return JSON.parse(localStorage.getItem(SORT_KEY) ?? '{}') as Record<string, OptionSort>;
  } catch {
    return {};
  }
}
const configured = (key: string) => props.fields[key]?.options ?? [];
// a date field reads in date order: A–Z of ISO dates, named Dates
const isDate = (key: string) => props.fields[key]?.type === 'date';
const sortModes = (key: string): OptionSort[] =>
  configured(key).length ? ['workflow', 'count', 'alpha'] : isDate(key) ? ['alpha', 'count'] : ['count', 'alpha'];
function modeLabel(key: string, m: OptionSort): string {
  if (m === 'count') return 'Count';
  if (m === 'alpha') return isDate(key) ? 'Dates' : 'A–Z';
  return props.fields[key]?.type === 'iteration' ? 'Dates' : 'Workflow';
}
const sortOf = (key: string): OptionSort => (sortModes(key).includes(sorts[key]!) ? sorts[key]! : sortModes(key)[0]!);
function setSort(key: string, mode: OptionSort) {
  sorts[key] = mode;
  try {
    localStorage.setItem(SORT_KEY, JSON.stringify(sorts));
  } catch {
    // private window: the order lasts until reload
  }
}

const find = reactive<Record<string, string>>({});
const order = (f: Facet) => sortOptions(f.options, sortOf(f.key), configured(f.key)).map(([v]) => v);
function shown(f: Facet): [string, number][] {
  const sorted = sortOptions(f.options, sortOf(f.key), configured(f.key));
  const needle = (find[f.key] ?? '').trim().toLowerCase();
  return needle ? sorted.filter(([v]) => `${v} ${valueLabel(f.key, v, props.fields)}`.toLowerCase().includes(needle)) : sorted;
}
const maxCount = (f: Facet) => Math.max(1, ...f.options.map(([, n]) => n));
const onlyThis = (key: string, value: string) => props.sel[key]?.length === 1 && props.sel[key]![0] === value;

// Click toggles, Alt keeps only one (then all but it), Shift a range from
// the last value clicked in the section (facets.ts pick).
const anchors: Record<string, string> = {};
function onPick(f: Facet, value: string, e: MouseEvent) {
  const next = pick(props.sel[f.key] ?? [], value, order(f), { alt: e.altKey, shift: e.shiftKey, anchor: anchors[f.key] });
  if (!e.shiftKey) anchors[f.key] = value;
  emit('set', f.key, next);
}

// Open state is set once: re-deriving it from the selection would fold a
// group under the pointer as its last box is unticked.
const wide = window.matchMedia('(min-width: 64rem)').matches;
const openAtStart = new Set([...Object.keys(props.sel), ...(props.queryChanged ? ['query'] : [])]);
const active = computed(() => Object.values(props.sel).flat().length + Number(props.queryChanged));
</script>

<style scoped>
.filters-current { margin-left: .375rem; padding: 0 .3rem; border: 1px solid var(--fg); font-size: .6875rem; font-weight: 700; text-transform: uppercase; letter-spacing: .04em; vertical-align: 1px; }
.filters {
  padding: var(--space-4) var(--space-4) var(--space-5) 0;
  border-right: var(--border-w) solid var(--border-subtle);
}
.filters-toggle { display: none; }

.filters-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-2);
}
.filters-title { margin: 0; font-size: var(--fs-h6); font-weight: var(--w-bold); line-height: var(--lh-normal); }

.filters-reset {
  padding: 0.375rem 0.625rem;
  font: inherit;
  font-size: 0.8125rem;
  color: var(--danger-fg);
  background: var(--bg);
  border: var(--border-w) solid var(--danger-border);
  border-radius: var(--radius);
  cursor: pointer;
}
.filters-reset:disabled { cursor: default; opacity: 0.5; }
.filters-reset:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }

.filters-item { padding: 0.375rem 0 0.625rem; border-bottom: var(--border-w) solid var(--border-subtle); }
.filters-item summary {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 0.375rem 0;
  font-size: 0.9375rem;
  list-style: none;
  cursor: pointer;
}
.filters-item summary::-webkit-details-marker { display: none; }
.filters-item summary:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }

/* A chevron that turns down when the group is open. */
.filters-chev {
  width: 0.4rem;
  height: 0.4rem;
  margin: 0 0.2rem;
  border-right: 0.125rem solid currentColor;
  border-bottom: 0.125rem solid currentColor;
  transform: rotate(-45deg);
  transition: transform var(--t-fast);
}
.filters-item[open] .filters-chev { transform: rotate(45deg); }

.filters-item-label { flex: 1; }
.filters-badge {
  padding: 0 0.375rem;
  font-size: 0.75rem;
  font-weight: var(--w-bold);
  color: var(--on-primary);
  background: var(--primary);
  border-radius: var(--radius);
}

.filters-head-r { display: inline-flex; align-items: center; gap: var(--space-2); }
.filters-hide { display: inline-flex; align-items: center; justify-content: center; width: 2rem; height: 2rem; padding: 0; border: 1px solid var(--border); background: none; color: var(--fg); cursor: pointer; }
.filters-hide:hover, .filters-hide:focus-visible { border-color: var(--fg); }
@media (max-width: 63.99rem) { .filters-hide { display: none; } }
.filters-help { display: inline-flex; align-items: center; justify-content: center; width: 1.5rem; height: 1.5rem; border: var(--border-w) solid var(--border); border-radius: 50%; font-size: 0.75rem; font-weight: var(--w-bold); color: var(--fg-muted); cursor: pointer;  padding: 0; background: none; }
.filters-help:hover, .filters-help:focus-visible { border-color: var(--fg); color: var(--fg); }

.filters-tools { display: flex; align-items: center; justify-content: space-between; gap: var(--space-2); padding: 0.25rem 0 0 1.25rem; }
.filters-sort { display: inline-flex; border: var(--border-w) solid var(--border); border-radius: var(--radius); overflow: hidden; }
.filters-sort button { padding: 0.0625rem 0.4375rem; font: inherit; font-size: 0.75rem; color: var(--fg-muted); background: var(--bg); border: 0; cursor: pointer; }
.filters-sort button + button { border-left: var(--border-w) solid var(--border); }
.filters-sort button[aria-pressed='true'] { color: var(--fg); background: var(--bg-subtle); font-weight: var(--w-bold); }
.filters-clear { padding: 0; font: inherit; font-size: 0.75rem; color: var(--fg); background: none; border: 0; text-decoration: underline; cursor: pointer; }
.filters-find { margin: 0.5rem 0 0 1.25rem; width: calc(100% - 1.25rem); padding: 0.25rem 0.5rem; font-size: var(--fs-sm); }

.filters-options { display: flex; flex-direction: column; gap: 0.125rem; padding: 0.375rem 0 0 1.25rem; }
/* the bar behind a value is its count against the section's largest
   (from resslab-hub's filter rail) */
.filters-option { position: relative; display: flex; align-items: center; gap: var(--space-2); padding: 0.1875rem 0.375rem; font-size: var(--fs-sm); cursor: pointer; user-select: none; border-radius: var(--radius); }
.filters-option > * { position: relative; }
.filters-option > .filters-bar { position: absolute; inset: 0 auto 0 0; background: var(--bg-subtle); border-radius: var(--radius); pointer-events: none; }
.filters-option.on > .filters-bar { background: color-mix(in srgb, var(--primary) 12%, transparent); }
.filters-option input { width: 1rem; height: 1rem; margin: 0; flex: none; accent-color: var(--primary); }
.filters-option-label { flex: 1; overflow-wrap: anywhere; }
.filters-only { visibility: hidden; padding: 0 0.25rem; font: inherit; font-size: 0.6875rem; color: var(--fg); background: var(--bg); border: var(--border-w) solid var(--border); border-radius: var(--radius); cursor: pointer; }
.filters-option:hover .filters-only, .filters-option:focus-within .filters-only { visibility: visible; }
.filters-count { min-width: 1.75rem; text-align: right; color: var(--fg-muted); font-variant-numeric: tabular-nums; }
.filters-none { margin: 0.25rem 0 0; font-size: var(--fs-sm); color: var(--fg-muted); }

.filters-query { display: grid; gap: var(--space-2); padding: 0.375rem 0 0 1.25rem; }
.filters-query .epfl-input { font-family: var(--font-mono); font-size: var(--fs-sm); }
.filters-query .epfl-btn { justify-self: start; }

@media (max-width: 63.99rem) {
  .filters { padding: var(--space-3) 0 0; border-right: 0; }
  .filters-toggle {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) 0;
    font-weight: var(--w-bold);
    list-style: none;
    cursor: pointer;
    border-bottom: var(--border-w) solid var(--border-subtle);
  }
  .filters-toggle::-webkit-details-marker { display: none; }
  .filters-fold[open] > .filters-toggle .filters-chev { transform: rotate(45deg); }
  .filters-fold[open] > .filters-head { justify-content: flex-end; margin-top: var(--space-3); }
  .filters-title { display: none; } /* the toggle says it */
}
</style>
