<!-- The Plan workspace (E1): filled in the next task. -->
<template>
  <h3 class="gb-ws-title">Plan</h3>
  <p class="gb-gate">{{ cycle.gate }}</p>
  <div class="gb-cycle-actions">
    <button v-if="mine" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="emit('approve')">Approve the plan</button>
  </div>
</template>

<script setup lang="ts">
import type { Cycle } from '@/api/board';

defineProps<{ cycle: Cycle; busy: boolean; mine: boolean }>();
const emit = defineEmits<{ approve: []; revise: [body: string] }>();
</script>
