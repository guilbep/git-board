// The new issue dialog's fields as POST /api/issues's set: field=value
// each; a multi field's "a, b" is one +value each (each adds a value), a
// single field's value one assignment. Pure: node runs newissue.test.ts.

export function assignments(values: Record<string, string>, isMulti: (name: string) => boolean): string[] {
  const out: string[] = [];
  for (const [name, raw] of Object.entries(values)) {
    const v = raw.trim();
    if (!v) continue;
    if (isMulti(name)) {
      for (const one of v.split(',').map((s) => s.trim()).filter(Boolean)) out.push(`${name}=+${one}`);
    } else out.push(`${name}=${v}`);
  }
  return out;
}
