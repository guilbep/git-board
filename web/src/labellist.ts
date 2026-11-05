// The project's [labels] list (.git-board.toml), read with the file's
// model and shared by every issue's label picker (LabelPicker): a save
// reloads it.
import { computed } from 'vue';
import { configModel, loadConfigModel } from '@/configmodel';

export interface LabelList {
  defined: boolean; // the file has a [labels] list
  list: string[];
  colors: Record<string, string>;
  descriptions: Record<string, string>;
}

export const labelList = computed<LabelList | null>(() => {
  const m = configModel.value;
  if (!m) return null;
  return { defined: 'labels' in m.raw, list: m.labels.list, colors: m.labels.colors ?? {}, descriptions: m.labels.descriptions ?? {} };
});

export function loadLabelList(): Promise<void> {
  return configModel.value ? Promise.resolve() : loadConfigModel();
}
