<!-- Settings › Worktrees (G1): wtx, on the machine where git board serve
     runs. The tools line, then "Set up wtx": what wtx read in the
     repository (wtx init --edit), corrected here, previewed through wtx
     and written by it. git-board renders no TOML: wtx is the only writer
     of wtx.toml and .wt.toml (docs/configuration.md, Worktrees). -->
<template>
  <div class="set-panel wk">
    <h2 class="h4">Worktrees</h2>
    <p class="wk-lead">One agent, one worktree, one branch, one PR. git-board starts them with wtx, on the machine where git-board serve runs.</p>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
    <p v-if="!d && !error" class="wk-muted">Loading…</p>

    <template v-if="d">
      <p class="wk-tools" :class="{ 'wk-bad': !tools.ok }">{{ tools.ok ? '✓' : '!' }} {{ tools.text }}</p>
      <p v-if="d.wtx && d.local && !d.config" class="wk-muted">No wtx.toml in this repository yet</p>
      <p v-if="d.reason && d.code !== 'no-config'" class="epfl-alert epfl-alert-warning" role="alert">{{ d.reason }}</p>
      <p v-if="result" class="epfl-alert epfl-alert-success" role="status">{{ result }}</p>

      <form v-if="a" class="wk-form" @submit.prevent>
        <h3 class="h5">Set up wtx</h3>
        <p class="wk-muted">What wtx read in the repository. Change what's wrong.</p>

        <div class="wk-row">
          <label class="wk-label" for="wk-base">Base branch<span class="gb-hint">From the repository's branches.</span></label>
          <input id="wk-base" v-model.trim="a.repo.base_branch" class="epfl-input wk-mono wk-short" spellcheck="false" autocomplete="off" />
        </div>

        <div class="wk-row">
          <span class="wk-label">Protected<span class="gb-hint">A worktree never pushes these.</span></span>
          <div class="wk-chips">
            <ul class="gb-chips" aria-label="Protected branches">
              <li v-for="b in a.repo.protected_branches" :key="b" class="gb-chip wk-mono">
                {{ b }}<button type="button" class="gb-chip-x" :aria-label="`Remove ${b}`" @click="drop(a.repo.protected_branches, b)">×</button>
              </li>
            </ul>
            <input v-model.trim="addProtected" class="epfl-input wk-mono wk-short" placeholder="Add" aria-label="Add a protected branch" @keydown.enter.prevent="add(a.repo.protected_branches, addProtected), (addProtected = '')" />
          </div>
        </div>

        <div class="wk-row">
          <span class="wk-label">Ports<span class="gb-hint">Each worktree gets its own. The main checkout keeps these.</span></span>
          <ul v-if="ports.length" class="gb-chips" aria-label="Ports">
            <li v-for="p in ports" :key="p.name" class="gb-chip wk-mono">{{ p.name }} {{ p.main }}</li>
          </ul>
          <span v-else class="wk-muted">None</span>
        </div>

        <div class="wk-row">
          <span class="wk-label">Panes<span class="gb-hint">What runs beside the agent.</span></span>
          <ul v-if="panes.length" class="gb-chips" aria-label="Panes">
            <li v-for="p in panes" :key="p.name" class="gb-chip wk-mono">{{ p.name }}{{ p.cmd ? ` ${p.cmd}` : '' }}</li>
          </ul>
          <span v-else class="wk-muted">None</span>
        </div>

        <div class="wk-row">
          <label class="wk-label" for="wk-agent">Agent<span class="gb-hint">Plan on the strongest model, build on the next.</span></label>
          <div class="wk-agent">
            <select id="wk-agent" v-model="a.agent.tool" class="epfl-input wk-short">
              <option value="claude">Claude Code</option>
              <option value="opencode">opencode</option>
            </select>
            <label v-if="a.agent.orchestration" class="wk-orch">
              <input v-model="a.agent.orchestration.enabled" type="checkbox" />
              plans on
              <input v-model.trim="a.agent.orchestration.plan_model" class="epfl-input wk-mono wk-model" aria-label="Plans on" :disabled="!a.agent.orchestration.enabled" />
              builds on
              <input v-model.trim="a.agent.orchestration.build_model" class="epfl-input wk-mono wk-model" aria-label="Builds on" :disabled="!a.agent.orchestration.enabled" />
            </label>
          </div>
        </div>

        <div class="wk-row">
          <span id="wk-who" class="wk-label">Who commits<span class="gb-hint">The gate on step 5.</span></span>
          <div>
            <div class="gb-opts" role="radiogroup" aria-labelledby="wk-who">
              <label class="gb-opt" :class="{ on: who === 'person' }">
                <input v-model="who" type="radio" name="wk-who" value="person" />
                <span><b>A person commits and opens the PR</b></span>
              </label>
              <label class="gb-opt" :class="{ on: who === 'agent' }">
                <input v-model="who" type="radio" name="wk-who" value="agent" />
                <span><b>The agent may commit; a person pushes</b></span>
              </label>
            </div>
            <p v-if="unset" class="gb-hint">Not in wtx.toml yet: the default is the deck's gate.</p>
          </div>
        </div>

        <div class="wk-row">
          <span class="wk-label">Never allowed<span class="gb-hint">Added to wtx's own rules.</span></span>
          <div class="wk-chips">
            <ul class="gb-chips" aria-label="Never allowed">
              <li v-for="r in extra" :key="r" class="gb-chip wk-mono">
                {{ r }}<button type="button" class="gb-chip-x" :aria-label="`Remove ${r}`" @click="drop(a.permissions.deny, r)">×</button>
              </li>
            </ul>
            <input v-model.trim="addDeny" class="epfl-input wk-mono" placeholder="Bash(git tag *)" aria-label="Add a rule" @keydown.enter.prevent="add(a.permissions.deny, addDeny), (addDeny = '')" />
            <p class="wk-muted">wtx's own: {{ WTX_OWN }}.</p>
          </div>
        </div>

        <div v-if="a._changes?.length" class="gb-note">
          <p>Since wtx.toml was written the repository moved:</p>
          <!-- ponytail: wtx's _changes shape is not documented; shown raw until it is -->
          <ul>
            <li v-for="(c, i) in a._changes" :key="i"><code>{{ change(c) }}</code></li>
          </ul>
        </div>

        <div v-if="confirming" class="wk-confirm" role="alert">
          <p><b>Writes wtx.toml and .wt.toml in {{ a.repo.name || 'the repository' }}'s main checkout, uncommitted.</b></p>
          <div class="wk-actions">
            <button type="button" class="epfl-btn epfl-btn-primary" :disabled="!!busy" @click="write">{{ busy === 'write' ? 'Writing…' : 'Write' }}</button>
            <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="!!busy" @click="confirming = false">Cancel</button>
          </div>
        </div>
        <div v-else class="wk-actions">
          <button type="button" class="epfl-btn epfl-btn-primary" :disabled="!!busy || !d.local || !d.wtx" @click="confirming = true">Write wtx.toml</button>
          <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="!!busy || !d.local || !d.wtx" @click="preview">{{ busy === 'preview' ? 'Previewing…' : 'Preview the files' }}</button>
        </div>
        <p class="gb-hint">Writes wtx.toml and .wt.toml. You commit them.</p>

        <section v-for="f in files" :key="f.path" class="wk-file">
          <h4 class="wk-path wk-mono">{{ f.path }}</h4>
          <pre class="gb-md">{{ f.text }}</pre>
        </section>
      </form>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { getWorktrees, previewWtx, writeWtx, type Worktrees, type WtxAnswers } from '@/api/board';
