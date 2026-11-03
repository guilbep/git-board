// Settings in tabs, each with its address: #/settings/<tab>. No tab, or
// one this list doesn't know, is the Sync. Pure, no "@/" imports: node
// runs settingstabs.test.ts on it.

// the tabs by where they are kept (each menu item says where it stands:
// no Overview repeating them): the project
// file (.git-board.toml) first, the Sync with it, for the forges and the
// GitHub Project are its tables, the raw file last; then what lives
// elsewhere, each saying where: the numbers in git, the worktrees in
// wtx.toml, you in git config and this browser
export const settingsTabs = [
  { id: 'sync', label: 'Sync', group: 'The project file', via: 'forges, Project' },
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
  { id: 'advanced', label: 'The file', group: 'The project file', via: '.git-board.toml' },
  { id: 'numbers', label: 'Numbers', group: 'Kept elsewhere', via: 'refs/board/seq' },
  { id: 'worktrees', label: 'Worktrees', group: 'Kept elsewhere', via: 'wtx.toml' },
  { id: 'you', label: 'You', group: 'Kept elsewhere', via: 'git config, this browser' },
] as const;

export type SettingsTab = (typeof settingsTabs)[number]['id'];

// old tabs' links: General held who you write as; Remotes, Forges and the
// GitHub Project are one Sync; the Project file is a tab per section, the
// fields first
// Forge fields: where a field lives is edited on the field (docs/prd/fields-one-editor.md)
const moved: Record<string, SettingsTab> = { overview: 'sync', general: 'you', remotes: 'sync', forges: 'sync', 'github-project': 'sync', 'project-file': 'fields', 'forge-fields': 'fields' };

// settingsTab is the tab of the route's :tab param
export function settingsTab(param: unknown): SettingsTab {
  if (typeof param === 'string' && moved[param]) return moved[param];
  return settingsTabs.find((t) => t.id === param)?.id ?? 'sync';
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
  return out.filter((g) => g.tabs.length);
}

// syncSummary is the Sync's line in the menu: "origin, 2 forges with
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
