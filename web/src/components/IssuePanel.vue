<!-- An issue, whole: the side panel (in the board's q-dialog) or the full
     page (#/issues/<id>). Design: the canvas's "Issue side panel" and "Full
     page" artboards. Every write answers with the whole history (issue.ts). -->
<template>
  <component
    :is="page ? 'div' : 'aside'"
    class="gb-panel"
    :class="page ? 'gb-panel-page' : panelMode === 'wide' ? 'gb-panel-wide' : 'gb-panel-xl'"
    :style="!page && panelMode === 'side' && panelWidth ? { width: `${panelWidth}px` } : undefined"
    :aria-labelledby="`title-${uid}`"
    :tabindex="page ? undefined : -1"
    ref="root"
    @keydown.esc="leaveField"
  >
    <div
      v-if="!page && panelMode === 'side'"
      class="gb-resize"
      :class="{ 'is-dragging': dragging }"
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize the panel: drag, or use the arrow keys; double-click for the default width"
      tabindex="0"
      @pointerdown="startResize"
      @dblclick="panelWidth = 0"
      @keydown.left.prevent="nudge(32)"
      @keydown.right.prevent="nudge(-32)"
    />
    <div class="gb-ptop">
      <p class="gb-ptop-crumb">
        <template v-if="page"><router-link :to="{ path: '/' }"><GbIcon name="back" small /> Board</router-link></template>
        <template v-else>
          <span>{{ crumb }}</span>
          <template v-if="position"><span aria-hidden="true">/</span><span>{{ position }}</span></template>
        </template>
      </p>
      <template v-if="!page">
        <button type="button" class="gb-icon-btn" aria-label="Previous issue (k)" :disabled="!hasPrev" @click="emit('prev')"><GbIcon name="up" /></button>
        <button type="button" class="gb-icon-btn" aria-label="Next issue (j)" :disabled="!hasNext" @click="emit('next')"><GbIcon name="down" /></button>
        <span class="gb-keys" aria-hidden="true"><kbd>k</kbd><kbd>j</kbd></span>
        <span class="gb-tbsep" aria-hidden="true" />
      </template>
      <button type="button" class="gb-icon-btn" :aria-label="copied ? 'Link copied' : 'Copy link to this issue'" @click="copyLink"><GbIcon :name="copied ? 'check' : 'link'" /></button>
      <button v-if="!page" type="button" class="gb-icon-btn" :aria-label="panelMode === 'side' ? 'Expand over the board (f)' : 'Back to the side panel (f)'" :aria-pressed="panelMode === 'wide'" @click="togglePanelMode"><GbIcon name="expand" /></button>
      <router-link v-if="!page" class="gb-icon-btn" :to="`/issues/${id}`" aria-label="Open as a page of its own"><GbIcon name="external" /></router-link>
      <template v-if="!page">
        <span class="gb-tbsep" aria-hidden="true" />
        <span class="gb-keys" aria-hidden="true"><kbd>Esc</kbd></span>
        <button type="button" class="gb-icon-btn" aria-label="Close panel (Esc)" @click="emit('close')"><GbIcon name="close" /></button>
      </template>
    </div>

    <div v-if="error && !thread" class="gb-ihead">
      <div class="epfl-alert epfl-alert-danger" role="alert">
        <b>Couldn’t load this issue.</b> {{ error }}
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="load">Try again</button>
      </div>
    </div>
    <div v-else-if="!thread" class="gb-ihead" role="status" aria-label="Loading the issue">
      <h2 class="gb-skel-title">{{ title || 'Loading…' }}</h2>
      <div class="gb-skel" /><div class="gb-skel gb-skel-short" />
    </div>

    <template v-else>
      <div class="gb-ihead">
        <div class="gb-ititle">
          <h2 v-if="!renaming" :id="`title-${uid}`">{{ thread.title }}</h2>
          <form v-else class="gb-rename" @submit.prevent="saveTitle">
            <input ref="titleInput" v-model="newTitle" class="epfl-input" aria-label="Title" :disabled="saving" @keydown.esc.stop="renaming = false" />
            <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="saving || !newTitle.trim()">Save</button>
            <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="renaming = false">Cancel</button>
          </form>
          <button v-if="!renaming" type="button" class="gb-icon-btn" aria-label="Edit title" @click="startRename"><GbIcon name="edit" /></button>
          <button v-if="!renaming" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="saving" @click="toggleStatus">
            <GbIcon :name="thread.status === 'open' ? 'closed' : 'open'" small />
            {{ thread.status === 'open' ? 'Close issue' : 'Reopen issue' }}
          </button>
        </div>
        <div class="gb-imeta">
          <span :class="['epfl-badge', thread.status === 'open' ? 'epfl-badge-tertiary' : 'epfl-badge-dark']">
            <GbIcon :name="thread.status === 'open' ? 'open' : 'closed'" small />{{ thread.status === 'open' ? 'Open' : 'Closed' }}
          </span>
          <span><b>{{ thread.author.name }}</b> opened this {{ on(thread.created_at) }} · {{ comments }} comment{{ comments === 1 ? '' : 's' }}</span>
          <span v-if="thread.forges?.length" class="gb-forgelinks">
            <template v-for="(f, i) in thread.forges" :key="f.url">
              <span v-if="i" aria-hidden="true">·</span>
              <a :href="f.url" target="_blank" rel="noopener noreferrer">{{ f.name }}{{ f.number ? ` #${f.number}` : '' }}<GbIcon name="external" small /><span class="gb-sr">(opens in a new tab)</span></a>
            </template>
          </span>
          <span class="gb-mono" title="git-bug id">git-bug {{ thread.id.slice(0, 7) }}</span>
        </div>
        <p v-if="saveError && !renaming" class="gb-save-error" role="alert"><GbIcon name="error" small /> <b>Not saved.</b> {{ saveError }}</p>
        <p v-if="waiting" class="gb-wait" role="status"><GbIcon name="clock" small /> Syncing with GitHub/GitLab… your change is saved after it.</p>
      </div>

      <div class="gb-cols">
        <div class="gb-maincol">
          <CommentCard v-if="created" :data-event="created.id" :event="created" :forges="thread.forges" :mine="created.author.id === meId" :save="edit" @ref="onRef" />
          <ol class="gb-tl" aria-label="History">
            <template v-for="e in rest" :key="e.id">
              <li v-if="e.kind === 'comment'" :data-event="e.id">
                <CommentCard :event="e" :forges="thread.forges" :mine="e.author.id === meId" :save="edit" @ref="onRef" />
              </li>
              <TimelineEvent v-else :data-event="e.id" :event="e" :fields="fields" />
            </template>
          </ol>
          <IssueComposer :status="thread.status" :forges="thread.forges" :busy="saving" :waiting="waiting" :error="saveError" :comment="comment" />
        </div>
        <IssueSide
          :id="thread.id"
          :labels="thread.labels"
          :names="sideNames"
          :fields="fields"
          :forges="thread.forges"
          :participants="thread.participants"
          :events="thread.events"
          :saving="saving"
          @changed="fieldsChanged"
          @labels="labels"
          @reveal="reveal"
        />
      </div>
    </template>
  </component>
</template>

<script setup lang="ts">
import { clampWidth, panelMode, panelWidth, togglePanelMode } from '@/panelsize';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRef, useId } from 'vue';
import type { Thread } from '@/api/board';
import type { Fields } from '@/fields';
import { useIssue } from '@/issue';
import { me, on } from '@/me';
import CommentCard from '@/components/CommentCard.vue';
import GbIcon from '@/components/GbIcon.vue';
import IssueComposer from '@/components/IssueComposer.vue';
import IssueSide from '@/components/IssueSide.vue';
import TimelineEvent from '@/components/TimelineEvent.vue';

const props = defineProps<{
  id: string;
  names: string[];
  fields: Fields;
  page?: boolean;
  title?: string; // shown while loading: the card's
  crumb?: string;
  position?: string; // "in-progress, 1 of 4"
  hasPrev?: boolean;
  hasNext?: boolean;
  findRef?: (n: string) => string | undefined; // #n → a local issue's id
}>();
const emit = defineEmits<{ close: []; prev: []; next: []; changed: [thread: Thread]; open: [id: string] }>();

const uid = useId();
const { thread, error, saving, waiting, saveError, load, comment, edit, rename, setStatus, labels } =
  useIssue(toRef(props, 'id'), (t) => emit('changed', t));
const meId = computed(() => me.value?.me?.id ?? '');
const created = computed(() => thread.value?.events.find((e) => e.kind === 'created'));
const rest = computed(() => thread.value?.events.filter((e) => e.kind !== 'created') ?? []);
const comments = computed(() => rest.value.filter((e) => e.kind === 'comment').length);
// the board's fields, and any namespace this issue has besides
const sideNames = computed(() => {
  const own = (thread.value?.labels ?? []).filter((l) => l.includes(':')).map((l) => l.slice(0, l.indexOf(':')));
  return [...props.names, ...[...new Set(own)].filter((n) => !props.names.includes(n)).sort()];
});

const renaming = ref(false);
const newTitle = ref('');
const titleInput = ref<HTMLInputElement | null>(null);
async function startRename() {
  newTitle.value = thread.value?.title ?? '';
  renaming.value = true;
  await nextTick();
  titleInput.value?.focus();
}
async function saveTitle() {
  if (await rename(newTitle.value.trim())) renaming.value = false;
}
const toggleStatus = () => setStatus(thread.value?.status === 'open' ? 'closed' : 'open');

// a field changed in the side column: its history line comes from git-bug
async function fieldsChanged() {
  await load();
  if (thread.value) emit('changed', thread.value);
}

const copied = ref(false);
async function copyLink() {
  const url = `${location.origin}${location.pathname}#/issues/${props.id}`;
  try {
    await navigator.clipboard.writeText(url);
    copied.value = true;
    setTimeout(() => (copied.value = false), 2000);
  } catch {
    window.prompt('Copy the link', url);
  }
}

// #123 of a local issue opens it here, else the link goes to the forge
// reveal scrolls to a history item and flashes it (a waiting change's row).
function reveal(id: string) {
  const el = document.querySelector<HTMLElement>(`[data-event="${CSS.escape(id)}"]`);
  if (!el) return;
  el.scrollIntoView({ block: 'center', behavior: 'smooth' });
  el.classList.remove('gb-flash');
  void el.offsetWidth; // restart the animation on a second click
  el.classList.add('gb-flash');
  setTimeout(() => el.classList.remove('gb-flash'), 1800);
}

function onRef(n: string, e: MouseEvent) {
  const local = props.findRef?.(n);
  if (local) {
    e.preventDefault();
    emit('open', local);
  }
}

// j / k move through the board's issues, unless typing
function onKey(e: KeyboardEvent) {
  if (props.page || e.ctrlKey || e.metaKey || e.altKey) return;
  if (e.target instanceof Element && e.target.closest('input, textarea, select, [contenteditable]')) return;
  if (e.key === 'j' && props.hasNext) emit('next');
  if (e.key === 'k' && props.hasPrev) emit('prev');
  if (e.key === 'f') togglePanelMode();
}

// the side panel's width: dragged from its left edge, or ←/→ on it
const dragging = ref(false);
function startResize(e: PointerEvent) {
  e.preventDefault();
  dragging.value = true;
  const move = (m: PointerEvent) => (panelWidth.value = clampWidth(window.innerWidth - m.clientX, window.innerWidth));
  const up = () => {
    dragging.value = false;
    window.removeEventListener('pointermove', move);
    window.removeEventListener('pointerup', up);
  };
  window.addEventListener('pointermove', move);
  window.addEventListener('pointerup', up);
}
function nudge(px: number) {
  const now = panelWidth.value || (document.querySelector('.gb-panel-xl') as HTMLElement | null)?.offsetWidth || 1024;
  panelWidth.value = clampWidth(now + px, window.innerWidth);
}
// The panel opens with the keyboard on itself, not in a field (the dialog
// has no-focus): j / k / f / Esc work at once, Tab goes to the fields. Esc
// in a field leaves the field, stopped here before the dialog's own Esc
// (a window listener) closes the panel; a second Esc closes it.
const root = ref<{ $el?: HTMLElement } | HTMLElement | null>(null);
function rootEl(): HTMLElement | null {
  const r = root.value;
  return r instanceof HTMLElement ? r : (r?.$el ?? null);
}
function leaveField(e: KeyboardEvent) {
  const t = e.target;
  if (!(t instanceof HTMLElement) || !t.closest('input, textarea, select, [contenteditable]')) return;
  e.stopPropagation();
  t.blur();
  rootEl()?.focus({ preventScroll: true });
}
onMounted(() => {
  window.addEventListener('keydown', onKey);
  if (!props.page) void nextTick(() => rootEl()?.focus({ preventScroll: true }));
});
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));
</script>

<style scoped>
.gb-rename { flex: 1; display: flex; gap: .5rem; }
.gb-rename .epfl-input { flex: 1; font-size: 1.25rem; font-weight: 700; }
.gb-save-error, .gb-wait { display: flex; gap: .375rem; align-items: center; margin: .75rem 0 0; padding: .5rem .75rem; font-size: .875rem; }
.gb-save-error { color: var(--danger-fg); background: var(--danger-bg); }
.gb-wait { background: var(--bg-subtle); }
.gb-skel-title { margin: 0 0 1rem; font-size: 1.5rem; }
.gb-skel { height: 1rem; margin-top: .5rem; background: var(--epfl-gray-100); }
.gb-skel-short { width: 40%; }
.gb-ptop-crumb a { display: inline-flex; align-items: center; gap: .25rem; color: var(--fg); }
</style>
