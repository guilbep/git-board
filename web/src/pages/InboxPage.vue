<!-- The inbox (GET /api/inbox, as git board status): what happened that
     concerns you on the issues you follow, then on those you opened,
     newest first. Refresh also fetches origin (nothing local changes) and
     offers Pull; opening a row opens the issue and marks it read. Read
     rows stay, dimmed, until Refresh, so the list doesn't jump. -->
<template>
  <AppHeader :views="viewNames" />
  <main class="gb-page gb-inbox">
    <div class="epfl-eyebrow">Inbox</div>
    <h1 class="h3">What happened that concerns you</h1>
    <p class="gb-lead">On the issues you follow, since you last opened them.</p>

    <div class="gb-inbox-bar">
      <div class="gb-inbox-status" role="status">
        <span v-if="check" :class="{ 'gb-inbox-fail': check.failure }">
          <template v-if="check.failure">{{ check.failure }}</template>
          <template v-else>
            Checked <strong>{{ check.name }}</strong>: {{ check.line }}
            <template v-if="check.incoming">
              · <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm gb-inbox-pull" :disabled="busy" @click="pull(check.name)">Pull</button>
            </template>
          </template>
        </span>
        <span v-else>{{ remote ? `${remote}: not checked from here yet` : 'No remote to check' }}</span>
      </div>
      <div class="gb-inbox-actions">
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="refresh()">Refresh</button>
        <button v-if="unread.length" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="markAll">Mark all read</button>
      </div>
    </div>

    <p v-if="error?.code === 'no-identity'" class="gb-inbox-need">
      The inbox shows what concerns the identity you write as, and none is chosen here yet.
      <router-link to="/settings/you">Choose who you write as</router-link>
    </p>
    <p v-else-if="error" class="gb-error">{{ error.message }}</p>
    <p v-if="writeError" class="gb-error" role="alert">{{ writeError }}</p>

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
            <GbAvatar v-for="p in r.who" :key="p.id" :name="p.name" :src="p.avatar_url" small dark :title="p.name" />
          </span>
          <span class="gb-hint gb-inbox-when">{{ when(r.latest, true) }}</span>
        </button>
      </li>
    </ul>
    <div v-else-if="loaded && !error" class="gb-inbox-empty">
      <h2 class="h4">Nothing new for you</h2>
      <p class="gb-hint">Issues you open, comment on or are named in show here when they change.</p>
    </div>
  </main>

  <GbDialog
    v-model="detailOpen"
    :position="panelMode === 'side' ? 'right' : 'standard'"
    :full-height="panelMode === 'side'"
    :class="`gb-dialog-${panelMode}`"
    :blur="panelMode === 'wide'"
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
  </GbDialog>
</template>

<script setup lang="ts">
import GbDialog from '@/components/GbDialog.vue';
import GbAvatar from '@/components/GbAvatar.vue';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import AppHeader from '@/components/AppHeader.vue';
import GbIcon from '@/components/GbIcon.vue';
import IssuePanel from '@/components/IssuePanel.vue';
import { getFields, markSeen, remoteAction, type InboxRow } from '@/api';
import { configModel, loadConfigModel } from '@/configmodel';
import { namespaces } from '@/facets';
import type { Fields } from '@/fields';
import { forgeNumbers } from '@/idlabel';
import { inbox, inboxError, loadInbox } from '@/inbox';
import { mergeRows } from '@/inboxcheck';
import { debounced, onLive } from '@/live';
import { me, when } from '@/me';
import { message } from '@/issue';
import { panelMode } from '@/panelsize';
import { loadSync, syncStatus } from '@/sync';
import { remoteWords } from '@/ways';

const route = useRoute();
const router = useRouter();

const viewNames = computed(() => configModel.value?.view_order ?? []);
const fields = ref<Fields>({});
const error = computed(() => inboxError.value);
const rows = ref<InboxRow[]>([]);
// read: the rows opened here, by the latest they had then (mergeRows)
const read = ref(new Map<string, string>());
const loaded = ref(false); // the first rows are in: before, no "Nothing new"
const unread = computed(() => rows.value.filter((r) => !read.value.has(r.issue.id)).map((r) => r.issue.id));
const names = computed(() => namespaces(rows.value.map((r) => r.issue), fields.value));
const busy = ref(false);

// origin, else the first remote: what Refresh fetches
const remote = computed(() => {
  const rs = syncStatus.value?.remotes ?? [];
  return rs.includes('origin') ? 'origin' : (rs[0] ?? '');
});
const check = ref<{ name: string; line: string; incoming: number; failure: string } | null>(null);

// numberOf: the issue's number on a forge it's still on
const numberOf = (r: InboxRow) => forgeNumbers(r.issue)[0] ?? '';

// the rows: what you follow, then what you opened, newest first
const rowsOf = () => [...(inbox.value?.follows ?? []), ...(inbox.value?.opened ?? [])].sort((a, b) => Date.parse(b.latest) - Date.parse(a.latest));

// reload: the server's rows; dropRead (Refresh) forgets the rows read
// since, else they stay, dimmed, where they were.
async function reload(dropRead: boolean) {
  await loadInbox();
  const fresh = rowsOf();
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

// refresh: the rows, and origin fetched beside them (nothing local
// changes): the rows don't wait for a remote that's slow to answer
async function refresh() {
  await Promise.all([fetchRemote(), reload(true)]);
}

async function fetchRemote() {
  if (!syncStatus.value) await loadSync();
  const name = remote.value;
  if (!name) return;
  busy.value = true;
  try {
    const r = await remoteAction(name, 'fetch');
    check.value = { name, line: remoteWords(r), incoming: r.issues?.receive ?? 0, failure: '' };
  } catch (e) {
    check.value = { name, line: '', incoming: 0, failure: `${name}: couldn't check: ${message(e)}` };
  } finally {
    busy.value = false;
  }
}

async function pull(name: string) {
  busy.value = true;
  try {
    const r = await remoteAction(name, 'pull');
    check.value = { name, line: remoteWords(r), incoming: 0, failure: '' };
  } catch (e) {
    check.value = { name, line: '', incoming: 0, failure: `${name}: couldn't pull: ${message(e)}` };
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

// a write refused (Mark all read, a follow): said above the rows
const writeError = ref('');

async function markAll() {
  const ids = unread.value;
  const before = read.value;
  const m = new Map(before);
  for (const r of rows.value) if (ids.includes(r.issue.id)) m.set(r.issue.id, r.latest);
  read.value = m;
  writeError.value = '';
  try {
    await markSeen(ids);
  } catch (e) {
    read.value = before; // not read after all
    writeError.value = `Not marked read: ${message(e)}`;
    return;
  }
  await loadInbox(); // the header's count: this tab's own write comes back skipped (live.ts)
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
    if (e.kind === 'issue' || e.kind === 'inbox' || e.kind === 'resync') reloadSoon();
  });
  try {
    [, fields.value] = await Promise.all([loadConfigModel(), getFields()]);
  } catch {
    // no config: fields shown as labels
  }
  await reload(true);
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
.gb-inbox-menu { position: absolute; right: 0; top: 100%; z-index: 5; width: min(20rem, calc(100vw - 2rem)); margin-top: var(--space-2); padding: var(--space-3); display: flex; flex-direction: column; gap: var(--space-3); background: var(--bg); border: var(--border-w) solid var(--border-strong); box-shadow: var(--shadow); }
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
.gb-inbox-empty .gb-hint { justify-content: center; }
</style>
