<!-- ⌘K: what's typed is the board's filter (one syntax, the server's), on
     every issue, open first; its first row puts it into the board's Filter
     box. An issue at once by its #number or id, and the commands (a view,
     New issue, Insights, Settings, Sync, the filters, shortcuts). Nothing
     typed: the issues opened lately, then the commands. -->
<template>
  <q-dialog v-model="paletteOpen" position="top" @show="onShow">
    <div class="cp" role="dialog" aria-label="Filter the issues, jump to one, or run a command">
      <div class="cp-search">
        <GbIcon name="search" small />
        <input
          ref="input"
          v-model="query"
          class="cp-input"
          type="text"
          role="combobox"
          aria-expanded="true"
          aria-controls="cp-list"
          :aria-activedescendant="results[active] ? `cp-${results[active]!.key}` : undefined"
          placeholder="Filter the issues, #2528, or a command"
          autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other"
          @keydown.down.prevent="move(1)"
          @keydown.up.prevent="move(-1)"
          @keydown.enter.prevent="run(results[active])"
          @keydown.tab="complete"
        />
        <kbd class="cp-esc">Esc</kbd>
      </div>
      <ul id="cp-list" class="cp-list" role="listbox">
        <template v-for="(item, i) in results" :key="item.key">
          <li v-if="i === 0 || results[i - 1]!.group !== item.group" class="cp-group" role="presentation">{{ item.group }}</li>
          <!-- picked by moving the mouse onto it, not by rows sliding under a
               still pointer (a move without movement): Enter would open
               that issue instead of the first row -->
          <li
            :id="`cp-${item.key}`"
            class="cp-item"
            :class="{ 'is-active': i === active }"
            role="option"
            :aria-selected="i === active"
            @mousemove="(e: MouseEvent) => (e.movementX || e.movementY) && (active = i)"
            @click="run(item)"
          >
            <GbIcon :name="item.icon" small />
            <span class="cp-label">{{ item.label }}</span>
            <span v-if="item.hint" class="cp-hint">{{ item.hint }}</span>
          </li>
        </template>
        <li v-if="error" class="cp-empty" role="status">{{ error }}</li>
      </ul>
    </div>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { filterValues, getIssues, type Issue } from '@/api/board';
import { filtersHidden, newIssueRequested, paletteOpen, recentIssues, recordRecent } from '@/commands';
import { message } from '@/issue';
import { jumps, openFirst, rank } from '@/palette';
import { suggest } from '@/viewfilter';
import { helpOpen } from '@/shortcuts';
import GbIcon, { type IconName } from '@/components/GbIcon.vue';

const props = defineProps<{ views: string[] }>();
const route = useRoute();
const router = useRouter();

type Entry = { key: string; label: string; group: string; icon: IconName; hint?: string | undefined; keep?: boolean; act: () => void };

const query = ref('');
const active = ref(0);
const input = ref<HTMLInputElement | null>(null);
const all = ref<Issue[]>([]); // every issue, for the jumps by number or id
const found = ref<Issue[] | null>(null); // the filter's answer; null: not yet
const searching = ref(false);
const error = ref('');
const values = ref<Record<string, string[]>>({});
void filterValues().then((v) => (values.value = v));

// status: → status:ready, …: Tab takes the first, as the board's box offers them
const suggestions = computed(() => suggest(query.value, values.value).slice(0, 4));
function complete(e: KeyboardEvent) {
  const s = suggestions.value[0];
  if (!s) return;
  e.preventDefault();
  query.value = s;
}

async function onShow() {
  query.value = '';
  active.value = 0;
  await nextTick();
  input.value?.focus();
  try {
    all.value = (await getIssues('is:open,closed')).flatMap((b) => b.issues);
  } catch {
    all.value = [];
  }
}

// The filter runs on the server once typing pauses, on every issue (no
// is:open added: the board shows the same issues for it); only the latest
// answer counts.
let timer: ReturnType<typeof setTimeout> | undefined;
let latest = 0;
watch(query, (typed) => {
  active.value = 0;
  clearTimeout(timer);
  const f = typed.trim();
  error.value = '';
  searching.value = !!f;
  if (!f) {
    found.value = null;
    return;
  }
  const mine = ++latest;
  timer = setTimeout(async () => {
    try {
      const res = (await getIssues(f)).flatMap((b) => b.issues);
      if (mine === latest) found.value = res;
    } catch (e) {
      if (mine === latest) {
        found.value = [];
        error.value = message(e);
      }
    } finally {
      if (mine === latest) searching.value = false;
    }
  }, 150);
});
watch(paletteOpen, (open) => {
  if (!open) {
    clearTimeout(timer);
    latest++;
  }
});

function openIssue(id: string, title: string, number?: string) {
  recordRecent({ id, title, number });
  if (route.path === '/') void router.replace({ query: { ...route.query, issue: id } });
  else void router.push(`/issues/${id}`);
}
const go = (path: string) => () => void router.push(path);

// Into the board's Filter box: on the board, the same view; elsewhere, the
// board's first one.
function filterBoard(f: string) {
  void router.push({ path: '/', query: route.path === '/' ? { ...route.query, issue: undefined, filter: f } : { filter: f } });
}

