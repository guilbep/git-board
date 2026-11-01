<!-- Settings, in tabs with their address (#/settings/<tab>, settingstabs.ts):
     each with where it stands under its name; the Sync first
     (where your issues go: the git remote, the forges, the GitHub Project,
     one Check and one Sync), then this board: the project file
     (.git-board.toml), the project's own numbers, attachments, and You (who
     you write as, the appearance). #/settings/sync?add=1 opens "Add a
     forge". -->
<template>
  <AppHeader :views="viewNames" />
  <div class="gb-page set">
    <!-- the menu: a sidebar from 60rem up, a dropdown under it -->
    <nav class="set-nav" aria-label="Settings">
      <h1 class="h3 set-title">Settings</h1>
      <template v-for="(g, gi) in groups" :key="gi">
        <div v-if="g.group" class="set-group">{{ g.group }}</div>
        <router-link
          v-for="t in g.tabs"
          :key="t.id"
          :to="`/settings/${t.id}`"
          class="set-tab"
          :aria-current="tab === t.id ? 'page' : undefined"
        >
          {{ t.label }}<span v-if="'via' in t && t.via" class="set-via">{{ t.via }}</span>
          <span v-if="menuState[t.id]" class="set-state" :title="menuState[t.id]">{{ menuState[t.id] }}</span>
          <span v-if="dirtySections[t.id]" class="set-dot" title="Unsaved changes" />
        </router-link>
      </template>
    </nav>
    <div class="set-menudrop">
      <label class="set-menulabel" for="set-menu">Settings</label>
      <select id="set-menu" class="set-menuselect" :value="tab" @change="goTab(($event.target as HTMLSelectElement).value)">
        <option v-for="t in tabs" :key="t.id" :value="t.id">{{ t.label }}{{ dirtySections[t.id] ? ' — unsaved changes' : '' }}</option>
      </select>
    </div>

    <main class="set-main">
      <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>

      <div v-if="tab === 'you'" class="set-wrap">
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

        <section v-if="me?.view" aria-labelledby="set-view">
          <h3 id="set-view" class="h5">Your issue page</h3>
          <p>{{ viewWords(me) }}</p>
          <div class="gb-opts" role="radiogroup" aria-labelledby="set-view">
            <label v-for="o in viewOptions" :key="o.value" class="gb-opt" :class="{ on: viewChoice === o.value }">
              <input type="radio" name="view" :value="o.value" :checked="viewChoice === o.value" :disabled="viewSaving" @change="pickView(o.value)" />
              <span><b>{{ o.label }}</b><br /><span class="gb-hint">{{ o.hint }}</span></span>
            </label>
          </div>
          <p v-if="viewError" class="gb-save-error" role="alert"><b>Not saved.</b> {{ viewError }}</p>
          <p class="gb-hint">
            This sets <code>board.view</code> in this repository’s git config, for you alone. For the whole team: a <code>[cycle]</code> table in <code>.git-board.toml</code>.
            From a terminal, <code>git config --global board.view classic</code> or <code>cycle</code> sets it for every repository; a change made there shows when you reload the page.
          </p>
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

      <!-- the project file, a tab per section: each form drafts its own
           section and saves it alone, the file's other lines untouched.
           v-show: switching tabs keeps an unsaved draft. -->
      <div v-show="tab === 'fields'" class="set-wrap set-wrap-wide"><FieldsSettings /></div>
      <div v-show="tab === 'forge-fields'" class="set-wrap set-wrap-wide"><ForgeFieldsSettings /></div>
      <div v-show="tab === 'labels'" class="set-wrap set-wrap-wide"><LabelsSettings @changed="labels = $event" /></div>
      <div v-show="tab === 'views'" class="set-wrap set-wrap-wide"><ViewsSettings /></div>
      <div v-show="tab === 'sprints'" class="set-wrap set-wrap-wide"><SprintsSettings /></div>
      <div v-show="tab === 'roles'" class="set-wrap set-wrap-wide"><RolesSettings /></div>
      <div v-show="tab === 'branches'" class="set-wrap set-wrap-wide"><BranchesSettings /></div>
      <div v-show="tab === 'cycle'" class="set-wrap set-wrap-wide"><CycleSettings /></div>
      <div v-show="tab === 'release'" class="set-wrap set-wrap-wide"><ReleaseSettings /></div>
      <div v-show="tab === 'repos'" class="set-wrap set-wrap-wide"><ReposSettings /></div>
      <div v-show="tab === 'attachments'" class="set-wrap set-wrap-wide"><AttachmentsSettings /></div>
      <!-- Advanced: the file itself, for what the forms don't cover -->
      <div v-show="tab === 'advanced'" class="set-wrap set-wrap-wide">
        <p v-if="cycleView && settings?.cycle.guessed && settings.cycle.closing" class="gb-hint">
          The cycle's stages are guessed from the status names: ready <code>{{ settings.cycle.ready || '—' }}</code>, in progress <code>{{ settings.cycle.in_progress.join(', ') || '—' }}</code>,
          in review <code>{{ settings.cycle.in_review.join(', ') || '—' }}</code>, validation <code>{{ settings.cycle.validation.join(', ') || '—' }}</code>, closing <code>{{ settings.cycle.closing || '—' }}</code>.
          Set them in <router-link to="/settings/cycle">Cycle</router-link>, or edit the file below.
        </p>
        <ConfigEditor @saved="reload" />
      </div>

      <SyncSettings v-if="tab === 'sync'" @changed="whole = $event" />

      <NumbersSettings v-if="tab === 'numbers'" @changed="numbers = $event" />
      <WorktreesSettings v-if="tab === 'worktrees'" @changed="worktrees = $event" />
    </main>
  </div>
