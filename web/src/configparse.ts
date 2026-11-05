// The project file's sections, typed, as the settings' forms read them:
// the text of GET /api/config parsed here (v1's server answered a model,
// GET /api/config/model; v2 answers the text and its keys), with the forges
// GET /api/sync finds and the features GET /api/board says are on. Pure:
// node runs configparse.test.ts on it.
import { parse } from 'smol-toml';
import type { ForgeModel } from './fieldtargets.ts';
import { known } from './experimental.ts';

// a saved view, [views.<name>]
export interface ViewConfig {
  layout: string;
  filter: string;
  group: string;
  sort: string;
  columns?: string[] | undefined; // a table's columns, in order (tablecols.ts); none: the default
  lanes?: string[] | undefined; // a board's lanes, in order (lanes.ts); none: every value
  limits?: Record<string, number> | undefined; // a lane's limit (work in progress)
}

export interface ConfigField {
  single?: boolean;
  options?: string[] | null;
  numeric?: boolean;
  type?: string | undefined; // select, iteration, person, number, date, issue, text; '': guessed
  unit?: string | undefined; // a number's: h
  project?: Record<string, string> | undefined; // the GitHub Project's name for a value
  aliases?: Record<string, string> | undefined; // a value's old names, folded into the current one
  colors?: Record<string, string> | undefined; // a palette name or #rrggbb
  descriptions?: Record<string, string> | undefined; // GitHub's, 100 characters at most
}

export interface ConfigLabels {
  list: string[];
  colors?: Record<string, string> | undefined;
  descriptions?: Record<string, string> | undefined;
}

export interface ConfigSprint {
  slug: string;
  title: string;
  start: string; // RFC 3339 as the server holds it; a form edits YYYY-MM-DD
}

export interface ConfigSuite {
  run?: string[] | null; // an argv, never a shell line
  files?: string[] | null; // git glob pathspecs of what it reads
  where?: string | undefined; // worktree (default) or ci
  dir?: string | undefined; // from the checkout's root
  env?: Record<string, string> | undefined;
  tools?: string[][] | null; // version commands: their output names the toolchain
}

export interface ConfigRelease {
  remote?: string | undefined;
  environments?: string[] | null;
  follow?: string | null | undefined; // the environment that follows the trunk
  checks?: string | undefined;
  images?: { name: string; context: string }[] | null;
  chart?: { path: string; registry: string } | null;
  version_values?: string[] | null;
  builder?: string | undefined;
  deploy?: { repo: string; path: string; registry?: string }[] | null;
  binaries?: string[] | null;
  platforms?: string[] | null;
  build?: string | undefined;
}

export interface ConfigModel {
  hash: string; // the file's version: what a save sends back (If-Match)
  exists: boolean;
  raw: Record<string, unknown>; // the whole file, parsed: what a save compares with
  // [experimental] features as the file lists them; local, this clone's
  // board.experimental when set (it wins: v2's API doesn't say, null); on,
  // what runs here (GET /api/board); known, every feature
  experimental: { features: string[]; local: string[] | null; on: string[]; known: readonly { name: string; title: string; description: string }[] };
  error?: string; // the file doesn't parse: the forms show it, Advanced fixes it
  fields: Record<string, ConfigField>;
  forges: ForgeModel[]; // the [forges.<name>] tables, mappings with them
  github_project?: { owner: string; number: number; write?: boolean };
  labels: ConfigLabels;
  views: Record<string, ViewConfig>;
  view_order: string[]; // the file's order: the tabs', the first is the landing one
  field_order: string[]; // the file's order likewise: the fields page's list
  sprints: { length_days: number; start_weekday: string; list: ConfigSprint[] };
  roles: Record<string, string>;
  branches: { base?: string | undefined; name?: string | undefined; kinds?: Record<string, string> | undefined };
  cycle: {
    on: boolean; // the file has a [cycle] table
    ready?: string | undefined;
    'in-progress'?: string[] | null;
    'in-review'?: string[] | null;
    validation?: string[] | null;
    plans?: string | undefined;
    'plans-in-pr'?: boolean | null | undefined;
    'test-files'?: string[] | null;
    criteria?: string[] | null;
    steps?: string[] | null;
    kinds?: Record<string, string> | undefined;
    risk?: string | undefined;
    plan?: string | undefined;
    review?: string | undefined;
    spec?: string | undefined;
    signoffs?: Record<string, string> | undefined;
    tests?: Record<string, ConfigSuite> | undefined;
    checks?: Record<string, ConfigSuite> | undefined;
    signers?: string | undefined;
  };
  repos: { list: string[]; assignees: string }; // assignees: how the forges hold them; native, labels
  uploads: { endpoint?: string | undefined; public_url?: string | undefined; prefix?: string | undefined; region?: string | undefined };
  release: ConfigRelease;
}


type Table = Record<string, unknown>;
const table = (v: unknown): Table => (v && typeof v === 'object' && !Array.isArray(v) && !(v instanceof Date) ? (v as Table) : {});
const strings = (v: unknown): string[] => (Array.isArray(v) ? v.map(String) : []);
const str = (v: unknown): string | undefined => (typeof v === 'string' ? v : undefined);
const record = (v: unknown): Record<string, string> | undefined => {
  const t = table(v);
  return Object.keys(t).length ? Object.fromEntries(Object.entries(t).map(([k, x]) => [k, String(x)])) : undefined;
};
// a date as the forms edit it, YYYY-MM-DD; a time as RFC 3339
const dateOf = (v: unknown): string => {
  if (v instanceof Date) return v.toISOString().slice(0, 10);
  const s = String(v ?? '');
  return /^\d{4}-\d{2}-\d{2}/.test(s) ? s.slice(0, 10) : s;
};

