// How a card names an issue: its project number (CO2-3075), else its
// number on the first forge it is on (#2518), which its shown title leaves
// out, then its id. Pure: node runs idlabel.test.ts on it.
import type { Forge } from './api/types.ts';

// forgeNumbers are an issue's numbers on the forges
export const forgeNumbers = (i: { forges?: Forge[] | undefined }): string[] => (i.forges ?? []).flatMap((f) => (f.number ? [f.number] : []));

export function idLabel(i: { id: string; number?: string | undefined; forges?: Forge[] | undefined }, n = 7): string {
  const forge = forgeNumbers(i)[0];
  const number = i.number ?? (forge ? `#${forge}` : '');
  return number ? `${number} · ${i.id.slice(0, n)}` : i.id.slice(0, n);
}
