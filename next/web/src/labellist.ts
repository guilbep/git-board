// The project's [labels] list, read once a page and shared by every
// issue's label picker (LabelPicker): it changes with the project file,
// which a save reloads.
import { ref } from 'vue';
import { getLabelList, type LabelList } from '@/api/board';

export const labelList = ref<LabelList | null>(null);

let reading: Promise<void> | null = null;

export function loadLabelList(): Promise<void> {
  reading ??= getLabelList().then(
    (l) => void (labelList.value = l),
    () => void (labelList.value = null), // an older server: no list, no picker
  );
  return reading;
}
