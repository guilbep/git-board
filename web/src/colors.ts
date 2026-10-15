// Field value colors, resolved by the server (config.Color): asked again
// with every load of the issues, so a new value gets its color.
import { ref } from 'vue';
import { getColors } from '@/api/board';

export const colors = ref<Record<string, Record<string, string>>>({});

export async function loadColors(): Promise<void> {
  try {
    colors.value = await getColors();
  } catch {
    // values stay plain
  }
}

// colorClass is the CSS class of a value's color (gitboard.css), '' for none.
export function colorClass(field: string, value: string): string {
  const c = colors.value[field]?.[value];
  return c ? `gb-c-${c}` : '';
}
