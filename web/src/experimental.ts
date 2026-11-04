// The work-in-progress features, behind a switch (docs/guide/configuration.md,
// Experimental features): the cycle, the coding agents, the worktrees. Off,
// the page hides their tabs, pages and controls; /api/me lists those on.
// Pure, no "@/" imports: node runs experimental.test.ts on it.

export const features = ['cycle', 'agents', 'worktrees'] as const;
export type Feature = (typeof features)[number];

// featureOn: the feature is in the list /api/me answered (none: off)
export function featureOn(on: readonly string[] | null | undefined, f: Feature): boolean {
  return !!on?.includes(f);
}

// toggled is the [experimental] features list with f turned on or off, in
// the features' order, the names the page doesn't know kept
export function toggled(list: readonly string[], f: Feature, on: boolean): string[] {
  const set = new Set(list);
  if (on) set.add(f);
  else set.delete(f);
  const known = features.filter((k) => set.has(k));
  return [...known, ...[...set].filter((k) => !(features as readonly string[]).includes(k))];
}

// whereFrom says what decides the features in this clone: its own
// board.experimental, which wins over the file, or the file
export function whereFrom(local: readonly string[] | null | undefined): string {
  if (local == null) return 'The whole team’s: .git-board.toml’s [experimental] features.';
  return `This clone’s own git config board.experimental (${local.length ? local.join(', ') : 'empty'}) wins over the file here: the toggles write the file, for the team.`;
}
