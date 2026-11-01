// A field card's two renderings: the form and the TOML. fieldToToml
// serializes one field as a [fields.<name>] snippet; tomlToField parses a
// snippet back — through smol-toml, since hand-parsing TOML is not worth
// it — refusing anything the card's form cannot hold, so a switch back
// never drops an edit silently. The rest of the file is none of this'.
import { parse } from 'smol-toml';

// the field as the file holds it (FieldsSettings keeps its rows as pairs)
export type FieldToml = {
  single?: boolean | undefined;
  numeric?: boolean | undefined;
  type?: string | undefined;
  unit?: string | undefined;
  options?: string[] | undefined;
  colors?: Record<string, string> | undefined;
  descriptions?: Record<string, string> | undefined;
  project?: Record<string, string> | undefined;
};

export type ParsedField = { ok: true; name?: string | undefined; field: FieldToml } | { ok: false; error: string };

const bare = (k: string) => /^[A-Za-z0-9_-]+$/.test(k);
// JSON.stringify quotes and escapes a string exactly as a TOML basic one
const str = (v: string) => JSON.stringify(v);
const tomlKey = (k: string) => (bare(k) ? k : str(k));

const isStringMap = (v: unknown): v is Record<string, string> =>
  typeof v === 'object' && v !== null && Object.values(v).every((x) => typeof x === 'string');

// fieldToToml renders one field's table; an unnamed field serializes as
// new-field, which the switch back adopts as its name
export function fieldToToml(name: string, f: FieldToml): string {
  const lines = [`[fields.${tomlKey(name.trim() || 'new-field')}]`];
  if (f.single !== undefined) lines.push(`single = ${f.single}`);
  if (f.numeric !== undefined) lines.push(`numeric = ${f.numeric}`);
  if (f.type) lines.push(`type = ${str(f.type)}`);
  if (f.unit) lines.push(`unit = ${str(f.unit)}`);
  if (f.options?.length) lines.push(`options = [${f.options.map(str).join(', ')}]`);
  for (const map of ['colors', 'descriptions', 'project'] as const) {
    const m = f[map];
    if (m && Object.keys(m).length) {
      const body = Object.entries(m)
        .map(([k, v]) => `${tomlKey(k)} = ${str(v)}`)
        .join(', ');
      lines.push(`${map} = { ${body} }`);
    }
  }
  return lines.join('\n') + '\n';
}

// tomlToField reads a snippet: a [fields.<name>] table — one field, whose
// header may rename it — or a bare key = value table for the field at
// hand. Unknown keys are an error, not a silent drop.
export function tomlToField(text: string): ParsedField {
  let doc: unknown;
  try {
    doc = parse(text);
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
  if (typeof doc !== 'object' || doc === null) return { ok: false, error: 'Not a TOML table.' };
  let name: string | undefined;
  let body: unknown = doc;
  const fields = (doc as Record<string, unknown>)['fields'];
  if (fields !== undefined) {
    const keys = typeof fields === 'object' && fields !== null ? Object.keys(fields) : [];
    const first = keys[0];
    if (first === undefined) return { ok: false, error: 'No field table: keep one [fields.<name>].' };
    if (keys.length > 1) return { ok: false, error: 'One field per snippet: this holds several tables.' };
    name = first;
    body = (fields as Record<string, unknown>)[first];
  }
  if (typeof body !== 'object' || body === null) return { ok: false, error: 'The field is not a table of keys.' };

  const field: FieldToml = {};
  const unknown: string[] = [];
  for (const [k, v] of Object.entries(body)) {
    if (k === 'single' || k === 'numeric') {
      if (typeof v !== 'boolean') return { ok: false, error: `${k} = ${JSON.stringify(v)} — a true or false was expected.` };
      field[k] = v;
    } else if (k === 'type' || k === 'unit') {
      if (typeof v !== 'string') return { ok: false, error: `${k} = ${JSON.stringify(v)} — a quoted string was expected.` };
      field[k] = v;
    } else if (k === 'options') {
      if (!Array.isArray(v) || !v.every((x) => typeof x === 'string')) {
        return { ok: false, error: 'options is a list of quoted strings.' };
      }
      field.options = v;
    } else if (k === 'colors' || k === 'descriptions' || k === 'project') {
      if (!isStringMap(v)) return { ok: false, error: `${k} maps values to quoted strings.` };
      field[k] = Object.fromEntries(Object.entries(v)); // smol-toml hands out null-prototype maps
    } else {
      unknown.push(k);
    }
  }
  if (unknown.length) {
    return { ok: false, error: `Unknown key${unknown.length > 1 ? 's' : ''} ${unknown.join(', ')} — a field knows single, numeric, type, unit, options, colors, descriptions, project.` };
  }
  return { ok: true, name, field };
}
