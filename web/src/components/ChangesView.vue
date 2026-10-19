<!-- The Changes tab (docs/prd/work-unit.md, M4, layout A): the issue's
     branch from where it left the base, file by file, with the review on
     it: line comments, replies, resolve, and Comment / Approve / Request
     changes. All of it is comments on the issue, which the sync mirrors to
     the pull request. -->
<template>
  <div class="gb-ch">
    <p v-if="error === 'no-branch'" class="gb-hint">
      No branch names this issue yet. Create one from Development, push it, and its changes show here.
    </p>
    <p v-else-if="error" class="gb-hint" role="alert"><b>Couldn’t read the changes.</b> {{ error }}</p>
    <p v-else-if="!changes" class="gb-hint" role="status">Reading the changes…</p>
    <template v-else>
      <div class="gb-ch-bar">
        <select v-if="branchNames.length > 1" v-model="branch" class="epfl-select" aria-label="Branch">
          <option v-for="n in branchNames" :key="n" :value="n">{{ n }}</option>
        </select>
        <span v-else class="gb-mono gb-ch-branch">{{ changes.branch }}</span>
        <span class="gb-hint">into {{ changes.base_name }}</span>
        <select v-model="view" class="epfl-select" aria-label="Commits">
          <option value="all">All changes ({{ changes.commits.length }} commit{{ changes.commits.length === 1 ? '' : 's' }})</option>
          <option v-if="sinceMine" value="mine">Since your last review ({{ sinceMine }} commit{{ sinceMine === 1 ? '' : 's' }})</option>
          <option v-for="c in changes.commits" :key="c.sha" :value="c.sha">{{ c.sha.slice(0, 7) }} {{ c.subject }}</option>
        </select>
        <span class="gb-grow" />
        <span class="gb-hint">
          {{ files.length }} file{{ files.length === 1 ? '' : 's' }},
          <span class="gb-df-add">+{{ total[0] }}</span> <span class="gb-df-del">−{{ total[1] }}</span>
          · viewed {{ seen[0] }}/{{ seen[1] }}
        </span>
      </div>
      <ul v-if="verdicts.length" class="gb-ch-verdicts">
        <li v-for="v in verdicts" :key="v.id">
          <b>{{ v.state === 'APPROVE' ? 'Approved' : 'Changes requested' }}</b> by {{ v.who }} at
          <span class="gb-mono">{{ v.commit.slice(0, 7) }}</span><template v-if="v.ago !== null">, {{ v.ago ? `${v.ago} commit${v.ago === 1 ? '' : 's'} ago` : 'the head' }}</template>
        </li>
      </ul>

      <p v-if="!files.length" class="gb-hint">No changes in these commits.</p>
      <details v-for="g in groups" :key="g.key" class="gb-ch-group" :open="g.key === 'review'">
        <summary>{{ g.title }} <span class="gb-tag">{{ g.files.length }}</span></summary>
        <DiffFile
          v-for="f in g.files"
          :key="`${range.to}:${f.path}`"
          :id="id"
          :file="f"
          :range="range"
          :base="changes.base"
          :head="changes.head"
          :threads="threads.filter((t) => t.root.review!.path === f.path)"
          :viewed="viewed[f.path] === f.blob"
          :busy="busy"
          :comment="comment"
          :reply="reply"
          :resolve="resolve"
          @viewed="(on) => mark(f, on)"
        />
      </details>

      <section class="gb-ch-review" aria-labelledby="ch-review">
        <h3 id="ch-review" class="gb-section-title">Your review</h3>
        <div class="gb-ch-states" role="radiogroup" aria-label="Verdict">
          <label v-for="s in states" :key="s.value" class="epfl-radio"><input v-model="state" type="radio" name="gb-review-state" :value="s.value" /> {{ s.label }}</label>
        </div>
        <MarkdownEditor v-model="summary" label="Review summary" :placeholder="state === 'APPROVE' ? 'Optional' : 'Say what to change'" :disabled="busy" @submit="submit" />
        <div class="gb-composer-foot">
          <span class="gb-hint">At {{ changes.head.slice(0, 7) }}{{ pr ? `, on ${pr.replace('github.com/', '')}` : '' }} at the next sync.</span>
          <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || (state !== 'APPROVE' && !summary.trim())" @click="submit">Submit review</button>
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { ApiError, getBranches, getChanges, getPulls, type Changes, type FileStat, type ReviewRequest, type Thread } from '@/api/board';
import { message } from '@/issue';
import { onLive } from '@/live';
import { commitsAgo, groupOf, lastReviewBy, loadViewed, reviewsOf, saveViewed, threadsOf, viewedCount, type Group, type Viewed } from '@/review';
import DiffFile from '@/components/DiffFile.vue';
import MarkdownEditor from '@/components/MarkdownEditor.vue';

const props = defineProps<{
  id: string;
  thread: Thread;
  meId: string;
  busy: boolean;
  focus?: string | undefined; // a file to scroll to: a comment's path:line clicked in the conversation
  review: (req: ReviewRequest) => Promise<boolean>;
  resolve: (root: string, resolved: boolean) => Promise<boolean>;
}>();

const changes = ref<Changes | null>(null); // the whole branch
const part = ref<FileStat[] | null>(null); // the files of a view other than the whole branch
const error = ref('');
const branch = ref(''); // '' until read: the server's choice
const view = ref('all'); // all, mine (since my last review), or a commit
const branchNames = ref<string[]>([]);
const pulls = ref<Record<string, string>>({}); // branch → its open PR, as review:pr writes it

const mine = computed(() => lastReviewBy(props.thread.events, props.meId)?.review?.commit);
const sinceMine = computed(() => {
  const n = mine.value && changes.value ? commitsAgo(mine.value, changes.value.commits) : null;
  return n || 0; // 0: not in this branch, or at its head
});

