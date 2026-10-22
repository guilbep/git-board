<!-- Step 6, Review (docs/prd/cycle-protocol.md, M5; the canvas's C6): the
     issue's branch from where it left the base, a files list beside the
     diff, the review on it (line comments, replies, Send to Claude Code,
     resolve) and the gate: Request changes or Approve, Start verification
     once the PR is merged. All of it is comments on the issue, which the
     sync mirrors to the pull request; neither verdict moves the status. -->
<template>
  <div class="gb-ch">
    <p v-if="error === 'no-branch'" class="gb-hint">
      No branch names this issue yet. Create one from Development, push it, and its changes show here.
    </p>
    <p v-else-if="error" class="gb-hint" role="alert"><b>Couldn’t read the changes.</b> {{ error }}</p>
    <p v-else-if="!changes" class="gb-hint" role="status">Reading the changes…</p>
    <template v-else>
      <div class="gb-ch-bar">
        <a v-if="cycle.evidence.pr" :href="cycle.evidence.pr.url" target="_blank" rel="noopener noreferrer" class="gb-ch-branch">
          PR #{{ cycle.evidence.pr.number }}<span class="gb-sr">(opens in a new tab)</span>
        </a>
        <select v-if="branchNames.length > 1" v-model="branch" class="epfl-select" aria-label="Branch">
          <option v-for="n in branchNames" :key="n" :value="n">{{ n }}</option>
        </select>
        <span v-else-if="!cycle.evidence.pr" class="gb-mono gb-ch-branch">{{ changes.branch }}</span>
        <span class="gb-hint">into {{ changes.base_name }}</span>
        <select v-model="view" class="epfl-select" aria-label="Commits">
          <option value="all">All changes ({{ changes.commits.length }} commit{{ changes.commits.length === 1 ? '' : 's' }})</option>
          <option v-if="sinceMine" value="mine">Since your last review ({{ sinceMine }} commit{{ sinceMine === 1 ? '' : 's' }})</option>
          <option v-for="c in changes.commits" :key="c.sha" :value="c.sha">{{ c.sha.slice(0, 7) }} {{ c.subject }}</option>
        </select>
        <span class="gb-hint">
          {{ files.length }} file{{ files.length === 1 ? '' : 's' }}
          <span class="gb-df-add">+{{ total[0] }}</span> <span class="gb-df-del">−{{ total[1] }}</span>
          · viewed {{ seen[0] }}/{{ seen[1] }}
        </span>
        <span class="gb-grow" />
        <span v-if="checks" class="gb-hint">{{ checks }}</span>
        <b>Approvals: {{ approved }} of {{ need }}</b>
      </div>
      <ul v-if="verdicts.length" class="gb-ch-verdicts">
        <li v-for="v in verdicts" :key="v.id">
          <b>{{ v.state === 'APPROVE' ? 'Approved' : 'Changes requested' }}</b> by {{ v.who }} at
          <span class="gb-mono">{{ v.commit.slice(0, 7) }}</span><template v-if="v.ago !== null">, {{ v.ago ? `${v.ago} commit${v.ago === 1 ? '' : 's'} ago` : 'the head' }}</template>
        </li>
      </ul>

      <p v-if="!files.length" class="gb-hint">No changes in these commits.</p>
      <div v-else class="gb-rv">
        <nav class="gb-rv-files" aria-label="Files">
          <template v-for="g in groups" :key="g.key">
            <p v-if="groups.length > 1" class="gb-rv-group">{{ g.title }}</p>
            <button v-for="f in g.files" :key="f.path" type="button" :aria-current="active === f.path ? 'true' : undefined" @click="goTo(f.path)">
              <span class="gb-mono">{{ f.path }}</span>
              <span><span class="gb-df-add">+{{ f.added }}</span> <span class="gb-df-del">−{{ f.deleted }}</span><template v-if="viewed[f.path] === f.blob"> ✓<span class="gb-sr">viewed</span></template></span>
            </button>
          </template>
        </nav>
        <div class="gb-rv-diff">
          <details v-for="g in groups" :key="g.key" class="gb-ch-group" :open="g.key === 'review'">
            <summary>{{ g.title }} <span class="gb-tag">{{ g.files.length }}</span></summary>
            <DiffFile
              v-for="f in g.files"
              :key="`${fileRange.to}:${f.path}`"
              :id="id"
              :file="f"
              :range="fileRange"
              :base="changes.base"
              :head="changes.head"
              :threads="threads.filter((t) => t.root.review!.path === f.path)"
              :viewed="viewed[f.path] === f.blob"
              :busy="busy"
              :comment="comment"
              :reply="reply"
              :resolve="resolve"
              :brief="sends ? brief : null"
              @viewed="(on) => mark(f, on)"
            />
          </details>
        </div>
      </div>

      <footer class="gb-ch-review" aria-label="The gate">
        <p class="gb-gate">{{ gate[0] }}</p>
        <p class="gb-hint">{{ gate[1] }} {{ gate[2] }}</p>
        <template v-if="current && merged">
          <p v-if="cycle.note" class="gb-note">{{ cap(cycle.note) }}.</p>
          <div v-if="can" class="gb-cycle-actions">
            <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="verify">Start verification</button>
          </div>
          <p v-else class="gb-note">{{ cap(turn(cycle)) }}.</p>
        </template>
        <template v-else-if="current">
          <p v-if="!can" class="gb-note">{{ cap(turn(cycle)) }}.</p>
          <!-- no @submit: Ctrl+Enter must not approve -->
          <MarkdownEditor v-model="summary" label="Your review" placeholder="Explain the change in one line, to approve it, or say what to change" :disabled="busy || !can" />
          <div class="gb-composer-foot">
            <span class="gb-hint">At {{ changes.head.slice(0, 7) }}{{ pr ? `, on ${pr.replace('github.com/', '')}` : '' }} at the next sync.</span>
            <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || !can || !summary.trim()" @click="submit('REQUEST_CHANGES')">Request changes</button>
            <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !can || !summary.trim()" @click="submit('APPROVE')">Approve</button>
          </div>
        </template>
      </footer>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { agentAction, ApiError, getBranches, getChanges, getPulls, type Changes, type Cycle, type FileStat, type ReviewRequest, type Thread } from '@/api/board';
import { turn } from '@/cycle';
import { message } from '@/issue';
import { onLive } from '@/live';
import { approvalsNeeded, approvers, checksLine, commitsAgo, groupOf, lastReviewBy, loadViewed, reviewGate, reviewsOf, saveViewed, shownRange, threadsOf, viewedCount, type Group, type Viewed } from '@/review';
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
  cycle: Cycle;
  current: boolean; // step 6 is the issue's step
  can: boolean; // the step's action (review, or to-validation once merged) is this person's
  sends: boolean; // the issue has an agent to brief
  verify: () => void;
}>();
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
// merged: the review is over, Start verification moves the status
const merged = computed(() => props.cycle.evidence.pr?.state === 'MERGED');

