import { test } from 'node:test';
import assert from 'node:assert/strict';
import { cells, controls, criteria, prLine, ranAtHead } from './cycleact.ts';
import type { Cycle, GatePR } from './api/cycletypes.ts';

const stages = { ready: 'ready', in_progress: ['in-progress'], in_review: ['in-review'], validation: ['qa'], closing: 'done', criteria: ['Done when'], kinds: {}, guessed: false };
const cycle = (over: Partial<Cycle>): Cycle => ({ step: 1, at: 'intent', of: 7, name: 'Intent', stage: 'ready', done: false, gate: '', waits: { what: '', anyone: true }, mine: true, evidence: {}, stages, ...over });

test('the cells: before the step done, low risk’s Plan skipped, after it ahead', () => {
  const c = cells(cycle({ step: 4, at: 'tests', skipped: [3] }));
  assert.deepEqual(c.map((x) => `${x.n}${x.name}:${x.state}`), ['1Intent:done', '2Risk:done', '3Plan:skipped', '4Tests:current', '5Code:ahead', '6Review:ahead', '7Verify:ahead']);
  assert.ok(cells(cycle({ done: true, at: 'verify' })).every((x) => x.state === 'done'));
});

test('the controls follow the actions the server offers', () => {
  assert.deepEqual(controls(cycle({ actions: ['set-risk'] })), [{ kind: 'risk' }]);
  assert.deepEqual(controls(cycle({ actions: ['start'] })), [{ kind: 'press', action: 'start', label: 'Start: move to in-progress' }]);
  assert.deepEqual(controls(cycle({ actions: ['verify', 'check', 'signoff'] })), [{ kind: 'verify' }]);
  assert.deepEqual(controls(cycle({ actions: ['to-validation'] })), [{ kind: 'press', action: 'to-validation', label: 'Move to qa' }]);
  assert.deepEqual(controls(cycle({})), []);
});

test('the criteria carry their last check', () => {
  const c = cycle({ evidence: { criteria: ['it saves', 'it loads'], checks: [{ index: 1, by: 'ann', at: '' }] } });
  assert.deepEqual(criteria(c), [{ index: 0, text: 'it saves' }, { index: 1, text: 'it loads', by: 'ann' }]);
});

test('a run at the head needs no reason to sign off; none at all does', () => {
  assert.equal(ranAtHead(cycle({ evidence: { head: 'abc', runs: [{ by: 'a', commit: 'abc', at: '' }] } })), true);
  assert.equal(ranAtHead(cycle({ evidence: { head: 'abc', runs: [{ by: 'a', commit: 'old', at: '' }] } })), false);
  assert.equal(ranAtHead(cycle({ evidence: {} })), false); // no run at all
});

test('a PR as its chip says it', () => {
  const pr = { number: 7, state: 'OPEN', draft: true } as GatePR;
  assert.equal(prLine(pr), '#7 open, draft');
  assert.equal(prLine({ ...pr, state: 'MERGED', draft: false }), '#7 merged');
});
