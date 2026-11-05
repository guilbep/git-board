// Where each field lives on each forge, as the field editor drafts it
// (docs/prd/fields-one-editor.md): the places per forge, a choice of
// target, a value's name there, and what the Fields save sends for them.
// The rules of what a place is are fieldtargets.ts's. Pure: node --test
// imports it.
import { fieldTarget, targetLabels, type ForgeMapping, type ForgeModel } from './fieldtargets.ts';
import type { ConfigField } from './configparse.ts';

// forge → field → its mapping (only overrides: a field on its default has none)
export type Places = Record<string, Record<string, ForgeMapping>>;

export function placesOf(forges: ForgeModel[]): Places {
  const out: Places = {};
  for (const f of forges) {
    out[f.name] = {};
    for (const [name, m] of Object.entries(f.fields ?? {})) out[f.name]![name] = { ...m, ...(m.values ? { values: { ...m.values } } : {}) };
  }
  return out;
}

// a mapping as the file takes it: its target, its column, its renames; none without a target
export function cleanMapping(m: ForgeMapping | undefined): ForgeMapping | null {
  if (!m?.to) return null;
  const out: ForgeMapping = { to: m.to };
  if (m.field) out.field = m.field;
  if (m.values && Object.keys(m.values).length) out.values = { ...m.values };
  return out;
}

const same = (a: ForgeMapping | null, b: ForgeMapping | null) => JSON.stringify(a) === JSON.stringify(b);

// a Project column's name when none is set: the known ones, else the field's, capitalized
const known: Record<string, string> = { status: 'Status', prio: 'Priority', mod: 'Module', est: 'Est. hours', sprint: 'Sprint', moscow: 'MoSCoW', milestone: 'Milestone' };
export const columnFor = (field: string) => known[field] ?? field.charAt(0).toUpperCase() + field.slice(1);

// a target picked: the default again with no renames is no override
// (null); a Project column keeps its name, or gets one
export function choose(prev: ForgeMapping | undefined, to: string, defaultTo: string, field: string): ForgeMapping | null {
  const values = prev?.values && Object.keys(prev.values).length ? prev.values : undefined;
  if (to === defaultTo && !values) return null;
  const out: ForgeMapping = { to };
  if (to === 'project') out.field = prev?.field ?? columnFor(field);
  if (values) out.values = values;
  return out;
}

// a value's name on the forge: empty, or the value itself, is no rename
export function rename(prev: ForgeMapping | undefined, to: string, value: string, on: string): ForgeMapping {
  const values = { ...prev?.values };
  if (!on.trim() || on === value) delete values[value];
  else values[value] = on;
  const out: ForgeMapping = { ...prev, to: prev?.to ?? to };
  if (Object.keys(values).length) out.values = values;
  else delete out.values;
  return out;
}

// what the save sends: per forge, the mappings that changed and those
// gone back to the default; a removed field's go with it on the server
export function placesBody(saved: ForgeModel[], draft: Places, removedFields: string[] = []): Record<string, { set: Record<string, ForgeMapping>; remove: string[] }> | undefined {
  const out: Record<string, { set: Record<string, ForgeMapping>; remove: string[] }> = {};
  for (const f of saved) {
    const was = f.fields ?? {};
    const now = draft[f.name] ?? {};
    const set: Record<string, ForgeMapping> = {};
    const remove: string[] = [];
    for (const name of new Set([...Object.keys(was), ...Object.keys(now)])) {
      if (removedFields.includes(name)) continue;
      const a = cleanMapping(was[name]);
      const b = cleanMapping(now[name]);
      if (same(a, b)) continue;
      if (b) set[name] = b;
      else if (a) remove.push(name);
    }
    if (Object.keys(set).length || remove.length) out[f.name] = { set, remove: remove.sort() };
  }
  return Object.keys(out).length ? out : undefined;
}

// the TOML a field's places write, for the editor's aside
export function placesToml(field: string, draft: Places): string {
  const lines: string[] = [];
  for (const [forge, fields] of Object.entries(draft)) {
    const m = cleanMapping(fields[field]);
    if (!m) continue;
    const parts = [`to = "${m.to}"`, ...(m.field ? [`field = "${m.field}"`] : [])];
    lines.push(`[forges.${forge}.fields]`, `${field} = { ${parts.join(', ')} }`);
    if (m.values) {
      lines.push('', `[forges.${forge}.fields.${field}.values]`);
      for (const [v, on] of Object.entries(m.values)) lines.push(`${v} = "${on}"`);
    }
    lines.push('');
  }
  return lines.join('\n').trim();
}

// the same places, whatever the keys' order and empty mappings
export function samePlaces(a: Places, b: Places): boolean {
  const norm = (p: Places) =>
    Object.keys(p)
      .sort()
      .map((f) => [f, Object.keys(p[f] ?? {}).sort().map((n) => [n, cleanMapping(p[f]![n])]).filter(([, m]) => m)]);
  return JSON.stringify(norm(a)) === JSON.stringify(norm(b));
}

// where a forge's fields live, in one line, for its row in Sync: the
// default first, by count; the others named ("5 as labels · prio, est in
// the Project · parent as sub-issues")
export function placesSummary(forge: ForgeModel, fields: Record<string, ConfigField>, order: string[], roles: string[], projectNumber: number): string {
  const by = new Map<string, string[]>();
  for (const name of order) {
    if (!fields[name]) continue;
    const to = fieldTarget(forge, fields, name, roles, projectNumber);
    by.set(to, [...(by.get(to) ?? []), name]);
  }
  const word = (to: string) => (to === 'project' ? 'the Project' : (targetLabels[to] ?? to).toLowerCase());
  const parts = [...by.entries()].sort((a, b) => b[1].length - a[1].length);
  return parts.map(([to, names], i) => (i === 0 && names.length > 2 ? `${names.length} as ${word(to)}s` : `${names.join(', ')} ${to === 'project' ? 'in' : 'as'} ${word(to)}`)).join(' · ');
}
