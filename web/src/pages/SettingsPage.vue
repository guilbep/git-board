<!-- Settings, in tabs with their address (#/settings/<tab>, settingstabs.ts):
     the Overview (what git-board is, and where each part stands), the Sync
     (where your issues go: the git remote, the forges, the GitHub Project,
     one Check and one Sync), then this board: the project file
     (.git-board.toml), the project's own numbers, attachments, and You (who
     you write as, the appearance). #/settings/sync?add=1 opens "Add a
     forge". -->
<template>
  <AppHeader :views="viewNames" active="" />
  <div class="gb-page set">
    <nav class="set-nav" aria-label="Settings">
      <h1 class="h3 set-title">Settings</h1>
      <router-link to="/settings/overview" class="set-tab" :aria-current="tab === 'overview' ? 'page' : undefined">Overview</router-link>
      <div class="set-group">Where your issues go</div>
      <router-link to="/settings/sync" class="set-tab" :aria-current="tab === 'sync' ? 'page' : undefined">Sync <span class="set-via">git, forges, Project</span></router-link>
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
            The issues themselves are kept in the repository, under <code>refs/board</code>: they travel with git like your code, work offline, and need no database.
          </p>
          <p class="set-lead">GitHub, GitLab and the GitHub Project are mirrors, for the people who work there: one Sync moves the issues everywhere. Each part below says where it stands.</p>
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
        <p v-if="settings?.cycle.guessed && settings.cycle.closing" class="gb-hint">
          The cycle's stages are guessed from the status names: ready <code>{{ settings.cycle.ready || '—' }}</code>, in progress <code>{{ settings.cycle.in_progress.join(', ') || '—' }}</code>,
          in review <code>{{ settings.cycle.in_review.join(', ') || '—' }}</code>, validation <code>{{ settings.cycle.validation.join(', ') || '—' }}</code>, closing <code>{{ settings.cycle.closing || '—' }}</code>.
          Write <code>[cycle]</code> below to fix one the guess got wrong.
        </p>
        <ConfigEditor @saved="reload" />
      </div>

      <SyncSettings v-if="tab === 'sync'" @changed="whole = $event" />

      <LabelsSettings v-if="tab === 'labels'" @changed="labels = $event" />

      <NumbersSettings v-if="tab === 'numbers'" @changed="numbers = $event" />
      <WorktreesSettings v-if="tab === 'worktrees'" @changed="worktrees = $event" />

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
    </main>
  </div>
</template>

<script setup lang="ts">
import { themePref } from '@/theme';
import { debounced, onLive } from '@/live';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { getNumbers, getSettings, getViews, getWhole, getWorktrees, type Labels, type Numbers, type Settings, type WholeStatus, type Worktrees } from '@/api/board';
import { message } from '@/issue';
import { loadMe, me } from '@/me';
import { settingsTab, settingsTabs, syncSummary } from '@/settingstabs';
import { numbersSummary } from '@/numbers';
import AppHeader from '@/components/AppHeader.vue';
import IdentityChooser from '@/components/IdentityChooser.vue';
import ConfigEditor from '@/components/ConfigEditor.vue';
import LabelsSettings from '@/components/LabelsSettings.vue';
import SyncSettings from '@/components/SyncSettings.vue';
import NumbersSettings from '@/components/NumbersSettings.vue';
import WorktreesSettings from '@/components/WorktreesSettings.vue';

const route = useRoute();
const tab = computed(() => settingsTab(route.params.tab));
const boardTabs = settingsTabs.filter((t) => ['project-file', 'labels', 'numbers', 'attachments', 'worktrees', 'you'].includes(t.id));

const settings = ref<Settings | null>(null);
const worktrees = ref<Worktrees | null>(null);
const whole = ref<WholeStatus | null>(null);
const numbers = ref<Numbers | null>(null);
const labels = ref<Labels | null>(null);

