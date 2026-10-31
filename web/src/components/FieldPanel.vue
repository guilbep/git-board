<!-- The fields, as a dense list (docs/design/issue-head, 16a): one row
     each, its name and its value; a row opens its editor in place, and
     Back returns to the list. A single name opens its editor at once. -->
<template>
  <div class="gb-fp">
    <template v-if="open">
      <button v-if="names.length > 1" type="button" class="gb-fp-back" @click="open = ''"><GbIcon name="back" small /> All fields</button>
      <p v-if="names.length > 1" class="gb-fp-name">{{ displayName(open) }}</p>
      <FieldEditor :id="id" :name="open" :fields="fields" :labels="labels" :refs="refs[open] ?? []" :refs-known="refsKnown" @changed="emit('changed')" @done="names.length > 1 ? (open = '') : emit('done')" @open="(id) => emit('open', id)" />
    </template>
    <ul v-else class="gb-fp-rows">
      <li v-for="n in names" :key="n">
        <button type="button" class="gb-fp-row" @click="open = n">
          <span class="gb-fp-label">{{ displayName(n) }}</span>
          <span class="gb-fp-value" :class="{ 'is-unset': !said(n) }">
            <ValueMark v-if="valuesOf(n).length === 1 && typeOf(n, fields) !== 'person'" :field="n" :value="valuesOf(n)[0]!" />
            {{ said(n) || inheritedOf(n) || 'Not set' }}
          </span>
          <GbIcon name="forward" small class="gb-fp-go" />
        </button>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import type { IssueRef } from '@/api/board';
import { displayName, fieldValues, formatValue, typeOf, type Fields } from '@/fields';
import { me } from '@/me';
import FieldEditor from '@/components/FieldEditor.vue';
import GbIcon from '@/components/GbIcon.vue';
import ValueMark from '@/components/ValueMark.vue';

// refs: the issues each issue field names, as the head read them (IssueHead)
const props = defineProps<{ id: string; names: string[]; fields: Fields; labels: string[]; refs: Record<string, IssueRef[]>; refsKnown: boolean }>();
const emit = defineEmits<{ changed: []; done: []; open: [id: string] }>();

const open = ref('');
watch(() => props.names.join(' '), () => (open.value = props.names.length === 1 ? props.names[0]! : ''), { immediate: true });

const valuesOf = (n: string) => fieldValues(props.labels, n);
const nameOf = (login: string) => me.value?.identities?.find((p) => p.login?.toLowerCase() === login.replace(/^@/, '').toLowerCase())?.name ?? login;
const said = (n: string) => valuesOf(n).map((v) => (typeOf(n, props.fields) === 'person' ? nameOf(v) : formatValue(n, v, props.fields))).join(', ');
const inheritedOf = (n: string) => (props.fields[n]?.inherits ? `${nameOf(props.fields[n]!.inherits!)} · project` : '');
</script>
