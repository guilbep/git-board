// The board page's issues (ADR 0023): the page keeps every issue
// (local/store.ts); a list is the ids the server says it holds (GET
// /api/issues/ids: the filter grammar stays the server's), shown from the
// board kept here, a page at a time. Ungrouped, one list; grouped, a list
// per value of the field (the lane's term added to the filter, "no:field"
// for the issues without one), every lane and the list's total in one
// request. The values come from GET /api/labels for the filter, which
// also counts the filters panel (facets.ts facetsOf). A newer load aborts
// the older one's requests.
import { ref, shallowRef, watch, type Ref } from 'vue';
import { batchIssues, getLabels, issueIDs, type Issue, type LabelCount } from '@/api';
import { current, get, issues as kept } from '@/local/store';
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

  // a list's page: its ids (a page of them: the next asked as one scrolls),
  // shown from the board kept here
  const pageSize = () => (params.group ? LANE_PAGE : PAGE);
  const bucketOf = (group: string, ids: string[], total: number): Bucket => ({
    group, ids, issues: shown(ids), total, next: total > ids.length ? String(ids.length) : undefined,
  });
  const shown = (ids: string[]): Issue[] => ids.map(get).filter((i): i is Issue => !!i);
  const filterOf = (b: Bucket) => (params.group ? laneFilter(params.filter, params.group, b.group) : params.filter);

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
      const limit = pageSize();
      if (!p.group) {
        const r = await issueIDs({ f: [p.filter], sort, limit }, signal);
        next = [bucketOf('', r.lists[0] ?? [], r.totals[0] ?? 0)];
        total.value = r.totals[0] ?? 0;
      } else {
        // the options' lanes, a page of ids each, and the list's total in
        // one request, the labels beside it; then, in one more, a lane for
        // each value in use that no option names
        const options = [...optionsOf(p.group), NONE];
        labels = getLabels(p.filter, signal);
        const known = issueIDs({ f: [p.filter, ...options.map((v) => laneFilter(p.filter, p.group, v))], sort, limit }, signal);
        // awaited below, after labels: a new load aborting it meanwhile
        // is this load's to catch, not an unhandled rejection of the page's
        known.catch(() => {});
        const extra = [...new Set((await labels).filter((l) => l.field === p.group && !options.includes(l.value)).map((l) => l.value))];
        const none = { stamp: '', lists: [] as string[][], totals: [] as number[] };
        const [k, x] = await Promise.all([known, extra.length ? issueIDs({ f: extra.map((v) => laneFilter(p.filter, p.group, v)), sort, limit }, signal) : Promise.resolve(none)]);
        next = [
          ...options.map((v, i) => bucketOf(v, k.lists[i + 1] ?? [], k.totals[i + 1] ?? 0)),
          ...extra.map((v, i) => bucketOf(v, x.lists[i] ?? [], x.totals[i] ?? 0)),
        ];
        total.value = k.totals[0] ?? 0;
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

  // more shows a bucket's next page: its ids asked, its issues kept here
  async function more(b: Bucket): Promise<void> {
    if (!b.next || b.loading) return;
    b.loading = true;
    const from = Number(b.next);
    try {
      const r = await issueIDs({ f: [filterOf(b)], sort: params.sort || undefined, limit: pageSize(), offset: from });
      const ids = [...(b.ids ?? []).slice(0, from), ...(r.lists[0] ?? [])];
      const total = r.totals[0] ?? ids.length;
      const fresh: Bucket = { ...b, ids, issues: shown(ids), total, next: total > ids.length ? String(ids.length) : undefined, loading: false };
      buckets.value = (buckets.value ?? []).map((x) => (x === b ? fresh : x));
    } catch (e) {
      b.loading = false;
      error.value = e instanceof Error ? e.message : String(e);
    }
  }

  // moved: a card's lane changed here before the server said so
  function moved(issue: Issue, from: string, to: string) {
    buckets.value = (buckets.value ?? []).map((b) =>
      b.group === from ? { ...b, issues: b.issues.filter((i) => i.id !== issue.id), ids: b.ids?.filter((id) => id !== issue.id), total: (b.total ?? 1) - 1 }
      : b.group === to ? { ...b, issues: [issue, ...b.issues], ids: b.ids && [issue.id, ...b.ids.filter((id) => id !== issue.id)], total: (b.total ?? 0) + 1 }
      : b,
    );
  }

  // an issue changed in the board kept here (an event, a write of this
  // page's): its card shows it at once; the lists' ids follow with the
  // load the event asks (a lane moved, a filter no longer matched)
  watch(kept, (m) => {
    const bs = buckets.value;
    if (!bs) return;
    let changed = false;
    const next = bs.map((b) => {
      const issues = b.issues.map((i) => {
        const now = m.get(i.id);
        if (now && now !== i) changed = true;
        return now ?? i;
      });
      return changed ? { ...b, issues } : b;
    });
    if (changed) buckets.value = next;
  });

  return { buckets, total, loading, error, facets, load, more, moved, reload: () => load(params) };
}
