<!-- Settings › Cycle: the [cycle] table — on, and the board walks the
     stages; the stages' statuses (guessed from their names until set),
     the roles that hold the gates, the kinds' rules, and the suites:
     [cycle.tests.<name>] run at step 4, [cycle.checks.<name>] gate Mark
     ready for review. "Propose from the project" reads lefthook.yml, the
     Makefile and package.json and fills the checks with what they run
     (GET /api/checks/setup). Off, every [cycle.*] table goes and the
     issues open on the classic page. -->
<template>
  <div class="set-panel cs">
    <h2 class="h4">Cycle</h2>
    <p class="cs-lead">
      The project's way of working, as stages every issue walks: ready, in progress, in review, validation, done. On, the
      board is a kanban of them and every issue has a next step; off, the issues open on the classic page.
    </p>

    <div class="set-cols">
      <div class="set-forms">
        <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>
        <p v-else-if="!draft.value.value" class="cs-muted">Loading…</p>

        <template v-else>
          <label class="cs-on">
            <input type="checkbox" :checked="on" @change="setOn(($event.target as HTMLInputElement).checked)" />
            <span>Walk the cycle — the board's issues take the stages' kanban, the cycle page opens them</span>
          </label>

          <template v-if="on">
            <div class="cs-grid">
              <label class="cs-field">
                <span>Ready (the first stage)</span>
                <input :value="draft.value.value.cycle.ready" class="cs-input" placeholder="guessed: ready" spellcheck="false" aria-label="The ready status" @input="set({ ready: ($event.target as HTMLInputElement).value })" />
              </label>
              <label class="cs-field">
                <span>In progress</span>
                <input :value="(draft.value.value.cycle['in-progress'] ?? []).join(', ')" class="cs-input" placeholder="guessed from the names" spellcheck="false" aria-label="The in-progress statuses, comma-separated" @input="set({ 'in-progress': list(inputValue($event)) })" />
              </label>
              <label class="cs-field">
                <span>In review</span>
                <input :value="(draft.value.value.cycle['in-review'] ?? []).join(', ')" class="cs-input" spellcheck="false" aria-label="The in-review statuses, comma-separated" @input="set({ 'in-review': list(inputValue($event)) })" />
              </label>
              <label class="cs-field">
                <span>Validation</span>
                <input :value="(draft.value.value.cycle.validation ?? []).join(', ')" class="cs-input" spellcheck="false" aria-label="The validation statuses, comma-separated" @input="set({ validation: list(inputValue($event)) })" />
              </label>
            </div>
            <p class="gb-hint">
              Comma-separated status names; empty takes the guess from the names' words (ready, wip, review, validation…). The
              last validation status closes the issue.
            </p>

            <div class="cs-grid">
              <label v-for="g in gates" :key="g.key" class="cs-field">
                <span>{{ g.label }}</span>
                <input :value="draft.value.value.cycle[g.key]" class="cs-input" :placeholder="`a role of [roles], else anyone`" spellcheck="false" :aria-label="g.label" @input="set({ [g.key]: ($event.target as HTMLInputElement).value })" />
              </label>
              <label class="cs-field">
                <span>Plans live in</span>
                <input :value="draft.value.value.cycle.plans" class="cs-input" placeholder="docs/src/implementation-plans" spellcheck="false" aria-label="Where the plans live" @input="set({ plans: ($event.target as HTMLInputElement).value })" />
              </label>
            </div>

            <h3 class="h5">Checks — they gate Mark ready for review</h3>
            <div class="cs-tools">
              <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="proposing" @click="propose">
                {{ proposing ? 'Reading the project…' : 'Propose from the project' }}
              </button>
              <span v-if="proposalNote" class="gb-hint">{{ proposalNote }}</span>
            </div>
            <ConfigRows :rows="suiteRows('checks')" add-label="Add a check" @update="setSuites('checks', $event)" @add="addSuite('checks')">
              <template #row="{ row, set: setRow }">
                <div class="cs-suite">
                  <input :value="row.name" class="cs-input cs-name" placeholder="make ci" aria-label="The check's name" @input="setRow({ ...row, name: inputValue($event) })" />
                  <input :value="(row.suite.run ?? []).join(' ')" class="cs-input cs-run" placeholder="the command's words, space-separated" aria-label="What runs" @input="setRow({ ...row, suite: { ...row.suite, run: words(inputValue($event)) } })" />
                  <select :value="row.suite.where ?? 'worktree'" class="cs-input cs-where" aria-label="Where it runs" @change="setRow({ ...row, suite: { ...row.suite, where: inputValue($event) } })">
                    <option value="worktree">in the worktree</option>
                    <option value="ci">in CI</option>
                  </select>
                </div>
              </template>
            </ConfigRows>

            <h3 class="h5">Tests — they run at the validate step</h3>
            <ConfigRows :rows="suiteRows('tests')" add-label="Add a test suite" @update="setSuites('tests', $event)" @add="addSuite('tests')">
              <template #row="{ row, set: setRow }">
                <div class="cs-suite">
                  <input :value="row.name" class="cs-input cs-name" placeholder="unit" aria-label="The suite's name" @input="setRow({ ...row, name: inputValue($event) })" />
                  <input :value="(row.suite.run ?? []).join(' ')" class="cs-input cs-run" placeholder="the command's words, space-separated" aria-label="What runs" @input="setRow({ ...row, suite: { ...row.suite, run: words(inputValue($event)) } })" />
                  <select :value="row.suite.where ?? 'worktree'" class="cs-input cs-where" aria-label="Where it runs" @change="setRow({ ...row, suite: { ...row.suite, where: inputValue($event) } })">
                    <option value="worktree">in the worktree</option>
                    <option value="ci">in CI</option>
                  </select>
                </div>
              </template>
            </ConfigRows>

            <h3 class="h5">Kinds' rules</h3>
            <ConfigRows :rows="kindRows" add-label="Add a kind" @update="kindRows = $event" @add="kindRows = [...kindRows, { k: 'bug', v: 'failing-test' }]">
              <template #row="{ row, set: setRow }">
                <div class="cs-pair">
                  <input :value="row.k" class="cs-input cs-opt" placeholder="bug" spellcheck="false" aria-label="The kind" @input="setRow({ ...row, k: inputValue($event) })" />
                  <select :value="row.v" class="cs-input cs-opt" aria-label="The rule" @change="setRow({ ...row, v: inputValue($event) })">
                    <option value="failing-test">failing-test — a red test justifies it</option>
                    <option value="per-criterion">per-criterion — the criteria say done</option>
                    <option value="no-tests">no-tests — none needed</option>
                  </select>
                </div>
              </template>
            </ConfigRows>

          </template>
          <p v-else class="gb-hint">
            Off: the file holds no [cycle] table, the issues open on the classic page. Saving removes every [cycle.*] table —
            including the tests and checks.
          </p>
          <!-- outside the v-if: turning the cycle off is saved here too -->
          <ConfigSaveBar
            section="cycle"
            :dirty="draft.dirty.value"
            :saving="draft.saving.value"
            :saved="draft.saved.value"
            :valid="valid"
            :error="draft.saveError.value"
            :conflict="draft.conflict.value"
            :state="state"
            @save="draft.save()"
            @discard="reset"
            @reload="draft.load()"
          />
        </template>


      </div>

      <ConfigRefPanel :sections="['cycle']" title="the cycle's stages, the suites" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { getChecksSetup, type ConfigSuite } from '@/api/board';
