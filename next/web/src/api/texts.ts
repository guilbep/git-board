// Texts and files (docs/rewrite/api.md, routes 39 to 42).
import { API_BASE, ApiError, errorOf, get, send } from './http.ts';
import type { UploadStatus } from './types.ts';

export function renderMarkdown(body: string): Promise<{ html: string }> {
  return send('POST', '/api/markdown', { body: { body } });
}

// uploads come with their tool (M6): until then the route is not-found,
// and uploads are off
export async function getUploads(): Promise<UploadStatus> {
  try {
    return await get<UploadStatus>('/api/uploads');
  } catch (e) {
    if (e instanceof ApiError && e.code === 'not-found') return { enabled: false };
    throw e;
  }
}

export async function upload(file: File): Promise<{ url: string; name: string; markdown: string }> {
  const form = new FormData();
  form.append('file', file);
  const res = await fetch(API_BASE + '/api/uploads', { method: 'POST', body: form, headers: { 'X-Git-Board-Upload': '1' } });
  const text = await res.text();
  if (!res.ok) {
    const e = errorOf(text);
    throw new ApiError(e.message || `upload failed (${res.status})`, res.status, e.code, e.field);
  }
  return JSON.parse(text) as { url: string; name: string; markdown: string };
}

// a picture through this server
export const mediaURL = (url: string) => `${API_BASE}/api/media?url=${encodeURIComponent(url)}`;

// a section of the guide (docs/guide/<page>.md, a ## heading each); id is
// its heading's anchor
export interface DocSection {
  id: string;
  title: string;
  html: string;
}

// GET /api/docs/{page} (later: api.md, 26): until it answers, no sections,
// and the forms show the reference without the guide beside them
// asked once a page load: every form's reference reads the same
const docs = new Map<string, Promise<DocSection[]>>();
export function getDocs(page: 'configuration' | 'forges'): Promise<DocSection[]> {
  let d = docs.get(page);
  if (!d) docs.set(page, (d = readDocs(page)));
  return d;
}

async function readDocs(page: string): Promise<DocSection[]> {
  try {
    const { items } = await get<{ items: { title: string; html: string }[] }>(`/api/docs/${page}`);
    return items.map((s) => ({ ...s, id: s.title.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') }));
  } catch (e) {
    if (e instanceof ApiError && e.code === 'not-found') return [];
    throw e;
  }
}
