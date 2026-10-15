// An issue's history in words: a label change is, field by field, what
// people did ("set Status to qa", "added assignee bob").

export interface LabelChange {
  verb: 'set' | 'cleared' | 'added' | 'removed';
  field: string; // a namespace, or "label" for plain labels
  values: string[];
}

const split = (l: string): [string, string] => {
  const i = l.indexOf(':');
  return i < 0 ? ['label', l] : [l.slice(0, i), l.slice(i + 1)];
};

// describeLabels groups a change by field: one value in, one out (or none
// before) is a field set; several, or a plain label, are added and removed;
// only out is cleared.
export function describeLabels(added: string[], removed: string[]): LabelChange[] {
  const fields: string[] = [];
  const by = new Map<string, { added: string[]; removed: string[] }>();
  const entry = (field: string) => {
    if (!by.has(field)) {
      by.set(field, { added: [], removed: [] });
      fields.push(field);
    }
    return by.get(field)!;
  };
  for (const l of added) {
    const [f, v] = split(l);
    entry(f).added.push(v);
  }
  for (const l of removed) {
    const [f, v] = split(l);
    entry(f).removed.push(v);
  }
  // fields first, plain labels last
  fields.sort((a, b) => Number(a === 'label') - Number(b === 'label'));

  const out: LabelChange[] = [];
  for (const field of fields) {
    const e = by.get(field)!;
    const single = field !== 'label' && field !== 'assignee' && e.added.length === 1 && e.removed.length <= 1;
    if (single) {
      out.push({ verb: 'set', field, values: e.added });
    } else if (field !== 'label' && field !== 'assignee' && e.added.length === 0 && e.removed.length === 1) {
      out.push({ verb: 'cleared', field, values: e.removed });
    } else {
      if (e.added.length) out.push({ verb: 'added', field, values: e.added });
      if (e.removed.length) out.push({ verb: 'removed', field, values: e.removed });
    }
  }
  return out;
}
