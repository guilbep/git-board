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


        <section aria-labelledby="set-look">
          <h3 id="set-look" class="h5">Appearance</h3>
          <div class="gb-opts" role="radiogroup" aria-labelledby="set-look">
            <label v-for="o in themeOptions" :key="o.value" class="gb-opt" :class="{ on: themePref === o.value }">
              <input v-model="themePref" type="radio" name="theme" :value="o.value" />
              <span><b>{{ o.label }}</b><br /><span class="gb-hint">{{ o.hint }}</span></span>
            </label>
          </div>
          <p id="set-panel" class="set-sub">The issue panel</p>
          <div class="gb-opts" role="radiogroup" aria-labelledby="set-panel">
            <label v-for="o in panelSizes" :key="o.value" class="gb-opt" :class="{ on: panelSize === o.value }">
              <input type="radio" name="panel" :value="o.value" :checked="panelSize === o.value" @change="setPanelSize(o.value)" />
              <span><b>{{ o.label }}</b><br /><span class="gb-hint">{{ o.hint }}</span></span>
            </label>
          </div>
          <p class="gb-hint">Kept in this browser. Its edge also drags to any width, and <kbd>f</kbd> switches to expanded and back.</p>
          <p id="set-fonts" class="set-sub">Fonts</p>
          <div class="gb-opts" role="radiogroup" aria-labelledby="set-fonts">
            <label v-for="o in fontOptions" :key="o.value" class="gb-opt" :class="{ on: fontsPref === o.value }">
              <input v-model="fontsPref" type="radio" name="fonts" :value="o.value" />
              <span><b>{{ o.label }}</b><br /><span class="gb-hint">{{ o.hint }}</span></span>
            </label>
          </div>
        </section>

        <section aria-labelledby="set-tips">
          <h3 id="set-tips" class="h5">Tips</h3>
          <label class="set-check"><input type="checkbox" :checked="!tipsOff" @change="setTipsOff(!($event.target as HTMLInputElement).checked)" /> Show a tip in the header: one keyboard shortcut at a time, for where you are</label>
          <p class="gb-hint">A tip stops once you have used its shortcut three times, or hidden it with its ×. Kept in this browser. <button type="button" class="gb-linkbtn" @click="resetTips()">Show every tip again</button></p>
        </section>

        <section v-if="me" aria-labelledby="set-pictures">
          <h3 id="set-pictures" class="h5">Pictures</h3>
          <div class="gb-opts" role="radiogroup" aria-labelledby="set-pictures">
            <label v-for="o in avatarOptions" :key="o.value" class="gb-opt" :class="{ on: avatarChoice === o.value }">
              <input type="radio" name="avatars" :value="o.value" :checked="avatarChoice === o.value" :disabled="avatarSaving" @change="pickAvatars(o.value)" />
              <span><b>{{ o.label }}</b><br /><span class="gb-hint">{{ o.hint }}</span></span>
            </label>
          </div>
          <p v-if="avatarError" class="gb-save-error" role="alert"><b>Not saved.</b> {{ avatarError }}</p>
          <p class="gb-hint">
            This sets <code>board.avatars</code> in this repository’s git config, for you alone. A picture that can’t load shows the initials.
            <button type="button" class="gb-linkbtn" :disabled="avatarSaving" @click="pickAvatarsEverywhere">{{ avatarSaving ? 'Saving…' : 'Set it for every repository of yours' }}</button>
            writes it at git’s global level, where a repository’s own choice is read over it.
          </p>
        </section>
      </div>

      <!-- the project file, a tab per section: each form drafts its own
           section and saves it alone, the file's other lines untouched.
           v-show: switching tabs keeps an unsaved draft. -->
      <div v-show="tab === 'fields'" class="set-wrap set-wrap-wide"><FieldsSettings /></div>
      <div v-show="tab === 'labels'" class="set-wrap set-wrap-wide"><LabelsSettings /></div>
      <div v-show="tab === 'views'" class="set-wrap set-wrap-wide"><ViewsSettings /></div>
      <div v-show="tab === 'sprints'" class="set-wrap set-wrap-wide"><SprintsSettings /></div>
      <div v-show="tab === 'roles'" class="set-wrap set-wrap-wide"><RolesSettings /></div>
      <div v-show="tab === 'branches'" class="set-wrap set-wrap-wide"><BranchesSettings /></div>
      <div v-if="cycleOn" v-show="tab === 'cycle'" class="set-wrap set-wrap-wide"><CycleSettings /></div>
      <div v-show="tab === 'release'" class="set-wrap set-wrap-wide"><ReleaseSettings /></div>
      <div v-show="tab === 'repos'" class="set-wrap set-wrap-wide"><ReposSettings /></div>
      <div v-show="tab === 'attachments'" class="set-wrap set-wrap-wide"><AttachmentsSettings /></div>
      <div v-show="tab === 'experimental'" class="set-wrap set-wrap-wide"><ExperimentalSettings /></div>
      <!-- Advanced: the file itself, for what the forms don't cover -->
      <div v-show="tab === 'advanced'" class="set-wrap set-wrap-wide">
        <ConfigEditor @saved="reload" />
      </div>

      <SyncSettings v-if="tab === 'sync'" />
      <ProjectSettings v-if="tab === 'project'" />

      <DatastoreSettings v-if="tab === 'datastore'" />
      <NumbersSettings v-if="tab === 'numbers'" />
    </main>
  </div>
