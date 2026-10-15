<!-- Issue text: the server's clean HTML (internal/markdown), then #123 and
     @name as links (richtext.ts). A click on #123 is the parent's to take
     (open that issue here) before the browser follows it to the forge. -->
<template>
  <div ref="el" class="gb-md" @click="onClick" v-html="html" />
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue';
import { apiURL, type Forge } from '@/api/board';
import { enhance } from '@/richtext';

const props = defineProps<{ html: string; forges?: Forge[] | null | undefined }>();
const emit = defineEmits<{ ref: [n: string, event: MouseEvent] }>();
const el = ref<HTMLElement | null>(null);

// #123 points at the issue's own forge (the first: where it came from),
// @name at that forge's profile.
function base() {
  const f = props.forges?.[0];
  if (!f) return null;
  const u = new URL(f.url);
  return { refs: f.url.replace(/\d+$/, ''), people: `${u.origin}/` };
}

function apply() {
  if (!el.value) return;
  const b = base();
  enhance(el.value, {
    refHref: (n) => (b ? b.refs + n : undefined),
    mentionHref: (login) => (b ? b.people + login : undefined),
    mediaURL: apiURL,
  });
}

function onClick(e: MouseEvent) {
  const a = (e.target as HTMLElement).closest('a.issue-ref');
  if (a instanceof HTMLAnchorElement && a.dataset.ref) emit('ref', a.dataset.ref, e);
}

onMounted(apply);
watch(() => props.html, () => void nextTick(apply));
</script>
