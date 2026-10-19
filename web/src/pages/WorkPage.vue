<!-- The Work page: what needs a person, from every open issue's step
     (internal/server/work.go). Needs you is the rows whose gate waits for
     the viewer (or, in Team, for anyone), or that carry an agent's
     question; Waiting on others is the viewer's other issues (Team: every
     other open one). Agents are the linked worktrees; the aside is what
     waits to go to the forges and the sprint's corrections. -->
<template>
  <AppHeader :views="viewNames" active="" />
  <main class="gb-page gb-work">
    <div class="gb-work-main">
      <div class="epfl-eyebrow">Work</div>
      <h1 class="h3">What needs a person</h1>
      <p class="gb-lead">Human time is the bottleneck. Agents wait here, not in a terminal.</p>

      <div class="gb-seg gb-work-who" role="group" aria-label="Whose work">
        <button type="button" :aria-pressed="who === 'me'" @click="setWho('me')">Mine</button>
        <button type="button" :aria-pressed="who === 'team'" @click="setWho('team')">Team</button>
      </div>

      <p v-if="error?.code === 'no-identity'" class="gb-work-need">
        The page shows what waits for the identity you write as, and none is chosen here yet.
        <router-link to="/settings/you">Choose who you write as</router-link>
      </p>
      <p v-else-if="error?.code === 'no-login'" class="gb-work-need">
        Tell git-board your forge login: without it, no role and no field can name you. Run <code>git bug bridge new</code>, or start the server with <code>--me your-login</code>.
      </p>
      <p v-else-if="error" class="gb-error">{{ error.message }}</p>

      <template v-if="data">
        <section class="gb-work-group" aria-labelledby="needs-h">
          <h2 id="needs-h" class="h5">Needs you <span class="gb-work-n">{{ data.needs.length }}</span></h2>
          <div v-if="data.needs.length" class="gb-table-wrap">
            <table class="gb-table gb-work-table">
              <thead><tr><th>Issue</th><th>Step</th><th>Waits for</th><th>Since</th><th><span class="visually-hidden">Action</span></th></tr></thead>
              <tbody>
                <tr v-for="r in data.needs" :key="r.issue.id">
                  <td><router-link :to="`/issues/${r.issue.id}`"><span v-if="numberOf(r.issue)" class="gb-work-num">#{{ numberOf(r.issue) }}</span> {{ r.issue.shown_title }}</router-link></td>
                  <td class="gb-work-step">{{ stepLabel(r.step, r.name) }}</td>
                  <td>{{ waitsFor(r) }}</td>
                  <td class="gb-work-since">{{ ago(r.since) }}</td>
                  <td class="gb-work-action">
                    <template v-if="actionFor(r).kind === 'risk'">
                      <button v-for="k in ['low', 'normal', 'structural']" :key="k" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="setRisk(r, k)">{{ cap(k) }}</button>
                    </template>
                    <button v-else-if="actionFor(r).kind === 'answer'" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" @click="answer(r)">Answer</button>
                    <router-link v-else :to="linkTo(r)" class="epfl-btn epfl-btn-secondary epfl-btn-sm">{{ label(r) }}</router-link>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-else class="gb-work-empty">
            <p>{{ who === 'team' ? 'Nothing waits for anyone.' : 'Nothing waits for you.' }}</p>
            <p class="gb-hint">Every open issue is at a step someone else owns, or nothing is open.</p>
          </div>
        </section>

        <section v-if="data.agents.length" class="gb-work-group" aria-labelledby="agents-h">
          <h2 id="agents-h" class="h5">Agents <span class="gb-work-n">{{ data.agents.length }} {{ data.agents.length === 1 ? 'worktree' : 'worktrees' }}</span></h2>
          <ul class="gb-work-agents">
            <li v-for="a in data.agents" :key="a.path">
              <code class="gb-work-branch" :title="a.path">{{ a.branch }}</code>
              <span v-if="a.issue" class="gb-work-step">{{ stepLabel(a.step ?? 0, a.name ?? '') }}</span>
              <span>{{ a.state }}<template v-if="a.issue"> · <router-link :to="`/issues/${a.issue.id}`">{{ a.issue.shown_title }}</router-link></template></span>
              <span class="gb-work-since">Last commit {{ ago(a.at) }}</span>
            </li>
          </ul>
        </section>

        <section class="gb-work-group" aria-labelledby="waiting-h">
          <h2 id="waiting-h" class="h5">Waiting on others <span class="gb-work-n">{{ data.waiting.length }}</span></h2>
          <div v-if="data.waiting.length" class="gb-table-wrap">
            <table class="gb-table gb-work-table">
              <thead><tr><th>Issue</th><th>Step</th><th>Waits for</th><th>Since</th></tr></thead>
              <tbody>
                <tr v-for="r in data.waiting" :key="r.issue.id">
                  <td><router-link :to="`/issues/${r.issue.id}`"><span v-if="numberOf(r.issue)" class="gb-work-num">#{{ numberOf(r.issue) }}</span> {{ r.issue.shown_title }}</router-link></td>
                  <td class="gb-work-step">{{ stepLabel(r.step, r.name) }}</td>
                  <td>{{ waitsFor(r) }}</td>
                  <td class="gb-work-since">{{ ago(r.since) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="gb-hint">Nothing waits on anyone else.</p>
        </section>
      </template>
    </div>

    <aside v-if="data" class="gb-work-aside">
      <section aria-labelledby="send-h">
        <h2 id="send-h" class="h5">To send <span class="gb-work-n">{{ sendWords(data.pending) }}</span></h2>
        <ul v-if="data.to_send.length" class="gb-work-lines">
          <li v-for="(l, i) in data.to_send" :key="i"><span class="gb-work-num">#{{ numberOf(l.issue) || l.issue.id.slice(0, 7) }}</span> {{ l.what }}</li>
          <li v-if="data.pending > data.to_send.length" class="gb-hint">and {{ data.pending - data.to_send.length }} more</li>
        </ul>
        <p v-else class="gb-hint">The forges have everything.</p>
        <div v-if="data.pending" class="gb-work-send">
          <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="send(false)">Send all</button>
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="send(true)">Check first</button>
        </div>
        <p v-if="sent" class="gb-hint">{{ sent }}</p>
        <p class="gb-hint"><router-link to="/sync">Sync</router-link> has the git remotes.</p>
      </section>
      <section aria-labelledby="retro-h">
        <h2 id="retro-h" class="h5">Since the last retro</h2>
        <p>{{ retroWords(data.retro) }}</p>
      </section>
    </aside>
  </main>

  <q-dialog
    v-model="detailOpen"
    :position="panelMode === 'side' ? 'right' : 'standard'"
    :full-height="panelMode === 'side'"
    :class="`gb-dialog-${panelMode}`"
    :backdrop-filter="panelMode === 'wide' ? 'blur(6px)' : undefined"
    no-route-dismiss
    no-focus
    @hide="setIssue(undefined)"
  >
    <IssuePanel
      v-if="detailId"
      :id="detailId"
      :key="detailId"
      :names="names"
      :fields="fields"
      :title="detailRow?.issue.shown_title ?? ''"
      crumb="Work"
      :at="detailRow?.question?.id"
      @close="detailOpen = false"
      @open="(id: string) => setIssue(id)"
    />
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import AppHeader from '@/components/AppHeader.vue';
import IssuePanel from '@/components/IssuePanel.vue';
import { ApiError, getFields, getViews, getWork, patchIssue, startForgeSync, type Issue, type WorkResponse, type WorkRow } from '@/api/board';
import { namespaces } from '@/facets';
import type { Fields } from '@/fields';
import { forgeNumbers } from '@/issuesync';
import { debounced, onLive } from '@/live';
import { me } from '@/me';
import { panelMode } from '@/panelsize';
import { actionFor, ago, retroWords, sendWords, stepLabel, waitsFor } from '@/work';

const route = useRoute();
const router = useRouter();

const viewNames = ref<string[]>([]);
const fields = ref<Fields>({});
const data = ref<WorkResponse | null>(null);
const error = ref<ApiError | null>(null);
const busy = ref(false);
const sent = ref('');
const who = computed<'me' | 'team'>(() => (route.query.who === 'team' ? 'team' : 'me'));
const rows = computed(() => (data.value ? [...data.value.needs, ...data.value.waiting] : []));
const names = computed(() => namespaces(rows.value.map((r) => r.issue), fields.value));

const numberOf = (i: Issue) => forgeNumbers(i)[0] ?? '';
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const label = (r: WorkRow) => {
  const a = actionFor(r);
  return a.kind === 'open' ? a.label : '';
};
const linkTo = (r: WorkRow) => {
  const a = actionFor(r);
  return a.kind === 'open' && a.cycle ? { path: `/issues/${r.issue.id}`, query: { view: 'cycle' } } : `/issues/${r.issue.id}`;
};

async function reload() {
  try {
    data.value = await getWork(who.value);
    error.value = null;
  } catch (e) {
    if (e instanceof ApiError) error.value = e;
    else error.value = new ApiError(e instanceof Error ? e.message : String(e), 0, '');
  }
}

function setWho(w: 'me' | 'team') {
  void router.replace({ query: { ...route.query, who: w === 'me' ? undefined : w } });
}
watch(who, () => void reload());

async function setRisk(r: WorkRow, risk: string) {
  busy.value = true;
  try {
    await patchIssue(r.issue.id, { risk });
    await reload();
  } catch (e) {
    error.value = e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e), 0, '');
  } finally {
    busy.value = false;
  }
}

