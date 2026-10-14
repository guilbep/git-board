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
    <nav v-if="views.length" class="epfl-nav" aria-label="Saved views">
      <router-link
        v-for="name in views"
        :key="name"
        :to="{ path: '/', query: { view: name } }"
        :class="{ active: name === active }"
        :aria-current="name === active ? 'page' : undefined"
      >{{ name }}</router-link>
    </nav>
  </header>
</template>

<script setup lang="ts">
defineProps<{ views: string[]; active: string }>();
</script>

<style scoped>
.logo-mark {
  display: block;
  height: 2rem;
  aspect-ratio: 182.4 / 53;
  background: var(--primary);
  mask: url('/epfl-logo.svg') center / contain no-repeat;
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
