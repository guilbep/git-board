<!-- The release job running, or the last one (internal/server/releases.go
     releaseJob): its steps as the canvas's plan list, each as the server
     last reported it, and why it stopped. -->
<template>
  <section class="gb-job" aria-labelledby="job-h" aria-live="polite">
    <div class="gb-side-head">
      <h2 id="job-h" class="gb-section-title">{{ jobTitle(job) }}</h2>
      <span class="gb-note">{{ job.state === 'running' ? 'It goes on if you leave this page.' : when(job.ended ?? job.started, true) }}</span>
    </div>
    <ol v-if="job.steps.length" class="gb-plan">
      <li v-for="(s, i) in job.steps" :key="i" :class="stateClass[stepState(job, i)]">
        <span class="gb-plan-body">{{ stepName(s.name) }}<small v-if="s.detail">{{ s.detail }}</small></span>
      </li>
    </ol>
    <p v-else-if="job.state === 'running'" class="gb-note" role="status">Starting…</p>
    <div v-if="job.state === 'failed'" class="epfl-alert epfl-alert-danger gb-alert-row" role="alert">
      <GbIcon name="error" />
      <span><b>{{ job.error }}</b> <template v-if="codeNote[job.code]">{{ codeNote[job.code] }}</template></span>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { ReleaseJob } from '@/api/board';
import GbIcon from '@/components/GbIcon.vue';
import { jobTitle, stepState } from '@/releases';
import { when } from '@/when';

// names: each config repo's target name (enack8s), for the steps that
// name the repo: "dev on <repo>"
const props = defineProps<{ job: ReleaseJob; names?: Record<string, string> }>();
const stepName = (n: string) => {
  const [env, repo] = n.split(' on ');
  const name = repo && props.names?.[repo];
  return name ? `${env} on ${name}` : n;
};

const stateClass = { done: 'gb-plan-done', now: 'gb-plan-now', todo: '', failed: 'gb-plan-failed' };

// the States artboard's sentences, by the failed job's code
const codeNote: Record<string, string> = {
  older: 'An environment only moves forward. Going back is a rollback, and you have to ask for it.',
  taken: 'Nothing was written: the tag is pushed first, and origin refused the second one.',
  'not-adopted': "The overlays don't say which commit they run yet. Until they're adopted, git-board won't deploy there: it couldn't tell newer from older.",
};
</script>

<style scoped>
.gb-job { display: flex; flex-direction: column; gap: .75rem; margin-bottom: 1.5rem; }
</style>
