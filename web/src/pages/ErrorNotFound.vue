<template>
  <AppHeader :views="[]" active="" />
  <main class="not-found">
    <div class="epfl-eyebrow">Error 404</div>
    <h1 class="h3">Page not found</h1>
    <p>This address doesn't match anything in git-board.</p>
    <router-link to="/" class="epfl-btn epfl-btn-primary">Back to the board</router-link>
  </main>
</template>

<script setup lang="ts">
import AppHeader from '@/components/AppHeader.vue';
</script>

<style scoped>
.not-found { padding: var(--space-5) var(--gutter); }
</style>
