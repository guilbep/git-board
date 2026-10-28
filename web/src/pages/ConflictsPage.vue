<!-- Sync conflicts: the fields git-board and the GitHub Project both changed
     since they last agreed (git board project sync holds them, nothing is
     overwritten), one card each, to keep one side. Design: the Claude
     Design canvas's "Conflicts" artboard. -->
<template>
  <AppHeader :views="viewNames" />

  <main class="page">
    <div class="page-head">
      <div class="epfl-eyebrow">Sync with the GitHub Project{{ status?.project ? ` ${status.project}` : '' }}</div>
      <h1 class="h3 page-title">{{ heading }}</h1>
      <p class="lede">
        git-board and the GitHub Project both changed these fields since they last agreed. Each side keeps
        its own value until someone picks one; every other field keeps syncing.
      </p>
      <p v-if="last" class="page-meta">
        Last checked {{ when(last.checked_at) }}
        <template v-if="pending"> · a sync would {{ pending }}</template>
        <template v-if="last.warnings"> · {{ plural(last.warnings, 'value warning') }}</template>
      </p>
      <div class="actions">
        <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="!!busy || !status?.project" @click="run(checkSync)">
          Check the Project
        </button>
        <button type="button" class="epfl-btn epfl-btn-primary" :disabled="!!busy || !status?.project" @click="syncNow">
          {{ status?.write ? 'Sync fields' : 'Pull fields' }}
        </button>
        <span v-if="busy" class="busy" role="status">{{ busy }}</span>
      </div>
    </div>

    <div v-if="error" class="epfl-alert epfl-alert-danger notice" role="alert"><b>Not done.</b> {{ error }}</div>
    <div v-if="status && !status.project" class="epfl-alert epfl-alert-warning notice">
      No GitHub Project is configured: run <code>git board project configure</code>.
    </div>
    <div v-else-if="status && !status.write" class="epfl-alert epfl-alert-info notice">
      This board doesn't write to the Project (no <code>write = true</code> under <code>[github-project]</code>):
      a sync only updates the labels here, and keeping git-board's value isn't available.
    </div>

    <p v-if="status?.project && !last" class="state">
      Not checked yet. Reading the Project takes a few seconds on a big one, and costs GitHub API points, so it
      happens only when you ask.
    </p>
    <p v-else-if="last && !last.conflicts.length" class="state">No conflicts: both sides agree on every field they share.</p>

    <ul v-if="last?.conflicts.length" class="conflicts">
      <li v-for="c in last.conflicts" :key="`${c.number} ${c.field}`" class="conflict">
        <div class="conflict-head">
          <a class="conflict-number" :href="c.url" target="_blank" rel="noopener noreferrer">#{{ c.number }}</a>
          <span class="conflict-title">{{ c.title }}</span>
          <span class="conflict-field">Field <b>{{ displayName(c.field) }}</b></span>
        </div>
        <dl class="sides">
          <div class="side">
            <dt>git-board</dt>
            <dd>{{ show(c.field, c.local) }}</dd>
          </div>
          <div class="side">
            <dt>Project</dt>
            <dd>{{ show(c.field, c.project) }}</dd>
          </div>
          <div class="side side-base">
            <dt>Last agreed</dt>
            <dd>{{ c.has_base ? show(c.field, c.base) : 'never' }}</dd>
          </div>
        </dl>
        <div class="conflict-actions">
          <button
            type="button"
            class="epfl-btn epfl-btn-secondary epfl-btn-sm"
            :disabled="!!busy || !status?.write"
            @click="resolve(c, 'git-board')"
          >
            Keep git-board's ({{ show(c.field, c.local) }})
          </button>
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!!busy" @click="resolve(c, 'project')">
            Keep the Project's ({{ show(c.field, c.project) }})
          </button>
        </div>
      </li>
    </ul>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { applySync, checkSync, getViews, resolveConflict, type Conflict, type SyncStatus } from '@/api/board';
