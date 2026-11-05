// The file tab's reference follows the cursor: the section of
// docs/guide/configuration.md for the table under it.
// docSectionAt names the reference section (docs/guide/configuration.md, one per
// ## heading) for the table the cursor is in: a view's filter line is the
// filter syntax, a colors table the colors.
export function docSectionAt(text: string, pos: number): string {
  const before = text.slice(0, pos).split('\n');
  const end = text.indexOf('\n', pos);
  const line = (before.at(-1) ?? '') + text.slice(pos, end < 0 ? undefined : end);
  const header = [...before.slice(0, -1), line].reverse().find((l) => /^\s*\[/.test(l))?.trim() ?? '';
  if (/^\[\[?sprints/.test(header)) return 'sprints';
  if (/^\[fields\.[^\]]+\.colors\]/.test(header)) return 'colors';
  if (header.startsWith('[fields.')) return /^\s*colors\b/.test(line) ? 'colors' : /^\s*(type|unit)\b/.test(line) ? 'types' : 'fields';
  if (header.startsWith('[views.')) return /^\s*filter\b/.test(line) ? 'filters' : 'views';
  if (header.startsWith('[github-project]')) return 'github';
  if (/^\[forges[.\]]/.test(header)) return 'forges';
  if (header.startsWith('[uploads]')) return 'attachments';
  if (header.startsWith('[repos]')) return 'repos';
  if (header.startsWith('[branches]')) return 'branches';
  if (header.startsWith('[roles]')) return 'roles';
  if (/^\[labels[.\]]/.test(header)) return 'labels';
  return 'overview';
}
