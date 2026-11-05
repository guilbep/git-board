// Sets <html data-theme> before the first paint, from Settings →
// Appearance (localStorage gb-theme: system, light, dark), so a dark page
// never flashes white. The app keeps it in step afterwards (src/theme.ts).
(function () {
  var pref = 'system';
  try { pref = localStorage.getItem('gb-theme') || 'system'; } catch (e) {}
  var dark = pref === 'dark' || (pref === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
})();
