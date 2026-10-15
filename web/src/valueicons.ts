// valueIcon is the icon of a field value, picked from its words as the
// server picks colors (config.Colors): where a status stands (backlog to
// done), a priority, a kind of work. '' when the value names a thing (a
// sprint, a person) rather than a state. Order matters: the first rule a
// word matches wins, so ready-for-validation is in review, not ready.
const rules: [icon: string, words: string[]][] = [
  ['st-blocked', ['blocked', 'blocker', 'stuck']],
  ['st-done', ['done', 'validated', 'closed', 'shipped', 'merged', 'complete', 'completed', 'resolved', 'released']],
  ['st-review', ['review', 'qa', 'validation', 'approval']],
  ['st-test', ['test', 'tests', 'testing']],
  ['st-progress', ['progress', 'doing', 'wip', 'active', 'started', 'ongoing']],
  ['st-waiting', ['waiting', 'wait', 'hold', 'pending', 'comite', 'comité', 'committee', 'triage', 'paused']],
  ['st-ready', ['ready', 'todo', 'planned', 'next']],
  ['st-backlog', ['backlog', 'new', 'icebox', 'someday', 'draft', 'idea', 'later']],
  ['prio-urgent', ['p0', 'urgent', 'critical']],
  ['prio-high', ['p1', 'high']],
  ['prio-medium', ['p2', 'medium', 'normal']],
  ['prio-low', ['p3', 'p4', 'low', 'minor']],
  ['kind-bug', ['bug', 'fix', 'defect', 'regression']],
  ['kind-feat', ['feat', 'feature', 'enhancement']],
  ['kind-doc', ['doc', 'docs', 'documentation']],
  ['kind-chore', ['chore', 'refactor', 'maintenance', 'ci', 'build']],
];

export function valueIcon(_field: string, value: string): string {
  const words = value.toLowerCase().split(/[^\p{L}\p{N}]+/u);
  for (const [icon, list] of rules) if (words.some((w) => list.includes(w))) return icon;
  return '';
}
