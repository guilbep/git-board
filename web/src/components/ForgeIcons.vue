<!-- The issue's forges, in the bar (docs/design/issue-head/README.md, 16e
     and 18): GitHub's or GitLab's icon each; nothing waiting, the bare
     icon; changes to send, a count with an up arrow; new there since the
     last sync, after this issue's Refresh, a count with a down arrow; a
     sync running, out of reach, changed on both sides: a small badge.
     Each opens the forges: their links, their state in words, the
     waiting changes, and Send now or Refresh, the one-issue sync's dry
     run then its ask (IssueSync). An issue on no forge yet, with forges
     configured: one icon to put it there. -->
<template>
  <span v-if="forges?.length || bridges" class="gb-hforges">
    <GbPopover v-for="f in states" :key="f.forge.url" :label="f.state.label" title="On the forges" button-class="gb-icon-btn gb-forge-btn" align="right">
      <template #button>
        <GbIcon :name="f.state.kind === 'other' ? 'external' : f.state.kind" />
        <template v-if="f.state.badge === 'count'">
          <span v-if="f.state.n" class="gb-fcount" aria-hidden="true"><GbIcon name="arrow-up" small />{{ f.state.n }}</span>
          <span v-if="f.state.incoming" class="gb-fcount is-in" :class="{ 'is-under': f.state.n }" aria-hidden="true"><GbIcon name="arrow-down" small />{{ f.state.incoming }}</span>
        </template>
        <span v-else-if="f.state.badge" class="gb-fbadge" :class="`is-${f.state.badge}`" aria-hidden="true"><GbIcon :name="badgeIcon[f.state.badge]" small /></span>
      </template>
      <ul class="gb-pop-list">
        <li v-for="g in states" :key="g.forge.url" class="gb-pop-forge">
          <a :href="g.forge.url" target="_blank" rel="noopener noreferrer">{{ g.forge.name }}{{ g.forge.number ? ` #${g.forge.number}` : '' }}<GbIcon name="external" small /><span class="gb-sr">(opens in a new tab)</span></a>
          <span class="gb-pop-muted">{{ g.state.label.slice(g.state.label.indexOf(': ') + 2) }}</span>
        </li>
      </ul>
      <template v-if="waiting.length">
        <button type="button" class="gb-linkbtn" :aria-expanded="showWaiting" @click="showWaiting = !showWaiting">
          {{ showWaiting ? 'Hide' : 'See' }} the {{ waiting.length }} waiting change{{ waiting.length === 1 ? '' : 's' }}
        </button>
        <WaitingChanges v-if="showWaiting" :changes="waiting" :forges="forges ?? []" @reveal="(id) => emit('reveal', id)" />
      </template>
      <p class="gb-pop-note">Waiting changes go out at the next <router-link to="/settings/sync">Sync</router-link>, or now:</p>
      <IssueSync :id="id" :waiting="waiting.length" on-forges @checked="(r) => noteCheck(id, r)" @synced="synced" />
    </GbPopover>
    <GbPopover v-if="!forges?.length && bridges" label="Not on the forges yet" title="On the forges" button-class="gb-icon-btn gb-forge-btn" align="right">
      <template #button><GbIcon name="upload" /></template>
      <p class="gb-pop-note">Not on the forges yet. The next <router-link to="/settings/sync">Sync</router-link> puts it there, or now:</p>
      <IssueSync :id="id" :waiting="waiting.length" :on-forges="false" @synced="synced" />
    </GbPopover>
  </span>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Forge, IssueEvent, IssueSyncResult } from '@/api/board';
import { forgeStatus, issueChecks, noteCheck } from '@/forges';
import { checkHolds, forgeStates, type ForgeBadge } from '@/forgestate';
import { waitingChanges } from '@/history';
import GbIcon from '@/components/GbIcon.vue';
import GbPopover from '@/components/GbPopover.vue';
import IssueSync from '@/components/IssueSync.vue';
import WaitingChanges from '@/components/WaitingChanges.vue';

const props = defineProps<{ id: string; forges?: Forge[] | null | undefined; events: IssueEvent[] }>();
const emit = defineEmits<{ changed: []; reveal: [id: string] }>();

const bridges = computed(() => forgeStatus.value?.bridges.length ?? 0);
// texts and changes not on a forge yet, as the side column counts them
const waitingFor = (forge: string) => props.events.filter((e) => e.pending?.includes(forge) || e.edit_pending?.includes(forge)).length;
// syncing from the server's job; out of reach from it or this issue's last
// run, the newer; changed on both sides and new there from this issue's
// last check, while it holds (forgestate.ts)
const check = computed(() => {
  const c = issueChecks.value[props.id];
  return c && checkHolds(c, forgeStatus.value?.job ?? null) ? c : null;
});
const states = computed(() =>
  forgeStates(props.forges ?? [], waitingFor, { bridges: forgeStatus.value?.bridges ?? [], job: forgeStatus.value?.job ?? null, check: check.value }),
);
// a real run of this issue: its check said what it would do, and now it's
// done; it stays the issue's last run, which says which forges it reached
function synced(r: IssueSyncResult) {
  noteCheck(props.id, r);
  emit('changed');
}
const waiting = computed(() => waitingChanges(props.events));
const showWaiting = ref(false);

const badgeIcon: Record<Exclude<ForgeBadge, 'count' | null>, 'sync' | 'cloud-off' | 'warn'> = { syncing: 'sync', unreachable: 'cloud-off', conflict: 'warn' };
</script>
