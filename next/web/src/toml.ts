// tomlTokens splits one line of .git-board.toml into colored pieces for the
// settings page's editor: comments, [tables], keys, strings, numbers,
// dates and booleans. A highlighter, not a parser: the server checks the
// file (POST /api/config/check).
export type TomlToken = { cls: '' | 'comment' | 'table' | 'key' | 'string' | 'value' | 'punct'; text: string };

const valueRe = /^(true|false|[+-]?\d[\d_]*(\.\d+)?([eE][+-]?\d+)?|\d{4}-\d{2}-\d{2}([T ][\d:.]+)?(Z|[+-]\d{2}:\d{2})?)\b/;

export function tomlTokens(line: string): TomlToken[] {
  const out: TomlToken[] = [];
  const push = (cls: TomlToken['cls'], text: string) => {
    if (!text) return;
    const last = out.at(-1);
    if (last && last.cls === cls) last.text += text;
    else out.push({ cls, text });
  };
  const lead = line.match(/^\s*/)![0];
  push('', lead);
  let rest = line.slice(lead.length);
  if (rest.startsWith('[')) {
    const double = rest.startsWith('[[');
    const close = double ? rest.indexOf(']]') + 2 : rest.indexOf(']') + 1;
    if (close > (double ? 1 : 0)) {
      push('table', rest.slice(0, close));
      rest = rest.slice(close);
    }
  } else {
    const key = rest.match(/^("[^"]*"|'[^']*'|[A-Za-z0-9_.-]+)(\s*)=/);
    if (key) {
      push('key', key[1]!);
      push('', key[2]!);
      push('punct', '=');
      rest = rest.slice(key[0].length);
    }
  }
  while (rest) {
    const c = rest[0]!;
    if (c === '#') {
      push('comment', rest);
      break;
    }
    if (c === '"' || c === "'") {
      let i = 1;
      while (i < rest.length && rest[i] !== c) i += c === '"' && rest[i] === '\\' ? 2 : 1;
      push('string', rest.slice(0, i + 1));
      rest = rest.slice(i + 1);
      continue;
    }
    const v = rest.match(valueRe);
    if (v) {
      push('value', v[0]);
      rest = rest.slice(v[0].length);
      continue;
    }
    const bare = rest.match(/^[A-Za-z0-9_-]+\s*(?==)/);
    if (bare) {
      push('key', bare[0]); // a key inside an inline table: { comite = "…" }
      rest = rest.slice(bare[0].length);
      continue;
    }
    push('[]{},='.includes(c) ? 'punct' : '', c);
    rest = rest.slice(1);
  }
  return out;
}
