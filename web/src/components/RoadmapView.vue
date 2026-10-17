<!-- layout = "roadmap": every issue on one time line. A bar over its
     sprint's dates, a diamond on its date fields, colored by its status;
     today a red line. Issues with neither are listed below, unscheduled.
     A row opens the issue, as in the table. -->
<template>
  <div class="rm">
    <div class="rm-scale" aria-hidden="true">
      <span class="rm-scale-pad" />
      <div class="rm-track">
        <span v-for="sp in sprintBands" :key="sp.slug" class="rm-sprint" :style="{ left: pct(sp.from), width: `calc(${widthOf(sp.from, sp.to)} - 2px)` }" :title="sp.title">{{ sp.title }}</span>
        <span v-for="t in tickList" :key="t.at" class="rm-tick" :style="{ left: pct(t.at) }">{{ t.label }}</span>
      </div>
    </div>

    <section v-for="b in rows" :key="b.group" class="rm-group" :aria-label="group ? groupLabel(b.group) : 'Scheduled issues'">
      <h2 v-if="group" class="rm-group-title">
        <ValueMark :field="group" :value="b.group" />
        {{ groupLabel(b.group) }} <span class="rm-count">{{ b.items.length }}</span>
      </h2>
      <div v-for="it in b.items" :key="it.issue.id" class="rm-row" @click="emit('open', it.issue)">
        <button type="button" class="rm-title" :title="it.issue.shown_title">
          <span class="rm-id">{{ it.issue.id.slice(0, 7) }}</span> {{ it.issue.shown_title }}
        </button>
        <div class="rm-track" :class="colorOf(it.issue)">
          <span v-for="t in tickList" :key="t.at" class="rm-grid" :style="{ left: pct(t.at) }" aria-hidden="true" />
          <span class="rm-today" :style="{ left: pct(today) }" aria-hidden="true" />
          <template v-for="m in it.marks" :key="m.field + m.start">
            <span v-if="m.kind === 'span'" class="rm-bar" :style="{ left: pct(m.start), width: width(m) }" :title="`${m.label}: ${dates(m)}`" />
            <span v-else class="rm-diamond" :class="{ 'rm-late': m.start < today && it.issue.status === 'open' }" :style="{ left: pct(m.start) }" :title="m.label" />
          </template>
        </div>
      </div>
    </section>

    <section v-if="unscheduled.length" class="rm-unscheduled" aria-labelledby="rm-uns">
      <h2 id="rm-uns" class="rm-group-title">Unscheduled <span class="rm-count">{{ unscheduled.length }}</span></h2>
      <p class="gb-hint">No sprint with dates and no date field yet.</p>
      <ul>
        <li v-for="i in unscheduled" :key="i.id">
          <button type="button" class="rm-title" @click="emit('open', i)"><span class="rm-id">{{ i.id.slice(0, 7) }}</span> {{ i.shown_title }}</button>
        </li>
      </ul>
    </section>
    <p v-if="!rows.length" class="gb-hint rm-empty">
      Nothing to place on the time line: give issues a sprint listed in <code>[[sprints.list]]</code>, or a date field such as <code>due:2026-11-30</code>.
    </p>
  </div>
</template>

<script setup lang="ts">
import ValueMark from '@/components/ValueMark.vue';
import { computed, onMounted, ref } from 'vue';
import { getSprints, type Bucket, type Issue, type SprintWindow } from '@/api/board';
import { colorClass } from '@/colors';
import { displayName, fieldValues, type Fields } from '@/fields';
import { marksOf, span, ticks, type Mark } from '@/roadmap';

const props = defineProps<{ buckets: Bucket[]; group: string; fields: Fields }>();
const emit = defineEmits<{ open: [issue: Issue] }>();

const sprints = ref<SprintWindow[]>([]);
onMounted(async () => {
  try {
    sprints.value = await getSprints();
  } catch {
    sprints.value = [];
  }
});

const today = Date.now();
const placed = computed(() =>
  props.buckets.map((b) => ({
    group: b.group,
    items: b.issues.map((issue) => ({ issue, marks: marksOf(issue.labels, props.fields, sprints.value) })),
  })),
);
const rows = computed(() =>
  placed.value
    .map((b) => ({ group: b.group, items: b.items.filter((i) => i.marks.length).sort((x, y) => first(x.marks) - first(y.marks)) }))
    .filter((b) => b.items.length),
);
const unscheduled = computed(() => placed.value.flatMap((b) => b.items.filter((i) => !i.marks.length).map((i) => i.issue)));
const extent = computed(() => span(rows.value.flatMap((b) => b.items.flatMap((i) => i.marks)), today));
const tickList = computed(() => ticks(extent.value.from, extent.value.to));

