package http

import "net/http"

// trackerScript is the cookieless tracking snippet, served at /track.js for
// sites that install by dropping in a <script src> rather than using the npm
// package.
//
// It mirrors TRACKER_SOURCE in the npm package (npm/src/tracker.ts), which is
// the canonical copy. The duplication is deliberate and small: core is Go and
// cannot import a TypeScript module, and a non-npm user needs a real URL to
// point a script tag at. The two are simple enough to keep in step by hand,
// and a drift would show up the first time either path was tested.
//
// It reads its site key and endpoint from the script tag's data- attributes,
// so this one file serves every site unchanged.
const trackerScript = `(function () {
  var s = document.currentScript;
  if (!s) return;

  var endpoint = s.getAttribute('data-endpoint');
  var siteKey = s.getAttribute('data-site-key');
  if (!endpoint || !siteKey) return;

  function send(name, props) {
    var body = { site_key: siteKey, url: location.href, referrer: document.referrer || '' };
    if (name) body.name = name;
    if (props) body.props = props;

    var payload = JSON.stringify(body);
    var type = 'text/plain;charset=UTF-8';

    if (navigator.sendBeacon) {
      navigator.sendBeacon(endpoint, new Blob([payload], { type: type }));
      return;
    }
    fetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': type },
      body: payload,
      keepalive: true,
      mode: 'cors',
      credentials: 'omit'
    }).catch(function () {});
  }

  var queued = window.zenith && window.zenith.q ? window.zenith.q : [];
  window.zenith = function () { send.apply(null, arguments); };
  window.zenith.track = function (name, props) { if (name) send(name, props); };
  for (var i = 0; i < queued.length; i++) window.zenith.track.apply(null, queued[i]);

  send();

  var lastPath = location.pathname;
  function onRouteChange() {
    if (location.pathname === lastPath) return;
    lastPath = location.pathname;
    send();
  }
  var push = history.pushState;
  if (push) {
    history.pushState = function () { push.apply(history, arguments); onRouteChange(); };
    addEventListener('popstate', onRouteChange);
  }
})();`

// handleTrackerScript serves the tracking snippet.
func (s *Server) handleTrackerScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	// The snippet is loaded by a script tag on someone else's domain.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// Stable content under a stable URL: revalidate rather than cache forever,
	// so a fix to the snippet reaches every installed site on their next load.
	w.Header().Set("Cache-Control", "public, max-age=3600")

	_, _ = w.Write([]byte(trackerScript))
}