import { SectionDraft, configModel, configModelError, inputValue } from '@/configmodel';
import { message } from '@/issue';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
import ConfigRows from '@/components/configforms/ConfigRows.vue';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';

type Pair = { k: string; v: string };
type SuiteRow = { name: string; suite: ConfigSuite };
type Cycle = {
  ready: string;
  'in-progress': string[];
  'in-review': string[];
  validation: string[];
  plans: string;
  risk: string;
  plan: string;
  review: string;
  spec: string;
  kinds: Pair[];
  tests: SuiteRow[];
  checks: SuiteRow[];
};

const modelError = configModelError;
const gates = [
  { key: 'risk', label: 'Risk is set by' },
  { key: 'plan', label: 'The plan is approved by' },
  { key: 'review', label: 'The review is held by' },
  { key: 'spec', label: "The description's verdict by" },
] as const;

const toPairs = (m?: Record<string, string>): Pair[] => Object.entries(m ?? {}).map(([k, v]) => ({ k, v }));
const kvOf = (rows: Pair[]): Record<string, string> | undefined => {
  const out = Object.fromEntries(rows.filter((r) => r.k.trim()).map((r) => [r.k.trim(), r.v]));
  return Object.keys(out).length ? out : undefined;
};
const toSuiteRows = (m?: Record<string, ConfigSuite>): SuiteRow[] => Object.entries(m ?? {}).map(([name, suite]) => ({ name, suite }));
const suitesOf = (rows: SuiteRow[]): Record<string, ConfigSuite> | undefined => {
  const out = Object.fromEntries(rows.filter((r) => r.name.trim() && (r.suite.run?.length ?? 0) > 0).map((r) => [r.name.trim(), r.suite]));
  return Object.keys(out).length ? out : undefined;
};

