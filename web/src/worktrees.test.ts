import { test } from 'node:test';
import assert from 'node:assert/strict';
import { agentButton, agentWords, extraDeny, sessionName, setWhoCommits, toolsLine, whoCommits, withGate, writeConfirm, agentTools, previewOf, previewStale } from './worktrees.ts';
import type { IssueAgent, Worktrees, WtxCheck, WtxSession } from './api/board.ts';

const PERSON = ['Bash(git commit *)', 'Bash(git push *)', 'Bash(gh pr create *)'];
const GIT_BOARD = ['Bash(git tag *)', 'Bash(git push * --tags)', 'Bash(git bug push *)', 'Bash(git bug bridge *)'];

test('who commits reads the deny rules: all three, a person; push alone, the agent; else not set', () => {
  assert.equal(whoCommits(PERSON), 'person');
  assert.equal(whoCommits(['Bash(git tag *)', ...PERSON]), 'person');
  assert.equal(whoCommits(['Bash(git push *)']), 'agent');
  assert.equal(whoCommits([]), '');
  assert.equal(whoCommits(GIT_BOARD), '');
});

test('setting who commits swaps its rules and keeps the others in order', () => {
  const deny = ['Bash(git tag *)', ...PERSON, 'Bash(git bug push *)'];
  assert.deepEqual(setWhoCommits(deny, 'agent'), ['Bash(git tag *)', 'Bash(git bug push *)', 'Bash(git push *)']);
  assert.deepEqual(setWhoCommits(setWhoCommits(deny, 'agent'), 'person'), ['Bash(git tag *)', 'Bash(git bug push *)', ...PERSON]);
  assert.deepEqual(setWhoCommits(GIT_BOARD, 'person'), [...GIT_BOARD, ...PERSON]);
});

test('a deny that says nothing of who commits gets the gate: a person commits', () => {
  assert.deepEqual(withGate(['Bash(git tag *)']), ['Bash(git tag *)', ...PERSON]);
  assert.deepEqual(withGate([]), PERSON);
  assert.deepEqual(withGate(['Bash(git push *)']), ['Bash(git push *)']);
});

test('Never allowed is the deny list without the who-commits rules', () => {
  assert.deepEqual(extraDeny([...GIT_BOARD, ...PERSON]), GIT_BOARD);
  assert.deepEqual(extraDeny(['Bash(git push *)']), []);
});

const check = (name: string, ok: boolean, fix = ''): WtxCheck => ({ name, ok, detail: '', fix, hard: true });
const doctor = (...checks: WtxCheck[]): Worktrees => ({ local: true, wtx: true, config: true, doctor: { ok: true, checks } });

test('the tools line: all there, no wtx, the first missing tool with its fix', () => {
  assert.deepEqual(toolsLine(doctor(check('wt', true), check('tmux', true))), { ok: true, text: 'wtx, wt and tmux on this machine' });
  assert.deepEqual(toolsLine({ local: true, wtx: false, config: false }), { ok: false, text: 'wtx is not on PATH where git board serve runs.' });
  assert.deepEqual(toolsLine(doctor(check('wt', true), check('tmux', false, 'brew install tmux'))), { ok: false, text: 'tmux missing: brew install tmux' });
  // inside a sandbox the tmux server is out of reach: not a missing tool
  assert.deepEqual(toolsLine(doctor(check('wt', true), check('tmux', true), check('tmux server reachable', false))).ok, true);
});

const session = (live: boolean, state: string): WtxSession => ({ branch: 'fix/1-x', path: '/w', main: false, session: 'r/fix/1-x', live, state, agent: 'claude', llm: 'opus' });

test('the agent in words, from its worktree and its session', () => {
  assert.equal(agentWords(undefined, false), 'No worktree');
  assert.equal(agentWords(undefined, true), 'Worktree, no session');
  assert.equal(agentWords(session(false, 'idle'), true), 'Session down');
  assert.equal(agentWords(session(true, 'permission'), true), 'Agent asks permission');
  assert.equal(agentWords(session(true, 'idle'), true), 'Agent waits for a brief');
  assert.equal(agentWords(session(true, 'stop'), true), 'Agent done, waits for you');
  assert.equal(agentWords(session(true, 'running'), true), 'Agent running');
  assert.equal(agentWords(session(true, 'start'), true), 'Agent running');
  assert.equal(agentWords(session(true, ''), true), 'Session up');
});

test('the button: none when refused, Brief with a worktree, Start without', () => {
  const a: IssueAgent = { branch: 'fix/1-x' };
  assert.equal(agentButton({ ...a, reason: 'exposed' }), null);
  assert.equal(agentButton({ ...a, worktree: { path: '/w', head: 'abc' } }), 'brief');
  assert.equal(agentButton(a), 'start');
  assert.equal(sessionName({ ...a, session: session(true, '') }), 'r/fix/1-x');
  assert.equal(sessionName(a), '');
});

test('the confirm names every file wtx touches, and says replaces when wtx.toml is there', () => {
  assert.equal(writeConfirm('fx', false), "Writes wtx.toml and .wt.toml, and adds wtx's lines to .gitignore, in fx's main checkout, uncommitted.");
  assert.equal(writeConfirm('', true), "Replaces wtx.toml and .wt.toml, and adds wtx's lines to .gitignore, in the repository's main checkout, uncommitted.");
});

test("the Agent select lists the tool wtx.toml names, even one it doesn't know", () => {
  const known = [{ value: 'claude', label: 'Claude Code' }, { value: 'opencode', label: 'opencode' }];
  assert.deepEqual(agentTools('claude'), known);
  assert.deepEqual(agentTools('aider'), [...known, { value: 'aider', label: 'aider (from wtx.toml)' }]);
});

test('a preview goes stale once the answers change, a written file never does', () => {
  const answers = { repo: { base_branch: 'dev' } };
  const shown = previewOf(answers);
  assert.equal(previewStale(shown, answers), false);
  answers.repo.base_branch = 'main';
  assert.equal(previewStale(shown, answers), true);
  assert.equal(previewStale('', answers), false);
});
