// The nouns of v2's API (docs/rewrite/api.md), as the CLI's --json prints
// them. Types only: node's tests import them freely.

// A page of a list: GET /api/issues. next is absent on the last page.
export interface Page<T> {
  items: T[];
  total: number;
  next?: string;
}

export interface Items<T> {
  items: T[];
}

// a person, as `me --json`
export interface Person {
  id: string;
  name: string;
  login?: string;
  avatar_url?: string;
}

// where an issue is on a forge
export interface Forge {
  name: string; // GitHub, GitLab, or the host
  url: string;
  number?: string;
}

// an issue, as `ls --json`, and its version
export interface Issue {
  id: string;
  title: string;
  shown_title: string; // the title the board shows
  status: string; // open or closed
  labels: string[];
  inherited?: string[]; // from the project's [roles], for the roles it doesn't set
  github_url?: string;
  forges?: Forge[];
  number?: string; // its project number, CO2-3075
  counted?: boolean; // the project numbers its issues
  updated_at: string;
  created_at: string;
  closed_at?: string;
  comments: number;
  version: string;
}

// a comment's review metadata (M6, the cycle's)
export interface Review {
  path?: string;
  line?: number;
  side?: 'old' | 'new';
  commit?: string;
  pr?: string;
  reply_to?: string;
  state?: 'APPROVE' | 'REQUEST_CHANGES' | 'COMMENT';
  resolved?: boolean;
  branch?: string;
  outdated?: boolean;
  head_line?: number;
  text?: string;
}

export interface TextVersion {
  id?: string;
  author: Person;
  at: string;
  body: string;
  html?: string;
}

// one item of a thread: created (the description), comment, labels,
// status, title
export interface IssueEvent {
  kind: 'created' | 'comment' | 'labels' | 'status' | 'title' | 'version' | 'unknown';
  id: string; // created, comment: the comment's id in the routes
  author: Person;
  at: string;
  body?: string;
  html?: string;
  edited?: boolean;
  history?: TextVersion[];
  pending?: string[]; // the forges the next sync takes this change to
  edit_pending?: string[];
  edit_was?: string;
  added?: string[];
  removed?: string[];
  status?: 'open' | 'closed';
  title?: string;
  was?: string;
  meta?: Record<string, string>;
  meta_history?: { id: string; author: Person; at: string; meta: Record<string, string> }[];
  review?: Review;
}

// a thread, as `show --json`, and its version
export interface Thread extends Issue {
  author: Person;
  edited_at: string;
  participants: Person[] | null;
  events: IssueEvent[];
}

// a log entry, as `log --json`
export interface LogEntry {
  at: string;
  author: Person;
  kind: string; // created, comment, edit, title, state, labels
  id: string;
  target?: string;
  title?: string;
  was?: string;
  state?: string;
  added?: string[];
  removed?: string[];
  body?: string;
}

export interface Undone {
  issue: string;
  title: string;
  undone: string;
  what: string;
}

export interface FollowState {
  following: boolean;
  reasons: string[];
  set: boolean | null; // what you chose; null: automatic
}

// a label in use, counted among the issues a filter matches
export interface LabelCount {
  label: string;
  field?: string; // status for status:ready; absent for a plain label
  value: string;
  open: number;
  closed: number;
}

// a field and its type: GET /api/fields
export interface FieldConfig {
  type?: string; // select, iteration, person, number, date, issue, text (always, from v2's server)
  single: boolean;
  numeric: boolean;
  options: string[] | null;
  unit?: string;
  current?: string; // an iteration's sprint today
  inherits?: string; // a role's project value
  people?: string[]; // who a person field offers
  colors?: Record<string, string>; // a value's color, as the file sets it
}

export type AvatarsMode = 'forge' | 'gravatar' | 'none';

export interface MeResponse {
  me: Person | null;
  avatars: AvatarsMode;
}

export interface BoardInfo {
  git_dir: string;
  dir: string;
  version: string;
  datastore: { name: string; where: string };
  experimental: string[];
  // this clone opened with no board, and the server pulled origin's, or
  // couldn't (docs/prd/clone-shows-board.md)
  first_pull?: { remote: string; issues: number; error?: string };
}

// the inbox, as `status --json`
export interface InboxRow {
  issue: Issue;
  reasons: string[];
  news: string[];
  who: Person[];
  latest: string;
  first: string;
}

export interface InboxResponse {
  me: { id: string; login: string };
  assigned: Issue[];
  opened: InboxRow[];
  follows: InboxRow[];
}

// .git-board.toml
export interface ConfigFile {
  schema_version: number;
  path: string;
  version: string;
  exists: boolean;
  text: string;
  keys: { key: string; value: string }[];
  problems: { table: string; message: string }[];
}

export interface Datastore {
  name: string; // git, Markdown files, PostgreSQL
  where: string;
  replica: boolean; // pulled and pushed (git)
  counter: boolean; // hands out project numbers
}

export interface Copied {
  to: string;
  entities: number;
  counter: boolean;
}

// sync: the forges, the remotes, the run
export interface SyncForge {
  name: string;
  kind: string; // github, gitlab
  project: string;
  host: string;
  login?: string;
  sync: boolean;
  discovered?: boolean; // found in origin's URLs, no [forges] table
}

export interface SyncStep {
  name?: 'pull' | 'forges' | 'project' | 'push'; // a whole sync's step
  state?: 'running' | 'done' | 'skipped' | 'failed';
  forge?: string;
  way?: string;
  what?: string; // created, updated, linked, posted, pushed, warning, waiting, left, held, note
  issue?: string;
  line?: string;
  done?: number;
  total?: number;
}

export interface SyncRun {
  id: string;
  scope: 'forges' | 'all';
  way: 'sync' | 'pull' | 'push';
  dry_run: boolean;
  started_at: string;
  ended_at?: string;
  state: 'running' | 'done' | 'failed';
  steps: SyncStep[];
  report?: { created: number; updated: number; unchanged: number; posted: number; pushed: number };
  error?: string;
}

export interface SyncStatus {
  forges: SyncForge[];
  remotes: string[];
  run: SyncRun | null;
}

export interface Ahead {
  receive: number;
  send: number;
}

export interface RemoteResult {
  remote: string;
  action: 'fetch' | 'pull' | 'push';
  note?: string;
  issues?: Ahead;
  people?: Ahead;
}

export interface Numbered {
  where?: string;
  numbered: number;
}

export interface UploadStatus {
  enabled: boolean;
  where?: string;
}

// a change event: GET /api/events
export interface ChangeEvent {
  kind: 'issue' | 'person' | 'config' | 'me' | 'inbox' | 'sync' | 'progress' | 'release';
  id?: string;
  version?: string;
  by?: string;
  step?: SyncStep;
  issue?: string;
}