// the tables' order in the text: [fields.a] before [fields.b], or the
// keys of an inline table
export function tableOrder(text: string, parent: string, parsed: Table): string[] {
  const out: string[] = [];
  const re = new RegExp(`^\\s*\\[\\s*${parent.replace('.', '\\.')}\\.([A-Za-z0-9_-]+)\\s*\\]`, 'gm');
  for (const m of text.matchAll(re)) if (!out.includes(m[1]!)) out.push(m[1]!);
  for (const k of Object.keys(parsed)) if (!out.includes(k)) out.push(k);
  return out;
}

function fieldOf(v: unknown): ConfigField {
  const t = table(v);
  const f: ConfigField = {};
  if (t.single !== undefined) f.single = !!t.single;
  if (t.options !== undefined) f.options = strings(t.options);
  if (t.numeric !== undefined) f.numeric = !!t.numeric;
  for (const k of ['type', 'unit'] as const) if (str(t[k]) !== undefined) f[k] = str(t[k]);
  for (const k of ['project', 'aliases', 'colors', 'descriptions'] as const) if (record(t[k])) f[k] = record(t[k]);
  return f;
}

function viewOf(v: unknown): ViewConfig {
  const t = table(v);
  const out: ViewConfig = { layout: str(t.layout) ?? '', filter: str(t.filter) ?? '', group: str(t.group) ?? '', sort: str(t.sort) ?? '' };
  if (Array.isArray(t.columns)) out.columns = strings(t.columns);
  if (Array.isArray(t.lanes)) out.lanes = strings(t.lanes);
  const limits = table(t.limits);
  if (Object.keys(limits).length) out.limits = Object.fromEntries(Object.entries(limits).map(([k, n]) => [k, Number(n)]));
  return out;
}

// forgesOf: the [forges.<name>] tables, then those the server found in the
// remotes (discovered), each with its field mappings
function forgesOf(raw: Table, found: { name: string; kind: string; project: string; host: string; sync: boolean; discovered?: boolean }[]): ForgeModel[] {
  const tables = table(raw.forges);
  const out: ForgeModel[] = Object.entries(tables).map(([name, v]) => {
    const t = table(v);
    const seen = found.find((f) => f.name === name);
    const fields = table(t.fields);
    return {
      name,
      kind: str(t.kind) ?? seen?.kind ?? '',
      project: str(t.project) ?? str(t.repo) ?? seen?.project ?? '',
      host: str(t.host) ?? seen?.host ?? '',
      sync: t.sync === undefined ? (seen?.sync ?? true) : !!t.sync,
      fields: Object.keys(fields).length
        ? Object.fromEntries(Object.entries(fields).map(([k, m]) => {
            const x = table(m);
            return [k, { to: str(x.to), field: str(x.field), values: record(x.values) ?? null }];
          }))
        : null,
    };
  });
  for (const f of found) if (!out.some((o) => o.name === f.name)) out.push({ ...f, fields: null });
  return out;
}

export interface ModelInputs {
  text: string;
  version: string;
  exists: boolean;
  forges?: { name: string; kind: string; project: string; host: string; sync: boolean; discovered?: boolean }[];
  on?: string[]; // the experimental features on (GET /api/board)
}

// modelOf is the forms' model of the file; a file that doesn't parse is
// an empty one with its error
export function modelOf(i: ModelInputs): ConfigModel {
  let raw: Table = {};
  let error = '';
  try {
    raw = parse(i.text) as Table;
  } catch (e) {
    error = e instanceof Error ? e.message : String(e);
  }
  const fields = table(raw.fields);
  const views = table(raw.views);
  const sprints = table(raw.sprints);
  const cycle = table(raw.cycle);
  const release = table(raw.release);
  const labels = table(raw.labels);
  const branches = table(raw.branches);
  const repos = table(raw.repos);
  const uploads = table(raw.uploads);
  const project = table(raw['github-project']);
  const fieldOrder = tableOrder(i.text, 'fields', fields);
  const viewOrder = tableOrder(i.text, 'views', views);
  const model: ConfigModel = {
    hash: i.version,
    exists: i.exists,
    raw,
    experimental: { features: strings(table(raw.experimental).features), local: null, on: i.on ?? [], known },
    fields: Object.fromEntries(fieldOrder.map((n) => [n, fieldOf(fields[n])])),
    forges: forgesOf(raw, i.forges ?? []),
    labels: { list: strings(labels.list), colors: record(labels.colors), descriptions: record(labels.descriptions) },
    views: Object.fromEntries(viewOrder.map((n) => [n, viewOf(views[n])])),
    view_order: viewOrder,
    field_order: fieldOrder,
    sprints: {
      length_days: Number(sprints.length_days ?? 14),
      start_weekday: str(sprints.start_weekday) ?? '',
      list: (Array.isArray(sprints.list) ? sprints.list : []).map((s) => {
        const t = table(s);
        return { slug: str(t.slug) ?? '', title: str(t.title) ?? '', start: dateOf(t.start) };
      }),
    },
    roles: record(raw.roles) ?? {},
    branches: { base: str(branches.base), name: str(branches.name), kinds: record(branches.kinds) },
    cycle: { ...(cycle as ConfigModel['cycle']), on: raw.cycle !== undefined },
    repos: { list: strings(repos.list), assignees: str(repos.assignees) ?? '' },
    uploads: { endpoint: str(uploads.endpoint), public_url: str(uploads.public_url), prefix: str(uploads.prefix), region: str(uploads.region) },
    release: release as ConfigRelease,
  };
  if (project.owner !== undefined) model.github_project = { owner: String(project.owner), number: Number(project.number ?? 0), write: !!project.write };
  if (error) model.error = error;
  return model;
}
