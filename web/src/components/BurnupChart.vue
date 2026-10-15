<!-- The current sprint, day by day: its scope (issues in it) and what is
     done (closed), against the even pace from nothing to the scope at the
     sprint's end. The axis runs to the end; the lines stop today. -->
<template>
  <div ref="el"><svg :viewBox="`0 0 ${W} ${H}`" role="img" :aria-label="label" class="bu">
    <g class="bu-grid">
      <template v-for="t in yTicks" :key="t">
        <line :x1="L" :x2="W - R" :y1="y(t)" :y2="y(t)" />
        <text :x="L - 6" :y="y(t) + 4" text-anchor="end">{{ t }}</text>
      </template>
      <text :x="L" :y="H - 6" text-anchor="start">{{ short(start) }}</text>
      <text :x="W - R" :y="H - 6" text-anchor="end">{{ short(end) }}</text>
    </g>
    <line class="bu-pace" :x1="x(0)" :y1="y(0)" :x2="x(span - 1)" :y2="y(lastScope)" />
    <path class="bu-done-area" :d="`${line(done)}L${x(done.length - 1)},${y(0)}L${x(0)},${y(0)}Z`" />
    <path class="bu-scope" :d="line(scope)" />
    <path class="bu-done" :d="line(done)" />
    <line v-if="days.length < span" class="bu-today" :x1="x(days.length - 1)" :x2="x(days.length - 1)" :y1="T" :y2="H - B" />
    <circle class="bu-dot bu-dot-scope" :cx="x(scope.length - 1)" :cy="y(lastScope)" r="4" />
    <circle class="bu-dot bu-dot-done" :cx="x(done.length - 1)" :cy="y(done.at(-1) ?? 0)" r="4" />
    <text class="bu-lab" :x="x(scope.length - 1) + 8" :y="y(lastScope) + 4">Scope {{ lastScope }}</text>
    <text class="bu-lab" :x="x(done.length - 1) + 8" :y="doneLabelY">Closed {{ done.at(-1) }}</text>
  </svg></div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { niceTicks, useWidth } from '@/chartsize';

const props = defineProps<{ days: string[]; scope: number[]; done: number[]; start: string; end: string; label: string }>();

const el = ref<HTMLElement | null>(null);
const W = useWidth(el, 360);
const H = 220;
const L = 30;
const R = 64; // room for the end labels
const T = 14;
const B = 24;

// the sprint's length in days, end excluded
const span = computed(() => Math.max(2, Math.round((Date.parse(props.end) - Date.parse(props.start)) / 864e5)));
const lastScope = computed(() => props.scope.at(-1) ?? 0);
const yTicks = computed(() => niceTicks(Math.max(...props.scope), 3));
const max = computed(() => yTicks.value.at(-1)!);
const x = (i: number) => L + (i / (span.value - 1)) * (W.value - L - R);
const y = (v: number) => H - B - (v / max.value) * (H - B - T);
// the Done label moves down when it would sit on the Scope one
const doneLabelY = computed(() => {
  const ys = y(lastScope.value) + 4;
  const yd = y(props.done.at(-1) ?? 0) + 4;
  return Math.abs(yd - ys) < 13 ? ys + 13 : yd;
});
const line = (s: number[]) => `M${s.map((v, i) => `${x(i)},${y(v)}`).join('L')}`;
const fmt = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
const short = (iso: string) => fmt.format(new Date(`${iso}T12:00:00`));
</script>

<style scoped>
.bu { display: block; width: 100%; height: auto; }
.bu-grid line { stroke: var(--border-subtle); stroke-width: 1; }
.bu-grid text { fill: var(--fg-muted); font-size: 10.5px; font-variant-numeric: tabular-nums; }
.bu-pace { stroke: var(--border-strong); stroke-width: 1; }
.bu-scope { fill: none; stroke: var(--fg); stroke-width: 2; stroke-linejoin: round; stroke-linecap: round; }
.bu-done { fill: none; stroke: var(--gb-green-dot); stroke-width: 2; stroke-linejoin: round; stroke-linecap: round; }
.bu-done-area { fill: var(--gb-green-dot); fill-opacity: .1; }
.bu-today { stroke: var(--primary); stroke-width: 1.5; opacity: .6; }
.bu-dot { stroke: var(--bg); stroke-width: 2; }
.bu-dot-scope { fill: var(--fg); }
.bu-dot-done { fill: var(--gb-green-dot); }
.bu-lab { fill: var(--fg); font-size: 11px; font-weight: 700; font-variant-numeric: tabular-nums; }
</style>
