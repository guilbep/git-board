// Insights (docs/rewrite/api.md, route 51): the issues' history replayed
// day by day, as insights --json; with sprint, a sprint's report.
import { get } from './http.ts';

export interface InsightsReport {
  field: string;
  values: string[];
  days: string[];
  all: Record<string, number[]>;
  open: Record<string, number[]>;
  weeks: { start: string; opened: number; closed: number }[];
  cycle: { from: 'started' | 'opened'; start: string; days: number[]; median: number; p85: number; imported?: number };
  burnup?: { sprint: string; title: string; start: string; end: string; days: string[]; scope: number[]; done: number[]; status: Record<string, number> };
  headlines: Record<string, string>;
}

export function getInsights(field: string, days: number, view: string): Promise<InsightsReport> {
  return get('/api/insights', { query: { field, days, view: view || undefined } });
}

// A sprint's report as Markdown: the burn-up's sprint unless one is named.
export interface SprintReport {
  sprint: string;
  title: string;
  markdown: string;
}

export function getReport(sprint = '@current'): Promise<SprintReport> {
  return get('/api/insights', { query: { sprint } });
}
