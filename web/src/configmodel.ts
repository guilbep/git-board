// The project file's sections as the settings' forms hold them. One load
// for the page (GET /api/config/model), one save per tab (PUT
// /api/config/section/<name>): a tab drafts its own section and never
// holds the file's other lines — the server patches those, comments kept.
// A tab that is dirty says so in its row; leaving with a draft asks.
import { computed, ref, watch } from 'vue';
import { ApiError, getConfigModel, saveConfigSection, type ConfigModel } from '@/api/board';
import { message } from '@/issue';

export const configModel = ref<ConfigModel | null>(null);
export const configModelError = ref('');
const loaded = ref(false);
let pending: Promise<void> | null = null;

// loadConfigModel reads the file's sections once; a save refreshes. The
// same promise for everyone calling while the first is in the air.
export function loadConfigModel(): Promise<void> {
  if (pending) return pending;
  pending = getConfigModel().then(
    (m) => {
      configModel.value = m;
      configModelError.value = m.error ?? '';
      loaded.value = true;
      pending = null;
    },
    (e) => {
      configModelError.value = message(e);
      pending = null;
    },
  );
  return pending;
}

export const configModelReady = computed(() => loaded.value);

// the sections' dirty marks, for the tab row's dot: a tab registers its
// draft's dirtiness here under its own name
export const dirtySections = ref<Record<string, boolean>>({});
export function setDirty(section: string, dirty: boolean) {
  if (dirtySections.value[section] === dirty) return;
  dirtySections.value = { ...dirtySections.value, [section]: dirty };
}

// SectionDraft is one tab's state: the model's value cloned, saved whole
// through bodyOf, with the names the draft dropped for the sections that
// take a remove list (fields, views — the file's other tables stay).
export class SectionDraft<T> {
  value = ref<T>();
  removed = ref<string[]>([]);
  base = ref(''); // the file's hash when the draft was (re)loaded
  conflict = ref(false);
  saving = ref(false);
  saved = ref(false);
  saveError = ref('');

  constructor(
    readonly section: string,
    readonly from: (m: ConfigModel) => T,
    readonly dirtyOf: (a: T, b: T) => boolean,
    readonly bodyOf: (value: T, removed: string[]) => unknown,
  ) {
    // the tab row's dot (constructed in setup: the watcher dies with the
    // component)
    watch(
      () => this.dirty.value,
      (d) => setDirty(section, d),
      { immediate: true },
    );
  }

  reset(m: ConfigModel) {
    this.value.value = structuredClone(this.from(m));
    this.removed.value = [];
    this.base.value = m.hash;
    this.conflict.value = false;
    this.saveError.value = '';
    this.saved.value = false;
  }

  load(): Promise<void> {
    return loadConfigModel().then(() => {
      if (configModel.value) this.reset(configModel.value);
    });
  }

  // drop marks a name for removal (a field's card, a view's): the save
  // sends it in the remove list, the file's table goes
  drop(name: string) {
    if (!this.removed.value.includes(name)) this.removed.value = [...this.removed.value, name];
  }

  dirty = computed(() => {
    const m = configModel.value;
    if (!m || this.value.value === undefined) return false;
    return this.dirtyOf(this.from(m), this.value.value) || this.removed.value.length > 0;
  });

  async save(): Promise<boolean> {
    if (this.value.value === undefined || !this.base.value) return false;
    this.saving.value = true;
    this.saveError.value = '';
    this.conflict.value = false;
    try {
      const r = await saveConfigSection(this.section, this.bodyOf(this.value.value, this.removed.value), this.base.value);
      configModel.value = r.model;
      this.reset(r.model);
      this.saved.value = true;
      return true;
    } catch (e) {
      this.conflict.value = e instanceof ApiError && e.code === 'changed';
      this.saveError.value = message(e);
      return false;
    } finally {
      this.saving.value = false;
    }
  }
}
