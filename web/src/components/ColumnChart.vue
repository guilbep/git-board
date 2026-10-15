<!-- Columns from one baseline: weekly throughput, the cycle-time
     distribution. Thin columns with a rounded top; the value on the
     columns that matter (the highest, the last), the others on hover. -->
<template>
  <div ref="el"><svg :viewBox="`0 0 ${W} ${H}`" role="img" :aria-label="label" class="cc">
    <g class="cc-grid">
      <template v-for="t in yTicks" :key="t">
        <line :x1="L" :x2="W - R" :y1="y(t)" :y2="y(t)" />
        <text :x="L - 6" :y="y(t) + 4" text-anchor="end">{{ t }}</text>
      </template>
    </g>
    <g v-for="(it, i) in items" :key="it.label" class="cc-col">
      <rect :x="slot(i)" :y="T" :width="slotW" :height="H - B - T" class="cc-hit"><title>{{ it.title }}</title></rect>
      <path v-if="it.value > 0" :d="col(i, it.value)" :class="{ 'cc-now': i === items.length - 1 && highlightLast }" />
      <text v-if="shown(i)" :x="mid(i)" :y="y(it.value) - 6" text-anchor="middle" class="cc-val">{{ it.value }}</text>
      <text v-if="labelled(i)" :x="mid(i)" :y="H - 6" text-anchor="middle" class="cc-x">{{ it.label }}</text>
    </g>
  </svg></div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { niceTicks, useWidth } from '@/chartsize';

const props = defineProps<{ items: { label: string; value: number; title: string }[]; label: string; highlightLast?: boolean; everyLabel?: number }>();

const el = ref<HTMLElement | null>(null);
const W = useWidth(el, 360);
const H = 220;
const L = 30;
const R = 8;
const T = 18;
const B = 24;

const yTicks = computed(() => niceTicks(Math.max(...props.items.map((i) => i.value)), 3));
const max = computed(() => yTicks.value.at(-1)!);
const slotW = computed(() => (W.value - L - R) / Math.max(1, props.items.length));
const barW = computed(() => Math.min(24, slotW.value * 0.62));
const slot = (i: number) => L + i * slotW.value;
const mid = (i: number) => slot(i) + slotW.value / 2;
const y = (v: number) => H - B - (v / max.value) * (H - B - T);

// a column with a 4px rounded top, square on the baseline
function col(i: number, v: number): string {
  const w = barW.value;
  const x0 = mid(i) - w / 2;
  const top = y(v);
  const base = H - B;
  const r = Math.min(4, w / 2, base - top);
  return `M${x0},${base}V${top + r}Q${x0},${top} ${x0 + r},${top}H${x0 + w - r}Q${x0 + w},${top} ${x0 + w},${top + r}V${base}Z`;
}

const peak = computed(() => props.items.reduce((m, it, i) => (it.value > (props.items[m]?.value ?? -1) ? i : m), 0));
const shown = (i: number) => props.items[i]!.value > 0 && (i === peak.value || i === props.items.length - 1);
// every nth label, and the last, but not a regular one crowding it
const labelled = (i: number) => {
  const every = props.everyLabel ?? 1;
  const last = props.items.length - 1;
  return i === last || (i % every === 0 && last - i >= Math.max(1, every / 2));
};
</script>

<style scoped>
.cc { display: block; width: 100%; height: auto; }
.cc-grid line { stroke: var(--border-subtle); stroke-width: 1; }
.cc-grid text, .cc-x { fill: var(--fg-muted); font-size: 10.5px; font-variant-numeric: tabular-nums; }
.cc-col path { fill: var(--gb-blue-dot, var(--fg-muted)); }
.cc-col path.cc-now { fill: var(--primary); }
.cc-hit { fill: transparent; }
.cc-col:hover path { opacity: .8; }
.cc-val { fill: var(--fg); font-size: 11px; font-weight: 700; font-variant-numeric: tabular-nums; }
</style>
