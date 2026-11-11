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

// The fonts chosen in Settings: the system's, or EPFL's (Suisse Int'l,
// from enac-it-fonts.epfl.ch, which answers on the EPFL network alone).
// <html data-fonts=epfl> sets them (css/app.css); none asks that host.
export type FontsPref = 'system' | 'epfl';

function readFonts(): FontsPref {
  try {
    return localStorage.getItem('gb-fonts') === 'epfl' ? 'epfl' : 'system';
  } catch {
    return 'system';
  }
}

export const fontsPref = ref<FontsPref>(readFonts());

function applyFonts() {
  if (fontsPref.value === 'epfl') document.documentElement.dataset.fonts = 'epfl';
  else delete document.documentElement.dataset.fonts;
}

watch(fontsPref, (v) => {
  try {
    localStorage.setItem('gb-fonts', v);
  } catch {
    // private window: lasts until reload
  }
  applyFonts();
});
applyFonts();
