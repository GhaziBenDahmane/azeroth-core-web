(() => {
  // Marks the document script-capable before first paint so CSS can gate its
  // progressive-enhancement rules on a `.js` class. Kept in this external file
  // rather than inline because the Content-Security-Policy forbids inline
  // scripts (script-src has no 'unsafe-inline').
  document.documentElement.classList.add("js");
  const saved = localStorage.getItem("portal-theme");
  // Obsidian is the product's authored realm identity. Light remains an
  // explicit, persistent accessibility preference instead of silently making
  // first-time visitors see the generic control-panel palette.
  document.documentElement.dataset.theme = saved === "light" ? "light" : "dark";
})();
