<!-- Write / Preview, as on GitHub. The preview is the server's rendering, the
     same a saved comment gets. Ctrl+Enter (⌘+Enter) submits. -->
<template>
  <div>
    <div class="gb-tabs" role="tablist">
      <button type="button" class="gb-tab" role="tab" :aria-selected="tab === 'write'" @click="tab = 'write'">Write</button>
      <button type="button" class="gb-tab" role="tab" :aria-selected="tab === 'preview'" @click="showPreview">Preview</button>
      <span class="gb-tabs-hint">Markdown · # links an issue · @ mentions</span>
      <button v-if="uploadStatus?.enabled" type="button" class="gb-tab gb-attach" :disabled="disabled" @click="picker?.click()">
        Attach files
      </button>
      <input ref="picker" type="file" multiple hidden @change="onPick" />
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
      @paste="onPaste"
      @dragover="onDragOver"
      @drop="onDropFiles"
    />
    <p v-if="uploading" class="gb-attach-state" role="status">Uploading {{ uploading }}…</p>
    <p v-if="uploadError" class="gb-attach-state gb-attach-error" role="alert">{{ uploadError }}</p>
    <div v-if="tab === 'preview'" class="gb-preview">
      <p v-if="previewing" class="gb-hint">Rendering…</p>
      <p v-else-if="!text.trim()" class="gb-hint">Nothing to preview.</p>
      <MarkdownBody v-else :html="preview" :forges="forges" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { renderMarkdown, uploadFile, type Forge } from '@/api/board';
import { message } from '@/issue';
import { loadUploadStatus, uploadStatus } from '@/uploads';
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

// Attachments: picked, pasted or dropped files go to the CDN; each leaves
// a placeholder at the cursor that its Markdown replaces.
const picker = ref<HTMLInputElement | null>(null);
const uploading = ref('');
const uploadError = ref('');
onMounted(loadUploadStatus);

async function attach(files: File[]) {
  if (!files.length) return;
  if (!uploadStatus.value?.enabled) {
    uploadError.value = "Attaching files isn't set up here: the git-board server needs ACCESS_KEY_ID, ACCESS_KEY_SECRET and BUCKET_NAME.";
    return;
  }
  uploadError.value = '';
  for (const file of files) {
    const placeholder = `[Uploading ${file.name}…]()`;
    insertAtCursor(placeholder);
    uploading.value = file.name;
    try {
      const up = await uploadFile(file);
      text.value = text.value.replace(placeholder, up.markdown);
    } catch (e) {
      text.value = text.value.replace(placeholder, '');
      uploadError.value = `${file.name} wasn't attached: ${message(e)}`;
    }
  }
  uploading.value = '';
}

function insertAtCursor(s: string) {
  const el = area.value;
  const at = el ? el.selectionStart : text.value.length;
  const before = text.value.slice(0, at);
  const sep = before && !before.endsWith('\n') ? '\n' : '';
  text.value = before + sep + s + '\n' + text.value.slice(at);
}

function onPick() {
  void attach([...(picker.value?.files ?? [])]);
  if (picker.value) picker.value.value = '';
}
function onPaste(e: ClipboardEvent) {
  const files = [...(e.clipboardData?.files ?? [])];
  if (files.length) {
    e.preventDefault();
    void attach(files);
  }
}
function onDragOver(e: DragEvent) {
  if (e.dataTransfer?.types.includes('Files')) e.preventDefault();
}
function onDropFiles(e: DragEvent) {
  const files = [...(e.dataTransfer?.files ?? [])];
  if (files.length) {
    e.preventDefault();
    void attach(files);
  }
}

defineExpose({ focus: () => area.value?.focus() });
</script>

<style scoped>
.gb-preview { min-height: 8rem; padding: .75rem 1rem; }
.gb-attach { margin-left: .25rem; text-decoration: underline; text-underline-offset: 2px; }
.gb-attach-state { margin: 0; padding: .375rem 1rem; font-size: .8125rem; color: var(--fg-muted); border-top: 1px solid var(--border-subtle); }
.gb-attach-error { color: var(--danger-fg); background: var(--danger-bg); }
</style>