</template>

<script setup lang="ts">
import { fontsPref, themePref } from '@/theme';
import { panelMode, panelSizes, panelWidth, setPanelSize, sizeOf } from '@/panelsize';
import { onLive } from '@/live';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import { resetTips, setTipsOff, tipsOff } from '@/tips';
import { message, setMe, type AvatarsMode } from '@/api';
import { board, cycleOn, loadMe, me } from '@/me';
import { featureOn, type Feature } from '@/experimental';
import { configModel, dirtySections, loadConfigModel } from '@/configmodel';
import { settingsGroups, settingsTab, syncSummary, visibleTabs } from '@/settingstabs';
import AppHeader from '@/components/AppHeader.vue';
import IdentityChooser from '@/components/IdentityChooser.vue';
import AttachmentsSettings from '@/components/AttachmentsSettings.vue';
import BranchesSettings from '@/components/BranchesSettings.vue';
import ConfigEditor from '@/components/ConfigEditor.vue';
import CycleSettings from '@/components/CycleSettings.vue';
import ExperimentalSettings from '@/components/ExperimentalSettings.vue';
import FieldsSettings from '@/components/FieldsSettings.vue';
import LabelsSettings from '@/components/LabelsSettings.vue';
import ReleaseSettings from '@/components/ReleaseSettings.vue';
import ReposSettings from '@/components/ReposSettings.vue';
import SyncSettings from '@/components/SyncSettings.vue';
import ProjectSettings from '@/components/ProjectSettings.vue';
import NumbersSettings from '@/components/NumbersSettings.vue';
import DatastoreSettings from '@/components/DatastoreSettings.vue';
import { datastoreSummary } from '@/datastore';
import { datastore, loadDatastore } from '@/kept';
import { loadSync, syncStatus } from '@/sync';
import { loadUploadStatus, uploadStatus } from '@/uploads';
import RolesSettings from '@/components/RolesSettings.vue';
import SprintsSettings from '@/components/SprintsSettings.vue';
import ViewsSettings from '@/components/ViewsSettings.vue';

const route = useRoute();
const router = useRouter();
// an experimental feature's tab shows while it is on (GET /api/board)
const isOn = (f: string) => featureOn(board.value?.experimental, f as Feature);
const tab = computed(() => settingsTab(route.params.tab, isOn));
const groups = computed(() => settingsGroups(isOn));
const tabs = computed(() => visibleTabs(isOn));

