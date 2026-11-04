// The inbox, shared by the header (its count) and the inbox page. Loading
// never checks the remotes: the page does that (inboxcheck.ts).
import { ref } from 'vue';
import { ApiError, getInbox, type InboxResponse } from '@/api/board';
import { latestOnly } from '@/latest';

export const inbox = ref<InboxResponse | null>(null);
// no-identity, no-login: the page says why; the header shows no count
export const inboxError = ref<{ code: string; message: string } | null>(null);

// the header, the panel (after marking seen), the page: the one asked last
// answers, an older one landing after it would bring back what was unread
const loads = latestOnly();

export async function loadInbox(): Promise<void> {
  const current = loads.start();
  try {
    const got = await getInbox();
    if (!current()) return;
    inbox.value = got;
    inboxError.value = null;
  } catch (e) {
    if (!current()) return;
    inbox.value = null;
    inboxError.value = { code: e instanceof ApiError ? e.code : '', message: e instanceof Error ? e.message : String(e) };
  }
}
