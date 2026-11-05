<!-- The release job running, or the last one (internal/server/releases.go
     releaseJob): its steps as the canvas's plan list, each as the server
     last reported it, and why it stopped, in the States board's forms. -->
<template>
  <section class="gb-job" aria-labelledby="job-h">
    <div class="gb-side-head">
      <h2 id="job-h" class="gb-section-title">{{ jobTitle(job) }}</h2>
      <span class="gb-note">{{ job.state === 'running' ? 'It goes on if you leave this page.' : when(job.ended ?? job.started, true) }}</span>
    </div>
    <ol v-if="job.steps.length" class="gb-plan" aria-live="polite">
      <li v-for="(s, i) in job.steps" :key="i" :class="stateClass[stepState(job, i)]">
        <span class="gb-plan-body">{{ stepLabel(s, names) }}<small v-if="s.detail">{{ s.detail }}</small></span>
      </li>
    </ol>
    <p v-else-if="job.state === 'running'" class="gb-note" role="status">Starting…</p>
    <template v-if="job.state === 'failed'">
      <!-- the page's own words and actions, when it knows more (a taken cut) -->
      <slot>
        <!-- older, taken: someone else got there first, nothing is wrong (States w2, w3) -->
        <div v-if="job.code === 'older' || job.code === 'taken'" class="epfl-alert epfl-alert-info gb-alert-row" role="status">
          <GbIcon :name="job.code === 'taken' ? 'tag' : 'info'" />
          <span><b>{{ job.error }}</b></span>
        </div>
        <div v-else class="epfl-alert epfl-alert-danger gb-alert-row" role="alert">
          <GbIcon name="error" />
          <span><b>{{ job.error }}</b></span>
        </div>
      </slot>
      <p v-if="codeNote[job.code]" class="gb-note">{{ codeNote[job.code] }}</p>
    </template>
  </section>
</template>

<script setup lang="ts">
import type { ReleaseJob } from '@/api/board';
import GbIcon from '@/components/GbIcon.vue';
import { jobTitle, stepLabel, stepState } from '@/releases';
import { when } from '@/when';

// names: each config repo's target name (enack8s), for the steps that
// name the repo
defineProps<{ job: ReleaseJob; names?: Record<string, string> }>();

const stateClass = { done: 'gb-plan-done', now: 'gb-plan-now', todo: '', failed: 'gb-plan-failed' };

// the States board's sentences, by the failed job's code
const codeNote: Record<string, string> = {
  older: 'An environment only moves forward. Going back is a rollback, and you have to ask for it.',
  taken: 'Nothing was written: the tag is pushed first, and origin refused the second one.',
  'not-adopted': "The overlays don't say which commit they run yet. Until they're adopted, git-board won't deploy there: it couldn't tell newer from older.",
};
</script>

<style scoped>
.gb-job { display: flex; flex-direction: column; gap: .75rem; margin-bottom: 1.5rem; }
</style>
