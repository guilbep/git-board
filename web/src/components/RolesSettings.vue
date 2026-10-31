<!-- Settings › Roles: the project's people by role ([roles]), one login
     each; the tab's draft saves whole, the file's other lines untouched. -->
<template>
  <div class="set-panel">
    <h2 class="h4">Roles</h2>
    <p class="rs-lead">
      A project's people by role: who sets the risk, approves the plan, reviews, signs off. Every issue takes them, unless it sets its own —
      the side panel shows the inherited value greyed, <em>bob · project</em>. A role is a person field of the same name.
    </p>

    <div class="set-cols">
      <div class="set-main">
        <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>
        <p v-else-if="!draft.value.value" class="rs-muted">Loading…</p>

        <template v-else>
          <ConfigRows :rows="rows" add-label="Add a role" @update="rows = $event" @add="rows = [...rows, { role: '', login: '' }]">
            <template #row="{ row }">
              <div class="rs-row">
                <input v-model="row.role" class="rs-input" placeholder="lead-dev" aria-label="Role" spellcheck="false" />
                <span class="rs-arrow">→</span>
                <input v-model="row.login" class="rs-input" placeholder="login" aria-label="Who holds the role" spellcheck="false" />
              </div>
            </template>
          </ConfigRows>
          <p class="gb-hint">
            The name is a field's: lower-case letters, digits and <code>-</code>. One login each; a role declared here is a
            <code>person</code> field, so <code>lead-dev:@me</code> finds the issues that take you as lead dev while the project names you.
          </p>

          <ConfigSaveBar
            section="roles"
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
        </template>


      </div>

      <ConfigRefPanel :sections="['roles']" title="what a role does" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { SectionDraft, configModel, configModelError } from '@/configmodel';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
import ConfigRows from '@/components/configforms/ConfigRows.vue';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';

type RoleRow = { role: string; login: string };
type Roles = Record<string, string>;

const modelError = configModelError;

const draft = new SectionDraft<Roles>(
  'roles',
  (m) => m.roles,
  (a, b) => JSON.stringify(a) !== JSON.stringify(b),
  (value) => value,
);

// the rows, and back: the draft is the roles map, the rows are its pairs
const rows = ref<RoleRow[]>([]);
watch(
  () => draft.value.value,
  (v) => {
    if (v) rows.value = Object.entries(v).map(([role, login]) => ({ role, login }));
  },
  { immediate: true },
);

const valid = computed(() => rows.value.every((r) => /^[a-z0-9-]*$/.test(r.role)));
const state = computed(() => `${rows.value.filter((r) => r.role).length} role${rows.value.length === 1 ? '' : 's'}`);

function reset() {
  if (configModel.value) draft.reset(configModel.value);
}

async function save() {
  const roles: Roles = {};
  for (const r of rows.value) {
    if (r.role.trim() && r.login.trim()) roles[r.role.trim()] = r.login.trim();
  }
  draft.value.value = roles;
  await draft.save();
}

onMounted(() => void draft.load());
</script>

<style scoped>
.set-panel { display: grid; gap: .75rem; container-type: inline-size; }
/* the forms left, the reference right, like the old editor; one column when narrow */
.set-cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: .75rem; align-items: start; }
.set-main { display: grid; gap: .75rem; min-width: 0; }
@container (max-width: 52rem) { .set-cols { grid-template-columns: minmax(0, 1fr); } }
h2, p { margin: 0; }
.rs-lead { max-width: 42rem; color: var(--fg-muted); }
.rs-muted { color: var(--fg-muted); }
.rs-row { display: flex; align-items: center; gap: .5rem; }
.rs-input { flex: 1; max-width: 14rem; padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.rs-input:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
.rs-arrow { color: var(--fg-muted); }
</style>
