// The appearance chosen in Settings: system (the computer's), light or
// dark. public/theme.js applies it before the first paint; this keeps
// <html data-theme> in step with a change of choice or of the computer's.
import { ref, watch } from 'vue';

export type ThemePref = 'system' | 'light' | 'dark';

function read(): ThemePref {
  try {
    const v = localStorage.getItem('gb-theme');
    return v === 'light' || v === 'dark' ? v : 'system';
  } catch {
    return 'system';
  }
}

export const themePref = ref<ThemePref>(read());
const media = window.matchMedia('(prefers-color-scheme: dark)');

function apply() {
  const dark = themePref.value === 'dark' || (themePref.value === 'system' && media.matches);
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
}

watch(themePref, (v) => {
  try {
    localStorage.setItem('gb-theme', v);
  } catch {
    // private window: lasts until reload
  }
  apply();
});
media.addEventListener('change', apply);
apply();
