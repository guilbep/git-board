<!-- Insights: the issues' history, which git-bug keeps operation by
     operation, replayed day by day (internal/insights): how the work
     flowed, how fast it closes, how long it takes, the sprint's burn-up,
     and the board as it was on any day. Each chart says its finding in
     one sentence. -->
<template>
  <AppHeader :views="viewNames" active="" />
  <main class="gb-page ins">
    <div class="epfl-eyebrow">Insights</div>
    <h1 class="h3">How the work flows</h1>
    <p class="gb-lead">Replayed from every change git-bug keeps, day by day: nothing to set up, nothing to export.</p>

    <div class="ins-controls" role="group" aria-label="What to look at">
      <label class="ins-control">
        <span class="epfl-label">Issues</span>
        <select v-model="view" class="epfl-select">
          <option value="">All issues</option>
          <option v-for="v in viewNames" :key="v" :value="v">View: {{ v }}</option>
        </select>
      </label>
      <label class="ins-control">
        <span class="epfl-label">Field</span>
        <select v-model="field" class="epfl-select">
          <option v-for="f in fieldChoices" :key="f" :value="f">{{ displayName(f) }}</option>
        </select>
      </label>
      <label class="ins-control">
        <span class="epfl-label">Period</span>
        <select v-model.number="days" class="epfl-select">
          <option :value="30">Last 30 days</option>
          <option :value="90">Last 90 days</option>
          <option :value="180">Last 6 months</option>
          <option :value="365">Last year</option>
        </select>
      </label>
      <span v-if="loading" class="gb-hint" role="status">Replaying the history…</span>
      <button v-if="r?.burnup" type="button" class="epfl-btn epfl-btn-secondary ins-report" @click="reportOpen = true">Sprint report</button>
    </div>
    <SprintReportDialog v-model="reportOpen" />

    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>

    <template v-if="r">
      <div class="ins-tiles">
        <div class="ins-tile">
          <span class="ins-tile-label">Open today</span>
          <span class="ins-tile-value">{{ openToday }}</span>
          <span class="ins-tile-delta">{{ signed(openToday - openAtStart) }} in {{ r.days.length - 1 }} days</span>
        </div>
        <div class="ins-tile">
          <span class="ins-tile-label">Closed, last 4 weeks</span>
          <span class="ins-tile-value">{{ closedRecent }}</span>
          <span class="ins-tile-delta">{{ closedBefore }} the 4 weeks before</span>
        </div>
        <div class="ins-tile">
          <span class="ins-tile-label">Median {{ r.cycle.from === 'opened' ? 'lead' : 'cycle' }} time</span>
          <span class="ins-tile-value">{{ r.cycle.days.length ? days1(r.cycle.median) : '–' }}</span>
          <span class="ins-tile-delta">{{ r.cycle.days.length ? `85% within ${days1(r.cycle.p85)}` : 'no issue started and closed' }}</span>
        </div>
        <div v-if="r.burnup" class="ins-tile">
          <span class="ins-tile-label">{{ r.burnup.title }}</span>
          <span class="ins-tile-value">{{ sprintPct }}%</span>
          <span class="ins-tile-delta">{{ r.burnup.done.at(-1) }} of {{ r.burnup.scope.at(-1) }} done</span>
        </div>
      </div>

      <section class="ins-card ins-wide" aria-labelledby="h-flow">
        <h2 id="h-flow" class="ins-headline">{{ r.headlines.flow }}</h2>
        <p class="ins-sub">Cumulative flow: issues per {{ displayName(r.field).toLowerCase() }}, each day. A band that widens is work piling up there.</p>
        <FlowChart :field="r.field" :days="r.days" :values="r.values" :series="r.all" :label="`Cumulative flow of ${r.field}`" />
      </section>

      <div class="ins-grid">
        <section v-if="r.burnup" class="ins-card" aria-labelledby="h-sd">
          <h2 id="h-sd" class="ins-headline">{{ standsHeadline(sprintEntries, r.burnup.title) }}</h2>
          <p class="ins-sub">Where the sprint stands: its issues by {{ displayName(r.field).toLowerCase() }}, today.</p>
          <DonutChart :entries="sprintEntries" :field="r.field" :caption="r.burnup.title" :label="`${r.burnup.title} by ${r.field}`" />
        </section>
        <section class="ins-card" aria-labelledby="h-od">
          <h2 id="h-od" class="ins-headline">{{ standsHeadline(openEntries, 'the open issues') }}</h2>
          <p class="ins-sub">The open issues today, by {{ displayName(r.field).toLowerCase() }}.</p>
          <DonutChart :entries="openEntries" :field="r.field" caption="open" :label="`Open issues by ${r.field}`" />
        </section>
        <section class="ins-card" aria-labelledby="h-thr">
          <h2 id="h-thr" class="ins-headline">{{ r.headlines.throughput }}</h2>
          <p class="ins-sub">Throughput: issues closed each week. This week in red.</p>
          <ColumnChart :items="weekItems" :label="'Issues closed per week'" highlight-last :every-label="Math.ceil(weekItems.length / 6)" />
        </section>
        <section class="ins-card" aria-labelledby="h-cyc">
          <h2 id="h-cyc" class="ins-headline">{{ r.headlines.cycle }}</h2>
          <p class="ins-sub">{{ r.cycle.from === 'opened' ? 'Lead time: from opened to closed' : `Cycle time: from “${r.cycle.start}” to closed` }}, for the issues closed in the period.</p>
          <ColumnChart :items="cycleItems" :label="'Cycle time distribution'" />
        </section>
        <section v-if="r.burnup" class="ins-card" aria-labelledby="h-bu">
          <h2 id="h-bu" class="ins-headline">{{ r.headlines.burnup }}</h2>
          <p class="ins-sub">Burn-up: the sprint's issues and the closed ones, against an even pace.</p>
          <BurnupChart v-bind="r.burnup" :label="`Burn-up of ${r.burnup.title}`" />
        </section>
        <section class="ins-card" aria-labelledby="h-tm">
          <h2 id="h-tm" class="ins-headline">The board on {{ dayLabel(r.days[tm]!) }}: {{ tmTotal }} open issues</h2>
          <p class="ins-sub">Time machine: drag to any day of the period. The ticks are today's counts.</p>
          <input v-model.number="tm" class="ins-slider" type="range" :min="0" :max="r.days.length - 1" :aria-label="`Day: ${dayLabel(r.days[tm]!)}`" />
          <ul class="ins-bars">
            <li v-for="v in r.values" :key="v">
              <span class="ins-bar-name"><i :class="cls(v)" />{{ v }}</span>
              <span class="ins-bar-track">
                <span class="ins-bar" :class="cls(v)" :style="{ width: `${(count(v, tm) / tmMax) * 100}%` }" />
                <span class="ins-bar-now" :style="{ left: `${(count(v, r.days.length - 1) / tmMax) * 100}%` }" :title="`Today: ${count(v, r.days.length - 1)}`" />
              </span>
              <span class="ins-bar-n">{{ count(v, tm) }}</span>
            </li>
          </ul>
        </section>
      </div>

      <details class="ins-data">
        <summary>The numbers, as a table</summary>
        <div class="gb-table-wrap">
          <table class="gb-table">
            <thead><tr><th>Week of</th><th>Opened</th><th>Closed</th></tr></thead>
            <tbody><tr v-for="w in [...r.weeks].reverse()" :key="w.start"><td>{{ dayLabel(w.start) }}</td><td>{{ w.opened }}</td><td>{{ w.closed }}</td></tr></tbody>
          </table>
        </div>
      </details>
    </template>
  </main>
