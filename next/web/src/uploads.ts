// Whether files can be attached here (a bucket is set up), asked once.
// Uploads come with their tool (M6): until then they are off.
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
