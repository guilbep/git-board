<!-- The issue's forges, in the bar (docs/design/issue-head/README.md, 16e
     and 18): GitHub's or GitLab's icon each; nothing waiting, the bare
     icon; changes to send, a count with an up arrow; a sync running, a
     small badge. Each opens the forges: their links, their state in words,
     the waiting changes. They go out at the next Sync (v1's one-issue sync
     waits for v2's: docs/rewrite/api.md, POST /api/sync with issue). -->
<template>
  <span v-if="forges?.length" class="gb-hforges">
    <GbPopover v-for="f in states" :key="f.forge.url" :label="f.state.label" title="On the forges" button-class="gb-icon-btn gb-forge-btn" align="right">
      <template #button>
        <GbIcon :name="f.state.kind === 'other' ? 'external' : f.state.kind" />
        <!-- the number beside the logo: which issue there, said once -->
        <span v-if="f.forge.number" class="gb-fnum" aria-hidden="true">#{{ f.forge.number }}</span>
        <span v-if="f.state.badge === 'count' && f.state.n" class="gb-fcount" aria-hidden="true"><GbIcon name="arrow-up" small />{{ f.state.n }}</span>
        <span v-else-if="f.state.badge === 'syncing'" class="gb-fbadge is-syncing" aria-hidden="true"><GbIcon name="sync" small /></span>
      </template>
      <ul class="gb-pop-list">
        <li v-for="g in states" :key="g.forge.url" class="gb-pop-forge">
          <a :href="safeHref(g.forge.url)" target="_blank" rel="noopener noreferrer">{{ g.forge.name }}{{ g.forge.number ? ` #${g.forge.number}` : '' }}<GbIcon name="external" small /><span class="gb-sr">(opens in a new tab)</span></a>
          <span class="gb-pop-muted">{{ g.state.label.slice(g.state.label.indexOf(': ') + 2) }}</span>
        </li>
      </ul>
      <template v-if="waiting.length">
        <button type="button" class="gb-linkbtn" :aria-expanded="showWaiting" @click="showWaiting = !showWaiting">
          {{ showWaiting ? 'Hide' : 'See' }} the {{ waiting.length }} waiting change{{ waiting.length === 1 ? '' : 's' }}
        </button>
        <WaitingChanges v-if="showWaiting" :changes="waiting" :forges="forges ?? []" @reveal="(id) => emit('reveal', id)" />
        <p class="gb-pop-note">Waiting changes go out at the next <router-link to="/settings/sync">Sync</router-link>.</p>
      </template>
      <p v-else class="gb-pop-note">Nothing waits to go out. What changed on the forges comes in at the next <router-link to="/settings/sync">Sync</router-link>.</p>
    </GbPopover>
  </span>
</template>

<script setup lang="ts">
import { safeHref } from '@/links';
import { computed, ref } from 'vue';
import type { Forge, IssueEvent } from '@/api';
import { forgeState } from '@/forgestate';
import { waitingChanges } from '@/history';
import { syncStatus } from '@/sync';
import GbIcon from '@/components/GbIcon.vue';
import GbPopover from '@/components/GbPopover.vue';
import WaitingChanges from '@/components/WaitingChanges.vue';

const props = defineProps<{ id: string; forges?: Forge[] | null | undefined; events: IssueEvent[] }>();
const emit = defineEmits<{ changed: []; reveal: [id: string] }>();

// texts and changes not on a forge yet, as the side column counts them
const waitingFor = (forge: string) => props.events.filter((e) => e.pending?.includes(forge) || e.edit_pending?.includes(forge)).length;
const running = computed(() => syncStatus.value?.run?.state === 'running');
const states = computed(() => (props.forges ?? []).map((forge) => ({ forge, state: forgeState(forge, waitingFor(forge.name), { running: running.value }) })));
const waiting = computed(() => waitingChanges(props.events));
const showWaiting = ref(false);
</script>

<style scoped>
.gb-fnum { margin-left: .25rem; font-size: .8125rem; font-weight: 700; }
</style>
