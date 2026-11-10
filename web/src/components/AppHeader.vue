<!-- EPFL header (css/organisms/header.css classes, used directly: the Vue
     kit's EpflHeader hardcodes a "Search EPFL" box, an EN/FR/DE switch and
     href="#" links, none of which fit this app). git-board's logo, then the
     repository's name; the main navigation, Issues first (the saved views
     are its tabs, IndexPage).
     The right side: Sync, badged as an issue's forges are, the search field, one menu for you
     (the canvas's H1 artboard).
     The logo is drawn as a mask over var(--primary): git-board.svg is drawn
     black, the logo is red. Same file, same shape, only the color. -->
<template>
  <header class="epfl-header app-header" role="banner">
    <router-link class="epfl-logo" to="/">
      <span class="logo-mark" role="img" aria-label="git-board" />
    </router-link>
    <span class="app-name">{{ title }}</span>
    <nav class="epfl-nav" aria-label="Main">
      <!-- an issue's page (/issues/:id) is under Issues too -->
      <router-link to="/" :class="{ active: issuesHere }" :aria-current="issuesHere ? 'page' : undefined">Issues</router-link>
      <router-link to="/inbox" active-class="active">Inbox<span v-if="inboxCount" class="nav-count" :aria-label="`, ${inboxCount} new`">{{ inboxCount }}</span></router-link>
      <!-- the cycle's work units: with the cycle on alone (experimental) -->
      <router-link v-if="cycleOn" to="/work" active-class="active">Work</router-link>
      <router-link to="/insights" active-class="active">Insights</router-link>
      <!-- a release's page (/releases/:tag) is under Releases too -->
      <router-link to="/releases" active-class="active" :class="{ active: releasesHere }" :aria-current="releasesHere ? 'page' : undefined">Releases</router-link>
      <!-- Settings, as a cog: every /settings page is under it -->
      <router-link to="/settings" class="nav-cog" :class="{ active: settingsHere }" :aria-current="settingsHere ? 'page' : undefined" aria-label="Settings" title="Settings"><GbIcon name="settings" /></router-link>
    </nav>
    <!-- the tip, when the header has room for it (else the strip below) -->
    <div ref="tipSlot" class="app-tip"><TipLine v-if="tipWhere === 'header'" /></div>
    <div class="app-right">
      <!-- a new board: the steps left (GET /api/state), to the Start page -->
      <router-link v-if="setupLeft && route.path !== '/start'" to="/start" class="app-setup" data-test="setup-chip" :title="setupTitle">
        Set up · {{ setupLeft }} {{ setupLeft === 1 ? 'step' : 'steps' }} left
      </router-link>
      <!-- Sync, drawn as an issue's forge button (ForgeIcons): a badge
           while a run goes on, a red one when the last one failed -->
      <router-link to="/settings/sync" class="gb-iconbtn gb-sync" active-class="active" :aria-label="`Sync: ${syncWords}`" :title="`Sync: ${syncWords}`">
        <GbIcon name="sync" />
        <span v-if="run?.state === 'running'" class="gb-fbadge is-syncing" aria-hidden="true"><GbIcon name="sync" small /></span>
        <span v-else-if="run?.state === 'failed'" class="gb-fbadge is-conflict" aria-hidden="true"><GbIcon name="warn" small /></span>
      </router-link>
      <!-- the palette, as a search field (Zensical's), and the shortcut
           list's key beside ⌘K: two buttons in one field -->
      <div class="gb-search" role="group" aria-label="Search and shortcuts">
        <button type="button" class="gb-search-go" aria-label="Search: filter or jump to" title="Filter the issues, jump to one, or run a command" aria-keyshortcuts="Meta+K Control+K" @click="paletteOpen = true">
          <GbIcon name="search" />
          <span class="gb-search-label">Search</span>
          <kbd class="gb-search-key" aria-hidden="true">{{ mac ? '⌘K' : 'Ctrl K' }}</kbd>
        </button>
        <button type="button" class="gb-search-key gb-search-help" aria-label="Keyboard shortcuts" title="Keyboard shortcuts (?)" aria-keyshortcuts="?" @click="helpOpen = true">?</button>
      </div>
      <!-- you: the account card (A2), Settings being the nav's cog -->
      <details v-if="me?.me" ref="menu" class="gb-menu" @keydown.esc="close(menu)">
        <summary class="gb-iconbtn" :aria-label="`${me.me.name}: your account`" :title="me.me.name">
          <GbAvatar :name="me.me.name" :src="me.me.avatar_url" dark />
        </summary>
        <AccountCard :me="me.me" class="gb-menu-panel" @close="close(menu)" />
      </details>
      <router-link v-else to="/settings/you" class="gb-iconbtn" aria-label="Choose who you write as" title="Choose who you write as">
        <GbIcon name="user" />
      </router-link>
    </div>
  </header>
  <div v-if="tipWhere === 'strip'" class="app-tipstrip"><TipLine /></div>
  <!-- a fresh clone's first open: what the server pulled from origin, or why it couldn't -->
  <p v-if="firstPull && !firstPullHidden" class="epfl-alert app-firstpull" :class="firstPull.error ? 'epfl-alert-warning' : 'epfl-alert-success'" role="status">
    <template v-if="firstPull.error"><b>This clone has no board yet.</b> {{ firstPull.error }}. Pull it from Sync once {{ firstPull.remote }} answers.</template>
    <template v-else><b>This clone had no board:</b> {{ firstPull.issues }} issue{{ firstPull.issues === 1 ? '' : 's' }} pulled from {{ firstPull.remote }}.</template>
    <button type="button" class="gb-linkbtn" @click="firstPullHidden = true">Hide</button>
  </p>
  <KeyboardHelp />
  <KeyLine />
  <CommandPalette :views="views" />
  <LiveNotice />
</template>

<script setup lang="ts">
import LiveNotice from '@/components/LiveNotice.vue';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { helpOpen } from '@/shortcuts';
import '@/theme'; // keeps <html data-theme> in step with the choice
import KeyboardHelp from '@/components/KeyboardHelp.vue';
import KeyLine from '@/components/KeyLine.vue';
import TipLine from '@/components/TipLine.vue';
import { noteUse } from '@/keys';
import CommandPalette from '@/components/CommandPalette.vue';
import { openShortcuts, paletteOpen } from '@/commands';
import { board, cycleOn, loadMe, me } from '@/me';
import { loadSync, onStep, syncStatus } from '@/sync';
import { runWords } from '@/ways';
import { setFavicon } from '@/favicon';
import GbIcon from '@/components/GbIcon.vue';
import GbAvatar from '@/components/GbAvatar.vue';
import AccountCard from '@/components/AccountCard.vue';
import { inbox, loadInbox } from '@/inbox';
import { debounced, onLive } from '@/live';
import { clientId, getState, type BoardState } from '@/api';

// the saved views, for the palette's jumps
defineProps<{ views: string[] }>();
const route = useRoute();
// the issues, or one issue's page
const issuesHere = computed(() => route.path === '/' || route.path.startsWith('/issues/'));
// every Settings page, under the cog
const settingsHere = computed(() => route.path.startsWith('/settings'));
const run = computed(() => syncStatus.value?.run ?? null);
// the tab's dot: red when the last sync failed
watch(() => (run.value?.state === 'failed' ? 'send' : null), setFavicon, { immediate: true });
const syncWords = computed(() => runWords(run.value));
const firstPull = computed(() => board.value?.first_pull);
const firstPullHidden = ref(false);
// Releases, or one release's page
const releasesHere = computed(() => route.path === '/releases' || route.path.startsWith('/releases/'));
// the palette's key, as this keyboard writes it
const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

const inboxCount = computed(() => (inbox.value ? inbox.value.follows.length + inbox.value.opened.length : 0));
// the repository's folder, else git-board's; the tab says it too, with the
// inbox's count: "(3) co2-calculator"
const title = computed(() => board.value?.dir.split('/').filter(Boolean).at(-1) || 'git-board');
watch([inboxCount, title], ([n, t]) => (document.title = n ? `(${n}) ${t}` : t), { immediate: true });
// ponytail: every issues event reloads the inbox, debounced; the server
// recomputes it each time (inbox.go says when to keep it instead)
const reloadInbox = debounced(() => void loadInbox());
// a run started or ended
const reloadSync = debounced(() => void loadSync(), 200);
// another identity (here, or in another tab: LiveNotice reloads me): another inbox
watch(
  () => me.value?.me?.id,
  (id, was) => {
    if (was && id !== was) void loadInbox();
  },
);

// the board's state: while it is new, the chip says how many steps are left
const state = ref<BoardState | null>(null);
const setupLeft = computed(() => (state.value?.phase === 'new' ? state.value.next.length : 0));
const setupTitle = computed(() => state.value?.next.map((a) => a.say).join('\n') ?? '');
const reloadState = debounced(() => void getState().then((s) => (state.value = s)).catch(() => {}), 300);

const menu = ref<HTMLDetailsElement | null>(null);

// where the tip goes (docs/prd/header-help.md, 4): the header from 80rem
// when its free space takes a tip, a strip under it from 48rem, else none
const tipSlot = ref<HTMLElement | null>(null);
const slotWidth = ref(0);
const viewport = ref(window.innerWidth);
const remPx = () => parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
const tipWhere = computed<'header' | 'strip' | null>(() => {
  const rem = remPx();
  if (viewport.value >= 80 * rem && slotWidth.value >= 24 * rem) return 'header';
  return viewport.value >= 48 * rem ? 'strip' : null;
});
const slotWatch = new ResizeObserver(([e]) => (slotWidth.value = e?.contentRect.width ?? 0));
const onResize = () => (viewport.value = window.innerWidth);

// a menu folds on a click elsewhere, on a choice in it, and on Escape
function close(d: HTMLDetailsElement | null) {
  if (!d?.open) return;
  d.open = false;
  d.querySelector('summary')?.focus();
}
const fold = (e: MouseEvent) => {
  const d = menu.value;
  if (d?.open && !d.contains(e.target as Node)) d.open = false;
};

let stopLive = () => {};

// ⌘K (Ctrl+K) opens the palette, even from a field; ? the shortcuts,
// unless typing
function onKey(e: KeyboardEvent) {
  if (e.key.toLowerCase() === 'k' && (e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey) {
    e.preventDefault();
    if (!paletteOpen.value) noteUse('palette');
    paletteOpen.value = !paletteOpen.value;
    return;
  }
  if (e.key !== '?' || e.ctrlKey || e.metaKey || e.altKey || e.defaultPrevented) return;
  if (e.target instanceof Element && e.target.closest('input, textarea, select, [contenteditable]')) return;
  e.preventDefault();
  noteUse('help');
  openShortcuts();
}
onBeforeUnmount(() => {
  slotWatch.disconnect();
  window.removeEventListener('resize', onResize);
  window.removeEventListener('keydown', onKey);
  document.removeEventListener('click', fold);
  stopLive();
});

onMounted(() => {
  if (tipSlot.value) slotWatch.observe(tipSlot.value);
  window.addEventListener('resize', onResize);
  window.addEventListener('keydown', onKey);
  document.addEventListener('click', fold);
  if (!syncStatus.value) void loadSync();
  if (!me.value) void loadMe();
  // the inbox's count after what the page shows first (it reads every
  // issue you follow)
  if (!inbox.value) setTimeout(() => void loadInbox(), 600);
  reloadState();
  stopLive = onLive((e) => {
    const own = e.by === clientId;
    if (!own && (e.kind === 'issue' || e.kind === 'inbox' || e.kind === 'resync')) reloadInbox();
    if (e.kind === 'sync') reloadSync();
    // a state reads every forge's tokens: after setup, only a file changed
    // can make the board new again
    const setup = !state.value || setupLeft.value > 0;
    if (e.kind === 'config' || (setup && (e.kind === 'me' || e.kind === 'sync' || e.kind === 'resync' || e.kind === 'issue'))) reloadState();
    onStep(e);
  }, { own: true });
});
</script>

<style scoped>
.logo-mark {
  display: block;
  height: 2rem;
  aspect-ratio: 1;
  background: var(--primary);
  mask: url('/git-board.svg') center / contain no-repeat;
}

.app-setup { padding: .25rem .625rem; border: 1px solid var(--primary); color: var(--primary); font-size: .8125rem; font-weight: 700; text-decoration: none; white-space: nowrap; }
.app-setup:hover { background: var(--primary); color: var(--bg); }

/* the header stays on top while the page scrolls */
.app-header { position: sticky; top: 0; z-index: 30; }
.app-unswitched, .app-firstpull { margin: 0; border-radius: 0; }
/* the nav takes what's left, on one line */
.app-header .epfl-nav { flex: 0 1 auto; min-width: 0; align-items: center; }
.app-header .epfl-nav > a { flex: none; white-space: nowrap; }

/* the menu's panel, as the column chooser's */
.gb-menu-panel { position: absolute; z-index: 15; width: 18.75rem; background: var(--bg); border: var(--border-w) solid var(--border-strong); box-shadow: var(--shadow); }

.app-right { display: flex; align-items: center; gap: .75rem; }
/* the space between the nav and the right side: the tip's, when it fits */
.app-tip { flex: 1; display: flex; justify-content: flex-end; min-width: 0; padding: 0 1rem; }
.app-tipstrip { display: flex; justify-content: flex-end; align-items: center; min-height: 1.875rem; padding: 0 var(--gutter, 1.75rem); border-bottom: var(--border-w) solid var(--border-subtle); background: var(--bg-subtle); }
.gb-iconbtn { font: inherit; background: none; cursor: pointer; display: inline-flex; align-items: center; justify-content: center; width: 2.75rem; height: 2.75rem; padding: 0; border: 1px solid var(--border-strong); color: var(--fg); }
.gb-iconbtn:hover, .gb-iconbtn.active, .gb-menu[open] .gb-iconbtn { border-color: var(--fg); }
.gb-iconbtn:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }

/* Sync: the issue's forge button, its badges from head.css */
.gb-sync { position: relative; text-decoration: none; }
.gb-sync:hover { color: var(--primary); }
.gb-sync .gb-fcount, .gb-sync .gb-fbadge { top: -0.375rem; right: -0.375rem; }
.gb-sync .gb-fcount.is-under { top: auto; bottom: -0.375rem; }

/* the palette, drawn as a search field: grey, its keys at the end; the
   ? keycap is a button of its own */
.gb-search { display: inline-flex; align-items: center; width: 15rem; height: 2.75rem; border: var(--border-w) solid transparent; border-radius: var(--radius); background: var(--bg-subtle); }
.gb-search:hover { border-color: var(--border-strong); }
.gb-search-go { flex: 1; display: inline-flex; align-items: center; gap: .5rem; height: 100%; min-width: 0; padding: 0 .375rem 0 .75rem; border: 0; background: none; color: var(--fg-muted); font: inherit; font-size: var(--fs-sm); cursor: pointer; text-align: left; }
.gb-search-go .gb-icon { flex: none; width: 1.125rem; height: 1.125rem; color: var(--fg); }
.gb-search-go:hover { color: var(--fg); }
.gb-search-label { flex: 1; }
.gb-search-key { padding: .0625rem .375rem; border: var(--border-w) solid var(--border); border-bottom-width: .125rem; border-radius: var(--radius); background: var(--bg); color: var(--fg-muted); font-family: var(--font-sans); font-size: .75rem; line-height: 1.3; }
.gb-search-help { margin-right: .5rem; min-width: 1.5rem; cursor: pointer; font-weight: var(--w-bold); }
.gb-search-help:hover { border-color: var(--fg); color: var(--fg); }
.gb-search-go:focus-visible, .gb-search-help:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }

/* Settings, the nav's last entry: a cog, the red dot under it when there */
.app-header .epfl-nav > a.nav-cog { display: inline-flex; align-items: center; }
.nav-cog .gb-icon { width: 1.25rem; height: 1.25rem; }

.gb-menu { position: relative; }
.gb-menu summary { list-style: none; }
.gb-menu summary::-webkit-details-marker { display: none; }
.gb-menu-panel { right: 0; margin-top: .375rem; font-size: var(--fs-sm); }

.nav-count {
  margin-left: var(--space-1);
  padding: 0 0.375rem;
  font-size: 0.75rem;
  font-weight: var(--w-bold);
  color: var(--on-primary);
  background: var(--primary);
  border-radius: var(--radius);
}

.app-name {
  margin-left: var(--space-3);
  padding-left: var(--space-3);
  border-left: var(--border-w) solid var(--border);
  font-weight: var(--w-bold);
  font-size: var(--fs-h5);
  line-height: var(--lh-tight);
  white-space: nowrap;
}

@media (max-width: 47.99rem) {
  /* The nav moves to its own row, scrolling sideways if it must. */
  .app-header {
    flex-wrap: wrap;
    height: auto;
    padding: var(--space-3) var(--space-3) 0;
  }
  .app-header .epfl-nav {
    flex-basis: 100%;
    margin-left: calc(-1 * var(--space-3));
    margin-right: calc(-1 * var(--space-3));
    padding: 0 var(--space-2);
    overflow-x: auto;
  }
  .app-right { order: 2; }
  .app-tip { display: none; }
  .gb-search { width: 2.75rem; }
  .gb-search-go { justify-content: center; padding: 0; }
  .gb-search-label, .gb-search-key { display: none; }
}
</style>
