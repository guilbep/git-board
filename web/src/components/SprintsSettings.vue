<!-- Settings › Sprints: the [sprints] table (the length, the weekday) and
     the [[sprints.list]] entries, each with its slug, title and start
     date; + Sprint adds the next one after the last, its start plus
     length_days, its number plus one — as the board's button does. -->
<template>
  <div class="set-panel">
    <h2 class="h4">Sprints</h2>
    <p class="ss-lead">The sprints' dates, so <code>sprint:@current</code> finds today's.</p>

    <div class="set-cols">
      <div class="set-forms">
        <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>
        <p v-else-if="!draft.value.value" class="ss-muted">Loading…</p>

        <template v-else>
          <div class="ss-keys">
            <label class="ss-key">
              <span>Sprint length, in days</span>
              <input v-model.number="lengthDays" type="number" min="1" class="ss-num" aria-label="Sprint length in days" />
            </label>
            <label class="ss-key">
              <span>Starts on</span>
              <select v-model="startWeekday" class="ss-num" aria-label="The weekday a sprint starts on">
                <option value="">—</option>
                <option v-for="d in weekdays" :key="d" :value="d">{{ d }}</option>
              </select>
            </label>
          </div>
          <p class="gb-hint">The length is the last sprint's, whose end has no next start, and what <b>+ Sprint</b> adds to the last start.</p>

          <ConfigRows :rows="list" add-label="+ Sprint" @update="list = $event" @add="add">
            <template #row="{ row }">
              <div class="ss-row">
                <input v-model="row.slug" class="ss-input ss-slug" placeholder="e.g. s12" aria-label="Slug" spellcheck="false" />
                <input v-model="row.title" class="ss-input" placeholder="e.g. Sprint 12" aria-label="Title" />
                <input v-model="row.start" type="date" class="ss-input ss-date" aria-label="First day" />
              </div>
            </template>
          </ConfigRows>
          <p class="gb-hint">The slug is the label's value: <code>sprint:s12</code>. A sprint lasts until the next one starts.</p>

          <ConfigSaveBar
            section="sprints"
            :dirty="draft.dirty.value || edited"
            :saving="draft.saving.value"
            :saved="draft.saved.value"
            :valid="valid"
            :error="draft.saveError.value"
            :conflict="draft.conflict.value"
            :state="state"
            @save="save"
            @discard="reset"
            @reload="draft.load()"
          />
        </template>


      </div>

      <ConfigRefPanel :sections="['sprints']" title="sprints and their dates" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import type { ConfigSprint } from '@/api/board';
import { nextSprint, sprintsPreset, toSprintStart, fromSprintStart } from '@/configpresets';
import { SectionDraft, configModel, configModelError } from '@/configmodel';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
import ConfigRows from '@/components/configforms/ConfigRows.vue';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';

type Sprints = { length_days: number; start_weekday: string; list: { slug: string; title: string; start: string }[] };

const modelError = configModelError;
const weekdays = ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday'];

const draft = new SectionDraft<Sprints>(
  'sprints',
  (m) => ({
    length_days: m.sprints.length_days,
    start_weekday: m.sprints.start_weekday,
    list: m.sprints.list.map((s) => ({ ...s, start: fromSprintStart(s.start) })),
  }),
  (a, b) =>
    a.length_days !== b.length_days || a.start_weekday !== b.start_weekday || JSON.stringify(a.list) !== JSON.stringify(b.list),
  (value) => ({
    length_days: value.length_days,
    start_weekday: value.start_weekday,
    list: value.list.filter((s) => s.slug.trim()).map((s) => ({ slug: s.slug.trim(), title: s.title, start: toSprintStart(s.start) })),
  }),
);

// the list's dates as the date input holds them (YYYY-MM-DD), the
// server's RFC 3339 read back
const lengthDays = ref(sprintsPreset.length_days);
const startWeekday = ref('');
const list = ref<ConfigSprint[]>([]);
watch(
  () => draft.value.value,
  (v) => {
    if (!v) return;
    lengthDays.value = v.length_days || sprintsPreset.length_days;
    startWeekday.value = v.start_weekday;
    list.value = v.list.map((s) => ({ ...s }));
  },
  { immediate: true },
);

// the next sprint after the last: its start plus the length, its number plus one
function add() {
  list.value = [...list.value, nextSprint(list.value, lengthDays.value)];
}

const valid = computed(() => list.value.every((s) => /^[a-z0-9._-]*$/i.test(s.slug)) && (!lengthDays.value || lengthDays.value > 0));
const state = computed(() => `${list.value.length} sprint${list.value.length === 1 ? '' : 's'} · a fortnight of ${lengthDays.value || '—'} days`);

function reset() {
  if (configModel.value) draft.reset(configModel.value);
}

// the inputs edit their own copies, the draft takes them at the save:
// edited is what makes Save live meanwhile
const edited = computed(() => {
  const v = draft.value.value;
  if (!v) return false;
  return lengthDays.value !== (v.length_days || sprintsPreset.length_days) || startWeekday.value !== v.start_weekday || JSON.stringify(list.value) !== JSON.stringify(v.list);
});

async function save() {
  const v = draft.value.value;
  draft.value.value = {
    // the preset shown for an unset length isn't written
    length_days: v && !v.length_days && lengthDays.value === sprintsPreset.length_days ? 0 : lengthDays.value,
    start_weekday: startWeekday.value,
    list: list.value,
  };
  await draft.save();
}

onMounted(() => void draft.load());
</script>

<style scoped>
.set-panel { display: grid; gap: .75rem; container-type: inline-size; }
/* the forms left, the reference right, like the old editor; one column when narrow */
.set-cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: .75rem; align-items: start; }
.set-forms { display: grid; gap: .75rem; min-width: 0; }
@container (max-width: 52rem) { .set-cols { grid-template-columns: minmax(0, 1fr); } }
h2, p { margin: 0; }
.ss-lead { max-width: 42rem; color: var(--fg-muted); }
.ss-muted { color: var(--fg-muted); }
.ss-keys { display: flex; gap: 1.5rem; flex-wrap: wrap; }
.ss-key { display: grid; gap: .25rem; font-size: .875rem; }
.ss-num { max-width: 8rem; padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.ss-num:focus, .ss-input:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
.ss-row { display: flex; gap: .5rem; }
.ss-input { flex: 1; padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.ss-slug { flex: none; width: 6rem; }
.ss-date { flex: none; width: 10rem; }
</style>
