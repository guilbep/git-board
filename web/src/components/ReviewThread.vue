<!-- A line comment and its replies, under its line in the review (step 6):
     Reply, Send to Claude Code (the agent of the issue's worktree answers
     it), Resolve; a resolved thread folds. loose: its line isn't in the
     view (outdated, or written at another commit), so it quotes it. -->
<template>
  <div class="gb-rt">
    <p v-if="loose || r.outdated || thread.resolved" class="gb-rt-sum">
      <span v-if="loose" class="gb-mono">line {{ r.line }}<template v-if="r.text">: {{ r.text }}</template></span>
      <span v-if="r.outdated" class="gb-tag">outdated</span>
      <span v-if="thread.resolved" class="gb-tag">resolved</span>
      <span v-if="thread.resolved || loose" class="gb-hint">{{ thread.root.author.name }}, {{ 1 + thread.replies.length }} comment{{ thread.replies.length ? 's' : '' }}</span>
      <button v-if="thread.resolved" type="button" class="gb-rt-show" :aria-expanded="shown" @click="shown = !shown">{{ shown ? 'Hide' : 'Show' }}</button>
    </p>
    <template v-if="!thread.resolved || shown">
      <article v-for="e in [thread.root, ...thread.replies]" :key="e.id" class="gb-rt-c" :data-event="e.id">
        <p class="gb-rt-who"><b>{{ e.author.name }}</b> <span class="gb-time">{{ when(e.at, true) }}</span></p>
        <MarkdownBody v-if="e.html" :html="e.html" />
      </article>
      <div v-if="replying" class="gb-rt-reply">
        <MarkdownEditor v-model="draft" label="Reply" :disabled="busy" @submit="send" />
        <div class="gb-composer-foot">
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="replying = false">Cancel</button>
          <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !draft.trim()" @click="send">Reply</button>
        </div>
      </div>
      <div v-else class="gb-rt-acts">
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="replying = true">Reply</button>
        <!-- ponytail: Claude Code by name, as the canvas; read wtx.toml's agent when a repo runs opencode -->
        <button v-if="brief" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="sending" @click="toAgent">
          {{ sending ? 'Sending…' : 'Send to Claude Code' }}
        </button>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="resolve(thread.root.id, !thread.resolved)">
          {{ thread.resolved ? 'Unresolve' : 'Resolve' }}
        </button>
        <span v-if="sent" class="gb-hint" role="status">{{ sent }}</span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { when } from '@/me';
import type { ReviewThread } from '@/review';
import MarkdownBody from '@/components/MarkdownBody.vue';
import MarkdownEditor from '@/components/MarkdownEditor.vue';

const props = defineProps<{
  thread: ReviewThread;
  busy: boolean;
  reply: (to: string, body: string) => Promise<boolean>;
  resolve: (root: string, resolved: boolean) => Promise<boolean>;
  loose?: boolean;
  brief?: ((root: string) => Promise<string>) | null | undefined; // Send to Claude Code: what came of it
}>();

const r = computed(() => props.thread.root.review!);
const shown = ref(false); // a resolved thread, unfolded
const replying = ref(false);
const draft = ref('');
const sending = ref(false);
const sent = ref('');
async function toAgent() {
  sending.value = true;
  sent.value = await props.brief!(props.thread.root.id);
  sending.value = false;
}
async function send() {
  if (draft.value.trim() && (await props.reply(props.thread.root.id, draft.value))) {
    draft.value = '';
    replying.value = false;
  }
}
</script>

<style scoped>
.gb-rt { display: flex; flex-direction: column; gap: .5rem; padding: .5rem .75rem; border: 1px solid var(--border); background: var(--bg); font-size: .875rem; }
.gb-rt-sum { display: flex; flex-wrap: wrap; align-items: center; gap: .375rem; margin: 0; }
.gb-rt-show { padding: 0; border: 0; background: none; color: var(--primary); font: inherit; text-decoration: underline; cursor: pointer; }
.gb-rt-c + .gb-rt-c { margin-top: .5rem; padding-top: .5rem; border-top: 1px solid var(--border); }
.gb-rt-who { margin: 0 0 .25rem; }
.gb-rt-acts { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; margin-top: .5rem; }
.gb-rt-reply { margin-top: .5rem; }
</style>
