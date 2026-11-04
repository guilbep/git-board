// What a refused request says, from the body the server answered with:
// - {"error": "...", "code"?: "..."}: writeError, writeCoded (internal/server)
// - a config check refused (PUT /api/config, its sections, a view, the
//   labels): {"ok": false, "error": {line, col, message}} for a file that
//   isn't TOML, {"ok": false, "message": "..."} for the rest
// - anything else: the body as it is.

export function errorOf(body: string): { message: string; code: string } {
  let parsed: unknown;
  try {
    parsed = JSON.parse(body);
  } catch {
    return { message: body, code: '' }; // not JSON: the raw body
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return { message: body, code: '' };
  const p = parsed as { error?: unknown; message?: unknown; code?: unknown };
  const code = typeof p.code === 'string' ? p.code : '';
  if (typeof p.error === 'string' && p.error) return { message: p.error, code };
  if (p.error && typeof p.error === 'object') {
    const e = p.error as { line?: unknown; col?: unknown; message?: unknown };
    if (typeof e.message === 'string' && e.message) {
      const at = typeof e.line === 'number' && e.line > 0 ? `line ${e.line}${typeof e.col === 'number' && e.col > 0 ? `, column ${e.col}` : ''}: ` : '';
      return { message: at + e.message, code };
    }
  }
  if (typeof p.message === 'string' && p.message) return { message: p.message, code };
  return { message: body, code };
}