// the Overview: a card per tab, what it's for and where it stands
const cards = computed(() => {
  const s = settings.value;
  const state: Record<string, string> = {
    sync: whole.value ? syncSummary(whole.value) : '…',
    'project-file': `${viewNames.value.length} saved view${viewNames.value.length === 1 ? '' : 's'}.`,
    labels: labels.value ? `${labels.value.rows.length} label${labels.value.rows.length === 1 ? '' : 's'}${labels.value.repos.length ? `, on ${labels.value.repos.join(', ')}` : ''}.` : 'Open the tab to read GitHub.',
    numbers: numbers.value ? numbersSummary(numbers.value) : '…',
    attachments: !s ? '…' : s.uploads.enabled ? `On, served at ${s.uploads.public_url}.` : 'Off.',
    worktrees: !worktrees.value ? '…' : worktrees.value.config ? 'wtx.toml written.' : worktrees.value.code === 'no-config' ? 'No wtx.toml yet.' : (worktrees.value.reason ?? ''),
    you: me.value?.me ? `You write as ${me.value.me.name}.` : 'Not chosen yet: pick who you write as.',
  };
  const what: Record<string, string> = {
    sync: 'Where your issues go: git, the forges, the GitHub Project. One Check, one Sync.',
    'project-file': 'Fields, views, sprints, colors, branch names: .git-board.toml.',
    labels: "The project's labels, their colors and descriptions, on GitHub; lint.",
    numbers: 'One number per issue, the same on every forge: CO2-3075.',
    attachments: 'Files attached to comments, on S3 behind a CDN.',
    worktrees: 'One agent, one worktree, one branch, one PR: wtx, started from here.',
    you: 'Who you write as, and the appearance.',
  };
  return [{ id: 'sync', label: 'Sync' }, ...boardTabs].map((t) => ({ id: t.id, title: t.label, what: what[t.id] ?? '', state: state[t.id] ?? '' }));
});
const themeOptions = [
  { value: 'system', label: 'System', hint: 'as the computer is set' },
  { value: 'light', label: 'Light', hint: 'always' },
  { value: 'dark', label: 'Dark', hint: 'always' },
] as const;
const viewNames = ref<string[]>([]);
const error = ref('');
const choosing = ref(false);

// back to the sentence once an identity is picked
watch(() => me.value?.me?.id, () => (choosing.value = false));

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
  getWhole().then((w) => (whole.value = w), () => {});
  loadNumbers();
  getWorktrees().then((w) => (worktrees.value = w), () => {});
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
.set-via { margin-left: auto; font-size: .8125rem; font-weight: 400; color: var(--fg-muted); }
.set-main { flex: 1; min-width: 0; }
.set-panel { display: flex; flex-direction: column; gap: 1.25rem; max-width: 64rem; }
.set-panel-wide { max-width: none; }
.set-panel h2, .set-panel h3, .set-panel p { margin: 0; }
.set-panel section { display: grid; gap: .5rem; }
.set-lead { max-width: 40rem; color: var(--fg-muted); }
.set-about { display: grid; gap: .625rem; max-width: 46rem; }
.set-about p { font-size: 1.0625rem; }
.set-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(17rem, 100%), 1fr)); gap: .75rem; margin: 0; padding: 0; list-style: none; }
.set-cards li { border: 1px solid var(--border); border-top: 4px solid var(--card-top, var(--border)); }
.set-card-sync { --card-top: var(--primary); }
.set-cards a { display: grid; gap: .25rem; height: 100%; box-sizing: border-box; padding: .875rem 1rem; color: var(--fg); text-decoration: none; }
.set-cards a:hover { background: var(--bg-subtle); }
.set-card-title { font-weight: 700; }
.set-card-what { font-size: .875rem; color: var(--fg-muted); }
.set-card-state { margin-top: .25rem; font-size: .9375rem; overflow-wrap: anywhere; }
.gb-linkbtn { justify-self: start; padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
.set-login { color: var(--fg-muted); }
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
