// Labels, fields, people (docs/rewrite/api.md, routes 15 to 19).
import { get, send } from './http.ts';
import type { AvatarsMode, FieldConfig, Items, LabelCount, MeResponse, Person } from './types.ts';

// the labels in use among the issues filter matches (every issue when
// absent), each counted open and closed: the filter panel's counts
export async function getLabels(filter?: string, signal?: AbortSignal, background?: boolean): Promise<LabelCount[]> {
  return (await get<Items<LabelCount>>('/api/labels', { query: { filter }, signal, background })).items;
}

export function getFields(): Promise<Record<string, FieldConfig>> {
  return get('/api/fields');
}

export async function getPeople(): Promise<Person[]> {
  return (await get<Items<Person>>('/api/people')).items;
}

export function getMe(): Promise<MeResponse> {
  return get('/api/me');
}

// writes as an existing person, as a new one, or sets board.avatars
// (global: at git's global level, so every repository of yours reads it)
export function setMe(
  to: { id: string } | { name: string; email: string } | { avatars: AvatarsMode; global?: boolean },
): Promise<MeResponse> {
  return send('PUT', '/api/me', { body: to });
}
