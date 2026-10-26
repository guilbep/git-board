// Settings in tabs, each with its address: #/settings/<id>. No tab, or
// one this list doesn't know, is the Overview. Pure, no "@/" imports: node
// runs settingstabs.test.ts on it.

// the Overview first: what git-board is and where things stand; then where
// your issues go, one Sync; the appearance, which few change, last under You
export const settingsTabs = [
  { id: 'overview', label: 'Overview' },
  { id: 'sync', label: 'Sync' },
  { id: 'project-file', label: 'Project file' },
  { id: 'labels', label: 'Labels' },
  { id: 'numbers', label: 'Numbers' },
  { id: 'attachments', label: 'Attachments' },
  { id: 'worktrees', label: 'Worktrees' },
  { id: 'you', label: 'You' },
] as const;

export type SettingsTab = (typeof settingsTabs)[number]['id'];

// old tabs' links: General held who you write as; Remotes, Forges and the
// GitHub Project are one Sync
const moved: Record<string, SettingsTab> = { general: 'you', remotes: 'sync', forges: 'sync', 'github-project': 'sync' };

// settingsTab is the tab of the route's :tab param
export function settingsTab(param: unknown): SettingsTab {
  if (typeof param === 'string' && moved[param]) return moved[param];
  return settingsTabs.find((t) => t.id === param)?.id ?? 'overview';
}

// syncSummary is the Overview's line for the Sync: "origin, 2 forges with
// sync on, the GitHub Project EPFL-ENAC/40."
export function syncSummary(s: { remote: { name: string } | null; forges: { sync: boolean }[]; project: { project: string } }): string {
  const on = s.forges.filter((f) => f.sync).length;
  const parts = [
    s.remote ? s.remote.name : 'No origin: the issues are only on this computer',
    on ? `${on} forge${on === 1 ? '' : 's'} with sync on` : 'no forge with sync on',
  ];
  if (s.project.project) parts.push(`the GitHub Project ${s.project.project}`);
  return `${parts.join(', ')}.`;
}
