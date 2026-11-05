// A branch's chip on the head's branch line (docs/design/issue-head/README.md,
// 16c and decision 5). Its tone says whose move it is: red the author's,
// yellow a reviewer's, blue the machines', green anyone may merge, grey a
// draft or a branch not in review, canard merged. Its icon and word say
// the state; its face names the branch (2771/2 in the issue's folder); its
// sentence, the chip's name and tooltip, says every wait in
// internal/ghpr's words. Pure: BranchLine reads the server.
import type { GatePR, IssueBranch } from './api/board';

export type Tone = 'author' | 'review' | 'checks' | 'ready' | 'draft' | 'merged' | 'closed' | 'branch';
export type ChipIcon = 'branch' | 'pull' | 'merge' | 'error' | 'clock' | 'eye' | 'check' | 'close' | 'upload' | 'edit' | 'warn' | 'arrow-down';

export interface BranchState {
  tone: Tone;
  icon: ChipIcon;
  word: string;
  who: string; // whose move: a login, or '' (the machines, anyone)
  face: string; // "2771/2", "#3155", or the branch's last part
  sentence: string;
}

export interface BranchItem {
  name: string;
  branch: IssueBranch;
  pr: GatePR | null;
  state: BranchState;
}

// The chip's face. In the issue's folder ({kind}/{number}/{n}-{slug}) the
// number and n; else the gate's number; else the name's last part.
export function faceOf(name: string, pr?: GatePR | null): string {
  const m = /(?:^|\/)(\d+)\/(\d+)(?:-|$)/.exec(name);
  if (m) return `${m[1]}/${m[2]}`;
  if (pr) return `#${pr.number}`;
  const last = name.slice(name.lastIndexOf('/') + 1);
  return last.length > 24 ? `${last.slice(0, 23)}…` : last;
}

// Its work is in the base: merged (the base moved on since) or squashed or
// rebased in. A new branch still on the base's commit is not merged work.
const inBase = (b: IssueBranch) => (b.merged && b.behind > 0) || (b.in_base && !b.merged);

function said(name: string, word: string, who: string, pr: GatePR | null, base: string): string {
  const parts = [`${name}: ${word}`];
  if (who) parts.push(`${who}’s move`);
  if (pr) parts.push(`the gate: GitHub #${pr.number}${pr.state === 'OPEN' ? ` into ${pr.base || base}` : ''}`);
  if (pr?.state === 'OPEN' && pr.waits.length) parts.push(`waits for ${pr.waits.join(', ')}`);
  return parts.join(' · ');
}

export function branchState(b: IssueBranch, onRemote: boolean, pr: GatePR | null, base: string): BranchState {
  const face = faceOf(b.name, pr);
  const make = (tone: Tone, icon: ChipIcon, word: string, who = ''): BranchState => ({ tone, icon, word, who, face, sentence: said(b.name, word, who, pr, base) });
  if (pr) {
    if (pr.state === 'MERGED') return make('merged', 'merge', 'merged');
    if (pr.state === 'CLOSED') return make('closed', 'close', 'closed');
    if (pr.draft) return make('draft', 'pull', 'draft', pr.author);
    if (pr.checks.some((c) => c.required && c.state === 'failure')) return make('author', 'error', 'checks fail', pr.author);
    if (pr.decision === 'CHANGES_REQUESTED') return make('author', 'edit', 'changes asked', pr.author);
    if (pr.mergeable === 'CONFLICTING') return make('author', 'warn', 'conflict', pr.author);
    if (pr.merge_state === 'BEHIND') return make('author', 'arrow-down', `behind ${pr.base || base}`, pr.author);
    if (pr.checking || pr.checks.some((c) => c.state === 'pending' || c.state === 'expected')) return make('checks', 'clock', 'checks run');
    if (pr.decision === 'REVIEW_REQUIRED') return make('review', 'eye', 'review', pr.requested[0] ?? '');
    if (pr.can_merge) return make('ready', 'check', 'ready');
    return make('review', 'clock', 'waiting');
  }
  if (inBase(b)) return make('merged', 'merge', 'merged');
  if (!onRemote) return make('branch', 'upload', 'local');
  return make('branch', 'branch', 'not in review');
}

// The issue's branches as chips: one per name (the remote's and the local
// copy are one branch), each with its gate when origin is on GitHub.
export function branchItems(branches: IssueBranch[], pulls: { branch: string; pr: GatePR | null }[] | undefined, base: string): BranchItem[] {
  const byName = new Map<string, IssueBranch[]>();
  for (const b of branches) byName.set(b.name, [...(byName.get(b.name) ?? []), b]);
  return [...byName].map(([name, copies]) => {
    const branch = copies.find((c) => c.remote === 'origin') ?? copies.find((c) => c.remote) ?? copies[0]!;
    const pr = pulls?.find((p) => p.branch === name)?.pr ?? null;
    return { name, branch, pr, state: branchState(branch, copies.some((c) => !!c.remote), pr, base) };
  });
}

const ORDER: Tone[] = ['author', 'review', 'checks', 'ready', 'draft', 'branch'];

// The line: red, yellow, blue, green, drafts, branches; `max` chips, the
// rest as more; merged and closed counted apart, at the end.
export function branchLine<T extends { state: BranchState }>(items: T[], max = 4): { chips: T[]; more: T[]; merged: T[]; closed: T[] } {
  const live = items.filter((i) => ORDER.includes(i.state.tone)).sort((a, b) => ORDER.indexOf(a.state.tone) - ORDER.indexOf(b.state.tone));
  return {
    chips: live.slice(0, max),
    more: live.slice(max),
    merged: items.filter((i) => i.state.tone === 'merged'),
    closed: items.filter((i) => i.state.tone === 'closed'),
  };
}

// The branch the create popover proposes, in the issue's folder (decision
// 3, settled 2026-10-30): {kind}/{number}/{n}-{slug} from the server's
// {kind}/{number}-{slug}, n one more than the highest among the issue's
// branches. A name of another shape is kept as the project file makes it.
// ponytail: the server's Suggest takes this over (plan W3-2), with the
// merge commits on the base counted too.
export function folderName(suggested: string, existing: string[]): string {
  if (/(?:^|\/)\d+\/\d+-/.test(suggested)) return suggested; // already in its folder
  const m = /^((?:[^/]+\/)*?)(\d+)-(.+)$/.exec(suggested);
  if (!m) return suggested;
  const [, kind, number, slug] = m;
  let n = 0;
  for (const name of existing) {
    const f = new RegExp(`(?:^|/)${number}/(\\d+)(?:-|$)`).exec(name);
    if (f) n = Math.max(n, Number(f[1]));
  }
  return `${kind}${number}/${n + 1}-${slug}`;
}
