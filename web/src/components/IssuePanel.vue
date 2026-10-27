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
    :tabindex="page ? undefined : 0"
    :data-autofocus="page ? undefined : ''"
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
    <p v-if="stale" class="gb-stale" role="status">
      Someone else changed this issue. <button type="button" class="gb-stale-btn" @click="reloadStale">Reload</button>
    </p>
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
          <h2 v-if="!renaming" :id="`title-${uid}`"><span v-if="thread.number" class="gb-num">{{ thread.number }}</span>{{ thread.shown_title }}</h2>
          <form v-else class="gb-rename" @submit.prevent="saveTitle">
            <input autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" ref="titleInput" v-model="newTitle" class="epfl-input" aria-label="Title" :disabled="saving" @keydown.esc.stop="renaming = false" />
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
          <span class="gb-mono" title="The issue's id in git-board">id {{ thread.id.slice(0, 7) }}</span>
        </div>
        <p v-if="saveError && !renaming" class="gb-save-error" role="alert"><GbIcon name="error" small /> <b>Not saved.</b> {{ saveError }}</p>
        <p v-if="waiting" class="gb-wait" role="status"><GbIcon name="clock" small /> Syncing with the forges… your change is saved after it.</p>
      </div>

      <IssueStep ref="step" :id="thread.id" :status="statusValue" :labels="thread.labels" :events="thread.events" :forges="thread.forges" :head="reading.tab === 'conversation'" @jump="jumpTo" @changed="load" @comment="(b, n) => comment(b, undefined, n)">
        <template #tabs>
          <div class="gb-tabs gb-issue-tabs" role="tablist" aria-label="Issue">
            <button type="button" class="gb-tab" role="tab" :aria-selected="reading.tab === 'conversation'" @click="reading.tab = 'conversation'">Conversation</button>
            <button type="button" class="gb-tab" role="tab" :aria-selected="reading.tab === 'history'" @click="reading.tab = 'history'">History <span class="gb-modes-count">{{ changeCount }}</span></button>
            <button v-if="created" type="button" class="gb-tab" role="tab" :aria-selected="reading.tab === 'changes'" @click="reading.tab = 'changes'">Changes <span class="gb-modes-count">{{ bodyVersions.length + ticksOf(created).length }}</span></button>
          </div>
        </template>
        <template #intent="{ folded, cycle }">
          <div v-if="created && folded && !reading.description" class="gb-desc-folded">
            <span>
              <b>{{ created.author.name }}</b> · the description<template v-if="cycle?.evidence.criteria?.length"> · {{ cycle.evidence.criteria.length }} {{ cycle.evidence.criteria.length === 1 ? 'criterion' : 'criteria' }}</template><template v-if="created.history?.length"> · edited {{ when(created.history.at(-1)!.at) }}</template>
            </span>
            <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" aria-expanded="false" @click="reading.description = true">Read the description</button>
          </div>
          <template v-else-if="created">
            <CommentCard :data-event="created.id" :event="created" :forges="thread.forges" :mine="created.author.id === meId" :save="edit" :role="roleOf(fields, created.author.login)" @ref="onRef">
              <template v-if="bodyVersions.length > 1" #head>
                <span class="gb-seg gb-seg-sm" role="group" aria-label="Version">
                  <button v-for="bv in bodyVersions" :key="bv.n" type="button" :aria-pressed="bv.n === shownV" @click="shownV = bv.n">v{{ bv.n }}</button>
                </span>
                <label v-if="shownV > 1" class="gb-since"><input v-model="changesSince" type="checkbox" /> Changes since v{{ shownV - 1 }}</label>
              </template>
              <IssueSections
                ref="sections"
                :html="created.html!"
                :versions="bodyVersions"
                :shown="shownV"
                :since="changesSince"
                :events="thread.events"
                :forges="thread.forges"
                :status="thread.status"
                :busy="saving"
                :waiting="waiting"
                :error="saveError"
                :carried="carries(cycle?.step ?? 0)"
                :comment="(anchor: string, b: string) => comment(b, undefined, cycle?.step ?? 0, { anchor })"
                :me-id="meId"
                :fields="fields"
                :save="edit"
                :reply="(root: string, b: string) => comment(b, undefined, cycle?.step ?? 0, { reply_to: root })"
                :resolve="resolveThread"
                :apply="apply"
                :in-flow="reading.where === 'flow'"
                @ref="onRef"
                @flow="reveal"
              />
            </CommentCard>
            <VerdictBar
              v-if="cycle && (cycle.step === 1 || spec.on || spec.before)"
              :state="spec"
              :reviewer="reviewer(cycle.evidence.people?.spec)"
              :may="mayGive(cycle.evidence.people, cycle.evidence.author, me?.me?.login)"
              :moves="cycle.step === 1 && !!cycle.stages.ready"
              :criteria="!!cycle.evidence.criteria?.length"
              :busy="giving"
              :give="give"
            />
            <p v-if="verdictError" class="gb-save-error" role="alert"><GbIcon name="error" small /> {{ verdictError }}</p>
            <p v-if="folded"><button type="button" class="gb-linkbtn" aria-expanded="true" @click="reading.description = false">Fold the description</button></p>
          </template>
        </template>
        <template v-if="page" #review="r">
          <CycleReview :id="thread.id" :thread="thread" :me-id="meId" :busy="saving" :focus="focus" :review="review" :resolve="resolve" v-bind="r" />
        </template>
        <template #flow="{ cycle, failed, send }">
          <HistoryView v-if="reading.tab === 'history'" :thread="thread" :fields="fields" :cycle="cycle" />
          <IssueChanges v-else-if="reading.tab === 'changes'" :thread="thread" />
          <template v-else>
            <IssueFlow
              ref="flow"
              :events="thread.events"
              :cycle="cycle"
              :failed="failed"
              :me-id="meId"
              :since="since"
              :forges="thread.forges"
              :fields="fields"
              :save="edit"
              :reading="reading"
              :shown-v="shownV"
              :pointers="pointers"
              :busy="saving"
              :reply="(root: string, b: string) => comment(b, undefined, cycle?.step ?? 0, { reply_to: root })"
              :resolve="resolveThread"
              :apply="apply"
              @ref="onRef"
              @line="showLine"
              @workspace="showWorkspace"
              @thread="toSection"
              @compare="compare"
            />
            <IssueComposer
              :status="thread.status"
              :forges="thread.forges"
              :busy="saving"
              :waiting="waiting"
              :error="saveError"
              :carries="[carries(cycle?.step ?? 0), created ? `v${version(created)}` : ''].filter(Boolean)"
              :send="send?.name"
              :comment="(b: string, st?: 'open' | 'closed', toAgent?: boolean) => post(b, st, cycle?.step ?? 0, toAgent ? send : undefined)"
            />
          </template>
        </template>
        <template #side="{ cycle }">
          <IssueSide
            :id="thread.id"
            :title="thread.title"
            :labels="thread.labels"
            :names="sideNames"
            :fields="fields"
            :forges="thread.forges"
            :participants="thread.participants"
            :events="thread.events"
            :saving="saving"
            :spec="cycle && created && (cycle.step === 1 || spec.on || spec.before) ? spec : undefined"
            :reviewer="holder(cycle?.evidence.people?.spec)"
            @changed="fieldsChanged"
            @labels="labels"
            @reveal="reveal"
            @open="(other: string) => emit('open', other)"
          />
        </template>
      </IssueStep>
    </template>
  </component>
