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
      <MarkdownEditor v-model="text" :forges="forges" label="Comment" placeholder="Leave a comment" :disabled="busy" @submit="send()" />
      <p v-if="waiting" class="gb-wait" role="status"><GbIcon name="clock" small /> Syncing with the forges… your comment is saved after it.</p>
      <p v-else-if="error" class="gb-save-error" role="alert"><GbIcon name="error" small /> <b>Your comment was not posted.</b> {{ error }} Your text is still here.</p>
      <div class="gb-composer-foot">
        <p class="gb-hint">Saved in git-bug now, on {{ forgeNames }} at the next sync.</p>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="send(status === 'open' ? 'closed' : 'open')">
          <GbIcon :name="status === 'open' ? 'closed' : 'open'" small />
          {{ status === 'open' ? (text.trim() ? 'Close with comment' : 'Close issue') : (text.trim() ? 'Reopen with comment' : 'Reopen issue') }}
        </button>
        <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !text.trim()" @click="send()">Comment</button>
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
  comment: (body: string, status?: 'open' | 'closed') => Promise<boolean>;
}>();

const text = ref('');
const forgeNames = computed(() => {
  const names = [...new Set((props.forges ?? []).map((f) => f.name))];
  return names.length ? names.join(' and ') : 'the forges';
});

async function send(status?: 'open' | 'closed') {
  if (!status && !text.value.trim()) return;
  if (await props.comment(text.value, status)) text.value = '';
}
</script>

<style scoped>
.gb-wait { display: flex; gap: .375rem; align-items: center; margin: 0; padding: .5rem 1rem; font-size: .875rem; color: var(--fg); background: var(--bg-subtle); }
</style>
