// The Changes tab's logic (docs/prd/work-unit.md, M4), kept out of the
// components so node --test reaches it: which files open, the unified diff
// as rows, line comments as threads, viewed marks.
import type { FileStat, IssueEvent, Review } from './api/board.ts';

export type Group = 'review' | 'large' | 'generated';

const changed = (f: FileStat) => f.added + f.deleted;

// Generated files fold whatever their size; past 3000 changed lines a
// file is "large" and loads on demand.
export function groupOf(f: FileStat): Group {
  if (f.generated) return 'generated';
  return changed(f) > 3000 ? 'large' : 'review';
}

export function autoOpens(f: FileStat): boolean {
  return groupOf(f) === 'review' && !f.binary && changed(f) < 1000;
}

// A large file's "Load the first 5,000 lines".
export const LARGE_LIMIT = 5000;

const shellQuote = (s: string) => (/^[\w./@:+-]+$/.test(s) ? s : `'${s.replaceAll("'", `'\\''`)}'`);

// The command that shows the same diff in a terminal.
export function diffCommand(from: string, to: string, path: string): string {
  return `git diff ${from.slice(0, 12)} ${to.slice(0, 12)} -- ${shellQuote(path)}`;
}

export type Row = { op: ' ' | '-' | '+' | '@'; old?: number; new?: number; text: string };

// parseUnified reads one file's git diff: hunk headers as '@' rows, each
// line numbered on its side(s); the file header before the first hunk and
// "\ No newline at end of file" are left out.
export function parseUnified(diff: string): Row[] {
  const rows: Row[] = [];
  let old = 0;
  let neu = 0;
  let inHunk = false;
  for (const l of diff.split('\n')) {
    const h = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(l);
    if (h) {
      [old, neu, inHunk] = [Number(h[1]), Number(h[2]), true];
      rows.push({ op: '@', text: l });
    } else if (!inHunk || l === '' || l.startsWith('\\')) {
      continue;
    } else if (l[0] === '-') {
      rows.push({ op: '-', old: old++, text: l.slice(1) });
    } else if (l[0] === '+') {
      rows.push({ op: '+', new: neu++, text: l.slice(1) });
    } else {
      rows.push({ op: ' ', old: old++, new: neu++, text: l.slice(1) });
    }
  }
  return rows;
}

export type ReviewThread = { root: IssueEvent; replies: IssueEvent[]; resolved: boolean };

// threadsOf groups the line comments, in the order their first comment
// was written; a reply whose first comment isn't here stands alone.
export function threadsOf(events: IssueEvent[]): ReviewThread[] {
  const roots = new Set(events.filter((e) => e.review?.path && !e.review.reply_to).map((e) => e.id));
  const threads = new Map<string, ReviewThread>();
  for (const e of events) {
    const r = e.review;
    if (!r?.path) continue;
    const parent = r.reply_to && roots.has(r.reply_to) ? threads.get(r.reply_to) : undefined;
    if (parent) parent.replies.push(e);
    else threads.set(e.id, { root: e, replies: [], resolved: !!r.resolved });
  }
  return [...threads.values()];
}

export function reviewsOf(events: IssueEvent[]): IssueEvent[] {
  return events.filter((e) => e.review?.state);
}

export function lastReviewBy(events: IssueEvent[], who: string): IssueEvent | undefined {
  return reviewsOf(events)
    .filter((e) => e.author.id === who)
    .at(-1);
}

// lineIn is the line a comment sits on in a view ending at `to`: where the
// line is at head (the server's head_line) when the view ends at head, the
// line as written when it ends at the comment's commit; null otherwise,
// outdated or not annotated: the file's box above the diff.
export function lineIn(r: Review, to: string, head: string): number | null {
  if (to === head) return r.outdated || !r.head_line ? null : r.head_line;
  return to === r.commit && r.line ? r.line : null;
}

// commitsAgo: how far a commit is from head in the newest-first list.
export function commitsAgo(sha: string, commits: { sha: string }[]): number | null {
  const i = commits.findIndex((c) => c.sha === sha);
  return i < 0 ? null : i;
}

// Viewed marks, path → the file's blob when viewed: a push un-views only
// the files it changed. Kept in this browser, per issue and branch.
export type Viewed = Record<string, string>;

const viewedKey = (issue: string, branch: string) => `gb-viewed:${issue}:${branch}`;

export function loadViewed(issue: string, branch: string): Viewed {
  try {
    return JSON.parse(localStorage.getItem(viewedKey(issue, branch)) ?? '{}') as Viewed;
  } catch {
    return {};
  }
}

export function saveViewed(issue: string, branch: string, v: Viewed) {
  try {
    localStorage.setItem(viewedKey(issue, branch), JSON.stringify(v));
  } catch {
    // private window: the marks last until reload
  }
}

// viewedCount is [viewed, of], generated files left out.
export function viewedCount(files: FileStat[], viewed: Viewed): [number, number] {
  const counted = files.filter((f) => !f.generated);
  return [counted.filter((f) => viewed[f.path] === f.blob).length, counted.length];
}
