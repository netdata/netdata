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
  // A stored Faro session can retain an earlier sampling decision. Zero must
  // stop before loading the SDK or installing any instrumentation.
  if (opt.sampling === 0) { return; }
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
      var sdk = GrafanaFaroWebSdk;
      var inst = [new sdk.PerformanceInstrumentation(), new sdk.ErrorsInstrumentation(),
        new sdk.WebVitalsInstrumentation(), new sdk.SessionInstrumentation(), new sdk.ViewInstrumentation()];
      if (opt.consoleLogs) { inst.push(new sdk.ConsoleInstrumentation()); }
      if (opt.tracing && window.GrafanaFaroWebTracing) {
        // Same-origin requests always get traceparent; other origins only
        // when listed, since their CORS must allow the header.
        inst.push(new GrafanaFaroWebTracing.TracingInstrumentation({
          instrumentationOptions: {
            propagateTraceHeaderCorsUrls: opt.tracing.map(function (p) { return new RegExp(p); })
          }
        }));
      }
      var cfg = { url: base + '/rum/' + k + '/collect' + (bot ? '?bot=1' : ''), app: app, instrumentations: inst, beforeSend: shapeItem };
      if (opt.consoleLogs) { cfg.consoleInstrumentation = { consoleErrorAsLog: true }; }
      cfg.sessionTracking = { samplingRate: opt.sampling };
      GrafanaFaroWebSdk.initializeFaro(cfg);
      if (user) { applyUser(); }
      if (opt.frustrationSignals) { watchFrustration(); }
    } catch (e) { /* never break the host page */ }
  }
  function pick(source, keys) {
    var result = {};
    keys.forEach(function (key) { if (source && source[key] !== undefined) { result[key] = source[key]; } });
    return result;
  }
  function cleanURL(value) {
    if (typeof value !== 'string' || !value) { return value; }
    // Resolve relative references so request and page hosts remain meaningful.
    try {
      // Opaque URLs can embed content or another URL with credentials.
      // The receiver also rejects them; hierarchical script schemes remain useful.
      var opaqueURL = /^[a-z][a-z0-9+.-]*:(?!\/)/i;
      if (opaqueURL.test(value.trim())) { return ''; }
      var url = new URL(value, location.href);
      // URL parsing removes embedded tabs/newlines from scheme names.
      if (opaqueURL.test(url.href)) { return ''; }
      url.username = ''; url.password = ''; url.search = ''; url.hash = '';
      return url.href;
    } catch (e) { return ''; }
  }
  function cleanView(value) {
    if (typeof value !== 'string') { return value; }
    value = value.replace(/[\u0000-\u001f\u007f-\u009f]/g, '').trim();
    // Keep relative route labels relative; resolving them against the current
    // page would add an unrelated path prefix. Full URLs retain only their path.
    if (/^[a-z][a-z0-9+.-]*:/i.test(value.trim()) || value.indexOf('//') === 0) {
      var cleaned = cleanURL(value);
      return cleaned ? new URL(cleaned).pathname : '';
    }
    return value.indexOf('/') < 0 ? value : value.split(/[?#]/, 1)[0];
  }
  function cleanURLAttrs(attrs) {
    ['url.full', 'http.url', 'url'].forEach(function (key) {
      if (attrs[key] !== undefined) { attrs[key] = cleanURL(attrs[key]); }
    });
    if (typeof attrs.name === 'string' && (attrs.name.indexOf('://') >= 0 || attrs.name.charAt(0) === '/')) {
      attrs.name = cleanURL(attrs.name);
    }
    return attrs;
  }
  function cleanFrames(frames) {
    return frames.map(function (frame) {
      var result = pick(frame, ['filename', 'function', 'lineno', 'colno']);
      if (result.filename !== undefined) { result.filename = cleanURL(result.filename); }
      return result;
    });
  }
  var eventAttributeKeys = {
    'faro.performance.navigation': ['pageLoadTime', 'domContentLoadHandlerTime'],
    'faro.performance.resource': ['name', 'httpHost', 'duration', 'transferSize', 'initiatorType'],
    'faro.tracing.fetch': ['url.full', 'http.request.method', 'http.response.status_code', 'duration_ns'],
    'faro.tracing.xml-http-request': ['url.full', 'http.request.method', 'http.response.status_code', 'duration_ns'],
    'view_changed': ['fromView', 'toView'],
    'rage_click': ['target'], 'dead_click': ['target'], 'error_click': ['target']
  };
  var payloadKeys = {
    event: ['name', 'domain', 'timestamp'], measurement: ['type', 'timestamp'],
    exception: ['type', 'value', 'timestamp'], log: ['message', 'level', 'timestamp'], trace: []
  };
  function shapeTraceAttrs(attrs) {
    return (attrs || []).map(function (attr) {
      var value = pick(attr.value, ['stringValue', 'intValue', 'boolValue', 'doubleValue']);
      if (['url.full', 'http.url', 'url', 'name'].indexOf(attr.key) >= 0 && typeof value.stringValue === 'string' &&
          (attr.key !== 'name' || value.stringValue.indexOf('://') >= 0 || value.stringValue.charAt(0) === '/')) {
        value.stringValue = cleanURL(value.stringValue);
      }
      return { key: attr.key, value: value };
    }).filter(function (attr) { return Object.keys(attr.value).length > 0; });
  }
  function shapeTraces(payload) {
    return { resourceSpans: (payload.resourceSpans || []).map(function (resource) {
      return {
        resource: { attributes: shapeTraceAttrs(resource.resource && resource.resource.attributes).filter(function (attr) {
          return attr.key === 'service.name' && typeof attr.value.stringValue === 'string';
        }) },
        scopeSpans: (resource.scopeSpans || []).map(function (scope) {
          return {
            scope: pick(scope.scope, ['name', 'version']),
            spans: (scope.spans || []).map(function (span) {
              var result = pick(span, ['traceId', 'spanId', 'parentSpanId', 'name', 'kind', 'startTimeUnixNano', 'endTimeUnixNano']);
              result.attributes = shapeTraceAttrs(span.attributes);
              result.status = pick(span.status, ['code', 'message']);
              return result;
            })
          };
        })
      };
    }) };
  }
  function shapeItem(item) {
    if (!Object.prototype.hasOwnProperty.call(payloadKeys, item.type)) { return null; }
    if ((item.type === 'trace' && !opt.tracing) || (item.type === 'log' && !opt.consoleLogs)) { return null; }
    var source = item.payload || {};
    if (item.type === 'event') {
      if (['securitypolicyviolation', 'faro.user.action'].indexOf(source.name) >= 0) { return null; }
      if (!opt.frustrationSignals && ['rage_click', 'dead_click', 'error_click'].indexOf(source.name) >= 0) { return null; }
    }
    // Metadata is shared with the SDK and later observations. Never rewrite it
    // in place. Its session hook runs after this hook and consumes isSampled.
    var copy = { type: item.type }, meta = item.meta || {};
    copy.meta = {
      page: pick(meta.page, ['id', 'url']), session: pick(meta.session, ['id']),
      browser: pick(meta.browser, ['name', 'version', 'os', 'mobile', 'viewportWidth']),
      view: pick(meta.view, ['name']), app: pick(meta.app, ['version', 'environment']),
      user: pick(meta.user, ['id'])
    };
    if (meta.session && meta.session.attributes) {
      copy.meta.session.attributes = pick(meta.session.attributes, ['isSampled']);
    }
    if (copy.meta.page.url !== undefined) { copy.meta.page.url = cleanURL(copy.meta.page.url); }
    if (copy.meta.view.name !== undefined) { copy.meta.view.name = cleanView(copy.meta.view.name); }
    var payload = copy.payload = pick(source, payloadKeys[item.type]);
    if (item.type === 'event') {
      var keys = Object.prototype.hasOwnProperty.call(eventAttributeKeys, source.name) ? eventAttributeKeys[source.name] : [];
      payload.attributes = opt.eventLogs ? Object.assign({}, source.attributes) : pick(source.attributes, keys || []);
      cleanURLAttrs(payload.attributes);
      if (/^faro\.performance\./.test(source.name) && payload.attributes.name !== undefined) {
        payload.attributes.name = cleanURL(payload.attributes.name);
      }
      ['fromView', 'toView'].forEach(function (key) {
        if (payload.attributes[key] !== undefined) { payload.attributes[key] = cleanView(payload.attributes[key]); }
      });
      if (source.trace) { payload.trace = pick(source.trace, ['trace_id']); }
      // Lifecycle envelopes are how metadata reaches transport on quiet pages.
    } else if (item.type === 'measurement') {
      payload.values = {};
      Object.keys(source.values || {}).forEach(function (key) {
        if (/^(lcp|fcp|ttfb|inp|cls)$/i.test(key)) { payload.values[key] = source.values[key]; }
      });
      payload.context = pick(source.context, ['element', 'interaction_target', 'largest_shift_target']);
    } else if (item.type === 'exception') {
      if (source.stacktrace) { payload.stacktrace = { frames: cleanFrames(source.stacktrace.frames || []) }; }
    } else if (item.type === 'log') {
      payload.context = pick(source.context, ['type', 'stackFrames']);
      if (payload.context.stackFrames) {
        try {
          // Faro encodes frames as space-separated JSON objects. Skip quoted
          // strings when finding separators, since a function can contain braces.
          var encoded = payload.context.stackFrames.replace(/"(?:\\.|[^"\\])*"|}\s+{/g, function (token) {
            return token.charAt(0) === '"' ? token : '},{';
          });
          var frames = JSON.parse('[' + encoded + ']');
          payload.context.stackFrames = cleanFrames(frames).map(function (frame) { return JSON.stringify(frame); }).join(' ');
        } catch (e) { delete payload.context.stackFrames; }
      }
    } else if (item.type === 'trace') {
      // Construct the decoder's OTLP subset without changing SDK-owned spans.
      copy.payload = shapeTraces(source);
    }
    return copy;
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
