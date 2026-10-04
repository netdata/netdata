(function (k, base, opt) {
  // data-version/data-env on our OWN <script> tag become
  // Faro app.version/app.environment; currentScript must be read
  // synchronously, before anything yields, or it stops pointing here.
  var cs = document.currentScript;
  var ver = (cs && cs.getAttribute('data-version')) || '';
  var env = (cs && cs.getAttribute('data-env')) || '';
  // Crawlers, headless browsers and automation: send nothing unless
  // the site keeps bots, then flag them to the collector.
  // netdataRum.setUser({id}) links sessions to the site's own user id
  //; only the id is passed on, and it works before Faro has loaded.
  var user = null;
  var api = window.netdataRum = window.netdataRum || {};
  api.setUser = function (u) {
    user = u && u.id != null && u.id !== '' ? { id: String(u.id) } : null;
    applyUser();
  };
  function faroApi() { return window.GrafanaFaroWebSdk && GrafanaFaroWebSdk.faro && GrafanaFaroWebSdk.faro.api; }
  function applyUser() {
    var f = faroApi();
    if (!f) { return; }
    try { if (user) { f.setUser(user); } else { f.resetUser(); } } catch (e) {}
  }
  var bot = false;
  try { bot = navigator.webdriver === true || new RegExp(opt.bots, 'i').test(navigator.userAgent || ''); } catch (e) {}
  if (bot && !opt.includeBots) { return; }
  // Pinned Faro web SDK and tracing add-on (Apache-2.0), from the public CDN.
  var cdn = '__SDK_ORIGIN__/npm/@grafana/';
  function load(name, next, failed) {
    var s = document.createElement('script');
    s.async = true;
    s.src = cdn + name + '@__VERSION__/dist/bundle/' + name + '.iife.js';
    s.onload = next;
    if (failed) { s.onerror = failed; }
    document.head.appendChild(s);
  }
  function start() {
    try {
      var app = { name: document.location.hostname };
      if (ver) { app.version = ver; }
      if (env) { app.environment = env; }
      var inst = GrafanaFaroWebSdk.getWebInstrumentations();
      if (opt.tracing && window.GrafanaFaroWebTracing) {
        // Same-origin requests always get traceparent; other origins only
        // when listed, since their CORS must allow the header.
        inst.push(new GrafanaFaroWebTracing.TracingInstrumentation({
          instrumentationOptions: {
            propagateTraceHeaderCorsUrls: opt.tracing.map(function (p) { return new RegExp(p); })
          }
        }));
      }
      var cfg = { url: base + '/rum/' + k + '/collect' + (bot ? '?bot=1' : ''), app: app, instrumentations: inst };
      if (opt.sampling > 0 && opt.sampling < 1) { cfg.sessionTracking = { samplingRate: opt.sampling }; }
      GrafanaFaroWebSdk.initializeFaro(cfg);
      if (user) { applyUser(); }
      watchFrustration();
    } catch (e) { /* never break the host page */ }
  }
  // Frustration signals: rage clicks (3 within 1s on one spot), dead
  // clicks (a link or button that changes nothing within 1s) and error
  // clicks (a JavaScript error within 1s). Only a short CSS selector of the
  // element is sent, never its text; ordinary clicks send nothing.
  function selector(el) {
    var parts = [];
    for (var i = 0; el && el.nodeType === 1 && i < 3; i++, el = el.parentElement) {
      var tag = el.tagName.toLowerCase();
      if (el.id) { parts.unshift(tag + '#' + el.id); break; }
      var cls = (el.getAttribute('class') || '').trim().split(/\s+/).slice(0, 2).join('.');
      parts.unshift(cls ? tag + '.' + cls : tag);
    }
    return parts.join(' > ').slice(0, 200);
  }
  function signal(name, target) {
    var f = faroApi();
    if (f) { try { f.pushEvent(name, { target: target }); } catch (e) {} }
  }
  function watchFrustration() {
    var burst = [], last = null;
    document.addEventListener('click', function (ev) {
      var t = ev.target;
      if (!t || t.nodeType !== 1) { return; }
      var now = Date.now(), sel = selector(t);
      burst = burst.filter(function (c) { return now - c.t < 1000 && Math.abs(c.x - ev.clientX) < 30 && Math.abs(c.y - ev.clientY) < 30; });
      burst.push({ t: now, x: ev.clientX, y: ev.clientY });
      if (burst.length === 3) { signal('rage_click', sel); }
      last = { t: now, sel: sel };
      var control = t.closest && t.closest('a[href],button,[role=button]');
      if (!control || !window.MutationObserver || control.target === '_blank' || control.hasAttribute('download') || /^(mailto|tel):/.test(control.getAttribute('href') || '')) { return; }
      var changed = false, href = location.href;
      var requests = performance.getEntriesByType ? performance.getEntriesByType('resource').length : 0;
      var mo = new MutationObserver(function () { changed = true; });
      mo.observe(document.documentElement, { childList: true, subtree: true, attributes: true, characterData: true });
      setTimeout(function () {
        mo.disconnect();
        var now2 = performance.getEntriesByType ? performance.getEntriesByType('resource').length : 0;
        if (!changed && location.href === href && now2 === requests && !document.hidden) { signal('dead_click', sel); }
      }, 1000);
    }, true);
    function onError() {
      if (last && Date.now() - last.t < 1000) { signal('error_click', last.sel); last = null; }
    }
    window.addEventListener('error', onError);
    window.addEventListener('unhandledrejection', onError);
  }
  load('faro-web-sdk', function () {
    if (opt.tracing) { load('faro-web-tracing', start, start); } else { start(); }
  });
})(__KEY__, __BASE__, __OPT__);
