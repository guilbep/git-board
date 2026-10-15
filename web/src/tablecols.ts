// The table's columns: id, title, is (open/closed), one per field, links
// (the forge issues), other (labels without a namespace). A view without
// `columns` shows the default ones, so a new field gets its column; a
// view with them shows those, in that order. The title is always shown.
export function defaultColumns(names: string[]): string[] {
  return ['id', 'title', 'is', ...names, 'other'];
}

export function shownColumns(saved: string[] | undefined, names: string[]): string[] {
  if (!saved?.length) return defaultColumns(names);
  const known = new Set([...defaultColumns(names), 'links']);
  const out = saved.filter((k, i) => known.has(k) && saved.indexOf(k) === i);
  if (!out.includes('title')) out.splice(Math.min(1, out.length), 0, 'title');
  return out;
}

// the chooser's list: the shown columns in their order, then the hidden ones
export function chooserColumns(saved: string[] | undefined, names: string[]): string[] {
  const shown = shownColumns(saved, names);
  return [...shown, ...[...defaultColumns(names), 'links'].filter((k) => !shown.includes(k))];
}

export function toggleColumn(cols: string[], key: string): string[] {
  if (key === 'title') return cols;
  return cols.includes(key) ? cols.filter((k) => k !== key) : [...cols, key];
}

export function moveColumn(cols: string[], key: string, by: -1 | 1): string[] {
  const i = cols.indexOf(key);
  const j = i + by;
  if (i < 0 || j < 0 || j >= cols.length) return cols;
  const out = [...cols];
  [out[i], out[j]] = [out[j]!, out[i]!];
  return out;
}

// what a view saves: nothing for the default columns, which stay dynamic
export function savedColumns(cols: string[], names: string[]): string[] | undefined {
  const d = defaultColumns(names);
  return cols.length === d.length && cols.every((k, i) => k === d[i]) ? undefined : cols;
}

// the rows from one to another in the table's order, for Shift-click
export function rangeIds(order: string[], anchor: string, target: string): string[] {
  const a = order.indexOf(anchor);
  const b = order.indexOf(target);
  if (a < 0 || b < 0) return [target];
  return order.slice(Math.min(a, b), Math.max(a, b) + 1);
}
