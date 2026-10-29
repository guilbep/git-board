// Live updates: one stream per page (GET /api/events, Server-Sent Events)
// tells what changed elsewhere — issues, the configuration, a sync,
// settings — and each part of the page reloads what it shows. A change
// this page made itself (clientId) comes back too and is skipped: the page
// already shows it. The browser reconnects by itself; after a break,
// listeners hear an "issues" event, since some may have been missed.
import { ref } from 'vue';
import { API_BASE, clientId, type SyncStep } from '@/api/board';

export interface LiveEvent {
  kind: 'issues' | 'config' | 'sync' | 'settings' | 'inbox' | 'releases' | 'progress' | 'cycle'; // cycle: a step 4 run ended
  id?: string; // the issue, when the change named one
  by?: string;
  step?: SyncStep; // progress: where a forge run is
}

type Listener = (e: LiveEvent) => void;
const listeners = new Set<Listener>();

// when a change from elsewhere last arrived (the header's notice); a run's
// progress is no change, and a long sync's would keep the notice on
export const lastLive = ref(0);

let source: EventSource | null = null;
let broken = false;

function tell(e: LiveEvent) {
  if (e.kind !== 'progress') lastLive.value = Date.now();
  for (const l of listeners) l(e);
}

export function onLive(fn: Listener): () => void {
  listeners.add(fn);
  if (!source && typeof EventSource !== 'undefined') {
    source = new EventSource(API_BASE + '/api/events');
    source.onmessage = (m) => {
      let e: LiveEvent;
      try {
        e = JSON.parse(m.data) as LiveEvent;
      } catch {
        return;
      }
      if (e.by !== clientId) tell(e);
    };
    source.onerror = () => (broken = true);
    source.onopen = () => {
      if (broken) tell({ kind: 'issues' });
      broken = false;
    };
  }
  return () => {
    listeners.delete(fn);
  };
}

// debounced: a burst of events (a sync writing 40 issues) reloads once
export function debounced(fn: () => void, ms = 400): () => void {
  let t: ReturnType<typeof setTimeout> | undefined;
  return () => {
    clearTimeout(t);
    t = setTimeout(fn, ms);
  };
}
