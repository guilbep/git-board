<!-- The Work page (experimental, the cycle's; GET /api/cycle): what needs
     a person, from every open issue's step. Needs you is the rows whose
     gate waits for the viewer (or, in Team, for anyone), or that carry a
     question to them; Waiting on others is the viewer's other issues
     (Team: every other open one). v1's agents went to their own binary
     (ADR 0021). -->
<template>
  <AppHeader :views="viewNames" />
  <main class="gb-page gb-work">
    <div class="gb-work-main">
      <div class="epfl-eyebrow">Work</div>
      <h1 class="h3">What needs a person</h1>
      <p class="gb-lead">Human time is the bottleneck. What waits for a person shows here.</p>

      <div class="gb-seg gb-work-who" role="group" aria-label="Whose work">
        <button type="button" :aria-pressed="who === 'me'" @click="setWho('me')">Mine</button>
        <button type="button" :aria-pressed="who === 'team'" @click="setWho('team')">Team</button>
      </div>

      <p v-if="error?.code === 'experimental'" class="gb-work-need">
        The Work page is the development cycle’s, an experimental feature that is off on this board.
        <router-link to="/settings/experimental">Turn it on in Settings › Experimental</router-link>
      </p>
      <p v-else-if="error?.code === 'no-identity'" class="gb-work-need">
        The page shows what waits for the person you write as, and none is chosen here yet, or they have neither a name nor a login.
        <router-link to="/settings/you">Choose who you write as</router-link>
      </p>
      <p v-else-if="error" class="gb-error">{{ error.message }}</p>

      <template v-if="data">
        <section class="gb-work-group" aria-labelledby="needs-h">
          <h2 id="needs-h" class="h5">Needs you <span class="gb-work-n">{{ data.needs.length }}</span></h2>
          <div v-if="data.needs.length" class="gb-table-wrap">
            <!-- grouped by what they wait for: the step and its gate, five a group until asked -->
            <table class="gb-table gb-work-table">
              <thead><tr><th>Issue</th><th>Since</th><th><span class="visually-hidden">Action</span></th></tr></thead>
              <tbody v-for="g in needGroups" :key="g.key">
                <tr class="gb-work-grouprow">
                  <th colspan="3" scope="colgroup"><span class="gb-work-step">{{ g.step }}</span> · {{ g.waits }} <span class="gb-work-n">{{ g.rows.length }}</span></th>
                </tr>
                <tr v-for="r in shownRows(g)" :key="r.issue.id">
                  <td>
                    <router-link :to="`/issues/${r.issue.id}`"><span v-if="numberOf(r.issue)" class="gb-work-num">{{ numberOf(r.issue) }}</span> {{ r.issue.shown_title }}</router-link>
                  </td>
                  <td class="gb-work-since">{{ ago(r.since) }}</td>
                  <td class="gb-work-action">
                    <template v-if="actionFor(r).kind === 'risk'">
                      <button v-for="k in ['low', 'normal', 'structural']" :key="k" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="setRisk(r, k)">{{ cap(k) }}</button>
                    </template>
                    <button v-else-if="actionFor(r).kind === 'answer'" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" @click="answer(r)">Answer</button>
                    <router-link v-else-if="label(r) !== 'Open'" :to="`/issues/${r.issue.id}`" class="epfl-btn epfl-btn-secondary epfl-btn-sm">{{ label(r) }}</router-link>
                  </td>
                </tr>
                <tr v-if="g.rows.length > shownRows(g).length">
                  <td colspan="3"><button type="button" class="gb-linkbtn" @click="opened.push(g.key)">Show {{ g.rows.length - shownRows(g).length }} more</button></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-else class="gb-work-empty">
            <p>{{ who === 'team' ? 'Nothing waits for anyone.' : 'Nothing waits for you.' }}</p>
            <p class="gb-hint">Every open issue is at a step someone else owns, or nothing is open.</p>
          </div>
        </section>

        <section class="gb-work-group" aria-labelledby="waiting-h">
          <h2 id="waiting-h" class="h5">Waiting on others <span class="gb-work-n">{{ data.waiting.length }}</span></h2>
          <div v-if="data.waiting.length" class="gb-table-wrap">
            <table class="gb-table gb-work-table">
              <thead><tr><th>Issue</th><th>Step</th><th>Waits for</th><th>Since</th></tr></thead>
              <tbody>
                <tr v-for="r in data.waiting" :key="r.issue.id">
                  <td>
                    <router-link :to="`/issues/${r.issue.id}`"><span v-if="numberOf(r.issue)" class="gb-work-num">{{ numberOf(r.issue) }}</span> {{ r.issue.shown_title }}</router-link>
                  </td>
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
  </main>

  <GbDialog
    v-model="detailOpen"
    :position="panelMode === 'side' ? 'right' : 'standard'"
    :full-height="panelMode === 'side'"
    :class="`gb-dialog-${panelMode}`"
    :blur="panelMode === 'wide'"
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
  </GbDialog>
</template>

<script setup lang="ts">
import GbDialog from '@/components/GbDialog.vue';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import AppHeader from '@/components/AppHeader.vue';
import IssuePanel from '@/components/IssuePanel.vue';
import { ApiError, cycleAct, getFields, getWork, type Issue } from '@/api';
import type { WorkGroups, WorkRow } from '@/api/cycletypes';
import { configModel, loadConfigModel } from '@/configmodel';
import { namespaces } from '@/facets';
import type { Fields } from '@/fields';
import { forgeNumbers } from '@/idlabel';
import { debounced, onLive } from '@/live';
import { latestOnly } from '@/latest';
import { me } from '@/me';
import { panelMode } from '@/panelsize';
import { actionFor, ago, stepLabel, waitsFor } from '@/work';

const route = useRoute();
const router = useRouter();

const viewNames = computed(() => configModel.value?.view_order ?? []);
const fields = ref<Fields>({});
const data = ref<WorkGroups | null>(null);
const error = ref<ApiError | null>(null);
const busy = ref(false);
const who = computed<'me' | 'team'>(() => (route.query.who === 'team' ? 'team' : 'me'));
const rows = computed(() => (data.value ? [...data.value.needs, ...data.value.waiting] : []));
const names = computed(() => namespaces(rows.value.map((r) => r.issue), fields.value));

// the project's number (GB-22) names an issue; a forge's only without one
const numberOf = (i: Issue) => i.number || (forgeNumbers(i)[0] ? `#${forgeNumbers(i)[0]}` : '');
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const label = (r: WorkRow) => {
  const a = actionFor(r);
  return a.kind === 'open' ? a.label : '';
};
const asError = (e: unknown) => (e instanceof ApiError ? e : new ApiError(e instanceof Error ? e.message : String(e), 0, 'internal'));

// Me then Team quickly: the one asked last shows, whichever answers last
const reloads = latestOnly();
async function reload() {
  const current = reloads.start();
  try {
    const got = await getWork(who.value);
    if (!current()) return;
    data.value = got;
    error.value = null;
  } catch (e) {
    if (!current()) return;
    error.value = asError(e);
  }
}

function setWho(w: 'me' | 'team') {
  void router.replace({ query: { ...route.query, who: w === 'me' ? undefined : w } });
}
watch(who, () => void reload());

// the risk class: the cycle's action, by the risk role when one holds it
async function setRisk(r: WorkRow, risk: string) {
  busy.value = true;
  try {
    await cycleAct(r.issue.id, 'set-risk', undefined, { risk }); // one field: the last writer wins it
    await reload();
  } catch (e) {
    error.value = asError(e);
  } finally {
    busy.value = false;
  }
}

// Needs you, grouped by step and what it waits for, in the server's order;
// a group shows five rows until it is opened
const needGroups = computed(() => {
  const out: { key: string; step: string; waits: string; rows: WorkRow[] }[] = [];
  for (const r of data.value?.needs ?? []) {
    const key = `${r.step}|${waitsFor(r)}`;
    let g = out.find((x) => x.key === key);
    if (!g) out.push((g = { key, step: stepLabel(r.step, r.name), waits: waitsFor(r), rows: [] }));
    g.rows.push(r);
  }
  return out;
});
const opened = ref<string[]>([]);
const shownRows = (g: { key: string; rows: WorkRow[] }) => (opened.value.includes(g.key) ? g.rows : g.rows.slice(0, 5));

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
  stopLive = onLive((e) => (e.kind === 'issue' || e.kind === 'config' || e.kind === 'me' || e.kind === 'resync') && reloadSoon());
  void loadConfigModel();
  try {
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
.visually-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.gb-work-who { margin: var(--space-3) 0 var(--space-4); }
.gb-work-need { margin: var(--space-4) 0; }
.gb-work-group { margin-top: var(--space-5); }
.gb-work-group h2 { display: flex; flex-wrap: wrap; align-items: baseline; gap: 0 .5rem; margin-bottom: var(--space-3); }
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
.gb-work-table td { vertical-align: top; }
.gb-work-grouprow th { padding-top: .75rem; background: var(--bg-subtle); font-weight: 700; text-align: left; }
</style>
