// Settings in tabs, each with its address: #/settings/<tab>. No tab, or
// one this list doesn't know, is the Overview. Pure, no "@/" imports: node
// runs settingstabs.test.ts on it.

// the Overview first: what git-board is and where things stand; then where
// your issues go, one Sync; the project file, a tab per section, the raw
// file last under Advanced; the appearance, which few change, last under You
export const settingsTabs = [
  { id: 'overview', label: 'Overview' },
  { id: 'sync', label: 'Sync', via: 'git, forges, Project' },
  { id: 'forge-fields', label: 'Forge fields', group: 'The project file', via: 'where each field lives' },
  { id: 'fields', label: 'Fields', group: 'The project file' },
  { id: 'labels', label: 'Labels', group: 'The project file' },
  { id: 'views', label: 'Views', group: 'The project file' },
  { id: 'sprints', label: 'Sprints', group: 'The project file' },
  { id: 'roles', label: 'Roles', group: 'The project file' },
  { id: 'branches', label: 'Branches', group: 'The project file' },
  { id: 'cycle', label: 'Cycle', group: 'The project file' },
  { id: 'release', label: 'Release', group: 'The project file' },
  { id: 'repos', label: 'Repos', group: 'The project file' },
  { id: 'attachments', label: 'Attachments', group: 'The project file' },
  { id: 'numbers', label: 'Numbers', group: 'The project file' },
  { id: 'worktrees', label: 'Worktrees', group: 'The project file' },
  { id: 'advanced', label: 'Advanced', group: 'The project file', via: 'the file itself' },
  { id: 'you', label: 'You' },
] as const;

export type SettingsTab = (typeof settingsTabs)[number]['id'];

// old tabs' links: General held who you write as; Remotes, Forges and the
// GitHub Project are one Sync; the Project file is a tab per section, the
// fields first
const moved: Record<string, SettingsTab> = { general: 'you', remotes: 'sync', forges: 'sync', 'github-project': 'sync', 'project-file': 'fields' };

// settingsTab is the tab of the route's :tab param
export function settingsTab(param: unknown): SettingsTab {
  if (typeof param === 'string' && moved[param]) return moved[param];
  return settingsTabs.find((t) => t.id === param)?.id ?? 'overview';
}

// the groups, in order, as the sidebar shows them: a run of tabs under one
// heading, the ungrouped ones run together where they sit
export function settingsGroups(): { group: string; tabs: { id: SettingsTab; label: string; via?: string }[] }[] {
  const out: { group: string; tabs: { id: SettingsTab; label: string; via?: string }[] }[] = [{ group: '', tabs: [] }];
  for (const t of settingsTabs) {
    const group = 'group' in t && t.group ? t.group : '';
    let g = out[out.length - 1];
    if (!g || g.group !== group) out.push((g = { group, tabs: [] }));
    g.tabs.push(t);
  }
  return out;
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
