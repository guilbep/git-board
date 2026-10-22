<!-- The Risk step (the canvas's C2): the three classes, the AI's suggestion
     when there is one (a comment, never the field), and Set <class>, which
     writes the issue's risk field. -->
<template>
  <h3 class="gb-ws-title">How much can this change break?</h3>
  <p class="gb-note">The class decides the next steps: whether there is a plan, who approves it, and how many people review.</p>

  <fieldset class="gb-risk" :disabled="!current || !can || busy">
    <legend class="gb-eyebrow">The class</legend>
    <label v-for="(line, r) in CLASSES" :key="r" :class="['gb-risk-opt', { 'is-on': picked === r }]">
      <input type="radio" name="gb-risk" :value="r" :checked="picked === r" @change="chosen = r" />
      <span><b>{{ cap(r) }}</b><span class="gb-note">{{ line }}</span></span>
    </label>
  </fieldset>

  <div v-if="ai || (current && cycle.ai)" class="gb-risk-ai">
    <p class="gb-eyebrow">Suggested by the AI</p>
    <p v-if="ai"><b>{{ cap(ai.risk) }}.</b> {{ ai.why }}</p>
    <button v-else type="button" class="gb-linkbtn" :disabled="busy" @click="emit('suggest')">{{ busy ? 'Asking…' : 'Ask the AI' }}</button>
  </div>

  <footer v-if="current" class="gb-step-foot">
    <div>
      <p class="gb-step-by">Set by {{ role ? `the ${role}` : 'anyone' }}</p>
      <p class="gb-note">{{ cycle.waits.anyone ? 'Anyone' : cycle.waits.login }} sets it. It becomes the issue's risk field; {{ riskWords(picked) }}.</p>
    </div>
    <button v-if="can" type="button" class="epfl-btn epfl-btn-primary" :disabled="busy" @click="emit('set', picked)">Set {{ picked }}</button>
  </footer>
  <template v-else>
    <p class="gb-gate">{{ gate }}</p>
    <p v-if="riskBy" class="gb-note">Set by {{ riskBy }}.</p>
  </template>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Cycle, IssueEvent } from '@/api/board';
import { riskWords, suggestion } from '@/cycle';

// can: the viewer may set it (the server's actions); current: step 2 is the
// issue's step, else it is looked at from the step bar and only read
const props = defineProps<{ cycle: Cycle; gate: string; current: boolean; busy: boolean; can: boolean; events: IssueEvent[]; riskBy?: string | undefined }>();
const emit = defineEmits<{ set: [risk: string]; suggest: [] }>();

const CLASSES: Record<string, string> = {
  low: 'A copy, a style, a config value. Step 3 is skipped; a plan is written after, if at all.',
  normal: 'Behaviour changes in one area. The plan is approved here by the plan role; one review.',
  structural: 'Data, a schema, a public interface. The plan goes through its own PR first, and review needs a second person.',
};
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

const ai = computed(() => suggestion(props.events));
// the issue's class once set; before, the viewer's pick, the AI's, or normal
const chosen = ref<string | null>(null);
const picked = computed(() => props.cycle.risk ?? props.cycle.evidence.risk ?? chosen.value ?? (ai.value && ai.value.risk in CLASSES ? ai.value.risk : 'normal'));
const role = computed(() => props.cycle.stages.roles?.risk?.replace(/-/g, ' '));
</script>

<style scoped>
.gb-risk { display: grid; gap: .5rem; margin: 0; padding: 0; border: 0; min-width: 0; }
.gb-risk legend { margin-bottom: .5rem; padding: 0; }
.gb-risk-opt { display: grid; grid-template-columns: 1.125rem 1fr; gap: .75rem; padding: 1rem 1.25rem; border: 1px solid var(--border-strong); cursor: pointer; }
.gb-risk-opt.is-on { border: 2px solid var(--fg); padding: calc(1rem - 1px) calc(1.25rem - 1px); }
.gb-risk:disabled .gb-risk-opt { cursor: default; }
.gb-risk-opt input { width: 1.125rem; height: 1.125rem; margin: .125rem 0 0; accent-color: var(--fg); }
.gb-risk-opt > span { display: grid; gap: .25rem; }
.gb-risk-opt b { font-size: 1.125rem; }
.gb-risk-ai { display: grid; gap: .25rem; padding: .875rem 1rem; background: var(--bg-subtle); font-size: .875rem; }
.gb-risk-ai p { margin: 0; }
.gb-risk-ai button { justify-self: start; }
</style>
