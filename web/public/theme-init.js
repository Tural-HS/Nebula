// Applies the stored colour theme before the first paint (ADR-0035), so a
// dark-mode user never sees a white flash. A classic same-origin script: the
// CSP (script-src 'self') blocks inline code. Mirrors src/lib/theme.ts.
(function () {
  var pref = null;
  try {
    pref = localStorage.getItem("nebula.theme");
  } catch (e) {
    // storage blocked: follow the system
  }
  var known = { light: 1, dark: 1, green: 1, paper: 1, midnight: 1, contrast: 1 };
  var theme = known[pref] ? pref : (window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
  var dark = theme === "dark" || theme === "midnight";
  var colors = { light: "#16788b", dark: "#18181b", green: "#00524d", paper: "#f4ecdd", midnight: "#0f1a33", contrast: "#000000" };
  var root = document.documentElement;
  if (dark) root.classList.add("dark");
  if (theme !== "light" && theme !== "dark") root.setAttribute("data-theme", theme);
  root.style.colorScheme = dark ? "dark" : "light";
  var metas = document.querySelectorAll('meta[name="theme-color"]');
  for (var i = 0; i < metas.length; i++) metas[i].content = colors[theme];
})();
