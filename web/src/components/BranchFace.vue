<!-- A branch chip's face (README 16c): its state's icon, its name in the
     issue's folder (2771/2), a word, and an arrow to the face whose move it
     is. Color never says the state alone: the icon and the word do too. -->
<template>
  <GbIcon :name="state.icon" small />
  <span class="gb-mono">{{ state.face }}</span>
  <span class="gb-bchip-word">{{ state.word }}</span>
  <template v-if="state.who">
    <GbIcon name="forward" small class="gb-bchip-to" />
    <span class="gb-avatar gb-avatar-sm" aria-hidden="true">{{ initials(state.who) }}</span>
  </template>
</template>

<script setup lang="ts">
import type { BranchState } from '@/branchstate';
import { initials } from '@/me';
import GbIcon from '@/components/GbIcon.vue';

defineProps<{ state: BranchState }>();
</script>
