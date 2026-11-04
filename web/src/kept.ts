// Where the board is kept and what that datastore can do (GET
// /api/datastore), read once and shared by the pages that disable what it
// can't do (datastore.ts needsGit) and Settings' menu line. Settings ›
// Datastore writes its answers here.
import { ref } from 'vue';
import { getDatastore, type Datastore } from '@/api/board';

export const datastore = ref<Datastore | null>(null);

let asked: Promise<void> | null = null;

// loadDatastore asks once; a failed answer leaves it unknown, as on git
export function loadDatastore(): Promise<void> {
  asked ??= getDatastore().then(
    (d) => void (datastore.value = d),
    () => {},
  );
  return asked;
}
