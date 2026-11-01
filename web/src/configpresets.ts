// The presets: the project file's ready-made elements, drawn from the
// default file (internal/config/default.git-board.toml) and the guide's
// examples, each with its one-line why. A preset menu adds and selects; it
// never overwrites silently — a name the section already has comes back
// with -2, as the board's "Save as new view" does.
import type { ConfigField, ConfigLabels, ConfigSprint, ViewConfig } from '@/api/board';

export type Preset<T> = { label: string; why: string; value: T };

// takeUnique is preset's value under a name the list doesn't hold yet:
// name, then name-2, name-3…
export function takeUnique<T extends string>(want: T, taken: string[]): T {
  if (!taken.includes(want)) return want;
  for (let i = 2; ; i++) {
    const next = `${want}-${i}` as T;
    if (!taken.includes(next)) return next;
  }
}

export const fieldPresets: Preset<{ name: string; field: ConfigField }>[] = [
  {
    label: 'status',
    why: 'the workflow, in order: the board columns, git board move --next; the last closes',
    value: { name: 'status', field: { single: true, options: ['backlog', 'ready', 'in-progress', 'in-review', 'ready-for-validation', 'validated'] } },
  },
  {
    label: 'prio',
    why: 'the priority: p0 first in the sorts, red in the palette',
    value: { name: 'prio', field: { single: true, options: ['p0', 'p1', 'p2', 'p3'] } },
  },
  {
    label: 'est',
    why: 'the estimate in hours: numeric, so 3.5 sorts between 3 and 4',
    value: { name: 'est', field: { single: true, numeric: true, options: ['0.5', '1', '2', '4', '8', '16'] } },
  },
  {
    label: 'hold',
    why: 'on hold, beside the status, never a status: blocked, or paused',
    value: {
      name: 'hold',
      field: {
        single: true,
        options: ['blocked', 'paused'],
        descriptions: { blocked: 'waiting for something outside the team, or for another issue', paused: 'deprioritized, or its developer away for a while' },
      },
    },
  },
  {
    label: 'moscow',
    why: 'is it promised for its milestone: must, should, could, wont',
    value: { name: 'moscow', field: { single: true, options: ['must', 'should', 'could', 'wont'], project: { wont: "Won't" } } },
  },
  {
    label: 'kind',
    why: 'the kinds as kind: labels, one per issue: a column, a group, a filter',
    value: {
      name: 'kind',
      field: {
        single: true,
        options: ['bug', 'feat', 'polish', 'docs', 'refactor', 'perf', 'test', 'style', 'chore', 'design'],
        descriptions: {
          bug: 'behaviour that was specified or expected, restored',
          feat: 'a new capability, or a deliberate change of behaviour',
          polish: 'looks and wording: UI, text, layout, colours; no new capability',
          docs: 'documentation only',
          refactor: 'code structure; behaviour unchanged',
          perf: 'speed or footprint; behaviour unchanged',
          test: 'tests only',
          style: 'code formatting only; rarely an issue',
          chore: 'dependencies, CI, tooling, upgrades, housekeeping',
          design: 'design work (Figma), usually a sub-issue of the feat it designs',
        },
      },
    },
  },
  { label: 'assignee', why: 'who works on it: the forges’ assignees, several at once', value: { name: 'assignee', field: { single: false } } },
  { label: 'due', why: 'a date: red once past, amber within a week, “3 days late”', value: { name: 'due', field: { single: true, type: 'date' } } },
  { label: 'points', why: 'a number with its unit, pt here', value: { name: 'points', field: { single: true, type: 'number', unit: 'pt' } } },
  { label: 'parent', why: 'another issue: a link both ways, Sub-issues on it, Blocks here', value: { name: 'parent', field: { single: true, type: 'issue' } } },
];

export const viewPresets: Preset<{ name: string; view: ViewConfig }>[] = [
  { label: 'open', why: 'every open issue, as a forge’s issues page: one click from its kanban', value: { name: 'open', view: { layout: 'table', filter: 'is:open', group: '', sort: 'prio asc' } } },
  { label: 'backlog', why: 'the open issues still waiting or ready, priority first', value: { name: 'backlog', view: { layout: 'table', filter: 'is:open status:backlog,ready', group: '', sort: 'prio asc' } } },
  { label: 'board', why: 'the kanban: open issues, lanes the status', value: { name: 'board', view: { layout: 'board', filter: 'is:open', group: 'status', sort: '' } } },
  { label: 'assigned-to-me', why: 'yours only', value: { name: 'assigned-to-me', view: { layout: 'table', filter: 'assignee:@me is:open', group: 'status', sort: '' } } },
  { label: 'current-sprint', why: 'today’s sprint on a board, grouped by status', value: { name: 'current-sprint', view: { layout: 'board', filter: 'sprint:@current is:open', group: 'status', sort: '' } } },
];

export const labelsPreset: Preset<ConfigLabels> = {
  label: 'plain labels',
  why: 'yes-or-no flags that stack, that no field holds: needs-design, regression, security',
  value: { list: ['needs-design', 'regression', 'security'] },
};

// the sprints' defaults: a fortnight from Monday, an empty list — + Sprint
// fills it
export const sprintsPreset: { length_days: number; start_weekday: string } = { length_days: 14, start_weekday: 'monday' };

// the branches' kinds, the default file's: bug's branches say fix
export const branchKindsPreset: Preset<Record<string, string>> = {
  label: 'the usual kinds',
  why: 'bug → fix, feat → feat, docs, test, chore, design and the rest as they are',
  value: { bug: 'fix', feat: 'feat', docs: 'docs', refactor: 'refactor', perf: 'perf', test: 'test', style: 'style', chore: 'chore', design: 'design' },
};

// the release environments, small first
export const releasePresets: Preset<string[]>[] = [
  { label: 'dev, prod', why: 'dev follows the trunk, prod takes the finals: two environments, no candidates', value: ['dev', 'prod'] },
  { label: 'dev, stage, prod', why: 'stage takes the candidates, a release is checked there before prod', value: ['dev', 'stage', 'prod'] },
];

// nextSprint is the sprint after the list's last: its start plus
// length_days, its number plus one (s12 → s13), as the board's button
export function nextSprint(list: ConfigSprint[], lengthDays: number): ConfigSprint {
  const last = list[list.length - 1];
  if (!last) {
    const start = new Date();
    const monday = new Date(start);
    monday.setDate(start.getDate() - ((start.getDay() + 6) % 7));
    return { slug: 's1', title: 'Sprint 1', start: iso(monday) };
  }
  const start = new Date(last.start);
  start.setDate(start.getDate() + (lengthDays || 14));
  const n = Number(last.slug.replace(/^s/, ''));
  const slug = Number.isFinite(n) ? `s${n + 1}` : `${last.slug}-b`;
  return { slug, title: last.title.replace(/\d+/, String(n + 1)) || slug, start: iso(start) };
}

// iso is a date as the file holds it: YYYY-MM-DD, the day as it is here
function iso(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

// fromSprintStart reads a sprint's start back from the server's RFC 3339:
// the file's day, as written, whatever this browser's time zone
export function fromSprintStart(rfc: string): string {
  return /^\d{4}-\d{2}-\d{2}/.exec(rfc)?.[0] ?? '';
}

// toSprintStart sends a date the way the server's time.Time reads it
export function toSprintStart(day: string): string {
  return day ? `${day}T00:00:00Z` : '';
}
