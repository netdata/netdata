// SPDX-License-Identifier: GPL-3.0-or-later
// Pinned SDK, default batching and FetchTransport. Browser performance entries
// are synthetic; no network or browser delivery reliability is inferred.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const crypto = require('node:crypto');
const bundle = fs.readFileSync(process.env.FARO_SDK_BUNDLE);
assert.equal(crypto.createHash('sha256').update(bundle).digest('hex'),
  'd7be021a7344131c89c02cf5b39aa9825c21cf17482bc1cd47e23cd7023ce118');
const bootstrap = fs.readFileSync(0, 'utf8');
const bot = process.argv[2] === 'bot';
const handlers = new Map(), observers = [], requests = [];
function add(name, callback) {
  if (!handlers.has(name)) handlers.set(name, new Set());
  handlers.get(name).add(callback);
}
function remove(name, callback) { handlers.get(name)?.delete(callback); }
function fire(name, value = {}) {
  for (const fn of [...(handlers.get(name) || [])]) fn({ type: name, timeStamp: now, isTrusted: true, ...value });
}
let now = 1000, clock = Date.now(), initialized, retried = false;
class BrowserDate extends Date { static now() { return clock; } }
const nav = { name: 'https://shop.example.org/entry?private=value', responseStart: 100, fetchStart: 0,
  startTime: 0, activationStart: 0, type: 'navigate', domInteractive: 200,
  domContentLoadedEventStart: 250, domContentLoadedEventEnd: 255, domComplete: 300 };
