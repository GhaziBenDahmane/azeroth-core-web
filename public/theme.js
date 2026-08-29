(() => {
  const saved = localStorage.getItem('portal-theme');
  document.documentElement.dataset.theme = saved || (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark');
})();
