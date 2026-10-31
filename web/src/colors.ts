// Field value colors, resolved by the server (config.Color): asked again
// with every load of the issues, so a new value gets its color.
import { ref } from 'vue';
import { getColors } from '@/api/board';
import { hexRules } from '@/labels';

export const colors = ref<Record<string, Record<string, string>>>({});

export async function loadColors(): Promise<void> {
  try {
    colors.value = await getColors();
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
