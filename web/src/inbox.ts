// The inbox, shared by the header (its count) and the inbox page. Loading
// never checks the remotes: the page does that (inboxcheck.ts).
import { ref } from 'vue';
import { ApiError, getInbox, type InboxResponse } from '@/api/board';

export const inbox = ref<InboxResponse | null>(null);
// no-identity, no-login: the page says why; the header shows no count
export const inboxError = ref<{ code: string; message: string } | null>(null);

export async function loadInbox(): Promise<void> {
  try {
    inbox.value = await getInbox();
    inboxError.value = null;
  } catch (e) {
    inbox.value = null;
    inboxError.value = { code: e instanceof ApiError ? e.code : '', message: e instanceof Error ? e.message : String(e) };
  }
}
