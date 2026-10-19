<!-- Roll an environment back (internal/release Rollback) to a final older
     than what it runs: its images and chart, no tag moving, and why, for
     the record of what it ran. Rolling back starts a job: the dialog closes
     into the page's ReleaseJob. The States board's "Roll back prod". -->
<template>
  <section class="gb-dialog gb-rb" role="dialog" aria-modal="true" aria-labelledby="rb-title">
    <header class="gb-dialog-head">
      <h2 id="rb-title">Roll back {{ env }}</h2>
      <button type="button" class="gb-icon-btn" aria-label="Close" :disabled="sending" @click="emit('close')"><GbIcon name="close" /></button>
    </header>

    <form class="gb-dialog-body" @submit.prevent="rollback">
      <p v-if="!targets.length" class="gb-hint">No final is older than what {{ env }} runs: there is nothing to roll back to.</p>
      <template v-else>
        <div class="gb-field">
          <label class="epfl-label" for="rb-tag">Back to</label>
          <select id="rb-tag" v-model="chosen" class="epfl-select">
            <option v-for="r in targets" :key="r.tag.name" :value="r.tag.name">{{ r.tag.name }} · cut {{ when(r.tag.date) }}</option>
          </select>
        </div>
        <p class="gb-rb-lead">Point {{ env }} back at <b>{{ chosen }}</b>'s images and chart.</p>
        <ul class="gb-checks">
          <li>
            <GbIcon :name="images.icon.name" :class="images.icon.cls" />
            <span>{{ images.text }}<small>If one is gone, the rollback stops before changing anything.</small></span>
          </li>
          <li>
            <GbIcon :name="checkIcon(true).name" :class="checkIcon(true).cls" />
            <span>No tag moves<small v-if="tagPattern.test(now)">{{ now }} stays a release. It's just not running.</small></span>
          </li>
        </ul>
        <label class="gb-field"><span class="epfl-label">Why, for the record (required)</span><input ref="reasonInput" v-model="reason" class="epfl-input" autocomplete="off" data-1p-ignore required /></label>
      </template>
      <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert"><b>Not started.</b> {{ error }}</p>
    </form>

    <footer class="gb-dialog-foot">
      <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="sending" @click="emit('close')">Cancel</button>
      <button type="button" class="epfl-btn epfl-btn-primary" :disabled="!ready" @click="rollback"><GbIcon name="undo" small />Roll {{ env }} back{{ chosen ? ` to ${chosen}` : '' }}</button>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { rollbackEnvironment, type EnvState, type ReleaseJob, type ReleaseRow } from '@/api/board';
import { message } from '@/issue';
import { byVersion, checkIcon, olderFinals, registryName, tagPattern } from '@/releases';
import { when } from '@/when';
import GbIcon from '@/components/GbIcon.vue';

// busy: a job runs, or the page waits for an answer; sending: the
// rollback's POST is out
const props = defineProps<{ env: string; envs: EnvState[]; releases: ReleaseRow[]; busy: boolean }>();
const emit = defineEmits<{ close: []; started: [job: ReleaseJob] }>();
const sending = defineModel<boolean>('sending', { default: false });

const state = computed(() => props.envs.find((e) => e.name === props.env));
const targets = computed(() => (state.value ? olderFinals(state.value, props.releases) : []));
const chosen = ref(targets.value[0]?.tag.name ?? ''); // the newest older final
const reason = ref('');
const error = ref('');
const reasonInput = ref<HTMLInputElement | null>(null);
// the releases reloaded under the dialog: a choice no longer offered falls
// back to the newest older final
watch(targets, (t) => {
  if (!t.some((r) => r.tag.name === chosen.value)) chosen.value = t[0]?.tag.name ?? '';
});
const list = new Intl.ListFormat('en', { type: 'conjunction' });

// what it runs now (the target ahead, in a split)
const now = computed(() => (state.value ? (byVersion(state.value.targets)[0]?.deployed.version ?? '') : ''));
// its images, as its record has them; without one, the rollback checks them
const images = computed(() => {
  const regs = [...new Set(props.releases.find((r) => r.tag.name === chosen.value)?.record?.artifacts.map((a) => registryName(a.registry)) ?? [])];
  return regs.length ? { icon: checkIcon(true), text: `Its images are recorded in ${list.format(regs)}` } : { icon: checkIcon(undefined), text: 'Its images are checked when rolling back: no record lists them yet' };
});
const ready = computed(() => !!chosen.value && !!reason.value.trim() && !sending.value && !props.busy);

async function rollback() {
  if (!ready.value) return;
  sending.value = true;
  error.value = '';
  try {
    emit('started', await rollbackEnvironment(props.env, chosen.value, reason.value.trim()));
  } catch (e) {
    error.value = message(e);
  } finally {
    sending.value = false;
  }
}

onMounted(() => reasonInput.value?.focus());
</script>

<style scoped>
.gb-rb { width: min(40rem, calc(100vw - 2rem)); max-height: calc(100vh - 2rem); overflow-y: auto; }
.gb-rb .gb-dialog-body { gap: 1.5rem; }
.gb-rb-lead { font-size: .9375rem; }
.gb-rb .gb-dialog-foot { justify-content: flex-end; }
</style>
