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
import { current, get, issues as kept, pull } from '@/local/store';
import { localIDs } from '@/local/query';
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

  // load lists p; keep (a live reload): as many rows as each list shows
  // now, not back to its first page under the reader's scroll
  async function load(p: ListParams, keep = false): Promise<void> {
    params = p;
    abort?.abort();
    const ctl = (abort = new AbortController());
    const signal = ctl.signal;
    loading.value = true;
    error.value = '';
    let listed = () => {};
    try {
      await current(); // the board kept here, caught up with the events heard
      // the filters panel's counts and labels go beside the list, not after
      // it: they wait for its total only to compare with it
      const listedP = new Promise<void>((r) => (listed = r));
      const labels = getLabels(p.filter, signal, true);
      labels.catch(() => {});
      void loadFacets(p.filter, labels, signal, listedP);
      let next: Bucket[];
      const sort = p.sort || undefined;
      // ponytail: one limit for every lane, the longest's; per lane if a
      // board's lanes page very unevenly
      const limit = keep ? Math.max(pageSize(), ...(buckets.value ?? []).map((b) => b.ids?.length ?? b.issues.length)) : pageSize();
      const local = answeredHere(p, limit);
      if (local) {
        // answered from the board kept here, at once; the server's word in
        // the background, which wins where they differ
        next = local.buckets;
        total.value = local.total;
        void check(p, local.filters, local.buckets, signal, limit);
      } else if (!p.group) {
        const r = await issueIDs({ f: [p.filter], sort, limit }, signal);
        next = [bucketOf('', r.lists[0] ?? [], r.totals[0] ?? 0)];
        total.value = r.totals[0] ?? 0;
      } else {
        // the options' lanes, a page of ids each, and the list's total in
        // one request, the labels beside it; then, in one more, a lane for
        // each value in use that no option names
        const options = [...optionsOf(p.group), NONE];
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
      if (signal.aborted) return listed();
      buckets.value = next;
      loading.value = false;
      listed();
    } catch (e) {
      listed();
      if (signal.aborted) return;
      error.value = e instanceof Error ? e.message : String(e);
      buckets.value = null;
      loading.value = false;
    }
  }

  // answeredHere is every list of p from the board kept here (local/query),
  // and the filters they are; null when one is the server's alone
  function answeredHere(p: ListParams, limit: number): { buckets: Bucket[]; total: number; filters: string[] } | null {
    const all = [...kept.value.values()];
    const sort = p.sort || '';
    const whole = localIDs(all, p.filter, sort, 0);
    if (!whole) return null;
    if (!p.group) {
      return { buckets: [bucketOf('', whole.ids.slice(0, limit), whole.total)], total: whole.total, filters: [p.filter] };
    }
    // the options' lanes, and one for each value in use that no option names
    const options = [...optionsOf(p.group), NONE];
    const prefix = `${p.group}:`;
    const inUse = new Set<string>();
    for (const id of whole.ids) for (const l of get(id)?.labels ?? []) if (l.startsWith(prefix)) inUse.add(l.slice(prefix.length));
    const values = [...options, ...[...inUse].filter((v) => !options.includes(v)).sort()];
    const filters = values.map((v) => laneFilter(p.filter, p.group, v));
    const lanes = filters.map((f) => localIDs(all, f, sort, limit));
    if (lanes.some((l) => !l)) return null;
    return { buckets: values.map((v, i) => bucketOf(v, lanes[i]!.ids, lanes[i]!.total)), total: whole.total, filters };
  }

  // check asks the server the lists the page answered, in the background:
  // where its answer differs, it wins, and the console says so (a filter's
  // rule the page got wrong: local/query.ts)
  async function check(p: ListParams, filters: string[], mine: Bucket[], signal: AbortSignal, limit: number) {
    try {
      const r = await issueIDs({ f: filters, sort: p.sort || undefined, limit }, signal, true);
      if (signal.aborted || buckets.value !== mine) return;
      const same = (i: number, ids: string[], total: number) => ids.join() === (r.lists[i] ?? []).join() && total === (r.totals[i] ?? 0);
      const off = mine.map((b, i) => !same(i, b.ids ?? [], b.total ?? 0));
      if (!off.some(Boolean)) return;
      // a change the page hadn't caught up with yet (the server's answer is
      // newer): caught up, answered again; one that still differs is a
      // rule the page got wrong, and says so
      await pull().catch(() => {});
      const all = [...kept.value.values()];
      off.forEach((o, i) => {
        if (!o) return;
        const again = localIDs(all, filters[i]!, p.sort || '', limit);
        if (again && !same(i, again.ids, again.total)) console.warn(`git-board: the page's answer for ${filters[i]} differs from the server's; the server's shows`);
      });
      if (signal.aborted || buckets.value !== mine) return;
      buckets.value = mine.map((b, i) => (off[i] ? bucketOf(b.group, r.lists[i] ?? [], r.totals[i] ?? 0) : b));
    } catch {
      // the page's answer stays
    }
  }

  // the filters panel: the shared counts, and each facet the filter has a
  // term on counted against the rest of it. Open and closed are asked
  // only where the state facet reads them, labels only where a field's
  // does: on is:open (a view's default) that is the list, its labels and
  // two counts
  async function loadFacets(filter: string, labels: Promise<LabelCount[]>, signal: AbortSignal, listed: Promise<void>) {
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
        batchIssues({ counts: ask }, signal, true),
        labels,
        Promise.all(own.map((k) => (k === STATE ? Promise.resolve([]) : getLabels(rest(filter, k), signal, true)))),
      ]);
      await listed; // its total, for the counts of its own filter
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
  // more loads a list's next page. One load a list at a time (a click and
  // the end coming near ask together); the list is found by its group,
  // not by the object asked with (an event may have replaced it since);
  // an id already shown is not shown twice (the list moved between pages:
  // a duplicate key breaks the table's rendering).
  const asking = new Set<string>();
  async function more(asked: Bucket): Promise<void> {
    const group = asked.group;
    if (!asked.next || asking.has(group)) return;
    asking.add(group);
    const at = (bs: Bucket[] | null | undefined) => bs?.find((x) => x.group === group);
    try {
      const b = at(buckets.value);
      if (!b?.next) return;
      b.loading = true;
      const from = Number(b.next);
      const r = await issueIDs({ f: [filterOf(b)], sort: params.sort || undefined, limit: pageSize(), offset: from });
      const now = at(buckets.value);
      if (!now || now.next !== b.next) return; // reloaded meanwhile: its own pages
      const page = r.lists[0] ?? [];
      const head = (now.ids ?? []).slice(0, from);
      const seen = new Set(head);
      const ids = [...head, ...page.filter((id) => !seen.has(id) && seen.add(id))];
      const total = r.totals[0] ?? ids.length;
      const after = from + page.length; // the server's offset, whatever was dropped
      const fresh: Bucket = { ...now, ids, issues: shown(ids), total, next: page.length && total > after ? String(after) : undefined, loading: false };
      buckets.value = (buckets.value ?? []).map((x) => (x.group === group ? fresh : x));
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      asking.delete(group);
      const c = at(buckets.value);
      if (c?.loading) c.loading = false;
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

  return { buckets, total, loading, error, facets, load, more, moved, reload: () => load(params, true) };
}
