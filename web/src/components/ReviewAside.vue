<!-- Step 6's discussion, above its comments (the canvas's C6): each review
     thread in one line, a click opens its file in the diff. -->
<template>
  <section class="gb-ra" aria-labelledby="ra-title">
    <h3 id="ra-title" class="gb-section-title">Review · {{ threads.length }} thread{{ threads.length === 1 ? '' : 's' }}</h3>
    <ul v-if="threads.length">
      <li v-for="t in threads" :key="t.root.id">
        <!-- ponytail: no bot tag, the Person type has no bot flag yet -->
        <button type="button" @click="emit('line', t.root.review!.path!)">
          <span><b>{{ t.root.author.name }}</b> <span class="gb-mono">{{ at(t) }}</span></span>
          <span class="gb-ra-body">{{ (t.root.body ?? '').trim().split('\n')[0] }}</span>
          <span :class="['gb-tag', { 'gb-ra-open': !t.resolved }]">{{ t.resolved ? 'resolved' : 'open' }}</span>
        </button>
      </li>
    </ul>
    <p class="gb-hint">Line comments {{ pr ? `go to PR #${pr} on GitHub and ` : '' }}stay on the issue, under this step.</p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { IssueEvent } from '@/api/board';
import { threadsOf, type ReviewThread } from '@/review';

const props = defineProps<{ events: IssueEvent[] }>();
const emit = defineEmits<{ line: [path: string] }>();

const threads = computed(() => threadsOf(props.events));
const at = (t: ReviewThread) => `${t.root.review!.path!.replace(/^.*\//, '')}:${t.root.review!.line}`;
// the pull request the comments name, as review:pr writes it: host/o/r#N
const pr = computed(() => threads.value.map((t) => t.root.review?.pr?.split('#')[1]).find(Boolean));
</script>

<style scoped>
.gb-ra { display: flex; flex-direction: column; gap: .5rem; margin-bottom: 1rem; }
.gb-ra ul { display: flex; flex-direction: column; gap: .25rem; margin: 0; padding: 0; list-style: none; }
.gb-ra button { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: .125rem .5rem; width: 100%; padding: .5rem; border: 1px solid var(--border); background: none; text-align: left; font-size: .8125rem; color: inherit; cursor: pointer; }
.gb-ra button:hover { background: var(--bg-subtle); }
.gb-ra button > :first-child { overflow-wrap: anywhere; }
.gb-ra-body { grid-column: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--fg-muted); }
.gb-ra .gb-tag { grid-row: 1; grid-column: 2; align-self: start; }
.gb-ra-open { font-weight: 700; }
</style>
