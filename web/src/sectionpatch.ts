// What a settings tab's save sends: its section's draft as PATCH
// /api/config's keys (docs/rewrite/api.md, Settings). A field or a view is
// a whole table, its name null when the draft dropped it; the other
// sections send each of their keys, a key emptied removed when the file
// had it, so a key the form doesn't show stays (its tests: a form keeps
// what it doesn't show). Pure: node runs sectionpatch.test.ts on it.

type Table = Record<string, unknown>;

// empty: nothing to write for the key (undefined, null, '', [] or {})
function empty(v: unknown): boolean {
  if (v === undefined || v === null || v === '') return true;
  if (Array.isArray(v)) return v.length === 0;
  if (typeof v === 'object') return Object.keys(v).length === 0;
  return false;
}

// clean is v without its empty keys, deep (a table the file can take)
export function clean(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(clean);
  if (v && typeof v === 'object') {
    const out: Table = {};
    for (const [k, x] of Object.entries(v as Table)) {
      const c = clean(x);
      if (!empty(c)) out[k] = c;
    }
    return out;
  }
  return v;
}

// the keys of a section whose '' is a value, not "no key": follow = "" says
// no environment follows the trunk
const emptyIsValue: Record<string, string[]> = { release: ['follow'] };

// keysOf: each key of body under section; an empty one removes the key
// when the file has it, and is left out when it hasn't
function keysOf(section: string, body: Table, had: Table): Table {
  const out: Table = {};
  for (const [k, v] of Object.entries(body)) {
    const key = `${section}.${k}`;
    if (v === '' && emptyIsValue[section]?.includes(k)) out[key] = '';
    else if (!empty(clean(v))) out[key] = clean(v);
    else if (k in had) out[key] = null;
  }
  return out;
}

const tableOf = (v: unknown): Table => (v && typeof v === 'object' && !Array.isArray(v) ? (v as Table) : {});

// a forge the draft maps fields on: found in the remotes (no table), it
// gets its table too
export interface ForgeOf {
  name: string;
  kind: string;
  project: string;
  host: string;
}

// sectionPatch is the PATCH set for one section's save; raw is the file as
// the draft read it
export function sectionPatch(section: string, body: unknown, model: { raw: Table; forges?: ForgeOf[] }): Table {
  const b = tableOf(body);
  const had = tableOf(model.raw[section]);
  switch (section) {
    case 'fields':
    case 'views': {
      // {set: {name: table}, remove: [names], forges?: {forge: {set, remove}}}
      const out: Table = {};
      for (const [name, v] of Object.entries(tableOf(b.set))) out[`${section}.${name}`] = clean(v);
      for (const name of (b.remove as string[] | undefined) ?? []) if (name in had) out[`${section}.${name}`] = null;
      const tables = tableOf(model.raw.forges);
      for (const [forge, change] of Object.entries(tableOf(b.forges))) {
        const c = tableOf(change);
        const mapped = tableOf(tableOf(tables[forge]).fields);
        if (!(forge in tables)) {
          const f = model.forges?.find((x) => x.name === forge);
          if (f) Object.assign(out, clean({ [`forges.${forge}.kind`]: f.kind, [`forges.${forge}.project`]: f.project }));
        }
        for (const [field, m] of Object.entries(tableOf(c.set))) out[`forges.${forge}.fields.${field}`] = clean(m);
        for (const field of (c.remove as string[] | undefined) ?? []) if (field in mapped) out[`forges.${forge}.fields.${field}`] = null;
      }
      return out;
    }
    case 'cycle': { // {on, cycle}: off removes the table
      if (!b.on) return model.raw.cycle === undefined ? {} : { cycle: null };
      const { on: _, ...cycle } = tableOf(b.cycle); // the model's own mark, not a key
      return model.raw.cycle === undefined ? { cycle: clean(cycle) } : keysOf('cycle', cycle, had);
    }
    case 'roles': {
      const out: Table = {};
      for (const [k, v] of Object.entries(b)) if (!empty(v)) out[`roles.${k}`] = v;
      for (const k of Object.keys(had)) if (!(`roles.${k}` in out)) out[`roles.${k}`] = null;
      return out;
    }
    default:
      return keysOf(section, b, had);
  }
}
