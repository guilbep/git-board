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
