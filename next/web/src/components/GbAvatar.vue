<!-- A person's square (docs/prd/avatars.md): their picture when the server
     gave one, else, or when it fails to load (offline, a private GitHub
     avatar, Gravatar's 404), their initials, same size, same place. The
     picture is decoration: the name is said beside it, or by the control
     around it. -->
<template>
  <img
    v-if="src && !failed"
    class="gb-avatar gb-avatar-img"
    :class="{ 'gb-avatar-sm': small }"
    :src="src"
    alt=""
    aria-hidden="true"
    referrerpolicy="no-referrer"
    loading="lazy"
    decoding="async"
    @error="failed = true"
  />
  <span v-else class="gb-avatar" :class="{ 'gb-avatar-sm': small, 'gb-avatar-dark': dark }" aria-hidden="true">{{ initials(name) }}</span>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { initials } from '@/initials';

const props = defineProps<{ name: string; src?: string | undefined; small?: boolean; dark?: boolean }>();
// a failed picture stays failed for this page load; another one is tried
const failed = ref(false);
watch(() => props.src, () => (failed.value = false));
</script>
