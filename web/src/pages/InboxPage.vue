<!-- The inbox: what happened that concerns you on the issues of the last
     14 days (internal/server/inbox.go), newest first. Opening it
     checks the git remotes (a fetch: nothing local changes) and offers
     Pull; opening a row opens the issue and marks it read. Read rows stay,
     dimmed, until Refresh, so the list doesn't jump while you read. -->
<template>
  <AppHeader :views="viewNames" active="" />
  <main class="gb-page gb-inbox">
    <div class="epfl-eyebrow">Inbox</div>
    <h1 class="h3">What happened that concerns you</h1>
    <p class="gb-lead">On the issues you follow, since you last opened them.</p>

    <div class="gb-inbox-bar">
      <div class="gb-inbox-status" role="status">
        <span v-for="c in checks" :key="c.name" :class="{ 'gb-inbox-fail': c.failure }">
          <template v-if="c.failure">{{ c.failure }}</template>
          <template v-else>
            Checked <strong>{{ c.name }}</strong> {{ c.when }}
            <template v-if="c.incoming">
              · {{ c.incoming }} {{ c.incoming === 1 ? 'issue' : 'issues' }} new ·
              <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm gb-inbox-pull" :disabled="busy" @click="pull(c.name)">Pull</button>
            </template>
          </template>
        </span>
      </div>
      <div class="gb-inbox-actions">
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="refresh(true)">Refresh</button>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!unread.length" @click="markAll">Mark all read</button>
        <button type="button" class="gb-icon-btn" aria-label="Inbox settings" :aria-expanded="menuOpen" @click="menuOpen = !menuOpen">
          <GbIcon name="settings" small />
        </button>
      </div>
      <div v-if="menuOpen && data" class="gb-inbox-menu" role="group" aria-label="Inbox settings">
        <label class="gb-inbox-check">
          <input type="checkbox" :checked="data.fields" @change="saveSettings(data.views, !data.fields)" />
          <span><strong>Also field changes on issues I'm in</strong><br /><span class="gb-hint">Sprint, priority, status… where you're assignee, reviewer, pm…</span></span>
        </label>
        <fieldset class="gb-inbox-views">
          <legend><strong>Follow views</strong></legend>
          <span v-if="!viewNames.length" class="gb-hint">No saved views yet: <code>[views]</code> in <code>.git-board.toml</code>.</span>
          <label v-for="v in viewNames" :key="v" class="gb-inbox-check">
            <input type="checkbox" :checked="data.views.includes(v)" @change="toggleView(v)" /> {{ v }}
          </label>
          <span v-for="v in data.views_missing" :key="v" class="gb-inbox-check gb-hint">
            {{ v }} (no longer exists)
            <button type="button" class="gb-inbox-x" :aria-label="`Stop following ${v}`" @click="toggleView(v)">×</button>
          </span>
        </fieldset>
      </div>
    </div>

    <p v-if="error?.code === 'no-identity'" class="gb-inbox-need">
      The inbox shows what concerns the identity you write as, and none is chosen here yet.
      <router-link to="/settings/you">Choose who you write as</router-link>
    </p>
    <p v-else-if="error?.code === 'no-login'" class="gb-inbox-need">
      Tell git-board your forge login: without it, no field and no mention can name you. Run <code>git bug bridge new</code>, or start the server with <code>--me your-login</code>.
    </p>
    <p v-else-if="error" class="gb-error">{{ error.message }}</p>

    <ul v-if="rows.length" class="gb-inbox-rows">
      <li v-for="r in rows" :key="r.issue.id" :class="{ 'gb-inbox-read': read.has(r.issue.id) }">
        <button type="button" class="gb-inbox-row" @click="open(r)">
          <span class="visually-hidden">{{ read.has(r.issue.id) ? 'Read' : 'Unread' }}, {{ r.issue.status }}:</span>
          <span class="gb-inbox-dot" aria-hidden="true" />
          <GbIcon :name="r.issue.status === 'closed' ? 'closed' : 'open'" small />
          <span class="gb-inbox-main">
            <span class="gb-inbox-title"><span v-if="numberOf(r)" class="gb-inbox-num">#{{ numberOf(r) }}</span> {{ r.issue.shown_title }}</span>
            <span class="gb-inbox-news">
              {{ r.news.join(' · ') }}
              <span v-for="x in r.reasons" :key="x" class="gb-inbox-chip">{{ x }}</span>
            </span>
          </span>
          <span class="gb-inbox-who">
            <span v-for="p in r.who" :key="p.id" class="gb-avatar gb-avatar-sm gb-avatar-dark" :title="p.name">{{ initials(p.name) }}</span>
          </span>
          <span class="gb-hint gb-inbox-when">{{ when(r.latest, true) }}</span>
        </button>
      </li>
    </ul>
    <div v-else-if="loaded && data && !error" class="gb-inbox-empty">
      <h2 class="h4">Nothing new for you</h2>
      <p class="gb-hint">{{ following }}</p>
    </div>
  </main>

  <q-dialog
    v-model="detailOpen"
    :position="panelMode === 'side' ? 'right' : 'standard'"
    :full-height="panelMode === 'side'"
    :class="`gb-dialog-${panelMode}`"
    :backdrop-filter="panelMode === 'wide' ? 'blur(6px)' : undefined"
    no-route-dismiss
    no-focus
    @hide="setIssue(undefined)"
  >
    <IssuePanel
      v-if="detailId"
      :id="detailId"
      :key="detailId"
      :names="names"
      :fields="fields"
      :title="detailRow?.issue.shown_title ?? ''"
      crumb="Inbox"
      :position="index >= 0 ? `${index + 1} of ${rows.length}` : ''"
      :has-prev="index > 0"
      :has-next="index >= 0 && index < rows.length - 1"
      :at="detailRow?.first"
      @close="detailOpen = false"
      @prev="step(-1)"
      @next="step(1)"
      @open="(id: string) => setIssue(id)"
    />
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import AppHeader from '@/components/AppHeader.vue';
import GbIcon from '@/components/GbIcon.vue';
import IssuePanel from '@/components/IssuePanel.vue';
import { ApiError, getFields, getRemotes, getViews, markSeen, remoteAction, setInboxSettings, type InboxRow, type Remote } from '@/api/board';
import { namespaces } from '@/facets';
import type { Fields } from '@/fields';
import { inbox, inboxError, loadInbox } from '@/inbox';
import { forgeNumbers } from '@/issuesync';
import { checkFailure, followingLine, mergeRows, toCheck, toPull } from '@/inboxcheck';
import { debounced, onLive } from '@/live';
import { initials, me, when } from '@/me';
import { panelMode } from '@/panelsize';

