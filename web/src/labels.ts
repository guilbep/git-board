// Settings › Labels, the parts that are only logic. Pure, no "@/" imports:
// node runs labels.test.ts on it.

export type LabelState = 'same' | 'color' | 'description' | 'missing' | 'held';

// a label's state on a GitHub repository (ghlabels.State)
export const stateText: Record<LabelState, string> = { same: 'Same', color: 'Color differs', description: 'Description differs', missing: 'Missing', held: 'In the Project' };

interface Label { name: string; color: string; description: string }

// changeText is a change Send to GitHub makes, as git board lint
// --create-labels says it
export function changeText(c: Label & { was?: Label }): string {
  if (!c.was) return `Create ${c.name}${c.color ? ` #${c.color}` : ''}${c.description ? ` "${c.description}"` : ''}`;
  const what = [];
  if (c.color !== c.was.color) what.push(`color #${c.was.color} → #${c.color}`);
  if (c.description !== c.was.description) what.push(`description "${c.was.description}" → "${c.description}"`);
  return `Update ${c.name}: ${what.join(', ')}`;
}

// byCheck is lint's findings a group per check, in the server's order
// (lint.CheckOrder)
export function byCheck<T extends { check: string }>(findings: T[]): { check: string; findings: T[] }[] {
  const res: { check: string; findings: T[] }[] = [];
  for (const f of findings) {
    const last = res[res.length - 1];
    if (last?.check === f.check) last.findings.push(f);
    else res.push({ check: f.check, findings: [f] });
  }
  return res;
}

// deleteConfirm asks before a GitHub label goes; GitHub counts its pull
// requests among the issues
export function deleteConfirm(repo: string, name: string, count: number): string {
  const lose = count === 0 ? 'No issue carries it.' : `${count} issue${count === 1 ? ' loses' : 's lose'} it.`;
  return `Delete ${name} from ${repo}? ${lose}`;
}

// hexRules is a CSS rule per #rrggbb color of the board's values (a
// GitHub label's): it paints the dot only; the chip keeps the ink and a
// plain background, since a label's color (ebcf04, ffffff) is often
// unreadable as text. colorClass names them gb-c-x<hex>.
export function hexRules(colors: Record<string, Record<string, string>>): string {
  const hex = new Set(Object.values(colors).flatMap((values) => Object.values(values).filter((c) => /^#[0-9a-f]{6}$/.test(c))));
  return [...hex].map((c) => `.gb-c-x${c.slice(1)}{--c-bg:var(--bg-subtle);--c-fg:var(--fg);--c-dot:${c}}`).join('\n');
}

// lintLines is lint's findings as Labels › Lint shows them: a group per
// check, in the server's order; a missing value and a dead option one
// line per field (est: 32 open issues have none · Show them), the rest one
// line per finding, linked to its issue
export type LintLine = { text: string; issue?: { id: string; title: string }; filter?: string };
export function lintLines(findings: { check: string; message: string; issue_id: string; title: string }[]): { check: string; items: LintLine[] }[] {
  return byCheck(findings).map((g) => {
    if (g.check === 'missing-value') {
      const by = new Map<string, number>();
      for (const f of g.findings) {
        const field = /^no (\S+) value$/.exec(f.message)?.[1] ?? f.message;
        by.set(field, (by.get(field) ?? 0) + 1);
      }
      return { check: g.check, items: [...by].map(([field, n]) => ({ text: `${field}: ${n} open issue${n === 1 ? ' has' : 's have'} none`, filter: `is:open no:${field}` })) };
    }
    if (g.check === 'dead-option') {
      const by = new Map<string, string[]>();
      for (const f of g.findings) {
        const [field = '', value = ''] = (/^([^:]+):(.*): no issue carries it$/.exec(f.message) ?? []).slice(1);
        by.set(field, [...(by.get(field) ?? []), value]);
      }
      return { check: g.check, items: [...by].map(([field, values]) => ({ text: `${field}: no issue carries ${values.join(', ')}` })) };
    }
    return { check: g.check, items: g.findings.map((f) => (f.issue_id ? { text: f.message, issue: { id: f.issue_id, title: f.title } } : { text: f.message })) };
  });
}
