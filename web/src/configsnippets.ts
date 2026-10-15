// The settings page's "+ View", "+ Sprint" and "+ Field" buttons: each
// adds a ready-to-edit table to .git-board.toml and says which words to
// select (the name to change).
export type Snippet = { text: string; select: [number, number] };

function append(text: string, block: string, name: string): Snippet {
  const base = text.replace(/\s*$/, '');
  const out = (base ? base + '\n\n' : '') + block + '\n';
  const at = out.lastIndexOf(name);
  return { text: out, select: [at, at + name.length] };
}

export function addView(text: string): Snippet {
  return append(text, '[views.new-view]\nlayout = "board"\nfilter = "is:open"\ngroup = "status"', 'new-view');
}

export function addField(text: string): Snippet {
  return append(text, '[fields.new-field]\nsingle = true\noptions = ["todo", "doing", "done"]', 'new-field');
}

// addSprint follows the last sprint: its start + length_days (14 by
// default), its number + 1. Placed after the last [[sprints.list]] entry.
export function addSprint(text: string, today = new Date()): Snippet {
  const len = Number(text.match(/^\s*length_days\s*=\s*(\d+)/m)?.[1] ?? 14);
  const entries = [...text.matchAll(/\[\[sprints\.list\]\][^[]*/g)];
  let start = today.toISOString().slice(0, 10);
  let n = entries.length + 1;
  let insertAt = -1;
  const last = entries.at(-1);
  if (last) {
    const d = last[0].match(/start\s*=\s*(\d{4}-\d{2}-\d{2})/)?.[1];
    if (d) start = new Date(Date.parse(d) + len * 864e5).toISOString().slice(0, 10);
    const num = last[0].match(/slug\s*=\s*"[^"]*?(\d+)"/)?.[1];
    if (num) n = Number(num) + 1;
    insertAt = last.index! + last[0].replace(/\s*$/, '').length;
  }
  const slug = `sprint-${n}`;
  const block = `[[sprints.list]]\nslug = "${slug}"\ntitle = "Sprint ${n}"\nstart = ${start}`;
  if (insertAt < 0) return append(text, block, slug);
  const out = text.slice(0, insertAt) + '\n\n' + block + text.slice(insertAt);
  const at = insertAt + 2 + block.indexOf(slug);
  return { text: out, select: [at, at + slug.length] };
}
