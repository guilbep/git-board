// actThenRead runs a GitHub action of the PR gate (Open PR, Merge), then
// re-reads the gate: the message to show is the action's when it failed
// (GitHub's refusal), else the read's, so the read can't wipe it.
export async function actThenRead(act: () => Promise<unknown>, read: () => Promise<string>): Promise<string> {
  let failed = '';
  try {
    await act();
  } catch (e) {
    failed = e instanceof Error ? e.message : String(e);
  }
  const unread = await read();
  return failed || unread;
}

type Branch = { name: string; remote?: string; in_base: boolean };

// needsPush tells whether a branch is one to push before a PR can open:
// only here (origin has none of that name), its work not in the base yet,
// and origin on GitHub (repo). A merged branch whose origin copy was
// deleted is done, not waiting for a push.
export function needsPush(b: Branch, all: Branch[], repo: string): boolean {
  return !!repo && !b.remote && !b.in_base && !all.some((o) => o.remote === 'origin' && o.name === b.name);
}