const draft = new SectionDraft<{ on: boolean; cycle: Cycle }>(
  'cycle',
  (m) => ({
    on: m.cycle.on,
    cycle: {
      ready: m.cycle.ready ?? '',
      'in-progress': [...(m.cycle['in-progress'] ?? [])],
      'in-review': [...(m.cycle['in-review'] ?? [])],
      validation: [...(m.cycle.validation ?? [])],
      plans: m.cycle.plans ?? '',
      risk: m.cycle.risk ?? '',
      plan: m.cycle.plan ?? '',
      review: m.cycle.review ?? '',
      spec: m.cycle.spec ?? '',
      kinds: toPairs(m.cycle.kinds),
      tests: toSuiteRows(m.cycle.tests),
      checks: toSuiteRows(m.cycle.checks),
    },
  }),
  (a, b) => a.on !== b.on || JSON.stringify(a.cycle) !== JSON.stringify(b.cycle),
  (value) => {
    if (!value.on) return { on: false };
    const c = value.cycle;
    return {
      on: true,
      // the keys this form doesn't show (test-files, criteria, steps,
      // signoffs, signers, plans-in-pr) go back as the file has them: the
      // writer drops a key left out
      cycle: {
        ...configModel.value?.cycle,
        ready: c.ready || undefined,
        'in-progress': c['in-progress'].filter((s) => s.trim()),
        'in-review': c['in-review'].filter((s) => s.trim()),
        validation: c.validation.filter((s) => s.trim()),
        plans: c.plans || undefined,
        risk: c.risk || undefined,
        plan: c.plan || undefined,
        review: c.review || undefined,
        spec: c.spec || undefined,
        kinds: kvOf(c.kinds),
        tests: suitesOf(c.tests),
        checks: suitesOf(c.checks),
      },
    };
  },
);

const on = computed(() => draft.value.value?.on ?? false);

function set(patch: Partial<Cycle> & Record<string, unknown>) {
  if (draft.value.value) draft.value.value = { on: true, cycle: { ...draft.value.value.cycle, ...patch } };
}

function setOn(v: boolean) {
  if (draft.value.value) draft.value.value = { on: v, cycle: draft.value.value.cycle };
}

// list reads a comma-separated input back
function list(line: string): string[] {
  return (line || '').split(',').map((s) => s.trim());
}