const route = useRoute();
const router = useRouter();

const viewNames = ref<string[]>([]);
const fields = ref<Fields>({});
const data = computed(() => inbox.value);
const error = computed(() => inboxError.value);
const rows = ref<InboxRow[]>([]);
// read: the rows opened here, by the latest they had then (mergeRows)
const read = ref(new Map<string, string>());
const loaded = ref(false); // the first rows are in: before, no "Nothing new"
const unread = computed(() => rows.value.filter((r) => !read.value.has(r.issue.id)).map((r) => r.issue.id));
const names = computed(() => namespaces(rows.value.map((r) => r.issue), fields.value));
const menuOpen = ref(false);
const busy = ref(false);

const remotes = ref<Remote[]>([]);
const failures = ref<Record<string, string>>({});
const checks = computed(() => {
  const incoming = new Map(toPull(remotes.value).map((p) => [p.name, p.issues]));
  return remotes.value.map((r) => ({
    name: r.name,
    when: r.checked ? when(r.checked, true) : 'not yet',
    incoming: incoming.get(r.name) ?? 0,
    failure: failures.value[r.name] ?? '',
  }));
});

// numberOf: the issue's number on a forge it's still on
const numberOf = (r: InboxRow) => forgeNumbers(r.issue)[0] ?? '';

const following = computed(() => (data.value ? followingLine(data.value.following, data.value.views, data.value.roles) : ''));

// reload: the server's rows; dropRead (Refresh) forgets the rows read
// since, else they stay, dimmed, where they were.
async function reload(dropRead: boolean) {
  await loadInbox();
  const fresh = inbox.value?.rows ?? [];
  loaded.value = true;
  if (dropRead) {
    read.value = new Map();
    rows.value = fresh;
    return;
  }
  const m = mergeRows(fresh, rows.value, read.value);
  rows.value = m.rows;
  read.value = m.read;
}

// refresh: the rows, and the remotes checked beside them (all of them when
// force: Refresh). A check changes nothing local: the rows don't wait for
// a remote that's slow to answer.
async function refresh(force: boolean) {
  await Promise.all([check(force), reload(force)]);
}

async function check(force: boolean) {
  busy.value = true;
  try {
    remotes.value = (await getRemotes()).remotes;
    const toFetch = toCheck(remotes.value, Date.now(), force);
    const results = await Promise.allSettled(toFetch.map((n) => remoteAction(n, 'fetch')));
    const failed: Record<string, string> = {};
    results.forEach((res, i) => {
      const name = toFetch[i]!;
      if (res.status === 'rejected') {
        const e = res.reason as unknown;
        failed[name] = checkFailure(name, e instanceof ApiError ? e.code : '', e instanceof Error ? e.message : String(e));
      }
    });
    failures.value = failed;
    remotes.value = (await getRemotes()).remotes;
  } catch {
    // no remotes listed: the rows still show
  } finally {
    busy.value = false;
  }
}

async function pull(name: string) {
  busy.value = true;
  try {
    remotes.value = (await remoteAction(name, 'pull')).remotes;
    delete failures.value[name];
  } catch (e) {
    failures.value = { ...failures.value, [name]: e instanceof Error ? `${name}: couldn't pull: ${e.message}` : String(e) };
  } finally {
    busy.value = false;
  }
  await reload(false);
}

// open: read here; the panel marks it seen once it loads (IssuePanel), as
// it does for an issue opened anywhere else
function open(r: InboxRow) {
  read.value = new Map(read.value).set(r.issue.id, r.latest);
  setIssue(r.issue.id);
}

