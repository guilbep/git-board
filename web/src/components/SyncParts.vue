<!-- What a whole check found, part by part (docs/prd/config-fit.md, Part
     2): the issues to pull that changed, the new ones, those to create on
     the forge, those to send. Each count opens its issues; each box says
     whether Sync runs that part. A part left out waits for the next sync. -->
<template>
  <fieldset class="sp" data-testid="sync-parts">
    <legend class="h6">What Sync runs</legend>
    <div v-for="p in syncParts" :key="p.id" class="sp-row">
      <label class="sp-pick">
        <input v-model="chosen" type="checkbox" :value="p.id" :disabled="disabled" />
        <span>{{ plural(issues(p.id).length, p.one, p.many) }}</span>
      </label>
      <details v-if="issues(p.id).length">
        <summary>The issues</summary>
        <ol class="sp-list">
          <li v-for="(x, i) in issues(p.id)" :key="i">{{ x.forge ? `${x.forge}: ` : '' }}{{ x.title || x.issue }}</li>
        </ol>
      </details>
    </div>
    <p v-for="(h, i) in held" :key="i" class="gb-hint">{{ h }}</p>
    <p class="gb-hint">A part you leave out is left for the next sync. Leave out both sends to push nothing to the forges.</p>
  </fieldset>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { SyncPart, SyncRun } from '@/api';
import { plural, syncParts } from '@/ways';

const props = defineProps<{ run: SyncRun; disabled?: boolean }>();
const chosen = defineModel<SyncPart[]>({ required: true });
const issues = (part: SyncPart) => props.run.parts?.[part] ?? [];
// the creations a forge holds for consent, which "Create issues" gives
const held = computed(() => props.run.steps.filter((s) => s.what === 'held').map((s) => `${s.forge}: ${s.line}`));
</script>

<style scoped>
.sp { display: grid; gap: .5rem; margin: 0; padding: .75rem; border: var(--border-w) solid var(--border-subtle); }
.sp legend { padding: 0 .25rem; }
.sp p { margin: 0; }
.sp-row { display: flex; flex-wrap: wrap; align-items: baseline; gap: .25rem 1rem; }
.sp-pick { display: flex; align-items: center; gap: .5rem; cursor: pointer; }
.sp-pick input { accent-color: var(--primary-hover); }
.sp-row summary { cursor: pointer; font-size: .875rem; }
.sp-list { margin: .25rem 0 0; padding-left: 1.25rem; font-size: .875rem; overflow-wrap: anywhere; }
</style>
