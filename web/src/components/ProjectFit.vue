<!-- The fit (docs/prd/config-fit.md): what .git-board.toml lacks for the
     Project the team already has, from the last check. A change git-board
     derived is a ticked box; another is a question, with no answer chosen.
     Apply shows the diff a dry run answers, then writes it, and the
     Project is checked again. A change adds: nothing of the file goes. -->
<template>
  <details class="pf" :open="run.first" data-testid="project-fit">
    <summary class="pf-line">{{ fitLine(run) }}</summary>
    <p class="pf-muted">
      Each change adds to <code>.git-board.toml</code>: no option, kind, sprint or line of yours is removed. A question you leave
      unanswered writes nothing. Pull and Sync still run without them, with the warnings.
    </p>
    <ul class="pf-list">
      <li v-for="p in run.fit" :key="p.id" class="pf-item">
        <label v-if="p.ticked" class="pf-row">
          <input v-model="taken[p.id]" type="checkbox" :disabled="busy" />
          <span>{{ p.says }}</span>
        </label>
        <label v-else class="pf-row">
          <span>{{ p.says }}</span>
          <select v-model="answers[p.id]" class="epfl-input pf-select" :disabled="busy" :aria-label="`${p.says}: choose`">
            <option value="">Not answered</option>
            <option v-for="c in p.choices ?? []" :key="c" :value="c">{{ answerWords(p.field, c) }}</option>
          </select>
        </label>
        <span v-if="p.clears" class="pf-muted">clears {{ plural(p.clears, 'warning') }}</span>
        <span v-if="p.note" class="pf-muted">{{ p.note }}</span>
      </li>
    </ul>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert"><b>Not written.</b> {{ error }}</p>
    <div v-if="fitted" class="pf-diff-wrap">
      <pre class="pf-diff" aria-label="Changes to .git-board.toml">{{ fitted.diff || 'Nothing to write.' }}</pre>
      <div class="pf-actions">
        <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !fitted.diff" @click="write">{{ busy ? 'Writing…' : `Write ${plural(fitted.changes, 'change')}` }}</button>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="fitted = null">Cancel</button>
      </div>
    </div>
    <div v-else class="pf-actions">
      <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="preview">{{ busy ? 'Reading…' : 'Apply to .git-board.toml' }}</button>
    </div>
  </details>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import { fitProject, message, type Fitted, type ProjectRun, type ProjectStatus } from '@/api';
import { answerWords, fitLine, plural } from '@/projectview';

const props = defineProps<{ run: ProjectRun }>();
const emit = defineEmits<{ fitted: [status: ProjectStatus & { fitted: Fitted }] }>();

// a ticked change is taken unless unticked; a question once answered
const taken = reactive<Record<string, boolean>>(Object.fromEntries(props.run.fit.filter((p) => p.ticked).map((p) => [p.id, true])));
const answers = reactive<Record<string, string>>(Object.fromEntries(props.run.fit.filter((p) => !p.ticked).map((p) => [p.id, ''])));
const busy = ref(false);
const error = ref('');
const fitted = ref<Fitted | null>(null);

function chosen() {
  const given = Object.fromEntries(Object.entries(answers).filter(([, a]) => a));
  const take = props.run.fit.filter((p) => (p.ticked ? taken[p.id] : given[p.id])).map((p) => p.id);
  return { take, given };
}

async function send(dry: boolean) {
  busy.value = true;
  error.value = '';
  try {
    const { take, given } = chosen();
    const res = await fitProject(take, given, dry);
    fitted.value = dry ? res.fitted : null;
    if (!dry) emit('fitted', res);
  } catch (e) {
    error.value = message(e);
    fitted.value = null;
  } finally {
    busy.value = false;
  }
}
const preview = () => send(true);
const write = () => send(false);
</script>

<style scoped>
.pf { padding: .75rem; border: var(--border-w) solid var(--border-subtle); background: var(--warning-bg); }
.pf-line { font-weight: var(--w-bold); cursor: pointer; }
.pf-muted { margin: .5rem 0 0; color: var(--fg-muted); font-size: .875rem; }
.pf-list { display: grid; gap: .5rem; margin: .75rem 0; padding: 0; list-style: none; }
.pf-item { display: flex; flex-wrap: wrap; align-items: baseline; gap: .25rem .75rem; }
.pf-item .pf-muted { margin: 0; }
.pf-row { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; }
.pf-row input { accent-color: var(--primary-hover); }
.pf-select { width: auto; }
.pf-diff { max-height: 24rem; margin: 0 0 .5rem; padding: .5rem; overflow: auto; background: var(--bg); font-size: .8125rem; }
.pf-actions { display: flex; flex-wrap: wrap; gap: .5rem; }
</style>
