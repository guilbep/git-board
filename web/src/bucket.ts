// A group of the board: a lane, or a table's section. Each is its own
// list (its ids, GET /api/issues/ids with the lane's term; ADR 0023):
// issues are those shown, from the board the page keeps, total how many
// the list holds, next where the next page starts in ids.
import type { Issue } from './api/types.ts';

export interface Bucket {
  group: string;
  issues: Issue[];
  total?: number | undefined;
  next?: string | undefined;
  loading?: boolean | undefined;
  ids?: string[] | undefined; // every id the list holds, in its order
}
