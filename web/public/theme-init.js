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
  var dark = pref === "dark" || (pref !== "light" && window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches);
  var root = document.documentElement;
  if (dark) root.classList.add("dark");
  root.style.colorScheme = dark ? "dark" : "light";
  var metas = document.querySelectorAll('meta[name="theme-color"]');
  for (var i = 0; i < metas.length; i++) metas[i].content = dark ? "#18181b" : "#16788b";
})();
