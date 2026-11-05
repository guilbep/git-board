<!-- An issue as a full page, at a link to share: #/issues/<git-bug id> (not
     a forge number: an issue can be local, or on two forges). -->
<template>
  <AppHeader :views="viewNames" />
  <main class="gb-issuepage">
    <IssuePanel :id="String(route.params.id)" :key="String(route.params.id)" :names="names" :fields="fields" page @open="(other: string) => router.push(`/issues/${other}`)" />
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { getFields, type FieldConfig } from '@/api';
import { loadColors } from '@/colors';
import { configModel, loadConfigModel } from '@/configmodel';
import { namespaces } from '@/facets';
import AppHeader from '@/components/AppHeader.vue';
import IssuePanel from '@/components/IssuePanel.vue';

const route = useRoute();
const router = useRouter();
const viewNames = computed(() => configModel.value?.view_order ?? []);
const fields = ref<Record<string, FieldConfig>>({});
const names = ref<string[]>([]);

onMounted(async () => {
  try {
    const [, f] = await Promise.all([loadConfigModel(), getFields(), loadColors()]);
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
