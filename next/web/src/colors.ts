// Field value colors (valuecolors.ts), from the fields' configured colors
// (GET /api/fields) and the values in use (GET /api/labels): asked again
// with the board's config, so a new value gets its color.
import { ref } from 'vue';
import { getFields, getLabels } from '@/api';
import { hexRules, valueColors } from '@/valuecolors';

export const colors = ref<Record<string, Record<string, string>>>({});
// the values in use, field → values: what the filter boxes suggest
// (viewfilter.suggest)
export const values = ref<Record<string, string[]>>({});

export async function loadColors(): Promise<void> {
  try {
    const [fields, labels] = await Promise.all([getFields(), getLabels('')]);
    const inUse: Record<string, string[]> = {};
    for (const l of labels) if (l.field) (inUse[l.field] ??= []).push(l.value);
    values.value = inUse;
    const all = { ...inUse };
    for (const [name, f] of Object.entries(fields)) all[name] = [...new Set([...(all[name] ?? []), ...(f.options ?? [])])];
    colors.value = valueColors(fields, all);
  } catch {
    // values stay plain
  }
  hexStyle.textContent = hexRules(colors.value);
}

// the classes of the #rrggbb colors (labels.hexRules)
const hexStyle = document.head.appendChild(document.createElement('style'));

// colorClass is the CSS class of a value's color (gitboard.css, its values
// in tokens/values.css), '' for none.
export function colorClass(field: string, value: string): string {
  const c = colors.value[field]?.[value];
  return !c ? '' : c.startsWith('#') ? `gb-c-x${c.slice(1)}` : `gb-c-${c}`;
}
