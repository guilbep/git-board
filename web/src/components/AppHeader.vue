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
    <nav v-if="views.length || syncStatus?.project" class="epfl-nav" aria-label="Saved views">
      <router-link
        v-for="name in views"
        :key="name"
        :to="{ path: '/', query: { view: name } }"
        :class="{ active: name === active }"
        :aria-current="name === active ? 'page' : undefined"
      >{{ name }}</router-link>
      <!-- The count is the last check's: the header never reads the Project. -->
      <router-link
        v-if="syncStatus?.project"
        to="/conflicts"
        active-class="active"
        aria-current-value="page"
      >Conflicts<span v-if="conflicts" class="nav-count" :aria-label="`, ${conflicts} held`">{{ conflicts }}</span></router-link>
    </nav>
  </header>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { loadSyncStatus, syncStatus } from '@/sync';

defineProps<{ views: string[]; active: string }>();

const conflicts = computed(() => syncStatus.value?.last?.conflicts.length ?? 0);

onMounted(() => {
  if (!syncStatus.value) void loadSyncStatus();
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

.nav-count {
  margin-left: var(--space-1);
  padding: 0 0.375rem;
  font-size: 0.75rem;
  font-weight: var(--w-bold);
  color: var(--epfl-white);
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
}
</style>
