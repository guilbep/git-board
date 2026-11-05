<!-- An issue's images, full screen (MarkdownBody opens it on a click on an
     image or its expand button): ←/→ through the issue's images, Download,
     Open in a new tab, Esc or a click beside the image closes. -->
<template>
  <q-dialog :model-value="index !== null" maximized class="lb-dialog" @update:model-value="(v) => !v && emit('close')">
    <div v-if="img" class="lb" role="dialog" aria-modal="true" :aria-label="`Image ${pos + 1} of ${images.length}: ${img.name}`" @click.self="emit('close')" @keydown="onKey">
      <header class="lb-bar">
        <span class="lb-name" :title="img.name">{{ img.name }}</span>
        <span v-if="images.length > 1" class="lb-count">{{ pos + 1 }} / {{ images.length }}</span>
        <span class="lb-grow" />
        <a class="lb-btn" :href="img.src" :download="img.name" target="_blank" rel="noopener noreferrer"><GbIcon name="down" small /> Download</a>
        <a class="lb-btn" :href="img.src" target="_blank" rel="noopener noreferrer"><GbIcon name="external" small /> Open</a>
        <button ref="closeBtn" type="button" class="lb-btn" aria-label="Close (Esc)" @click="emit('close')"><GbIcon name="close" small /></button>
      </header>
      <div class="lb-stage" @click.self="emit('close')">
        <button v-if="images.length > 1" type="button" class="lb-nav lb-prev" aria-label="Previous image (←)" @click="step(-1)"><GbIcon name="back" /></button>
        <img :src="img.src" :alt="img.name" class="lb-img" />
        <button v-if="images.length > 1" type="button" class="lb-nav lb-next" aria-label="Next image (→)" @click="step(1)"><GbIcon name="back" /></button>
      </div>
    </div>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import GbIcon from '@/components/GbIcon.vue';

export interface LightboxImage {
  src: string;
  name: string;
}

const props = defineProps<{ images: LightboxImage[]; index: number | null }>();
const emit = defineEmits<{ close: []; move: [index: number] }>();

const pos = computed(() => props.index ?? 0);
const img = computed(() => (props.index === null ? undefined : props.images[props.index]));
const closeBtn = ref<HTMLButtonElement | null>(null);
watch(() => props.index, async (i, was) => {
  if (i !== null && was === null) {
    await nextTick();
    closeBtn.value?.focus();
  }
});

function step(d: number) {
  const n = props.images.length;
  emit('move', (pos.value + d + n) % n);
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'ArrowLeft') step(-1);
  else if (e.key === 'ArrowRight') step(1);
  else return;
  e.preventDefault();
}
</script>

<style scoped>
.lb { display: flex; flex-direction: column; width: 100vw; height: 100vh; background: rgba(10, 10, 10, .94); color: #F2F2F2; }
.lb-bar { display: flex; align-items: center; gap: .75rem; padding: .625rem 1rem; }
.lb-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 700; }
.lb-count { flex: none; color: #B5B5B5; font-size: .875rem; }
.lb-grow { flex: 1; }
.lb-btn { display: inline-flex; align-items: center; gap: .375rem; min-height: 2.25rem; padding: 0 .75rem; border: 1px solid rgba(255, 255, 255, .3); background: none; color: inherit; font: inherit; font-size: .875rem; text-decoration: none; cursor: pointer; }
.lb-btn:hover, .lb-btn:focus-visible { border-color: #FFF; outline: none; }
.lb-stage { position: relative; flex: 1; display: flex; align-items: center; justify-content: center; min-height: 0; padding: 0 4rem 1.5rem; }
.lb-img { max-width: 100%; max-height: 100%; object-fit: contain; background: #FFF; box-shadow: 0 1rem 3rem rgba(0, 0, 0, .5); }
.lb-nav { position: absolute; top: 50%; display: inline-flex; align-items: center; justify-content: center; width: 2.75rem; height: 2.75rem; border: 1px solid rgba(255, 255, 255, .3); border-radius: 50%; background: rgba(0, 0, 0, .4); color: #FFF; transform: translateY(-50%); cursor: pointer; }
.lb-nav:hover, .lb-nav:focus-visible { border-color: #FFF; outline: none; }
.lb-prev { left: 1rem; }
.lb-next { right: 1rem; }
.lb-next :deep(svg) { transform: scaleX(-1); }
@media (max-width: 47.99rem) { .lb-stage { padding: 0 .5rem 1rem; } .lb-nav { top: auto; bottom: 1rem; transform: none; } }
</style>
