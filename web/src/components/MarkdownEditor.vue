<!-- Write / Preview, as on GitHub. The preview is the server's rendering, the
     same a saved comment gets. Ctrl+Enter (⌘+Enter) submits. -->
<template>
  <div>
    <div class="gb-tabs" role="tablist">
      <button type="button" class="gb-tab" role="tab" :aria-selected="tab === 'write'" @click="tab = 'write'">Write</button>
      <button type="button" class="gb-tab" role="tab" :aria-selected="tab === 'preview'" @click="showPreview">Preview</button>
      <span class="gb-tabs-hint">Markdown · # links an issue · @ mentions</span>
    </div>
    <textarea
      v-show="tab === 'write'"
      ref="area"
      v-model="text"
      class="gb-textarea"
      :aria-label="label"
      :placeholder="placeholder"
      :disabled="disabled"
      @keydown.enter.ctrl.exact.prevent="emit('submit')"
      @keydown.enter.meta.exact.prevent="emit('submit')"
    />
    <div v-if="tab === 'preview'" class="gb-preview">
      <p v-if="previewing" class="gb-hint">Rendering…</p>
      <p v-else-if="!text.trim()" class="gb-hint">Nothing to preview.</p>
      <MarkdownBody v-else :html="preview" :forges="forges" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { renderMarkdown, type Forge } from '@/api/board';
import MarkdownBody from '@/components/MarkdownBody.vue';

defineProps<{ label: string; placeholder?: string; disabled?: boolean; forges?: Forge[] | null | undefined }>();
const text = defineModel<string>({ required: true });
const emit = defineEmits<{ submit: [] }>();

const tab = ref<'write' | 'preview'>('write');
const preview = ref('');
const previewing = ref(false);
const area = ref<HTMLTextAreaElement | null>(null);

async function showPreview() {
  tab.value = 'preview';
  if (!text.value.trim()) return;
  previewing.value = true;
  try {
    preview.value = (await renderMarkdown(text.value)).html;
  } catch {
    preview.value = '';
  } finally {
    previewing.value = false;
  }
}

defineExpose({ focus: () => area.value?.focus() });
</script>

<style scoped>
.gb-preview { min-height: 8rem; padding: .75rem 1rem; }
</style>