// send: everything the forges wait for; dry: a check, nothing moves
async function send(dry: boolean) {
  busy.value = true;
  sent.value = '';
  try {
    await startForgeSync(dry, dry ? 'sync' : 'send');
    sent.value = dry ? 'Checking: Sync shows what would move.' : 'Sending: Sync shows the progress.';
  } catch (e) {
    sent.value = e instanceof Error ? e.message : String(e);
  } finally {
    busy.value = false;
  }
}

// Answer: the panel, scrolled to the question
const detailOpen = ref(false);
const detailId = ref('');
const detailRow = computed(() => rows.value.find((r) => r.issue.id === detailId.value));
function answer(r: WorkRow) {
  setIssue(r.issue.id);
}
function setIssue(id: string | undefined) {
  void router.replace({ query: { ...route.query, issue: id } });
}
watch(
  () => route.query.issue,
  (id) => {
    if (typeof id === 'string' && id) {
      detailId.value = id;
      detailOpen.value = true;
    } else detailOpen.value = false;
  },
  { immediate: true },
);

const reloadSoon = debounced(() => void reload());
watch(
  () => me.value?.me?.id,
  (id, was) => {
    if (was && id !== was) void reload();
  },
);
let stopLive = () => {};
onMounted(async () => {
  stopLive = onLive((e) => {
    if (e.kind === 'issues' || e.kind === 'sync') reloadSoon();
  });
  try {
    viewNames.value = Object.keys(await getViews()).sort();
    fields.value = await getFields();
  } catch {
    // no config: fields shown as labels
  }
  await reload();
});
onBeforeUnmount(() => stopLive());
</script>