import { message } from '@/issue';
import { extraDeny, setWhoCommits, toolsLine, whoCommits, WTX_OWN } from '@/worktrees';

// changed: what a load or a write left, for the Overview's line
const emit = defineEmits<{ changed: [d: Worktrees] }>();
const d = ref<Worktrees | null>(null);
const a = ref<WtxAnswers | null>(null);
const files = ref<{ path: string; text: string }[]>([]);
const error = ref('');
const result = ref('');
const busy = ref<'' | 'preview' | 'write'>('');
const confirming = ref(false);
const addProtected = ref('');
const addDeny = ref('');

const tools = computed(() => (d.value ? toolsLine(d.value) : { ok: true, text: '' }));
const ports = computed(() => a.value?.ports?.family ?? []);
const panes = computed(() => a.value?.panes?.pane ?? []);
const extra = computed(() => extraDeny(a.value?.permissions.deny ?? []));
const unset = computed(() => whoCommits(a.value?.permissions.deny ?? []) === '');
// '' preselects a person: the deck's gate
const who = computed({
  get: () => whoCommits(a.value?.permissions.deny ?? []) || 'person',
  set: (w) => a.value && (a.value.permissions.deny = setWhoCommits(a.value.permissions.deny, w)),
});

function add(list: string[], v: string) {
  if (v && !list.includes(v)) list.push(v);
}
function drop(list: string[], v: string) {
  list.splice(list.indexOf(v), 1);
}
function change(c: unknown): string {
  if (c && typeof c === 'object') {
    const o = c as Record<string, unknown>;
    const picked = Object.fromEntries(['key', 'file', 'scan'].filter((k) => k in o).map((k) => [k, o[k]]));
    if (Object.keys(picked).length) return JSON.stringify(picked);
  }
  return JSON.stringify(c);
}

