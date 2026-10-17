<!-- Where your issues go: the three channels (settingstabs.ts), one line
     each, what it moves and its verbs, the current one marked; each leads
     to its tab, or where `to` says (the Sync page's parts). -->
<template>
  <ol class="cm" aria-label="Where your issues go">
    <li v-for="(c, i) in channels" :key="c.id" :class="[`cm-${c.id}`, { 'cm-here': c.id === current }]">
      <router-link :to="to(c.id)" :aria-current="c.id === current ? 'true' : undefined">
        <b>{{ i + 1 }} · {{ c.label }}</b> · {{ c.via }}<span v-if="c.id === current" class="cm-here-tag"> · you are here</span>
        <span class="cm-what">{{ c.moves }} {{ c.verbs }}</span>
      </router-link>
    </li>
  </ol>
</template>

<script setup lang="ts">
import { channels, type Channel } from '@/settingstabs';

withDefaults(defineProps<{ current: Channel; to?: (c: Channel) => string }>(), {
  to: (c: Channel) => `/settings/${c}`,
});
</script>

<style scoped>
.cm { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(14rem, 100%), 1fr)); gap: .75rem; margin: 0; padding: 0; list-style: none; font-size: .875rem; }
.cm li { border: 1px solid var(--border); border-top: 4px solid var(--cm-top); }
.cm-remotes { --cm-top: var(--bg-dark); }
.cm-forges { --cm-top: var(--primary); }
.cm-github-project { --cm-top: var(--gb-purple-dot); }
.cm a { display: block; height: 100%; box-sizing: border-box; padding: .625rem .875rem; color: var(--fg); text-decoration: none; }
.cm a:hover { background: var(--bg-subtle); }
.cm .cm-here { border-color: var(--fg); border-top-color: var(--cm-top); background: var(--bg-subtle); }
.cm-here-tag { color: var(--fg-muted); }
.cm-what { display: block; color: var(--fg-muted); }
</style>
