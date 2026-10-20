<!-- Where a forge run is: a bar and a line, "GitHub (gh) · pulling 12 of
     40 · #43 import: users lose their work" (ways.ts stepWords), from the
     progress events (forges.ts onStep). A pull's bar has no end: the forge
     doesn't say how many it holds. -->
<template>
  <div class="gb-progress" role="status">
    <progress v-if="job.step?.total" :value="job.step.done" :max="job.step.total" />
    <progress v-else />
    <span>{{ job.step ? stepWords(job.step, bridges) : starting }}</span>
  </div>
</template>

<script setup lang="ts">
import type { ForgeJob } from '@/api/board';
import { stepWords } from '@/ways';

// starting is the line before the first step
defineProps<{ job: ForgeJob; bridges: { name: string; target: string }[]; starting: string }>();
</script>

<style scoped>
.gb-progress { display: grid; gap: .25rem; }
.gb-progress progress { width: 100%; height: .5rem; accent-color: var(--primary); }
</style>
