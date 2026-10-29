<!-- Step 4, Tests (the canvas's C4): the suites and checks the base
     branch's .git-board.toml declares, the touched ones ticked, a run in a
     clean checkout of the branch's last commit whose result is a signed
     line in git notes (docs/adr/0001). For a bug, the two runs: red at the
     commit adding the test, green at the head. The checks gate Mark ready
     for review on the server. Without any, it proposes them from the
     project's lefthook.yml, Makefile or package.json. -->
<template>
  <h3 class="gb-ws-title">{{ title }}</h3>
  <p class="gb-note">{{ ruleLine }}</p>
  <p v-if="error" class="gb-save-error" role="alert"><GbIcon name="error" small /> {{ error }}</p>

  <template v-if="t">
    <div v-if="runs" class="gb-runs">
      <div v-for="(r, i) in runs" :key="i" class="gb-box">
        <span><span :class="['gb-pill', r.pill.color && `gb-c-${r.pill.color}`]">{{ r.pill.text }}</span> {{ r.line }}</span>
        <span class="gb-note">{{ r.note }}</span>
      </div>
    </div>

    <p v-if="!t.suites.length && !t.checks.length" class="gb-note">
      No suites or checks declared in <span class="gb-mono">.git-board.toml</span> on <span class="gb-mono">{{ t.base }}</span>.
    </p>
    <section v-for="g in groups" :key="g.key" class="gb-box" :aria-labelledby="`t-${g.key}`">
      <div class="gb-box-head">
        <h4 :id="`t-${g.key}`" class="gb-box-title">{{ g.title }}</h4>
      </div>
      <div class="gb-tests-wrap">
        <table class="gb-tests">
          <tbody>
            <tr v-for="s in g.rows" :key="s.name">
              <td><input v-model="picked" type="checkbox" :value="s.name" :disabled="!runnable(s) || running" :aria-label="`Run ${s.name}`" /></td>
              <td><b>{{ s.name }}</b> <span class="gb-mono gb-note">{{ s.run.join(' ') }}</span></td>
              <td>{{ whereLabel(s) }}</td>
              <td class="gb-note">{{ t.running.includes(s.name) ? 'running…' : reason(s) }}</td>
              <td>
                <button v-if="s.head?.result" type="button" :class="['gb-pill', 'gb-pill-btn', pill(s.head).color && `gb-c-${pill(s.head).color}`]" :aria-expanded="log?.name === s.name" @click="showLog(s.name, s.head.result.log)">
                  {{ pill(s.head).text }}<span class="gb-sr"> at the head, show the log</span>
                </button>
                <span v-else class="gb-note">{{ pill(s.head).text }}</span>
                <span v-if="s.head?.result" class="gb-note"> {{ ran(s.head) }}</span>
                <span v-if="t.test_commit && s.at_test?.verdict" class="gb-note"> · {{ pill(s.at_test).text }} at {{ t.test_commit.slice(0, 7) }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="g.key === 'checks'" class="gb-note">{{ t.gate.proven ? 'Mark ready for review: open.' : `Mark ready for review waits: ${t.gate.reason}.` }}</p>
    </section>
    <pre v-if="log" class="gb-log" :aria-label="`The log of ${log.name}`">{{ log.text || '(no output)' }}</pre>

    <div v-if="t.suites.length || t.checks.length" class="gb-cycle-actions">
      <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || running || !picked.length || !t.head" @click="run('head')">
        Run the selected<template v-if="t.head"> at {{ t.head.slice(0, 7) }}</template>
      </button>
      <button v-if="t.test_commit" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || running || !picked.length" @click="run('test')">
        Run at {{ t.test_commit.slice(0, 7) }}, before the fix
      </button>
      <span v-if="running" class="gb-note" role="status">Running {{ t.running.join(', ') }} in a clean checkout…</span>
      <span v-else-if="t.error" class="gb-save-error" role="alert">{{ t.error }}</span>
    </div>
    <p class="gb-note">The result is git-board’s run, signed with your key, never the agent’s word.</p>

    <section v-if="!t.checks.length" class="gb-box" aria-labelledby="t-setup">
      <div class="gb-box-head">
        <h4 id="t-setup" class="gb-box-title">Checks that gate Mark ready for review</h4>
        <button v-if="!setup" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="propose">Propose from the project</button>
      </div>
      <template v-if="setup">
        <p v-if="!setup.source" class="gb-note">No lefthook.yml, Makefile lint target or package.json script to propose from: write [cycle.checks.&lt;name&gt;] by hand.</p>
        <template v-else>
          <p class="gb-note">From <span class="gb-mono">{{ setup.source }}</span>:</p>
          <ul class="gb-ws-list">
            <li v-for="(s, name) in setup.checks" :key="name"><b>{{ name }}</b> <span class="gb-mono">{{ s.run.join(' ') }}</span><template v-if="s.dir"> in {{ s.dir }}</template></li>
            <li v-for="s in setup.skipped" :key="`skip-${s.name}`" class="gb-note">{{ s.name }} skipped: {{ s.why }}</li>
          </ul>
          <div v-if="Object.keys(setup.checks).length" class="gb-cycle-actions">
            <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || setup.declared" @click="write">Write to .git-board.toml</button>
            <span class="gb-note">They gate once committed on {{ t.base }}. They go in the board’s [cycle] table: a board with one opens every issue on the cycle’s page, for everyone.</span>
          </div>
        </template>
      </template>
      <p v-if="wrote" class="gb-note" role="status">Written. Commit .git-board.toml on {{ t.base }} for them to gate.</p>
    </section>
  </template>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { getChecksSetup, getRunLog, getStepTests, runSuites, writeChecks, type ChecksSetup, type Cycle, type StepTests } from '@/api/board';
import { message } from '@/issue';
import { onLive } from '@/live';
import { bugRuns, pill, preselect, ran, reason, runnable, whereLabel } from '@/steptests';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ id: string; cycle: Cycle }>();

const t = ref<StepTests | null>(null);
const error = ref('');
const busy = ref(false);
const picked = ref<string[]>([]);
const log = ref<{ name: string; text: string } | null>(null);
const setup = ref<ChecksSetup | null>(null);
const wrote = ref(false);

const running = computed(() => !!t.value?.running.length);
const runs = computed(() => (t.value ? bugRuns(t.value) : null));
const groups = computed(() => {
  const g = t.value;
  if (!g) return [];
  return [
    { key: 'suites', title: `Test suites · [cycle.tests] on ${g.base}`, rows: g.suites },
    { key: 'checks', title: `Checks · gate Mark ready for review`, rows: g.checks },
  ].filter((x) => x.rows.length);
});
const title = computed(() => ({ 'failing-test': 'The test that fails before the fix', none: 'No tests for this kind' }[t.value?.rule ?? ''] ?? 'The touched suites, green'));
const ruleLine = computed(
  () => ({ 'failing-test': 'Rule for a bug: a regression test, red first, then green at the head.', none: 'Skipped: this kind needs no test.' }[t.value?.rule ?? ''] ?? 'Rule for a feature: the touched suites green at the head, each criterion naming its test.'),
);

// the ticks are preselected once per head: a reload keeps the user's
let ticked = '';
async function load() {
  try {
    const next = await getStepTests(props.id);
    t.value = next;
    error.value = '';
    const at = `${props.id} ${next.head ?? ''}`;
    if (at !== ticked) {
      ticked = at;
      picked.value = preselect(next);
    }
  } catch (e) {
    error.value = `The tests: ${message(e)}`;
  }
}
watch(() => [props.id, props.cycle.evidence.head], load, { immediate: true });
// a run ends: the server says so (internal/server/checks.go)
const offLive = onLive((e) => e.kind === 'cycle' && void load());
onBeforeUnmount(offLive);

async function act(fn: () => Promise<unknown>) {
  busy.value = true;
  error.value = '';
  try {
    await fn();
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}

const run = (target: 'head' | 'test') =>
  act(async () => {
    await runSuites(props.id, picked.value, target);
    await load();
  });

const showLog = (name: string, hash: string) =>
  log.value?.name === name ? (log.value = null) : act(async () => (log.value = { name, text: await getRunLog(props.id, hash) }));

const propose = () => act(async () => (setup.value = await getChecksSetup()));
const write = () =>
  act(async () => {
    setup.value = await writeChecks();
    wrote.value = true;
  });
</script>
