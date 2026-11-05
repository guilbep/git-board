<!-- Parts of a whole: each value's share of the issues, in its color, the
     total in the middle, a legend with counts and shares. -->
<template>
  <div class="dn">
    <svg viewBox="-60 -60 120 120" role="img" :aria-label="label" class="dn-svg">
      <path v-for="s in slices" :key="s.key" :d="s.d" class="dn-slice" :class="cls(s.key)" fill-rule="evenodd"><title>{{ s.key }}: {{ s.n }} ({{ s.pct }}%)</title></path>
      <text class="dn-total" y="4" text-anchor="middle">{{ total }}</text>
      <text class="dn-caption" y="17" text-anchor="middle">{{ caption }}</text>
    </svg>
    <ul class="dn-legend">
      <li v-for="s in slices" :key="s.key">
        <i class="dn-key" :class="cls(s.key)" aria-hidden="true" />
        <span class="dn-name">{{ s.key === '(none)' ? 'Not set' : s.key }}</span>
        <span class="dn-n">{{ s.n }}</span>
        <span class="dn-pct">{{ s.pct }}%</span>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { colorClass } from '@/colors';
import { donutSlices } from '@/donut';

const props = defineProps<{ entries: { key: string; n: number }[]; field: string; label: string; caption: string }>();
const slices = computed(() => donutSlices(props.entries, 56, 38));
const total = computed(() => props.entries.reduce((t, e) => t + e.n, 0));
// no value: the flow chart's light neutral
const cls = (v: string) => (v === '(none)' ? 'dn-none' : colorClass(props.field, v) || 'gb-c-grey');
</script>

<style scoped>
.dn { display: flex; flex-wrap: wrap; align-items: center; gap: 1.25rem; }
.dn-svg { width: 11rem; height: 11rem; flex: none; }
.dn-slice { fill: var(--c-dot, var(--fg-subtle)); stroke: var(--bg); stroke-width: 1; }
.dn-none { --c-dot: var(--border); }
.dn-total { font-size: 22px; font-weight: 700; fill: var(--fg); }
.dn-caption { font-size: 7.5px; fill: var(--fg-muted); text-transform: uppercase; letter-spacing: .06em; }
.dn-legend { flex: 1 1 10rem; margin: 0; padding: 0; list-style: none; display: grid; gap: .25rem; font-size: .875rem; }
.dn-legend li { display: grid; grid-template-columns: auto 1fr auto 3rem; align-items: center; gap: .5rem; }
.dn-key { width: .75rem; height: .75rem; background: var(--c-dot, var(--fg-subtle)); border-radius: 2px; }
.dn-n { font-variant-numeric: tabular-nums; font-weight: 700; }
.dn-pct { font-variant-numeric: tabular-nums; color: var(--fg-muted); text-align: right; }
</style>
