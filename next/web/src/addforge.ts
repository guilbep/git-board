// Add a forge no git remote names: the forge, then the project, written as
// its [forges.<name>] table (PATCH /api/config). The token stays out of
// the file: git's credential helper, or GH_TOKEN and GITLAB_TOKEN (ADR
// 0010); v2's API takes one with POST /api/forges (later: api.md, 33).
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
}

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

// tableOf is the draft as its table: its name (the draft's, else the
// forge's short one, then 2, 3… past those taken), kind, project, host
// when not the forge's own
export function tableOf(d: ForgeDraft, taken: string[], sync: boolean): { name: string; table: Record<string, unknown> } {
  const f = forges.find((x) => x.target === d.target)!;
  let name = d.name.trim();
  for (let i = 1; !name || (taken.includes(name) && !d.name.trim()); i++) name = i === 1 ? f.short : `${f.short}${i}`;
  const u = parse(d.url)!;
  const project = u.pathname.replace(/\.git$/, '').replace(/\/-\/.*$/, '').split('/').filter(Boolean).join('/');
  const table: Record<string, unknown> = { kind: d.target, project, sync };
  if (u.host !== (d.target === 'github' ? 'github.com' : 'gitlab.com')) table.host = u.host;
  return { name, table };
}
