<!-- The description's verdict (docs/design/time-flow, F2): who reviews the
     current version, what was said on the one before, and Request changes
     or Approve vN, one verdict per version. The rules are spec.ts's. -->
<template>
  <div class="gb-vbar">
    <div class="gb-vbar-text">
      <span class="gb-eyebrow">{{ eyebrow }}</span>
      <span class="gb-vbar-who">{{ who }}</span>
      <span v-if="line" class="gb-vbar-line">{{ line }}</span>
      <span v-if="ask && !criteria" class="gb-vbar-line">Add acceptance criteria first.</span>
    </div>
    <div v-if="ask && !asking" class="gb-vbar-btns">
      <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy" @click="asking = true">Request changes</button>
      <button type="button" class="epfl-btn epfl-btn-primary" :disabled="busy || !criteria" @click="void give('approve', '')">Approve v{{ state.v }}</button>
    </div>
    <form v-if="asking" class="gb-vbar-ask" @submit.prevent="send">
      <label :for="uid">Which changes v{{ state.v }} needs</label>
      <textarea :id="uid" v-model="changes" class="epfl-input" rows="3" required :disabled="busy" />
      <span class="gb-vbar-btns">
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="asking = false">Cancel</button>
        <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !changes.trim()">Send</button>
      </span>
    </form>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useId } from 'vue';
import { when } from '@/me';
import type { SpecState, Verdict } from '@/spec';

const props = defineProps<{
  state: SpecState;
  reviewer: string; // "Person B, PM", or "Anyone but Person A" with no spec role
  may: boolean; // the viewer gives it (mayGive)
  moves: boolean; // an approval moves the status to ready
  criteria: boolean;
  busy: boolean;
  give: (verdict: Verdict['verdict'], body: string) => Promise<boolean>;
}>();

const uid = useId();
const asking = ref(false);
const changes = ref('');
async function send() {
  if (await props.give('request-changes', changes.value.trim())) ((asking.value = false), (changes.value = ''));
}

const s = computed(() => props.state);
const said = (x: Verdict) => (x.verdict === 'approve' ? 'approved' : 'changes requested');
const at = (x: Verdict) => when(x.event.at, true);
// the buttons: the viewer may, and vN has no approval yet
const ask = computed(() => props.may && s.value.on?.verdict !== 'approve');
const eyebrow = computed(() => {
  const { v, on, approved } = s.value;
  if (on?.verdict === 'approve') return `Spec · approved v${v} by ${on.event.author.name}, ${at(on)}`;
  if (on) return `Spec · changes requested on v${v}`;
  if (approved) return `Spec · approved v${approved.v}, v${v} awaits`;
  return `Spec · v${v} awaits a verdict`;
});
const who = computed(() => `${props.reviewer}${props.reviewer.includes(', ') ? ',' : ''} reviews v${s.value.v}`);
const line = computed(() => {
  const { v, on, before, resolvedBy, open } = s.value;
  const out: string[] = [];
  if (before) out.push(`v${before.v}: ${said(before)} by ${before.event.author.name}, ${at(before)}.`);
  if (on?.verdict === 'request-changes') out.push(`${on.event.author.name}: ${on.event.body ?? ''}`);
  if (v > 1 || open) out.push(`${resolvedBy ? `v${v} resolved ${resolvedBy} thread${resolvedBy === 1 ? '' : 's'}; ` : ''}${open} ${open === 1 ? 'is' : 'are'} open.`);
  if (props.moves && ask.value) out.push(`Approving moves the issue to ready and opens step 2.`);
  return out.join(' ');
});
</script>

<style scoped>
.gb-vbar { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 12px; margin-top: 1rem; padding: 16px 20px; border: 1px solid var(--border-strong); }
.gb-vbar-text { display: flex; flex: 1 1 320px; flex-direction: column; gap: 4px; }
.gb-vbar-who { font-size: 1.0625rem; font-weight: 700; }
.gb-vbar-line { font-size: .875rem; color: var(--fg-muted); }
.gb-vbar-btns { display: flex; gap: 8px; }
.gb-vbar-ask { display: flex; flex-direction: column; gap: 6px; flex: 1 1 100%; }
.gb-vbar-ask label { font-size: .8125rem; }
.gb-vbar-ask .gb-vbar-btns { justify-content: flex-end; }
</style>
