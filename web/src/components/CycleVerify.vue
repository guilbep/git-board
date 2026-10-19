<!-- The Verify workspace (E3): filled in a later task. -->
<template>
  <h3 class="gb-ws-title">Run it, read it, then sign it off</h3>
  <p class="gb-gate">{{ cycle.gate }}</p>
  <div class="gb-cycle-actions">
    <button v-if="mine" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="emit('signoff')">Sign off {{ status }}</button>
  </div>
</template>

<script setup lang="ts">
import type { Cycle, IssueEvent } from '@/api/board';

defineProps<{ cycle: Cycle; status: string; events: IssueEvent[]; busy: boolean; mine: boolean; needsWhy: boolean }>();
const emit = defineEmits<{ check: [index: number]; signoff: [why?: string]; run: [output: string, commit: string]; back: [] }>();
</script>
