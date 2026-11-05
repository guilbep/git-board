// Only the latest answer may write: of two requests for one thing, the one
// asked last wins, whichever answers last.
//
//   const seq = latestOnly();
//   const current = seq.start();
//   const data = await fetchIt();
//   if (current()) shown.value = data;
//
// skip() makes every request under way stale, as when a write answered
// with fresher data than a read asked before it.

export function latestOnly(): { start: () => () => boolean; skip: () => void } {
  let n = 0;
  return {
    start: () => {
      const mine = ++n;
      return () => mine === n;
    },
    skip: () => void n++,
  };
}
