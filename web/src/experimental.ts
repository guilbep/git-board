// The work-in-progress features, behind a switch (docs/guide/configuration.md,
// Experimental features): the cycle. Off, the page hides its tabs and
// controls; GET /api/board lists those on. (v1's agents and worktrees went
// to their own binary, ADR 0021.) Pure, no "@/" imports: node runs
// experimental.test.ts on it.

export const features = ['cycle'] as const;
export type Feature = (typeof features)[number];

// every feature, as Settings › Experimental lists them
export const known = [
  { name: 'cycle', title: 'The development cycle', description: 'An issue’s steps from intent to release: the plan, the code, the review, the tests, the verification.' },
] as const;

// featureOn: the feature is in the list /api/board answered (none: off)
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
