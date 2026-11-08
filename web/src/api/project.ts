// The GitHub Project bridge (docs/rewrite/api.md, routes 52 and 53): its
// last run, kept by the server, and where each field lives; a check, a
// run one side or both, a conflict resolved. Only an action reads the
// Project.
import { get, send } from './http.ts';

export interface ProjectConflict {
  issue: string;
  number: number;
  url: string;
  title: string;
  field: string;
  base: string;
  has_base: boolean;
  local: string; // git-board's value, "" when unset
  project: string; // the Project's
}

export interface ProjectChange {
  issue: string;
  number: number;
  url: string;
  title: string;
  added: string[];
  removed: string[];
  writes: string[];
  warnings: string[];
}

export interface ProjectRun {
  project: string;
  title: string;
  checked_at: string;
  side: 'labels' | 'project' | 'both';
  applied: boolean;
  write: boolean;
  truncated: boolean;
  items: number;
  skipped: number;
  columns: Record<string, string>; // the Project's fields, by name, with their type
  counts: { matched: number; label_changes: number; project_writes: number; conflicts: number; warnings: number; unmatched: number };
  conflicts: ProjectConflict[];
  unmatched: { number: number; url: string; title: string; reason: string; not_an_issue: boolean }[];
  changes: ProjectChange[];
}

// where a field lives on a forge: its target, its Project column, and
// whether the last run found that column (absent before any)
export interface FieldPlace {
  field: string;
  forge: string;
  kind: string;
  to: string;
  default: string;
  column?: string;
  on_project?: boolean;
}

export interface ProjectStatus {
  project: string; // owner/number, "" when none is configured
  write: boolean;
  fields: string[]; // the fields the Project holds as columns
  stated: boolean; // [github-project] fields names them; else they are the default
  last: ProjectRun | null;
  places: FieldPlace[];
  problem?: string;
  kept?: string;
}

export function getProject(): Promise<ProjectStatus> {
  return get('/api/github-project');
}

// route 58: the Project named, as project configure does; the fields it
// holds are written with it
export function configureProject(owner: string, number: number): Promise<ProjectStatus> {
  return send('PUT', '/api/github-project', { body: { owner, number } });
}

export function checkProject(): Promise<ProjectStatus> {
  return send('POST', '/api/github-project/check', { body: {} });
}

// side: labels is git board project pull, project its push, both its sync
export function applyProject(side: 'labels' | 'project' | 'both'): Promise<ProjectStatus> {
  return send('POST', '/api/github-project/apply', { body: { side } });
}

// issue: owner/repo#n, its number alone when one repository has it
export function resolveProject(issue: string, field: string, keep: 'git-board' | 'project'): Promise<ProjectStatus> {
  return send('POST', '/api/github-project/resolve', { body: { issue, field, keep } });
}
