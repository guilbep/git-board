<!-- Settings › Forges › a forge › Fields: where each field is written and
     read on one forge (docs/prd/forge-field-mappings.md). One row per
     field, in field order: the field, the target it lives in (a dropdown
     of what this forge offers; the default marked), the details the
     target needs, the values' renames and aliases, and the migration —
     counted before anything is written, run after the save. Only the
     overrides go into [forges.<forge>.fields]; the aside shows the TOML
     as it stands. -->
<template>
  <div class="set-panel ff">
    <p class="ff-crumb"><router-link to="/settings/sync">Forges</router-link> › {{ forge?.name ?? '…' }}</p>
    <h2 class="h4">{{ forge ? `${forge.name} · ${forge.kind} · ${forge.project}` : 'Forge fields' }}</h2>
    <p class="ff-lead">
      Where each field is written and read on {{ forge?.kind === 'gitlab' ? 'GitLab' : 'GitHub' }}. A field left on
      <b>default</b> follows its type; only overrides go into <code>[forges.{{ forgeName }}.fields]</code>.
    </p>

    <p v-if="!forge" class="ff-muted">No forge declared: add one in <router-link to="/settings/sync">Forges</router-link>.</p>
    <template v-else>
      <div class="ff-work">
        <div class="ff-main">
          <div class="gb-table-wrap">
            <table class="gb-table ff-table">
              <thead>
                <tr>
                  <th>Field</th>
                  <th>Lives in</th>
                  <th>Details</th>
                  <th>Values</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in rows" :key="row.name">
                  <td>
                    <code>{{ row.name }}</code><br />
                    <span class="ff-muted">{{ row.type }}</span>
                  </td>
                  <td>
                    <select
                      class="ff-select"
                      :class="{ 'ff-override': row.target !== row.defaultTarget }"
                      :value="row.target"
                      :aria-label="`${row.name} lives in`"
                      @change="setTarget(row, ($event.target as HTMLSelectElement).value)"
                    >
                      <option v-for="t in row.targets" :key="t.id" :value="t.id">{{ defaultMark(row, t.id) }}</option>
                    </select>
                  </td>
                  <td>
                    <select
                      v-if="row.target === 'project'"
                      class="ff-select"
                      :value="row.projectField"
                      :aria-label="`Project field for ${row.name}`"
                      @change="setProjectField(row, ($event.target as HTMLSelectElement).value)"
                    >
                      <option v-for="c in projectColumns(row)" :key="c" :value="c">{{ c }}</option>
                    </select>
                    <fieldset v-else-if="row.target === 'label'" class="ff-as">
                      <legend class="ff-muted">Label form</legend>
                      <label><input type="radio" :checked="row.as !== 'plain'" @change="setAs(row, 'namespaced')" /> namespaced</label>
                      <label><input type="radio" :checked="row.as === 'plain'" @change="setAs(row, 'plain')" /> plain</label>
                    </fieldset>
                    <span v-else class="ff-muted">—</span>
                  </td>
                  <td>
                    <span v-if="row.renamed" class="gb-linkbtn" @click="renaming = renaming === row.name ? '' : row.name">{{ row.renamed }} renamed</span>
                    <span v-if="row.renamed && row.aliases"> · </span>
                    <span v-if="row.aliases" class="ff-muted">{{ row.aliases }} alias{{ row.aliases === 1 ? '' : 'es' }}</span>
                    <span v-if="!row.renamed && !row.aliases" class="ff-muted">—</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <details v-if="renaming" class="ff-renames" :open="true">
            <summary>The names of <code>{{ renaming }}</code> on {{ forgeName }}</summary>
            <div class="ff-rename-rows">
              <div v-for="v in renamingValues" :key="v.value" class="ff-rename">
                <code>{{ v.value }}</code>
                <input :value="v.on" class="fs-input" :placeholder="v.value" spellcheck="false" :aria-label="`${v.value}'s name on ${forgeName}`" @input="setRename(v.value, inputValue($event))" />
              </div>
            </div>
            <p class="gb-hint">Empty: the value keeps its own name.</p>
          </details>

          <ConfigSaveBar
            section="forge-fields"
            :dirty="draft.dirty.value"
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

          <section v-if="migration" class="ff-migrate" aria-labelledby="ff-mig">
            <h3 id="ff-mig" class="h5">Migrate <code>{{ migration.field }}</code>: {{ migration.from }} → {{ migration.to }}</h3>
            <template v-if="migrationPlans">
              <ol>
                <li v-for="p in migrationPlans" :key="p.forge">
                  <b>{{ p.forge }}</b> ({{ p.repo }}): <b>{{ p.issues }} issues</b> carry it{{ p.labels ? `, ${p.labels} labels come off` : '' }} — {{ p.note }}
                </li>
              </ol>
              <p class="ff-muted">Nothing is written until you run it. Same as <code>git board sync --migrate {{ migration.field }}</code>.</p>
              <div class="ff-actions">
                <button type="button" class="epfl-btn epfl-btn-primary" :disabled="!!busy" @click="runMigration">
                  {{ busy === 'migrate' ? 'Migrating…' : `Run the migration` }}
                </button>
                <button type="button" class="epfl-btn epfl-btn-secondary" @click="migration = null">Not now</button>
              </div>
            </template>
            <p v-else class="ff-muted">Counting…</p>
          </section>
        </div>

        <aside class="ff-aside" aria-label="What this writes">
          <h3 class="h5">Written to <code>.git-board.toml</code></h3>
          <pre class="ff-toml">{{ toml || '— nothing: every field follows its type default' }}</pre>
          <p class="ff-muted">blue = an override. Everything else is the type's default, unwritten.</p>
        </aside>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { previewFieldMigration, runFieldMigration, type ConfigField, type ConfigModel, type MigratePlan } from '@/api/board';
