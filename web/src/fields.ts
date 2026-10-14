// The display convention from the root README ("Display convention"),
// mirrored from internal/tui.ParseLabels/DisplayFieldName and
// cmd/git-board/render.go's formatFieldValues: a label matching a
// configured field shows under that field's name, everything else under
// "Other labels". No component re-derives this.
import type { Bucket, FieldConfig } from './api/board';

export type Fields = Record<string, FieldConfig>;

// Configured field names, sorted: the column order the CLI table uses.
export function fieldNames(fields: Fields): string[] {
  return Object.keys(fields).sort();
}

export function displayName(name: string): string {
  return name.charAt(0).toUpperCase() + name.slice(1);
}

// Values of field `name` on an issue's labels ("prio:p1" -> ["p1"]).
export function fieldValues(labels: string[], name: string): string[] {
  const prefix = `${name}:`;
  return labels.filter((l) => l.startsWith(prefix)).map((l) => l.slice(prefix.length));
}

// Labels that aren't a configured field's value.
export function otherLabels(labels: string[], fields: Fields): string[] {
  return labels.filter((l) => {
    const i = l.indexOf(':');
    return i < 0 || !(l.slice(0, i) in fields);
  });
}

// One value as shown to people: a numeric field named "est" is hours.
export function formatValue(name: string, value: string, fields: Fields): string {
  return name === 'est' && fields[name]?.numeric ? `${value}h` : value;
}

// Buckets in the group field's configured option order; values outside the
// options keep the server's order after them, "(none)" (board.BucketBy's
// key for issues without the field) comes last.
export function orderBuckets(buckets: Bucket[], group: string, fields: Fields): Bucket[] {
  const options = fields[group]?.options ?? [];
  const rank = (b: Bucket) => {
    if (b.group === '(none)') return options.length + 1;
    const i = options.indexOf(b.group);
    return i < 0 ? options.length : i;
  };
  // Array.prototype.sort is stable, so equal ranks keep the server's order.
  return [...buckets].sort((a, b) => rank(a) - rank(b));
}
