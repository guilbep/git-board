<!-- Where a run is: a bar and its last step, "github pull: reading 412 of
     1,034", from the progress events (sync.ts onStep). -->
<template>
  <div class="gb-progress" role="status">
    <progress v-if="last?.total" :value="last.done ?? 0" :max="last.total" />
    <progress v-else />
    <span>{{ last ? stepWords(last) : run.dry_run ? 'Checking the forges…' : 'Syncing…' }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { SyncRun } from '@/api';
import { stepWords } from '@/ways';

const props = defineProps<{ run: SyncRun }>();
const last = computed(() => props.run.current ?? props.run.steps.at(-1));
</script>

<style scoped>
.gb-progress { display: grid; gap: .25rem; }
.gb-progress progress { width: 100%; height: .5rem; accent-color: var(--primary); }
</style>