import { configModel, configModelError, inputValue, loadConfigModel, SectionDraft } from '@/configmodel';
import { fieldTarget, targetsFor, targetLabels, type ForgeMapping, type ForgeModel } from '@/fieldtargets';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';
import { message } from '@/issue';

const route = useRoute();
const forgeName = computed(() => {
  const n = typeof route.params.name === 'string' ? route.params.name : '';
  if (n) return n;
  return configModel.value?.forges[0]?.name ?? ''; // a bare address: the first forge
});
const forge = computed<ForgeModel | undefined>(() => configModel.value?.forges.find((f) => f.name === forgeName.value));

const roles = computed(() => Object.keys(configModel.value?.roles ?? {}));
const projectNumber = computed(() => configModel.value?.github_project?.number ?? 0);

interface Row {
  name: string;
  type: string;
  target: string;
  defaultTarget: string;
  targets: { id: string; label: string }[];
  projectField: string;
  as: string;
  renamed: number;
  aliases: number;
}

const rows = computed<Row[]>(() => {
  const m = configModel.value ?? undefined;
  const f = forge.value;
  if (!m || !f) return [];
  const out: Row[] = [];
  for (const name of m.field_order) {
    const cf: ConfigField | undefined = m.fields[name];
    if (!cf) continue;
    const target = fieldTarget(f, m.fields, name, roles.value, projectNumber.value);
    const fm: ForgeMapping = f.fields?.[name] ?? {};
    out.push({
      name,
      type: `${cf.type || 'guessed'} · ${cf.single ? 'one' : 'several'}`,
      target,
      defaultTarget: fieldTarget({ ...f, fields: {} }, m.fields, name, roles.value, projectNumber.value),
      targets: targetsFor(f.kind, cf.type || 'select', name),
      projectField: fm.field ?? name.charAt(0).toUpperCase() + name.slice(1),
      as: fm.as ?? '',
      renamed: Object.keys(fm.values ?? {}).length,
      aliases: Object.keys(cf.aliases ?? {}).length,
    });
  }
  return out;
});