async function load() {
  try {
    const fresh = await getWorktrees();
    d.value = fresh;
    a.value = fresh.answers ?? null;
    if (a.value) {
      a.value.permissions ??= { deny: [] };
      a.value.permissions.deny ??= [];
      a.value.repo.protected_branches ??= [];
    }
    emit('changed', fresh);
  } catch (e) {
    error.value = message(e);
  }
}

async function act(kind: 'preview' | 'write') {
  if (!a.value) return;
  busy.value = kind;
  error.value = result.value = '';
  try {
    files.value = (await (kind === 'write' ? writeWtx : previewWtx)(a.value)).files;
    if (kind === 'write') {
      result.value = 'Wrote wtx.toml and .wt.toml. You commit them.';
      void load();
    }
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = '';
    confirming.value = false;
  }
}
const preview = () => act('preview');
const write = () => act('write');

onMounted(() => void load());
</script>

<style scoped>
.wk h2, .wk h3, .wk h4, .wk p { margin: 0; }
.wk-lead { max-width: 40rem; color: var(--fg-muted); }
.wk-muted { color: var(--fg-muted); }
.wk-tools { font-weight: 700; }
.wk-bad { color: var(--danger-fg); }
.wk-form { display: flex; flex-direction: column; gap: 1rem; max-width: 60rem; }
.wk-row { display: grid; grid-template-columns: 200px minmax(0, 1fr); gap: 1rem; align-items: start; padding-top: 1rem; border-top: 1px solid var(--border); }
.wk-label { display: flex; flex-direction: column; gap: .25rem; font-weight: 700; }
.wk-label .gb-hint { font-weight: 400; }
.wk-mono { font-family: var(--font-mono, ui-monospace, monospace); font-size: .8125rem; }
.wk-short { max-width: 14rem; }
.wk-model { max-width: 9rem; }
.wk-chips { display: grid; gap: .5rem; min-width: 0; }
.wk-agent { display: flex; flex-wrap: wrap; gap: .75rem; align-items: center; }
.wk-orch { display: flex; flex-wrap: wrap; gap: .5rem; align-items: center; }
.wk-confirm { display: grid; gap: .75rem; padding: .875rem 1rem; background: var(--warning-bg); }
.wk-actions { display: flex; flex-wrap: wrap; gap: .5rem; }
.wk-file { display: grid; gap: .375rem; }
.wk-path { font-weight: 700; }
.wk-file pre { margin: 0; padding: .75rem 1rem; overflow-x: auto; background: var(--bg-subtle); }
@media (max-width: 59.99rem) {
  .wk-row { grid-template-columns: minmax(0, 1fr); gap: .5rem; }
}
</style>
