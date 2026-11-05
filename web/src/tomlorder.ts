// The order of a parent's tables in the text: [fields.status] before
// [fields.prio]. PATCH /api/config writes keys where they are; a form that
// reorders (Fields, Views) puts the whole text after it (PUT), its tables'
// blocks moved and nothing else. Pure: node runs tomlorder.test.ts on it.

const header = /^\s*\[\[?\s*([^\]]+?)\s*\]\]?\s*(#.*)?$/;

// reorderTables moves each [parent.<name>] block, with its sub-tables
// ([parent.<name>.x]), to follow order, where the first of them was; a
// block's comment lines just above its header go with it. Names order
// doesn't hold keep their place after the ordered ones.
export function reorderTables(text: string, parent: string, order: string[]): string {
  const lines = text.split('\n');
  type Block = { name: string; lines: string[] };
  const out: (string | Block)[] = [];
  let cur: Block | null = null;
  const nameOf = (l: string) => {
    const m = header.exec(l);
    if (!m) return null;
    const t = m[1]!.split('.').map((x) => x.trim());
    return t[0] === parent.split('.')[0] && t.slice(0, parent.split('.').length).join('.') === parent && t.length > parent.split('.').length ? t[parent.split('.').length]! : '';
  };
  for (const l of lines) {
    const n = nameOf(l);
    if (n === null) {
      (cur ? cur.lines : (out as string[])).push(l);
      continue;
    }
    if (n && cur && cur.name === n) {
      cur.lines.push(l);
      continue;
    }
    // the comments over this header go with it
    const above: string[] = [];
    const tail = cur ? cur.lines : null;
    const pool = tail ?? out;
    while (pool.length && typeof pool[pool.length - 1] === 'string' && /^\s*#/.test(pool[pool.length - 1] as string)) above.unshift(pool.pop() as string);
    if (n) {
      cur = { name: n, lines: [...above, l] };
      out.push(cur);
    } else {
      cur = null;
      out.push(...above, l);
    }
  }
  const blocks = out.filter((x): x is Block => typeof x !== 'string');
  const rank = (n: string) => (order.includes(n) ? order.indexOf(n) : order.length);
  const sorted = [...blocks].sort((a, b) => rank(a.name) - rank(b.name));
  let i = 0;
  const flat = out.map((x) => (typeof x === 'string' ? x : sorted[i++]!));
  // a block's last line is blank between tables: the last one ends the file as it did
  const blank = (b: Block) => {
    const ls = [...b.lines];
    while (ls.length && ls[ls.length - 1] === '') ls.pop();
    return ls;
  };
  const res: string[] = [];
  flat.forEach((x) => {
    if (typeof x === 'string') res.push(x);
    else {
      if (res.length && res[res.length - 1] !== '') res.push('');
      res.push(...blank(x), '');
    }
  });
  while (res.length > 1 && res[res.length - 1] === '' && res[res.length - 2] === '') res.pop();
  if (!text.endsWith('\n')) while (res.length && res[res.length - 1] === '') res.pop();
  return res.join('\n');
}
