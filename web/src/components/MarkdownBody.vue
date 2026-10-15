<!-- Issue text: the server's clean HTML (internal/markdown), then #123 and
     @name as links (richtext.ts). A click on #123 is the parent's to take
     (open that issue here) before the browser follows it to the forge. -->
<template>
  <div ref="el" class="gb-md" v-bind="$attrs" @click="onClick" v-html="html" />
  <ImageLightbox :images="shown" :index="open" @close="open = null" @move="(i) => (open = i)" />
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue';
import { apiURL, type Forge } from '@/api/board';
import { enhance } from '@/richtext';
import ImageLightbox, { type LightboxImage } from '@/components/ImageLightbox.vue';

// two roots (the text, its lightbox): a class given goes to the text
defineOptions({ inheritAttrs: false });
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
  for (const img of el.value.querySelectorAll('img')) frame(img);
}

// An image gets a frame with an expand button at its top right (shown on
// hover or focus); a click on either opens the lightbox. DOM calls only,
// as richtext.ts: the server stays the one place text becomes HTML.
const SVG = 'http://www.w3.org/2000/svg';
function frame(img: HTMLImageElement) {
  if (img.parentElement?.classList.contains('gb-imgframe')) return;
  const box = document.createElement('span');
  box.className = 'gb-imgframe';
  img.replaceWith(box);
  box.append(img);
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'gb-imgexpand';
  btn.setAttribute('aria-label', `Expand image: ${nameOf(img)}`);
  btn.title = 'Expand';
  const svg = document.createElementNS(SVG, 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('aria-hidden', 'true');
  for (const d of ['M15 3h6v6', 'M9 21H3v-6', 'M21 3l-7 7', 'M3 21l7-7']) {
    const p = document.createElementNS(SVG, 'path');
    p.setAttribute('d', d);
    svg.append(p);
  }
  btn.append(svg);
  box.append(btn);
}

// an image's name: its alt text, else its file name
function nameOf(img: HTMLImageElement): string {
  const alt = img.getAttribute('alt')?.trim();
  if (alt && !/^image$/i.test(alt)) return alt;
  try {
    const u = new URL(img.src, location.href);
    const inner = u.searchParams.get('url'); // /api/media?url=…
    const path = new URL(inner ?? u.href).pathname;
    const file = decodeURIComponent(path.split('/').filter(Boolean).at(-1) ?? '');
    // GitHub's attachments are named by a UUID: no name to show
    return file && !/^[0-9a-f-]{20,}(\.\w+)?$/i.test(file) ? file : 'Image';
  } catch {
    return 'image';
  }
}

// The lightbox goes through all the issue's images: the panel's or the
// page's, else this text's.
const shown = ref<LightboxImage[]>([]);
const open = ref<number | null>(null);
function show(img: HTMLImageElement) {
  const scope = el.value?.closest('.gb-panel') ?? el.value;
  const imgs = [...(scope?.querySelectorAll<HTMLImageElement>('.gb-md img') ?? [])];
  shown.value = imgs.map((i) => ({ src: i.currentSrc || i.src, name: nameOf(i) }));
  open.value = Math.max(0, imgs.indexOf(img));
}

function onClick(e: MouseEvent) {
  const t = e.target as HTMLElement;
  const framed = t.closest('.gb-imgframe');
  if (framed) {
    const img = framed.querySelector('img');
    if (img) {
      e.preventDefault(); // an image inside a link: the lightbox, not the link
      show(img);
      return;
    }
  }
  const a = t.closest('a.issue-ref');
  if (a instanceof HTMLAnchorElement && a.dataset.ref) emit('ref', a.dataset.ref, e);
}

onMounted(apply);
watch(() => props.html, () => void nextTick(apply));
</script>

<style scoped>
.gb-md :deep(.gb-imgframe) { position: relative; display: inline-block; max-width: 100%; line-height: 0; cursor: zoom-in; }
.gb-md :deep(.gb-imgexpand) { position: absolute; top: .375rem; right: .375rem; display: inline-flex; align-items: center; justify-content: center; width: 2rem; height: 2rem; padding: 0; border: 1px solid var(--border-strong); border-radius: var(--radius); background: var(--bg); color: var(--fg); opacity: 0; cursor: pointer; transition: opacity var(--t-fast, .15s); }
.gb-md :deep(.gb-imgexpand svg) { width: 1rem; height: 1rem; fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
.gb-md :deep(.gb-imgframe:hover .gb-imgexpand), .gb-md :deep(.gb-imgexpand:focus-visible) { opacity: 1; }
@media (hover: none) { .gb-md :deep(.gb-imgexpand) { opacity: .85; } }
</style>
