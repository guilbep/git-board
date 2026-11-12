<!-- Settings › Sync, its GitHub Project: the bridge (git board project)
     between the Project's fields and the labels. The Sync page's Check
     and Sync read and run it with the forges; this is its part of their
     report: what the file lacks for it (the fit), what a sync changes, and
     its conflicts, the fields both sides changed since they last agreed,
     held on both until someone keeps one side (api.md, routes 52 and 53). -->
<template>
  <section class="pj" aria-labelledby="pj-title">
    <h3 id="pj-title" class="h5">GitHub Project</h3>
    <p class="pj-lead">
      The board is the truth; the Project mirrors the fields you give it, kept in step with the labels: a field changed on one
      side since both agreed takes that side; changed on both, it is a conflict, left alone until you keep one.
    </p>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert"><b>Not done.</b> {{ error }}</p>
    <p v-if="status?.problem" class="epfl-alert epfl-alert-danger" role="alert">[github-project]: {{ status.problem }}</p>
    <p v-else-if="status && !status.project" class="epfl-alert epfl-alert-info" role="note">
      <span class="pj-cfg-lead">No GitHub Project is configured. Give it the owner and the number (the one in the Project's URL), and the board writes it here.</span>
      <form class="pj-cfg" @submit.prevent="configure">
        <label class="pj-cfg-label">Owner
          <input v-model="owner" class="epfl-input" type="text" placeholder="EPFL-ENAC" required />
        </label>
        <label class="pj-cfg-label">Number
          <input v-model.number="number" class="epfl-input" type="number" min="1" placeholder="40" required />
        </label>
        <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="saving || !owner || !number">{{ saving ? 'Saving…' : 'Configure' }}</button>
      </form>
      <p v-if="cfgError" class="epfl-alert epfl-alert-danger" role="alert"><b>Not saved.</b> {{ cfgError }}</p>
    </p>
    <template v-else-if="status">
      <p class="pj-meta">
        <b>{{ last?.title || status.project }}</b> <span class="pj-muted">({{ status.project }})</span>
        <template v-if="last"> · {{ last.applied ? 'Ran' : 'Checked' }} {{ when(last.checked_at) }} · {{ runWords(last) }}</template>
      </p>
      <p class="pj-meta">
        The Project holds {{ status.fields.join(', ') || 'no field' }}: on GitHub these live in its columns, not in labels.
        <template v-if="!status.stated">
          This is the default, and <code>.git-board.toml</code> doesn't say it yet.
          <button type="button" class="gb-linkbtn" :disabled="saving" @click="writeFields">{{ saving ? 'Writing…' : 'Write it in the file' }}</button>
        </template>
      </p>
      <p v-if="cfgError && status.project" class="epfl-alert epfl-alert-danger" role="alert"><b>Not saved.</b> {{ cfgError }}</p>
      <p v-if="!status.write" class="epfl-alert epfl-alert-info" role="note">
        This board doesn't write to the Project: a pull only updates the labels here, and keeping git-board's value isn't available.
        <button type="button" class="gb-linkbtn" :disabled="saving" @click="setWrite(true)">{{ saving ? 'Saving…' : 'Let the board write to the Project' }}</button>
      </p>
      <p v-else class="pj-muted">
        The board writes its values to the Project.
        <button type="button" class="gb-linkbtn" :disabled="saving" @click="setWrite(false)">Stop writing to it</button>
      </p>
      <!-- what the file lacks for the Project: before Sync, open on the first check -->
      <ProjectFit v-if="last && !last.applied && last.fit.length" :key="last.checked_at" :run="last" @fitted="fitted" />
      <p v-if="wrote" class="pj-muted" role="status">{{ wrote }}</p>
      <p v-for="l in last?.unmapped_by_choice ?? []" :key="l" class="pj-muted">{{ l }}.</p>
      <span v-if="busy" class="pj-muted" role="status">{{ busy }}</span>
      <p v-if="!last && !wrote" class="pj-muted">Not checked yet. Check reads it with the forges: a big Project takes a few seconds and costs GitHub API points.</p>

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
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { configureProject, getConfig, getProject, message, patchConfig, resolveProject, type Fitted, type ProjectConflict, type ProjectStatus } from '@/api';
import { onLive } from '@/live';
import { safeHref } from '@/links';
import { issueRef, plural, runWords, shown } from '@/projectview';
import ProjectFit from '@/components/ProjectFit.vue';

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

// the fit written: the check it came from is dropped, the next reads the file
const wrote = ref('');
function fitted(res: ProjectStatus & { fitted: Fitted }) {
  status.value = res;
  wrote.value = `Wrote ${plural(res.fitted.changes, 'change')} to .git-board.toml. Check again to see the sync with it.`;
}

// [github-project] written as project configure writes it (route 58):
// the owner, the number, and the fields the Project holds
const owner = ref('');
const number = ref<number | null>(null);
const saving = ref(false);
const cfgError = ref('');
async function save(o: string, n: number) {
  saving.value = true;
  cfgError.value = '';
  try {
    status.value = await configureProject(o, n);
  } catch (e) {
    cfgError.value = message(e);
  } finally {
    saving.value = false;
  }
}
const configure = () => save(owner.value.trim(), number.value ?? 0);
// the default fields, stated in the file: the same Project configured again
function writeFields() {
  const [o, n] = (status.value?.project ?? '').split('/');
  void save(o ?? '', Number(n));
}

// write = true under [github-project], written from the file's version
async function setWrite(on: boolean) {
  saving.value = true;
  cfgError.value = '';
  try {
    const f = await getConfig();
    await patchConfig({ 'github-project.write': on }, f.version);
    status.value = await getProject();
  } catch (e) {
    cfgError.value = message(e);
  } finally {
    saving.value = false;
  }
}

function resolve(c: ProjectConflict, keep: 'git-board' | 'project') {
  void run(() => resolveProject(issueRef(c), c.field, keep), `Keeping ${keep === 'project' ? 'the Project’s' : 'git-board’s'} value…`);
}

onMounted(() => void run(getProject, ''));
// a whole check or sync ended, here or elsewhere: its Project step is the last run
const off = onLive((e) => {
  if (e.kind !== 'sync') return;
  wrote.value = '';
  void run(getProject, '');
}, { own: true });
onBeforeUnmount(off);
</script>

<style scoped>
.pj { display: grid; gap: .75rem; }
.pj h3 { margin: 0; }
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
.pj-cfg-lead { display: block; margin-bottom: .5rem; }
.pj-cfg { display: flex; flex-wrap: wrap; align-items: end; gap: .5rem; }
.pj-cfg-label { display: grid; gap: .25rem; font-size: .875rem; }
</style>
