// Add a forge no git remote names: the forge, then the project and a
// token, sent to POST /api/forges (api.md, 33), which writes its
// [forges.<name>] table and gives the token to git's credential helper,
// never the file (ADR 0010).
// Pure, no "@/" imports: node runs addforge.test.ts on it.

// the two forges the sync supports (token: what docs/guide/forges.md says
// it needs, in short)
export const forges = [
  { target: 'github', name: 'GitHub', short: 'gh', placeholder: 'https://github.com/owner/repo', token: 'a classic token with the repo scope' },
  { target: 'gitlab', name: 'GitLab', short: 'gl', placeholder: 'https://gitlab.epfl.ch/group/project', token: 'a token with the api scope' },
];

export const forgeSteps = ['Forge', 'Project'];

export interface ForgeDraft {
  target: string;
  url: string;
  name: string; // empty: gh, gh2… the first free
  token: string; // empty: none stored
  way: string; // push (a backup), pull (read-only), or both ('')
}

// the ways of a mirror ([forges.<name>] way, ADR 0022), in words
export const forgeWays = [
  { way: 'both', name: 'both ways', hint: 'the sync pulls from it, then pushes to it' },
  { way: 'push', name: 'push only, a backup', hint: 'the issues go there, and nothing comes back; an edit made there is drift, reported and never taken' },
  { way: 'pull', name: 'read only', hint: 'the sync reads its issues, and writes nothing there' },
];

function parse(u: string): URL | null {
  try {
    return new URL(u.trim());
  } catch {
    return null;
  }
}

export function hostOf(u: string): string {
  return parse(u)?.host ?? '';
}

// guessForge is the forge an address says, '' when it doesn't
export function guessForge(url: string): string {
  const host = hostOf(url);
  if (host === 'github.com') return 'github';
  if (host.includes('gitlab')) return 'gitlab';
  return '';
}

const forgeName = /^[A-Za-z0-9][A-Za-z0-9_-]*$/;

// canGoOn says whether a step is filled enough for the next one
export function canGoOn(step: number, d: ForgeDraft): boolean {
  if (step === 0) return forges.some((f) => f.target === d.target);
  const u = parse(d.url);
  const project = u && /^https?:$/.test(u.protocol) && u.pathname.split('/').filter(Boolean).length >= 2;
  return !!project && (!d.name.trim() || forgeName.test(d.name.trim()));
}