import { displayName } from '@/fields';
import { loadSyncStatus, syncStatus as status } from '@/sync';
import { pendingWords, plural } from '@/ways';
import AppHeader from '@/components/AppHeader.vue';

const viewNames = ref<string[]>([]);
const busy = ref(''); // what's running, in words
const error = ref('');

const last = computed(() => status.value?.last ?? null);
const heading = computed(() => {
  const n = last.value?.conflicts.length;
  if (n === undefined) return 'Sync conflicts';
  return n ? `${plural(n, 'conflict')}, nothing overwritten` : 'No conflicts';
});

const pending = computed(() => (last.value ? pendingWords(last.value) : ''));

function show(field: string, value: string): string {
  if (!value) return 'not set';
  return field === 'est' ? `${value}h` : value;
}

function when(iso: string): string {
  return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}

async function run(action: () => Promise<SyncStatus>, doing = 'Reading the Project…') {
  busy.value = doing;
  error.value = '';
  try {
    status.value = await action();
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    busy.value = '';
  }
}

function syncNow() {
  const what = status.value?.write ? 'Sync fields brings the Project’s values in as labels, and writes yours to the Project' : 'Pull fields brings the Project’s values in as labels; nothing is written to the Project';
  if (window.confirm(`${what}. Conflicts stay held. Go on?`)) {
    void run(applySync, 'Syncing…');
  }
}

function resolve(c: Conflict, keep: 'git-board' | 'project') {
  void run(() => resolveConflict(c.number, c.field, keep), `Keeping ${keep === 'project' ? "the Project's" : "git-board's"} value…`);
}

onMounted(async () => {
  void loadSyncStatus();
  try {
    viewNames.value = Object.keys(await getViews());
  } catch {
    // the header just shows no views
  }
});
</script>

<style scoped>
.page { max-width: 64rem; padding: var(--space-4) var(--gutter) var(--space-5); }

.page-head { margin-bottom: var(--space-4); }
.page-title { margin: 0 0 var(--space-2); }
.lede { max-width: 44rem; margin: 0 0 var(--space-2); color: var(--fg-muted); }
.page-meta { margin: 0; font-size: var(--fs-sm); color: var(--fg-muted); }
.actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); margin-top: var(--space-3); }
.busy { font-size: var(--fs-sm); color: var(--fg-muted); }

.notice { margin-bottom: var(--space-3); }
.state { margin: 0; padding: var(--space-5) var(--space-3); text-align: center; color: var(--fg-muted); border: var(--border-w) solid var(--border); }

.conflicts { display: grid; gap: var(--space-3); margin: 0; padding: 0; list-style: none; }
.conflict { padding: var(--space-3) var(--space-4) var(--space-4); border: var(--border-w) solid var(--border); border-left: 0.25rem solid var(--primary); }

.conflict-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--space-1) var(--space-3); margin-bottom: var(--space-3); }
.conflict-number { font-family: var(--font-mono); color: var(--fg); }
.conflict-title { flex: 1 1 16rem; font-weight: var(--w-bold); overflow-wrap: anywhere; }
.conflict-field { font-size: var(--fs-sm); color: var(--fg-muted); }
.conflict-field b { color: var(--fg); }

.sides { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--space-3); margin: 0 0 var(--space-3); }
.side { padding: var(--space-2) var(--space-3); background: var(--bg-subtle); }
.side dt { font-size: var(--fs-xs); color: var(--fg-muted); text-transform: uppercase; letter-spacing: 0.04em; }
.side dd { margin: var(--space-1) 0 0; font-weight: var(--w-bold); overflow-wrap: anywhere; }
.side-base { background: none; border: var(--border-w) dashed var(--border); }
.side-base dd { font-weight: var(--w-normal); }

.conflict-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); }

@media (max-width: 47.99rem) {
  .page { padding: var(--space-3) var(--space-3) var(--space-5); }
  .sides { grid-template-columns: minmax(0, 1fr); }
}
</style>
