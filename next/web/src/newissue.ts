// The new issue dialog's field writes, once the issue is created: one PATCH
// per value, so a refused one names its field and the others still go in.
// A multi field's "a, b" is two writes (each adds a value); a single
// field's value is one.

export type FieldWrite = { name: string; value: string };

export function fieldWrites(values: Record<string, string>, isMulti: (name: string) => boolean): FieldWrite[] {
  const out: FieldWrite[] = [];
  for (const [name, raw] of Object.entries(values)) {
    const v = raw.trim();
    if (!v) continue;
    if (isMulti(name)) {
      for (const one of v.split(',').map((s) => s.trim()).filter(Boolean)) out.push({ name, value: one });
    } else out.push({ name, value: v });
  }
  return out;
}
