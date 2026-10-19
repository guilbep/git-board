<!-- A release (internal/server/releases.go releaseDetail): its tag, what it
     is and where it runs, its notes, the issues it carries, its version's
     candidates, the conversation on its record; beside them where it runs
     per target, its images and its record. Yank… and Promote (a candidate
     of the environment before last) act on it. The canvas's release board. -->
<template>
  <AppHeader :views="viewNames" active="" />
  <main class="gb-page">
    <nav aria-label="Breadcrumb" class="gb-rel-crumb">
      <router-link to="/releases" class="gb-rel-back"><GbIcon name="back" small />Releases</router-link>
      <span aria-hidden="true" class="gb-time">/</span>
      <span aria-current="page" class="gb-time">{{ tag }}</span>
    </nav>

    <div class="gb-titlerow gb-rel-title">
      <h1 class="gb-h1">{{ tag }}</h1>
      <span v-if="badge" :class="['epfl-badge', badge.cls]">{{ badge.text }}</span>
      <div v-if="row" class="gb-titlerow-actions">
        <button v-if="record && !record.yanked" type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy" @click="yanking = true">Yank…</button>
        <button v-if="promote" type="button" class="epfl-btn epfl-btn-primary" :disabled="busy" @click="startPromote"><GbIcon name="forward" />Promote to {{ promote.to }} as {{ promote.final }}</button>
      </div>
    </div>

    <p v-if="!detail && !error" class="gb-hint" role="status">Fetching the release and the overlays' history…</p>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
    <p v-if="errors.top" class="epfl-alert epfl-alert-danger" role="alert"><b>Not started.</b> {{ errors.top }}</p>

    <template v-if="detail && row">
      <p class="gb-env-sub gb-rel-meta">
        <span>Commit <span class="gb-mono gb-fg">{{ sha7(row.tag.commit) }}</span></span>
        <span>cut <template v-if="author">by <b>{{ author }}</b></template> {{ on(row.tag.date, true) }}</span>
        <span v-if="row.beforeGitBoard" class="gb-tag">before git-board</span>
        <span v-if="record?.previous">after <router-link :to="releasePath(record.previous)">{{ record.previous }}</router-link></span>
        <span><b>{{ plural(detail.issues.length, 'issue') }}</b></span>
        <span v-if="promote">Promoting adds the tag <span class="gb-mono gb-fg">{{ promote.final }}</span> to this same commit: no new build.</span>
      </p>

      <ReleaseJob v-if="data?.job && (data.job.state === 'running' || watched)" :job="data.job" :names="repoNames" class="gb-rel-job" />

      <div class="gb-cols gb-rel-cols">
        <div class="gb-maincol">
          <section aria-labelledby="notes">
            <div class="gb-side-head">
              <h2 id="notes" class="gb-h2">Notes</h2>
              <button v-if="record && !editing" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="editNotes"><GbIcon name="edit" small />Edit notes</button>
            </div>
            <template v-if="editing">
              <div class="gb-composer">
                <MarkdownEditor ref="editor" v-model="notesText" label="Release notes" :disabled="busy" @submit="saveNotes" />
              </div>
              <p v-if="errors.notes" class="gb-save-error" role="alert"><GbIcon name="error" small /> <b>Not saved.</b> {{ errors.notes }} Your text is still here.</p>
              <div class="gb-actions gb-rel-edit">
                <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="editing = false">Cancel</button>
                <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="saveNotes">Save notes</button>
              </div>
            </template>
            <article v-else-if="record?.notes" class="gb-comment">
              <MarkdownBody class="gb-comment-body" :html="notesHtml" />
            </article>
            <p v-else-if="record" class="gb-hint">No notes yet.</p>
            <p v-else class="gb-hint">No release record yet: a cut, promotion or rollback writes one once its tag is an hour old. Its notes and conversation live there.</p>
            <p v-if="record && !row.beforeGitBoard" class="gb-note gb-rel-after">
              Drafted by git-board from the issues merged since {{ record.previous || 'the start' }}.<template v-if="!finalCut && row.tag.version.rc"> They become {{ finalTag }}'s notes when it is promoted.</template>
            </p>
          </section>

          <section aria-labelledby="issues">
            <div class="gb-side-head">
              <h2 id="issues" class="gb-h2">Issues it carries</h2>
              <span class="gb-note">Merged {{ record?.previous ? `between ${record.previous} and` : 'up to' }} {{ sha7(row.tag.commit) }}</span>
            </div>
            <p v-if="!detail.issues.length" class="gb-hint">No merged branch names an issue.</p>
            <div v-else class="gb-table-wrap">
              <table class="gb-table">
                <thead><tr><th scope="col">Issue</th><th scope="col">Title</th><th scope="col">Kind</th><th scope="col">Merge</th></tr></thead>
                <tbody>
                  <tr v-for="is in detail.issues" :key="is.number + is.merge">
                    <td><b>{{ is.number }}</b></td>
                    <td>{{ is.title }}</td>
                    <td><span v-if="is.kind" class="gb-tag">{{ is.kind }}</span></td>
                    <td class="gb-mono">{{ sha7(is.merge) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p v-if="kind" class="gb-note gb-rel-after">A {{ kind }} release<template v-if="kind === 'patch' && !detail.issues.some((i) => i.kind === 'feat')">: none of them is a feat</template>.</p>
          </section>

          <section v-if="candidates.length" aria-labelledby="cands">
            <div class="gb-side-head"><h2 id="cands" class="gb-h2">Candidates for {{ finalTag }}</h2></div>
            <div class="gb-table-wrap">
              <table class="gb-table">
                <thead><tr><th scope="col">Candidate</th><th scope="col">Commit</th><th scope="col">Cut</th><th scope="col">What happened</th></tr></thead>
                <tbody>
                  <tr v-for="c in candidates" :key="c.tag.name" :class="{ 'gb-rel-yanked': c.record?.yanked }">
                    <td><b v-if="c.tag.name === tag">{{ c.tag.name }}</b><router-link v-else :to="releasePath(c.tag.name)">{{ c.tag.name }}</router-link></td>
                    <td class="gb-mono">{{ sha7(c.tag.commit) }}</td>
                    <td>{{ when(c.tag.date) }}<template v-if="!c.beforeGitBoard && c.record?.author">, {{ c.record.author }}</template></td>
                    <td>{{ happened(c) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <section v-if="record" aria-labelledby="conv">
            <h2 id="conv" class="gb-h2 gb-rel-convh">Conversation</h2>
            <p v-if="!record.comments.length" class="gb-hint">No comment yet.</p>
            <div class="gb-rel-comments">
              <article v-for="(c, i) in record.comments" :key="i" class="gb-comment">
                <header class="gb-comment-head"><span class="gb-avatar gb-avatar-sm" aria-hidden="true">{{ initials(c.author) }}</span><b>{{ c.author }}</b><span class="gb-time">{{ when(c.date, true) }}</span></header>
                <MarkdownBody class="gb-comment-body" :html="commentsHtml[i] ?? ''" />
              </article>
            </div>
            <div class="gb-composer gb-rel-composer">
              <MarkdownEditor v-model="commentText" label="Comment" placeholder="Leave a comment" :disabled="busy" @submit="sendComment" />
              <p v-if="errors.comment" class="gb-save-error" role="alert"><GbIcon name="error" small /> <b>Your comment was not posted.</b> {{ errors.comment }} Your text is still here.</p>
              <div class="gb-composer-foot">
                <p class="gb-hint">Kept on the release's record in git, and pushed at once.</p>
                <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !commentText.trim()" @click="sendComment">Comment</button>
              </div>
            </div>
          </section>
        </div>

        <aside class="gb-side" aria-label="Release details">
          <section class="gb-side-sec" aria-labelledby="runs">
            <div class="gb-side-head"><h2 id="runs" class="gb-section-title">Runs in</h2></div>
            <div class="gb-rel-runs">
              <template v-for="e in runsIn" :key="e.env.name">
                <div v-if="e.here">
                  <span :class="markClass(envMark(e.env.role, true))">{{ e.env.name }}</span>
                  <ul class="gb-targets gb-rel-targets">
                    <li v-for="t in e.targets" :key="t.target.repo + t.target.path">
                      <GbIcon v-if="t.here" name="check" small class="gb-state-ok" />
                      <GbIcon v-else name="error" small class="gb-state-warn" />
                      <span>{{ t.target.name }} · <template v-if="t.named">since {{ when(t.named.date, true) }} by {{ t.named.author }}</template><template v-else>{{ runs(t.target) }}<template v-if="t.here && t.target.since"> since {{ when(t.target.since.date, true) }} by {{ t.target.since.author }}</template></template></span>
                    </li>
                  </ul>
                </div>
                <div v-else class="gb-envline">
                  <span :class="markClass('off')">{{ e.env.name }}</span>
                  <span class="gb-note">{{ elsewhere(e.env) }}</span>
                </div>
              </template>
            </div>
            <p class="gb-note gb-rel-after">Read from the config repos' overlays and their history.</p>
          </section>

          <section class="gb-side-sec" aria-labelledby="imgs">
            <div class="gb-side-head"><h2 id="imgs" class="gb-section-title">Images</h2></div>
            <p v-if="!images.length && !record?.chart.version" class="gb-hint">None recorded.</p>
            <div v-else class="gb-artifacts">
              <div v-for="[name, img] in images" :key="name" class="gb-artifact"><b>{{ name }}</b><span class="gb-mono">{{ [...img.registries, short(img.digest)].join(' · ') }}</span></div>
              <div v-if="record?.chart.version" class="gb-artifact"><b>chart</b><span class="gb-mono">{{ [record.chart.version, ...(record.chart.digest ? [short(record.chart.digest)] : [])].join(' · ') }}</span></div>
            </div>
            <p v-if="images.length" class="gb-note gb-rel-after">
              <template v-if="row.beforeGitBoard">Read from the registries by its tag, when it was recorded.</template>
              <template v-else><template v-if="!finalCut && row.tag.version.rc && last">The same digests go to {{ last.name }}. </template>Built once, for commit {{ sha7(row.tag.commit) }}.</template>
            </p>
          </section>

          <section class="gb-side-sec" aria-labelledby="rec">
            <div class="gb-side-head"><h2 id="rec" class="gb-section-title">Record</h2></div>
            <ul class="gb-plain-list">
              <li v-if="!record">None yet</li>
              <li v-else-if="row.beforeGitBoard">Recorded from its tag by {{ record.author }}, in git</li>
              <li v-else>Made by {{ record.author }}, in git</li>
              <li v-if="record"><span class="gb-mono">refs/releases/{{ sha7(record.id) }}</span></li>
              <li v-for="n in row.names" :key="n">
                <span class="gb-mono">tag <router-link v-if="n !== tag" :to="releasePath(n)">{{ n }}</router-link><template v-else>{{ n }}</template></span>
              </li>
            </ul>
          </section>
        </aside>
      </div>
    </template>
  </main>

  <q-dialog v-model="yanking">
    <section class="gb-dialog gb-rel-yank" role="dialog" aria-modal="true" aria-labelledby="yank-title">
      <header class="gb-dialog-head">
        <h2 id="yank-title" class="h5">Yank {{ tag }}</h2>
        <button type="button" class="gb-icon-btn" aria-label="Cancel" @click="yanking = false"><GbIcon name="close" /></button>
      </header>
      <form class="gb-dialog-body" @submit.prevent="yank">
        <p class="gb-rel-yank-lead">Its tag stays, and so does this page, marked yanked. git-board won't cut or promote its commit again.</p>
        <label class="gb-field"><span class="epfl-label">Why, for the record (required)</span><input v-model="yankReason" class="epfl-input" autocomplete="off" autofocus required /></label>
        <p v-if="errors.yank" class="epfl-alert epfl-alert-danger" role="alert"><b>Not yanked.</b> {{ errors.yank }}</p>
      </form>
      <footer class="gb-dialog-foot">
        <button type="button" class="epfl-btn epfl-btn-secondary" @click="yanking = false">Cancel</button>
        <button type="button" class="epfl-btn epfl-btn-primary" :disabled="busy || !yankReason.trim()" @click="yank">Yank {{ tag }}</button>
      </footer>
    </section>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import AppHeader from '@/components/AppHeader.vue';
import GbIcon from '@/components/GbIcon.vue';
import MarkdownBody from '@/components/MarkdownBody.vue';
import MarkdownEditor from '@/components/MarkdownEditor.vue';
import ReleaseJob from '@/components/ReleaseJob.vue';
import { commentRelease, getRelease, getReleases, getViews, promoteRelease, renderMarkdown, setReleaseNotes, yankRelease, type EnvState, type ReleaseDetail, type ReleaseRecord, type ReleaseRow, type Releases } from '@/api/board';
import { message } from '@/issue';
import { debounced, onLive } from '@/live';
import { initials } from '@/me';
import { bump, envMark, finalOf, newest, promotion, registryName, runs, tagPattern, type EnvMark } from '@/releases';
import { on, when } from '@/when';

const route = useRoute();
const tag = computed(() => String(route.params.tag));

const viewNames = ref<string[]>([]);
const detail = ref<ReleaseDetail | null>(null);
const data = ref<Releases | null>(null); // the environments and the job
const error = ref('');
const errors = reactive({ top: '', notes: '', comment: '', yank: '' }); // a write refused, by where it was asked
const pending = ref(false); // a write sent, its answer not back yet
const watched = ref(false); // a job this page saw running stays shown once ended; another's, only while it runs
const notesHtml = ref('');
const commentsHtml = ref<string[]>([]);
const editing = ref(false);
const notesText = ref('');
const editor = ref<InstanceType<typeof MarkdownEditor> | null>(null);
const commentText = ref('');
const yanking = ref(false);
const yankReason = ref('');

const running = computed(() => data.value?.job?.state === 'running');
const busy = computed(() => running.value || pending.value);
const envs = computed(() => data.value?.environments ?? []);
const last = computed(() => envs.value.find((e) => e.role === 'last'));
const row = computed(() => detail.value?.row);
const record = computed(() => row.value?.record ?? null);
// who cut it: its record's author, but a release before git-board's, whose
// record's author only reconciled it
const author = computed(() => (row.value?.beforeGitBoard ? '' : (record.value?.author ?? '')));
const finalTag = computed(() => (row.value ? `v${finalOf(row.value.tag.version)}` : ''));
// its version's final is cut already (a candidate promoted, maybe another)
const finalCut = computed(() => data.value?.releases.some((r) => r.tag.name === finalTag.value) ?? false);
const list = new Intl.ListFormat('en', { type: 'conjunction' });

// "Candidate · in stage", "Release · in prod", "Yanked"
const badge = computed(() => {
  const r = row.value;
  if (!r) return null;
  if (r.record?.yanked) return { text: 'Yanked', cls: 'gb-rel-badge-yanked' };
  const where = r.runsIn.length ? ` · in ${list.format(r.runsIn)}` : '';
  return r.tag.version.rc ? { text: `Candidate${where}`, cls: 'epfl-badge-tertiary' } : { text: `Release${where}`, cls: 'epfl-badge-dark' };
});

// a candidate the environment before last runs, not yanked (the engine
// refuses a yanked commit)
const promote = computed(() => {
  const r = row.value;
  if (!r || r.record?.yanked) return null;
  const i = envs.value.findIndex((e) => e.version === r.tag.name);
  const p = i < 0 ? null : promotion(envs.value, i);
  return p?.tag === r.tag.name ? p : null;
});

// what the release is to the final before it
const kind = computed(() => {
  const prev = tagPattern.exec(record.value?.previous ?? '');
  return row.value && prev ? bump({ major: +prev[1]!, minor: +prev[2]!, patch: +prev[3]!, rc: 0 }, row.value.tag.version) : '';
});

// its version's candidates, itself among them, the first first
const candidates = computed(() => {
  const d = detail.value;
  if (!d?.candidates.length) return [];
  return [...d.candidates, ...(d.row.tag.version.rc ? [d.row] : [])].sort((a, b) => a.tag.version.rc - b.tag.version.rc);
});

// each environment, where it runs first, then the last environment down:
// per target, whether it runs it and since when: the newest overlay change
// naming it, else (it runs another name of its commit) the overlay's last
const runsIn = computed(() => {
  const r = row.value;
  if (!r) return [];
  return [...envs.value].reverse().map((env) => {
    const targets = env.targets.map((target) => ({
      target,
      here: target.deployed.commit === r.tag.commit,
      named: target.deployed.commit === r.tag.commit ? detail.value?.history[env.name]?.find((h) => h.target === target.name) : undefined,
    }));
    return { env, targets, here: targets.some((t) => t.here) };
  }).sort((a, b) => +b.here - +a.here);
});

// the record's images, a line each: its registries and digest
const images = computed(() => {
  const m = new Map<string, { registries: string[]; digest: string }>();
  for (const a of record.value?.artifacts ?? []) {
    const img = m.get(a.image) ?? { registries: [], digest: a.digest };
    img.registries.push(registryName(a.registry));
    m.set(a.image, img);
  }
  return [...m];
});

const repoNames = computed(() => Object.fromEntries(envs.value.flatMap((e) => e.targets.map((t) => [t.repo, t.name]))));

const sha7 = (c: string) => c.slice(0, 7);
const short = (digest: string) => `sha256:${digest.replace(/^sha256:/, '').slice(0, 12)}`;
const releasePath = (t: string) => `/releases/${encodeURIComponent(t)}`;
const markClass = (m: EnvMark) => ['gb-envmark', m === 'dev' ? '' : `gb-envmark-${m}`];
const plural = (n: number, w: string) => `${n} ${w}${n === 1 ? '' : 's'}`;
const elsewhere = (e: EnvState) => (!e.adopted ? "isn't adopted yet" : e.split ? 'runs different releases' : `runs ${e.version || sha7(e.commit)}`);

// a candidate's story: yanked, promoted, running, or none of these
function happened(c: ReleaseRow): string {
  if (c.record?.yanked) return `Yanked: ${c.record.yanked}`;
  const final = c.names.find((n) => tagPattern.exec(n) && !tagPattern.exec(n)![4]);
  if (final) return `Promoted as ${final}.`;
  if (!c.runsIn.length) return 'Runs nowhere now.';
  const since = newest(envs.value.find((e) => e.name === c.runsIn[0])?.targets ?? []);
  return `In ${list.format(c.runsIn)}${since ? ` since ${when(since.date, true)}` : ''}.`;
}

// the record's notes and comments, as the server renders Markdown
async function rendered(rec: ReleaseRecord | null): Promise<string[]> {
  const md = (s: string) => renderMarkdown(s).then((r) => r.html);
  return Promise.all([rec?.notes ? md(rec.notes) : '', ...(rec?.comments ?? []).map((c) => md(c.message))]);
}

async function loadDetail() {
  const t = tag.value;
  try {
    const d = await getRelease(t);
    const [notes, ...comments] = await rendered(d.row.record);
    if (t !== tag.value) return; // another release was opened meanwhile
    detail.value = d;
    notesHtml.value = notes ?? '';
    commentsHtml.value = comments;
    error.value = '';
  } catch (e) {
    if (t === tag.value) error.value = message(e);
  }
}

// the environments and the job; once a job ends, the release too: it may
// have moved it
async function loadReleases() {
  try {
    const was = running.value;
    const r = await getReleases();
    if (r.job?.state === 'running') watched.value = true;
    data.value = r;
    if (was && r.job?.state !== 'running') void loadDetail();
  } catch (e) {
    error.value = message(e);
  }
}

// a write: busy until it answers, its refusal said where it was asked
async function act(where: keyof typeof errors, f: () => Promise<unknown>): Promise<boolean> {
  errors[where] = '';
  pending.value = true;
  try {
    await f();
    return true;
  } catch (e) {
    errors[where] = message(e);
    return false;
  } finally {
    pending.value = false;
  }
}

function startPromote() {
  const t = tag.value;
  void act('top', async () => {
    const job = await promoteRelease(t);
    watched.value = true;
    if (data.value) data.value.job = job;
  });
}

async function editNotes() {
  notesText.value = record.value?.notes ?? '';
  errors.notes = '';
  editing.value = true;
  await nextTick();
  editor.value?.focus();
}

async function saveNotes() {
  if (busy.value) return;
  if (await act('notes', () => setReleaseNotes(tag.value, notesText.value))) {
    editing.value = false;
    await loadDetail();
  }
}

async function sendComment() {
  if (busy.value || !commentText.value.trim()) return;
  if (await act('comment', () => commentRelease(tag.value, commentText.value))) {
    commentText.value = '';
    await loadDetail();
  }
}

async function yank() {
  if (busy.value || !yankReason.value.trim()) return;
  if (await act('yank', () => yankRelease(tag.value, yankReason.value.trim()))) {
    yanking.value = false;
    yankReason.value = '';
    await loadDetail();
  }
}

function load() {
  void loadDetail();
  void loadReleases();
}

// another release, the same page (its links: after, candidates, tags)
watch(tag, () => {
  detail.value = null;
  editing.value = false;
  commentText.value = yankReason.value = ''; // typed for the other release
  Object.assign(errors, { top: '', notes: '', comment: '', yank: '' });
  load();
});

// a job's steps: the environments and the job only, while it runs (the
// release's history is the slow read); config: [release] changed; issues,
// while a job runs: live.ts says it after a reconnect
const reloadSoon = debounced(() => (running.value ? void loadReleases() : load()));
const offLive = onLive((e) => (e.kind === 'releases' || e.kind === 'config' || (e.kind === 'issues' && running.value)) && reloadSoon());
onBeforeUnmount(offLive);

onMounted(() => {
  void getViews().then(
    (v) => (viewNames.value = Object.keys(v).sort()),
    () => {},
  );
  load();
});
</script>

<style scoped>
.gb-rel-crumb { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; font-size: .9375rem; }
.gb-rel-back { display: inline-flex; align-items: center; gap: .375rem; }
.gb-rel-title { margin-top: 1.25rem; }
/* the danger palette: the badge colours but dark's and canard's fall under 4.5:1 in dark mode */
.gb-rel-badge-yanked { background: var(--danger-bg); color: var(--danger-fg); box-shadow: inset 0 0 0 1px var(--danger-border); }
.gb-rel-meta { margin-top: -.75rem; }
.gb-fg { color: var(--fg); }
.gb-rel-job { margin: 1.5rem 0 0; }
/* the release board's columns: no panel padding, a rule above */
.gb-rel-cols { grid-template-columns: minmax(0, 1fr) 20rem; gap: 3rem; margin-top: 2rem; padding: 2rem 0 0; border-top: 1px solid var(--border-subtle); }
.gb-rel-cols .gb-maincol { gap: 2rem; }
.gb-rel-after { margin-top: .5rem; }
.gb-rel-edit { justify-content: flex-end; margin-top: .75rem; }
.gb-rel-convh { margin-bottom: 1rem; }
.gb-rel-comments { display: flex; flex-direction: column; gap: 1rem; }
.gb-rel-composer { margin-top: 1rem; }
.gb-rel-runs { display: flex; flex-direction: column; gap: 1rem; }
.gb-rel-targets { margin-top: .5rem; }
.gb-save-error { display: flex; gap: .375rem; align-items: center; margin: .5rem 0 0; padding: .5rem 1rem; font-size: .875rem; color: var(--danger-fg); background: var(--danger-bg); }
.gb-rel-yank { width: min(36rem, calc(100vw - 2rem)); }
.gb-rel-yank .gb-dialog-foot { justify-content: flex-end; }
.gb-rel-yank-lead { font-size: .9375rem; }
@media (max-width: 63.99rem) {
  .gb-rel-cols { grid-template-columns: minmax(0, 1fr); gap: 2rem; }
}
</style>
