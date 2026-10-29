<!-- Where a forge run is: a bar and a line, "GitHub (gh) · pulling 412 of
     1,034 · #412 import: users lose their work · about 6 min left"
     (ways.ts stepWords, timeLeft), from the progress events (forges.ts
     onStep). A long pull says it takes minutes, and that the page can
     close. -->
<template>
  <div class="gb-progress" role="status">
    <progress v-if="job.step?.total" :value="job.step.done" :max="job.step.total" />
    <progress v-else />
    <span>{{ job.step ? stepWords(job.step, bridges) : starting }}<template v-if="left"> · {{ left }}</template></span>
    <p v-if="long" class="gb-hint">
      About {{ long.toLocaleString() }} issues to read: this takes several minutes. You can close this page, the sync goes on in git-board’s server;
      a sync cut off goes on, next time, from where it stopped.
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import type { ForgeJob } from '@/api/board';
import { longPull, msLeft, stepWords, timeLeft, type BarSample } from '@/ways';

// starting is the line before the first step
const props = defineProps<{ job: ForgeJob; bridges: { name: string; target: string }[]; starting: string }>();

// the way's first sample and its last, for the rate; long: the total of a
// pull that takes minutes, said until the run ends
let way = '';
let first: BarSample = { at: 0, done: 0 };
const left = ref('');
const long = ref(0);
watch(
  () => props.job.step,
  (s) => {
    if (!s?.total) return;
    const now = { at: Date.now(), done: s.done };
    if (`${s.bridge} ${s.way}` !== way) {
      way = `${s.bridge} ${s.way}`;
      first = now;
    }
    left.value = timeLeft(first, now, s.total);
    if (longPull(s, msLeft(first, now, s.total))) long.value ||= s.total;
  },
  { immediate: true },
);
</script>

<style scoped>
.gb-progress { display: grid; gap: .25rem; }
.gb-progress progress { width: 100%; height: .5rem; accent-color: var(--primary); }
.gb-progress p { margin: 0; }
</style>
