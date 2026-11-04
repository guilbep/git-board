<!-- Write / Preview, as on GitHub. The preview is the server's rendering, the
     same a saved comment gets. Ctrl+Enter (⌘+Enter) submits, once no file
     is uploading: v-model:uploading tells the parent, to hold its Send. -->
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
      @keydown.enter.ctrl.exact.prevent="submit"
      @keydown.enter.meta.exact.prevent="submit"
      @paste="onPaste"
      @dragover="onDragOver"
      @drop="onDropFiles"
    />
    <p v-if="uploadingName" class="gb-attach-state" role="status">Uploading {{ uploadingName }}… Sending waits for it.</p>
    <p v-if="uploadError" class="gb-attach-state gb-attach-error" role="alert">{{ uploadError }}</p>
    <div v-if="tab === 'preview'" class="gb-preview">
      <p v-if="previewing" class="gb-hint">Rendering…</p>
      <p v-else-if="!text.trim()" class="gb-hint">Nothing to preview.</p>
      <p v-else-if="previewError" class="gb-attach-error" role="alert">{{ previewError }}</p>
      <MarkdownBody v-else :html="preview" :forges="forges" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { noteUse } from '@/keys';
import { onMounted, ref } from 'vue';
import { renderMarkdown, uploadFile, type Forge } from '@/api/board';
import { message } from '@/issue';
import { fillPlaceholder, insertAt, placeholderFor } from '@/attachtext';
import { latestOnly } from '@/latest';
import { loadUploadStatus, uploadStatus } from '@/uploads';
import MarkdownBody from '@/components/MarkdownBody.vue';

defineProps<{ label: string; placeholder?: string; disabled?: boolean; forges?: Forge[] | null | undefined }>();
const text = defineModel<string>({ required: true });
// true while a file uploads: the text holds its placeholder, not its link
const uploading = defineModel<boolean>('uploading', { default: false });
const emit = defineEmits<{ submit: [] }>();

function submit() {
  if (uploading.value) return;
  noteUse('send');
  emit('submit');
}

const tab = ref<'write' | 'preview'>('write');
const preview = ref('');
const previewing = ref(false);
const area = ref<HTMLTextAreaElement | null>(null);

// the text last asked for: an answer for an older one is dropped
const previews = latestOnly();
const previewError = ref('');
async function showPreview() {
  tab.value = 'preview';
  if (!text.value.trim()) return;
  const current = previews.start();
  previewing.value = true;
  previewError.value = '';
  try {
    const html = (await renderMarkdown(text.value)).html;
    if (current()) preview.value = html;
  } catch (e) {
    if (!current()) return;
    preview.value = '';
    previewError.value = `No preview: ${message(e)}`;
  } finally {
    if (current()) previewing.value = false;
  }
}

// Attachments: picked, pasted or dropped files go to the CDN; each leaves
// a placeholder at the cursor that its Markdown replaces.
const picker = ref<HTMLInputElement | null>(null);
const uploadingName = ref('');
const uploadError = ref('');
let pending = 0; // uploads under way, from every attach at once
let uploads = 0; // placeholders made, for their names to differ
onMounted(loadUploadStatus);

async function attach(files: File[]) {
  if (!files.length) return;
  if (!uploadStatus.value?.enabled) {
    uploadError.value = "Attaching files isn't set up here: the git-board server needs ACCESS_KEY_ID, ACCESS_KEY_SECRET and BUCKET_NAME.";
    return;
  }
  uploadError.value = '';
  pending++;
  uploading.value = true;
  try {
    for (const file of files) {
      const placeholder = placeholderFor(file.name, ++uploads);
      text.value = insertAt(text.value, area.value?.selectionStart ?? text.value.length, placeholder);
      uploadingName.value = file.name;
      try {
        const up = await uploadFile(file);
        text.value = fillPlaceholder(text.value, placeholder, up.markdown);
      } catch (e) {
        text.value = text.value.replace(placeholder, '');
        uploadError.value = `${file.name} wasn't attached: ${message(e)}`;
      }
    }
  } finally {
    if (--pending === 0) {
      uploading.value = false;
      uploadingName.value = '';
    }
  }
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