</template>

<script setup lang="ts">
import { onLive } from '@/live';
import { clampWidth, panelMode, panelWidth, togglePanelMode } from '@/panelsize';
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, toRef, useId, watch } from 'vue';
import { markSeen, type Thread } from '@/api/board';
import { inbox, inboxError, loadInbox } from '@/inbox';
import { loadVisit, saveVisit, visit } from '@/lastvisit';
import { fieldValues, type Fields } from '@/fields';
import { useIssue } from '@/issue';
import { me, on, when } from '@/me';
import CycleReview from '@/components/CycleReview.vue';
import CommentCard from '@/components/CommentCard.vue';
import HistoryView from '@/components/HistoryView.vue';
import IssueChanges from '@/components/IssueChanges.vue';
import { carries, loadReading, roleOf, rootOf, saveReading } from '@/flow';
import { ticksOf, version, versions } from '@/sections';
import { steps } from '@/replay';
import GbIcon from '@/components/GbIcon.vue';
import IssueComposer from '@/components/IssueComposer.vue';
import IssueFlow from '@/components/IssueFlow.vue';
import IssueSections from '@/components/IssueSections.vue';
import IssueSide from '@/components/IssueSide.vue';
import IssueStep from '@/components/IssueStep.vue';
import VerdictBar from '@/components/VerdictBar.vue';
import { mayGive, specOf } from '@/spec';
import { specVerdict } from '@/api/board';

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
  at?: string | undefined; // an event to scroll to once loaded: the inbox's first new one
  step?: number | undefined; // a segment to jump to once loaded: a rail link's ?step=
}>();
const emit = defineEmits<{ close: []; prev: []; next: []; changed: [thread: Thread]; open: [id: string] }>();