</template>

<script setup lang="ts">
import SprintReportDialog from '@/components/SprintReportDialog.vue';
import { debounced, onLive } from '@/live';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { getFields, getInsights, getViews, type FieldConfig, type InsightsReport } from '@/api/board';
import { colorClass, loadColors } from '@/colors';
import { displayName } from '@/fields';
import { message } from '@/issue';
import AppHeader from '@/components/AppHeader.vue';
import BurnupChart from '@/components/BurnupChart.vue';
import ColumnChart from '@/components/ColumnChart.vue';
import FlowChart from '@/components/FlowChart.vue';
import DonutChart from '@/components/DonutChart.vue';
import { valueIcon } from '@/valueicons';

const viewNames = ref<string[]>([]);
const fields = ref<Record<string, FieldConfig>>({});
const view = ref('');
const field = ref('status');
const days = ref(90);
const r = ref<InsightsReport | null>(null);
const loading = ref(false);
const error = ref('');
const tm = ref(0);

// fields with an order worth a flow: those with options
const fieldChoices = computed(() => {
  const names = Object.keys(fields.value).filter((n) => fields.value[n]?.options?.length && fields.value[n]?.type !== 'number');
  return names.includes('status') ? ['status', ...names.filter((n) => n !== 'status').sort()] : names.sort();
});

