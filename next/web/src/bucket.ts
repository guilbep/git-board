// A group of the board: a lane, or a table's section. Each is its own
// paged list (GET /api/issues with the lane's term): issues are the pages
// loaded, total how many the list holds, next the cursor of the page after.
import type { Issue } from './api/types.ts';

export interface Bucket {
  group: string;
  issues: Issue[];
  total?: number | undefined;
  next?: string | undefined;
  loading?: boolean | undefined;
}

// how many issues a bucket holds: its total once known
export const sizeOf = (b: Bucket) => b.total ?? b.issues.length;
