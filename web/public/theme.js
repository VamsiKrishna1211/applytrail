/* Applies a stored theme before the first paint.
 *
 * It cannot live in index.html: the bundle is served with `script-src 'self'`
 * and no inline script, so this is a real file fetched from the same origin.
 * Running it in `<head>` — ahead of the stylesheet's first paint and long
 * ahead of React — is what stops a chosen dark theme flashing light on every
 * load. Absence of the attribute means "follow the system", which is what the
 * tokens already do on their own. */
(function () {
  try {
    var stored = localStorage.getItem('applytrail.theme')
    if (stored === 'light' || stored === 'dark') {
      document.documentElement.setAttribute('data-theme', stored)
    }
  } catch (error) {
    /* Storage can throw outright — private mode, blocked site data. The system
       scheme is a perfectly good answer, so there is nothing to handle. */
  }
})()
