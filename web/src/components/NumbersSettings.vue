<!-- Settings › Numbers: the project's own numbers, GB-39
     (docs/guide/configuration.md, Numbers). Not on yet: what they are, then a
     form beside its guidelines: a prefix from the repository's name, a
     start above every number known (GitHub asked), what the issues
     become, and a confirm naming what is pushed where. On: where they
     stand, and Number them now for what origin has without one. -->
<template>
  <div class="set-panel nb">
    <h2 class="h4">Numbers</h2>
    <p class="nb-lead">
      One number per issue, the same on every forge: <b>{{ numberExample(n) }}</b>. The counter is kept in git, on origin, where every clone reads it.
    </p>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
    <p v-if="result" class="epfl-alert epfl-alert-success" role="status">{{ result }}</p>

    <p v-if="!n && !error" class="nb-muted">Loading…</p>

    <template v-if="n?.on">
      <p v-if="n.ahead" class="epfl-alert epfl-alert-warning" role="alert">
        <b>This clone's numbers aren't on origin yet.</b> The switch keyed them again here: the next
        <router-link to="/settings/sync">Sync</router-link> sends them first. If origin's numbers moved on meanwhile, it refuses: run <code>git board switch --apply</code> again.
      </p>
      <div class="nb-tiles">
        <div class="gb-value gb-value-agreed"><span class="gb-value-label">Prefix</span><span class="gb-value-val">{{ n.prefix }}</span></div>
        <div class="gb-value gb-value-agreed"><span class="gb-value-label">Start</span><span class="gb-value-val">{{ n.start }}</span></div>
        <div class="gb-value gb-value-agreed"><span class="gb-value-label">Next</span><span class="gb-value-val">{{ n.prefix }}-{{ n.next }}</span></div>
        <div class="gb-value gb-value-agreed"><span class="gb-value-label">Numbered</span><span class="gb-value-val">{{ issues(n.numbered) }}</span></div>
      </div>
      <section v-if="n.waiting" class="nb-wait" aria-labelledby="nb-wait">
        <p id="nb-wait"><b>{{ issues(n.waiting) }} on origin without a number.</b> {{ n.waiting === 1 ? 'It takes' : 'They take' }} {{ span(n.prefix!, n.next!, n.waiting) }}.</p>
        <div v-if="confirming" class="nb-confirm" role="alert">
          <p><b>This pushes refs/board/seq to origin ({{ where }}).</b> Nothing else goes anywhere.</p>
          <div class="nb-actions">
            <button type="button" class="epfl-btn epfl-btn-primary" :disabled="busy" @click="run">{{ busy ? 'Numbering…' : `Number ${n.waiting === 1 ? 'it' : 'them'}` }}</button>
            <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy" @click="confirming = false">Cancel</button>
          </div>
        </div>
        <button v-else type="button" class="epfl-btn epfl-btn-primary nb-start-btn" @click="confirming = true">Number them now</button>
      </section>
      <p v-if="n.unpushed">
        {{ issues(n.unpushed) }} not on origin yet: pushed and numbered with the next <router-link to="/settings/sync">Sync</router-link>.
      </p>
      <p v-if="!n.waiting && !n.unpushed" class="nb-muted">Every issue has its number.</p>
    </template>

    <div v-else-if="n" class="nb-setup">
      <form class="nb-form" @submit.prevent="confirming = true">
        <p>
          Today your issues go by their forge numbers. GitHub's stay the issues' own below the start (#41 is {{ prefixErr ? 'ABC' : prefix }}-41); every other issue, made here or
          imported later, takes the next number.
        </p>
        <p v-if="!n.origin" class="epfl-alert epfl-alert-warning" role="alert">
          No origin remote: the counter lives on origin. Add one (<code>git remote add origin &lt;url&gt;</code>), push your issues with a
          <router-link to="/settings/sync">Sync</router-link>, then come back.
        </p>
        <div>
          <label class="nb-label" for="nb-prefix">Prefix</label>
          <input
            id="nb-prefix"
            v-model.trim="prefix"
            class="epfl-input nb-prefix"
            autocomplete="off" data-1p-ignore data-lpignore="true" spellcheck="false"
            :disabled="!n.origin || confirming"
            :aria-invalid="!!prefixErr"
            aria-describedby="nb-prefix-hint"
          />
          <p id="nb-prefix-hint" class="gb-hint" :class="{ 'nb-bad': prefixErr }">{{ prefixErr || `Suggested from the repository's name, ${n.name || 'unknown'}.` }}</p>
        </div>
        <div>
          <label class="nb-label" for="nb-start">Start</label>
          <input
            id="nb-start"
            v-model.number="start"
            class="epfl-input nb-start"
            type="number" min="1" step="1"
            :disabled="!n.origin || confirming"
            :aria-invalid="!!startErr"
            aria-describedby="nb-start-hint"
          />
          <p id="nb-start-hint" class="gb-hint" :class="{ 'nb-bad': startErr }">{{ startErr || startHint }}</p>
          <p v-if="n.github?.error" class="gb-hint">
            Could be higher on GitHub: pull requests count too.
            <a :href="n.github.issues" target="_blank" rel="noopener noreferrer">See the repository's issues</a>
          </p>
        </div>
        <p v-if="n.origin && !prefixErr && !startErr" class="nb-preview" role="status">{{ preview }}</p>
        <div v-if="confirming" class="nb-confirm" role="alert">
          <p><b>This pushes refs/board/seq to origin ({{ where }}).</b> Then it numbers what origin has, with a second push of it. Nothing else goes anywhere: no issue, no forge.</p>
          <div class="nb-actions">
            <button type="button" class="epfl-btn epfl-btn-primary" :disabled="busy" @click="init">{{ busy ? 'Starting…' : 'Start numbering' }}</button>
            <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy" @click="confirming = false">Back</button>
          </div>
        </div>
        <button v-else type="submit" class="epfl-btn epfl-btn-primary nb-start-btn" :disabled="!ready">Start numbering…</button>
      </form>

      <aside class="nb-help" aria-labelledby="nb-help-title">
        <h3 id="nb-help-title" class="nb-help-title">Before you start</h3>
        <ul>
          <li><b>Choose the prefix once.</b> Once issues carry it, it stays: branches, commits and messages name them by it.</li>
          <li><b>2 to 5 capitals</b> is usual: CO2, GB, INFRA.</li>
          <li><b>Start above every GitHub issue and pull request number.</b> They share one sequence: at or below one, a later import collides with a number already given.</li>
          <li><b>Numbers go out only with a push you ask for</b>: Start numbering, Number them now, a Sync, <code>git board numbers</code>. Only issues origin has get one.</li>
          <li><b>Nothing pushes on its own</b>: not creating an issue, not a forge sync.</li>
        </ul>
        <p>From a terminal: <code>git board numbers init --prefix {{ prefixErr ? 'ABC' : prefix }} --start {{ startErr ? 100 : start }}</code>.</p>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { getNumbers, numberIssues, startNumbers, type Numbers } from '@/api/board';
import { debounced, onLive } from '@/live';
import { message } from '@/issue';
import { numbered, numberExample, numbersPreview, prefixProblem, startProblem, suggestPrefix, suggestStart } from '@/numbers';

// changed: what a start or a numbering left, for the Overview's line
const emit = defineEmits<{ changed: [n: Numbers] }>();
const n = ref<Numbers | null>(null);
const prefix = ref('');
const start = ref(0);
const confirming = ref(false);
const busy = ref(false);
const error = ref('');
const result = ref('');

const issues = (k: number) => `${k} issue${k === 1 ? '' : 's'}`;
const span = (p: string, from: number, count: number) => (count === 1 ? `${p}-${from}` : `${p}-${from} … ${p}-${from + count - 1}`);

// above every number known: the issues' forge numbers, GitHub's when it answered
const above = computed(() => Math.max(n.value?.highest ?? 0, n.value?.github?.highest ?? 0));
const prefixErr = computed(() => prefixProblem(prefix.value));
const startErr = computed(() => startProblem(start.value, above.value));
const ready = computed(() => !!n.value?.origin && !prefixErr.value && !startErr.value);
const where = computed(() => n.value?.origin?.push_urls.join(', ') ?? '');
const preview = computed(() => numbersPreview(prefix.value, start.value, n.value?.github_numbers ?? [], n.value?.unpushed ?? 0));
const startHint = computed(() => {
  const mine = n.value?.highest ?? 0;
  const gh = n.value?.github;
  if (gh && !gh.error && gh.highest >= mine) {
    return `Suggested: one above #${gh.highest}, GitHub's newest issue or pull request.${mine ? ` Your issues go up to #${mine}.` : ''}`;
  }
  return mine ? `Suggested: one above #${mine}, the highest forge number your issues have.` : 'No issue has a forge number yet.';
});

let filled = false;
function take(fresh: Numbers) {
  // a reload keeps what GitHub said: it is asked once
  n.value = { ...fresh, github: fresh.github ?? n.value?.github };
  if (!fresh.on && !filled) {
    filled = true;
    prefix.value = suggestPrefix(fresh.name);
    start.value = suggestStart(fresh.highest, fresh.github?.highest);
  }
}

async function load(github = false) {
  try {
    take(await getNumbers(github));
  } catch (e) {
    error.value = message(e);
  }
}

async function act(what: () => Promise<Numbers>, done: (r: Numbers) => string) {
  busy.value = true;
  error.value = result.value = '';
  try {
    const r = await what();
    take(r);
    emit('changed', r);
    result.value = done(r);
    error.value = r.error ?? '';
    confirming.value = false;
  } catch (e) {
    error.value = message(e);
    confirming.value = false;
    void load(); // someone else may have started or numbered meanwhile
  } finally {
    busy.value = false;
  }
}

const init = () => act(() => startNumbers(prefix.value, start.value), (r) => `Numbering as ${r.prefix}-N from ${r.start}. ${numbered(r.prefix!, r.next!, r.done)}`);
const run = () => act(numberIssues, (r) => numbered(r.prefix!, r.next!, r.done));

onMounted(() => void load(true));
// an issue made, or pushed with a Sync: the counts move
const reload = debounced(() => void load());
const offLive = onLive((e) => (e.kind === 'issues' || e.kind === 'sync') && reload());
onBeforeUnmount(offLive);
</script>

<style scoped>
.nb h2, .nb h3, .nb p { margin: 0; }
.nb-lead { max-width: 40rem; color: var(--fg-muted); }
.nb-muted { color: var(--fg-muted); }
.nb-tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr)); gap: .75rem; max-width: 45rem; }
.nb-wait { display: grid; gap: .75rem; justify-items: start; }
.nb-setup { display: grid; grid-template-columns: minmax(0, 1fr) minmax(16rem, 22rem); gap: 2rem; align-items: start; }
.nb-form { display: flex; flex-direction: column; gap: 1.25rem; min-width: 0; }
.nb-label { display: block; margin-bottom: .375rem; font-weight: 700; }
.nb-prefix { max-width: 10rem; }
.nb-start { max-width: 10rem; }
.nb-bad { color: var(--danger-fg); }
.nb-preview { padding: .75rem 1rem; background: var(--bg-subtle); border-left: 3px solid var(--border-strong); }
.nb-confirm { display: grid; gap: .75rem; padding: .875rem 1rem; background: var(--warning-bg); }
.nb-actions { display: flex; flex-wrap: wrap; gap: .5rem; }
.nb-start-btn { align-self: flex-start; }
.nb-help { display: flex; flex-direction: column; gap: .75rem; padding: 1.25rem 1.5rem; background: var(--bg-subtle); font-size: .875rem; }
.nb-help-title { font-size: 1.0625rem; font-weight: 700; }
.nb-help ul { display: grid; gap: .625rem; margin: 0; padding-left: 1.125rem; }
.nb-help code, .nb code { font-size: .8125rem; overflow-wrap: anywhere; }
@media (max-width: 59.99rem) {
  .nb-setup { grid-template-columns: minmax(0, 1fr); }
}
</style>
