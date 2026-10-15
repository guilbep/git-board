<!-- EPFL header (css/organisms/header.css classes, used directly: the Vue
     kit's EpflHeader hardcodes a "Search EPFL" box, an EN/FR/DE switch and
     href="#" links, none of which fit this app). The saved views are the
     main navigation; the active one carries the signature red dot.
     The logo is drawn as a mask over var(--primary): the vendored
     epfl-logo.svg carries no fill (it renders black as an <img>), while the
     brand logo is red on white. Same file, same shape, only the color. -->
<template>
  <header class="epfl-header app-header" role="banner">
    <router-link class="epfl-logo" to="/">
      <span class="logo-mark" role="img" aria-label="EPFL" />
    </router-link>
    <span class="app-name">git-board</span>
    <nav class="epfl-nav" aria-label="Saved views">
      <router-link
        v-for="name in views"
        :key="name"
        :to="{ path: '/', query: { view: name } }"
        :class="{ active: name === active }"
        :aria-current="name === active ? 'page' : undefined"
      >{{ name }}</router-link>
      <router-link to="/insights" active-class="active">Insights</router-link>
      <!-- The count is the last check's: the header never reads the Project. -->
      <router-link
        v-if="syncStatus?.project"
        to="/conflicts"
        active-class="active"
        aria-current-value="page"
      >Conflicts<span v-if="conflicts" class="nav-count" :aria-label="`, ${conflicts} held`">{{ conflicts }}</span></router-link>
    </nav>
    <div class="app-right">
      <button type="button" class="gb-jump" title="Jump to an issue or a command (⌘K)" aria-keyshortcuts="Meta+K Control+K" @click="paletteOpen = true">
        <GbIcon name="search" small /><span class="gb-jump-label">Jump to…</span><kbd>{{ mod }}K</kbd>
      </button>
      <!-- shown once a bridge has sync enabled; live while a run goes on -->
      <router-link v-if="forgeStatus?.bridges.length" to="/sync" class="gb-synclink" :class="{ 'gb-synclink-run': running }" :title="syncTitle">
        <GbIcon name="sync" small />
        <span>{{ running ? 'Syncing…' : 'Sync' }}</span>
        <span v-if="lastRun" class="gb-hint">{{ lastRun }}</span>
      </router-link>
      <router-link v-if="me?.me" to="/settings" class="gb-me" :title="`You write as ${me.me.name}. Settings`">
        <span class="gb-avatar gb-avatar-sm gb-avatar-dark" aria-hidden="true">{{ initials(me.me.name) }}</span>
        <span class="gb-me-name">{{ me.me.name }}</span>
      </router-link>
      <button type="button" class="gb-settingslink" aria-label="Keyboard shortcuts (?)" title="Keyboard shortcuts (?)" @click="helpOpen = true">?</button>
      <router-link to="/settings" class="gb-settingslink" active-class="active" aria-label="Settings" title="Settings">
        <GbIcon name="settings" small />
      </router-link>
    </div>
  </header>
  <KeyboardHelp />
  <CommandPalette :views="views" />
  <LiveNotice />
</template>

<script setup lang="ts">
import LiveNotice from '@/components/LiveNotice.vue';
import { computed, onBeforeUnmount, onMounted } from 'vue';
import { helpOpen } from '@/shortcuts';
import '@/theme'; // keeps <html data-theme> in step with the choice
import KeyboardHelp from '@/components/KeyboardHelp.vue';
import CommandPalette from '@/components/CommandPalette.vue';
import { paletteOpen } from '@/commands';
import { forgeStatus, loadForges } from '@/forges';
import { initials, loadMe, me, when } from '@/me';
import { loadSyncStatus, syncStatus } from '@/sync';
import GbIcon from '@/components/GbIcon.vue';

defineProps<{ views: string[]; active: string }>();

const conflicts = computed(() => syncStatus.value?.last?.conflicts.length ?? 0);
const running = computed(() => forgeStatus.value?.job?.state === 'running');
// the button says Sync; its tooltip, with what: "Sync with gh (GitHub), gl (GitLab)"
const forgeNames: Record<string, string> = { github: 'GitHub', gitlab: 'GitLab', gitea: 'Gitea' };
const syncTitle = computed(
  () => `Sync with ${(forgeStatus.value?.bridges ?? []).map((b) => `${b.name} (${forgeNames[b.target] ?? b.target})`).join(', ')}`,
);
const lastRun = computed(() => {
  const j = forgeStatus.value?.job;
  if (!j || running.value) return '';
  return `${j.dry_run ? 'dry run' : 'last run'} ${when(j.ended ?? j.started)}`;
});

const mod = /Mac/.test(navigator.platform) ? '⌘' : 'Ctrl+';

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
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));

onMounted(() => {
  window.addEventListener('keydown', onKey);
  if (!syncStatus.value) void loadSyncStatus();
  if (!forgeStatus.value) void loadForges();
  if (!me.value) void loadMe();
});
</script>

<style scoped>
.logo-mark {
  display: block;
  height: 2rem;
  aspect-ratio: 182.4 / 53;
  background: var(--primary);
  mask: url('/epfl-logo.svg') center / contain no-repeat;
}

/* the header stays on top while the page scrolls */
.app-header { position: sticky; top: 0; z-index: 30; }
.app-right { display: flex; align-items: center; gap: 1rem; margin-left: auto; }
.gb-jump { display: inline-flex; align-items: center; gap: .5rem; min-height: 2.75rem; padding: 0 .75rem; border: 1px solid var(--border-strong); background: var(--bg); color: var(--fg-muted); font: inherit; font-size: .875rem; cursor: pointer; }
.gb-jump:hover, .gb-jump:focus-visible { border-color: var(--fg); color: var(--fg); }
.gb-jump kbd { padding: 0 .3rem; border: 1px solid var(--border); font-family: var(--font-mono); font-size: .75rem; }
.gb-synclink { display: inline-flex; align-items: center; gap: .5rem; min-height: 2.75rem; padding: 0 1rem; border: 1px solid var(--border-strong); color: var(--fg); font-weight: 700; font-size: .875rem; text-decoration: none; white-space: nowrap; }
.gb-synclink:hover { border-color: var(--fg); }
.gb-synclink-run { border-color: var(--fg); }
.gb-me { display: inline-flex; align-items: center; gap: .5rem; font-weight: 700; font-size: .875rem; white-space: nowrap; color: var(--fg); text-decoration: none; }
.gb-settingslink { font: inherit; font-weight: 700; background: none; cursor: pointer; display: inline-flex; align-items: center; justify-content: center; width: 2.75rem; height: 2.75rem; border: 1px solid var(--border-strong); color: var(--fg); }
.gb-settingslink:hover, .gb-settingslink.active { border-color: var(--fg); }

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
  /* The nav moves to its own row and scrolls sideways instead of
     overflowing the page. */
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
  .app-header .epfl-nav a { white-space: nowrap; }
  .app-right { order: 2; }
  .gb-synclink .gb-hint, .gb-me-name, .gb-jump-label, .gb-jump kbd { display: none; }
}
</style>
