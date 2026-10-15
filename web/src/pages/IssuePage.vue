<!-- An issue as a full page, at a link to share: #/issues/<git-bug id> (not
     a forge number: an issue can be local, or on two forges). -->
<template>
  <AppHeader :views="viewNames" active="" />
  <main class="gb-issuepage">
    <IssuePanel :id="String(route.params.id)" :names="names" :fields="fields" page />
  </main>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { getFields, getViews, type FieldConfig } from '@/api/board';
import { loadColors } from '@/colors';
import { namespaces } from '@/facets';
import AppHeader from '@/components/AppHeader.vue';
import IssuePanel from '@/components/IssuePanel.vue';

const route = useRoute();
const viewNames = ref<string[]>([]);
const fields = ref<Record<string, FieldConfig>>({});
const names = ref<string[]>([]);

onMounted(async () => {
  try {
    const [views, f] = await Promise.all([getViews(), getFields(), loadColors()]);
    viewNames.value = Object.keys(views).sort();
    fields.value = f;
    names.value = namespaces([], f);
  } catch {
    // the panel shows its own errors
  }
});
</script>

<style scoped>
.gb-issuepage { padding: 0 0 3rem; }
</style>
