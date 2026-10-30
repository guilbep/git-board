// The conversation's people, at the end of the tab row (README 16f): the
// author, then each commenter by their latest comment, newest first; the
// viewer marked. Pure: PeopleButton draws them.
import type { IssueEvent, Person } from './api/board';

export interface PersonRow {
  person: Person;
  role: 'opened it' | 'commented';
  you: boolean;
}

export function peopleOf(author: Person, events: IssueEvent[], meId: string): PersonRow[] {
  const rows: PersonRow[] = [{ person: author, role: 'opened it', you: !!meId && author.id === meId }];
  const comments = events.filter((e) => e.kind === 'comment').sort((a, b) => Date.parse(b.at) - Date.parse(a.at));
  for (const c of comments) {
    if (rows.some((r) => r.person.id === c.author.id)) continue;
    rows.push({ person: c.author, role: 'commented', you: !!meId && c.author.id === meId });
  }
  return rows;
}

export const peopleLabel = (n: number) => `${n} in this conversation`;
