<!-- The bar every section tab ends with: the state (as the raw editor's
     chip reads it), Discard, Save — sticky, so it stays in reach however
     far the form scrolls. -->
<template>
  <div class="csb" role="toolbar" :aria-label="`Save the ${section}`">
    <span v-if="error" class="epfl-alert epfl-alert-danger csb-err" role="alert">{{ error }}</span>
    <span v-else-if="conflict" class="epfl-alert epfl-alert-warning csb-err" role="alert">
      The file changed since you opened it.
      <button type="button" class="gb-linkbtn" @click="$emit('reload')">Reload (your draft is dropped)</button>
    </span>
    <span v-else-if="saved && !dirty" class="csb-state csb-ok" role="status">Saved.</span>
    <span v-else-if="state" class="csb-state" :class="dirty ? 'csb-dirty' : 'csb-ok'" role="status">{{ state }}</span>
    <span class="gb-grow" />
    <button v-if="dirty" type="button" class="gb-linkbtn" @click="$emit('discard')">Discard changes</button>
    <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!dirty || saving || !valid" @click="$emit('save')">
      {{ saving ? 'Saving…' : 'Save' }}
    </button>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  section: string;
  dirty: boolean;
  saving?: boolean;
  saved?: boolean;
  valid?: boolean; // the form's own check: false holds Save
  state?: string; // what the section holds, "5 fields · 3 views"
  error?: string; // the save's failure, when it isn't a conflict
  conflict?: boolean; // the file changed since the form read it
}>();

defineEmits<{ save: []; discard: []; reload: [] }>();
</script>

<style scoped>
.csb { position: sticky; bottom: 0; z-index: 5; display: flex; align-items: center; gap: .75rem; flex-wrap: wrap; padding: .625rem 0; background: var(--bg); border-top: 1px solid var(--border); }
.csb .gb-linkbtn { font-size: .875rem; }
.gb-grow { flex: 1; }
.csb-state { font-size: .8125rem; font-weight: 700; padding: .25rem .625rem; border: 1px solid var(--border); color: var(--fg-muted); }
.csb-ok { color: var(--success-fg); border-color: var(--success-border); background: var(--success-bg); }
.csb-dirty { color: var(--warning-fg); border-color: var(--warning-border); background: var(--warning-bg); }
.csb-err { flex-basis: 100%; margin: 0; }
.csb-err .gb-linkbtn { color: inherit; font-weight: 700; }
</style>