const commands = computed<Entry[]>(() => [
  { key: 'new', label: 'New issue', group: 'Commands', icon: 'plus', hint: 'c', act: () => { newIssueRequested.value = true; if (route.path !== '/') void router.push('/'); } },
  ...props.views.map((v): Entry => ({ key: `view-${v}`, label: `View: ${v}`, group: 'Commands', icon: 'tag', act: () => void router.push({ path: '/', query: { view: v } }) })),
  { key: 'insights', label: 'Insights', group: 'Commands', icon: 'chart', act: go('/insights') },
  { key: 'settings', label: 'Settings', group: 'Commands', icon: 'settings', act: go('/settings') },
  { key: 'sync', label: 'Sync with forges', group: 'Commands', icon: 'sync', act: go('/sync') },
  { key: 'filters', label: filtersHidden.value ? 'Show the filters' : 'Hide the filters', group: 'Commands', icon: 'filter', act: () => { filtersHidden.value = !filtersHidden.value; if (route.path !== '/') void router.push('/'); } },
  { key: 'keys', label: 'Keyboard shortcuts', group: 'Commands', icon: 'title', hint: '?', act: () => (helpOpen.value = true) },
]);

function issueEntry(i: Issue, group: string): Entry {
  const number = i.forges?.[0]?.number;
  return { key: `${group}-${i.id}`, label: i.title, group, icon: i.status === 'closed' ? 'closed' : 'open', hint: number ? `#${number}` : i.id.slice(0, 7), act: () => openIssue(i.id, i.title, number) };
}

// An exact #number or id first, at once (Enter opens it); else the filter
// for the board; then the commands, and the issues the filter finds.
const results = computed<Entry[]>(() => {
  const f = query.value.trim();
  if (!f) {
    const recent = recentIssues().map((r): Entry => ({ key: `r-${r.id}`, label: r.title, group: 'Opened lately', icon: 'clock', hint: r.number ? `#${r.number}` : r.id.slice(0, 7), act: () => openIssue(r.id, r.title, r.number) }));
    return [...recent, ...commands.value];
  }
  const items = all.value.map((i) => ({ key: i.id, label: i.title, id: i.id, numbers: i.forges?.flatMap((x) => (x.number ? [x.number] : [])), issue: i }));
  const jumped = jumps(f, items).map((j) => j.issue);
  const n = found.value?.length ?? 0;
  const filterRow: Entry = { key: 'filter', label: `Filter the board by «${f}»`, group: 'Filter', icon: 'filter', hint: searching.value || !found.value ? '…' : `${n} ${n === 1 ? 'issue' : 'issues'}`, act: () => filterBoard(f) };
  const issues = openFirst(found.value ?? []).filter((i) => !jumped.some((j) => j.id === i.id)).slice(0, 10);
  const completions = suggestions.value.map((s, i): Entry => ({ key: `s-${s}`, label: s, group: 'Complete', icon: 'filter', hint: i ? undefined : 'Tab', keep: true, act: () => {
    query.value = s;
    input.value?.focus();
  } }));
  return [...jumped.map((i) => issueEntry(i, 'Jump to')), filterRow, ...completions, ...rank(f, commands.value).slice(0, 4), ...issues.map((i) => issueEntry(i, 'Issues'))];
});

function move(d: number) {
  const n = results.value.length;
  if (n) active.value = (active.value + d + n) % n;
  void nextTick(() => document.getElementById(`cp-${results.value[active.value]?.key}`)?.scrollIntoView({ block: 'nearest' }));
}

function run(item: Entry | undefined) {
  if (!item) return;
  if (!item.keep) paletteOpen.value = false;
  item.act();
}
</script>

<style scoped>
.cp { width: min(40rem, calc(100vw - 2rem)); margin-top: 10vh; background: var(--bg); border: var(--border-w) solid var(--border-strong); box-shadow: var(--shadow); }
.cp-search { display: flex; align-items: center; gap: .625rem; padding: .75rem 1rem; border-bottom: var(--border-w) solid var(--border); color: var(--fg-muted); }
.cp-input { flex: 1; min-width: 0; border: 0; outline: none; background: none; color: var(--fg); font: inherit; font-size: 1.0625rem; }
.cp-esc, .cp-hint { font-family: var(--font-mono); font-size: .75rem; color: var(--fg-muted); }
.cp-esc { padding: .05rem .375rem; border: var(--border-w) solid var(--border-strong); }
.cp-list { max-height: min(26rem, 60vh); margin: 0; padding: .375rem 0; overflow-y: auto; list-style: none; }
.cp-group { padding: .5rem 1rem .25rem; font-size: .75rem; font-weight: var(--w-bold); text-transform: uppercase; letter-spacing: .06em; color: var(--primary); }
.cp-item { display: flex; align-items: center; gap: .625rem; padding: .5rem 1rem; cursor: pointer; color: var(--fg); }
.cp-item.is-active { background: var(--bg-muted); box-shadow: inset 3px 0 0 var(--primary); }
.cp-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cp-empty { padding: 1rem; color: var(--fg-muted); }
</style>
