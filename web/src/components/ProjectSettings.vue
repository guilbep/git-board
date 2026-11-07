<!-- Settings › GitHub Project: the bridge (git board project) between the
     Project's fields and the labels. Its last run, a check, a pull or a
     sync, and its conflicts: the fields both sides changed since they last
     agreed, held on both until someone keeps one side (api.md, routes 52
     and 53). Only a button reads the Project. -->
<template>
  <div class="set-panel pj">
    <h2 class="h4">GitHub Project</h2>
    <p class="pj-lead">
      The board is the truth; the Project mirrors the fields you give it, kept in step with the labels: a field changed on one
      side since both agreed takes that side; changed on both, it is a conflict, left alone until you keep one.
    </p>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert"><b>Not done.</b> {{ error }}</p>
    <p v-if="status?.problem" class="epfl-alert epfl-alert-danger" role="alert">[github-project]: {{ status.problem }}</p>
    <p v-else-if="status && !status.project" class="epfl-alert epfl-alert-info" role="note">
      No GitHub Project is configured: <code>git board project configure --owner &lt;org&gt; --project &lt;number&gt;</code>.
    </p>
    <template v-else-if="status">
      <p class="pj-meta">
        <b>{{ last?.title || status.project }}</b> <span class="pj-muted">({{ status.project }})</span>
        <template v-if="last"> · {{ last.applied ? 'Ran' : 'Checked' }} {{ when(last.checked_at) }} · {{ runWords(last) }}</template>
      </p>
      <p v-if="!status.write" class="epfl-alert epfl-alert-info" role="note">
        This board doesn't write to the Project (no <code>write = true</code> under <code>[github-project]</code>): a pull only updates the
        labels here, and keeping git-board's value isn't available.
      </p>
      <div class="pj-actions">
        <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="!!busy" @click="run(checkProject, 'Reading the Project…')">Check the Project</button>
        <button type="button" class="epfl-btn epfl-btn-primary" :disabled="!!busy" @click="apply('labels')">Pull fields</button>
        <button v-if="status.write" type="button" class="epfl-btn epfl-btn-primary" :disabled="!!busy" @click="apply('both')">Sync fields</button>
        <span v-if="busy" class="pj-muted" role="status">{{ busy }}</span>
      </div>
      <p v-if="!last" class="pj-muted">Not checked yet. Reading a big Project takes a few seconds and costs GitHub API points, so it happens when you ask.</p>

      <section v-if="last" aria-labelledby="pj-conflicts">
        <h3 id="pj-conflicts" class="pj-h">{{ last.conflicts.length ? plural(last.conflicts.length, 'conflict') + ', nothing overwritten' : 'No conflicts' }}</h3>
        <ul v-if="last.conflicts.length" class="pj-list">
          <li v-for="c in last.conflicts" :key="`${c.url} ${c.field}`" class="pj-conflict">
            <div class="pj-head">
              <a :href="safeHref(c.url)" target="_blank" rel="noopener noreferrer">#{{ c.number }}</a>
              <span class="pj-title">{{ c.title }}</span>
              <span class="pj-muted">Field <b>{{ c.field }}</b></span>
            </div>
            <dl class="pj-sides">
              <div><dt>git-board</dt><dd>{{ shown(c.local) }}</dd></div>
              <div><dt>Project</dt><dd>{{ shown(c.project) }}</dd></div>
              <div class="pj-muted"><dt>Last agreed</dt><dd>{{ c.has_base ? shown(c.base) : 'never' }}</dd></div>
            </dl>
            <div class="pj-actions">
              <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!!busy || !status.write" @click="resolve(c, 'git-board')">Keep git-board's ({{ shown(c.local) }})</button>
              <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!!busy" @click="resolve(c, 'project')">Keep the Project's ({{ shown(c.project) }})</button>
            </div>
          </li>
        </ul>
      </section>

      <section v-if="last && last.changes.length" aria-labelledby="pj-changes">
        <h3 id="pj-changes" class="pj-h">{{ last.applied ? 'What it changed' : 'What a sync would change' }}</h3>
        <ul class="pj-list pj-changes">
          <li v-for="ch in last.changes" :key="ch.url">
            <a :href="safeHref(ch.url)" target="_blank" rel="noopener noreferrer">#{{ ch.number }}</a> {{ ch.title }}
            <span v-for="l in ch.added" :key="'+' + l" class="pj-add">+{{ l }}</span>
            <span v-for="l in ch.removed" :key="'-' + l" class="pj-del">−{{ l }}</span>
            <span v-for="w in ch.writes" :key="'w' + w" class="pj-write">→ Project: {{ w }}</span>
            <span v-for="w in ch.warnings" :key="'!' + w" class="pj-warn">! {{ w }}</span>
          </li>
        </ul>
      </section>
      <p v-if="last?.unmatched.length" class="pj-muted">{{ last.unmatched.length }} item{{ last.unmatched.length === 1 ? '' : 's' }} of the Project match no issue here: pull requests, drafts, or issues a forge sync hasn't brought yet.</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { applyProject, checkProject, getProject, message, resolveProject, type ProjectConflict, type ProjectStatus } from '@/api';
import { safeHref } from '@/links';
import { issueRef, plural, runWords, shown } from '@/projectview';

const status = ref<ProjectStatus | null>(null);
const busy = ref('');
const error = ref('');
const last = computed(() => status.value?.last ?? null);

const when = (iso: string) => new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });

async function run(action: () => Promise<ProjectStatus>, doing: string) {
  busy.value = doing;
  error.value = '';
  try {
    status.value = await action();
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = '';
  }
}

function apply(side: 'labels' | 'both') {
  const what = side === 'both' ? 'Sync brings the Project’s values in as labels, and writes yours to the Project' : 'Pull brings the Project’s values in as labels; nothing is written to the Project';
  if (window.confirm(`${what}. Conflicts stay held. Go on?`)) void run(() => applyProject(side), side === 'both' ? 'Syncing…' : 'Pulling…');
}

function resolve(c: ProjectConflict, keep: 'git-board' | 'project') {
  void run(() => resolveProject(issueRef(c), c.field, keep), `Keeping ${keep === 'project' ? 'the Project’s' : 'git-board’s'} value…`);
}

onMounted(() => void run(getProject, ''));
</script>

<style scoped>
.pj { display: grid; gap: .75rem; }
.pj-lead, .pj-meta { margin: 0; }
.pj-muted { color: var(--fg-muted); font-size: .875rem; }
.pj-actions { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; }
.pj-h { margin: .5rem 0 .25rem; font-size: .9375rem; font-weight: var(--w-bold); }
.pj-list { display: grid; gap: .5rem; margin: 0; padding: 0; list-style: none; }
.pj-conflict { display: grid; gap: .5rem; padding: .75rem; border: var(--border-w) solid var(--border-subtle); }
.pj-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: .5rem; }
.pj-title { flex: 1; min-width: 0; font-weight: var(--w-bold); }
.pj-sides { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: .5rem; margin: 0; }
.pj-sides dt { font-size: .75rem; color: var(--fg-muted); }
.pj-sides dd { margin: 0; }
.pj-changes li { display: flex; flex-wrap: wrap; gap: .25rem .5rem; font-size: .875rem; }
.pj-add { color: var(--success-fg, var(--fg)); }
.pj-del { color: var(--danger-fg, var(--fg)); }
.pj-write { color: var(--info-fg); }
.pj-warn { color: var(--warning-fg, var(--fg-muted)); }
</style>
