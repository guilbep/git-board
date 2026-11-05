// The open issue's writes, given by the panel (IssuePanel, useIssue) to
// the head's editors (FieldEditor, StatusStepper) without threading them
// through each: every one goes from the thread's version, and a 412 is the
// panel's to show (reload or overwrite). Each answers '' when it went
// through, else why not.
import type { InjectionKey } from 'vue';

export interface IssueWriter {
  set: (assignments: string[]) => Promise<string>;
}

export const issueWriter: InjectionKey<IssueWriter> = Symbol('issueWriter');