// the draft: the forge's mappings as the forms hold them
interface ForgeValue {
  set: Record<string, ForgeMapping>;
  remove: string[];
}
const draft = new SectionDraft<ForgeValue>(
  'forge-fields',
  (m) => {
    const f = m.forges.find((x) => x.name === forgeName.value);
    const set: Record<string, ForgeMapping> = {};
    for (const [name, fm] of Object.entries(f?.fields ?? {})) set[name] = { ...fm };
    return { set, remove: [] };
  },
  (a, b) => JSON.stringify(a.set) !== JSON.stringify(b.set) || a.remove.length > 0,
  (value) => ({ forge: forgeName.value, set: cleanMappings(value.set), remove: value.remove }),
);

function cleanMappings(set: Record<string, ForgeMapping>): Record<string, ForgeMapping> {
  const out: Record<string, ForgeMapping> = {};
  for (const [name, m] of Object.entries(set)) {
    const clean: ForgeMapping = { to: m.to };
    if (m.as) clean.as = m.as;
    if (m.field) clean.field = m.field;
    if (m.values && Object.keys(m.values).length) clean.values = m.values;
    if (clean.to) out[name] = clean;
  }
  return out;
}

function patch(name: string, m: Partial<ForgeMapping>) {
  const set = { ...draft.value.value?.set };
  const cur: ForgeMapping = { ...set[name], ...m };
  // a mapping equal to the type default is no override: it goes
  set[name] = cur;
  if (draft.value.value) draft.value.value = { ...draft.value.value, set, remove: draft.value.value.remove };
  askMigration(name, cur);
}

function setTarget(row: Row, to: string) {
  patch(row.name, { to });
}

function setProjectField(row: Row, field: string) {
  patch(row.name, { field: field === row.name ? undefined : field });
}

function setAs(row: Row, as: string) {
  patch(row.name, { as: as === 'plain' ? 'plain' : undefined });
}

// the values' renames, as the details expander holds them
const renaming = ref('');
const renamingValues = computed(() => {
  const m = forge.value?.fields?.[renaming.value]?.values ?? {};
  const options = configModel.value?.fields[renaming.value]?.options ?? [];
  return options.map((v) => ({ value: v, on: m[v] ?? '' }));
});

function setRename(value: string, on: string) {
  const name = renaming.value;
  const set = { ...draft.value.value?.set };
  const values = { ...set[name]?.values, [value]: on };
  if (!on || on === value) delete values[value];
  set[name] = { ...set[name], values: Object.keys(values).length ? values : undefined };
  if (draft.value.value) draft.value.value = { ...draft.value.value, set, remove: draft.value.value.remove };
}

// the project column's options: the mapping's, else the known names, else
// the field's own — the file doesn't list the Project's fields
function projectColumns(row: Row): string[] {
  const known: Record<string, string> = { status: 'Status', prio: 'Priority', mod: 'Module', est: 'Est. hours', sprint: 'Sprint', moscow: 'MoSCoW', milestone: 'Milestone' };
  const own = known[row.name] ?? row.name.charAt(0).toUpperCase() + row.name.slice(1);
  return [row.projectField, ...(own === row.projectField ? [] : [own])];
}

function defaultMark(row: Row, id: string): string {
  return targetLabels[id] + (id === row.defaultTarget ? ' (default)' : '');
}

// the migration: a target changed, the preview counts before anything
const migration = ref<{ field: string; from: string; to: string } | null>(null);
const migrationPlans = ref<MigratePlan[] | null>(null);
const busy = ref<'' | 'migrate'>('');

function askMigration(name: string, m: ForgeMapping) {
  if (!m.to || m.to === 'label') {
    migration.value = null;
    return;
  }
  const from = forge.value?.fields?.[name]?.to ?? 'label';
  migration.value = { field: name, from: targetLabels[from] ?? from, to: targetLabels[m.to] ?? m.to };
  migrationPlans.value = null;
  previewFieldMigration(name).then((r) => (migrationPlans.value = r.plans)).catch(() => (migrationPlans.value = []));
}

