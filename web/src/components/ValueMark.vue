<!-- A field value's mark: its icon when its words say what it is (status,
     priority, kind: valueicons.ts), in the value's color; else the color's
     dot (not in a chip, whose tint already says it). 16×16, 1.6 stroke,
     drawn in currentColor as resslab-hub's and co2's line icons are. -->
<template>
  <svg v-if="icon" class="gb-vmark" :class="cls" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    <template v-if="icon === 'st-backlog'"><circle cx="8" cy="8" r="6" stroke-dasharray="2.2 2.2" /></template>
    <template v-else-if="icon === 'st-ready'"><circle cx="8" cy="8" r="6" /></template>
    <template v-else-if="icon === 'st-progress'"><circle cx="8" cy="8" r="6" /><path d="M8 4.5a3.5 3.5 0 0 1 0 7Z" fill="currentColor" stroke="none" /></template>
    <template v-else-if="icon === 'st-review'"><circle cx="8" cy="8" r="6" /><path d="M8 4.5a3.5 3.5 0 1 1-3.5 3.5H8Z" fill="currentColor" stroke="none" /></template>
    <template v-else-if="icon === 'st-test'"><path d="M6.5 2h3M7 2v4.2L3.6 12.4A1 1 0 0 0 4.5 14h7a1 1 0 0 0 .9-1.6L9 6.2V2" /><path d="M5 10.5h6" /></template>
    <template v-else-if="icon === 'st-done'"><circle cx="8" cy="8" r="6.5" fill="currentColor" stroke="none" /><path d="M5.2 8.2 7.1 10l3.7-3.9" stroke="var(--bg)" /></template>
    <template v-else-if="icon === 'st-blocked'"><circle cx="8" cy="8" r="6" /><path d="M3.8 12.2 12.2 3.8" /></template>
    <template v-else-if="icon === 'st-waiting'"><circle cx="8" cy="8" r="6" /><path d="M6.6 5.8v4.4M9.4 5.8v4.4" /></template>
    <template v-else-if="icon === 'prio-urgent'"><rect x="1.5" y="1.5" width="13" height="13" rx="3" fill="currentColor" stroke="none" /><path d="M8 4.5v4.2M8 11.3v.1" stroke="var(--bg)" stroke-width="1.9" /></template>
    <template v-else-if="icon.startsWith('prio-')">
      <path d="M3.5 12.5v-2" :stroke-opacity="1" stroke-width="2.2" />
      <path d="M8 12.5v-5" :stroke-opacity="icon === 'prio-low' ? 0.25 : 1" stroke-width="2.2" />
      <path d="M12.5 12.5v-8" :stroke-opacity="icon === 'prio-high' ? 1 : 0.25" stroke-width="2.2" />
    </template>
    <template v-else-if="icon === 'kind-bug'"><rect x="5" y="5" width="6" height="8.5" rx="3" /><path d="M6.2 3.2 7 5M9.8 3.2 9 5M2.5 8h2.5M11 8h2.5M3 12l2-1M13 12l-2-1M3 4.5l2 1.5M13 4.5l-2 1.5M8 7v6" /></template>
    <template v-else-if="icon === 'kind-feat'"><path d="M8 1.8 9.3 6.2 13.7 7.5 9.3 8.8 8 13.2 6.7 8.8 2.3 7.5 6.7 6.2Z" fill="currentColor" fill-opacity=".18" /><path d="M13 11.5v3M11.5 13h3" /></template>
    <template v-else-if="icon === 'kind-doc'"><path d="M4 1.8h5.2L12.5 5v9.2H4Z" /><path d="M9 1.8V5h3.5M6 8h4.5M6 10.6h4.5" /></template>
    <template v-else-if="icon === 'kind-chore'"><path d="M10.2 2.3a3.2 3.2 0 0 0-3.9 4.2L2.2 10.6a1.3 1.3 0 0 0 1.9 1.9L8.2 8.4a3.2 3.2 0 0 0 4.2-3.9L10.6 6.3 9 5.9 8.6 4.3Z" /></template>
  </svg>
  <span v-else-if="!chip && cls" class="gb-dot" :class="cls" aria-hidden="true" />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { colorClass } from '@/colors';
import { valueIcon } from '@/valueicons';

const props = defineProps<{ field: string; value: string; chip?: boolean }>();
const icon = computed(() => valueIcon(props.field, props.value));
const cls = computed(() => colorClass(props.field, props.value));
</script>

<style scoped>
.gb-vmark { flex: none; width: 1rem; height: 1rem; color: var(--c-dot, var(--fg-muted)); vertical-align: -0.15em; }
</style>
