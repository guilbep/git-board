<!-- Settings › Labels: the [labels] table the project file holds — the
     plain labels the project allows, their colors and descriptions, saved
     through the section API like every form tab — and how the labels the
     project defines (each field's options, the sprints, the kinds) stand
     on each GitHub repository of [repos] (else origin), read through
     GitHub's API with the forge sync's token where git board serve runs.
     Adopt writes GitHub's color and description into the file through the
     same API; Send to GitHub creates and updates them after a preview,
     offered only once every repository was read. Then lint: git board
     lint's checks, and the GitHub labels outside the project's, each
     deleted after a confirm. -->
<template>
  <div class="set-panel lb">
    <h2 class="h4">Labels</h2>
    <p class="lb-lead">
      The project's own plain labels — yes-or-no flags that stack and that no field holds — then how every label the project defines
      stands on GitHub. A label an issue can carry only one of (a priority, a kind) is a field's option, in <router-link to="/settings/fields">Fields</router-link>.
    </p>

    <div class="set-cols">
      <div class="set-forms">
        <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>

        <!-- the [labels] table, as the forms hold it -->
        <section v-if="labelsDraft.value.value" class="lb-own" aria-labelledby="lb-own">
          <h3 id="lb-own" class="h5">The project's plain labels</h3>
          <ConfigRows :rows="labelsDraft.value.value.list" add-label="Add a label" @update="labelsDraft.value.value.list = $event" @add="labelsDraft.value.value.list = [...labelsDraft.value.value.list, '']">
            <template #row="{ row, set }">
              <input :value="row" class="lb-tag" placeholder="needs-design" spellcheck="false" aria-label="A plain label" @input="set(inputValue($event))" />
            </template>
            <template #extra>
              <ConfigPreset label="From the preset" :presets="[labelsPreset]" @pick="labelsDraft.value.value.list = [...labelsDraft.value.value.list, ...labelsPreset.value.list]" />
            </template>
          </ConfigRows>
          <p class="gb-hint">
            The order the issue's head offers them as checkboxes, never as free text. A label with a colon (<code>prio:p1</code>) is a field's value; a kind is the kind field's — both refused.
          </p>
          <details class="lb-more" :open="labelsDraft.value.value.colors.length > 0">
            <summary>Colors <span class="lb-count">{{ labelsDraft.value.value.colors.length }}</span></summary>
            <ConfigRows :rows="labelsDraft.value.value.colors" add-label="Add a color" @update="labelsDraft.value.value.colors = $event" @add="labelsDraft.value.value.colors = [...labelsDraft.value.value.colors, { k: '', v: 'grey' }]">
              <template #row="{ row, set }">
                <div class="lb-pair">
                  <input :value="row.k" class="lb-tag" list="lb-label-names" spellcheck="false" aria-label="Label" @input="set({ ...row, k: inputValue($event) })" />
                  <select :value="row.v" class="lb-tag lb-tiny" aria-label="Color" @change="set({ ...row, v: inputValue($event) })">
                    <option v-for="c in palette" :key="c" :value="c">{{ c }}</option>
                    <option v-if="row.v && !palette.includes(row.v)" :value="row.v">{{ row.v }}</option>
                  </select>
                </div>
              </template>
            </ConfigRows>
            <datalist id="lb-label-names"><option v-for="l in labelsDraft.value.value.list" :key="l" :value="l" /></datalist>
          </details>
          <details class="lb-more" :open="labelsDraft.value.value.descriptions.length > 0">
            <summary>Descriptions <span class="lb-count">{{ labelsDraft.value.value.descriptions.length }}</span></summary>
            <ConfigRows :rows="labelsDraft.value.value.descriptions" add-label="Add a description" @update="labelsDraft.value.value.descriptions = $event" @add="labelsDraft.value.value.descriptions = [...labelsDraft.value.value.descriptions, { k: '', v: '' }]">
              <template #row="{ row, set }">
                <div class="lb-pair">
                  <input :value="row.k" class="lb-tag" list="lb-label-names" spellcheck="false" aria-label="Label" @input="set({ ...row, k: inputValue($event) })" />
                  <input :value="row.v" class="lb-desc" placeholder="what the label means" aria-label="Description" @input="set({ ...row, v: inputValue($event) })" />
                </div>
              </template>
            </ConfigRows>
          </details>
          <ConfigSaveBar
            section="labels"
            :dirty="labelsDraft.dirty.value"
            :saving="labelsDraft.saving.value"
            :saved="labelsDraft.saved.value"
            :valid="true"
            :error="labelsDraft.saveError.value"
            :conflict="labelsDraft.conflict.value"
            :state="`${labelsDraft.value.value.list.length} label${labelsDraft.value.value.list.length === 1 ? '' : 's'}`"
            @save="labelsDraft.save()"
            @discard="resetLabels"
            @reload="labelsDraft.load()"
          />
        </section>

        <p v-if="error" ref="errorEl" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
        <p v-for="e in errors" :key="e.repo" class="epfl-alert epfl-alert-warning" role="alert">{{ e.repo }}: {{ e.message }}</p>
        <p v-if="result" ref="resultEl" class="epfl-alert epfl-alert-success" role="status">{{ result }}</p>
        <p v-if="!d && !error" class="lb-muted">Loading…</p>

        <template v-if="d">
          <p v-if="!d.repos.length" class="lb-muted">No GitHub repository: set <code>[repos]</code> in the project file, or an origin on GitHub.</p>
          <p v-if="!d.rows.length" class="lb-muted">The project file defines no label yet: no field with options, no sprint, no kind, no <code>[labels] list</code>.</p>
          <div v-else class="gb-table-wrap">
            <table class="gb-table">
              <thead>
                <tr>
                  <th>Label</th>
                  <th>Description</th>
                  <th>Open issues</th>
                  <th v-for="r in d.repos" :key="r">{{ r }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in d.rows" :key="row.name">
                  <td><span class="lb-name"><span class="gb-dot" :style="dot(row.color)" />{{ row.name }}</span></td>
                  <td class="lb-muted">{{ row.description }}</td>
                  <td>{{ row.count }}</td>
                  <td v-for="(g, i) in cells(row)" :key="i">
                    <span v-if="!g" class="lb-muted">—</span>
                    <div v-else class="lb-gh">
                      <span :class="`lb-${g.state}`">{{ stateText[g.state] }}</span>
                      <template v-if="g.state === 'color' || g.state === 'description'">
                        <span class="lb-name lb-muted"><span class="gb-dot" :style="dot(g.color)" />{{ g.description || 'No description' }}</span>
                        <button type="button" class="gb-linkbtn" :disabled="!!busy" @click="adopt(row, g)">Adopt</button>
                      </template>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- every namespace found is a field: the labels on the issues
               or a repository that no field's option declares, under
               their field, with the way to declare it -->
          <!-- the leftover migration's preview: counted, then run -->
          <section v-if="migratePlans" class="lb-migrate" aria-label="The migration's preview">
            <h3 class="h5">Migrate <code>{{ migratePlans.field }}</code></h3>
            <ul>
              <li v-for="p in migratePlans.plans" :key="p.forge">
                <b>{{ p.forge }}</b> ({{ p.repo }}): <b>{{ p.issues }} issues</b> carry it{{ p.labels ? `, ${p.labels} labels come off` : '' }} — {{ p.note }}
              </li>
            </ul>
            <p class="lb-muted">Nothing is written until you run it. Same as <code>git board sync --migrate {{ migratePlans.field }}</code>.</p>
            <div class="lb-migrate-actions">
              <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!!migrating" @click="runMigrate">
                {{ migrating ? 'Migrating…' : 'Run the migration' }}
              </button>
              <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="migratePlans = null">Not now</button>
            </div>
          </section>

          <template v-if="d.undeclared.length">
            <section aria-labelledby="lb-undecl">
              <h3 id="lb-undecl" class="h5">On the issues, not declared</h3>
              <p class="lb-muted">Every namespace on an issue is a field. These values exist on the issues or GitHub, but no field's options name them — add the option, or declare the field.</p>
              <div class="gb-table-wrap">
                <table class="gb-table">
                  <thead>
                    <tr>
                      <th>Field</th>
                      <th>Values</th>
                      <th>Open issues</th>
                      <th v-for="r in d.repos" :key="r">{{ r }}</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="g in undeclaredGroups" :key="g.field">
                      <td><code>{{ g.field }}</code><span v-if="!g.declared" class="lb-muted"> · not declared</span></td>
                      <td><span v-for="(v, i) in g.rows" :key="v.name">{{ i ? ', ' : '' }}<span class="lb-name"><span class="gb-dot" :style="dot(v.color)" />{{ v.name }}</span></span></td>
                      <td>{{ g.count }}</td>
                      <td v-for="(r, i) in d.repos" :key="i">
                        <span v-if="g.onRepo[i]" class="lb-same">On GitHub</span>
                        <span v-else class="lb-muted">—</span>
                      </td>
                      <td>
                        <router-link class="gb-linkbtn" :to="g.declared ? `/settings/fields/${g.field}` : '/settings/fields'">
                          {{ g.declared ? 'Add the option in Fields' : 'Declare this field' }}
                        </router-link>
                        <span v-if="g.staleHome && !migrated.includes(g.field)" class="lb-stale">
                          left over from an old mapping —
                          <button type="button" class="gb-linkbtn" @click="askMigrate(g.field)">Migrate</button>
                        </span>
                        <span v-else-if="g.staleHome" class="lb-muted">moved — the labels stay in git-board</span>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </section>
          </template>

          <!-- a repository GitHub didn't answer for: nothing to compare, nothing to send -->
          <template v-if="d.repos.length && d.rows.length && !errors.length">
            <div v-if="plan" class="lb-confirm" role="alert">
              <p v-if="!plan.length"><b>GitHub has every label as the file says.</b></p>
              <template v-else>
                <p><b>Send to GitHub: {{ changes(plan.length) }}</b></p>
                <ul>
                  <li v-for="c in plan" :key="c.repo + c.name"><code>{{ c.repo }}</code> {{ changeText(c) }}</li>
                </ul>
              </template>
              <div class="lb-actions">
                <button v-if="plan.length" type="button" class="epfl-btn epfl-btn-primary" :disabled="!!busy" @click="send">{{ busy === 'send' ? 'Sending…' : `Send ${changes(plan.length)}` }}</button>
                <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="!!busy" @click="plan = null">{{ plan.length ? 'Cancel' : 'Close' }}</button>
              </div>
            </div>
            <div v-else class="lb-actions">
              <button type="button" class="epfl-btn epfl-btn-primary" :disabled="!!busy" @click="preview">{{ busy === 'preview' ? 'Reading GitHub…' : 'Send to GitHub' }}</button>
            </div>
            <p class="gb-hint">Creates the missing labels and updates the colors and descriptions the file sets. A label the file doesn't define is never changed.</p>
          </template>
        </template>

        <section class="lb-lint" aria-labelledby="lb-lint">
          <h3 id="lb-lint" class="h5">Lint</h3>
          <p class="lb-muted">What <code>git board lint</code> finds in the issues, then the GitHub labels outside the project's.</p>
          <p v-if="!lint" class="lb-muted">Loading…</p>
          <template v-else>
            <p v-if="!lint.findings.length" class="lb-muted">No finding.</p>
            <div v-for="g in byCheck(lint.findings)" :key="g.check" class="lb-check">
              <h4 class="lb-check-name">{{ g.check }} · {{ issues(g.findings.length) }}</h4>
              <ul>
                <li v-for="(f, i) in g.findings" :key="i">
                  <template v-if="f.issue_id"><router-link :to="`/issues/${f.issue_id}`">{{ f.title }}</router-link>: </template>{{ f.message }}
                </li>
              </ul>
            </div>

            <h4 class="lb-check-name">On GitHub, outside the project's labels</h4>
            <p v-if="!lint.stray.length" class="lb-muted">None.</p>
            <div v-else class="gb-table-wrap">
              <table class="gb-table">
                <thead>
                  <tr><th>Repository</th><th>Label</th><th>Description</th><th>Issues here</th><th /></tr>
                </thead>
                <tbody>
                  <tr v-for="s in lint.stray" :key="s.repo + s.name">
                    <td><code>{{ s.repo }}</code></td>
                    <td><span class="lb-name"><span class="gb-dot" :style="dot(s.color)" />{{ s.name }}</span></td>
                    <td class="lb-muted">{{ s.description }}</td>
                    <td>{{ s.count }}</td>
                    <td>
                      <div v-if="deleting?.label === s" class="lb-confirm lb-confirm-row" role="alert">
                        <p><b>{{ deleteConfirm(s.repo, s.name, deleting.count) }}</b></p>
                        <div class="lb-actions">
                          <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!!busy" @click="remove(s)">{{ busy === 'delete' ? 'Deleting…' : 'Delete' }}</button>
                          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!!busy" @click="deleting = null">Cancel</button>
                        </div>
                      </div>
                      <button v-else type="button" class="gb-linkbtn" :disabled="!!busy" @click="askDelete(s)">{{ busy === 'count' && asked === s ? 'Counting…' : 'Delete' }}</button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </template>
        </section>
      </div>

      <ConfigRefPanel :sections="['labels']" title="the [labels] table, adopt, lint" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue';
import { deleteLabel, getLabels, getLint, previewFieldMigration, runFieldMigration, saveConfigSection, sendLabels, type ConfigField, type LabelChange, type LabelRow, type Labels, type Lint, type MigratePlan, type StrayLabel } from '@/api/board';
import { fieldTarget } from '@/fieldtargets';
import { labelsPreset } from '@/configpresets';
import { configModel, configModelError, loadConfigModel, SectionDraft, inputValue, plainClone } from '@/configmodel';
import ConfigPreset from '@/components/configforms/ConfigPreset.vue';
import ConfigRows from '@/components/configforms/ConfigRows.vue';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';
import { message } from '@/issue';
import { byCheck, changeText, deleteConfirm, stateText } from '@/labels';

// changed: what a load left, for the Overview's line
const emit = defineEmits<{ changed: [d: Labels] }>();
const d = ref<Labels | null>(null);
const lint = ref<Lint | null>(null);
const plan = ref<LabelChange[] | null>(null);
const deleting = ref<{ label: StrayLabel; count: number } | null>(null);
const asked = ref<StrayLabel | null>(null);
const error = ref('');
const result = ref('');
const busy = ref<'' | 'adopt' | 'preview' | 'send' | 'count' | 'delete'>('');
const errorEl = ref<HTMLElement | null>(null);
const resultEl = ref<HTMLElement | null>(null);
// a repository gh couldn't read, once even when both loads say it
const errors = computed(() => [...new Map([...(d.value?.errors ?? []), ...(lint.value?.errors ?? [])].map((e) => [e.repo, e])).values()]);

const dot = (hex: string) => (hex ? { '--c-dot': `#${hex}` } : undefined);
// a row's state on each repository, none where gh couldn't read it
const cells = (row: LabelRow) => (d.value?.repos ?? []).map((r) => row.github.find((g) => g.repo === r));
const changes = (n: number) => `${n} change${n === 1 ? '' : 's'}`;
const issues = (n: number) => `${n} issue${n === 1 ? '' : 's'}`;

// the undeclared labels, grouped under their field: a declared field's
// extra values (status:ready-for-validation, sprint:13, assignee:guilbep)
// and the fields no table declares (risk, roles), each with its count and
// where it stands on GitHub. A declared field whose forge home is no
// longer a label (assignee in the native assignees) is a mapping's
// leftover: Migrate moves the values (the forge-fields page's preview,
// run here after it).
const undeclaredGroups = computed(() => {
  const m = configModel.value;
  const roles = Object.keys(m?.roles ?? {});
  const projectNumber = m?.github_project?.number ?? 0;
  const groups: { field: string; declared: boolean; staleHome: boolean; rows: LabelRow[]; count: number; onRepo: boolean[] }[] = [];
  for (const row of d.value?.undeclared ?? []) {
    let g = groups.find((x) => x.field === row.field);
    if (!g) {
      const declared = !!m?.fields[row.field];
      const staleHome =
        declared &&
        (m?.forges ?? []).some((f) => {
          const to = fieldTarget(f, m!.fields, row.field, roles, projectNumber);
          return to !== 'label' && to !== 'scoped-label';
        });
      g = { field: row.field, declared, staleHome, rows: [], count: 0, onRepo: (d.value?.repos ?? []).map(() => false) };
      groups.push(g);
    }
    g.rows.push(row);
    g.count += row.count;
    g.onRepo = (d.value?.repos ?? []).map((r) => g.rows.some((row) => row.github.find((x) => x.repo === r)?.state === 'same'));
  }
  return groups;
});

// the leftover's migration: the preview counts, the run moves. What moved
// says so in its row — the labels stay in git-board by design, and
// re-running them is harmless but reads as unfinished.
const migratePlans = ref<{ field: string; plans: MigratePlan[] } | null>(null);
const migrating = ref('');
const migrated = ref<string[]>([]);

async function askMigrate(field: string) {
  migratePlans.value = { field, plans: (await previewFieldMigration(field)).plans };
}

async function runMigrate() {
  if (!migratePlans.value) return;
  migrating.value = migratePlans.value.field;
  try {
    const r = await runFieldMigration(migratePlans.value.field);
    if (r.error) error.value = r.error;
    else migrated.value = [...migrated.value, migratePlans.value.field];
    migratePlans.value = null;
    load();
  } catch (e) {
    error.value = message(e);
  } finally {
    migrating.value = '';
  }
}

async function run(kind: typeof busy.value, fn: () => Promise<void>) {
  busy.value = kind;
  error.value = result.value = '';
  try {
    await fn();
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = '';
  }
  // the outcome shows above the table, the buttons are below it
  await nextTick();
  (errorEl.value ?? resultEl.value)?.scrollIntoView({ block: 'nearest' });
}

function load() {
  getLabels().then(
    (l) => ((d.value = l), emit('changed', l)),
    (e) => (error.value = message(e)),
  );
  getLint().then(
    (l) => (lint.value = l),
    (e) => (error.value = message(e)),
  );
}

// the [labels] table, as the forms hold it: its maps as rows
type Pair = { k: string; v: string };
const modelError = configModelError;
const palette = ['grey', 'blue', 'purple', 'amber', 'orange', 'green', 'red', 'teal', 'pink'];
const toPairs = (m?: Record<string, string>): Pair[] => Object.entries(m ?? {}).map(([k, v]) => ({ k, v }));
const kvOf = (rows: Pair[]): Record<string, string> | undefined => {
  const out = Object.fromEntries(rows.filter((r) => r.k.trim() && r.v.trim()).map((r) => [r.k.trim(), r.v]));
  return Object.keys(out).length ? out : undefined;
};

const labelsDraft = new SectionDraft<{ list: string[]; colors: Pair[]; descriptions: Pair[] }>(
  'labels',
  (m) => ({ list: [...m.labels.list], colors: toPairs(m.labels.colors), descriptions: toPairs(m.labels.descriptions) }),
  (a, b) => JSON.stringify(a) !== JSON.stringify(b),
  (value) => ({ list: value.list.filter((l) => l.trim()), colors: kvOf(value.colors), descriptions: kvOf(value.descriptions) }),
);
function resetLabels() {
  if (configModel.value) labelsDraft.reset(configModel.value);
}

// adopt writes GitHub's color and description into the file through the
// section API: the field's table gets them, or the [labels] one for a
// plain label; the comparison below refreshes
const adopt = (row: LabelRow, g: { color: string; description: string }) =>
  run('adopt', async () => {
    const m = configModel.value;
    if (!m) throw new Error('The project file is not loaded yet.');
    const color = g.color ? (g.color.startsWith('#') ? g.color : `#${g.color}`) : '';
    if (row.field) {
      const f: ConfigField = plainClone(m.fields[row.field] ?? { single: false, options: [] });
      f.colors = { ...(f.colors ?? {}), [row.value]: color };
      if (g.description) f.descriptions = { ...(f.descriptions ?? {}), [row.value]: g.description };
      const r = await saveConfigSection('fields', { set: { [row.field]: f }, remove: [] }, m.hash);
      configModel.value = r.model;
    } else {
      const l = plainClone(m.labels);
      l.colors = { ...(l.colors ?? {}), [row.value]: color };
      if (g.description) l.descriptions = { ...(l.descriptions ?? {}), [row.value]: g.description };
      const r = await saveConfigSection('labels', { list: l.list, colors: l.colors, descriptions: l.descriptions }, m.hash);
      configModel.value = r.model;
      labelsDraft.reset(r.model);
    }
    result.value = `Adopted ${row.name}'s color and description into the file.`;
    load();
  });

const preview = () =>
  run('preview', async () => {
    plan.value = (await sendLabels(true)).changes;
  });

const send = () =>
  run('send', async () => {
    const done = (await sendLabels(false)).changes;
    const failed = done.filter((c) => c.error);
    const first = failed[0];
    plan.value = null;
    if (done.length > failed.length) result.value = `Sent ${changes(done.length - failed.length)} to GitHub.`;
    // ponytail: the first failure only, they mostly share one cause (a token without write access)
    if (first) error.value = `${failed.length} of ${changes(done.length)} failed, the first: ${first.repo} ${first.name}: ${first.error}`;
    load();
  });

const askDelete = (s: StrayLabel) =>
  run('count', async () => {
    asked.value = s;
    deleting.value = { label: s, count: (await deleteLabel(s.repo, s.name, true)).count ?? 0 };
  });

const remove = (s: StrayLabel) =>
  run('delete', async () => {
    await deleteLabel(s.repo, s.name, false);
    deleting.value = null;
    result.value = `Deleted ${s.name} from ${s.repo}.`;
    load();
  });

onMounted(() => {
  load();
  void labelsDraft.load();
});
</script>

<style scoped>
.lb h2, .lb h3, .lb h4, .lb p, .lb ul { margin: 0; }
.lb-lead { max-width: 48rem; color: var(--fg-muted); }
.lb-muted { color: var(--fg-muted); }
.lb-stale { display: block; font-size: .8125rem; color: var(--fg-muted); }
.lb-migrate { display: grid; gap: .5rem; margin: 1rem 0; padding: 1rem 1.25rem; border: 1px solid var(--fg); }
.lb-migrate ul { margin: 0; padding-left: 1.25rem; display: grid; gap: .25rem; }
.lb-migrate-actions { display: flex; gap: .5rem; }
.lb-own { display: grid; gap: .75rem; padding: .75rem 1rem; border: 1px solid var(--border); border-left: 3px solid var(--primary); background: var(--bg); }
.lb-tag { padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.lb-tag:focus, .lb-desc:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
.lb-tiny { max-width: 6rem; }
.lb-desc { flex: 1; min-width: 12rem; padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.lb-pair { display: flex; gap: .5rem; }
.lb-more summary { cursor: pointer; font-size: .875rem; font-weight: 700; color: var(--fg-muted); }
.lb-more summary:hover { color: var(--fg); }
.lb-more { display: grid; gap: .5rem; }
.lb-count { display: inline-block; margin-left: .375rem; padding: 0 .375rem; border: 1px solid var(--border); font-weight: 400; font-size: .75rem; color: var(--fg-muted); }
.lb-name { display: inline-flex; align-items: center; gap: .375rem; white-space: nowrap; }
.lb-gh { display: flex; flex-wrap: wrap; gap: .25rem .5rem; align-items: center; }
.lb-same { color: var(--success-fg); }
.lb-color, .lb-description { color: var(--warning-fg); font-weight: 700; }
.lb-missing { color: var(--danger-fg); font-weight: 700; }
.lb-confirm { display: grid; gap: .75rem; padding: .875rem 1rem; background: var(--warning-bg); }
.lb-confirm ul { padding-left: 1.25rem; }
.lb-confirm-row { padding: .5rem .75rem; gap: .5rem; }
.lb-actions { display: flex; flex-wrap: wrap; gap: .5rem; }
.lb-lint { display: grid; gap: .75rem; padding-top: 1rem; border-top: var(--border-w) solid var(--border); }
.lb-check { display: grid; gap: .25rem; }
.lb-check ul { padding-left: 1.25rem; }
.lb-check-name { font-size: .9375rem; }
</style>
