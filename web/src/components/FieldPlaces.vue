<!-- Where a field lives on each forge, in its editor
     (docs/prd/fields-one-editor.md): a row per forge, the target (the
     type's default marked; an override in blue), the Project column when
     the target is the Project, and the values' names there. It edits the
     Fields draft's places; the Fields save writes them with the field. -->
<template>
  <section class="fp" :aria-labelledby="`fp-${field}`">
    <h3 :id="`fp-${field}`" class="fp-title">Where it lives</h3>
    <p class="fp-hint">On each forge. <b>Default</b> follows the field’s type and writes nothing.</p>
    <div v-for="r in rows" :key="r.forge.name" class="fp-row">
      <span class="fp-forge"><GbIcon :name="r.forge.kind === 'gitlab' ? 'gitlab' : 'github'" small />{{ forgeTitle(r.forge) }}</span>
      <select class="fp-select" :class="{ 'fp-override': r.target !== r.defaultTarget }" :value="r.target" :aria-label="`${field} lives on ${forgeTitle(r.forge)} in`" @change="pick(r, ($event.target as HTMLSelectElement).value)">
        <option v-for="t in r.targets" :key="t.id" :value="t.id">{{ targetLabels[t.id] ?? t.label }}{{ t.id === r.defaultTarget ? ' (default)' : '' }}</option>
      </select>
      <span class="fp-detail">
        <template v-if="r.target === 'project'">
          column
          <input class="fp-input" :value="r.mapping?.field ?? columnFor(field)" spellcheck="false" :aria-label="`The Project column of ${field} on ${forgeTitle(r.forge)}`" @change="setColumn(r, ($event.target as HTMLInputElement).value)" />
        </template>
        <template v-else-if="r.target !== r.savedTarget">moves from {{ targetLabels[r.savedTarget] ?? r.savedTarget }}</template>
      </span>
      <details v-if="options.length && keepsNames(r.target)" class="fp-names" :open="r.renamed > 0 ? true : undefined">
        <summary>Names on {{ forgeTitle(r.forge) }}<span v-if="r.renamed" class="fp-muted"> · {{ r.renamed }} renamed</span></summary>
        <div class="fp-name-rows">
          <template v-for="v in options" :key="v">
            <code>{{ v }}</code>
            <input class="fp-input" :value="r.mapping?.values?.[v] ?? ''" :placeholder="v" spellcheck="false" :aria-label="`${v}'s name on ${forgeTitle(r.forge)}`" @input="setName(r, v, ($event.target as HTMLInputElement).value)" />
          </template>
        </div>
        <p class="fp-muted">Empty: the value keeps its own name.</p>
      </details>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { ConfigField } from '@/configparse';
import { fieldTarget, targetLabels, targetsFor, typeOf, type ForgeMapping, type ForgeModel } from '@/fieldtargets';
import { choose, columnFor, rename, type Places } from '@/fieldplaces';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{
  field: string;
  fields: Record<string, ConfigField>; // the model's, the field as drafted among them
  forges: ForgeModel[]; // as saved
  places: Places; // the draft's
  roles: string[];
  projectNumber: number;
  options: string[];
}>();
const emit = defineEmits<{ set: [forge: string, field: string, mapping: ForgeMapping | null] }>();

const kindName = (k: string) => (k === 'gitlab' ? 'GitLab' : 'GitHub');
const forgeTitle = (f: ForgeModel) => (f.name === f.kind ? kindName(f.kind) : `${f.name} (${kindName(f.kind)})`);
// the targets that keep the values' names: a label spells them, a Project column names them
const keepsNames = (to: string) => to === 'project' || to === 'label' || to === 'scoped-label';

const rows = computed(() =>
  props.forges.map((forge) => {
    const mapping = props.places[forge.name]?.[props.field];
    const drafted: ForgeModel = { ...forge, fields: props.places[forge.name] ?? {} };
    const type = typeOf(props.fields, props.field, props.roles);
    return {
      forge,
      mapping,
      target: fieldTarget(drafted, props.fields, props.field, props.roles, props.projectNumber),
      savedTarget: fieldTarget(forge, props.fields, props.field, props.roles, props.projectNumber),
      defaultTarget: fieldTarget({ ...forge, fields: {} }, props.fields, props.field, props.roles, props.projectNumber),
      targets: targetsFor(forge.kind, type, props.field),
      renamed: Object.keys(mapping?.values ?? {}).length,
    };
  }),
);
type Row = (typeof rows.value)[number];

function pick(r: Row, to: string) {
  emit('set', r.forge.name, props.field, choose(r.mapping, to, r.defaultTarget, props.field));
}
function setColumn(r: Row, field: string) {
  emit('set', r.forge.name, props.field, { ...r.mapping, to: 'project', field: field.trim() || columnFor(props.field) });
}
function setName(r: Row, value: string, on: string) {
  const m = rename(r.mapping, r.target, value, on);
  // back to the default target with no rename left: no override
  emit('set', r.forge.name, props.field, m.to === r.defaultTarget && !m.values && !m.field ? null : m);
}

</script>

<style scoped>
.fp { display: grid; gap: .375rem; padding-top: .75rem; border-top: var(--border-w) solid var(--border-subtle); }
.fp-title { margin: 0; font-size: .9375rem; font-weight: var(--w-bold); }
.fp-hint, .fp-muted { margin: 0; color: var(--fg-muted); font-size: .8125rem; }
.fp-row { display: grid; grid-template-columns: 8.5rem minmax(10rem, 14rem) minmax(0, 1fr); align-items: center; gap: .375rem .875rem; padding: .5rem 0; border-bottom: var(--border-w) solid var(--border-subtle); }
.fp-forge { display: inline-flex; align-items: center; gap: .375rem; font-size: .875rem; }
.fp-select { min-height: 2.125rem; padding: 0 .5rem; border: var(--border-w) solid var(--border-strong); background: var(--bg); color: var(--fg); font: inherit; font-size: .875rem; }
.fp-select.fp-override { border-color: var(--info-fg); color: var(--info-fg); }
.fp-detail { display: inline-flex; align-items: center; gap: .375rem; min-width: 0; color: var(--fg-muted); font-size: .8125rem; }
.fp-input { min-width: 0; width: 100%; max-width: 12rem; box-sizing: border-box; height: 1.875rem; padding: 0 .5rem; border: var(--border-w) solid var(--border-strong); background: var(--bg); color: var(--fg); font: inherit; font-size: .8125rem; }
.fp-names, .fp-move { grid-column: 1 / -1; }
@container (max-width: 34rem) { .fp-row { grid-template-columns: minmax(0, 1fr); } }
.fp-names summary { cursor: pointer; font-size: .8125rem; font-weight: var(--w-bold); }
.fp-name-rows { display: grid; grid-template-columns: max-content minmax(0, 14rem); align-items: center; gap: .375rem .75rem; margin: .5rem 0 .25rem; }
.fp-move { margin: 0; padding: .375rem .625rem; border-left: .1875rem solid var(--warning); background: var(--warning-bg); font-size: .8125rem; }
</style>
