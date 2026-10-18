<!-- Settings, in tabs with their address (#/settings/<tab>, settingstabs.ts):
     the Overview (what git-board is, and where each part stands), where
     your issues go (remotes, forges: git-bug bridges, add one, sync on or
     off, remove one; the GitHub Project), then this board: the project
     file (.git-board.toml), the project's own numbers, attachments, and
     You (who you write as, the appearance). #/settings/forges?add=1 opens
     "Add a forge". -->
<template>
  <AppHeader :views="viewNames" active="" />
  <div class="gb-page set">
    <nav class="set-nav" aria-label="Settings">
      <h1 class="h3 set-title">Settings</h1>
      <router-link to="/settings/overview" class="set-tab" :aria-current="tab === 'overview' ? 'page' : undefined">Overview</router-link>
      <div class="set-group">Where your issues go</div>
      <router-link v-for="(c, i) in channels" :key="c.id" :to="`/settings/${c.id}`" class="set-tab" :aria-current="tab === c.id ? 'page' : undefined">
        <span class="set-n">{{ i + 1 }}</span> {{ c.label }} <span class="set-via">{{ c.via }}</span>
      </router-link>
      <div class="set-group">This board</div>
      <router-link v-for="t in boardTabs" :key="t.id" :to="`/settings/${t.id}`" class="set-tab" :aria-current="tab === t.id ? 'page' : undefined">{{ t.label }}</router-link>
    </nav>

    <main class="set-main">
      <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>

      <div v-if="tab === 'overview'" class="set-panel">
        <div class="set-about">
          <h2 class="h4">git-board</h2>
          <p>
            A board for this repository's issues, as a GitHub Project is: views, a kanban, sprints, insights.
            The issues themselves are <a href="https://github.com/git-bug/git-bug" target="_blank" rel="noopener noreferrer">git-bug</a> data,
            kept in the repository: they travel with git like your code, work offline, and need no database.
          </p>
          <p class="set-lead">GitHub, GitLab and the GitHub Project are mirrors, for the people who work there. Each part below says where it stands.</p>
        </div>
        <ul class="set-cards">
          <li v-for="c in cards" :key="c.id" :class="`set-card-${c.id}`">
            <router-link :to="`/settings/${c.id}`">
              <span class="set-card-title">{{ c.title }}</span>
              <span class="set-card-what">{{ c.what }}</span>
              <b class="set-card-state">{{ c.state }}</b>
            </router-link>
          </li>
        </ul>
      </div>

      <div v-if="tab === 'you'" class="set-panel">
        <h2 class="h4">You</h2>
        <section aria-labelledby="set-me">
          <h3 id="set-me" class="h5">Who you write as</h3>
          <p v-if="me?.me && !choosing">
            You write as <b>{{ me.me.name }}</b><span v-if="me.me.login && !me.me.name.includes(me.me.login)" class="set-login"> ({{ me.me.login }})</span>.
            <button type="button" class="gb-linkbtn" @click="choosing = true">Change</button>
          </p>
          <template v-else>
            <IdentityChooser />
            <button v-if="me?.me" type="button" class="gb-linkbtn" @click="choosing = false">Keep writing as {{ me.me.name }}</button>
          </template>
        </section>

        <section aria-labelledby="set-look">
          <h3 id="set-look" class="h5">Appearance</h3>
          <div class="gb-opts" role="radiogroup" aria-labelledby="set-look">
            <label v-for="o in themeOptions" :key="o.value" class="gb-opt" :class="{ on: themePref === o.value }">
              <input v-model="themePref" type="radio" name="theme" :value="o.value" />
              <span><b>{{ o.label }}</b><br /><span class="gb-hint">{{ o.hint }}</span></span>
            </label>
          </div>
        </section>
      </div>

      <!-- v-show: switching tabs keeps an unsaved draft -->
      <div v-show="tab === 'project-file'" class="set-panel set-panel-wide">
        <ConfigEditor @saved="reload" />
      </div>

      <NumbersSettings v-if="tab === 'numbers'" @changed="numbers = $event" />

      <div v-if="tab === 'attachments'" class="set-panel">
        <h2 class="h4">Attachments</h2>
        <p v-if="settings?.uploads.enabled">
          Files attached to comments go to <code>{{ settings.uploads.endpoint }}</code>, under <code>{{ settings.uploads.prefix }}/</code>,
          served at <a :href="settings.uploads.public_url" target="_blank" rel="noopener noreferrer">{{ settings.uploads.public_url }}</a>.
        </p>
        <p v-else-if="settings">
          Off. Set <code>ACCESS_KEY_ID</code>, <code>ACCESS_KEY_SECRET</code> and <code>BUCKET_NAME</code> where the git-board server runs,
          and optionally <code>[uploads]</code> in <code>.git-board.toml</code>.
        </p>
      </div>

      <div v-if="tab === 'remotes'" class="set-panel">
        <ChannelMap current="remotes" />
        <h2 class="h4">Remotes</h2>
        <p class="set-lead">
          Your issues travel with git, like your code: <code>git bug push</code> sends them to a remote with their comments and people, <code>git bug pull</code> gets them.
          Push and pull them from <router-link to="/sync#remotes">Sync › Remotes</router-link>.
        </p>
      </div>

      <div v-if="tab === 'forges'" class="set-panel">
        <ChannelMap current="forges" />
        <div class="set-head">
          <div>
            <h2 class="h4">Forges</h2>
            <p class="set-lead">A forge mirrors this repository's issues on GitHub, GitLab or Gitea, for the people who work there. Your issues already travel with git: see <router-link to="/settings/remotes">Remotes</router-link>.</p>
          </div>
          <div class="set-actions">
            <button type="button" class="epfl-btn epfl-btn-secondary" @click="guideOpen = true">How forges work</button>
            <button type="button" class="epfl-btn epfl-btn-primary" @click="adding = true">+ Add a forge</button>
          </div>
        </div>
        <div v-if="settings?.bridges.length" class="gb-table-wrap">
          <table class="gb-table">
            <thead><tr><th>Name</th><th>Forge</th><th>Project</th><th>Sync</th><th><span class="visually-hidden">Remove</span></th></tr></thead>
            <tbody>
              <template v-for="b in settings.bridges" :key="b.name">
              <tr>
                <td><b>{{ b.name }}</b></td>
                <td>{{ forgeName(b.target) }}</td>
                <td><a v-if="b.where.startsWith('https://') && !b.where.includes(' ')" :href="b.where" target="_blank" rel="noopener noreferrer">{{ b.where }}</a><span v-else>{{ b.where || '—' }}</span></td>
                <td>
                  <label v-if="b.can_sync" class="gb-switch">
                    <input type="checkbox" :checked="b.sync" :disabled="busy === b.name" @change="toggle(b, ($event.target as HTMLInputElement).checked)" />
                    <span>{{ b.sync ? 'On' : 'Off' }}</span>
                  </label>
                  <span v-else class="gb-hint">import/export only</span>
                </td>
                <td><button type="button" class="gb-linkbtn" :disabled="busy === b.name" @click="confirming = b.name">Remove</button></td>
              </tr>
              <tr v-if="confirming === b.name" class="gb-confirm-row">
                <td colspan="5">
                  <p role="alert">
                    <b>Remove the forge {{ b.name }}?</b> Its settings go. The issues it brought stay, still linked to {{ forgeName(b.target) }}, and the stored token stays too.
                  </p>
                  <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy === b.name" @click="remove(b)">Remove {{ b.name }}</button>
                  <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="confirming = ''">Cancel</button>
                </td>
              </tr>
              </template>
            </tbody>
          </table>
        </div>
        <p v-else-if="settings" class="gb-hint">No forge yet: <button type="button" class="gb-linkbtn" @click="adding = true">add one</button>.</p>
        <p class="set-lead">
          Running a check or a sync: <router-link to="/sync#forges">Sync › Forges</router-link>.
          The Project's fields have their own tab: <router-link to="/settings/github-project">3 · GitHub Project</router-link>.
        </p>
      </div>

      <div v-if="tab === 'github-project'" class="set-panel">
        <ChannelMap current="github-project" />
        <h2 class="h4">GitHub Project</h2>
        <p class="set-lead">The Project's fields (status, priority, sprint) become labels here, and go back when writing is on.</p>
        <template v-if="settings?.github_project.number">
          <div class="set-tiles">
            <div class="gb-value gb-value-agreed"><span class="gb-value-label">Owner</span><span class="gb-value-val">{{ settings.github_project.owner }}</span></div>
            <div class="gb-value gb-value-agreed"><span class="gb-value-label">Project</span><span class="gb-value-val">#{{ settings.github_project.number }}</span></div>
            <div class="gb-value gb-value-agreed"><span class="gb-value-label">Writing</span><span class="gb-value-val">{{ settings.github_project.write ? 'On · read and written' : 'Off · read only' }}</span></div>
          </div>
          <p>Checking or pulling its fields: <router-link to="/sync#github-project">Sync › GitHub Project</router-link>; its conflicts: <router-link to="/conflicts">Conflicts</router-link>.</p>
        </template>
        <p v-else-if="settings">None. Set <code>[github-project]</code> in the <router-link to="/settings/project-file">project file</router-link>, or run <code>git board bridge pull --owner … --project …</code>.</p>
      </div>
    </main>
  </div>

  <AddForgeDialog v-model="adding" @added="onAdded" @guide="guideOpen = true" />
  <ForgesGuide v-model="guideOpen" />
</template>

<script setup lang="ts">
import { themePref } from '@/theme';
import { debounced, onLive } from '@/live';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { getNumbers, getRemotes, getSettings, getViews, removeBridge, setBridgeSync, type Bridge, type Numbers, type Remote, type Settings } from '@/api/board';
import { loadForges } from '@/forges';
import { message } from '@/issue';
import { loadMe, me } from '@/me';
import { channels, forgeSummary, remoteSummary, settingsTab, settingsTabs } from '@/settingstabs';
import { numbersSummary } from '@/numbers';
import AppHeader from '@/components/AppHeader.vue';
import IdentityChooser from '@/components/IdentityChooser.vue';
import ConfigEditor from '@/components/ConfigEditor.vue';
import ChannelMap from '@/components/ChannelMap.vue';
import AddForgeDialog from '@/components/AddForgeDialog.vue';
import ForgesGuide from '@/components/ForgesGuide.vue';
import NumbersSettings from '@/components/NumbersSettings.vue';

const route = useRoute();
const router = useRouter();
const tab = computed(() => settingsTab(route.params.tab));
// "Add a forge" is open while the address says ?add=1 (the Sync page
// links there); closing it, however, drops it
const adding = computed({
  get: () => tab.value === 'forges' && route.query.add === '1',
  set: (on) => void router.replace({ path: '/settings/forges', query: on ? { add: '1' } : {} }),
});
const guideOpen = ref(false);
const boardTabs = settingsTabs.filter((t) => ['project-file', 'numbers', 'attachments', 'you'].includes(t.id));

const settings = ref<Settings | null>(null);
const remotes = ref<Remote[] | null>(null);
const numbers = ref<Numbers | null>(null);

// the Overview: a card per tab, what it's for and where it stands
const cards = computed(() => {
  const s = settings.value;
  const gp = s?.github_project;
  const state: Record<string, string> = {
    remotes: remotes.value ? remoteSummary(remotes.value) : '…',
    forges: s ? forgeSummary(s.bridges) : '…',
    'github-project': !s ? '…' : gp?.number ? `${gp.owner} #${gp.number}, ${gp.write ? 'read and written' : 'read only'}.` : 'None.',
    'project-file': `${viewNames.value.length} saved view${viewNames.value.length === 1 ? '' : 's'}.`,
    numbers: numbers.value ? numbersSummary(numbers.value) : '…',
    attachments: !s ? '…' : s.uploads.enabled ? `On, served at ${s.uploads.public_url}.` : 'Off.',
    you: me.value?.me ? `You write as ${me.value.me.name}.` : 'Not chosen yet: pick who you write as.',
  };
  const what: Record<string, string> = {
    'project-file': 'Fields, views, sprints, colors, branch names: .git-board.toml.',
    numbers: 'One number per issue, the same on every forge: CO2-3075.',
    attachments: 'Files attached to comments, on S3 behind a CDN.',
    you: 'Who you write as, and the appearance.',
  };
  return [
    ...channels.map((c, i) => ({ id: c.id, title: `${i + 1} · ${c.label} · ${c.via}`, what: `${c.moves} ${c.verbs}`, state: state[c.id] ?? '' })),
    ...boardTabs.map((t) => ({ id: t.id, title: t.label, what: what[t.id] ?? '', state: state[t.id] ?? '' })),
  ];
});
const themeOptions = [
  { value: 'system', label: 'System', hint: 'as the computer is set' },
  { value: 'light', label: 'Light', hint: 'always' },
  { value: 'dark', label: 'Dark', hint: 'always' },
] as const;
const viewNames = ref<string[]>([]);
const error = ref('');
const busy = ref('');
const choosing = ref(false);

// back to the sentence once an identity is picked
watch(() => me.value?.me?.id, () => (choosing.value = false));

function forgeName(target: string) {
  return ({ github: 'GitHub', gitlab: 'GitLab', gitea: 'Gitea' } as Record<string, string>)[target] ?? target;
}

const confirming = ref('');

async function remove(b: Bridge) {
  busy.value = b.name;
  error.value = '';
  try {
    await removeBridge(b.name);
    confirming.value = '';
    await reload();
    void loadForges(); // the header's sync link
  } catch (e) {
    error.value = `${b.name} not removed: ${message(e)}`;
  } finally {
    busy.value = '';
  }
}

async function onAdded() {
  await reload();
  void loadForges();
}

async function toggle(b: Bridge, on: boolean) {
  busy.value = b.name;
  error.value = '';
  try {
    await setBridgeSync(b.name, on);
    b.sync = on;
    void loadForges(); // the header's sync link
  } catch (e) {
    error.value = `Sync of ${b.name} not changed: ${message(e)}`;
    settings.value = await getSettings().catch(() => settings.value);
  } finally {
    busy.value = '';
  }
}

// after a save of .git-board.toml: the header's views, the Project's line
async function reload() {
  try {
    settings.value = await getSettings();
  } catch (e) {
    error.value = message(e);
  }
  try {
    viewNames.value = Object.keys(await getViews()).sort();
  } catch {
    // the header shows no views
  }
}

onMounted(() => {
  if (!me.value) void loadMe();
  void reload();
  getRemotes().then((r) => (remotes.value = r.remotes)).catch(() => (remotes.value = []));
  loadNumbers();
});
// the Overview's numbers line: started, or an issue numbered, from anywhere
const loadNumbers = () => void getNumbers().then((n) => (numbers.value = n), () => {});
const reloadNumbers = debounced(loadNumbers);
const offLive = onLive((e) => {
  if (e.kind === 'settings' || e.kind === 'config') void reload();
  if (e.kind === 'issues' || e.kind === 'sync') reloadNumbers();
});
onBeforeUnmount(offLive);
</script>

<style scoped>
.set { display: flex; align-items: flex-start; gap: 3rem; }
.set-nav { flex: none; width: 15rem; display: flex; flex-direction: column; gap: .125rem; }
.set-title { margin: 0 0 1rem; }
.set-tab { display: flex; align-items: baseline; gap: .625rem; padding: .625rem .875rem; color: var(--fg); text-decoration: none; }
.set-tab:hover { background: var(--bg-subtle); color: var(--fg); }
.set-tab[aria-current='page'] { font-weight: 700; background: var(--bg-subtle); box-shadow: inset 3px 0 0 var(--primary); }
.set-group { margin: 1.25rem 0 .375rem; padding: 0 .875rem; font-size: .75rem; font-weight: 700; letter-spacing: .08em; text-transform: uppercase; color: var(--fg-muted); }
.set-n { color: var(--fg-muted); }
.set-tab[aria-current='page'] .set-n { color: var(--primary-hover); }
.set-via { margin-left: auto; font-size: .8125rem; font-weight: 400; color: var(--fg-muted); }
.set-main { flex: 1; min-width: 0; }
.set-panel { display: flex; flex-direction: column; gap: 1.25rem; max-width: 64rem; }
.set-panel-wide { max-width: none; }
.set-panel h2, .set-panel h3, .set-panel p { margin: 0; }
.set-panel section { display: grid; gap: .5rem; }
.set-head { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 1rem; }
.set-lead { max-width: 40rem; color: var(--fg-muted); }
.set-head h2 { margin-bottom: .375rem; }
.set-actions { display: flex; flex-wrap: wrap; gap: .75rem; }
.set-about { display: grid; gap: .625rem; max-width: 46rem; }
.set-about p { font-size: 1.0625rem; }
.set-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(17rem, 100%), 1fr)); gap: .75rem; margin: 0; padding: 0; list-style: none; }
.set-cards li { border: 1px solid var(--border); border-top: 4px solid var(--card-top, var(--border)); }
.set-card-remotes { --card-top: var(--bg-dark); }
.set-card-forges { --card-top: var(--primary); }
.set-card-github-project { --card-top: var(--gb-purple-dot); }
.set-cards a { display: grid; gap: .25rem; height: 100%; box-sizing: border-box; padding: .875rem 1rem; color: var(--fg); text-decoration: none; }
.set-cards a:hover { background: var(--bg-subtle); }
.set-card-title { font-weight: 700; }
.set-card-what { font-size: .875rem; color: var(--fg-muted); }
.set-card-state { margin-top: .25rem; font-size: .9375rem; overflow-wrap: anywhere; }
.set-tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr)); gap: .75rem; max-width: 45rem; }
.gb-linkbtn { justify-self: start; padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
.set-login { color: var(--fg-muted); }
.gb-confirm-row td { background: var(--warning-bg); }
.gb-confirm-row p { margin: 0 0 .5rem; }
.gb-confirm-row .epfl-btn + .epfl-btn { margin-left: .5rem; }
/* the hidden header stays in the table's scroll, not past the page */
.gb-table-wrap { position: relative; }
.visually-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.gb-switch { display: inline-flex; align-items: center; gap: .5rem; cursor: pointer; font-weight: 700; }
.gb-switch input { width: 1.125rem; height: 1.125rem; accent-color: var(--primary-hover); }
/* phones: the menu, a few rows above the tab */
@media (max-width: 47.99rem) {
  .set { flex-direction: column; align-items: stretch; gap: 1.5rem; padding: 1.25rem 1rem 2rem; }
  .set-nav { width: auto; flex-direction: row; flex-wrap: wrap; }
  .set-title, .set-group { flex-basis: 100%; }
  .set-title { margin-bottom: .25rem; }
  .set-group { margin-top: .5rem; }
  .set-tab { padding: .5rem .75rem; }
  .set-tab[aria-current='page'] { box-shadow: inset 0 -3px 0 var(--primary); }
  .set-via { margin-left: 0; }
}
</style>
