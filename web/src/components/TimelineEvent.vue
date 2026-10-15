<!-- One line of history: labels as field changes, closed / reopened, a new
     title. -->
<template>
  <li class="gb-event" :class="{ 'gb-event-closed': event.kind === 'status' && event.status === 'closed', 'gb-event-open': event.kind === 'status' && event.status === 'open' }">
    <span class="gb-event-icon"><GbIcon :name="icon" small /></span>
    <span class="gb-event-text">
      <b>{{ event.author.name }}</b>
      <template v-if="event.kind === 'labels'">
        <template v-for="(c, i) in changes" :key="i">
          {{ i === 0 ? '' : i === changes.length - 1 ? ' and ' : ', ' }}
          <template v-if="c.verb === 'set'"> set <b>{{ label(c.field) }}</b> to <span class="gb-tag">{{ value(c.field, c.values[0]!) }}</span></template>
          <template v-else-if="c.verb === 'cleared'"> cleared <b>{{ label(c.field) }}</b> (was <span class="gb-tag">{{ value(c.field, c.values[0]!) }}</span>)</template>
          <template v-else>
            {{ c.verb }} {{ c.field === 'label' ? (c.values.length > 1 ? 'labels' : 'label') : c.field }}
            <span v-for="v in c.values" :key="v" class="gb-tag">{{ value(c.field, v) }}</span>
          </template>
        </template>
      </template>
      <template v-else-if="event.kind === 'status'"> {{ event.status === 'closed' ? 'closed this' : 'reopened this' }}</template>
      <template v-else-if="event.kind === 'title'">
        changed the title <span class="gb-strike">{{ event.was }}</span> to <b>{{ event.title }}</b>
      </template>
      · <time :datetime="event.at">{{ when(event.at) }}</time>
      <span v-if="event.pending?.length" class="gb-fstate gb-fstate-local"><GbIcon name="clock" small /> not on {{ event.pending.join(' or ') }} yet, next sync</span>
    </span>
  </li>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { IssueEvent } from '@/api/board';
import { displayName, formatValue, type Fields } from '@/fields';
import { describeLabels } from '@/history';
import { when } from '@/me';
import GbIcon, { type IconName } from '@/components/GbIcon.vue';

const props = defineProps<{ event: IssueEvent; fields: Fields }>();

const changes = computed(() => describeLabels(props.event.added ?? [], props.event.removed ?? []));
const icon = computed<IconName>(() => {
  switch (props.event.kind) {
    case 'status':
      return props.event.status === 'closed' ? 'closed' : 'open';
    case 'title':
      return 'title';
    default:
      return changes.value.every((c) => c.field === 'assignee') ? 'user' : 'tag';
  }
});

const label = (field: string) => displayName(field);
const value = (field: string, v: string) => formatValue(field, v, props.fields);
</script>
