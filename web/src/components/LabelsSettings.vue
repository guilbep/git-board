<!-- Settings › Labels: the labels the project defines (each field's
     options, the sprints, the kinds) with their color, description and
     open issues, and how each GitHub repository of [repos] (else origin)
     has them, read through GitHub's API with the forge sync's token where
     git board serve runs. Adopt writes GitHub's color and description into
     the Project file tab's draft; Send to GitHub creates and updates them
     after a preview, offered only once every repository was read. Then lint:
     git board lint's checks, and the GitHub labels outside the project's,
     each deleted after a confirm. -->
<template>
  <div class="set-panel lb">
    <h2 class="h4">Labels</h2>
    <p class="lb-lead">
      The labels the <router-link to="/settings/project-file">project file</router-link> defines, and how GitHub has them.
      Colors and descriptions are set in the file; a hex color like <code>"#d73a4a"</code> paints the dot.
    </p>
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
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { adoptLabel, deleteLabel, getLabels, getLint, sendLabels, type LabelChange, type LabelRow, type Labels, type Lint, type StrayLabel } from '@/api/board';
import { configText } from '@/configdraft';
import { message } from '@/issue';
import { byCheck, changeText, deleteConfirm, stateText } from '@/labels';

// changed: what a load left, for the Overview's line
const emit = defineEmits<{ changed: [d: Labels] }>();
const router = useRouter();
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

// adopt writes GitHub's color and description into the project file, as
// the Project file tab holds it, and opens that tab to review and save
const adopt = (row: LabelRow, g: { color: string; description: string }) =>
  run('adopt', async () => {
    configText.value = (await adoptLabel(configText.value, row, g)).text;
    await router.push('/settings/project-file');
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

onMounted(load);
</script>

<style scoped>
.lb h2, .lb h3, .lb h4, .lb p, .lb ul { margin: 0; }
.lb-lead { max-width: 48rem; color: var(--fg-muted); }
.lb-muted { color: var(--fg-muted); }
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