// the sprints the time line crosses, named above the dates
const sprintBands = computed(() =>
  sprints.value
    .map((sp) => ({ slug: sp.slug, title: sp.title, from: Date.parse(`${sp.start}T00:00:00`), to: Date.parse(`${sp.end}T00:00:00`) }))
    .filter((sp) => sp.to > extent.value.from && sp.from < extent.value.to)
    .map((sp) => ({ ...sp, from: Math.max(sp.from, extent.value.from), to: Math.min(sp.to, extent.value.to) })),
);
const widthOf = (from: number, to: number) => `${((to - from) / (extent.value.to - extent.value.from)) * 100}%`;
const first = (marks: Mark[]) => Math.min(...marks.map((m) => m.start));
const pct = (t: number) => `${((t - extent.value.from) / (extent.value.to - extent.value.from)) * 100}%`;
const width = (m: Mark) => `${((m.end - m.start) / (extent.value.to - extent.value.from)) * 100}%`;
const fmt = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
const dates = (m: Mark) => `${fmt.format(m.start)} to ${fmt.format(m.end - 864e5)}`;

// the issue's status color, else the grouping's
function colorOf(issue: Issue): string {
  const status = fieldValues(issue.labels, 'status')[0];
  return (status && colorClass('status', status)) || (props.group && colorClass(props.group, fieldValues(issue.labels, props.group)[0] ?? '')) || 'gb-c-grey';
}

const groupLabel = (v: string) => (v === '(none)' ? `No ${displayName(props.group).toLowerCase()}` : v);
</script>

<style scoped>
.rm { --rm-title: 22rem; position: relative; }
.rm-scale { position: sticky; top: 0; z-index: 2; display: flex; background: var(--bg); border-bottom: 1px solid var(--border); }
.rm-scale-pad { flex: none; width: var(--rm-title); }
.rm-track { position: relative; flex: 1; min-width: 0; height: 2.25rem; }
.rm-scale .rm-track { height: 3.25rem; }
.rm-sprint { position: absolute; top: .25rem; height: 1.25rem; padding: 0 .375rem; overflow: hidden; border-left: 2px solid var(--border-strong); background: var(--bg-subtle); font-size: .75rem; font-weight: 700; line-height: 1.25rem; white-space: nowrap; text-overflow: ellipsis; }
.rm-tick { position: absolute; bottom: .375rem; transform: translateX(-50%); font-size: .75rem; color: var(--fg-muted); white-space: nowrap; font-variant-numeric: tabular-nums; }
.rm-group { margin-top: 1rem; }
.rm-group-title { display: flex; align-items: center; gap: .5rem; margin: 0 0 .25rem; font-size: 1rem; font-weight: 700; }
.rm-count { font-weight: 400; color: var(--fg-muted); }
.rm-row { display: flex; align-items: center; border-bottom: 1px solid var(--border-subtle); cursor: pointer; }
.rm-row:hover { background: var(--bg-subtle); }
.rm-title { flex: none; width: var(--rm-title); padding: .375rem .75rem .375rem 0; overflow: hidden; border: 0; background: none; color: var(--fg); font: inherit; font-size: .875rem; text-align: left; white-space: nowrap; text-overflow: ellipsis; cursor: pointer; }
.rm-id { font-family: var(--font-mono); font-size: .75rem; color: var(--fg-muted); }
.rm-grid { position: absolute; top: 0; bottom: 0; width: 1px; background: var(--border-subtle); }
.rm-today { position: absolute; top: 0; bottom: 0; width: 2px; margin-left: -1px; background: var(--primary); opacity: .55; }
.rm-bar { position: absolute; top: 50%; height: .75rem; margin-top: -.375rem; border-radius: 4px; background: var(--c-dot, var(--fg-muted)); box-shadow: 0 0 0 2px var(--bg); }
.rm-diamond { position: absolute; top: 50%; width: .75rem; height: .75rem; margin: -.375rem 0 0 -.375rem; transform: rotate(45deg); background: var(--c-fg, var(--fg)); box-shadow: 0 0 0 2px var(--bg); }
.rm-late { background: var(--danger); }
.rm-unscheduled { margin-top: 2rem; }
.rm-unscheduled ul { margin: .5rem 0 0; padding: 0; list-style: none; columns: 2 24rem; }
.rm-unscheduled .rm-title { width: 100%; }
.rm-empty { margin-top: 1.5rem; }
@media (max-width: 47.99rem) { .rm { --rm-title: 10rem; } }
</style>