async function markAll() {
  const ids = unread.value;
  const m = new Map(read.value);
  for (const r of rows.value) if (ids.includes(r.issue.id)) m.set(r.issue.id, r.latest);
  read.value = m;
  await markSeen(ids);
  await loadInbox(); // the header's count: this tab's own write comes back skipped (live.ts)
}

async function saveSettings(views: string[], f: boolean) {
  await setInboxSettings(views, f);
  await reload(false);
}

function toggleView(v: string) {
  const d = data.value;
  if (!d) return;
  const views = d.views.includes(v) ? d.views.filter((x) => x !== v) : [...d.views, v];
  void saveSettings(views, d.fields);
}

// The issue panel: ?issue=<id>, as on the board; j / k go through the rows.
const detailOpen = ref(false);
const detailId = ref('');
const detailRow = computed(() => rows.value.find((r) => r.issue.id === detailId.value));
const index = computed(() => rows.value.findIndex((r) => r.issue.id === detailId.value));
function setIssue(id: string | undefined) {
  void router.replace({ query: { ...route.query, issue: id } });
}
function step(delta: number) {
  const next = rows.value[index.value + delta];
  if (next) void open(next);
}
watch(
  () => route.query.issue,
  (id) => {
    if (typeof id === 'string' && id) {
      detailId.value = id;
      detailOpen.value = true;
    } else detailOpen.value = false;
  },
  { immediate: true },
);

const reloadSoon = debounced(() => void reload(false));
// another identity: another inbox
watch(
  () => me.value?.me?.id,
  (id, was) => {
    if (was && id !== was) void reload(true);
  },
);
let stopLive = () => {};
onMounted(async () => {
  stopLive = onLive((e) => {
    if (e.kind === 'issues' || e.kind === 'inbox') reloadSoon();
  });
  try {
    viewNames.value = Object.keys(await getViews()).sort();
    fields.value = await getFields();
  } catch {
    // no config: no views to follow, fields shown as labels
  }
  await refresh(false);
});
onBeforeUnmount(() => stopLive());
</script>

<style scoped>
.gb-inbox { max-width: 60rem; }
.visually-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.gb-inbox-bar { position: relative; display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2) var(--space-3); padding: var(--space-2) 0; border-top: var(--border-w) solid var(--border); border-bottom: var(--border-w) solid var(--border); }
.gb-inbox-status { display: flex; flex-direction: column; gap: var(--space-1); flex: 1 1 20rem; font-size: .875rem; color: var(--fg-muted); }
.gb-inbox-status strong { color: var(--fg); }
.gb-inbox-fail { color: var(--danger-fg); }
.gb-inbox-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); }
.gb-inbox-menu { position: absolute; right: 0; top: 100%; z-index: 5; width: min(20rem, calc(100vw - 2rem)); margin-top: var(--space-2); padding: var(--space-3); display: flex; flex-direction: column; gap: var(--space-3); background: var(--bg); border: var(--border-w) solid var(--border-strong); }
.gb-inbox-views { display: flex; flex-direction: column; gap: var(--space-2); margin: 0; padding: 0; border: 0; }
.gb-inbox-check { display: flex; gap: var(--space-2); align-items: flex-start; font-size: .875rem; }
.gb-inbox-x { width: 1.75rem; height: 1.75rem; border: var(--border-w) solid var(--border); background: var(--bg); color: var(--fg); cursor: pointer; }
.gb-inbox-need { margin: var(--space-4) 0; }
.gb-inbox-rows { list-style: none; margin: 0; padding: 0; }
.gb-inbox-rows li { border-bottom: var(--border-w) solid var(--border-subtle); }
.gb-inbox-row { display: flex; flex-wrap: wrap; align-items: flex-start; gap: var(--space-2) var(--space-3); width: 100%; padding: var(--space-3) var(--space-2); border: 0; background: transparent; color: var(--fg); font: inherit; text-align: left; cursor: pointer; }
.gb-inbox-row:hover { background: var(--bg-subtle); }
.gb-inbox-dot { width: .5rem; height: .5rem; margin-top: .4rem; border-radius: 50%; background: var(--primary); }
.gb-inbox-main { display: flex; flex-direction: column; gap: var(--space-1); flex: 999 1 24rem; min-width: 0; }
.gb-inbox-title { font-weight: var(--w-bold); }
.gb-inbox-num { font-family: var(--font-mono); font-weight: normal; color: var(--fg-muted); }
.gb-inbox-news { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1) var(--space-2); font-size: .875rem; }
.gb-inbox-chip { padding: 0 .375rem; border: var(--border-w) solid var(--border); border-radius: var(--radius); font-size: .75rem; color: var(--fg-muted); }
.gb-inbox-who { display: flex; gap: 2px; }
.gb-inbox-when { min-width: 5rem; text-align: right; }
.gb-inbox-read { opacity: .55; }
.gb-inbox-read .gb-inbox-dot { background: transparent; }
.gb-inbox-read .gb-inbox-title { font-weight: normal; }
.gb-inbox-empty { padding: var(--space-5) 0; text-align: center; }
</style>