const uid = useId();
const { thread, error, saving, waiting, saveError, load, comment, edit, rename, setStatus, labels, review, resolve, resolveThread, apply } =
  useIssue(toRef(props, 'id'), (t) => emit('changed', t));
// the inbox opens an issue at its first new event
watch(
  () => thread.value?.id,
  async (id) => {
    if (id && props.at) void nextTick(() => reveal(props.at!));
    if (id && props.step) void nextTick(() => jumpTo(props.step!));
    // the last visit, for the flow's "new since" line: once per issue
    if (id) {
      const v = visit(loadVisit(`issue:${id}`), Date.now());
      saveVisit(`issue:${id}`, v);
      since.value = v.prev;
    }
    // an issue the inbox lists is read once opened, from the inbox, the
    // board or a link; on a fresh link the thread can come before the
    // inbox, so the inbox loads first. A failed write leaves it listed,
    // which shows.
    if (!id) return;
    if (!inbox.value && !inboxError.value) await loadInbox();
    if (inbox.value?.rows.some((r) => r.issue.id === id)) void markSeen([id]).then(loadInbox, () => {});
  },
);
const meId = computed(() => me.value?.me?.id ?? '');
const since = ref<number | null>(null);
const created = computed(() => thread.value?.events.find((e) => e.kind === 'created'));
// the description's versions, the one shown (the current one on each
// issue and each new version: not remembered) and Changes since
const bodyVersions = computed(() => (created.value ? versions(created.value) : []));
// the description's verdict (VerdictBar): on the current version, from the thread
const spec = computed(() => specOf(thread.value?.events ?? [], bodyVersions.value));
// the spec role's holder, its name and project role; "Person B, PM" for
// the verdict bar, anyone but the author without one
function holder(login: string | undefined) {
  if (!login) return undefined;
  const name = me.value?.identities?.find((p) => p.login?.toLowerCase() === login.toLowerCase())?.name ?? login;
  return { name, role: roleOf(props.fields, login) };
}
function reviewer(login: string | undefined): string {
  const h = holder(login);
  if (!h) return `Anyone but ${created.value?.author.name ?? 'the author'}`;
  return h.role ? `${h.name}, ${h.role.toUpperCase()}` : h.name;
}
const giving = ref(false);
const verdictError = ref('');
async function give(verdict: 'approve' | 'request-changes', body: string) {
  giving.value = true;
  verdictError.value = '';
  try {
    await specVerdict(props.id, verdict, `body@v${spec.value.v}`, body);
    await fieldsChanged(); // the status may have moved: the board's row too
    return true;
  } catch (e) {
    verdictError.value = e instanceof Error ? e.message : String(e);
    return false;
  } finally {
    giving.value = false;
  }
}
const shownV = ref(0);
const changesSince = ref(false);
watch([() => thread.value?.id, () => bodyVersions.value.length], () => ((shownV.value = bodyVersions.value.length), (changesSince.value = false)), { immediate: true });

// the person's reading: the tab (Conversation, or every change in order
// with the issue as it was), the flow's filters, order and grouping; kept
// in the browser, one for every issue
const reading = reactive(loadReading());
watch(reading, saveReading, { deep: true });
const step = ref<InstanceType<typeof IssueStep> | null>(null);
const flow = ref<InstanceType<typeof IssueFlow> | null>(null);
// the description's threads show under its sections when it is rendered
// and the seg says so; else in the flow (ponytail: while the description
// is being edited its sections are gone, so are the threads, to the flow)
const sections = ref<InstanceType<typeof IssueSections> | null>(null);
const pointers = computed(() => reading.where === 'section' && !!sections.value);
// a body thread's anchor in the flow: its place in the description
async function toSection(root: string) {
  reading.description = true;
  await nextTick();
  await sections.value?.unfold(root);
}
const focus = ref<string>(); // the file a comment's path:line opens in Changes, step 6 on the page
function showLine(path: string) {
  if (!props.page) return;
  focus.value = path;
  step.value?.show(6);
}
const changeCount = computed(() => (thread.value ? steps(thread.value.events).length : 0));
const comments = computed(() => thread.value?.events.filter((e) => e.kind === 'comment').length ?? 0);
const statusValue = computed(() => fieldValues(thread.value?.labels ?? [], 'status')[0] ?? '');
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
// a comment carries the issue's step (cycle:step); ticked, it goes to the
// agent too, as a brief
async function post(body: string, status: 'open' | 'closed' | undefined, n: number, send?: { brief: (body: string) => void }) {
  const ok = await comment(body, status, n);
  if (ok && send && body.trim()) send.brief(body);
  return ok;
}

