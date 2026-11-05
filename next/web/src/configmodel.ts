// The project file's sections as the settings' forms hold them. One load
// for the page (GET /api/config, parsed here: configparse.ts), one save per
// tab (PATCH /api/config, sectionpatch.ts): a tab drafts its own section
// and sends its keys alone, from the version it read — the server writes
// those lines, comments kept, or answers 412 stale when the file moved
// since: the tab offers to reload, or to overwrite. A tab that is dirty
// says so in its row; leaving with a draft asks.
import { computed, ref, watch } from 'vue';
import { getBoard, getConfig, getSync, isStale, message, patchConfig, putConfig, type ConfigFile } from '@/api';
import { modelOf, type ConfigModel } from '@/configparse';
import { sectionPatch } from '@/sectionpatch';
import { reorderTables } from '@/tomlorder';

export const configModel = ref<ConfigModel | null>(null);
export const configModelError = ref('');
const loaded = ref(false);
let pending: Promise<void> | null = null;

// loadConfigModel reads the file's sections once; a save refreshes. The
// same promise for everyone calling while the first is in the air.
export function loadConfigModel(): Promise<void> {
  if (pending) return pending;
  pending = Promise.all([getConfig(), getSync().catch(() => null), getBoard().catch(() => null)]).then(
    ([file, sync, board]) => {
      forges = sync?.forges ?? [];
      on = board?.experimental ?? [];
      takeFile(file);
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

let forges: NonNullable<Parameters<typeof modelOf>[0]['forges']> = [];
let on: string[] = [];

// takeFile makes the model of the file a read or a write answered
export function takeFile(f: ConfigFile) {
  const m = modelOf({ text: f.text, version: f.version, exists: f.exists, forges, on });
  configModel.value = m;
  configModelError.value = m.error ?? f.problems.map((p) => `${p.table}: ${p.message}`).join('; ');
}

// inputValue reads a native input's or select's value out of its event:
// on a plain element @input and @change hand the event, not the value
export function inputValue(e: Event): string {
  const t = e.target;
  return t instanceof HTMLInputElement || t instanceof HTMLSelectElement ? t.value : '';
}

// plainClone takes a (possibly reactive) piece of the model back to plain
// JSON data: structuredClone refuses Vue's proxies, and the model is a
// deep reactive ref — a shallow spread leaves every nested object a proxy
export function plainClone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T;
}

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
    // plainClone, not structuredClone: the model is a deep reactive ref,
    // and a from() that copies a nested object or array shallowly — as the
    // views draft does — hands structuredClone a proxy, which it refuses
    // (DataCloneError). The drafts are pure JSON API data.
    this.value.value = plainClone(this.from(m));
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

  // save writes the section from the version it read; overwrite, after a
  // 412, writes it over whatever the file is now
  async save(overwrite = false): Promise<boolean> {
    const m = configModel.value;
    if (this.value.value === undefined || !this.base.value || !m) return false;
    this.saving.value = true;
    this.saveError.value = '';
    this.conflict.value = false;
    try {
      const body = this.bodyOf(this.value.value, this.removed.value) as { order?: string[] };
      const set = sectionPatch(this.section, body, m);
      let f = await patchConfig(set, overwrite ? '' : this.base.value);
      // a new order of the tables is the text's: put whole, from the
      // version the patch answered (tomlorder.ts)
      if (body.order) {
        const text = reorderTables(f.text, this.section, body.order);
        if (text !== f.text) f = await putConfig(text, f.version);
      }
      takeFile(f);
      this.reset(configModel.value!);
      this.saved.value = true;
      return true;
    } catch (e) {
      this.conflict.value = isStale(e);
      this.saveError.value = message(e);
      return false;
    } finally {
      this.saving.value = false;
    }
  }
}
