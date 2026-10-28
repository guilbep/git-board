<!-- Settings › Sync, Start over: the board started again from what the
     forges hold, on the git side alone (internal/server/startover.go,
     docs/guide/forges.md "Starting over"). One card per action, in the
     order a board starts over, each confirmed by typing the repository's
     name; what it found is read when the section opens. done: the page
     reloads its rows after an action. -->
<template>
  <details class="so" @toggle="onToggle">
    <summary class="so-summary">
      <span class="so-heading">Start over</span>
      <span class="sy-note">Empty the board, here and on origin, and refill it from the forges</span>
    </summary>
    <div class="so-body">
      <p class="so-lead">On the git side alone: no forge and no GitHub Project is touched. To start over everywhere:</p>
      <ol class="so-steps"><li v-for="s in steps" :key="s">{{ s }}</li></ol>
      <ul class="so-notes"><li v-for="n in notes" :key="n">{{ n }}</li></ul>
      <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
      <p v-else-if="!st" class="sy-note">Loading…</p>
      <article v-for="c in cards" :key="c.id" class="so-card" :aria-labelledby="`so-${c.id}-title`">
        <h4 :id="`so-${c.id}-title`" class="so-title">{{ c.title }}</h4>
        <dl class="so-what">
          <div><dt>What it does</dt><dd>{{ c.does }}</dd></div>
          <div><dt>What it loses</dt><dd>{{ c.loses }}</dd></div>
          <div><dt>When</dt><dd>{{ c.when }}</dd></div>
          <div><dt>In a terminal</dt><dd><code>{{ c.command }}{{ c.id === 'prune' && onRemote ? ' --remote origin' : '' }}</code></dd></div>
        </dl>
        <template v-if="c.button">
          <label v-if="c.id === 'prune' && st?.remote" class="sy-check">
            <input v-model="onRemote" type="checkbox" /> Also on origin, with no backup there: only once no clone and no automation uses git-bug
          </label>
          <form class="so-confirm" @submit.prevent="run(c)">
            <label :for="`so-${c.id}`">Type <b>{{ repo || '…' }}</b> to confirm</label>
            <input :id="`so-${c.id}`" v-model="typed[c.id]" class="epfl-input" autocomplete="off" spellcheck="false" data-1p-ignore />
            <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy !== '' || !canConfirm(typed[c.id] ?? '', repo)">
              {{ busy === c.id ? 'Working…' : c.button }}
            </button>
          </form>
        </template>
        <p v-if="results[c.id]" class="so-result" role="status"><GbIcon name="check" small /> {{ results[c.id] }}</p>
      </article>
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { getStartOver, startOver, type StartOverStatus } from '@/api/board';
import { message } from '@/issue';
import { loadRemotes, remotes, repo } from '@/remotes';
import { canConfirm, cardsFor, notes, resultLine, steps, type Card, type CardId } from '@/startover';
import GbIcon from '@/components/GbIcon.vue';

const emit = defineEmits<{ done: [] }>();

const st = ref<StartOverStatus | null>(null);
const error = ref('');
const busy = ref<CardId | ''>('');
const typed = ref<Partial<Record<CardId, string>>>({});
const results = ref<Partial<Record<CardId, string>>>({});
const onRemote = ref(false);
const cards = computed(() => (st.value ? cardsFor(st.value) : []));

async function load() {
  try {
    st.value = await getStartOver();
    error.value = '';
  } catch (e) {
    error.value = message(e);
  }
}

// read when opened: a dry prune and the remotes, not worth it on every
// visit; the name to type is origin's, which the header loads
function onToggle(e: Event) {
  if (!(e.target as HTMLDetailsElement).open) return;
  if (!st.value) void load();
  if (!remotes.value) void loadRemotes();
}

async function run(c: Card) {
  if (!canConfirm(typed.value[c.id] ?? '', repo.value)) return;
  busy.value = c.id;
  error.value = '';
  try {
    results.value[c.id] = resultLine(await startOver(c.id, c.id === 'prune' && onRemote.value));
    typed.value[c.id] = '';
    emit('done');
    await load();
  } catch (e) {
    error.value = `${c.title}: not done. ${message(e)}`;
  } finally {
    busy.value = '';
  }
}
</script>

<style scoped>
.so { border-top: var(--border-w) solid var(--border-subtle); padding-top: 1rem; }
.so-summary { display: flex; flex-wrap: wrap; align-items: baseline; gap: .25rem 1rem; cursor: pointer; list-style: none; }
/* flex hides the browser's marker: draw one, so that it reads as opening */
.so-summary::-webkit-details-marker { display: none; }
.so-summary::before { content: '▸'; color: var(--fg-muted); }
.so[open] > .so-summary::before { content: '▾'; }
.so-heading { font-size: 1.125rem; font-weight: 700; }
.so-body { display: grid; gap: 1rem; margin-top: 1rem; }
.so-lead { margin: 0; max-width: 44rem; }
.so-steps, .so-notes { margin: 0; padding-left: 1.25rem; max-width: 44rem; }
.so-notes { font-size: .875rem; color: var(--fg-muted); }
.so-card { display: grid; gap: .75rem; padding: 1rem; border: var(--border-w) solid var(--border-subtle); border-radius: 4px; }
.so-title { margin: 0; font-size: 1rem; font-weight: 700; }
.so-what { display: grid; gap: .5rem; margin: 0; }
.so-what div { display: grid; grid-template-columns: 9rem 1fr; gap: .75rem; }
.so-what dt { color: var(--fg-muted); font-size: .875rem; }
.so-what dd { margin: 0; overflow-wrap: anywhere; }
.so-confirm { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; }
.so-confirm .epfl-input { flex: 0 1 14rem; min-width: 0; }
.so-result { margin: 0; overflow-wrap: anywhere; }
.sy-note { font-size: .8125rem; color: var(--fg-muted); }
.sy-check { display: flex; gap: .5rem; align-items: center; }
.sy-check input { accent-color: var(--primary-hover); }
@media (max-width: 40rem) {
  .so-what div { grid-template-columns: 1fr; gap: .125rem; }
}
</style>
