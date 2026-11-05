// What the filter's words are, and examples for this board
// (docs/prd/header-help.md, 5): the "How filtering works" popover and the
// Filter box's list read it. The grammar is internal/filter's (its package
// comment, and docs/guide/configuration.md's Filters table): change one,
// change the others. Pure: node --test imports it.
import type { FieldConfig } from './api/types.ts';

export interface Word {
  term: string; // as the help shows it
  means: string;
  example: string; // what Try writes into the box
}

export const GRAMMAR: Word[] = [
  { term: 'status:ready', means: 'a field’s value', example: 'status:ready' },
  { term: 'status:ready,qa', means: 'either value', example: 'status:ready,qa' },
  { term: '-status:done', means: 'not that value; a - before any word', example: '-status:done' },
  { term: 'no:assignee', means: 'no value at all; no:label, no plain label', example: 'no:assignee' },
  { term: 'assignee:@me', means: 'you, by your forge login', example: 'assignee:@me' },
  { term: 'sprint:@current', means: 'this sprint; @next, @previous', example: 'sprint:@current' },
  { term: 'is:open, is:closed', means: 'open or closed issues', example: 'is:closed' },
  { term: 'author:@me', means: 'who opened it', example: 'author:@me' },
  { term: 'involves:@me', means: 'the author, a person field, a comment or a mention', example: 'involves:@me' },
  { term: 'follows:@me', means: 'what your inbox follows', example: 'follows:@me' },
  { term: 'label:bug', means: 'a plain label', example: 'label:bug' },
  { term: 'title:*sync*', means: 'a pattern in the title', example: 'title:*sync*' },
  { term: '#2528', means: 'an issue by its number on a forge', example: '#2528' },
  { term: '"car sharing"', means: 'words in the title, the text, a comment', example: '"car sharing"' },
  { term: 'in:title', means: 'where the words are looked for; in:body, in:comments', example: 'crash in:title' },
  { term: 'created:>=@today-7d', means: 'opened this last week; updated:, closed: alike', example: 'created:>=@today-7d' },
  { term: 'closed:2026-10-01..2026-10-31', means: 'a span of days; >, >=, <, <= a day', example: 'closed:2026-10-01..2026-10-31' },
  { term: 'comments:>5', means: 'how many comments; 5, <5, 2..5', example: 'comments:>5' },
];

// the panel's ticks, as the shortcut list says them (keys.ts: Alt is ⌥ on a Mac)
export const TICKS: { keys: string[]; means: string }[] = [
  { keys: ['Click'], means: 'add or remove a value' },
  { keys: ['Alt', 'Click'], means: 'only this value; again: every value but it' },
  { keys: ['Shift', 'Click'], means: 'a range, from the last value clicked' },
  { keys: ['Space'], means: 'toggle the focused value' },
];

const ofType = (fields: Record<string, FieldConfig>, type: string) => Object.keys(fields).filter((f) => fields[f]?.type === type);

// examples this board can answer: its own field names, @me only with a
// login; five at most
export function examples(fields: Record<string, FieldConfig>, myLogin: string): { term: string; means: string }[] {
  const res: { term: string; means: string }[] = [];
  const people = ofType(fields, 'person');
  const person = people.includes('assignee') ? 'assignee' : people[0];
  if (person && myLogin) res.push({ term: `${person}:@me`, means: 'yours' });
  const sprint = ofType(fields, 'iteration')[0];
  if (sprint) res.push({ term: `${sprint}:@current`, means: 'this sprint' });
  const status = 'status' in fields ? 'status' : Object.keys(fields).find((f) => (fields[f]?.type ?? 'select') === 'select');
  if (status) res.push({ term: `no:${status}`, means: `no ${status} yet` });
  res.push({ term: 'is:closed', means: 'the closed issues' });
  if (myLogin) res.push({ term: 'involves:@me', means: 'where you are: author, assigned, commented, mentioned' });
  return res.slice(0, 5);
}

// the tricks of the words that aren't a field (internal/filter)
const DATES = '>2026-10-01, >=@today-7d, 2026-10-01..2026-10-31';
const QUALIFIERS: Record<string, string> = {
  created: DATES,
  updated: DATES,
  closed: DATES,
  comments: '>5, <=2, 2..5',
  in: 'title, body, comments; title,body for either',
};

// the next trick for the word being typed: field:… → not, either, none
export function nextTrick(text: string): string | null {
  const last = text.split(/\s+/).at(-1) ?? '';
  const m = /^(-?)([\w-]+):/.exec(last);
  if (m && QUALIFIERS[m[2]!]) return QUALIFIERS[m[2]!]!;
  if (m && m[2] !== 'is' && m[2] !== 'no') {
    return m[1] ? `no:${m[2]} for no value at all` : `-${m[2]}: for “not”, ${m[2]}:a,b for either`;
  }
  if (last && !last.includes(':') && !last.startsWith('#') && !last.startsWith('"')) return '"quotes" for a phrase; -word to leave it out';
  return null;
}
