// Every field's type (config.TypeOf), as the last GET /api/config/fields
// answered: what ValueMark needs to give icons only to fields whose values
// are states (a status, a priority, a kind), not names (a sprint, a
// person, a date). Kept by getFields, read anywhere.
import { ref } from 'vue';

export const fieldTypes = ref<Record<string, string>>({});

// a value's icon tells where it stands; a sprint called "qa-fixes" or a
// person isn't a state
export function iconsFor(field: string): boolean {
  const t = fieldTypes.value[field];
  return t === undefined || t === 'select';
}
