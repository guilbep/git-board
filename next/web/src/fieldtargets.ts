// Where a field lives on a forge, resolved the way the server's
// config.FieldTarget resolves it (internal/config/mapping.go): the
// mapping's `to`, else the old homes the file implies, else the type's
// default. The Forges page's Fields table is this, made visible.
import { inferType, type FieldType } from './fieldinfer.ts';
import type { ConfigField } from './configparse.ts';

// ForgeModel is one [forges.<name>] table, as the config model carries it.
export interface ForgeModel {
  name: string;
  kind: string; // github, gitlab
  project: string;
  host: string;
  sync: boolean;
  discovered?: boolean; // found in the git remotes, no table yet
  fields?: Record<string, ForgeMapping> | null;
}

// ForgeMapping is one field's override: only what differs from the type's
// default.
export interface ForgeMapping {
  to?: string | undefined;
  field?: string | undefined; // the Project column, for project
  values?: Record<string, string> | null | undefined; // value → name on the forge
}

// The targets, as the server spells them, as the page says them.
export const targetLabels: Record<string, string> = {
  label: 'Label',
  'scoped-label': 'Scoped label',
  project: 'Project field',
  assignees: 'Native assignees',
  milestone: 'Milestone',
  iteration: 'Iteration',
  weight: 'Weight',
  'due-date': 'Due date',
  'sub-issues': 'Sub-issues',
  none: 'Not synced',
};

// A field's type, as the server reads it: the file's, else the inference.
export function typeOf(fields: Record<string, ConfigField>, name: string, roles: string[]): FieldType {
  const f = fields[name] ?? {};
  return f.type ? (f.type as FieldType) : inferType(name, f.options ?? [], !!f.numeric, roles);
}

// The GitHub Project's own columns, synced whatever the fields say, when
// a Project is configured (config.ProjectFields).
const projectFields = ['status', 'prio', 'mod', 'est', 'sprint', 'moscow', 'milestone'];

// defaultTarget is the target a field of that type uses on the forge when
// nothing is written (config.DefaultTarget): every default a free
// feature; the assignees slot is the field named assignee alone.
export function defaultTarget(kind: string, field: string, fieldType: string): string {
  if (kind === 'gitlab') {
    if (field === 'assignee' && fieldType === 'person') return 'assignees';
    if (fieldType === 'iteration') return 'milestone';
    if (fieldType === 'date') return 'due-date';
    return 'label';
  }
  if (field === 'assignee' && fieldType === 'person') return 'assignees';
  if (fieldType === 'issue') return 'sub-issues';
  if (fieldType === 'text') return 'none';
  return 'label';
}

// fieldTarget is where the field lives on the forge: the mapping, else
// the old homes (a project map on a GitHub field; the Project's own
// columns when a Project is configured), else the type default.
export function fieldTarget(forge: ForgeModel, fields: Record<string, ConfigField>, name: string, roles: string[], projectNumber: number): string {
  const m = forge.fields?.[name];
  if (m?.to) return m.to;
  if (forge.kind === 'github') {
    if (Object.keys(fields[name]?.project ?? {}).length > 0) return 'project';
    if (projectNumber !== 0 && projectFields.includes(name)) return 'project';
  }
  return defaultTarget(forge.kind, name, typeOf(fields, name, roles));
}

// targetsFor is the dropdown's options: the targets this forge offers for
// the type. The paid-tier ones carry their note — offered as overrides,
// the defaults stay free.
export function targetsFor(kind: string, fieldType: string, field: string): { id: string; label: string }[] {
  const entry = (id: string, note?: string) => ({ id, label: targetLabels[id] + (note ? ` — ${note}` : '') });
  switch (kind) {
    case 'gitlab': {
      if (field === 'assignee' && fieldType === 'person') return [entry('assignees'), entry('label'), entry('none')];
      // the native slots take their own type alone (config.checkTarget)
      const all = [entry('label'), ...(fieldType === 'person' ? [entry('assignees')] : []), ...(fieldType === 'iteration' ? [entry('milestone')] : []), ...(fieldType === 'date' ? [entry('due-date')] : []), entry('none')];
      if (fieldType === 'select') all.splice(1, 0, entry('scoped-label', 'needs a paid tier'));
      if (fieldType === 'iteration') all.splice(1, 0, entry('iteration', 'needs a paid tier'));
      if (fieldType === 'number') all.splice(1, 0, entry('weight', 'needs a paid tier'));
      return all;
    }
    default: {
      if (fieldType === 'issue') return [entry('sub-issues'), entry('label'), entry('none')];
      if (fieldType === 'person') return field === 'assignee' ? [entry('assignees'), entry('label'), entry('none')] : [entry('label'), entry('assignees'), entry('none')];
      if (fieldType === 'text') return [entry('none'), entry('project', 'a Project text field'), entry('label')];
      return [entry('label'), entry('project', 'a Project field'), entry('none')];
    }
  }
}
