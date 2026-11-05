// What a field is, inferred the way v1's config.TypeOf inferred it
// (internal/config/types.go; domain/issues/fields.go now): the explicit type; else numeric; else what
// the name says (sprint, assignee, due…); else what every value parses
// as (dates, issue references); else select. The fields editor shows the
// inference instead of a dead "guessed", and goes into its numbers UI
// when the values say numbers.
export type FieldType = 'select' | 'iteration' | 'person' | 'number' | 'date' | 'issue' | 'text';

const dateValue = /^\d{4}-\d{2}-\d{2}$/;
const issueValue = /^(#\d+|[0-9a-f]{7,64})$/;
const numberValue = /^[+-]?(\d+(\.\d+)?|\.\d+)([eE][+-]?\d+)?$/;

const byName: Record<string, FieldType> = {
  sprint: 'iteration',
  iteration: 'iteration',
  assignee: 'person',
  assignees: 'person',
  reviewer: 'person',
  owner: 'person',
  due: 'date',
  deadline: 'date',
  parent: 'issue',
  'blocked-by': 'issue',
  blocks: 'issue',
  'depends-on': 'issue',
  'duplicate-of': 'issue',
};

// allNumbers says whether every value parses as a number: the editor's
// numbers mode, where the values sort by value and a unit applies
export function allNumbers(options: string[]): boolean {
  return options.length > 0 && options.every((o) => numberValue.test(o.trim()));
}

// inferType is a field's type as the server would read it when `type` is
// empty: numbers first, then the name, then what the values are. The
// roles are the caller's ([roles] makes a name a person).
export function inferType(name: string, options: string[], numeric: boolean, roles: string[] = []): FieldType {
  if (numeric || allNumbers(options)) return 'number';
  if (roles.includes(name)) return 'person';
  if (byName[name]) return byName[name];
  if (options.length > 0 && options.every((o) => dateValue.test(o.trim()))) return 'date';
  if (options.length > 0 && options.every((o) => issueValue.test(o.trim()))) return 'issue';
  return 'select';
}
