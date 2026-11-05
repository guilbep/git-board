// donutSlices: each entry's share of a ring of outer radius r and inner
// radius inner, centred on 0,0, as an SVG path; empty entries left out.
export type Slice = { key: string; n: number; pct: number; d: string };

export function donutSlices(entries: { key: string; n: number }[], r: number, inner: number): Slice[] {
  const kept = entries.filter((e) => e.n > 0);
  const total = kept.reduce((s, e) => s + e.n, 0);
  if (!total) return [];
  const pt = (rad: number, a: number) => `${(rad * Math.sin(a)).toFixed(2)} ${(-rad * Math.cos(a)).toFixed(2)}`;
  if (kept.length === 1) {
    // a whole ring: two half arcs each way, the hole cut by the even-odd rule
    const e = kept[0]!;
    return [{ key: e.key, n: e.n, pct: 100, d: `M0 ${-r}A${r} ${r} 0 1 1 0 ${r}A${r} ${r} 0 1 1 0 ${-r}ZM0 ${-inner}A${inner} ${inner} 0 1 0 0 ${inner}A${inner} ${inner} 0 1 0 0 ${-inner}Z` }];
  }
  let a = 0;
  return kept.map((e) => {
    const b = a + (e.n / total) * 2 * Math.PI;
    const large = b - a > Math.PI ? 1 : 0;
    const d = `M${pt(r, a)}A${r} ${r} 0 ${large} 1 ${pt(r, b)}L${pt(inner, b)}A${inner} ${inner} 0 ${large} 0 ${pt(inner, a)}Z`;
    a = b;
    return { key: e.key, n: e.n, pct: Math.round((e.n / total) * 100), d };
  });
}
