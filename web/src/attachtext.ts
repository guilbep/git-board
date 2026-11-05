// The text side of an attachment (MarkdownEditor): a placeholder goes in at
// the cursor while the file uploads, the file's Markdown takes its place.

// placeholder is what stands for a file while it uploads; n tells two
// uploads of one name apart.
export const placeholderFor = (name: string, n: number): string => `[Uploading ${name}…${n > 1 ? ` (${n})` : ''}]()`;

// insertAt puts s on a line of its own at the cursor.
export function insertAt(text: string, at: number, s: string): string {
  const before = text.slice(0, at);
  const sep = before && !before.endsWith('\n') ? '\n' : '';
  return before + sep + s + '\n' + text.slice(at);
}

// fillPlaceholder puts the uploaded file's Markdown where its placeholder
// is. The placeholder gone (the text sent, cleared or edited meanwhile),
// the Markdown goes at the end: the file is never dropped without a word.
export function fillPlaceholder(text: string, placeholder: string, markdown: string): string {
  if (text.includes(placeholder)) return text.replace(placeholder, () => markdown);
  if (!text) return markdown;
  return text + (text.endsWith('\n') ? '' : '\n') + markdown;
}
