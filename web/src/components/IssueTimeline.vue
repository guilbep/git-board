<!-- An issue's life on one time axis, from its opening to now (top of the
     History mode, timeline.ts): a band of the status it was in, a strip
     where it was closed, and a marker per change — what was written on one
     lane, what was changed on the other. A marker selects its change. -->
<template>
  <div class="tl" role="group" aria-label="Timeline of the issue">
    <div class="tl-row">
      <span class="tl-name">Texts</span>
      <div class="tl-track">
        <button
          v-for="m in t.markers.filter((x) => x.lane === 'texts')"
          :key="m.index"
          type="button"
          class="tl-mark"
          :class="[`tl-${m.kind}`, { 'tl-on': m.index === current }]"
          :style="{ left: `${m.left}%` }"
          :title="label(m.index)"
          :aria-label="label(m.index)"
          :aria-current="m.index === current ? 'step' : undefined"
          @click="emit('select', m.index)"
        />
      </div>
    </div>
    <div class="tl-row">
      <span class="tl-name">Changes</span>
      <div class="tl-track">
        <button
          v-for="m in t.markers.filter((x) => x.lane === 'changes')"
          :key="m.index"
          type="button"
          class="tl-mark"
          :class="[`tl-${m.kind}`, steps[m.index]?.status === 'closed' ? 'tl-closing' : '', { 'tl-on': m.index === current }]"
          :style="{ left: `${m.left}%` }"
          :title="label(m.index)"
          :aria-label="label(m.index)"
          :aria-current="m.index === current ? 'step' : undefined"
          @click="emit('select', m.index)"
        />
      </div>
    </div>
    <div class="tl-row">
      <span class="tl-name">{{ displayName(field) }}</span>
      <div class="tl-track tl-band">
        <div
          v-for="(g, i) in t.segments"
          :key="i"
          class="tl-seg"
          :class="[g.value ? colorClass(field, g.value) : 'tl-unset']"
          :style="{ left: `${g.left}%`, width: `${g.width}%` }"
          :title="`${g.value ? formatValue(field, g.value, fields) : 'Not set'}: ${span(g)}`"
        >
          <span v-if="g.width > 8" class="tl-seg-text"><ValueMark v-if="g.value" chip :field="field" :value="g.value" />{{ g.value ? formatValue(field, g.value, fields) : 'not set' }}</span>
        </div>
        <div v-for="(c, i) in t.closed" :key="'c' + i" class="tl-closed" :style="{ left: `${c.left}%`, width: `${c.width}%` }" title="Closed" />
      </div>
    </div>
    <div class="tl-row tl-axis" aria-hidden="true">
      <span class="tl-name" />
      <div class="tl-track">
        <span v-for="(k, i) in ticks" :key="i" class="tl-tick" :class="{ 'tl-tick-end': i === ticks.length - 1, 'tl-tick-start': i === 0 }" :style="{ left: `${k.left}%` }">{{ k.text }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { colorClass } from '@/colors';
import { displayName, formatValue, type Fields } from '@/fields';
import { when } from '@/me';
import { metaWhat, type Step } from '@/replay';
import { timeline, type Segment } from '@/timeline';
import ValueMark from '@/components/ValueMark.vue';

const props = defineProps<{ steps: Step[]; current: number; fields: Fields; field?: string }>();
const emit = defineEmits<{ select: [index: number] }>();

const field = computed(() => props.field ?? 'status');
const t = computed(() => timeline(props.steps, Date.now(), field.value));

const what: Record<Step['kind'], string> = {
  created: 'opened the issue', comment: 'commented', edit: 'edited a text', labels: 'changed fields', status: '', title: 'renamed it', meta: '',
};
function label(i: number): string {
  const s = props.steps[i];
  if (!s) return '';
  const verb = s.kind === 'status' ? (s.status === 'closed' ? 'closed it' : 'reopened it') : s.kind === 'meta' ? metaWhat(s.meta) : what[s.kind];
  return `${s.author.name} ${verb} · ${when(s.at, true)}`;
}

const day = 864e5;
function span(g: Segment): string {
  const ms = ((g.width / 100) * (t.value.end - t.value.start));
  const d = Math.round(ms / day);
  return d >= 1 ? `${d} day${d === 1 ? '' : 's'}` : `${Math.max(1, Math.round(ms / 36e5))} h`;
}

// a few dates under the axis: the opening, now, and evenly between
const fmt = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
const ticks = computed(() => {
  const { start, end } = t.value;
  const n = end - start > 2 * day ? 4 : 1;
  return Array.from({ length: n + 1 }, (_, i) => {
    const at = start + ((end - start) * i) / n;
    return { left: (i / n) * 100, text: i === n ? 'now' : fmt.format(new Date(at)) };
  });
});
</script>

<style scoped>
.tl { display: grid; gap: .25rem; padding: .625rem .75rem .375rem; border: 1px solid var(--border); background: var(--bg); }
.tl-row { display: flex; align-items: center; gap: .625rem; }
.tl-name { flex: none; width: 4.5rem; font-size: .75rem; color: var(--fg-muted); text-align: right; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tl-track { position: relative; flex: 1; height: 1rem; margin: 0 .375rem; }
.tl-track::before { content: ''; position: absolute; left: 0; right: 0; top: 50%; border-top: 1px dashed var(--border); }
.tl-band { height: 1.375rem; }
.tl-band::before { display: none; }

.tl-mark { position: absolute; top: 50%; width: .625rem; height: .625rem; padding: 0; border: 2px solid var(--fg-muted); border-radius: 50%; background: var(--bg); transform: translate(-50%, -50%); cursor: pointer; }
.tl-mark:hover, .tl-mark:focus-visible { border-color: var(--fg); transform: translate(-50%, -50%) scale(1.35); outline: none; z-index: 2; }
.tl-created { border-radius: 2px; background: var(--fg-muted); }
.tl-comment { background: var(--fg-muted); }
.tl-edit { border-style: dotted; }
.tl-labels { border-radius: 1px; transform: translate(-50%, -50%) rotate(45deg); }
.tl-labels:hover, .tl-labels:focus-visible { transform: translate(-50%, -50%) rotate(45deg) scale(1.35); }
.tl-title { border-radius: 2px; }
.tl-status { border-color: var(--success); }
.tl-closing { border-color: var(--fg); background: var(--fg); }
.tl-on { border-color: var(--primary); background: var(--primary); z-index: 3; }

.tl-seg { position: absolute; top: 0; bottom: .25rem; display: flex; align-items: center; overflow: hidden; background: var(--c-bg, var(--bg-muted)); border-left: 2px solid var(--c-dot, var(--border-strong)); color: var(--c-fg, var(--fg-muted)); }
.tl-unset { background: repeating-linear-gradient(135deg, var(--bg-muted) 0 4px, var(--bg) 4px 8px); border-left-color: var(--border); }
.tl-seg-text { display: inline-flex; align-items: center; gap: .25rem; min-width: 0; padding: 0 .375rem; overflow: hidden; font-size: .6875rem; font-weight: 700; white-space: nowrap; text-overflow: ellipsis; }
.tl-closed { position: absolute; bottom: 0; height: .1875rem; background: var(--fg-muted); }

.tl-axis .tl-track { height: .875rem; }
.tl-axis .tl-track::before { display: none; }
.tl-tick { position: absolute; top: 0; font-size: .6875rem; color: var(--fg-subtle); transform: translateX(-50%); white-space: nowrap; }
.tl-tick-start { transform: none; }
.tl-tick-end { transform: translateX(-100%); font-weight: 700; color: var(--fg-muted); }
</style>
