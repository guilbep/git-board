<!-- ⌘K: jump to an issue by #number, id or title, or run a command (a
     view, New issue, Insights, Settings, Sync, the filters, shortcuts).
     Nothing typed: the issues opened lately, then the commands. -->
<template>
  <q-dialog v-model="paletteOpen" position="top" @show="onShow">
    <div class="cp" role="dialog" aria-label="Jump to an issue or a command">
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
          placeholder="Search issues by #number, id or title, or type a command"
          autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other"
          @keydown.down.prevent="move(1)"
          @keydown.up.prevent="move(-1)"
          @keydown.enter.prevent="run(results[active])"
        />
        <kbd class="cp-esc">Esc</kbd>
      </div>
      <ul id="cp-list" class="cp-list" role="listbox">
        <template v-for="(item, i) in results" :key="item.key">
          <li v-if="i === 0 || results[i - 1]!.group !== item.group" class="cp-group" role="presentation">{{ item.group }}</li>
          <li
            :id="`cp-${item.key}`"
            class="cp-item"
            :class="{ 'is-active': i === active }"
            role="option"
            :aria-selected="i === active"
            @mousemove="active = i"
            @click="run(item)"
          >
            <GbIcon :name="item.icon" small />
            <span class="cp-label">{{ item.label }}</span>
            <span v-if="item.hint" class="cp-hint">{{ item.hint }}</span>
          </li>
        </template>
        <li v-if="!results.length" class="cp-empty" role="presentation">{{ loading ? 'Loading the issues…' : 'Nothing matches.' }}</li>
      </ul>
    </div>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { getIssues, type Issue } from '@/api/board';
import { filtersHidden, newIssueRequested, paletteOpen, recentIssues, recordRecent } from '@/commands';
import { rank } from '@/palette';
import { helpOpen } from '@/shortcuts';
import GbIcon, { type IconName } from '@/components/GbIcon.vue';

const props = defineProps<{ views: string[] }>();
const route = useRoute();
const router = useRouter();

type Entry = { key: string; label: string; group: string; icon: IconName; hint?: string | undefined; id?: string | undefined; number?: string | undefined; act: () => void };

const query = ref('');
const active = ref(0);
const input = ref<HTMLInputElement | null>(null);
const issues = ref<Issue[]>([]);
const loading = ref(false);

async function onShow() {
  query.value = '';
  active.value = 0;
  await nextTick();
  input.value?.focus();
  loading.value = true;
  try {
    issues.value = (await getIssues('is:open', '')).flatMap((b) => b.issues);
  } catch {
    issues.value = [];
  } finally {
    loading.value = false;
  }
}

function openIssue(id: string, title: string, number?: string) {
  recordRecent({ id, title, number });
  if (route.path === '/') void router.replace({ query: { ...route.query, issue: id } });
  else void router.push(`/issues/${id}`);
}
const go = (path: string) => () => void router.push(path);

const commands = computed<Entry[]>(() => [
  { key: 'new', label: 'New issue', group: 'Commands', icon: 'plus', hint: 'c', act: () => { newIssueRequested.value = true; if (route.path !== '/') void router.push('/'); } },
  ...props.views.map((v): Entry => ({ key: `view-${v}`, label: `View: ${v}`, group: 'Commands', icon: 'tag', act: () => void router.push({ path: '/', query: { view: v } }) })),
  { key: 'insights', label: 'Insights', group: 'Commands', icon: 'chart', act: go('/insights') },
  { key: 'settings', label: 'Settings', group: 'Commands', icon: 'settings', act: go('/settings') },
  { key: 'sync', label: 'Sync with forges', group: 'Commands', icon: 'sync', act: go('/sync') },
  { key: 'add-forge', label: 'Add a forge', group: 'Commands', icon: 'plus', act: go('/settings/forges?add=1') },
  { key: 'filters', label: filtersHidden.value ? 'Show the filters' : 'Hide the filters', group: 'Commands', icon: 'filter', act: () => { filtersHidden.value = !filtersHidden.value; if (route.path !== '/') void router.push('/'); } },
  { key: 'keys', label: 'Keyboard shortcuts', group: 'Commands', icon: 'title', hint: '?', act: () => (helpOpen.value = true) },
]);

const issueEntries = computed<Entry[]>(() =>
  issues.value.map((i) => {
    const number = i.forges?.[0]?.number;
    return { key: `i-${i.id}`, label: i.title, group: 'Issues', icon: i.status === 'closed' ? 'closed' : 'open', hint: number ? `#${number}` : i.id.slice(0, 7), id: i.id, number, act: () => openIssue(i.id, i.title, number) };
  }),
);

const results = computed<Entry[]>(() => {
  if (!query.value.trim()) {
    const recent = recentIssues().map((r): Entry => ({ key: `r-${r.id}`, label: r.title, group: 'Opened lately', icon: 'clock', hint: r.number ? `#${r.number}` : r.id.slice(0, 7), act: () => openIssue(r.id, r.title, r.number) }));
    return [...recent, ...commands.value];
  }
  return rank(query.value, [...issueEntries.value, ...commands.value]).slice(0, 12);
});
watch(query, () => (active.value = 0));

function move(d: number) {
  const n = results.value.length;
  if (n) active.value = (active.value + d + n) % n;
  void nextTick(() => document.getElementById(`cp-${results.value[active.value]?.key}`)?.scrollIntoView({ block: 'nearest' }));
}

function run(item: Entry | undefined) {
  if (!item) return;
  paletteOpen.value = false;
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
