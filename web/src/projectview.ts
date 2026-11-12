// The GitHub Project's view, in words (Settings › GitHub Project, and the
// fields' places): pure, node --test imports it.
import type { FieldPlace, ProjectConflict, ProjectRun } from './api/project.ts';

export const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;

// a value as a person reads it: "not set" for none
export const shown = (value: string) => value || 'not set';

// the issue a conflict is about, as resolve takes it: owner/repo#n
export function issueRef(c: Pick<ProjectConflict, 'url' | 'number'>): string {
  const m = /^https:\/\/github\.com\/([^/]+\/[^/]+)\/issues\/\d+/.exec(c.url);
  return m ? `${m[1]}#${c.number}` : String(c.number);
}

// what a run did or would do, in one line
export function runWords(r: ProjectRun): string {
  const c = r.counts;
  const parts = [`${plural(c.matched, 'issue')} matched`];
  if (c.label_changes) parts.push(`${c.label_changes} with label changes`);
  if (c.project_writes) parts.push(`${plural(c.project_writes, 'Project write')}${r.skipped ? `, ${r.skipped} not written (no write = true)` : ''}`);
  if (c.conflicts) parts.push(plural(c.conflicts, 'conflict'));
  if (c.unmatched) parts.push(`${c.unmatched} unmatched`);
  if (r.truncated) parts.push(`only the first ${r.items} items read`);
  return parts.join(', ');
}

// the fit, in one line: what the file lacks for the Project; "" when it fits
export function fitLine(r: Pick<ProjectRun, 'fit'>): string {
  if (!r.fit.length) return '';
  const clears = r.fit.reduce((n, p) => n + p.clears, 0);
  return `The file and the Project disagree: ${plural(r.fit.length, 'change')} ${r.fit.length === 1 ? 'clears' : 'clear'} ${plural(clears, 'warning')}.`;
}

// an answer of a question, as its menu says it
export function answerWords(field: string, answer: string): string {
  if (answer === 'add-kind') return 'Add it as a kind';
  if (answer === 'unmapped') return 'Leave it unmapped';
  const v = answer.replace(/^map=/, '');
  return field === 'category' ? `It is the kind ${v}` : field === 'sprint' ? `Its label is sprint:${v}` : `Its value is ${v}`;
}

// a field's place in the Project, as the fields' list marks it: the
// column, and a warning when the last run found no such column there. A
// forge the server didn't name (one found in the remotes) is the
// bridge's: the Project is GitHub's, one per board.
export function projectMark(places: FieldPlace[], forge: string, field: string): { column: string; missing: boolean } | null {
  const inProject = places.filter((x) => x.field === field && x.to === 'project');
  const p = inProject.find((x) => x.forge === forge) ?? (places.some((x) => x.forge === forge) ? undefined : inProject.find((x) => x.kind === 'github'));
  if (!p) return null;
  return { column: p.column ?? field, missing: p.on_project === false };
}