<style scoped>
.gb-work { display: flex; flex-wrap: wrap; gap: var(--space-5) var(--space-6); align-items: flex-start; }
.gb-work-main { flex: 999 1 45rem; min-width: 0; }
.gb-work-aside { flex: 1 1 17rem; display: flex; flex-direction: column; gap: var(--space-5); padding: var(--space-4); border: var(--border-w) solid var(--border); }
.visually-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.gb-work-who { margin: var(--space-3) 0 var(--space-4); }
.gb-work-need { margin: var(--space-4) 0; }
.gb-work-group { margin-top: var(--space-5); }
.gb-work-group h2, .gb-work-aside h2 { display: flex; flex-wrap: wrap; align-items: baseline; gap: 0 .5rem; margin-bottom: var(--space-3); }
.gb-work-n { font-size: 1rem; font-weight: normal; color: var(--fg-muted); }
.gb-work-num { font-family: var(--font-mono); font-weight: normal; color: var(--fg-muted); }
.gb-work-step { font-family: var(--font-mono); white-space: nowrap; }
.gb-work-since { white-space: nowrap; font-size: .875rem; color: var(--fg-muted); }
.gb-work-action { white-space: nowrap; text-align: right; }
.gb-work-action > * + * { margin-left: var(--space-1); }
.gb-work-table td a { font-weight: var(--w-bold); }
.gb-work-table td .epfl-btn { font-weight: normal; }
.gb-work-empty { padding: var(--space-4) 0; }
.gb-work-empty p { margin: 0; }
.gb-work-agents, .gb-work-lines { list-style: none; margin: 0; padding: 0; }
.gb-work-agents li { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--space-1) var(--space-3); padding: var(--space-2) 0; border-bottom: var(--border-w) solid var(--border-subtle); }
.gb-work-branch { font-size: .875rem; }
.gb-work-lines li { padding: .125rem 0; font-size: .9375rem; }
.gb-work-send { display: flex; gap: var(--space-2); margin: var(--space-3) 0; }
.gb-work-aside p { margin: 0 0 var(--space-2); }
</style>
