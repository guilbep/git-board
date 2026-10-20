// Settings in tabs, each with its address: #/settings/<id>. No tab, or
// one this list doesn't know, is the Overview. Pure, no "@/" imports: node
// runs settingstabs.test.ts on it.

// where your issues go: three channels, each a tab, in this order (the
// menu numbers them, ChannelMap shows them one line each)
export const channels = [
  { id: 'remotes', label: 'Remotes', via: 'git', moves: 'Code, branches, issues, people.', verbs: 'Check, Pull, Push.' },
  { id: 'forges', label: 'Forges', via: 'API', moves: 'A mirror of issues and comments.', verbs: 'Check, Get, Send, Sync.' },
  { id: 'github-project', label: 'GitHub Project', via: 'fields', moves: 'Status, priority, sprint, as labels.', verbs: 'Check, Pull fields.' },
] as const;

export type Channel = (typeof channels)[number]['id'];

// the Overview first: what git-board is and where things stand; the
// appearance, which few change, last under You
export const settingsTabs = [
  { id: 'overview', label: 'Overview' },
  ...channels,
  { id: 'project-file', label: 'Project file' },
  { id: 'labels', label: 'Labels' },
  { id: 'numbers', label: 'Numbers' },
  { id: 'attachments', label: 'Attachments' },
  { id: 'worktrees', label: 'Worktrees' },
  { id: 'you', label: 'You' },
] as const;

export type SettingsTab = (typeof settingsTabs)[number]['id'];

// settingsTab is the tab of the route's :tab param
export function settingsTab(param: unknown): SettingsTab {
  if (param === 'general') return 'you'; // its old links: who you write as
  return settingsTabs.find((t) => t.id === param)?.id ?? 'overview';
}

// forgeSummary and remoteSummary are the Overview's lines for them
export function forgeSummary(bridges: { sync: boolean }[]): string {
  const n = bridges.length;
  if (!n) return 'No forge yet.';
  const on = bridges.filter((b) => b.sync).length;
  const sync = on === n ? 'sync on' : on === 0 ? 'sync off' : `${on} with sync on`;
  return `${n} forge${n === 1 ? '' : 's'}, ${sync}.`;
}

export function remoteSummary(remotes: { name: string }[]): string {
  return remotes.length ? `${remotes.map((r) => r.name).join(', ')}.` : 'No remote yet: the issues are only on this computer.';
}
