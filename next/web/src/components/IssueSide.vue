<!-- Under the head's branch line, while they have something to say
     (docs/design/issue-head/README.md, the side column goes): the
     description's verdict, Spec, and the issues naming this one, Linked
     issues. The rest of the old side column lives in the head: the fields
     line and each value's editor, the label picker, the branch line, the
     forge icons, the people in the tab row, the bell. -->
<template>
  <div class="gb-side">
    <section v-if="spec" class="gb-side-sec" aria-labelledby="side-spec">
      <h3 id="side-spec" class="gb-section-title">Spec</h3>
      <p class="gb-spec-now">
        v{{ spec.v }} ·
        <template v-if="spec.on?.verdict === 'approve'">approved by {{ spec.on.event.author.name }}, {{ when(spec.on.event.at) }}</template>
        <template v-else-if="spec.on">changes requested</template>
        <template v-else-if="reviewer">awaits the verdict of <b>{{ reviewer.name }}</b> <span v-if="reviewer.role" class="gb-role">{{ reviewer.role }}</span></template>
        <template v-else>awaits a verdict</template>
      </p>
      <p v-if="specLine" class="gb-hint">{{ specLine }}</p>
    </section>

    <section v-if="incoming.length" class="gb-side-sec" aria-labelledby="side-linked">
      <h3 id="side-linked" class="gb-section-title">Linked issues</h3>
      <div v-for="[field, list] in incoming" :key="field" class="gb-linked">
        <p class="gb-linked-name">{{ inverseName(field) }} <span class="gb-hint">{{ list.length }}</span></p>
        <button v-for="r in list" :key="r.id" type="button" class="gb-ref" @click="r.id && emit('open', r.id)">
          <GbIcon :name="r.status === 'closed' ? 'closed' : 'open'" small /> {{ r.title }}
        </button>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { IssueRef } from '@/api/board';
import { displayName } from '@/fields';
import { when } from '@/me';
import type { SpecState } from '@/spec';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{
  linked: Record<string, IssueRef[]>; // the issues naming this one, by field, as the head read them
  // the description's verdict (VerdictBar's), and the spec role's holder: none, anyone but the author
  spec?: SpecState | undefined;
  reviewer?: { name: string; role?: string | undefined } | undefined;
}>();
const emit = defineEmits<{ open: [id: string] }>();

const incoming = computed(() => Object.entries(props.linked).sort(([a], [b]) => a.localeCompare(b)));
// what an issue is to the issues naming it in that field
const inverses: Record<string, string> = { parent: 'Sub-issues', 'blocked-by': 'Blocks', blocks: 'Blocked by', 'depends-on': 'Needed by', 'duplicate-of': 'Duplicates' };
const inverseName = (field: string) => inverses[field] ?? `Named in ${displayName(field)}`;

// "v1: changes requested, Oct 21 · 2 threads open"
const specLine = computed(() => {
  const s = props.spec;
  if (!s) return '';
  const was = s.before && `v${s.before.v}: ${s.before.verdict === 'approve' ? 'approved' : 'changes requested'}, ${when(s.before.event.at)}`;
  return [was, s.open && `${s.open} thread${s.open === 1 ? '' : 's'} open`].filter(Boolean).join(' · ');
});
</script>

<style scoped>
.gb-ref { display: flex; align-items: center; gap: .375rem; width: 100%; margin-top: .375rem; padding: .25rem 0; border: 0; background: none; color: var(--fg); font: inherit; font-size: .875rem; text-align: left; cursor: pointer; }
.gb-ref:hover { text-decoration: underline; }
.gb-linked + .gb-linked { margin-top: .75rem; }
.gb-linked-name { margin: 0; font-size: .8125rem; font-weight: 700; }
.gb-linked-name .gb-hint { display: inline; margin-left: .25rem; font-weight: 400; }
</style>
