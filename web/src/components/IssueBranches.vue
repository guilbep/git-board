<!-- The side column's Development: the branches whose name names the issue
     (internal/branches), against the base, and Create branch: the name
     the issue suggests, as commands to copy. git-board creates no branch. -->
<template>
  <section class="gb-side-sec" aria-labelledby="side-dev">
    <h3 id="side-dev" class="gb-section-title">Development</h3>
    <p v-if="error" class="gb-hint" role="alert"><b>Couldn’t read the branches.</b> {{ error }}</p>
    <template v-else-if="dev">
      <ul v-if="dev.branches.length" class="gb-dev-list">
        <li v-for="b in dev.branches" :key="`${b.remote}:${b.name}`" class="gb-dev-branch">
          <span class="gb-mono gb-dev-name">{{ b.remote ? `${b.remote}/` : '' }}{{ b.name }}</span>
          <span class="gb-dev-meta">
            <b v-if="b.merged && b.behind" class="gb-state-ok">merged into {{ dev.base }}</b>
            <b v-else-if="b.in_base && !b.merged" class="gb-state-ok" :title="`Its changes are in ${dev.base}: squashed or rebased in`">in {{ dev.base }}, squashed</b>
            <template v-else-if="b.merged">same as {{ dev.base }}</template>
            <template v-else>not in {{ dev.base }} · {{ b.ahead }} ahead, {{ b.behind }} behind</template>
          </span>
          <span class="gb-dev-meta">{{ b.last_commit.subject }} · <time :datetime="b.last_commit.date">{{ when(b.last_commit.date) }}</time></span>
        </li>
      </ul>
      <p v-else class="gb-hint">No branch yet.</p>
      <details class="gb-dev-new">
        <summary class="epfl-btn epfl-btn-secondary epfl-btn-sm"><GbIcon name="plus" small /> Create branch</summary>
        <div class="gb-dev-panel">
          <label class="epfl-label" :for="`dev-name-${id}`">Branch name</label>
          <input :id="`dev-name-${id}`" v-model="name" class="epfl-input gb-mono" autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" spellcheck="false" />
          <p class="gb-dev-meta">From <span class="gb-mono">{{ dev.start }}</span>. Run in the repository:</p>
          <pre class="gb-dev-cmd"><code>{{ commands }}</code></pre>
          <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!name.trim()" @click="copy">
            <GbIcon v-if="copied" name="check" small /> {{ copied ? 'Copied' : 'Copy' }}
          </button>
        </div>
      </details>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { getBranches, type IssueBranches } from '@/api/board';
import { message } from '@/issue';
import { when } from '@/me';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ id: string; labels: string[]; title: string }>();

// the kind and the title name the branch: reread when they change
const dev = ref<IssueBranches | null>(null);
const error = ref('');
const name = ref('');
async function load() {
  try {
    dev.value = await getBranches(props.id);
    name.value = dev.value.suggested;
    error.value = '';
  } catch (e) {
    error.value = message(e);
  }
}
watch(() => [props.id, props.labels.join(' '), props.title], load, { immediate: true });

const commands = computed(() => `git switch -c ${name.value.trim()} ${dev.value?.start ?? ''}\ngit push -u origin ${name.value.trim()}`);

const copied = ref(false);
async function copy() {
  try {
    await navigator.clipboard.writeText(commands.value);
    copied.value = true;
    setTimeout(() => (copied.value = false), 2000);
  } catch {
    window.prompt('Copy the commands', commands.value.replace('\n', ' && '));
  }
}
</script>

<style scoped>
.gb-dev-list { display: flex; flex-direction: column; gap: .75rem; margin: 0; padding: 0; list-style: none; }
.gb-dev-branch { display: flex; flex-direction: column; gap: .125rem; font-size: .875rem; }
.gb-dev-name { font-weight: 700; overflow-wrap: anywhere; }
.gb-dev-meta { font-size: .8125rem; color: var(--fg-muted); }
.gb-dev-new { margin-top: .75rem; }
.gb-dev-new summary { display: inline-flex; align-items: center; gap: .375rem; list-style: none; cursor: pointer; }
.gb-dev-new summary::-webkit-details-marker { display: none; }
.gb-dev-panel { display: flex; flex-direction: column; align-items: flex-start; gap: .5rem; margin-top: .5rem; padding: .75rem; border: var(--border-w) solid var(--border-strong); background: var(--bg); }
.gb-dev-panel .epfl-input { width: 100%; min-width: 0; box-sizing: border-box; }
.gb-dev-panel .gb-dev-meta { margin: 0; }
.gb-dev-cmd { width: 100%; box-sizing: border-box; margin: 0; padding: .5rem; background: var(--bg-subtle); font-family: var(--font-mono); font-size: .8125rem; white-space: pre-wrap; overflow-wrap: anywhere; }
.gb-dev-cmd code { color: var(--fg); }
</style>