async function load() {
  loading.value = true;
  error.value = '';
  try {
    const [rep] = await Promise.all([getInsights(field.value, days.value, view.value), loadColors()]);
    r.value = rep;
    tm.value = Math.max(0, Math.min(tm.value || Infinity, rep.days.length - 1));
  } catch (e) {
    error.value = message(e);
  } finally {
    loading.value = false;
  }
}
watch([view, field, days], () => void load());

// the sprint report, and a new replay when the issues changed elsewhere
const reportOpen = ref(false);
const replay = debounced(() => void load(), 2000);
const offLive = onLive((e) => (e.kind === 'issues' || e.kind === 'sync') && replay());
onBeforeUnmount(offLive);

onMounted(async () => {
  try {
    const [v, f] = await Promise.all([getViews(), getFields()]);
    viewNames.value = Object.keys(v).sort();
    fields.value = f;
  } catch {
    // the defaults still work
  }
  await load();
});

const last = computed(() => (r.value ? r.value.days.length - 1 : 0));
const count = (v: string, i: number) => r.value?.open[v]?.[i] ?? 0;
const openAt = (i: number) => (r.value ? r.value.values.reduce((t, v) => t + count(v, i), 0) : 0);
const openToday = computed(() => openAt(last.value));
const openAtStart = computed(() => openAt(0));
const closedSum = (from: number, to: number) => (r.value ? r.value.weeks.slice(Math.max(0, r.value.weeks.length - to), r.value.weeks.length - from).reduce((t, w) => t + w.closed, 0) : 0);
const closedRecent = computed(() => closedSum(0, 4));
const closedBefore = computed(() => closedSum(4, 8));
const sprintPct = computed(() => {
  const b = r.value?.burnup;
  const scope = b?.scope.at(-1) ?? 0;
  return scope ? Math.round(((b?.done.at(-1) ?? 0) / scope) * 100) : 0;
});
const tmTotal = computed(() => openAt(tm.value));

// the donuts: in workflow order, no value last
const ordered = (counts: Record<string, number>) =>
  r.value ? [...r.value.values, ...Object.keys(counts).filter((k) => !r.value!.values.includes(k))].map((key) => ({ key, n: counts[key] ?? 0 })).filter((e) => e.n) : [];
const sprintEntries = computed(() => ordered(r.value?.burnup?.status ?? {}));
const openEntries = computed(() => ordered(Object.fromEntries((r.value?.values ?? []).map((v) => [v, count(v, last.value)]))));
// "61% of Hypercare is validated or done", from the values whose words
// say done (valueicons.ts); else the biggest share
function standsHeadline(entries: { key: string; n: number }[], what: string): string {
  const total = entries.reduce((t, e) => t + e.n, 0);
  if (!total || !r.value) return `Nothing in ${what} yet.`;
  const done = entries.filter((e) => valueIcon(r.value!.field, e.key) === 'st-done');
  if (done.length) {
    const pct = Math.round((done.reduce((t, e) => t + e.n, 0) / total) * 100);
    return `${pct}% of ${what} is ${done.map((e) => e.key).join(' or ')}.`;
  }
  const top = [...entries].sort((a, b) => b.n - a.n)[0]!;
  return `Most of ${what} is ${top.key === '(none)' ? 'without a ' + r.value.field : top.key}: ${Math.round((top.n / total) * 100)}%.`;
}
const tmMax = computed(() => {
  if (!r.value) return 1;
  let m = 1;
  for (const v of r.value.values) for (const n of r.value.open[v] ?? []) m = Math.max(m, n);
  return m;
});

