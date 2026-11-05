// The cycle's shapes (experimental), as v1's server answered them: what
// GET /api/issues/{id}/cycle will answer when M6 builds it (api.md, 49).
// Until then no route answers them; the thread's rendering (flow.ts,
// cycle.ts, review.ts) reads them when given, and works without.
import type { Person } from './types.ts';

export interface GatePR {
  number: number;
  url: string;
  state: 'OPEN' | 'MERGED' | 'CLOSED';
  draft: boolean;
  title: string;
  body?: string; // the description, Markdown
  base: string;
  head: string; // its head commit
  merge_commit?: string; // when merged
  author: string;
  decision: string; // APPROVED, REVIEW_REQUIRED, CHANGES_REQUESTED, '' (no review rule)
  mergeable: string;
  merge_state: string;
  checks: GateCheck[];
  reviews: GateReview[];
  requested: string[];
  can_merge: boolean;
  checking: boolean; // GitHub is still computing mergeability
  waits: string[];
}

export interface CycleWaits {
  role?: string; // risk, plan, review, assignee, author, signoff
  login?: string; // '' with anyone
  what: string;
  anyone: boolean;
}

export interface CycleEvidence {
  kind?: string; // [branches]' kind, for step 4's rule
  criteria?: string[];
  criteria_problems?: string[]; // where the description reads differently from how it looks
  no_criteria?: boolean; // accepted as it is (intent:criteria=none)
  risk?: string;
  author?: string;
  assignees?: string[];
  people?: Record<string, string>; // the login holding each gate: risk, plan, review, signoff:<status>
  branch?: string;
  head?: string;
  base?: string;
  plan_branch?: string;
  plan_head?: string;
  plan?: CyclePlan;
  plan_approval?: { by: string; commit?: string; at: string };
  plan_pr?: GatePR | null;
  tests?: { path: string; commit?: string }[];
  pr?: GatePR | null;
  signoffs?: CycleSignoff[];
  runs?: { by: string; commit?: string; at: string }[];
  checks?: { index: number; by: string; commit?: string; at: string }[];
  lanes?: CycleLane[]; // the issue's branches, branched out (core/cycle, PickLane)
}

export interface Cycle {
  step: number; // the kind's, 1 to 7
  at: string; // the id of the list's entry reached, '' before the first opens
  of: number;
  name: string;
  stage: string;
  done: boolean;
  gate: string;
  risk?: string;
  skipped?: number[];
  note?: string; // evidence that disagrees with the status
  waits: CycleWaits;
  mine: boolean; // the viewer's turn, or anyone's
  actions?: string[]; // set-risk, approve-plan, review, verify, check, signoff, move-ready, start, ready-for-review, to-validation
  evidence: CycleEvidence;
  stages: CycleStages;
  pr_checked_at?: string;
  ai?: boolean; // an AI address is set: step 2 can ask it for the class
}

export interface CycleStages {
  before?: string[];
  ready: string;
  in_progress: string[];
  in_review: string[];
  validation: string[];
  closing: string;
  plans?: string;
  plans_in_pr?: boolean; // the draft PR opened at step 3 holds the plan
  criteria: string[]; // the headings the criteria are read under
  kinds: Record<string, string>; // step 4's rule by kind: failing-test, per-criterion, none
  guessed: boolean;
  roles?: Record<string, string>; // the role holding each gate: risk, plan, review, signoff:<status>
  steps?: StageStep[]; // the rail's cells, in order ([cycle] steps); the default seven when absent
}

export interface StageStep { id: string; kind: string; name: string; status?: string }

export interface FileStat {
  path: string;
  from?: string; // renamed from
  added: number;
  deleted: number;
  binary?: boolean;
  generated?: boolean; // linguist-generated
  blob: string; // the file at to: what "viewed" remembers
}


// an issue an issue field names, or one naming it (v1: GET /api/issues/{id}/refs)
export interface IssueRef {
  value: string;
  id?: string;
  title?: string;
  status?: string;
}

export interface GateCheck {
  name: string;
  state: 'success' | 'failure' | 'pending' | 'expected'; // expected: required, never reported
  required: boolean;
  url?: string;
}

export interface CycleLane {
  branch: string;
  head: string;
  step: number;
  done: boolean;
  closed: boolean;
  tests: number;
  pr?: { number: number; url: string; state: string; draft: boolean } | null;
  at: string;
}

export interface GateReview {
  login: string;
  state: string; // APPROVED, CHANGES_REQUESTED, DISMISSED
  commit: string;
}

export interface CycleSignoff {
  status: string;
  by: string;
  commit?: string;
  why?: string;
  at: string;
}

export interface CyclePlan {
  path: string;
  ref: string;
  status?: string; // the front matter's: draft, accepted, in-progress, delivered
  issue?: string; // the front matter's issue: line, what found the file
  html?: string; // the file rendered, below its front matter
  by?: string; // who first committed it on the plan branch
}
