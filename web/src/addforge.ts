// Add a forge, in four steps: the forge, the project, the token, the
// check (POST /api/bridges, where the forge checks the token sees the
// project before anything is kept). Pure, no "@/" imports: node runs
// addforge.test.ts on it.
import type { Credential } from './api/board';

// ponytail: the three forges git-bug's sync supports (store.AddBridge
// refuses the others); GET /api/forges lists bridges, not forge kinds
// (token: what docs/forges.md says it needs, in short)
export const forges = [
  { target: 'github', name: 'GitHub', short: 'gh', placeholder: 'https://github.com/owner/repo', token: 'a classic token with the repo scope' },
  { target: 'gitlab', name: 'GitLab', short: 'gl', placeholder: 'https://gitlab.epfl.ch/group/project', token: 'a token with the api scope' },
  { target: 'gitea', name: 'Gitea', short: 'gitea', placeholder: 'https://gitea.example.org/owner/repo', token: 'a token with read and write on issues and repositories' },
];

export const forgeSteps = ['Forge', 'Project', 'Token', 'Check'];

export interface ForgeDraft {
  target: string;
  url: string;
  name: string; // empty: the server picks gh, gh2…
  credential: string; // a stored one's id, or empty for a pasted token
  token: string;
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
  if (host.includes('gitea')) return 'gitea';
  return '';
}

// matchingCredentials are the stored tokens of the forge, of its instance
// once the address says it
export function matchingCredentials(creds: Credential[], target: string, url: string): Credential[] {
  const host = target === 'github' ? 'github.com' : hostOf(url);
  return creds.filter((c) => c.target === target && (!host || c.host === host));
}

const bridgeName = /^[A-Za-z0-9][A-Za-z0-9_-]*$/; // as store.AddBridge

// canGoOn says whether a step is filled enough for the next one
export function canGoOn(step: number, d: ForgeDraft): boolean {
  switch (step) {
    case 0:
      return forges.some((f) => f.target === d.target);
    case 1: {
      const u = parse(d.url);
      const project = u && /^https?:$/.test(u.protocol) && u.pathname.split('/').filter(Boolean).length >= 2;
      return !!project && (!d.name.trim() || bridgeName.test(d.name.trim()));
    }
    case 2:
      return !!d.credential || !!d.token.trim();
    default:
      return true;
  }
}