// a done segment's Show the workspace: that step heads the column
async function showWorkspace(n: number) {
  step.value?.show(n);
  await nextTick();
  const head = rootEl()?.querySelector<HTMLElement>('.gb-column-body > .gb-eyebrow');
  head?.scrollIntoView({ block: 'start', behavior: 'smooth' });
  head?.focus({ preventScroll: true });
}

// the rail's jump: the Conversation tab, then the step's segment
async function jumpTo(n: number) {
  reading.tab = 'conversation';
  await nextTick();
  void flow.value?.jump(n);
}

// reveal scrolls to a history item and flashes it (a waiting change's
// row), on the Conversation tab with the flow settled and unfolded first.
async function reveal(id: string) {
  reading.tab = 'conversation';
  const e = thread.value?.events.find((x) => x.id === id);
  // a comment on the description under its section: the section unfolded
  const under = !!e?.meta?.['thread:anchor'] && reading.where === 'section';
  if (id === created.value?.id || under) reading.description = true;
  await nextTick();
  await flow.value?.settled();
  flow.value?.unfold(id);
  if (under) await sections.value?.unfold(rootOf(e!) ?? id);
  await nextTick();
  const el = document.querySelector<HTMLElement>(`[data-event="${CSS.escape(id)}"]`);
  if (!el) return;
  el.scrollIntoView({ block: 'center', behavior: 'smooth' });
  el.classList.remove('gb-flash');
  void el.offsetWidth; // restart the animation on a second click
  el.classList.add('gb-flash');
  setTimeout(() => el.classList.remove('gb-flash'), 1800);
}

// a milestone's Compare: the description open on that version, its
// changes since the one before marked
async function compare(v: number) {
  Object.assign(reading, { description: true });
  shownV.value = v;
  changesSince.value = true;
  await nextTick();
  document.querySelector(`[data-event="${CSS.escape(created.value?.id ?? '')}"]`)?.scrollIntoView({ block: 'start', behavior: 'smooth' });
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
// has no-focus): j / k / f / Esc work at once, Tab goes to the fields. It
// is also the first thing the dialog puts focus back on (tabindex 0,
// data-autofocus) when a browser loses it, as Safari does when j swaps the
// panel for the next one. Esc
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

// live updates (live.ts): this issue changed elsewhere. Reloaded at once,
// unless someone is writing here (a draft, an edit, a rename): then a
// notice offers the reload, and nothing typed is lost.
const stale = ref(false);
function writing(): boolean {
  if (renaming.value) return true;
  const el = rootEl();
  if (!el) return false;
  const active = document.activeElement;
  if (active && el.contains(active) && active.matches('input, textarea, select')) return true;
  return [...el.querySelectorAll('textarea')].some((t) => t.value.trim() !== '');
}
const offLive = onLive((e) => {
  if (e.kind !== 'issues' && e.kind !== 'sync') return;
  if (e.id && !props.id.startsWith(e.id) && !e.id.startsWith(props.id)) return;
  if (writing()) stale.value = true;
  else void load();
});
onBeforeUnmount(offLive);
async function reloadStale() {
  stale.value = false;
  await load();
}
</script>

<style scoped>
.gb-since { display: flex; gap: 6px; align-items: center; font-size: .8125rem; }
.gb-stale { display: flex; align-items: center; gap: .5rem; margin: 0; padding: .5rem 1rem; background: var(--info-bg); color: var(--info-fg); border-bottom: 1px solid var(--info-border); font-size: .875rem; }
.gb-stale-btn { padding: 0; border: 0; background: none; color: inherit; font: inherit; font-weight: 700; text-decoration: underline; cursor: pointer; }
.gb-modes-count { margin-left: .25rem; padding: 0 .375rem; border-radius: var(--radius); background: var(--bg-subtle); color: var(--fg-muted); font-size: .75rem; font-weight: 700; }
.gb-rename { flex: 1; display: flex; gap: .5rem; }
.gb-rename .epfl-input { flex: 1; font-size: 1.25rem; font-weight: 700; }
.gb-save-error, .gb-wait { display: flex; gap: .375rem; align-items: center; margin: .75rem 0 0; padding: .5rem .75rem; font-size: .875rem; }
.gb-save-error { color: var(--danger-fg); background: var(--danger-bg); }
.gb-wait { background: var(--bg-subtle); }
.gb-skel-title { margin: 0 0 1rem; font-size: 1.5rem; }
.gb-skel { height: 1rem; margin-top: .5rem; background: var(--bg-muted); }
.gb-skel-short { width: 40%; }
.gb-ptop-crumb a { display: inline-flex; align-items: center; gap: .25rem; color: var(--fg); }
</style>
