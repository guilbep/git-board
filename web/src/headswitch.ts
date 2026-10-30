// Which head the issue drawer shows (docs/prd/issue-head-plan.md, Calls
// taken 2): the new one, v2, unless this browser asked for the one before
// (localStorage gb-head = v1, the More menu's "Previous head"). Kept per
// browser, as the panel's width and mode are (panelsize.ts). Goes with
// the side column in W3-1.
import { ref, watch } from 'vue';

const KEY = 'gb-head';

function read(): 'v1' | 'v2' {
  try {
    return localStorage.getItem(KEY) === 'v1' ? 'v1' : 'v2';
  } catch {
    return 'v2';
  }
}

export const head = ref<'v1' | 'v2'>(read());

watch(head, (v) => {
  try {
    if (v === 'v1') localStorage.setItem(KEY, 'v1');
    else localStorage.removeItem(KEY);
  } catch {
    // no storage (a private window): the choice lasts the page
  }
});

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