const short = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
const long = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
const dayLabel = (iso: string) => long.format(new Date(`${iso}T12:00:00`));
const weekItems = computed(() =>
  (r.value?.weeks ?? []).map((w) => ({ label: short.format(new Date(`${w.start}T12:00:00`)), value: w.closed, title: `Week of ${dayLabel(w.start)}: ${w.closed} closed, ${w.opened} opened` })),
);
const bins = [
  { label: '<1d', to: 1 },
  { label: '1–2d', to: 2 },
  { label: '2–4d', to: 4 },
  { label: '4–7d', to: 7 },
  { label: '1–2w', to: 14 },
  { label: '2–4w', to: 28 },
  { label: '1–2m', to: 61 },
  { label: '>2m', to: Infinity },
];
const cycleItems = computed(() => {
  const counts = bins.map(() => 0);
  for (const d of r.value?.cycle.days ?? []) counts[bins.findIndex((b) => d < b.to)]!++;
  return bins.map((b, i) => ({ label: b.label, value: counts[i]!, title: `${counts[i]} issue${counts[i] === 1 ? '' : 's'} took ${b.label}` }));
});

// no value: the light neutral of the flow chart
const cls = (v: string) => (v === '(none)' ? 'ins-none' : (r.value && colorClass(r.value.field, v)) || 'gb-c-grey');
const signed = (d: number) => (d > 0 ? `+${d}` : `${d}`);
const days1 = (d: number) => (d < 1 ? `${Math.round(d * 24)} h` : `${Math.round(d)} d`);
</script>

<style scoped>
.ins-report { align-self: end; margin-left: auto; }
.ins-none { --c-dot: #cfcfcf; }
.ins-controls { display: flex; flex-wrap: wrap; align-items: flex-end; gap: .75rem 1rem; margin: 1.5rem 0; }
.ins-control { display: grid; gap: .25rem; min-width: 12rem; }
.ins-tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr)); gap: 1rem; margin-bottom: 1.5rem; }
.ins-tile { display: grid; gap: .125rem; padding: 1rem 1.25rem; border: 1px solid var(--border); border-top: 3px solid var(--primary); background: var(--bg); }
.ins-tile-label { font-size: .8125rem; font-weight: 700; color: var(--fg-muted); }
.ins-tile-value { font-size: 2.25rem; font-weight: 700; line-height: 1.1; }
.ins-tile-delta { font-size: .8125rem; color: var(--fg-muted); }
.ins-card { padding: 1.25rem 1.5rem; border: 1px solid var(--border); background: var(--bg); }
.ins-wide { margin-bottom: 1rem; }
.ins-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(22rem, 1fr)); gap: 1rem; }
.ins-headline { margin: 0; font-size: 1.0625rem; font-weight: 700; line-height: 1.35; }
.ins-sub { margin: .25rem 0 .75rem; font-size: .8125rem; color: var(--fg-muted); }
.ins-slider { width: 100%; accent-color: var(--primary); }
.ins-bars { display: grid; gap: .375rem; margin: .75rem 0 0; padding: 0; list-style: none; }
.ins-bars li { display: grid; grid-template-columns: 9rem 1fr 2.5rem; align-items: center; gap: .5rem; font-size: .8125rem; }
.ins-bar-name { display: flex; align-items: center; gap: .375rem; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.ins-bar-name i { display: inline-block; flex: none; width: .625rem; height: .625rem; border-radius: 2px; background: var(--c-dot); }
.ins-bar-track { position: relative; height: .875rem; background: var(--bg-subtle); }
.ins-bar { position: absolute; inset: 0 auto 0 0; border-radius: 0 4px 4px 0; background: var(--c-dot); transition: width .15s; }
.ins-bar-now { position: absolute; top: -3px; bottom: -3px; width: 2px; margin-left: -1px; background: var(--fg); }
.ins-bar-n { text-align: right; font-weight: 700; font-variant-numeric: tabular-nums; }
.ins-data { margin-top: 1.5rem; }
.ins-data summary { cursor: pointer; font-weight: 700; }
.ins-data .gb-table-wrap { max-width: 28rem; margin-top: .5rem; }
</style>