// the commits compared: base…head, my last review…head, or one commit
const range = computed(() => {
  const c = changes.value!;
  if (view.value === 'mine' && mine.value) return { branch: c.branch, from: mine.value, to: c.head };
  const i = c.commits.findIndex((x) => x.sha === view.value);
  if (i < 0) return { branch: c.branch, from: c.base, to: c.head };
  return { branch: c.branch, from: c.commits[i + 1]?.sha ?? c.base, to: c.commits[i]!.sha };
});

const files = computed(() => part.value ?? changes.value?.files ?? []);

let latest = 0;
async function load() {
  const n = ++latest;
  try {
    const c = await getChanges(props.id, { branch: branch.value });
    if (n !== latest) return;
    [changes.value, branch.value, error.value] = [c, c.branch, ''];
    const p = view.value === 'all' ? null : (await getChanges(props.id, range.value)).files;
    if (n === latest) part.value = p;
  } catch (e) {
    if (n === latest) error.value = e instanceof ApiError && e.code === 'no-branch' ? 'no-branch' : message(e);
  }
}
void load();
watch(view, load);
watch(branch, (b, was) => {
  if (!was) return; // the first read
  view.value = 'all';
  void load();
});
// a write here or a sync: the head may move. ponytail: a push alone shows
// when the tab is opened again (the server watches no code refs, as for
// Development); poll if that bites
watch(() => props.thread, load);
const offLive = onLive((e) => e.kind === 'sync' && void load());
onBeforeUnmount(offLive);

void getBranches(props.id).then((b) => (branchNames.value = [...new Set(b.branches.map((x) => x.name))]), () => {});
void getPulls(props.id).then((p) => {
  for (const x of p.pulls) if (p.repo && x.pr?.state === 'OPEN') pulls.value[x.branch] = `github.com/${p.repo}#${x.pr.number}`;
}, () => {});
const pr = computed(() => (changes.value ? pulls.value[changes.value.branch] : undefined));

const total = computed(() => [files.value.reduce((n, f) => n + f.added, 0), files.value.reduce((n, f) => n + f.deleted, 0)]);

const groupTitles: Record<Group, string> = { review: 'To review', large: 'Large', generated: 'Generated' };
const groups = computed(() =>
  (['review', 'large', 'generated'] as Group[])
    .map((key) => ({ key, title: groupTitles[key], files: files.value.filter((f) => groupOf(f) === key) }))
    .filter((g) => g.files.length),
);

const threads = computed(() => threadsOf(props.thread.events));

// each person's last verdict, with how far behind the head it is
const verdicts = computed(() => {
  const last = new Map<string, { id: string; who: string; state: string; commit: string; ago: number | null }>();
  for (const e of reviewsOf(props.thread.events)) {
    const r = e.review!;
    if (r.state === 'COMMENT') continue;
    last.set(e.author.id, { id: e.id, who: e.author.name, state: r.state!, commit: r.commit ?? '', ago: commitsAgo(r.commit ?? '', changes.value?.commits ?? []) });
  }
  return [...last.values()];
});

const viewed = ref<Viewed>({});
watch(() => changes.value?.branch, (b) => b && (viewed.value = loadViewed(props.id, b)), { immediate: true });
const seen = computed(() => viewedCount(files.value, viewed.value));
function mark(f: FileStat, on: boolean) {
  const v = { ...viewed.value };
  if (on) v[f.path] = f.blob;
  else delete v[f.path];
  viewed.value = v;
  saveViewed(props.id, changes.value!.branch, v);
}

// writes: the anchor's commit is the view's end; pr once there is one
const withPR = (req: ReviewRequest): ReviewRequest => (pr.value ? { ...req, pr: pr.value } : req);
const comment = (req: ReviewRequest) => props.review(withPR(req));
const reply = (to: string, body: string) => props.review(withPR({ body, reply_to: to }));

const states = [
  { value: 'COMMENT', label: 'Comment' },
  { value: 'APPROVE', label: 'Approve' },
  { value: 'REQUEST_CHANGES', label: 'Request changes' },
] as const;
const state = ref<ReviewRequest['state']>('COMMENT');
const summary = ref('');
async function submit() {
  if (!changes.value) return;
  if (await props.review(withPR({ body: summary.value, state: state.value, commit: changes.value.head }))) summary.value = '';
}

// a comment's path clicked in the conversation: its file, opened
watch(
  () => [props.focus, changes.value] as const,
  async ([path, c]) => {
    if (!path || !c) return;
    await nextTick();
    const el = document.querySelector<HTMLElement>(`.gb-ch [data-file="${CSS.escape(path)}"]`);
    el?.closest('details')?.setAttribute('open', '');
    el?.scrollIntoView({ block: 'start', behavior: 'smooth' });
  },
  { immediate: true },
);
</script>

<style scoped>
.gb-ch { display: flex; flex-direction: column; gap: .75rem; }
.gb-ch-bar { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; font-size: .875rem; }
.gb-ch-bar .epfl-select { width: auto; max-width: 22rem; }
.gb-ch-branch { font-weight: 700; }
.gb-ch-verdicts { margin: 0; padding: 0; list-style: none; font-size: .875rem; }
.gb-ch-group > summary { display: flex; align-items: center; gap: .375rem; margin-bottom: .5rem; font-weight: 700; cursor: pointer; }
.gb-ch-review { display: flex; flex-direction: column; gap: .5rem; padding-top: .75rem; border-top: 1px solid var(--border); }
.gb-ch-states { display: flex; flex-wrap: wrap; gap: 1rem; }
.gb-df-add { color: var(--success-fg); }
.gb-df-del { color: var(--danger-fg); }
</style>
