<!-- Cumulative flow: per day, the issues in each value of the field,
     stacked in workflow order (the last value at the bottom), each in its
     value's color. Hover: the day's counts. -->
<template>
  <div ref="el" class="fc" @mouseleave="hover = -1">
    <svg :viewBox="`0 0 ${W} ${H}`" role="img" :aria-label="label" class="fc-svg" @mousemove="onMove">
      <g class="fc-grid">
        <template v-for="t in yTicks" :key="t">
          <line :x1="L" :x2="W - R" :y1="y(t)" :y2="y(t)" />
          <text :x="L - 6" :y="y(t) + 4" text-anchor="end">{{ t }}</text>
        </template>
        <text v-for="(t, k) in xTicks" :key="t" :x="x(t)" :y="H - 6" :text-anchor="k === 0 ? 'start' : k === xTicks.length - 1 ? 'end' : 'middle'">{{ dayLabel(days[t]!) }}</text>
      </g>
      <path v-for="b in bands" :key="b.value" :d="b.d" :class="b.cls" class="fc-band" />
      <g v-if="hover >= 0">
        <line class="fc-cross" :x1="x(hover)" :x2="x(hover)" :y1="T" :y2="H - B" />
      </g>
    </svg>
    <div v-if="hover >= 0" class="fc-tip" :style="{ left: `${Math.min(Math.max((x(hover) / W) * 100, 12), 88)}%` }" role="status">
      <b>{{ dayLabel(days[hover]!, true) }}</b>
      <span v-for="v in [...stack].reverse()" :key="v" class="fc-tip-row">
        <i :class="cls(v)" />{{ v }} <b>{{ series[v]![hover] }}</b>
      </span>
    </div>
    <ul class="fc-legend" aria-label="Values">
      <li v-for="v in values" :key="v"><i :class="cls(v)" />{{ v }} <span>{{ series[v]![days.length - 1] }}</span></li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { colorClass } from '@/colors';
import { niceTicks, useWidth } from '@/chartsize';

const props = defineProps<{ field: string; days: string[]; values: string[]; series: Record<string, number[]>; label: string }>();

const el = ref<HTMLElement | null>(null);
const W = useWidth(el, 720);
const H = 280;
const L = 36;
const R = 12;
const T = 12;
const B = 26;
const hover = ref(-1);

// bottom to top: the workflow reversed (done at the bottom), no value last
const stack = computed(() => [...props.values.filter((v) => v !== '(none)').reverse(), ...props.values.filter((v) => v === '(none)')]);
const totals = computed(() => props.days.map((_, i) => props.values.reduce((t, v) => t + (props.series[v]?.[i] ?? 0), 0)));
const yTicks = computed(() => niceTicks(Math.max(...totals.value)));
const max = computed(() => yTicks.value.at(-1)!);
const xTicks = computed(() => {
  const n = props.days.length;
  const step = Math.max(1, Math.round(n / Math.max(2, Math.floor(W.value / 140))));
  return Array.from({ length: Math.ceil(n / step) }, (_, k) => k * step).filter((i) => i < n);
});
const x = (i: number) => L + (i / Math.max(1, props.days.length - 1)) * (W.value - L - R);
const y = (v: number) => H - B - (v / max.value) * (H - B - T);

const bands = computed(() => {
  const base = props.days.map(() => 0);
  return stack.value.map((v) => {
    const s = props.series[v] ?? [];
    const lower = [...base];
    s.forEach((c, i) => (base[i]! += c));
    const top = base.map((t, i) => `${x(i)},${y(t)}`);
    const bottom = lower.map((t, i) => `${x(i)},${y(t)}`).reverse();
    return { value: v, cls: cls(v), d: `M${top.join('L')}L${bottom.join('L')}Z` };
  });
});

// no value: a light neutral of its own, apart from a grey value (backlog)
function cls(v: string): string {
  return v === '(none)' ? 'fc-none' : colorClass(props.field, v) || 'gb-c-grey';
}

function onMove(e: MouseEvent) {
  const svg = e.currentTarget as SVGSVGElement;
  const r = svg.getBoundingClientRect();
  const px = ((e.clientX - r.left) / r.width) * W.value;
  const i = Math.round(((px - L) / (W.value - L - R)) * (props.days.length - 1));
  hover.value = i >= 0 && i < props.days.length ? i : -1;
}

const short = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
const long = new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
const dayLabel = (iso: string, full = false) => (full ? long : short).format(new Date(`${iso}T12:00:00`));

</script>

<style scoped>
.fc { position: relative; }
.fc-svg { display: block; width: 100%; height: auto; }
.fc-grid line { stroke: var(--border-subtle); stroke-width: 1; }
.fc-grid text { fill: var(--fg-muted); font-size: 11px; font-variant-numeric: tabular-nums; }
.fc-band { fill: var(--c-dot); fill-opacity: .82; stroke: var(--bg); stroke-width: 1.5; stroke-linejoin: round; }
.fc-cross { stroke: var(--fg); stroke-width: 1; }
.fc-tip { position: absolute; top: .5rem; transform: translateX(-50%); display: grid; gap: .125rem; min-width: 11rem; padding: .5rem .625rem; background: var(--bg); border: 1px solid var(--border-strong); box-shadow: var(--shadow); font-size: .8125rem; pointer-events: none; }
.fc-tip-row { display: flex; align-items: center; gap: .375rem; }
.fc-tip-row b { margin-left: auto; font-variant-numeric: tabular-nums; }
.fc-none { --c-dot: #cfcfcf; }
.fc-tip i, .fc-legend i { display: inline-block; width: .625rem; height: .625rem; border-radius: 2px; background: var(--c-dot); flex: none; }
.fc-legend { display: flex; flex-wrap: wrap; gap: .25rem 1rem; margin: .5rem 0 0; padding: 0; list-style: none; font-size: .8125rem; }
.fc-legend li { display: inline-flex; align-items: center; gap: .375rem; }
.fc-legend span { color: var(--fg-muted); font-variant-numeric: tabular-nums; }
</style>