const valid = computed(() => !!forge.value);
const state = computed(() => {
  const n = Object.keys(draft.value.value?.set ?? {}).length;
  return n ? `${n} override${n === 1 ? '' : 's'}` : 'every field on its default';
});

const toml = computed(() => {
  const m = forge.value;
  if (!m?.fields || !Object.keys(m.fields).length) return '';
  const lines = [`[forges.${m.name}.fields]`];
  for (const [name, fm] of Object.entries(m.fields)) {
    const parts = [`to = "${fm.to}"`];
    if (fm.as) parts.push(`as = "${fm.as}"`);
    if (fm.field) parts.push(`field = "${fm.field}"`);
    lines.push(`${name} = { ${parts.join(', ')} }`);
  }
  for (const [name, fm] of Object.entries(m.fields)) {
    if (!fm.values || !Object.keys(fm.values).length) continue;
    lines.push('', `[forges.${m.name}.fields.${name}.values]`);
    for (const [v, on] of Object.entries(fm.values)) lines.push(`${v} = "${on}"`);
  }
  return lines.join('\n');
});

const modelError = configModelError;

function reset() {
  if (configModel.value) draft.reset(configModel.value);
  migration.value = null;
}

async function save() {
  await draft.save();
}

const runMigration = () =>
  ((busy.value = 'migrate'),
    runFieldMigration(migration.value?.field ?? '')
      .then((r) => {
        if (r.error) draft.saveError.value = r.error;
      })
      .catch((e) => (draft.saveError.value = message(e)))
      .finally(() => (busy.value = '')));

onMounted(() => {
  void draft.load();
  void loadConfigModel();
});
</script>

<style scoped>
.ff h2, .ff h3, .ff p { margin: 0; }
.ff-crumb { font-size: .8125rem; color: var(--fg-muted); }
.ff-crumb a { color: inherit; }
.ff-lead { max-width: 46rem; color: var(--fg-muted); }
.ff-muted { color: var(--fg-muted); }
.ff-work { display: flex; gap: 1.5rem; align-items: flex-start; }
.ff-main { flex: 1; min-width: 0; display: grid; gap: 1rem; }
.ff-aside { flex: none; width: 20rem; display: grid; gap: .375rem; }
.ff-table code { font-weight: 700; }
.ff-select { min-height: 2.25rem; padding: 0 .5rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.ff-select:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
.ff-override { border: 2px solid var(--primary); background: var(--bg-accent, var(--bg)); font-weight: 700; }
.ff-as { display: flex; gap: .75rem; align-items: center; margin: 0; padding: 0; border: 0; }
.ff-as legend { float: left; margin-right: .5rem; padding: 0; font-size: .8125rem; }
.ff-as label { display: inline-flex; gap: .25rem; align-items: center; font-size: .8125rem; }
.ff-renames { display: grid; gap: .5rem; padding: .75rem 1rem; border: 1px solid var(--border); }
.ff-renames summary { cursor: pointer; font-weight: 700; color: var(--fg-muted); }
.ff-rename-rows { display: grid; gap: .375rem; }
.ff-rename { display: flex; gap: .625rem; align-items: center; }
.ff-rename code { flex: none; width: 9rem; }
.ff-rename input { max-width: 16rem; }
.ff-migrate { display: grid; gap: .75rem; padding: 1rem 1.25rem; border: var(--border-w) solid var(--fg); }
.ff-migrate ol { margin: 0; padding-left: 1.25rem; display: grid; gap: .25rem; }
.ff-actions { display: flex; gap: .5rem; flex-wrap: wrap; }
.ff-toml { margin: 0; padding: .875rem 1rem; border: 1px solid var(--border); background: var(--bg-subtle); font-size: .8125rem; line-height: 1.55; overflow-x: auto; }
@media (max-width: 59.99rem) {
  .ff-work { flex-direction: column-reverse; }
  .ff-aside { width: auto; }
}
</style>
