// The board page's issues, as v2's API pages them (docs/rewrite/api.md,
// List): never every issue at once. Ungrouped, one list; grouped, a list
// per value of the field (the lane's term added to the filter, "no:field"
// for the issues without one), each loading its first page and the next
// on demand. The values come from GET /api/labels for the filter, which
// also counts the filters panel (facets.ts facetsOf). A newer load aborts
// the older one's requests.
import { ref, shallowRef, type Ref } from 'vue';
import { getLabels, listIssues, type Issue, type LabelCount } from '@/api';
import type { Bucket } from '@/bucket';
import { configModel } from '@/configmodel';
import { facetsOf, type Counted, type Facet } from '@/facets';
import type { Fields } from '@/fields';
import { NONE, STATE, laneFilter, owns, rest, terms } from '@/filterterms';

export interface ListParams {
  filter: string;
  group: string; // a field, or '' for one list
  sort: string; // the API's: created, updated, title or a field, asc or desc
}

// a page: a table's 100 rows; a lane's 40 cards, a screen and more, times
// the lanes
export const PAGE = 100;
export const LANE_PAGE = 40;

// apiSort is a sort as the page holds it ({key: prio, desc}) for the API:
// "prio desc"; '' for an order the API doesn't have (id, state, links).
// A field the server doesn't know answers invalid, said over the list.
export function apiSort(s: { key: string; desc: boolean } | null): string {
  if (!s || ['id', 'is', 'links', 'other'].includes(s.key)) return '';
  return `${s.key} ${s.desc ? 'desc' : 'asc'}`;
}

export function useBoardLists(fields: Ref<Fields>) {
  const buckets = shallowRef<Bucket[] | null>(null); // null until a load lands
  const total = ref(0);
  const loading = ref(false);
  const error = ref('');
  const facets = shallowRef<Facet[]>([]);
  let params: ListParams = { filter: '', group: '', sort: '' };
  let abort: AbortController | null = null;

  const page = (filter: string, signal: AbortSignal, cursor?: string) =>
    listIssues({ filter, sort: params.sort || undefined, limit: params.group ? LANE_PAGE : PAGE, cursor }, signal);

  // the group field's options, known before any count: the fields', else
  // the file's (the page's fields come beside the first list)
  const optionsOf = (g: string): string[] => (g === STATE ? ['open', 'closed'] : (fields.value[g]?.options ?? configModel.value?.fields[g]?.options ?? []));

  async function load(p: ListParams): Promise<void> {
    params = p;
    abort?.abort();
    const ctl = (abort = new AbortController());
    const signal = ctl.signal;
    loading.value = true;
    error.value = '';
    try {
      // ungrouped, the list goes first, alone: the counts after it
      let labels: Promise<LabelCount[]> | undefined;
      let next: Bucket[];
      if (!p.group) {
        const first = await page(p.filter, signal);
        next = [{ group: '', issues: first.items, total: first.total, next: first.next }];
        total.value = first.total;
      } else {
        // the options' lanes and the counts at once; then a lane for each
        // value in use that no option names
        const lane = (v: string) => page(laneFilter(p.filter, p.group, v), signal).then((pg): Bucket => ({ group: v, issues: pg.items, total: pg.total, next: pg.next }));
        const options = [...optionsOf(p.group), NONE];
        labels = getLabels(p.filter, signal);
        const all = listIssues({ filter: p.filter, limit: 1 }, signal);
        const known = Promise.all(options.map(lane));
        const extra = (await labels).filter((l) => l.field === p.group && !options.includes(l.value)).map((l) => l.value);
        next = [...(await known), ...(await Promise.all([...new Set(extra)].map(lane)))];
        total.value = (await all).total;
      }
      if (signal.aborted) return;
      buckets.value = next;
      loading.value = false;
      void loadFacets(p.filter, labels ?? getLabels(p.filter, signal), signal);
    } catch (e) {
      if (signal.aborted) return;
      error.value = e instanceof Error ? e.message : String(e);
      buckets.value = null;
      loading.value = false;
    }
  }

  // the filters panel: the shared counts, and each facet the filter has a
  // term on counted against the rest of it. Open and closed are asked
  // only where the state facet reads them, labels only where a field's
  // does: on is:open (a view's default) that is the list, its labels and
  // two counts
  async function loadFacets(filter: string, labels: Promise<LabelCount[]>, signal: AbortSignal) {
    // how many a filter matches: the list's own total when it is the
    // list's filter (is:open, a view's default), else asked
    const same = (a: string, b: string) => terms(a).map((t) => t.text).sort().join(' ') === terms(b).map((t) => t.text).sort().join(' ');
    const count = (f: string) => (same(f, filter) ? Promise.resolve(total.value) : listIssues({ filter: f, limit: 1 }, signal).then((x) => x.total));
    const totals = (f: string) => Promise.all([count(`${f} is:open`.trim()), count(`${f} is:closed`.trim())]);
    const counted = async (f: string, ls: Promise<LabelCount[]> | null, states: boolean): Promise<Counted> => {
      const [l, [open = 0, closed = 0]] = await Promise.all([ls ?? Promise.resolve([]), states ? totals(f) : Promise.resolve([0, 0])]);
      return { labels: l, open, closed };
    };
    try {
      const ts = terms(filter);
      const keys = new Set<string>();
      for (const t of ts) for (const k of [STATE, t.key === 'no' ? t.value : t.key === 'label' ? 'label' : t.key]) if (k && owns(k, t)) keys.add(k);
      // the shared counts: open and closed when the state facet is no own
      // one, else this list's total for a field's "not set"
      const sharedStates = !keys.has(STATE);
      const shared = counted(filter, labels, sharedStates).then((c) => (sharedStates ? c : { ...c, open: total.value, closed: 0 }));
      const own = new Map(await Promise.all([...keys].map(async (k) => [k, await counted(rest(filter, k), k === STATE ? null : getLabels(rest(filter, k), signal), true)] as const)));
      const fs = facetsOf(filter, fields.value, await shared, own);
      if (!signal.aborted) facets.value = fs;
    } catch {
      // the panel keeps what it showed
    }
  }

  // more loads a bucket's next page
  async function more(b: Bucket): Promise<void> {
    if (!b.next || b.loading) return;
    b.loading = true;
    const filter = params.group ? laneFilter(params.filter, params.group, b.group) : params.filter;
    try {
      const pg = await listIssues({ filter, sort: params.sort || undefined, limit: params.group ? LANE_PAGE : PAGE, cursor: b.next });
      const seen = new Set(b.issues.map((i) => i.id));
      const fresh: Bucket = { ...b, issues: [...b.issues, ...pg.items.filter((i) => !seen.has(i.id))], total: pg.total, next: pg.next, loading: false };
      buckets.value = (buckets.value ?? []).map((x) => (x === b ? fresh : x));
    } catch (e) {
      b.loading = false;
      error.value = e instanceof Error ? e.message : String(e);
    }
  }

  // moved: a card's lane changed here before the server said so
  function moved(issue: Issue, from: string, to: string) {
    buckets.value = (buckets.value ?? []).map((b) =>
      b.group === from ? { ...b, issues: b.issues.filter((i) => i.id !== issue.id), total: (b.total ?? 1) - 1 }
      : b.group === to ? { ...b, issues: [issue, ...b.issues], total: (b.total ?? 0) + 1 }
      : b,
    );
  }

  return { buckets, total, loading, error, facets, load, more, moved, reload: () => load(params) };
}
