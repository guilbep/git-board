// How the issue drawer lays the issue out, kept per browser as the
// panel's width and mode are (panelsize.ts): the bar's Fit and Fill, and
// Comfortable and Compact (docs/design/issue-head/README.md).
import { ref, watch } from 'vue';

// The issue's width in the drawer, the bar's Fit and Fill: Fit, the
// default, keeps the head and the conversation to a reading width; Fill
// gives them the panel's. Kept per browser.
const WIDTH = 'gb-head-width';

function readWidth(): 'fit' | 'fill' {
  try {
    return localStorage.getItem(WIDTH) === 'fill' ? 'fill' : 'fit';
  } catch {
    return 'fit';
  }
}

export const headWidth = ref<'fit' | 'fill'>(readWidth());

watch(headWidth, (v) => {
  try {
    if (v === 'fill') localStorage.setItem(WIDTH, 'fill');
    else localStorage.removeItem(WIDTH);
  } catch {
    // no storage: the choice lasts the page
  }
});

// The issue's spacing, the bar's Comfortable and Compact: Comfortable, the
// default, the spacing the head was designed with; Compact, opted into,
// less space between the head's lines, the comments and the paragraphs,
// the text at its size (docs/design/issue-head/README.md, Comfortable and
// Compact). Kept per browser.
const DENSITY = 'gb-head-density';

function readDensity(): 'comfortable' | 'compact' {
  try {
    return localStorage.getItem(DENSITY) === 'compact' ? 'compact' : 'comfortable';
  } catch {
    return 'comfortable';
  }
}

export const headDensity = ref<'comfortable' | 'compact'>(readDensity());

watch(headDensity, (v) => {
  try {
    if (v === 'compact') localStorage.setItem(DENSITY, 'compact');
    else localStorage.removeItem(DENSITY);
  } catch {
    // no storage: the choice lasts the page
  }
});
