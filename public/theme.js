(() => {
  const saved = localStorage.getItem('portal-theme');
  // Obsidian is the product's authored realm identity. Light remains an
  // explicit, persistent accessibility preference instead of silently making
  // first-time visitors see the generic control-panel palette.
  document.documentElement.dataset.theme = saved === 'light' ? 'light' : 'dark';
})();
