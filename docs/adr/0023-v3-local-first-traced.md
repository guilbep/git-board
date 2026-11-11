# ADR 0023: The page keeps the board, the server says what it costs

- Status: accepted
- Date: 2026-11-11
- Plan: the v3 plan (perf/fewer-requests, after 828cc402): seven
  milestones, M1 to M7, named below.

## Context

`perf/fewer-requests` took a 5,000-issue board's page from Lighthouse 61
to 94 by asking less. What was left was the shape of the app, not a list
of requests:

- every list built every issue again on the server, and a board's lanes
  did it once a lane (9 lists, 18,225 rows read);
- every change event reloaded the board's lists, counts and labels;
- Quasar shipped 281 KB of CSS for 14 dialogs and one menu;
- nothing said where time went: the bottlenecks were found by reading
  code, and a sync's thousand sequential forge reads were never seen;
- a check of a forge never synced listed every issue as a creation, one
  page row a write (5,199), as it simulated a consent it wasn't given.

The goals beyond: a page that answers a person's act in under 100 ms on a
large board, and, later, a native app or a remote server on the same
protocol.

## Decision

### 1. Traces, without the SDK (M1)

Requests, git processes, loads, view queries, forge requests and their
waits, syncs and a page's slow interactions are OpenTelemetry spans,
sent as OTLP/JSON by the `obs` package when `OTEL_EXPORTER_OTLP_ENDPOINT`
is set (to Grafana Tempo: `make obs-up`). The wire format and the
environment are OpenTelemetry's; the SDK isn't used (ADR 0014: few
dependencies), so no sampling and no metrics. Every reply says its cost
in `Server-Timing`. A page sends `traceparent` on each request, and an
interaction over 200 ms to `POST /api/slow`, so a trace starts where the
person acted.

### 2. Sync says what it would do (M2)

A check does what a sync would, consent included, and says it as counts
by forge, way and operation, warnings grouped by reason, a few examples
each. With no `[forges]` table, a forge `origin` fetches from syncs; one
only a push URL names is listed off: the file alone turns it on (ADR
0008).

### 3. Vue on plain Vite (M3)

The web app keeps Vue and drops Quasar: dialogs are the browser's own
`<dialog>` (`GbDialog.vue`), the filters' help a popover. Considered and
left: a framework port or web components (a rewrite of 14,500 lines of
components for a smaller runtime no one would feel), pages rendered by
the Go server (two renderers once a native app comes), server or static
rendering in Node (a second server beside the binary, and nothing static
to render on a live board). The build writes brotli beside each file.

### 4. The page keeps the board (M4, M6)

The server keeps the issues it built while the board's version holds
(`issues.KeptList`), and serves the whole board as a snapshot with its
stamp (`GET /api/snapshot`), then the changes since a cursor (`GET
/api/changes`), from a ring of its last issue events. Lists answer
ETags and 304s. The page holds the snapshot, asks which ids each of its
lists holds (`GET /api/issues/ids`: many filters, sorted, in one request
of a few KB), shows its own copies of them, pages them without a request,
and writes optimistically, rolled back on a refusal. The filter grammar
stays the server's alone: a second implementation in TypeScript, kept
one with the Go by shared vectors, would save a request of a few
milliseconds on a filter not asked before, and is left for a remote
server where that request is slow. JSON stays the format: the browser's
own parser is faster than any decoder shipped to it.

### 5. Interactions are named and held (M7)

A person's acts are a registry (`web/e2e/interactions.ts`): each a
budget, whether it is optimistic, how many requests it may make. Every
pull request checks that an optimistic act shows before its answer and
asks no more than its share; the nightly times each one on the 5,000-issue
board against `testdata/perf/budgets.txt`. A feature that adds an act
adds its entry.

## Consequences

- `make obs-up` and one environment variable answer "where did the time
  go" from Tempo's API, for a person or an agent.
- The page holds the whole board in memory: fine to about 50,000 issues;
  beyond, the snapshot pages.
- A filter not asked before costs one request of ids; the grammar has one
  implementation.
- A native app or a remote server speaks the same snapshot, changes and
  events; this ADR adds no login and changes no guard (ADR 0010).
