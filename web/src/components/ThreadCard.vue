<!-- A thread on the description (docs/design/time-flow, 6a): its comments,
     the root first with its anchor, then the reply row: Reply, and Resolve
     while it is open, Unresolve once it is resolved. -->
<template>
  <div class="gb-tcard">
    <CommentCard
      v-for="(c, i) in [thread.event, ...thread.replies]"
      :key="c.id"
      :data-event="c.id"
      :event="c"
      :forges="forges"
      :mine="c.author.id === meId"
      :save="save"
      :role="roleOf(fields, c.author.login)"
      :anchor="i === 0 && anchor ? anchorLabel(anchor, sections) : undefined"
      @anchor="emit('anchor', anchor!.slug)"
      @ref="(n, e) => emit('ref', n, e)"
    />
    <div class="gb-treply">
      <label :for="`r-${uid}`" class="gb-hint">Reply</label>
      <input :id="`r-${uid}`" v-model="text" class="epfl-input" :disabled="busy" @keydown.enter.prevent="send" />
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || !text.trim()" @click="send">Reply</button>
      <button v-if="!thread.resolved" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="resolve(thread.event.id, meId)">Resolve</button>
      <template v-else>
        <span class="gb-hint">{{ resolvedBy(thread, events) }}</span>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="resolve(thread.event.id, '')">Unresolve</button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useId } from 'vue';
import type { Forge, IssueEvent } from '@/api/board';
import type { Fields } from '@/fields';
import { resolvedBy, roleOf, type Item } from '@/flow';
import { anchorLabel, anchorOf, type Section } from '@/sections';
import CommentCard from '@/components/CommentCard.vue';

const props = defineProps<{
  thread: Item;
  events: IssueEvent[]; // the issue's: who resolved it, by name
  sections: Section[];
  meId: string;
  fields: Fields;
  forges?: Forge[] | null | undefined;
  busy: boolean;
  save: (id: string, body: string, base: string) => Promise<unknown>;
  reply: (root: string, body: string) => Promise<boolean>;
  resolve: (root: string, value: string) => Promise<boolean>; // "" reopens
}>();
const emit = defineEmits<{ anchor: [slug: string]; ref: [n: string, event: MouseEvent] }>();

const uid = useId();
const anchor = computed(() => anchorOf(props.thread.event.meta?.['thread:anchor']));
const text = ref('');
async function send() {
  if (!text.value.trim()) return;
  if (await props.reply(props.thread.event.id, text.value)) text.value = '';
}
</script>

<style scoped>
.gb-tcard { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
.gb-treply { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 10px 14px; background: var(--bg-subtle); border: 1px solid var(--border-subtle); }
.gb-treply .epfl-input { flex: 1 1 200px; width: auto; }
</style>
