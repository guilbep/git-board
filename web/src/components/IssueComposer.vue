<!-- Commenting as …: a comment, "Close with comment" / "Reopen", and while a
     sync with the forges runs, the change waits and is saved after it. -->
<template>
  <IdentityChooser v-if="me && !me.me" />
  <template v-else>
    <p class="gb-composer-as">
      <span class="gb-avatar gb-avatar-sm gb-avatar-dark" aria-hidden="true">{{ initials(me?.me?.name ?? '?') }}</span>
      Commenting as <b>{{ me?.me?.name }}</b>
    </p>
    <div class="gb-composer">
      <MarkdownEditor v-model="text" :forges="forges" label="Comment" placeholder="Leave a comment" :disabled="busy" @submit="post()" />
      <p v-if="waiting" class="gb-wait" role="status"><GbIcon name="clock" small /> Syncing with the forges… your comment is saved after it.</p>
      <p v-else-if="error" class="gb-save-error" role="alert"><GbIcon name="error" small /> <b>Your comment was not posted.</b> {{ error }} Your text is still here.</p>
      <div class="gb-composer-foot">
        <p class="gb-hint">
          Saved in git-board now, on {{ forgeNames }} at the next sync.<template v-if="carries?.length"> Carries<template v-for="c in carries" :key="c"> <span class="gb-carries">{{ c }}</span></template></template>
        </p>
        <button v-if="cancel" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="cancel">Cancel</button>
        <button v-else type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="post(status === 'open' ? 'closed' : 'open')">
          <GbIcon :name="status === 'open' ? 'closed' : 'open'" small />
          {{ status === 'open' ? (text.trim() ? 'Close with comment' : 'Close issue') : (text.trim() ? 'Reopen with comment' : 'Reopen issue') }}
        </button>
        <label v-if="send" class="gb-composer-send"><input v-model="toAgent" type="checkbox" :disabled="busy" /> Send to {{ send }}</label>
        <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !text.trim()" @click="post()">Comment</button>
      </div>
    </div>
  </template>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Forge } from '@/api/board';
import { initials, me } from '@/me';
import GbIcon from '@/components/GbIcon.vue';
import IdentityChooser from '@/components/IdentityChooser.vue';
import MarkdownEditor from '@/components/MarkdownEditor.vue';

const props = defineProps<{
  status: string;
  forges?: Forge[] | null | undefined;
  busy: boolean;
  waiting: boolean;
  error: string;
  // posts; send: the comment goes to the agent too, as a brief
  comment: (body: string, status?: 'open' | 'closed', send?: boolean) => Promise<boolean>;
  carries?: string[]; // what the comment carries: its step "07 Verify", the version "v1", an anchor "§3 Expected"
  cancel?: (() => void) | undefined; // a composer under a section: Cancel in place of Close issue
  send?: string | undefined; // the agent a comment can go to, by name
}>();

const text = ref('');
const toAgent = ref(false);
const forgeNames = computed(() => {
  const names = [...new Set((props.forges ?? []).map((f) => f.name))];
  return names.length ? names.join(' and ') : 'the forges';
});

async function post(status?: 'open' | 'closed') {
  if (!status && !text.value.trim()) return;
  if (await props.comment(text.value, status, !!props.send && toAgent.value)) {
    text.value = '';
    toAgent.value = false;
  }
}
</script>

<style scoped>
.gb-carries { padding: 1px 6px; border: 1px solid var(--gb-done); color: var(--gb-done); font-family: var(--font-mono); font-size: .75rem; white-space: nowrap; }
.gb-composer-send { display: inline-flex; align-items: center; gap: .375rem; font-size: .875rem; }
.gb-wait { display: flex; gap: .375rem; align-items: center; margin: 0; padding: .5rem 1rem; font-size: .875rem; color: var(--fg); background: var(--bg-subtle); }
</style>
