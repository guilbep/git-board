<!-- The issue's labels (docs/design/issue-head, 16b, open point 7): the
     project's [labels] list as checkboxes, never free text, each with its
     color and description; a label outside the list (from a forge, or from
     before the list) shows with a warning, to remove here or to add to the
     list in the project file. Without the list, nothing to pick. -->
<template>
  <div class="gb-fe">
    <p v-if="!defined" class="gb-fe-note">The project file lists no labels to pick: add <code>[labels] list</code> in <router-link to="/settings/project-file">Settings › Project file</router-link>.</p>
    <ul v-if="list.length" class="gb-pop-list" role="menu" aria-label="Labels">
      <li v-for="l in list" :key="l" role="none">
        <button type="button" role="menuitemcheckbox" class="gb-pop-item gb-lp-item" :aria-checked="line.picked.includes(l)" :title="descriptions[l]" :disabled="busy" @click="toggle(l)">
          <span class="gb-fe-box" :class="{ 'is-on': line.picked.includes(l) }" aria-hidden="true"><GbIcon v-if="line.picked.includes(l)" name="check" small /></span>
          <span class="gb-dot" :style="dot(colors[l])" aria-hidden="true" />
          <span class="gb-lp-name">{{ l }}</span>
          <span v-if="descriptions[l]" class="gb-lp-desc">{{ descriptions[l] }}</span>
        </button>
      </li>
    </ul>
    <template v-if="line.outside.length">
      <p class="gb-fe-note is-held"><GbIcon name="warn" small /> Not in the project's list:</p>
      <ul class="gb-pop-list" aria-label="Labels outside the list">
        <li v-for="l in line.outside" :key="l" class="gb-fe-ref">
          <span>{{ l }}</span>
          <button type="button" class="gb-fe-x" :aria-label="`Remove ${l}`" :title="`Remove ${l}`" :disabled="busy" @click="emit('labels', [], [l])"><GbIcon name="close" small /></button>
        </li>
      </ul>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { labelLine } from '@/headfields';
import { labelList } from '@/labellist';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ labels: string[]; busy: boolean }>();
const emit = defineEmits<{ labels: [add: string[], remove: string[]] }>();

const defined = computed(() => !!labelList.value?.defined);
const list = computed(() => labelList.value?.list ?? []);
const colors = computed(() => labelList.value?.colors ?? {});
const descriptions = computed(() => labelList.value?.descriptions ?? {});
const line = computed(() => labelLine(props.labels, list.value));
const dot = (hex: string | undefined) => (hex ? { '--c-dot': `#${hex}` } : undefined);
const toggle = (l: string) => (line.value.picked.includes(l) ? emit('labels', [], [l]) : emit('labels', [l], []));
</script>