// words splits a command line the way an argv holds it
function words(line: string): string[] {
  return line.trim() ? line.trim().split(/\s+/) : [];
}

const kindRows = computed({
  get: () => draft.value.value?.cycle.kinds ?? [],
  set: (rows) => set({ kinds: rows }),
});

function suiteRows(family: 'tests' | 'checks'): SuiteRow[] {
  return draft.value.value?.cycle[family] ?? [];
}
function setSuites(family: 'tests' | 'checks', rows: SuiteRow[]) {
  set({ [family]: rows });
}
function addSuite(family: 'tests' | 'checks') {
  const rows = suiteRows(family);
  setSuites(family, [...rows, { name: '', suite: { run: [] } }]);
}

// "Propose from the project": what lefthook.yml, the Makefile and
// package.json run, as [cycle.checks] entries. The proposal's toml is its
// own write (POST /api/checks/setup); here its checks land in the draft,
// to save with the rest of the section.
const proposing = ref(false);
const proposalNote = ref('');

async function propose() {
  proposing.value = true;
  proposalNote.value = '';
  try {
    const setup = await getChecksSetup();
    if (setup.declared) {
      proposalNote.value = '[cycle.checks] has entries already; edit them here.';
      return;
    }
    if (!setup.checks || Object.keys(setup.checks).length === 0) {
      proposalNote.value = 'Nothing to propose: no lefthook pre-commit, Makefile target or package.json script runs as a check.';
      return;
    }
    setSuites(
      'checks',
      Object.entries(setup.checks).map(([name, suite]) => ({ name, suite: { ...suite, where: suite.where ?? 'worktree' } })),
    );
    proposalNote.value = `${Object.keys(setup.checks).length} check${Object.keys(setup.checks).length === 1 ? '' : 's'} proposed from ${setup.source} — review and save.`;
  } catch (e) {
    proposalNote.value = message(e);
  } finally {
    proposing.value = false;
  }
}

const valid = computed(() => {
  const c = draft.value.value?.cycle;
  if (!c) return false;
  const bare = (s: string) => /^[A-Za-z0-9_-]*$/.test(s.trim());
  return [...c.tests, ...c.checks].every((s) => bare(s.name)) && c.kinds.every((k) => bare(k.k));
});
const state = computed(() => {
  const c = draft.value.value?.cycle;
  if (!c) return '';
  return `${c.tests.length} test suite${c.tests.length === 1 ? '' : 's'} · ${c.checks.length} check${c.checks.length === 1 ? '' : 's'}`;
});

function reset() {
  if (configModel.value) draft.reset(configModel.value);
}

onMounted(() => void draft.load());
</script>

<style scoped>
.set-panel { display: grid; gap: .75rem; container-type: inline-size; }
/* the forms left, the reference right, like the old editor; one column when narrow */
.set-cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: .75rem; align-items: start; }
.set-forms { display: grid; gap: .75rem; min-width: 0; }
@container (max-width: 52rem) { .set-cols { grid-template-columns: minmax(0, 1fr); } }
h2, h3, p { margin: 0; }
.cs-lead { max-width: 42rem; color: var(--fg-muted); }
.cs-muted { color: var(--fg-muted); }
.cs-on { display: flex; align-items: center; gap: .5rem; font-size: .9375rem; }
.cs-grid { display: flex; gap: .75rem; flex-wrap: wrap; }
.cs-field { display: grid; gap: .25rem; font-size: .8125rem; }
.cs-field > span { color: var(--fg-muted); }
.cs-input { padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.cs-input:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
.cs-tools { display: flex; align-items: center; gap: .75rem; flex-wrap: wrap; }
.cs-suite { display: flex; gap: .5rem; flex-wrap: wrap; }
.cs-name { width: 10rem; }
.cs-run { flex: 1; min-width: 12rem; }
.cs-where { flex: none; }
.cs-pair { display: flex; gap: .5rem; }
.cs-opt { width: 14rem; }
</style>
