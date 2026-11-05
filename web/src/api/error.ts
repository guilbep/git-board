// The one error shape of v2's API (docs/rewrite/api.md): {code, message,
// field}. Pure, no "@/" imports: node runs error.test.ts on it.

// The codes of api.md's table, and the page's own for a server it can't
// reach.
export type ErrorCode =
  | 'invalid' | 'not-json' | 'not-found' | 'experimental' | 'no-identity' | 'changed'
  | 'stale' | 'busy' | 'missing' | 'forbidden' | 'internal' | 'unreachable';

// ApiError is the one error shape, {code, message, field}, and the status.
// (fields set by hand: node's tests strip types, and refuse parameter
// properties)
export class ApiError extends Error {
  readonly status: number;
  readonly code: ErrorCode | string;
  readonly field: string;
  constructor(message: string, status: number, code: ErrorCode | string, field = '') {
    super(message);
    this.status = status;
    this.code = code;
    this.field = field;
  }
}

// isStale: the write named a version the entity has moved from (412).
export const isStale = (e: unknown): e is ApiError => e instanceof ApiError && e.code === 'stale';
export const isNotFound = (e: unknown): e is ApiError => e instanceof ApiError && e.code === 'not-found';

// message is what to show a person for an error.
export const message = (e: unknown): string => (e instanceof Error ? e.message : String(e));

// errorOf reads the error shape; a body that isn't it (a proxy's page) is
// the message as it is.
export function errorOf(body: string): { code: string; message: string; field: string } {
  try {
    const p = JSON.parse(body) as { code?: unknown; message?: unknown; field?: unknown };
    if (p && typeof p === 'object' && typeof p.message === 'string') {
      return { code: typeof p.code === 'string' ? p.code : '', message: p.message, field: typeof p.field === 'string' ? p.field : '' };
    }
  } catch {
    // not JSON
  }
  return { code: '', message: body, field: '' };
}

