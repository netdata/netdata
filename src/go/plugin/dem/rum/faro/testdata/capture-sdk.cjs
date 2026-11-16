// SPDX-License-Identifier: GPL-3.0-or-later
// Actual pinned initialization, metadata providers, instrumentations, session
// hooks and transport serialization; only browser APIs and transport are fakes.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const crypto = require('node:crypto');
const bundle = fs.readFileSync(require('node:path').join(__dirname, '../assets/faro-web-sdk.iife.js'));
assert.equal(crypto.createHash('sha256').update(bundle).digest('hex'),
  'd7be021a7344131c89c02cf5b39aa9825c21cf17482bc1cd47e23cd7023ce118');
const bootstrap = fs.readFileSync(0, 'utf8');
const enabled = process.argv[2] === 'true';
const storage = () => {
  const values = new Map();
  return { getItem: k => values.get(k) ?? null, setItem: (k, v) => values.set(k, String(v)), removeItem: k => values.delete(k) };
};
const listeners = {}, observers = [], delivered = [];
let mutationObservers = 0, initialized, sdk;
const context = {
  console: Object.fromEntries(['info', 'warn', 'error', 'log', 'debug', 'trace'].map(k => [k, () => {}])),
  URL, URLSearchParams, TextEncoder, queueMicrotask,
  setTimeout: (...args) => setTimeout(...args).unref(), clearTimeout,
  navigator: { userAgent: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36', language: 'en' },
  location: { href: 'https://user:password@shop.example.org/products/123456?secret=page#fragment', hostname: 'shop.example.org', origin: 'https://shop.example.org' },
  performance: { now: () => 100, timeOrigin: Date.now() - 100, getEntriesByType: () => [] },
  innerWidth: 1024, innerHeight: 768,
  addEventListener(name, callback) { (listeners[name] ??= []).push(callback); }, removeEventListener() {},
  sessionStorage: storage(), localStorage: storage(),
  PerformanceObserver: class {
    static supportedEntryTypes = ['navigation', 'resource'];
    constructor(callback) { this.callback = callback; observers.push(this); }
    observe(options) { this.options = options; }
    disconnect() {} takeRecords() { return []; }
  },
  MutationObserver: class { constructor() { mutationObservers++; } observe() {} disconnect() {} },
};
context.window = context;
context.document = {
  location: context.location, cookie: '', readyState: 'complete', visibilityState: 'visible',
  currentScript: { src: 'https://rum.example.org/rum/shop.js', getAttribute: key => ({ 'data-version': 'release-1', 'data-env': 'test' })[key] },
  addEventListener(name, callback) { (listeners['document:' + name] ??= []).push(callback); }, removeEventListener() {},
  createElement() { return {}; },
  head: { appendChild(script) {
    if (script.src.includes('faro-web-tracing')) { script.onerror(); return; }
    assert.match(script.src, /faro-web-sdk-2\.11\.0-/);
    vm.runInContext(bundle.toString(), context);
    sdk = context.GrafanaFaroWebSdk;
    class Capture extends sdk.BaseTransport {
      constructor() { super(); this.name = 'capture-fixture'; this.version = '1'; }
      isBatched() { return false; }
      send(item) { delivered.push(JSON.parse(JSON.stringify(item))); }
    }
    const initialize = sdk.initializeFaro;
    sdk.initializeFaro = cfg => {
      assert.deepEqual(Array.from(cfg.instrumentations, i => i.name.split(':')[1]), [
        'instrumentation-performance', 'instrumentation-errors', 'instrumentation-web-vitals',
        'instrumentation-session', 'instrumentation-view', ...(enabled ? ['instrumentation-console'] : []),
      ]);
      initialized = initialize({ ...cfg, url: undefined, transports: [new Capture()], batching: { enabled: false } });
      return initialized;
    };
    script.onload();
  } },
};
require('./script-context.cjs')(context);
vm.createContext(context);
const plain = value => JSON.parse(JSON.stringify(value));
async function run() {
  vm.runInContext(bootstrap, context);
  assert.ok(initialized, 'SDK initialization must complete');
  assert.equal(mutationObservers, 0, 'default instrumentation installs no DOM observer');
  assert.equal((listeners['document:click'] || []).length, enabled ? 1 : 0);
  assert.equal((listeners.securitypolicyviolation || []).length, 0);
  assert.equal((listeners['document:securitypolicyviolation'] || []).length, 0);
  const api = initialized.api;
  const sourceMeta = plain(initialized.metas.value);
  assert.equal(sourceMeta.page.url, context.location.href);
  assert.ok(sourceMeta.browser.userAgent);
  api.setPage({ id: 'page-1', url: context.location.href });
  api.setUser({ id: 'user-1', email: 'private@example.org', username: 'unused' });
  api.pushEvent('identified', { custom: 'retained only in event logs', 'url.full': '/api?secret=custom#fragment' });
  context.netdataRum.setUser(null);
  api.pushEvent('anonymous', { custom: 'absent by default' });
  context.netdataRum.setUser({ id: 'user-2', email: 'ignored@example.org' });
  api.pushEvent('second_user');
  const identified = delivered.find(item => item.payload.name === 'identified');
  assert.deepEqual(identified.meta.user, { id: 'user-1' });
  assert.deepEqual(delivered.find(item => item.payload.name === 'anonymous').meta.user, {});
  assert.deepEqual(delivered.find(item => item.payload.name === 'second_user').meta.user, { id: 'user-2' });
  assert.deepEqual(identified.payload.attributes, enabled ? { custom: 'retained only in event logs', 'url.full': 'https://shop.example.org/api' } : {});
  assert.equal(identified.meta.page.url, 'https://shop.example.org/products/123456');
  assert.equal(identified.meta.page.id, sourceMeta.page.id);
  assert.notEqual(identified.meta.page.id, 'page-1', 'application page metadata cannot replace document identity');
  assert.deepEqual(Object.keys(identified.meta).sort(), ['app', 'browser', 'page', 'session', 'user', 'view']);
  assert.deepEqual(Object.keys(identified.meta.browser).sort(), ['mobile', 'name', 'os', 'version', 'viewportWidth']);
  assert.deepEqual(identified.meta.app, { version: 'release-1', environment: 'test' });
  assert.equal(initialized.metas.value.page.url, context.location.href, 'shared metadata must remain intact');
  assert.equal(initialized.metas.value.session.attributes.isSampled, 'true');
  const signalCount = delivered.length;
  api.pushEvent('rage_click', { target: 'button#pay' });
  assert.equal(delivered.length, signalCount + (enabled ? 1 : 0));
  // No custom event follows these view changes: lifecycle transport alone must
  // deliver both transitions even though native normalization groups their IDs.
  api.setView({ name: '/orders/123456?secret=first-view#fragment' });
  api.setView({ name: 'https://account:password@shop.example.org/orders/987654?secret=second-view#fragment' });
  const views = delivered.filter(item => item.payload.name === 'view_changed');
  assert.equal(views.length, 2);
  assert.equal(views[1].payload.attributes.fromView, '/orders/123456');
  assert.equal(views[1].payload.attributes.toView, '/orders/987654');
  assert.equal(views[1].meta.view.name, '/orders/987654');
  const emit = (type, entries) => {
    const matches = observers.filter(o => o.options?.type === type);
    assert.ok(matches.length, type + ' observer');
    for (const observer of matches) observer.callback({ getEntries: () => entries.map(entry => ({ ...entry, toJSON: () => entry })) });
  };
  emit('navigation', [{ name: context.location.href, entryType: 'navigation', duration: 90,
    fetchStart: 1, domComplete: 91, domContentLoadedEventStart: 70, domContentLoadedEventEnd: 75, startTime: 0 }]);
  await Promise.resolve(); await Promise.resolve();
  emit('resource', [{ name: 'https://account:password@cdn.example.org/app.js?secret=resource#fragment', entryType: 'resource',
    duration: 23, transferSize: 456, initiatorType: 'fetch', startTime: 10 }]);
  const navigation = delivered.find(item => item.payload.name === 'faro.performance.navigation');
  assert.equal(navigation.payload.attributes.pageLoadTime, '90');
  assert.equal(navigation.payload.attributes.domContentLoadHandlerTime, '5');
  const resource = delivered.find(item => item.payload.name === 'faro.performance.resource');
  assert.equal(resource.payload.attributes.name, 'https://cdn.example.org/app.js');
  assert.equal(resource.payload.attributes.httpHost, 'cdn.example.org');
  // Real public SDK APIs exercise typed measurement/request/trace shaping.
  api.pushMeasurement({ type: 'web-vitals', values: { lcp: 1200, unused: 999 }, action: { name: 'discard' }, unused: 'discard' }, {
    context: { element: 'button#pay', unused: 'discard' },
    spanContext: { traceId: '12345678901234567890123456789012', spanId: '1234567890123456' },
  });
  const measurement = delivered.find(item => item.type === 'measurement');
  assert.deepEqual(measurement.payload.context, { element: 'button#pay' });
  assert.deepEqual(measurement.payload.values, { lcp: 1200 });
  assert.deepEqual(Object.keys(measurement.payload).sort(), ['context', 'timestamp', 'type', 'values']);
  api.pushEvent('faro.tracing.fetch', { 'url.full': '/api?secret=request#fragment', 'http.request.method': 'GET',
    'http.response.status_code': '200', duration_ns: '1000000', unused: 'discard' });
  assert.equal(delivered.at(-1).payload.attributes['url.full'], 'https://shop.example.org/api');
  vm.runInContext(`
    const error = new TypeError('object error');
    error.stack = 'TypeError: object error\\n    at render (https://account:password@shop.example.org/app.js?secret=stack#fragment:12:3)\\n    at boot (https://shop.example.org/main.js?secret=stack#fragment:22:4)';
    onerror('Uncaught TypeError: object error', location.href, 1, 2, error);
    console.error(error);
  `, context);
  const exception = delivered.find(item => item.type === 'exception');
  assert.equal(exception.payload.stacktrace.frames[0].filename, 'https://shop.example.org/app.js');
  if (enabled) {
    const log = delivered.find(item => item.type === 'log');
    assert.ok(log.payload.context.stackFrames.includes('https://shop.example.org/app.js'));
    assert.ok(!log.payload.context.stackFrames.includes('secret='));
    assert.ok(!log.payload.context.stackFrames.includes('account:password'));
  }
  const traces = { unknown: 'discard', resourceSpans: [{ schemaUrl: 'discard',
    resource: { droppedAttributesCount: 2, unknown: 'discard', attributes: [
      { key: 'service.name', value: { stringValue: 'shop' } },
      { key: 'browser.user_agent', value: { stringValue: 'unused' } },
    ] },
    scopeSpans: [{ schemaUrl: 'discard', unknown: 'discard', scope: { name: 'capture-fixture', version: '1', attributes: [{ key: 'unused', value: { stringValue: 'discard' } }] }, spans: [{
      traceId: '12345678901234567890123456789012', spanId: '1234567890123456', parentSpanId: 'abcdef0123456789', name: 'GET', kind: 3,
      traceState: 'discard', flags: 1, unknown: 'discard',
      events: [{ name: 'discard', attributes: [{ key: 'url.full', value: { stringValue: 'data:secret' } }] }],
      links: [{ traceId: 'discard', attributes: [{ key: 'url.full', value: { stringValue: 'data:secret' } }] }],
      status: { code: 1, message: 'done', unknown: 'discard' },
      startTimeUnixNano: '1000000000', endTimeUnixNano: '1001000000',
      attributes: [{ key: 'http.url', value: { stringValue: 'https://account:password@api.example.org/orders?secret=trace#fragment' } },
        { key: 'name', unknown: 'discard', value: { stringValue: 'checkout', unknown: 'discard' } },
        { key: 'unused', value: { arrayValue: { values: [{ stringValue: 'discard' }] } } }],
    }] }],
  }] };
  const traceCount = delivered.length;
  api.pushTraces(traces);
  if (enabled) {
    const trace = delivered.at(-1);
    assert.equal(trace.type, 'trace');
    assert.deepEqual(trace.payload, { resourceSpans: [{
      resource: { attributes: [{ key: 'service.name', value: { stringValue: 'shop' } }] },
      scopeSpans: [{ scope: { name: 'capture-fixture', version: '1' }, spans: [{
        traceId: '12345678901234567890123456789012', spanId: '1234567890123456', parentSpanId: 'abcdef0123456789',
        name: 'GET', kind: 3, startTimeUnixNano: '1000000000', endTimeUnixNano: '1001000000',
        status: { code: 1, message: 'done' },
        attributes: [
          { key: 'http.url', value: { stringValue: 'https://api.example.org/orders' } },
          { key: 'name', value: { stringValue: 'checkout' } },
        ],
      }] }],
    }] });
    assert.ok(traces.resourceSpans[0].scopeSpans[0].spans[0].attributes[0].value.stringValue.includes('?secret='), 'shared trace remains intact');
  } else { assert.equal(delivered.length, traceCount); }
  // Preserve the real SDK's wire serialization; each body carries its own
  // metadata, ready for the native HTTP receiver instead of an invented shape.
  const bodies = delivered.map(item => sdk.getTransportBody([item]));
  // Additional boundary probes use the same real SDK, but keep the cross-layer
  // fixture above focused on one navigation, resource, error and trace.
  api.setView({ name: 'Checkout?logical#label' });
  assert.equal(delivered.at(-1).meta.view.name, 'Checkout?logical#label');
  assert.equal(delivered.at(-1).payload.attributes.toView, 'Checkout?logical#label');
  assert.equal(delivered.at(-1).payload.attributes.fromView, '/orders/987654');
  api.setView({ name: 'orders/list?secret=relative#fragment' });
  assert.equal(delivered.at(-1).meta.view.name, 'orders/list');
  assert.equal(delivered.at(-1).payload.attributes.toView, 'orders/list');
  assert.equal(initialized.metas.value.view.name, 'orders/list?secret=relative#fragment');
  let count = delivered.length;
  api.pushEvent('securitypolicyviolation', { blockedURI: 'data:private', sample: 'private' });
  api.pushEvent('faro.user.action', { name: 'private' });
  assert.equal(delivered.length, count);
  api.pushEvent('__proto__');
  assert.equal(delivered.at(-1).payload.name, '__proto__');
  api.pushEvent('allowlist_event', { custom: 'optional' }, 'browser', {
    spanContext: { traceId: '12345678901234567890123456789012', spanId: '1234567890123456' },
    customPayloadTransformer: payload => ({ ...payload, action: { name: 'discard' }, unknown: 'discard' }),
  });
  assert.deepEqual(Object.keys(delivered.at(-1).payload).sort(), ['attributes', 'domain', 'name', 'timestamp', 'trace']);
  assert.deepEqual(delivered.at(-1).payload.trace, { trace_id: '12345678901234567890123456789012' });
  const error = vm.runInContext(`new Error('allowlist error', { cause: new Error('unused cause') })`, context);
  api.pushError(error, { fingerprint: 'unused', fatal: true, context: { arbitrary: 'unused' },
    stackFrames: [{ filename: 'https://account:password@shop.example.org/error.js?secret=unused#fragment', function: 'render', lineno: 7, colno: 8, unused: 'discard' }] });
  assert.deepEqual(Object.keys(delivered.at(-1).payload).sort(), ['stacktrace', 'timestamp', 'type', 'value']);
  assert.deepEqual(delivered.at(-1).payload.stacktrace.frames, [{ filename: 'https://shop.example.org/error.js', function: 'render', lineno: 7, colno: 8 }]);
  if (enabled) {
    api.pushLog(['allowlist log'], { context: { type: 'Error', stackFrames: '', arbitrary: 'discard' },
      spanContext: { traceId: '12345678901234567890123456789012', spanId: '1234567890123456' } });
    assert.deepEqual(Object.keys(delivered.at(-1).payload).sort(), ['context', 'level', 'message', 'timestamp']);
    assert.deepEqual(delivered.at(-1).payload.context, { type: 'Error', stackFrames: '' });
  }
  for (const url of ['data:,private', 'da\tta:,private', 'java\nscript:private', 'data:text/plain,private', 'blob:https://account:password@shop.example.org/private?secret=opaque', 'javascript:private']) {
    api.setPage({ id: 'opaque-page', url });
    api.pushEvent('opaque_url', { 'url.full': url }, 'browser', { skipDedupe: true });
    assert.equal(delivered.at(-1).meta.page.url, 'https://shop.example.org/products/123456', 'document entry stays frozen');
    assert.deepEqual(delivered.at(-1).payload.attributes, enabled ? { 'url.full': '' } : {});
    api.pushError(error, { skipDedupe: true, stackFrames: [{ filename: url, function: 'render', lineno: 1, colno: 2 }] });
    assert.equal(delivered.at(-1).payload.stacktrace.frames[0].filename, '');
    if (enabled) {
      api.pushLog(['opaque stack'], { skipDedupe: true, context: { stackFrames: JSON.stringify({ filename: url, function: 'render } {', lineno: 1, colno: 2 }) } });
      assert.deepEqual(JSON.parse(delivered.at(-1).payload.context.stackFrames), { filename: '', function: 'render } {', lineno: 1, colno: 2 });
    }
    api.setView({ name: url });
    assert.equal(delivered.at(-1).meta.view.name, '');
    assert.equal(delivered.at(-1).payload.attributes.toView, '');
  }
  process.stdout.write(JSON.stringify(bodies));
}
run().catch(error => { console.error(error); process.exitCode = 1; });