</template>

<script setup lang="ts">
import { themePref } from '@/theme';
import { debounced, onLive } from '@/live';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import { getNumbers, getSettings, getViews, getWhole, getWorktrees, setView, type Labels, type Numbers, type Settings, type WholeStatus, type Worktrees } from '@/api/board';
import { message } from '@/issue';
import { classicPeek, cycleView, loadMe, me } from '@/me';
import { viewWords } from '@/view';
import { dirtySections, loadConfigModel } from '@/configmodel';
import { settingsGroups, settingsTab, settingsTabs, syncSummary } from '@/settingstabs';
import { numbersSummary } from '@/numbers';
import AppHeader from '@/components/AppHeader.vue';
import IdentityChooser from '@/components/IdentityChooser.vue';
import AttachmentsSettings from '@/components/AttachmentsSettings.vue';
import BranchesSettings from '@/components/BranchesSettings.vue';
import ConfigEditor from '@/components/ConfigEditor.vue';
import CycleSettings from '@/components/CycleSettings.vue';
import FieldsSettings from '@/components/FieldsSettings.vue';
import ForgeFieldsSettings from '@/components/ForgeFieldsSettings.vue';
import LabelsSettings from '@/components/LabelsSettings.vue';
import ReleaseSettings from '@/components/ReleaseSettings.vue';
import ReposSettings from '@/components/ReposSettings.vue';
import SyncSettings from '@/components/SyncSettings.vue';
import NumbersSettings from '@/components/NumbersSettings.vue';
import RolesSettings from '@/components/RolesSettings.vue';
import SprintsSettings from '@/components/SprintsSettings.vue';
import ViewsSettings from '@/components/ViewsSettings.vue';
import WorktreesSettings from '@/components/WorktreesSettings.vue';

const route = useRoute();
const router = useRouter();
const tab = computed(() => settingsTab(route.params.tab));
const groups = settingsGroups();
const tabs = settingsTabs;

// the dropdown's move: a plain push — the leave guard asks on dirty tabs
function goTab(id: string) {
  if (id !== tab.value) void router.push({ path: `/settings/${id}` });
}

const settings = ref<Settings | null>(null);
const worktrees = ref<Worktrees | null>(null);
const whole = ref<WholeStatus | null>(null);
const numbers = ref<Numbers | null>(null);
const labels = ref<Labels | null>(null);

// where each part stands, under its name in the menu (what the Overview's
// cards said, without repeating the menu)
const menuState = computed<Record<string, string>>(() => {
  const s = settings.value;
  return {
    sync: whole.value ? syncSummary(whole.value) : '',
    labels: labels.value ? `${labels.value.rows.length} label${labels.value.rows.length === 1 ? '' : 's'}` : '',
    numbers: numbers.value ? numbersSummary(numbers.value) : '',
    attachments: !s ? '' : s.uploads.enabled ? 'on' : 'off on this server',
    worktrees: !worktrees.value ? '' : worktrees.value.config ? 'wtx.toml written' : worktrees.value.code === 'no-config' ? 'no wtx.toml yet' : (worktrees.value.reason ?? ''),
    you: me.value?.me ? `as ${me.value.me.name}` : 'not chosen yet',
  };
});
const themeOptions = [
  { value: 'system', label: 'System', hint: 'as the computer is set' },
  { value: 'light', label: 'Light', hint: 'always' },
  { value: 'dark', label: 'Dark', hint: 'always' },
] as const;
// the issue page: '' follows the board, unless a global board.view says otherwise
const viewOptions = [
  { value: '', label: 'As the board says', hint: 'the cycle’s page if .git-board.toml has a [cycle] table, else the classic one; a board.view in your global git config comes first' },
  { value: 'classic', label: 'Classic', hint: 'the description, then the comments and changes in order; the fields aside' },
  { value: 'cycle', label: 'The cycle’s', hint: 'each issue’s steps, its review, and its changes' },
] as const;
const viewChoice = computed(() => (me.value?.view_from === 'board.view' && me.value.view_scope === 'local' ? me.value.view : ''));
const viewSaving = ref(false);
const viewError = ref('');
async function pickView(v: '' | 'classic' | 'cycle') {
  viewSaving.value = true;
  viewError.value = '';
  try {
    me.value = await setView(v);
    classicPeek.value = false; // the page picked here, not the issue switch's
  } catch (e) {
    viewError.value = message(e);
  } finally {
    viewSaving.value = false;
  }
}
const viewNames = ref<string[]>([]);
const error = ref('');
const choosing = ref(false);

