<!-- Every keyboard shortcut, in one list: ? opens it, Esc closes it. -->
<template>
  <q-dialog v-model="helpOpen">
    <section class="kh" role="document" aria-labelledby="kh-title">
      <header class="kh-head">
        <h2 id="kh-title" class="h5">Keyboard shortcuts</h2>
        <button type="button" class="gb-icon-btn" aria-label="Close" @click="helpOpen = false"><GbIcon name="close" /></button>
      </header>
      <div class="kh-groups">
        <div v-for="g in groups" :key="g.title" class="kh-group">
          <h3 class="kh-group-title">{{ g.title }}</h3>
          <dl>
            <template v-for="k in g.keys" :key="k.what">
              <dt><kbd v-for="key in k.keys" :key="key">{{ key }}</kbd></dt>
              <dd>{{ k.what }}</dd>
            </template>
          </dl>
        </div>
      </div>
    </section>
  </q-dialog>
</template>

<script setup lang="ts">
import { helpOpen } from '@/shortcuts';
import GbIcon from '@/components/GbIcon.vue';

const mod = typeof navigator !== 'undefined' && /Mac/.test(navigator.platform) ? '⌘' : 'Ctrl';
const alt = mod === '⌘' ? '⌥' : 'Alt';
const groups = [
  {
    title: 'Board',
    keys: [
      { keys: ['c'], what: 'New issue' },
      { keys: ['?'], what: 'This list' },
    ],
  },
  {
    title: 'Issue panel',
    keys: [
      { keys: ['j'], what: 'Next issue' },
      { keys: ['k'], what: 'Previous issue' },
      { keys: ['f'], what: 'Wide over the board, or back to the side' },
      { keys: ['Esc'], what: 'Leave the field you type in; again: close the panel' },
      { keys: [mod, 'Enter'], what: 'Send the comment' },
    ],
  },
  {
    title: 'Filters',
    keys: [
      { keys: ['Click'], what: 'Add or remove a value' },
      { keys: [alt, 'Click'], what: 'Only this value; again: every value but it' },
      { keys: ['Shift', 'Click'], what: 'A range, from the last value clicked' },
      { keys: ['Space'], what: 'Toggle the focused value' },
    ],
  },
  {
    title: 'Settings: configuration',
    keys: [{ keys: [mod, 'S'], what: 'Review the changes before saving' }],
  },
];
</script>

<style scoped>
.kh { width: min(44rem, calc(100vw - 2rem)); max-height: calc(100vh - 4rem); overflow: auto; padding: 1.25rem 1.5rem 1.5rem; background: var(--bg); }
.kh-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: .75rem; }
.kh-head h2 { margin: 0; }
.kh-groups { display: grid; grid-template-columns: repeat(auto-fit, minmax(18rem, 1fr)); gap: 1.25rem 2rem; }
.kh-group-title { margin: 0 0 .5rem; font-size: .8125rem; font-weight: 700; text-transform: uppercase; letter-spacing: .06em; color: var(--primary); }
.kh dl { display: grid; grid-template-columns: auto 1fr; gap: .5rem .75rem; margin: 0; align-items: baseline; }
.kh dt { display: flex; gap: .25rem; white-space: nowrap; }
.kh dd { margin: 0; font-size: .875rem; }
.kh kbd { padding: .05rem .375rem; border: 1px solid var(--border-strong); border-bottom-width: 2px; background: var(--bg-subtle); font-family: var(--font-mono); font-size: .75rem; }
</style>
