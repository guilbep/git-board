// The board page's issues (ADR 0023): the page keeps every issue
// (local/store.ts); a list is the ids the server says it holds (GET
// /api/issues/ids: the filter grammar stays the server's), shown from the
// board kept here, a page at a time. Ungrouped, one list; grouped, a list
// per value of the field (the lane's term added to the filter, "no:field"
// for the issues without one), every lane and the list's total in one
// request. The values come from GET /api/labels for the filter, which
// also counts the filters panel (facets.ts facetsOf). A newer load aborts
// the older one's requests.
import { ref, shallowRef, type Ref } from 'vue';
import { batchIssues, getLabels, issueIDs, type Issue, type LabelCount } from '@/api';
import { current, get } from '@/local/store';
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

  // a bucket of ids, its first page shown from the board kept here
  const bucketOf = (group: string, ids: string[]): Bucket => {
    const n = params.group ? LANE_PAGE : PAGE;
    return { group, ids, issues: shown(ids.slice(0, n)), total: ids.length, next: ids.length > n ? String(n) : undefined };
  };
  const shown = (ids: string[]): Issue[] => ids.map(get).filter((i): i is Issue => !!i);

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
      await current(); // the board kept here, caught up with the events heard
      let labels: Promise<LabelCount[]> | undefined;
      let next: Bucket[];
      const sort = p.sort || undefined;
      if (!p.group) {
        const [ids = []] = (await issueIDs({ f: [p.filter], sort }, signal)).lists;
        next = [bucketOf('', ids)];
        total.value = ids.length;
      } else {
        // the options' lanes and the list itself in one request, the
        // labels beside it; then, in one more, a lane for each value in
        // use that no option names
        const options = [...optionsOf(p.group), NONE];
        labels = getLabels(p.filter, signal);
        const known = issueIDs({ f: [p.filter, ...options.map((v) => laneFilter(p.filter, p.group, v))], sort }, signal);
        // awaited below, after labels: a new load aborting it meanwhile
        // is this load's to catch, not an unhandled rejection of the page's
        known.catch(() => {});
        const extra = [...new Set((await labels).filter((l) => l.field === p.group && !options.includes(l.value)).map((l) => l.value))];
        const [k, x] = await Promise.all([known, extra.length ? issueIDs({ f: extra.map((v) => laneFilter(p.filter, p.group, v)), sort }, signal) : Promise.resolve({ lists: [] as string[][] })]);
        const [all = [], ...lanes] = k.lists;
        next = [...options.map((v, i) => bucketOf(v, lanes[i] ?? [])), ...extra.map((v, i) => bucketOf(v, x.lists[i] ?? []))];
        total.value = all.length;
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
    // list's filter (is:open, a view's default), else counted, every
    // count in one request
    const same = (a: string, b: string) => terms(a).map((t) => t.text).sort().join(' ') === terms(b).map((t) => t.text).sort().join(' ');
    const states = (f: string) => [`${f} is:open`.trim(), `${f} is:closed`.trim()] as const;
    try {
      const ts = terms(filter);
      const keys = new Set<string>();
      for (const t of ts) for (const k of [STATE, t.key === 'no' ? t.value : t.key === 'label' ? 'label' : t.key]) if (k && owns(k, t)) keys.add(k);
      const own = [...keys];
      // the shared counts: open and closed when the state facet is no own
      // one, else this list's total for a field's "not set"
      const sharedStates = !keys.has(STATE);
      const wanted = [...(sharedStates ? states(filter) : []), ...own.flatMap((k) => states(rest(filter, k)))];
      const ask = [...new Set(wanted.filter((f) => !same(f, filter)))];
      const [counts, sharedLabels, ownLabels] = await Promise.all([
        batchIssues({ counts: ask }, signal),
        labels,
        Promise.all(own.map((k) => (k === STATE ? Promise.resolve([]) : getLabels(rest(filter, k), signal)))),
      ]);
      const n = (f: string) => (same(f, filter) ? total.value : (counts.totals[ask.indexOf(f)] ?? 0));
      const counted = (f: string, l: LabelCount[]): Counted => {
        const [o, c] = states(f);
        return { labels: l, open: n(o), closed: n(c) };
      };
      const shared = sharedStates ? counted(filter, sharedLabels) : { labels: sharedLabels, open: total.value, closed: 0 };
      const fs = facetsOf(filter, fields.value, shared, new Map(own.map((k, i) => [k, counted(rest(filter, k), ownLabels[i] ?? [])] as const)));
      if (!signal.aborted) facets.value = fs;
    } catch {
      // the panel keeps what it showed
    }
  }

  // more shows a bucket's next page: its ids are held, its issues kept here
  async function more(b: Bucket): Promise<void> {
    if (!b.next || !b.ids) return;
    const from = Number(b.next);
    const n = params.group ? LANE_PAGE : PAGE;
    const to = Math.min(b.ids.length, from + n);
    const fresh: Bucket = { ...b, issues: [...b.issues, ...shown(b.ids.slice(from, to))], next: to < b.ids.length ? String(to) : undefined };
    buckets.value = (buckets.value ?? []).map((x) => (x === b ? fresh : x));
  }

  // moved: a card's lane changed here before the server said so
  function moved(issue: Issue, from: string, to: string) {
    buckets.value = (buckets.value ?? []).map((b) =>
      b.group === from ? { ...b, issues: b.issues.filter((i) => i.id !== issue.id), ids: b.ids?.filter((id) => id !== issue.id), total: (b.total ?? 1) - 1 }
      : b.group === to ? { ...b, issues: [issue, ...b.issues], ids: b.ids && [issue.id, ...b.ids.filter((id) => id !== issue.id)], total: (b.total ?? 0) + 1 }
      : b,
    );
  }

  return { buckets, total, loading, error, facets, load, more, moved, reload: () => load(params) };
}
