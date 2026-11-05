<!-- The account card, under the header's avatar (docs/prd/header-help.md,
     A2): who you are, your profile, switching who you write as, and how
     the app looks. Settings is the nav's cog; the shortcuts are the search
     field's ?; Sync's last check is Sync's tooltip. -->
<template>
  <div class="ac">
    <div class="ac-who">
      <GbAvatar class="ac-pic" :name="me.name" :src="me.avatar_url" dark />
      <p class="ac-name"><b>{{ me.name }}</b><code v-if="me.login && me.login !== me.name">{{ me.login }}</code></p>
    </div>
    <div class="ac-links">
      <router-link to="/settings/you" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="emit('close')">Your profile</router-link>
      <router-link :to="{ path: '/settings/you', hash: '#set-me' }" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="emit('close')">Switch identity</router-link>
    </div>
    <div class="ac-look">
      <p id="ac-look" class="ac-label">Appearance</p>
      <div class="ac-seg" role="group" aria-labelledby="ac-look">
        <button v-for="o in looks" :key="o.value" type="button" :aria-pressed="themePref === o.value" @click="themePref = o.value">
          <GbIcon :name="o.icon" small />{{ o.label }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Person } from '@/api/board';
import { themePref, type ThemePref } from '@/theme';
import GbAvatar from '@/components/GbAvatar.vue';
import GbIcon from '@/components/GbIcon.vue';
import type { IconName } from '@/components/GbIcon.vue';

defineProps<{ me: Person }>();
const emit = defineEmits<{ close: [] }>();

// as Settings › You names them
const looks: { value: ThemePref; label: string; icon: IconName }[] = [
  { value: 'system', label: 'System', icon: 'monitor' },
  { value: 'light', label: 'Light', icon: 'sun' },
  { value: 'dark', label: 'Dark', icon: 'moon' },
];
</script>

<style scoped>
.ac-who { display: flex; align-items: center; gap: .75rem; padding: 1rem; }
.ac-who .ac-pic { width: 2.75rem; height: 2.75rem; font-size: .875rem; }
.ac-name { display: flex; flex-direction: column; gap: .125rem; min-width: 0; margin: 0; }
.ac-name b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ac-name code { color: var(--fg-muted); font-size: .75rem; }
.ac-links { display: flex; gap: .5rem; padding: 0 1rem .875rem; }
.ac-links > a { flex: 1; justify-content: center; text-align: center; }
.ac-look { padding: .75rem 1rem 1rem; border-top: var(--border-w) solid var(--border-subtle); }
.ac-label { margin: 0 0 .5rem; color: var(--fg-muted); font-size: .75rem; font-weight: var(--w-bold); letter-spacing: .06em; text-transform: uppercase; }
.ac-seg { display: flex; border: var(--border-w) solid var(--border); }
.ac-seg > button { flex: 1; display: inline-flex; align-items: center; justify-content: center; gap: .375rem; height: 2.125rem; padding: 0; border: 0; background: var(--bg); color: var(--fg); font: inherit; font-size: .8125rem; cursor: pointer; }
.ac-seg > button + button { border-left: var(--border-w) solid var(--border); }
.ac-seg > button[aria-pressed='true'] { background: var(--bg-dark); color: var(--fg-inverse); }
.ac-seg > button:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: -0.125rem; }
</style>
