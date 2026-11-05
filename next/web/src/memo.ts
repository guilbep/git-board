// memo keeps an async answer by its key: asked again, the same promise,
// with no second request. A refusal is not kept, the next ask goes again.
// At most max keys: past them, the oldest goes.
export function memo<T>(fn: (key: string) => Promise<T>, max = 200): (key: string) => Promise<T> {
  const kept = new Map<string, Promise<T>>();
  return (key) => {
    const hit = kept.get(key);
    if (hit) return hit;
    const p = fn(key);
    kept.set(key, p);
    p.catch(() => kept.get(key) === p && kept.delete(key));
    if (kept.size > max) kept.delete(kept.keys().next().value!);
    return p;
  };
}
