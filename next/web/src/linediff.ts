// lineDiff: the lines of a and b, each kept (' '), removed ('-') or added
// ('+'), from their longest common subsequence. For texts of a few hundred
// lines (a comment, .git-board.toml).
// ponytail: O(n·m) table, fine under ~2,000 lines a side.
export type DiffLine = { op: ' ' | '-' | '+'; text: string };

export function lineDiff(a: string, b: string): DiffLine[] {
  const x = a === '' ? [] : a.split('\n');
  const y = b === '' ? [] : b.split('\n');
  const n = x.length;
  const m = y.length;
  // lcs[i][j]: length of the longest common subsequence of x[i:] and y[j:]
  const lcs: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      lcs[i]![j] = x[i] === y[j] ? lcs[i + 1]![j + 1]! + 1 : Math.max(lcs[i + 1]![j]!, lcs[i]![j + 1]!);
    }
  }
  const out: DiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (x[i] === y[j]) {
      out.push({ op: ' ', text: x[i]! });
      i++;
      j++;
    } else if (lcs[i + 1]![j]! >= lcs[i]![j + 1]!) {
      out.push({ op: '-', text: x[i++]! });
    } else {
      out.push({ op: '+', text: y[j++]! });
    }
  }
  while (i < n) out.push({ op: '-', text: x[i++]! });
  while (j < m) out.push({ op: '+', text: y[j++]! });
  return out;
}
