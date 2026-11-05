// Settings in tabs, each with its address: #/settings/<tab>. No tab, or
// one this list doesn't know, is the Sync. Pure, no "@/" imports: node
// runs settingstabs.test.ts on it.

// the tabs by where they are kept (each menu item says where it stands:
// no Overview repeating them): the project file (.git-board.toml) first,
// the Sync with it, for the forges are its tables, the raw file last;
// then what lives elsewhere, each saying where: where the board is kept,
// in git config; the numbers in git; you in git config and this browser
export const settingsTabs = [
  { id: 'sync', label: 'Sync', group: 'The project file', via: 'remotes, forges' },
  { id: 'fields', label: 'Fields', group: 'The project file' },
  { id: 'labels', label: 'Labels', group: 'The project file' },
  { id: 'views', label: 'Views', group: 'The project file' },
  { id: 'sprints', label: 'Sprints', group: 'The project file' },
  { id: 'roles', label: 'Roles', group: 'The project file' },
  { id: 'branches', label: 'Branches', group: 'The project file' },
  { id: 'cycle', label: 'Cycle', group: 'The project file', feature: 'cycle' },
  { id: 'release', label: 'Release', group: 'The project file' },
  { id: 'repos', label: 'Repos', group: 'The project file' },
  { id: 'attachments', label: 'Attachments', group: 'The project file' },
  { id: 'experimental', label: 'Experimental', group: 'The project file' },
  { id: 'advanced', label: 'The file', group: 'The project file', via: '.git-board.toml' },
  { id: 'datastore', label: 'Datastore', group: 'Kept elsewhere', via: 'git config board.datastore' },
  { id: 'numbers', label: 'Numbers', group: 'Kept elsewhere', via: 'refs/board/seq' },
  { id: 'you', label: 'You', group: 'Kept elsewhere', via: 'git config, this browser' },
] as const;

export type SettingsTab = (typeof settingsTabs)[number]['id'];

// old tabs' links: General held who you write as; Remotes, Forges and the
// GitHub Project are one Sync; the Project file is a tab per section, the
// fields first
// Forge fields: where a field lives is edited on the field (docs/prd/fields-one-editor.md)
const moved: Record<string, SettingsTab> = { overview: 'sync', general: 'you', remotes: 'sync', forges: 'sync', 'github-project': 'sync', 'project-file': 'fields', 'forge-fields': 'fields' };

// a tab of an experimental feature (experimental.ts) shows while it is on:
// on tells which are, all of them when not given
type FeatureOn = (feature: string) => boolean;
const allOn: FeatureOn = () => true;
const shown = (t: (typeof settingsTabs)[number], on: FeatureOn) => !('feature' in t) || on(t.feature);

// the tabs the menu shows
export function visibleTabs(on: FeatureOn = allOn) {
  return settingsTabs.filter((t) => shown(t, on));
}

// settingsTab is the tab of the route's :tab param; a feature's tab while
// it is off is the Sync
export function settingsTab(param: unknown, on: FeatureOn = allOn): SettingsTab {
  if (typeof param === 'string' && moved[param]) return moved[param];
  return visibleTabs(on).find((t) => t.id === param)?.id ?? 'sync';
}

// the groups, in order, as the sidebar shows them: a run of tabs under one
// heading, the ungrouped ones run together where they sit
export function settingsGroups(on: FeatureOn = allOn): { group: string; tabs: { id: SettingsTab; label: string; via?: string }[] }[] {
  const out: { group: string; tabs: { id: SettingsTab; label: string; via?: string }[] }[] = [{ group: '', tabs: [] }];
  for (const t of visibleTabs(on)) {
    const group = 'group' in t && t.group ? t.group : '';
    let g = out[out.length - 1];
    if (!g || g.group !== group) out.push((g = { group, tabs: [] }));
    g.tabs.push(t);
  }
  return out.filter((g) => g.tabs.length);
}

// syncSummary is the Sync's line in the menu: "origin, 2 forges with
// sync on."
export function syncSummary(s: { remotes: string[]; forges: { sync: boolean }[] }): string {
  const on = s.forges.filter((f) => f.sync).length;
  return `${[
    s.remotes.includes('origin') ? 'origin' : s.remotes[0] ?? 'No remote: the issues are only on this computer',
    on ? `${on} forge${on === 1 ? '' : 's'} with sync on` : 'no forge with sync on',
  ].join(', ')}.`;
}
