<!-- This one issue synced with the forges, now, without a full run: a dry
     run first; when it would write nothing to a forge (a refresh) the real
     run follows at once, else it says what it would send and asks. -->
<template>
  <div class="gb-isync">
    <div class="gb-isync-actions">
      <button v-if="waiting || !onForges" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!!busy" @click="start">
        <GbIcon name="sync" small /> {{ onForges ? 'Send now' : 'Put it on the forges' }}
      </button>
      <button v-if="onForges" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!!busy" @click="start">
        <GbIcon name="sync" small /> Refresh from forges
      </button>
      <span v-if="busy" class="gb-hint" role="status">{{ busy }}</span>
    </div>

    <div v-if="plan" class="gb-isync-plan" role="region" aria-label="What the sync would do">
      <p class="gb-isync-title">This would change the forges:</p>
      <ul>
        <li v-for="[name, t] in rows(plan)" :key="name"><b>{{ label(name) }}</b>: {{ tallyWords(t, true) }}</li>
      </ul>
      <p v-if="rows(plan).length > 1" class="gb-hint">What comes in from one forge is then sent to the others.</p>
      <div class="gb-isync-actions">
        <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!!busy" @click="run">Send</button>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!!busy" @click="plan = null">Cancel</button>
      </div>
    </div>

    <div v-if="done" class="gb-isync-done" :class="{ 'gb-isync-err': done.errors }" role="status">
      <p class="gb-isync-title">{{ done.errors ? 'Synced with errors' : 'Synced just now' }}</p>
      <ul>
        <li v-for="[name, t] in rows(done)" :key="name"><b>{{ label(name) }}</b>: {{ tallyWords(t, false) }}</li>
      </ul>
      <details v-if="done.errors">
        <summary>Details</summary>
        <pre>{{ rows(done).flatMap(([, t]) => t.lines).join('\n') }}</pre>
      </details>
    </div>
    <p v-if="error" class="gb-isync-done gb-isync-err" role="alert">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { syncIssue, type BridgeTally, type IssueSyncResult } from '@/api/board';
import { forgeStatus } from '@/forges';
import { message } from '@/issue';
import { tallyWords } from '@/issuesync';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ id: string; waiting: number; onForges: boolean }>();
const emit = defineEmits<{ synced: [] }>();

const busy = ref('');
const plan = ref<IssueSyncResult | null>(null);
const done = ref<IssueSyncResult | null>(null);
const error = ref('');

const rows = (r: IssueSyncResult): [string, BridgeTally][] => Object.entries(r.bridges).sort(([a], [b]) => a.localeCompare(b));
const names: Record<string, string> = { github: 'GitHub', gitlab: 'GitLab', gitea: 'Gitea' };
function label(bridge: string): string {
  const target = forgeStatus.value?.bridges.find((b) => b.name === bridge)?.target;
  return target ? `${names[target] ?? target} (${bridge})` : bridge;
}

async function start() {
  error.value = '';
  done.value = null;
  plan.value = null;
  busy.value = 'Checking the forges…';
  try {
    const dry = await syncIssue(props.id, true);
    if (dry.errors) {
      done.value = dry;
      return;
    }
    // writes to a forge, or brings in what the real run then mirrors to
    // the other forges (a dry run can't count those): say what, and ask
    const incoming = Object.values(dry.bridges).some((t) => t.from.issues + t.from.changed + t.from.comments + t.from.edits + t.from.changes > 0);
    if (dry.writes || (incoming && Object.keys(dry.bridges).length > 1)) {
      plan.value = dry;
      return;
    }
    await real();
  } catch (e) {
    error.value = `Not synced: ${message(e)}`;
  } finally {
    busy.value = '';
  }
}

async function run() {
  try {
    await real();
  } catch (e) {
    error.value = `Not synced: ${message(e)}`;
  } finally {
    busy.value = '';
  }
}

async function real() {
  busy.value = 'Syncing…';
  plan.value = null;
  done.value = await syncIssue(props.id, false);
  emit('synced');
}
</script>

<style scoped>
.gb-isync { display: grid; gap: .5rem; margin-top: .75rem; }
.gb-isync-actions { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; }
.gb-isync-plan, .gb-isync-done { display: grid; gap: .375rem; margin: 0; padding: .625rem .75rem; border: 1px solid var(--border); background: var(--bg-subtle); font-size: .875rem; }
.gb-isync-plan { border-left: 3px solid var(--warning); background: var(--warning-bg); }
.gb-isync-done { border-left: 3px solid var(--success); }
.gb-isync-err { border-left-color: var(--danger); background: var(--danger-bg); color: var(--danger-fg); }
.gb-isync-title { margin: 0; font-weight: 700; }
.gb-isync ul { margin: 0; padding-left: 1.125rem; }
.gb-isync pre { margin: .375rem 0 0; white-space: pre-wrap; font-size: .75rem; }
</style>