class Observer {
  static supportedEntryTypes = ['navigation', 'resource', 'paint', 'layout-shift', 'event', 'first-input', 'largest-contentful-paint'];
  constructor(callback) { this.callback = callback; this.types = []; observers.push(this); }
  observe(options) { this.types.push(options.type); }
  disconnect() { this.types = []; }
  takeRecords() { return []; }
}
function emit(type, data) {
  for (const observer of [...observers]) {
    if (observer.types.includes(type)) observer.callback({ getEntries: () => [{ entryType: type, ...data, toJSON: () => data }] });
  }
}
const storage = () => {
  const values = new Map();
  return { getItem: k => values.get(k) ?? null, setItem: (k, v) => values.set(k, String(v)), removeItem: k => values.delete(k) };
};
function EventTiming() {}
EventTiming.prototype.interactionId = 0;
const c = {
  PerformanceEventTiming: EventTiming, TypeError, Date: BrowserDate,
  console, URL, URLSearchParams, TextEncoder, queueMicrotask,
  setTimeout: (...args) => setTimeout(...args).unref(), clearTimeout,
  setInterval: (...args) => setInterval(...args).unref(), clearInterval,
  requestAnimationFrame: f => setTimeout(() => f(now), 0), PerformanceObserver: Observer,
  PerformanceNavigationTiming: class {},
  performance: { now: () => now, timeOrigin: clock - now, getEntriesByType: t => t === 'navigation' ? [nav] : [], interactionCount: 0 },
  navigator: { webdriver: bot, userAgent: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36', language: 'en' },
  location: { href: nav.name, hostname: 'shop.example.org', origin: 'https://shop.example.org' },
  addEventListener: add, removeEventListener: remove,
  sessionStorage: storage(), localStorage: storage(),
  fetch: async (url, options) => {
    requests.push({ body: options.body, keepalive: options.keepalive });
    if (!retried) { retried = true; throw new TypeError('synthetic keepalive rejection'); }
    return { ok: true, status: 202, headers: { get: () => null }, text: async () => "" };
  },
};
c.window = c;
c.document = {
  location: c.location, cookie: '', readyState: 'complete', visibilityState: 'visible', currentScript: null,
  addEventListener: add, removeEventListener: remove, createElement() { return {}; },
  head: { appendChild(script) {
    c.location.href = 'https://shop.example.org/loading-route?private=route';
    vm.runInContext(bundle.toString(), c);
    const sdk = c.GrafanaFaroWebSdk, initialize = sdk.initializeFaro;
    sdk.initializeFaro = cfg => {
      assert.equal(cfg.batching, undefined, 'use upstream default batching');
      assert.equal(cfg.transports.length, 1);
      assert.ok(cfg.transports[0] instanceof sdk.FetchTransport, 'inherit the upstream FetchTransport delivery implementation');
      initialized = initialize(cfg);
      assert.equal(initialized.config.batching.sendTimeout, 250);
      assert.equal(initialized.config.batching.itemLimit, 50);
      return initialized;
    };
    script.onload();
  } },
};
vm.createContext(c);
const tick = () => new Promise(resolve => setTimeout(resolve, 15));
const settle = () => new Promise(resolve => setTimeout(resolve, 350));
const bodies = () => requests.filter(r => r.keepalive !== false).map(r => JSON.parse(r.body));
const records = (key) => bodies().flatMap(b => (b[key] || []).map(value => ({ value, meta: b.meta })));
async function run() {
  vm.runInContext(bootstrap, c);
  assert.ok(initialized);
  const api = initialized.api;
  const first = api.getPage().id;
  assert.ok(first);
  emit('paint', { name: 'first-contentful-paint', startTime: 150 });
  await tick();
  c.performance.interactionCount = 1;
  emit('event', { interactionId: 7, name: 'click', startTime: 250, duration: 100, processingStart: 260, processingEnd: 280 });
  emit('layout-shift', { value: 0.1, startTime: 200, hadRecentInput: false, sources: [] });
  emit('largest-contentful-paint', { startTime: 300, url: '', id: '' });
  emit('navigation', nav);
  await tick();
  emit('resource', { name: 'https://cdn.example.org/a.js?private=value', duration: 0, transferSize: 0, initiatorType: 'fetch', startTime: 10 });
  const collectorURL = 'https://rum.example.org/rum/shop/collect' + (bot ? '?bot=1' : '');
  emit('resource', { name: collectorURL, duration: 0, transferSize: 0, initiatorType: 'fetch', startTime: 20 });
  // Matching must be exact, not a shared prefix or regex metacharacter match.
  emit('resource', { name: 'https://rum.example.org/rum/shop/collect/application', duration: 0, transferSize: 0, initiatorType: 'fetch', startTime: 30 });
  emit('resource', { name: 'https://rumXexampleYorg/rum/shop/collect?app=1', duration: 0, transferSize: 0, initiatorType: 'fetch', startTime: 40 });
  await tick();
  // Late first LCP report follows SPA navigation, while document origin stays entry.
  c.location.href = 'https://shop.example.org/checkout?private=route';
  const callerView = Object.freeze({ name: 'checkout', id: 'caller-owned' });
  api.setView(callerView);
  const firstView = api.getView();
  assert.equal(callerView.id, 'caller-owned', 'setView does not mutate the caller object');
  assert.notEqual(firstView.id, callerView.id);
  api.setView(callerView);
  assert.equal(api.getView().id, firstView.id);
  c.document.visibilityState = 'hidden'; fire('visibilitychange'); await tick();
  c.document.visibilityState = 'visible'; fire('visibilitychange');
  now = 12000; clock += 11000;
  c.performance.interactionCount = 2;
  emit('event', { interactionId: 14, name: 'click', startTime: 11800, duration: 600, processingStart: 11810, processingEnd: 11900 });
  emit('layout-shift', { value: 0.3, startTime: 11800, hadRecentInput: false, sources: [] });
  const firstViewID = firstView.id;
  api.setView({ name: 'receipt' });
  assert.equal(firstView.id, firstViewID, 'later transitions do not mutate earlier view identity');
  await tick();
  c.document.visibilityState = 'hidden'; fire('visibilitychange'); await tick();
  // Restore before the 250ms queue drains. Old reports must retain old metadata.
  c.document.visibilityState = 'visible'; now = 13000; fire('pageshow', { persisted: true });
  const second = api.getPage().id;
  assert.notEqual(second, first);
  await tick();
  c.document.visibilityState = 'hidden'; fire('visibilitychange'); await settle();
  const originalEvents = records('events');
  const activations = originalEvents.filter(r => r.value.name === 'document_activated');
  assert.equal(activations.length, 2, 'startup + BFCache, independent of report gap and session events');
  assert.deepEqual(activations.map(r => r.value.attributes.observation_id), [first, second]);
  const views = originalEvents.filter(r => r.value.name === 'view_changed');
  assert.equal(views.length, 2, 'initial assignment and distinct transition, same assignment is not another view');
  assert.notEqual(views[0].value.attributes.observation_id, views[1].value.attributes.observation_id);
  for (const r of views) assert.equal(r.meta.view.id, r.value.attributes.observation_id);
  const resources = originalEvents.filter(r => r.value.name === 'faro.performance.resource');
  assert.equal(resources.length, 3, 'own transport excluded; unrelated application URLs retained');
  assert.ok(!resources.some(r => r.value.attributes.name === 'https://rum.example.org/rum/shop/collect'));
  assert.ok(resources.some(r => r.value.attributes.name === 'https://rum.example.org/rum/shop/collect/application'));
  assert.ok(resources.some(r => r.value.attributes.name === 'https://rumxexampleyorg/rum/shop/collect'));
  assert.ok(resources[0].value.attributes.observation_id);
  assert.equal(resources[0].value.attributes.duration, '0');
  const measurements = records('measurements');
  for (const vital of ['cls', 'inp']) {
    const reports = measurements.filter(r => r.meta.page.id === first && r.value.values[vital] !== undefined);
    assert.equal(reports.length, 2, vital + ' repeats with changed value');
    assert.equal(reports[0].value.context.id, reports[1].value.context.id);
    assert.ok(Number(reports[1].value.context.observation_sequence) > Number(reports[0].value.context.observation_sequence));
  }
  assert.ok(measurements.some(r => r.meta.page.id === first && r.value.values.lcp === 300));
  assert.ok(measurements.some(r => r.meta.page.id === second), 'restored vitals have the new activation');
  const identities = new Map();
  for (const r of measurements) {
    assert.equal(r.meta.page.url, 'https://shop.example.org/entry');
    assert.ok(r.value.context.id);
    assert.ok(Number(r.value.context.observation_sequence) > 0);
    if (identities.has(r.value.context.id)) assert.equal(identities.get(r.value.context.id), r.meta.page.id);
    identities.set(r.value.context.id, r.meta.page.id);
  }
  assert.ok(bodies().some(b => (b.measurements || []).length > 1), 'per-item revision must not split metadata batches');
  const retryIndex = requests.findIndex(r => r.keepalive === false);
  assert.ok(retryIndex >= 0);
  assert.equal(requests[retryIndex].body, requests[0].body, 'retry preserves the identical serialized identity and revision');
  assert.equal(requests.filter(r => r.keepalive === false).length, 1);
  // Session replacement changes only session identity, never document identity.
  api.setSession({ id: 'replacement-session', attributes: { isSampled: 'true' } });
  const callerAttrs = Object.freeze({ key: 'value' });
  api.pushEvent('after_session_change', callerAttrs);
  assert.deepEqual(callerAttrs, { key: 'value' }, 'instrumentation identity wrappers do not mutate caller attributes');
  await settle();
  const last = records('events').find(r => r.value.name === 'after_session_change');
  assert.equal(last.meta.page.id, second);
  assert.equal(last.meta.session.id, 'replacement-session');
  assert.equal(last.value.attributes?.observation_id, undefined, 'raw pushEvent is not identity-stamped');
  assert.equal(last.value.attributes?.observation_sequence, undefined);
  const sessions = records('events').filter(r => ['session_start', 'session_resume', 'session_extend'].includes(r.value.name));
  assert.equal(sessions.length, 2);
  assert.equal(new Set(sessions.map(r => r.value.attributes.observation_id)).size, sessions.length);
  for (const event of sessions) {
    assert.ok(event.value.attributes.observation_id);
    assert.ok(Number(event.value.attributes.observation_sequence) > 0);
  }
  assert.equal(records('events').filter(r => r.value.name === 'document_activated').length, 2);
  initialized.pause();
  process.stdout.write(JSON.stringify(bodies()));
}
run().catch(error => { console.error(error); process.exitCode = 1; });
