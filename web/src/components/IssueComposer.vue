<!-- Commenting as …: a comment, "Close with comment" / "Reopen", and while a
     sync with the forges runs, the change waits and is saved after it. -->
<template>
  <IdentityChooser v-if="me && !me.me" />
  <template v-else>
    <p class="gb-composer-as">
      <GbAvatar :name="me?.me?.name ?? '?'" :src="me?.me?.avatar_url" small dark />
      Commenting as <b>{{ me?.me?.name }}</b>
    </p>
    <div class="gb-composer">
      <MarkdownEditor v-model="text" v-model:uploading="uploading" :forges="forges" label="Comment" placeholder="Leave a comment" :disabled="busy" @submit="post()" />
      <p v-if="error" class="gb-save-error" role="alert"><GbIcon name="error" small /> <b>Your comment was not posted.</b> {{ error }} Your text is still here.</p>
      <div class="gb-composer-foot">
        <p class="gb-hint">
          Saved in git-board now, on {{ forgeNames }} at the next sync.<template v-if="carries?.length"> Carries<template v-for="c in carries" :key="c"> <span class="gb-carries">{{ c }}</span></template></template>
        </p>
        <button v-if="cancel" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="cancel">Cancel</button>
        <button v-else type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || uploading" @click="post(status === 'open' ? 'closed' : 'open')">
          <GbIcon :name="status === 'open' ? 'closed' : 'open'" small />
          {{ status === 'open' ? (text.trim() ? 'Close with comment' : 'Close issue') : (text.trim() ? 'Reopen with comment' : 'Reopen issue') }}
        </button>
        <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || uploading || !text.trim()" @click="post()">Comment</button>
      </div>
    </div>
  </template>
</template>

<script setup lang="ts">
import GbAvatar from '@/components/GbAvatar.vue';
import { computed, ref } from 'vue';
import type { Forge } from '@/api';
import { me } from '@/me';
import GbIcon from '@/components/GbIcon.vue';
import IdentityChooser from '@/components/IdentityChooser.vue';
import MarkdownEditor from '@/components/MarkdownEditor.vue';

const props = defineProps<{
  status: string;
  forges?: Forge[] | null | undefined;
  busy: boolean;
  error: string;
  comment: (body: string, status?: 'open' | 'closed') => Promise<boolean>;
  carries?: string[]; // what the comment carries: its step "07 Verify", the version "v1", an anchor "§3 Expected"
  cancel?: (() => void) | undefined; // a composer under a section: Cancel in place of Close issue
}>();

const text = ref('');
const uploading = ref(false); // a file uploads in the editor: sending waits for it (MarkdownEditor)
const forgeNames = computed(() => {
  const names = [...new Set((props.forges ?? []).map((f) => f.name))];
  return names.length ? names.join(' and ') : 'the forges';
});

// the box empties at once, as the comment shows (issue.ts); refused, it
// comes back with what was typed
async function post(status?: 'open' | 'closed') {
  if (uploading.value || (!status && !text.value.trim())) return;
  const typed = text.value;
  text.value = '';
  if (!(await props.comment(typed, status)) && !text.value) text.value = typed;
}
</script>

<style scoped>
.gb-carries { padding: 1px 6px; border: 1px solid var(--gb-done); color: var(--gb-done); font-family: var(--font-mono); font-size: .75rem; white-space: nowrap; }
.gb-wait { display: flex; gap: .375rem; align-items: center; margin: 0; padding: .5rem 1rem; font-size: .875rem; color: var(--fg); background: var(--bg-subtle); }
</style>
