// The inbox (docs/rewrite/api.md, routes 20 to 22).
import { get, send } from './http.ts';
import type { InboxResponse } from './types.ts';

export function getInbox(): Promise<InboxResponse> {
  return get('/api/inbox');
}

export function markSeen(issues: string[]): Promise<object> {
  return send('POST', '/api/inbox/seen', { body: { issues } });
}

export function setInboxSettings(views: string[], fields: boolean): Promise<object> {
  return send('PUT', '/api/inbox/settings', { body: { views, fields } });
}