const changes = ref<Changes | null>(null); // the whole branch
const part = ref<FileStat[] | null>(null); // the files of a view other than the whole branch
const error = ref('');
const branch = ref(''); // '' until read: the server's choice
const view = ref('all'); // all, mine (since my last review), or a commit
const branchNames = ref<string[]>([]);
const pulls = ref<Record<string, string>>({}); // branch → its open PR, as review:pr writes it

const mine = computed(() => lastReviewBy(props.thread.events, props.meId, changes.value?.branch)?.review?.commit);
const sinceMine = computed(() => {
  const n = mine.value && changes.value ? commitsAgo(mine.value, changes.value.commits) : null;
  return n || 0; // 0: not in this branch, or at its head
});

// the commits compared: base…head, my last review…head, or one commit
const range = computed(() => ({ branch: changes.value!.branch, ...shownRange(changes.value!, view.value, mine.value) }));
// as the server resolved it: a commit's parent is the base when the commit
// is the branch's first, and the old side is commentable from the base
const partFrom = ref('');
const fileRange = computed(() => ({ ...range.value, from: partFrom.value || range.value.from }));

const files = computed(() => part.value ?? changes.value?.files ?? []);

let latest = 0;
async function load() {
  const n = ++latest;
  try {
    const c = await getChanges(props.id, { branch: branch.value });
    if (n !== latest) return;
    [changes.value, branch.value, error.value] = [c, c.branch, ''];
    const p = view.value === 'all' ? null : await getChanges(props.id, range.value);
    if (n === latest) [part.value, partFrom.value] = [p?.files ?? null, p?.from ?? ''];
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

const threads = computed(() => threadsOf(props.thread.events, changes.value?.branch));

// each person's last verdict, with how far behind the head it is
const verdicts = computed(() => {
  const last = new Map<string, { id: string; who: string; state: string; commit: string; ago: number | null }>();
  for (const e of reviewsOf(props.thread.events, changes.value?.branch)) {
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

// Send to Claude Code: the agent answers the thread in its worktree; the
// person resolves it
const brief = (root: string) => agentAction(props.id, 'brief', '', root).then(() => 'Sent to the agent.', message);

const need = computed(() => approvalsNeeded(props.cycle.risk ?? props.cycle.evidence.risk));
const approved = computed(() => approvers(props.cycle.evidence.pr, props.thread.events).length);
const checks = computed(() => checksLine(props.cycle.evidence.pr));
const gate = computed(() => reviewGate(props.cycle));

// the verdict: a line is required for both (the gate is a human who can
// explain the diff); the COMMENT verdict went, ponytail: replies and line
// comments say the rest
const summary = ref('');
async function submit(state: 'APPROVE' | 'REQUEST_CHANGES') {
  if (!changes.value) return;
  if (await props.review(withPR({ body: summary.value, state, commit: changes.value.head }))) summary.value = '';
}

// a file picked in the list, or a comment's path clicked in the
// conversation: its file, opened
const active = ref('');
async function goTo(path: string) {
  active.value = path;
  await nextTick();
  const el = document.querySelector<HTMLElement>(`.gb-ch [data-file="${CSS.escape(path)}"]`);
  el?.closest('details')?.setAttribute('open', '');
  el?.scrollIntoView({ block: 'start', behavior: 'smooth' });
}
watch(() => [props.focus, changes.value] as const, ([path, c]) => path && c && void goTo(path), { immediate: true });
</script>

<style scoped>
.gb-ch { display: flex; flex-direction: column; gap: .75rem; }
.gb-ch-bar { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; font-size: .875rem; }
.gb-ch-bar .epfl-select { width: auto; max-width: 22rem; }
.gb-ch-branch { font-weight: 700; }
.gb-ch-verdicts { margin: 0; padding: 0; list-style: none; font-size: .875rem; }
.gb-ch-group > summary { display: flex; align-items: center; gap: .375rem; margin-bottom: .5rem; font-weight: 700; cursor: pointer; }
.gb-ch-bar a { font-weight: 700; }
.gb-ch-review { display: flex; flex-direction: column; gap: .5rem; padding-top: .75rem; border-top: 1px solid var(--border); }
.gb-ch-review p { margin: 0; }
.gb-rv { display: grid; grid-template-columns: 13rem minmax(0, 1fr); gap: 1rem; align-items: start; }
.gb-rv-files { position: sticky; top: .5rem; display: flex; flex-direction: column; gap: .125rem; font-size: .8125rem; }
.gb-rv-files button { display: flex; flex-direction: column; align-items: flex-start; gap: .125rem; padding: .375rem .5rem; border: 0; border-left: 3px solid transparent; background: none; text-align: left; cursor: pointer; color: inherit; }
.gb-rv-files button .gb-mono { overflow-wrap: anywhere; }
.gb-rv-files button:hover { background: var(--bg-subtle); }
.gb-rv-files button[aria-current] { border-left-color: var(--gb-done); background: var(--bg-subtle); font-weight: 700; }
.gb-rv-group { margin: .5rem 0 0; font-size: .75rem; font-weight: 700; color: var(--fg-muted); }
.gb-rv-diff { display: flex; flex-direction: column; gap: .75rem; min-width: 0; }
@media (max-width: 63.99rem) {
  .gb-rv { grid-template-columns: minmax(0, 1fr); }
  .gb-rv-files { position: static; flex-flow: row wrap; }
  .gb-rv-files button { border-left: 0; border-bottom: 3px solid transparent; }
  .gb-rv-files button[aria-current] { border-bottom-color: var(--gb-done); }
}
.gb-df-add { color: var(--success-fg); }
.gb-df-del { color: var(--danger-fg); }
</style>
