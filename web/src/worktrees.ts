// The words of Settings › Worktrees and of an issue's agent (wtx).
import type { IssueAgent, Worktrees, WtxSession } from './api/board.ts';

// Who commits is a set of deny rules in wtx.toml: a person commits and
// opens the PR, or the agent may commit and a person pushes.
export const PERSON_RULES = ['Bash(git commit *)', 'Bash(git push *)', 'Bash(gh pr create *)'];
export const AGENT_RULES = ['Bash(git push *)'];
const WHO_RULES = new Set([...PERSON_RULES, ...AGENT_RULES]);

export type WhoCommits = 'person' | 'agent' | '';

export function whoCommits(deny: string[]): WhoCommits {
  if (PERSON_RULES.every((r) => deny.includes(r))) return 'person';
  return AGENT_RULES.every((r) => deny.includes(r)) && !deny.some((r) => PERSON_RULES.includes(r) && !AGENT_RULES.includes(r)) ? 'agent' : '';
}

export function setWhoCommits(deny: string[], who: 'person' | 'agent'): string[] {
  return [...deny.filter((r) => !WHO_RULES.has(r)), ...(who === 'person' ? PERSON_RULES : AGENT_RULES)];
}

// withGate: a deny that says nothing of who commits gets the gate, a
// person commits, so the form writes what its radio shows.
export function withGate(deny: string[]): string[] {
  return whoCommits(deny) ? deny : setWhoCommits(deny, 'person');
}

// writeConfirm: what Write does, every file wtx init --force touches.
export function writeConfirm(repo: string, config: boolean): string {
  return `${config ? 'Replaces' : 'Writes'} wtx.toml and .wt.toml, and adds wtx's lines to .gitignore, in ${repo || 'the repository'}'s main checkout, uncommitted.`;
}

// extraDeny is the Never allowed chips: the deny rules who commits doesn't own.
export function extraDeny(deny: string[]): string[] {
  return deny.filter((r) => !WHO_RULES.has(r));
}

// WTX_OWN is wtx's baseline deny, said in one line.
export const WTX_OWN = 'force pushes, pushes to the protected branches, changing the remote';

export function toolsLine(d: Worktrees): { ok: boolean; text: string } {
  if (!d.wtx) return { ok: false, text: 'wtx is not on PATH where git board serve runs.' };
  // 'tmux server reachable' is left out: it fails inside a sandbox
  const missing = d.doctor?.checks.find((c) => (c.name === 'wt' || c.name === 'tmux') && !c.ok);
  if (missing) return { ok: false, text: `${missing.name} missing: ${missing.fix}` };
  return { ok: true, text: 'wtx, wt and tmux on this machine' };
}

export function agentWords(s?: WtxSession, worktree?: boolean): string {
  if (!worktree) return 'No worktree';
  if (!s) return 'Worktree, no session';
  if (!s.live) return 'Session down';
  switch (s.state) {
    case 'permission':
      return 'Agent asks permission';
    case 'idle':
      return 'Agent waits for a brief';
    case 'stop':
      return 'Agent done, waits for you';
    case 'start':
    case 'running':
      return 'Agent running';
  }
  return 'Session up';
}

export function agentButton(a: IssueAgent): 'start' | 'brief' | null {
  if (a.reason) return null;
  return a.worktree ? 'brief' : 'start';
}

export function sessionName(a: IssueAgent): string {
  return a.session?.session ?? '';
}
