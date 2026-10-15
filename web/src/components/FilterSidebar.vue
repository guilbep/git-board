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
        <button type="button" class="filters-reset" :disabled="!modified" @click="emit('reset')">Reset</button>
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
        <div class="filters-options">
          <label v-for="[value, count] in f.options" :key="value" class="filters-option">
            <input
              type="checkbox"
              :checked="!!sel[f.key]?.includes(value)"
              @change="emit('toggle', f.key, value)"
            />
            <span class="filters-option-label">{{ valueLabel(f.key, value, fields) }}</span>
            <span class="filters-count">{{ count }}</span>
          </label>
        </div>
      </details>
    </details>
  </aside>
</template>

<script setup lang="ts">
import { facetLabel, valueLabel, type Facet, type Selection } from '@/facets';
import type { Fields } from '@/fields';

import { computed } from 'vue';

const props = defineProps<{ facets: Facet[]; sel: Selection; fields: Fields; modified: boolean; queryChanged: boolean }>();
const query = defineModel<string>('query', { required: true });
const emit = defineEmits<{ toggle: [key: string, value: string]; reset: []; apply: [] }>();

// Open state is set once: re-deriving it from the selection would fold a
// group under the pointer as its last box is unticked.
const wide = window.matchMedia('(min-width: 64rem)').matches;
const openAtStart = new Set([...Object.keys(props.sel), ...(props.queryChanged ? ['query'] : [])]);
const active = computed(() => Object.values(props.sel).flat().length + Number(props.queryChanged));
</script>

<style scoped>
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
  color: var(--epfl-white);
  background: var(--primary);
  border-radius: var(--radius);
}

.filters-options { display: flex; flex-direction: column; gap: var(--space-2); padding: 0.375rem 0 0 1.25rem; }
.filters-option { display: flex; align-items: center; gap: var(--space-2); font-size: var(--fs-sm); cursor: pointer; }
.filters-option input { width: 1rem; height: 1rem; margin: 0; accent-color: var(--primary); }
.filters-option-label { flex: 1; overflow-wrap: anywhere; }
.filters-count { color: var(--fg-muted); font-variant-numeric: tabular-nums; }

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
