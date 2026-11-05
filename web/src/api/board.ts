// The board (docs/rewrite/api.md, routes 1 and 2): what a page reads
// first, and the one stream of change events.
import { API_BASE, get } from './http.ts';
import type { BoardInfo } from './types.ts';

export function getBoard(): Promise<BoardInfo> {
  return get('/api/board');
}

export const eventsURL = () => API_BASE + '/api/events';
