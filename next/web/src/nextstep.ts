// The status's next step, as the head's arrow says it
// (docs/design/issue-head/README.md, 19d). No status moves to the first
// option; the step onto the last option closes the issue there, as the
// cycle's sign-off does (internal/server/cycle.go, signedOff); an open
// issue already at the last option only closes; a closed issue reopens.
// No workflow, no arrow. Pure: the stepper calls the server.
export interface NextStep {
  icon: 'forward' | 'check' | 'undo'; // GbIcon's arrow-right, check, rotate-ccw
  label: string; // the button's name and tooltip
  target: string; // the status to set, '' for none
  closes: boolean;
  reopens: boolean;
}

export function nextStep(statuses: string[], current: string, closed: boolean): NextStep | null {
  if (closed) return { icon: 'undo', label: 'Reopen', target: '', closes: false, reopens: true };
  if (!statuses.length) return null;
  const i = current ? statuses.indexOf(current) : -1;
  if (i === statuses.length - 1) return { icon: 'check', label: 'Close the issue', target: '', closes: true, reopens: false };
  const next = statuses[i + 1]!;
  if (i + 1 === statuses.length - 1) return { icon: 'check', label: `Close as ${next}`, target: next, closes: true, reopens: false };
  return { icon: 'forward', label: `Move to ${next}`, target: next, closes: false, reopens: false };
}
