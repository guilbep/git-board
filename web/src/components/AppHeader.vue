<!-- EPFL header (css/organisms/header.css classes, used directly: the Vue
     kit's EpflHeader hardcodes a "Search EPFL" box, an EN/FR/DE switch and
     href="#" links, none of which fit this app). git-board's logo, then the
     repository's name; the main navigation, Issues first (the saved views
     are its tabs, IndexPage).
     The right side: Sync counting what waits to go out, the search, one menu for you
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
      <router-link to="/work" active-class="active">Work</router-link>
      <router-link to="/insights" active-class="active">Insights</router-link>
      <!-- a release's page (/releases/:tag) is under Releases too -->
      <router-link to="/releases" active-class="active" :class="{ active: releasesHere }" :aria-current="releasesHere ? 'page' : undefined">Releases</router-link>
      <router-link to="/inbox" active-class="active">Inbox<span v-if="inboxCount" class="nav-count" :aria-label="`, ${inboxCount} new`">{{ inboxCount }}</span></router-link>
    </nav>
    <div class="app-right">
      <!-- Sync, with the count of what waits to go out to the remotes and forges -->
      <router-link to="/settings/sync" class="gb-sync" :class="{ 'gb-sync-waiting': w.send || w.running }" :title="`${waitWords(w)} · ${w.checked ? `checked ${when(w.checked, true)}` : 'not checked yet'}`">
        {{ w.running ? 'Syncing…' : 'Sync' }}<span v-if="w.send && !w.running" class="nav-count" :aria-label="`, ${w.send} to send`">{{ w.send.toLocaleString() }}</span>
      </router-link>
      <button type="button" class="gb-iconbtn" aria-label="Filter or jump to" title="Filter or jump to (⌘K)" aria-keyshortcuts="Meta+K Control+K" @click="paletteOpen = true">
        <GbIcon name="search" />
      </button>
      <details v-if="me?.me" ref="menu" class="gb-menu" @keydown.esc="close(menu)">
        <summary class="gb-iconbtn" :aria-label="`${me.me.name}: you, settings, shortcuts`">
          <span class="gb-avatar gb-avatar-dark" aria-hidden="true">{{ initials(me.me.name) }}</span>
        </summary>
        <div class="gb-menu-panel" @click="close(menu)">
          <p class="gb-menu-who"><b>{{ me.me.name }}</b><code v-if="me.me.login">{{ me.me.login }}</code></p>
          <router-link to="/settings/you">You</router-link>
          <router-link to="/settings">Settings</router-link>
          <router-link to="/settings/sync">Sync</router-link>
          <button type="button" @click="helpOpen = true">Keyboard shortcuts <kbd>?</kbd></button>
          <p class="gb-menu-foot">{{ w.checked ? `Forges checked ${when(w.checked, true)}` : 'Not checked yet' }}</p>
        </div>
      </details>
      <router-link v-else to="/settings" class="gb-iconbtn" active-class="active" aria-label="Settings" title="Settings">
        <GbIcon name="settings" />
      </router-link>
    </div>
  </header>
  <p v-if="me?.unswitched" class="epfl-alert epfl-alert-warning app-unswitched" role="alert">
    <b>This repository’s issues are still git-bug’s</b> (in <code>refs/bugs</code>), so the board shows none of them.
    Move them once, in a terminal: <code>git board switch</code> says what it would do, <code>git board switch --apply</code> does it, then <code>git board sync</code>.
  </p>
  <KeyboardHelp />
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
import CommandPalette from '@/components/CommandPalette.vue';
import { paletteOpen } from '@/commands';
import { forgeStatus, loadForges } from '@/forges';
import { initials, loadMe, me, when } from '@/me';
import { loadSyncStatus, syncStatus } from '@/sync';
import { loadRemotes, remotes, repo } from '@/remotes';
import { faviconDot, waitWords, waiting } from '@/ways';
import { setFavicon } from '@/favicon';
import GbIcon from '@/components/GbIcon.vue';
import { inbox, loadInbox } from '@/inbox';
import { debounced, onLive } from '@/live';

// the saved views, for the palette's jumps
defineProps<{ views: string[] }>();
const route = useRoute();
// the issues, or one issue's page
const issuesHere = computed(() => route.path === '/' || route.path.startsWith('/issues/'));
// Releases, or one release's page
const releasesHere = computed(() => route.path === '/releases' || route.path.startsWith('/releases/'));

const w = computed(() => waiting(remotes.value, forgeStatus.value, syncStatus.value));
// the tab's dot: red to send, teal to get
watch(() => faviconDot(w.value), setFavicon, { immediate: true });

