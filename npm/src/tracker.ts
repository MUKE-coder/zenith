/**
 * The tracking snippet.
 *
 * Written as a source string rather than an imported module because it is
 * inlined into the owner's HTML. Inlining is deliberate: no extra request on
 * every pageview, nothing for a blocker to match by URL, and no third-party
 * script tag on a page that promises not to have any.
 *
 * It is cookieless by construction -- there is nothing here that reads or
 * writes storage, and no identifier is generated. Who a visitor is gets
 * decided server-side by a hash that rotates daily; this file only reports
 * what page was viewed.
 *
 * Kept small and dependency-free by hand. Every byte is on the critical path
 * of someone else's site.
 */
export const TRACKER_SOURCE = `(function () {
  var s = document.currentScript;
  if (!s) return;

  var endpoint = s.getAttribute('data-endpoint');
  var siteKey = s.getAttribute('data-site-key');
  if (!endpoint || !siteKey) return;

  function send(name, props) {
    var body = {
      site_key: siteKey,
      url: location.href,
      referrer: document.referrer || ''
    };
    if (name) body.name = name;
    if (props) body.props = props;

    var payload = JSON.stringify(body);

    // text/plain, not application/json, and this is load-bearing.
    //
    // The body is JSON and the server parses it as JSON. But application/json
    // is not a CORS-safelisted content type, so it forces a preflight -- and
    // sendBeacon always sends with credentials mode 'include', which a
    // preflight answered with 'Access-Control-Allow-Origin: *' must reject.
    // The event would never leave the browser. text/plain is safelisted, so
    // the POST is a simple request: no preflight, nothing to reject.
    var type = 'text/plain;charset=UTF-8';

    // sendBeacon survives the page being closed, which a plain fetch does not:
    // a visitor who clicks away immediately still counts.
    if (navigator.sendBeacon) {
      navigator.sendBeacon(endpoint, new Blob([payload], { type: type }));
      return;
    }

    // keepalive gives fetch the same survive-unload behaviour where beacon is
    // unavailable. credentials 'omit' keeps it a simple request too -- and no
    // cookie has any business on this endpoint.
    fetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': type },
      body: payload,
      keepalive: true,
      mode: 'cors',
      credentials: 'omit'
    }).catch(function () {
      // Analytics must never break the page it measures.
    });
  }

  function pageview() {
    send();
  }

  // The queue lets zenith.track() be called before this script has run:
  // window.zenith may already be an array of pending calls.
  var queued = window.zenith && window.zenith.q ? window.zenith.q : [];

  window.zenith = function () {
    send.apply(null, arguments);
  };
  window.zenith.track = function (name, props) {
    if (!name) return;
    send(name, props);
  };

  for (var i = 0; i < queued.length; i++) {
    window.zenith.track.apply(null, queued[i]);
  }

  pageview();

  // Single-page apps change the URL without reloading, so history has to be
  // watched or every route after the first goes uncounted.
  var lastPath = location.pathname;
  function onRouteChange() {
    if (location.pathname === lastPath) return;
    lastPath = location.pathname;
    pageview();
  }

  var push = history.pushState;
  if (push) {
    history.pushState = function () {
      push.apply(history, arguments);
      onRouteChange();
    };
    addEventListener('popstate', onRouteChange);
  }
})();`

/** Attributes the snippet reads. */
export type TrackerAttributes = {
  'data-endpoint': string
  'data-site-key': string
}

/**
 * Builds the attributes for the snippet's script tag.
 *
 * The endpoint and site key are attributes rather than baked into the source,
 * so the source string stays one constant and cannot be built wrong per site.
 */
export function trackerAttributes(backendUrl: string, siteKey: string): TrackerAttributes {
  return {
    'data-endpoint': `${backendUrl.replace(/\/+$/, '')}/api/collect`,
    'data-site-key': siteKey,
  }
}
