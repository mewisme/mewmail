(function () {
  initThemeToggles();
  const el = document.getElementById('email-data');
  const email = JSON.parse(el.textContent);
  document.title = emailTitle(email);
  renderEmail(email, document.getElementById('email'));
})();
