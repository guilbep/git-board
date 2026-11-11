// Sets <html data-theme> before the first paint, from Settings →
// Appearance (localStorage gb-theme: system, light, dark), so a dark page
// never flashes white; and <html data-fonts=epfl> when EPFL's fonts are
// chosen (gb-fonts), so the text doesn't change face once the app runs.
// The app keeps both in step afterwards (src/theme.ts).
(function () {
  var pref = 'system';
  var fonts = 'system';
  try {
    pref = localStorage.getItem('gb-theme') || 'system';
    fonts = localStorage.getItem('gb-fonts') || 'system';
  } catch (e) {}
  var dark = pref === 'dark' || (pref === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
  if (fonts === 'epfl') document.documentElement.dataset.fonts = 'epfl';
})();