// the dropdown's move: a plain push — the leave guard asks on dirty tabs
function goTab(id: string) {
  if (id !== tab.value) void router.push({ path: `/settings/${id}` });
}

// where each part stands, under its name in the menu
const menuState = computed<Record<string, string>>(() => {
  const labels = configModel.value?.labels.list.length;
  return {
    sync: syncStatus.value ? syncSummary(syncStatus.value) : '',
    labels: labels === undefined ? '' : `${labels} label${labels === 1 ? '' : 's'}`,
    numbers: datastore.value ? (datastore.value.counter ? 'handed out here' : 'the forges’ alone') : '',
    datastore: datastore.value ? datastoreSummary(datastore.value) : '',
    attachments: !uploadStatus.value ? '' : uploadStatus.value.enabled ? 'on' : 'off on this server',
    you: me.value?.me ? `as ${me.value.me.name}` : 'not chosen yet',
  };
});
const panelSize = computed(() => sizeOf(panelMode.value, panelWidth.value));
const themeOptions = [
  { value: 'system', label: 'System', hint: 'as the computer is set' },
  { value: 'light', label: 'Light', hint: 'always' },
  { value: 'dark', label: 'Dark', hint: 'always' },
] as const;
const fontOptions = [
  { value: 'system', label: 'System', hint: 'the computer’s: nothing to load' },
  { value: 'epfl', label: 'EPFL', hint: 'Suisse Int’l, on the EPFL network only (elsewhere, Arial)' },
] as const;
// where pictures come from (docs/prd/avatars.md): each says what the browser asks, and whom
const avatarOptions = [
  { value: 'forge', label: 'Forge accounts', hint: 'GitHub pictures, by account: GitHub learns which of its users this page shows' },
  { value: 'gravatar', label: 'Forge accounts, then Gravatar', hint: 'also Gravatar, by a hash of each person’s email: mostly people made here, who have one' },
  { value: 'none', label: 'Initials only', hint: 'no picture: the browser asks nobody' },
] as const;
const avatarChoice = computed(() => me.value?.avatars ?? 'forge');
const avatarSaving = ref(false);
const avatarError = ref('');
async function pickAvatars(v: AvatarsMode) {
  avatarSaving.value = true;
  avatarError.value = '';
  try {
    me.value = await setMe({ avatars: v });
  } catch (e) {
    avatarError.value = message(e);
  } finally {
    avatarSaving.value = false;
  }
}

// the same choice, written at git's global level, so every repository
// of the user reads it (a repository's own board.avatars wins over it)
async function pickAvatarsEverywhere() {
  avatarSaving.value = true;
  avatarError.value = '';
  try {
    me.value = await setMe({ avatars: avatarChoice.value, global: true });
  } catch (e) {
    avatarError.value = message(e);
  } finally {
    avatarSaving.value = false;
  }
}
const viewNames = computed(() => configModel.value?.view_order ?? []);
const error = ref('');
const choosing = ref(false);

// back to the sentence once an identity is picked
watch(() => me.value?.me?.id, () => (choosing.value = false));

// after a save of .git-board.toml: the header's views, the features on
function reload() {
  void loadMe();
  loadConfigModel().catch((e: unknown) => (error.value = message(e)));
}

onMounted(() => {
  if (!me.value) void loadMe();
  void loadConfigModel();
  void loadSync();
  void loadDatastore();
  loadUploadStatus();
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
// another page, a command or a hand edit changed the file or who writes
const offLive = onLive((e) => {
  if (e.kind === 'config' || e.kind === 'me') reload();
  if (e.kind === 'sync') void loadSync();
});
onBeforeUnmount(offLive);
</script>

<style scoped>
.set-sub { margin: var(--space-3) 0 var(--space-1); font-weight: 600; }
.set-check { display: flex; align-items: flex-start; gap: .625rem; line-height: 1.45; }
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
