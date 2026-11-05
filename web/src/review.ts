// A thread's review comments (the cycle's, M6: a line comment is a comment
// with review metadata): grouped under their first comment, and what each
// did, as a comment's header says it. Pure: node runs review.test.ts.
import type { IssueEvent } from './api/types.ts';

export type ReviewThread = { root: IssueEvent; replies: IssueEvent[]; resolved: boolean };

// onBranch: a review event of this branch, or of none the server found
// (its commit on no branch of the issue any more); every one without a branch.
const onBranch = (e: IssueEvent, branch?: string) => !branch || !e.review?.branch || e.review.branch === branch;

// threadsOf groups the line comments, in the order their first comment
// was written; a reply whose first comment isn't here stands alone.
export function threadsOf(events: IssueEvent[], branch?: string): ReviewThread[] {
  events = events.filter((e) => onBranch(e, branch));
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

// editable: a plain comment. A review comment is the pull request's: the
// sync sends it once and never its edits.
export const editable = (e: IssueEvent) => !e.review;

const verbs = { APPROVE: 'approved', REQUEST_CHANGES: 'requested changes', COMMENT: 'reviewed' } as const;

// verbOf: what a comment did, as its header says it.
export function verbOf(e: IssueEvent): string {
  if (e.kind === 'created') return 'opened';
  const r = e.review;
  if (r?.state) return verbs[r.state];
  if (r?.reply_to) return 'replied';
  if (r?.path) return 'commented on a line';
  return r?.pr ? `commented on pull request #${r.pr.split('#')[1]}` : 'commented';
}