// back to the sentence once an identity is picked
watch(() => me.value?.me?.id, () => (choosing.value = false));

// after a save of .git-board.toml: the header's views, the Project's line
async function reload() {
  void loadMe(); // the issue page's view follows a [cycle] table saved here
  try {
    settings.value = await getSettings();
  } catch (e) {
    error.value = message(e);
  }
  try {
    viewNames.value = Object.keys(await getViews());
  } catch {
    // the header shows no views
  }
}

onMounted(() => {
  if (!me.value) void loadMe();
  void loadConfigModel();
  void reload();
  getWhole().then((w) => (whole.value = w), () => {});
  loadNumbers();
  getWorktrees().then((w) => (worktrees.value = w), () => {});
});
// a form tab with a draft: say so before the page goes — the browser's
// beforeunload for a reload or a close, the router's guard for a link
const beforeUnload = (e: BeforeUnloadEvent) => {
  if (Object.values(dirtySections.value).some(Boolean)) e.preventDefault();
};
onMounted(() => window.addEventListener('beforeunload', beforeUnload));
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
onBeforeRouteLeave(() => {
  if (!Object.values(dirtySections.value).some(Boolean)) return true;
  return window.confirm('A project-file form has unsaved changes. Leave anyway?');
});
// the menu's numbers line: started, or an issue numbered, from anywhere
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
.set-tab { display: flex; flex-wrap: wrap; align-items: baseline; gap: 0 .625rem; padding: .625rem .875rem; color: var(--fg); text-decoration: none; }
.set-tab:hover { background: var(--bg-subtle); color: var(--fg); }
.set-tab[aria-current='page'] { font-weight: 700; background: var(--bg-subtle); box-shadow: inset 3px 0 0 var(--primary); }
.set-group { margin: 1.25rem 0 .375rem; padding: 0 .875rem; font-size: .75rem; font-weight: 700; letter-spacing: .08em; text-transform: uppercase; color: var(--fg-muted); }
.set-state { flex-basis: 100%; min-width: 0; overflow: hidden; font-size: .75rem; font-weight: 400; color: var(--fg-muted); text-overflow: ellipsis; white-space: nowrap; }
.set-via { margin-left: auto; font-size: .8125rem; font-weight: 400; color: var(--fg-muted); }
.set-dot { flex: none; width: .5rem; height: .5rem; margin-left: auto; border-radius: 50%; background: var(--warning); }
.set-tab:has(.set-via) .set-dot { margin-left: .5rem; }
/* the content column, capped and centered: two 50/50 columns stretch
   absurdly past ~1400px, the forms go limp and the reference's lines run */
.set-main { flex: 1; min-width: 0; max-width: 87.5rem; margin-inline: auto; }
.set-wrap { display: flex; flex-direction: column; gap: 1.25rem; max-width: 64rem; }
.set-wrap-wide { max-width: none; }
.set-wrap h2, .set-wrap h3, .set-wrap p { margin: 0; }
.set-wrap section { display: grid; gap: .5rem; }
.gb-linkbtn { justify-self: start; padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
.set-login { color: var(--fg-muted); }
/* the dropdown, under 60rem (960px): the sidebar's rows don't fit, a
   select above the page does — the options carry the unsaved marks */
.set-menudrop { display: none; }
@media (max-width: 59.99rem) {
  .set { flex-direction: column; align-items: stretch; gap: 1.5rem; padding: 1.25rem 1rem 2rem; }
  .set-nav { display: none; }
  .set-menudrop { display: grid; gap: .25rem; max-width: 24rem; }
  .set-menulabel { font-size: .75rem; font-weight: 700; letter-spacing: .08em; text-transform: uppercase; color: var(--fg-muted); }
  .set-menuselect { min-height: 2.5rem; padding: 0 .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
  .set-main { align-self: center; width: 100%; }
}
</style>