const inboxCount = computed(() => inbox.value?.rows.length ?? 0);
// the repository's name, else git-board's; the tab says it too, with the
// inbox's count: "(3) co2-calculator"
const title = computed(() => repo.value || 'git-board');
watch([inboxCount, title], ([n, t]) => (document.title = n ? `(${n}) ${t}` : t), { immediate: true });
// ponytail: every issues event reloads the inbox, debounced; the server
// recomputes it each time (inbox.go says when to keep it instead)
const reloadInbox = debounced(() => void loadInbox());
// a sync elsewhere, or an edit: what waits changes
const reloadWaiting = debounced(() => {
  void loadRemotes();
  void loadForges();
  void loadSyncStatus();
});
// another identity (here, or in another tab: LiveNotice reloads me): another inbox
watch(
  () => me.value?.me?.id,
  (id, was) => {
    if (was && id !== was) void loadInbox();
  },
);

const menu = ref<HTMLDetailsElement | null>(null);

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
    paletteOpen.value = !paletteOpen.value;
    return;
  }
  if (e.key !== '?' || e.ctrlKey || e.metaKey || e.altKey) return;
  if (e.target instanceof Element && e.target.closest('input, textarea, select, [contenteditable]')) return;
  e.preventDefault();
  helpOpen.value = true;
}
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey);
  document.removeEventListener('click', fold);
  stopLive();
});

onMounted(() => {
  window.addEventListener('keydown', onKey);
  document.addEventListener('click', fold);
  if (!syncStatus.value) void loadSyncStatus();
  if (!forgeStatus.value) void loadForges();
  if (!remotes.value) void loadRemotes();
  if (!me.value) void loadMe();
  if (!inbox.value) void loadInbox();
  stopLive = onLive((e) => {
    if (e.kind === 'issues' || e.kind === 'inbox') reloadInbox();
    if (e.kind === 'issues' || e.kind === 'sync') reloadWaiting();
  });
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

/* the header stays on top while the page scrolls */
.app-header { position: sticky; top: 0; z-index: 30; }
.app-unswitched { margin: 0; border-radius: 0; }
/* the nav takes what's left, on one line */
.app-header .epfl-nav { min-width: 0; align-items: center; }
.app-header .epfl-nav > a { flex: none; white-space: nowrap; }

/* the menu's panel, as the column chooser's */
.gb-menu-panel { position: absolute; z-index: 15; width: 16rem; padding: .25rem 0; background: var(--bg); border: var(--border-w) solid var(--border-strong); box-shadow: var(--shadow); }

.app-right { display: flex; align-items: center; gap: .75rem; margin-left: auto; }
.gb-iconbtn { font: inherit; background: none; cursor: pointer; display: inline-flex; align-items: center; justify-content: center; width: 2.75rem; height: 2.75rem; padding: 0; border: 1px solid var(--border-strong); color: var(--fg); }
.gb-iconbtn:hover, .gb-iconbtn.active, .gb-menu[open] .gb-iconbtn { border-color: var(--fg); }
.gb-iconbtn:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }

/* Sync: a quiet link when nothing waits, a small button when something does */
.gb-sync { display: inline-flex; align-items: center; min-height: 2.75rem; padding: 0 .5rem; color: var(--fg-muted); font-size: .875rem; text-decoration: none; white-space: nowrap; }
.gb-sync:hover { color: var(--fg); }
.gb-sync-waiting { min-height: 2.25rem; padding: .25rem .6rem; border: 1px solid var(--border-strong); color: var(--fg); font-weight: 700; }
.gb-sync-waiting:hover { border-color: var(--fg); }

.gb-menu { position: relative; }
.gb-menu summary { list-style: none; }
.gb-menu summary::-webkit-details-marker { display: none; }
.gb-menu-panel { right: 0; margin-top: .375rem; font-size: var(--fs-sm); }
.gb-menu-panel > a, .gb-menu-panel > button { display: flex; justify-content: space-between; align-items: center; box-sizing: border-box; width: 100%; padding: .5rem .875rem; border: 0; background: none; color: var(--fg); font: inherit; text-align: left; text-decoration: none; cursor: pointer; }
.gb-menu-panel > a:hover, .gb-menu-panel > button:hover { color: var(--primary); }
.gb-menu-panel kbd { padding: 0 .3rem; border: 1px solid var(--border); font-family: var(--font-mono); font-size: .75rem; }
.gb-menu-who { display: flex; flex-direction: column; gap: .125rem; margin: 0; padding: .5rem .875rem .625rem; border-bottom: 1px solid var(--border-subtle); }
.gb-menu-who code { color: var(--fg-muted); font-size: .75rem; }
.gb-menu-foot { margin: 0; padding: .5rem .875rem; border-top: 1px solid var(--border-subtle); color: var(--fg-muted); font-size: .8125rem; }

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
}
</style>
