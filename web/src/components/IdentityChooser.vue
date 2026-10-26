<!-- Choose who you are: comments, edits and new issues are signed with a
     person of git-board's. Adopting one sets board.me in git's config. -->
<template>
  <section class="gb-statecard gb-me-choose" aria-labelledby="choose-me">
    <h3 id="choose-me" class="h5">Choose who you are</h3>
    <p>Comments, edits and new issues are signed with a person of git-board's. Pick yours to start writing.</p>
    <fieldset v-if="people.length" class="gb-pick">
      <legend class="gb-hint">People in this repository</legend>
      <label v-for="p in shownPeople" :key="p.id" class="gb-pick-row">
        <input v-model="picked" type="radio" name="me" :value="p.id" />
        <span><b>{{ p.name }}</b><br /><span class="gb-hint">{{ p.login || p.id.slice(0, 7) }}</span></span>
      </label>
      <button v-if="people.length > 5 && !all" type="button" class="gb-linkbtn" @click="all = true">Show all {{ people.length }} identities</button>
    </fieldset>
    <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!picked || busy" @click="adopt">This is me</button>

    <details class="gb-me-new" :open="!people.length">
      <summary>Not in the list? Create an identity</summary>
      <form class="gb-me-form" @submit.prevent="create">
        <label class="epfl-label" for="me-name">Name</label>
        <input id="me-name" v-model="name" class="epfl-input" autocomplete="name" required />
        <label class="epfl-label" for="me-email">Email</label>
        <input id="me-email" v-model="email" class="epfl-input" type="email" autocomplete="email" />
        <button type="submit" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!name.trim() || busy">Create and use</button>
      </form>
    </details>
    <p v-if="error" class="gb-hint" role="alert"><b>Not saved.</b> {{ error }}</p>
    <p class="gb-hint">Saved for this repository, in its git config (<code>board.me</code>).</p>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { adoptIdentity, createIdentity } from '@/api/board';
import { message } from '@/issue';
import { me } from '@/me';

const picked = ref('');
const name = ref('');
const email = ref('');
const busy = ref(false);
const error = ref('');
const all = ref(false);

const people = computed(() => me.value?.identities ?? []);
const shownPeople = computed(() => (all.value ? people.value : people.value.slice(0, 5)));

async function run(fn: () => ReturnType<typeof adoptIdentity>) {
  busy.value = true;
  error.value = '';
  try {
    me.value = await fn();
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}

const adopt = () => run(() => adoptIdentity(picked.value));
const create = () => run(() => createIdentity(name.value, email.value));
</script>

<style scoped>
.gb-me-choose { display: grid; gap: .75rem; padding: 1.25rem; border: 1px solid var(--border-strong); }
.gb-me-choose h3 { margin: 0; }
.gb-me-choose p { margin: 0; }
.gb-me-choose > .epfl-btn { justify-self: start; }
.gb-pick { display: grid; gap: .5rem; margin: 0; padding: 0; border: 0; }
.gb-pick-row { display: flex; gap: .5rem; align-items: flex-start; cursor: pointer; }
.gb-pick-row input { margin-top: .3rem; accent-color: var(--primary-hover); }
.gb-linkbtn { justify-self: start; padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
.gb-me-new summary { font-weight: 700; cursor: pointer; }
.gb-me-form { display: grid; gap: .375rem; margin-top: .75rem; max-width: 24rem; }
.gb-me-form .epfl-btn { justify-self: start; margin-top: .5rem; }
</style>
