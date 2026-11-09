// The board's state, named (route 60, app.State): each part's state in a
// word, and the next actions. The Start page reads it.
import { get } from './http.ts';

export interface StateAction {
  id: string; // me, resume, login:<forge>, sync:<forge>, forge, project, fields, new
  say: string;
  cli?: string; // for the CLI and agents: the page shows a button, never this
  api?: string;
}

export interface ForgeState {
  name: string;
  kind: string;
  project: string;
  way: 'push' | 'pull' | 'both';
  state: 'off' | 'no-token' | 'first' | 'on' | 'synced';
  discovered?: boolean;
  token?: string;
  last_run?: string;
}

export interface BoardState {
  phase: 'new' | 'ready';
  file: { state: 'none' | 'current' | 'older' | 'newer'; path: string; version?: number };
  me: { state: 'none' | 'chosen'; id?: string };
  datastore: { state: 'git' | 'markdown' | 'postgres' | 'forges'; where: string; from_env?: boolean };
  forges: ForgeState[];
  project: { state: 'none' | 'broken' | 'configured'; project?: string; fields?: string[]; stated?: boolean; write?: boolean; problem?: string };
  run: { state: 'idle' | 'running' | 'stopped'; problem?: string };
  issues: number;
  cycle: boolean;
  next: StateAction[];
}

export function getState(): Promise<BoardState> {
  return get('/api/state');
}
