<!-- The conversation's people, at the end of the tab row (docs/design/
     issue-head/README.md, 16f): four faces, then +N; the list in a
     popover, the author first, then by their latest
     comment. -->
<template>
  <GbPopover v-if="rows.length" :label="peopleLabel(rows.length)" title="In this conversation" button-class="gb-people" align="right">
    <template #button>
      <span v-for="r in rows.slice(0, 4)" :key="r.person.id" class="gb-avatar gb-avatar-sm" :class="{ 'gb-avatar-dark': r.you }" aria-hidden="true">{{ initials(r.person.name) }}</span>
      <span v-if="rows.length > 4" class="gb-people-more" aria-hidden="true">+{{ rows.length - 4 }}</span>
    </template>
    <ul class="gb-pop-list">
      <li v-for="r in rows" :key="r.person.id" class="gb-pop-person">
        <span class="gb-avatar gb-avatar-sm" :class="{ 'gb-avatar-dark': r.you }" aria-hidden="true">{{ initials(r.person.name) }}</span>
        <b>{{ r.person.name }}</b>
        <span class="gb-pop-muted">{{ r.role }}<template v-if="r.you"> · you</template></span>
      </li>
    </ul>
  </GbPopover>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { IssueEvent, Person } from '@/api/board';
import { initials, me } from '@/me';
import { peopleLabel, peopleOf } from '@/people';
import GbPopover from '@/components/GbPopover.vue';

const props = defineProps<{ author: Person; events: IssueEvent[] }>();
const rows = computed(() => peopleOf(props.author, props.events, me.value?.me?.id ?? ''));
</script>
