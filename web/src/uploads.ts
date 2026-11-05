// Whether files can be attached here (a bucket is set up: GET
// /api/uploads), asked once.
import { ref } from 'vue';
import { getUploads, type UploadStatus } from '@/api';

export const uploadStatus = ref<UploadStatus | null>(null);
let asked = false;

export function loadUploadStatus(): void {
  if (asked) return;
  asked = true;
  getUploads().then(
    (s) => (uploadStatus.value = s),
    () => (uploadStatus.value = { enabled: false }),
  );
}
