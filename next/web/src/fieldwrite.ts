// One field's write from the head (FieldEditor, StatusStepper): a PATCH
// of one value, "-value" removing it (internal/server issues.go). Refused
// because a forge sync runs, it waits for the end of the sync and goes
// again, as every write of the panel does (issue.ts).
import { ApiError, patchIssue } from '@/api/board';
import { untilSyncEnds } from '@/forges';

export async function patchField(id: string, name: string, value: string, onWait?: (waiting: boolean) => void): Promise<void> {
  for (;;) {
    try {
      await patchIssue(id, { [name]: value });
      return;
    } catch (e) {
      if (!(e instanceof ApiError && e.code === 'syncing')) throw e;
      onWait?.(true);
      await untilSyncEnds();
      onWait?.(false);
    }
  }
}
