(function () {
  var k = 'mewmail_theme';
  var t = localStorage.getItem(k);
  if (t !== 'dark' && t !== 'light') {
    t = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  document.documentElement.setAttribute('data-bs-theme', t);
})();
