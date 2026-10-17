// Settings in tabs, each with its address: #/settings/<id>. No tab, or
// one this list doesn't know (an old link), is General. Pure, no "@/"
// imports: node runs settingstabs.test.ts on it.

// where your issues go: three channels, each a tab, in this order (the
// menu numbers them, ChannelMap shows them one line each)
export const channels = [
  { id: 'remotes', label: 'Remotes', via: 'git', moves: 'Code, branches, issues, people.', verbs: 'Check, Pull, Push.' },
  { id: 'forges', label: 'Forges', via: 'API', moves: 'A mirror of issues and comments.', verbs: 'Check, Get, Send, Sync.' },
  { id: 'github-project', label: 'GitHub Project', via: 'fields', moves: 'Status, priority, sprint, as labels.', verbs: 'Check, Pull fields.' },
] as const;

export type Channel = (typeof channels)[number]['id'];

export const settingsTabs = [
  { id: 'general', label: 'General' },
  { id: 'project-file', label: 'Project file' },
  { id: 'attachments', label: 'Attachments' },
  ...channels,
] as const;

export type SettingsTab = (typeof settingsTabs)[number]['id'];

// settingsTab is the tab of the route's :tab param
export function settingsTab(param: unknown): SettingsTab {
  return settingsTabs.find((t) => t.id === param)?.id ?? 'general';
}
