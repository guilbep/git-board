// Whether files can be attached here (a bucket is set up), asked once.
import { ref } from 'vue';
import { getUploadStatus, type UploadStatus } from '@/api/board';

export const uploadStatus = ref<UploadStatus | null>(null);
let asked = false;

export function loadUploadStatus(): void {
  if (asked) return;
  asked = true;
  getUploadStatus().then(
    (s) => (uploadStatus.value = s),
    () => (uploadStatus.value = { enabled: false, max_bytes: 0 }),
  );
}
