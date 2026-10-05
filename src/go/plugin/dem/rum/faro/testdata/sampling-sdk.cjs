// SPDX-License-Identifier: GPL-3.0-or-later
// Faro 2.11.0 from the pinned jsDelivr URL used by bootstrap.go. We use the
// actual SDK initialization, SessionInstrumentation and transport filtering.
// Browser APIs are synthetic; configured instrumentations, metadata and
// transport hooks are the unchanged SDK implementations.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const crypto = require('node:crypto');
const bundle = fs.readFileSync(process.env.FARO_SDK_BUNDLE);
assert.equal(crypto.createHash('sha256').update(bundle).digest('hex'),
  'd7be021a7344131c89c02cf5b39aa9825c21cf17482bc1cd47e23cd7023ce118');
const bootstraps = JSON.parse(fs.readFileSync(0, 'utf8'));
function storage() {
  const values = new Map();
  return {
    getItem: k => values.get(k) ?? null,
    setItem: (k, v) => values.set(k, String(v)),
    removeItem: k => values.delete(k),
  };
}
function run({ rate, random = 0.8, savedStorage = storage(), bootstrap, bot = false }) {
  const delivered = [];
  let clock = Date.now(), sessionSequence = 0, timerSequence = 0;
  const timers = new Map();
  class BrowserDate extends Date { static now() { return clock; } }
  let loads = 0, initialized = 0, listeners = 0, storageReads = 0;
  const math = Object.create(Math);
  math.random = () => random;
  const context = {
    console, URL, URLSearchParams, TextEncoder, Math: math, Date: BrowserDate,
    performance: { now: () => 100, timeOrigin: Date.now(), getEntriesByType: () => [] },
    innerWidth: 1024, innerHeight: 768,
    setTimeout(callback, delay) { const id = ++timerSequence; timers.set(id, { callback, at: clock + delay }); return id; },
    clearTimeout(id) { timers.delete(id); },
    navigator: { userAgent: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36', language: 'en', webdriver: bot },
    location: { href: 'https://shop.example.org/', hostname: 'shop.example.org', origin: 'https://shop.example.org' },
    addEventListener() { listeners++; }, removeEventListener() {},
    sessionStorage: {
      ...savedStorage,
      getItem(k) { storageReads++; return savedStorage.getItem(k); },
    },
    localStorage: storage(),
  };
  function advanceTime(delay) {
    clock += delay;
    for (const [id, timer] of Array.from(timers)) {
      if (timer.at <= clock && timers.delete(id)) { timer.callback(); }
    }
  }
  // A due callback may cancel another due timer before its turn.
  let cancelledTimer;
  context.setTimeout(() => context.clearTimeout(cancelledTimer), 0);
  cancelledTimer = context.setTimeout(() => assert.fail('cancelled timer ran'), 0);
  advanceTime(0);
  context.window = context;
  context.document = {
    location: context.location, currentScript: null, readyState: 'complete',
    cookie: '', visibilityState: 'visible',
    addEventListener() { listeners++; }, removeEventListener() {},
    createElement() { return {}; },
    head: { appendChild(script) { loads++; loadSDK(); script.onload(); } },
  };
  vm.createContext(context);
  let faro;
  function loadSDK() {
    vm.runInContext(bundle.toString(), context);
    const sdk = context.GrafanaFaroWebSdk;
    assert.equal(sdk.VERSION, '2.11.0');
    class Capture extends sdk.BaseTransport {
      constructor() { super(); this.name = 'sampling-fixture'; this.version = '1'; }
      isBatched() { return false; }
      send(item) { delivered.push(item); }
    }
    const initialize = sdk.initializeFaro;
    sdk.initializeFaro = cfg => {
      initialized++;
      assert.equal(cfg.sessionTracking.samplingRate, rate);
      faro = initialize({ ...cfg, url: undefined, transports: [new Capture()], batching: { enabled: false },
        sessionTracking: { ...cfg.sessionTracking, generateSessionId: () => 'sampling-session-' + (++sessionSequence) } });
      return faro;
    };
  }
  if (bootstrap !== undefined) {
    vm.runInContext(bootstrap, context);
    // Public calls remain safe even when instrumentation is disabled.
    context.netdataRum.setUser({ id: 'synthetic-user' });
    context.netdataRum.setUser(null);
  } else {
    loadSDK();
    const sdk = context.GrafanaFaroWebSdk;
    sdk.initializeFaro({ app: { name: 'sampling-fixture' },
      instrumentations: [new sdk.SessionInstrumentation()], sessionTracking: { samplingRate: rate } });
  }
  if (faro) {
    faro.api.pushEvent('known_event', { key: 'value' });
    faro.api.pushError(new Error('known_error'));
    faro.api.pushMeasurement({ type: 'web-vitals', values: { lcp: 1200 } });
  }
  return { sampled: faro?.api.getSession()?.attributes?.isSampled, delivered,
    savedStorage, loads, initialized, listeners, storageReads,
    rollover(nextRandom) {
      const previous = faro.api.getSession().id;
      random = nextRandom;
      advanceTime(16 * 60 * 1000);
      faro.api.pushEvent('after_idle_' + clock);
      assert.notEqual(faro.api.getSession().id, previous);
      return faro.api.getSession().attributes.isSampled;
    } };
}
function checkSelected(result, expected) {
  assert.equal(result.sampled, String(expected));
  for (const type of ['event', 'exception', 'measurement']) {
    assert.equal(result.delivered.some(item => item.type === type), expected, type);
  }
}
// First verify the upstream semantics independently of our generated loader.
for (const [rate, random, selected] of [[0, 0.4, false], [0.25, 0.1, true], [0.25, 0.8, false], [1, 0.9, true]]) {
  checkSelected(run({ rate, random }), selected);
}
const selected = run({ rate: 1 });
// A stored decision is resumed, even if the current configured rate is zero.
checkSelected(run({ rate: 0, savedStorage: selected.savedStorage }), true);
const excluded = run({ rate: 0.25 });
checkSelected(run({ rate: 1, savedStorage: excluded.savedStorage }), false);
for (const savedStorage of [storage(), selected.savedStorage]) {
  const result = run({ rate: 0, savedStorage, bootstrap: bootstraps.zero });
  assert.equal(result.loads, 0);
  assert.equal(result.initialized, 0);
  assert.equal(result.listeners, 0);
  assert.equal(result.storageReads, 0);
  assert.equal(result.delivered.length, 0);
}
for (const [name, rate, random, selected] of [['fraction', 0.25, 0.1, true], ['fraction', 0.25, 0.8, false], ['full', 1, 0.9, true]]) {
  const result = run({ rate, random, bootstrap: bootstraps[name] });
  assert.equal(result.loads, 1);
  assert.equal(result.initialized, 1);
  checkSelected(result, selected);
}
// Existing bot exclusion is still applied before loading the SDK at rate 1.
assert.equal(run({ rate: 1, bootstrap: bootstraps.full, bot: true }).loads, 0);

// Expiry is driven by browser time and a public event. The SDK owns the new
// session and rewrites/filtering of the event that triggered the rollover.
const rollover = run({ rate: 0.25, random: 0.1, bootstrap: bootstraps.fraction });
const beforeRollover = rollover.delivered.length;
assert.equal(rollover.rollover(0.8), 'false');
assert.equal(rollover.delivered.length, beforeRollover);
assert.equal(rollover.rollover(0.1), 'true');
assert.ok(rollover.delivered.slice(beforeRollover).some(item => item.payload.name.startsWith('after_idle_')));
assert.ok(rollover.delivered.slice(beforeRollover).some(item => ['session_start', 'session_extend'].includes(item.payload.name)));
