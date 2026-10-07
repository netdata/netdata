(function (k, opt) {
  // data-version/data-env on our OWN <script> tag become
  // Faro app.version/app.environment; currentScript must be read
  // synchronously, before anything yields, or it stops pointing here.
  // Named HTML elements can shadow document.currentScript. Read the native
  // getter so only the executing script supplies URLs, attributes and nonce.
  var cs;
  try { cs = Object.getOwnPropertyDescriptor(Document.prototype, 'currentScript').get.call(document); } catch (e) {}
  var entryURL = location.href;
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
  // Capture the original console method before Faro can instrument it. Messages
  // contain no page URLs, identifiers or exception text and never throw.
  var warn;
  try { warn = window.console.warn.bind(window.console); } catch (e) {}
  function diagnose(message) {
    try { if (warn) { warn('Netdata RUM: ' + message); } } catch (e) {}
  }
  var scriptURL, nonce, collectorURL;
  try {
    if (!cs || !cs.src) { throw new Error('external script required'); }
    scriptURL = new URL(cs.src);
    if (!/^https?:$/.test(scriptURL.protocol) || scriptURL.username || scriptURL.password) {
      throw new Error('invalid script URL');
    }
    nonce = cs.nonce;
    // Relative resolution keeps the external proxy prefix. src is the element's
    // requested URL, not the final URL of an HTTP redirect.
    collectorURL = new URL(encodeURIComponent(k) + '/collect', scriptURL);
    if (bot) { collectorURL.searchParams.set('bot', '1'); }
    collectorURL = collectorURL.href;
  } catch (e) {
    diagnose('cannot determine receiver URL. Use the external script tag from RUM setup.');
    return;
  }
  function load(path, next, failed, name) {
    function failure() {
      diagnose('cannot load ' + name + '. Check the browser Network and CSP errors for the RUM script requests.');
      if (failed) { failed(); }
    }
    try {
      var s = document.createElement('script');
      s.async = true;
      if (nonce) { s.nonce = nonce; }
      s.src = new URL(path, scriptURL).href;
      s.onload = next;
      s.onerror = failure;
      document.head.appendChild(s);
    } catch (e) { failure(); }
  }
  function start() {
    try {
      var app = { name: document.location.hostname };
      if (ver) { app.version = ver; }
      if (env) { app.environment = env; }
      var sdk = GrafanaFaroWebSdk;
      // The metadata provider returns a fresh object before queueing each item.
      // Never stamp origin in beforeSend: it runs when a queued batch flushes.
      var sequence = 0, occurrence = 0;
      var documentID = sdk.genShortID();
      var activation = { id: documentID + ':' + (++occurrence), url: entryURL };
      function revision() { return String(++sequence); }
      function eventIdentity(attrs) {
        var seq = revision();
        return Object.assign({}, attrs, { observation_id: activation.id + ':' + seq, observation_sequence: seq });
      }
      class DocumentPerformance extends sdk.PerformanceInstrumentation {
        initialize() {
          var push = this.api.pushEvent;
          this.api = Object.assign({}, this.api, { pushEvent: function (name, attrs, domain, options) {
            push(name, eventIdentity(attrs), domain, options);
          } });
          super.initialize();
        }
      }
      class DocumentVitals extends sdk.WebVitalsInstrumentation {
        initialize() {
          var push = this.api.pushMeasurement;
          this.api = Object.assign({}, this.api, { pushMeasurement: function (measurement, options) {
            options = options || {};
            push(measurement, Object.assign({}, options, {
              context: Object.assign({}, options.context, { observation_sequence: revision() })
            }));
          } });
          super.initialize();
        }
      }
      class DocumentSessions extends sdk.SessionInstrumentation {
        initialize() {
          var push = this.api.pushEvent;
          this.api = Object.assign({}, this.api, { pushEvent: function (name, attrs, domain, options) {
            push(name, eventIdentity(attrs), domain, options);
          } });
          super.initialize();
        }
      }
      class ApplicationViews extends sdk.ViewInstrumentation {
        initialize() {
          var push = this.api.pushEvent;
          this.api = Object.assign({}, this.api, { pushEvent: function (name, attrs, domain, options) {
            var id = this.getView().id;
            push(name, Object.assign({}, attrs, { observation_id: id, observation_sequence: revision() }), domain, options);
          } });
          super.initialize();
        }
      }
      function activate() {
        var f = faroApi();
        if (f) { f.pushEvent('document_activated', { observation_id: activation.id, observation_sequence: revision() }); }
      }
      // Register before web-vitals registers its BFCache callbacks.
      window.addEventListener('pageshow', function (event) {
        if (!event.persisted) { return; }
        activation = { id: documentID + ':' + (++occurrence), url: entryURL };
        activate();
      });
      var inst = [new DocumentPerformance(), new sdk.ErrorsInstrumentation(),
        new DocumentVitals(), new DocumentSessions(), new ApplicationViews()];
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
      var collectorPattern = new RegExp('^' + collectorURL.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '$');
      // Faro treats URL strings as regex patterns and otherwise observes its
      // own ?bot=1 requests. Inherit delivery unchanged; match only this URL.
      class CollectorTransport extends sdk.FetchTransport {
        getIgnoreUrls() { return [collectorPattern]; }
      }
      var cfg = { transports: [new CollectorTransport({ url: collectorURL })], app: app, instrumentations: inst, beforeSend: shapeItem };
      if (opt.consoleLogs) { cfg.consoleInstrumentation = { consoleErrorAsLog: true }; }
      cfg.sessionTracking = { samplingRate: opt.sampling };
      cfg.metas = [sdk.browserMeta, sdk.osMeta, sdk.sdkMeta, function () { return { page: { id: activation.id, url: activation.url } }; }];
      var faro = GrafanaFaroWebSdk.initializeFaro(cfg);
      var setView = faro.api.setView;
      faro.api.setView = function (view, options) {
        var previous = faro.api.getView();
        var id = previous && previous.name === (view && view.name) ? previous.id : documentID + ':view:' + (++occurrence);
        setView(view && Object.assign({}, view, { id: id }), options);
      };
      // Application page metadata must not replace the document's entry origin.
      faro.api.setPage = function () {};
      if (user) { applyUser(); }
      activate();
      if (opt.frustrationSignals) { watchFrustration(); }
    } catch (e) { diagnose('SDK initialization failed. Check the browser console and site script policy.'); }
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
    'faro.performance.navigation': ['pageLoadTime', 'domContentLoadHandlerTime', 'observation_id', 'observation_sequence'],
    'faro.performance.resource': ['name', 'httpHost', 'duration', 'transferSize', 'initiatorType', 'observation_id', 'observation_sequence'],
    'faro.tracing.fetch': ['url.full', 'http.request.method', 'http.response.status_code', 'duration_ns'],
    'faro.tracing.xml-http-request': ['url.full', 'http.request.method', 'http.response.status_code', 'duration_ns'],
    'session_start': ['observation_id', 'observation_sequence'],
    'session_resume': ['observation_id', 'observation_sequence'],
    'session_extend': ['observation_id', 'observation_sequence'],
    'document_activated': ['observation_id', 'observation_sequence'],
    'view_changed': ['fromView', 'toView', 'observation_id', 'observation_sequence'],
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
      view: pick(meta.view, ['name', 'id']), app: pick(meta.app, ['version', 'environment']),
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
      payload.context = pick(source.context, ['id', 'observation_sequence', 'element', 'interaction_target', 'largest_shift_target']);
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
  load('__SDK_PATH__', function () {
    if (opt.tracing) { load('__TRACING_PATH__', start, start, 'the tracing SDK'); } else { start(); }
  }, null, 'the core SDK');
})(__KEY__, __OPT__);
