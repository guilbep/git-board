// Live updates: one stream per page (GET /api/events, Server-Sent Events)
// tells what changed, here or elsewhere (a command, a fetch, a sync): an
// issue, a person, the configuration, who writes, the inbox, a sync run
// and its progress. Each part of the page reloads what it shows. A change
// this page made itself (clientId, the event's `by`) comes back too and is
// skipped: the page already shows it, unless the listener asks for its own
// ({ own: true }). A page that falls behind is dropped by the server and
// reconnects (retry): it may have missed events, so the listeners hear a
// `resync`, and reload.
import { ref } from 'vue';
import { clientId, eventsURL, forgetPending, type ChangeEvent } from '@/api';

export type LiveEvent = ChangeEvent | { kind: 'resync'; id?: undefined; by?: undefined; step?: undefined; version?: undefined };

type Listener = (e: LiveEvent) => void;
const listeners = new Map<Listener, boolean>(); // → hears this page's own changes

// when a change from elsewhere last arrived (the header's notice); a run's
// progress is no change, and a long sync's would keep the notice on
export const lastLive = ref(0);

let source: EventSource | null = null;
let opened = false;

function tell(e: LiveEvent) {
  if (e.kind !== 'progress') forgetPending(); // something changed: what is asked now is asked anew
  const own = !!e.by && e.by === clientId;
  if (e.kind !== 'progress' && !own) lastLive.value = Date.now();
  for (const [l, hearsOwn] of [...listeners]) if (!own || hearsOwn) l(e);
}

export function onLive(fn: Listener, opts: { own?: boolean } = {}): () => void {
  listeners.set(fn, !!opts.own);
  if (!source && !document.hidden) connect();
  return () => {
    listeners.delete(fn);
  };
}

function connect() {
  if (typeof EventSource === 'undefined') return;
  source = new EventSource(eventsURL());
  source.onmessage = (m) => {
    try {
      tell(JSON.parse(m.data) as ChangeEvent);
    } catch {
      // not an event
    }
  };
  // every open after the first is a reconnection: what was missed is unknown
  source.onopen = () => {
    if (opened) tell({ kind: 'resync' });
    opened = true;
  };
}

// A browser keeps 6 connections to a host over HTTP/1.1, and each tab's
// stream holds one: with five tabs open, every request of every tab waits
// for the last one. A tab hidden a while lets its stream go, and on its
// return reconnects, its listeners told to resync.
// ponytail: one stream per tab while visible; a stream shared by the tabs
// (a BroadcastChannel leader) if many visible windows fill the six.
export const HIDDEN_CLOSE_MS = 10_000;
let hiddenTimer: ReturnType<typeof setTimeout> | undefined;
if (typeof document !== 'undefined')
  document.addEventListener('visibilitychange', () => {
    clearTimeout(hiddenTimer);
    if (document.hidden) {
      hiddenTimer = setTimeout(() => {
        source?.close();
        source = null;
      }, HIDDEN_CLOSE_MS);
    } else if (!source && listeners.size) connect();
  });

// debounced: a burst of events (a sync writing 40 issues) reloads once
export function debounced(fn: () => void, ms = 400): () => void {
  let t: ReturnType<typeof setTimeout> | undefined;
  return () => {
    clearTimeout(t);
    t = setTimeout(fn, ms);
  };
}
