// SPDX-License-Identifier: GPL-3.0-or-later
// Execute the unchanged Faro 2.11.0 IIFE in a minimal browser surface. Only
// initialization scheduling and the receiving Faro API are test doubles.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const crypto = require('node:crypto');
const bundle = fs.readFileSync(process.env.FARO_SDK_BUNDLE);
assert.equal(crypto.createHash('sha256').update(bundle).digest('hex'),
  'd7be021a7344131c89c02cf5b39aa9825c21cf17482bc1cd47e23cd7023ce118');
const bootstrap = fs.readFileSync(0, 'utf8');
const enabled = process.argv[2] === 'true';
const listeners = {};
const logs = [], exceptions = [];
const context = {
  console: Object.fromEntries(['info', 'warn', 'error', 'log', 'debug', 'trace'].map(k => [k, () => {}])),
  URL, URLSearchParams, TextEncoder, setTimeout, clearTimeout,
  navigator: { userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36', webdriver: false },
  location: { href: 'https://shop.example.org/', hostname: 'shop.example.org' },
  addEventListener(name, callback) { (listeners[name] ??= []).push(callback); },
};
context.window = context;
context.document = {
  location: context.location,
  currentScript: null,
  createElement() { return {}; },
  head: { appendChild(script) { script.onload(); } },
  addEventListener() {},
};
vm.createContext(context);
vm.runInContext(bundle.toString(), context);
const sdk = context.GrafanaFaroWebSdk;
assert.equal(sdk.VERSION, '2.11.0');
let initialized = false;
sdk.initializeFaro = cfg => {
  initialized = true;
  assert.equal(cfg.instrumentations.some(i => i instanceof sdk.ConsoleInstrumentation), enabled);
  assert.equal(cfg.instrumentations.some(i => i instanceof sdk.ErrorsInstrumentation), true);
  if (enabled) assert.equal(cfg.consoleInstrumentation.consoleErrorAsLog, true);
  for (const i of cfg.instrumentations) {
    if (!(i instanceof sdk.ConsoleInstrumentation || i instanceof sdk.ErrorsInstrumentation)) continue;
    i.config = cfg;
    i.api = {
      pushLog(args, options) { logs.push({ message: args.join(' '), ...options }); },
      pushError(error, options) {
        exceptions.push({ value: error.message, type: options?.type || error.name,
          stacktrace: { frames: options?.stackFrames || [] } });
      },
    };
    i.initialize();
  }
};
vm.runInContext(bootstrap, context);
assert.equal(initialized, true);
vm.runInContext(`
  console.info('info message'); console.warn('warn message');
  console.error('string error');
  const error = new TypeError('object error');
  error.stack = 'TypeError: object error\\n    at render (https://shop.example.org/app.js:12:3)\\n    at boot (https://shop.example.org/main.js:22:4)';
  console.error(error);
  console.debug('ignored debug'); console.trace('ignored trace'); console.log('ignored log');
  onerror('Uncaught Error: real exception', location.href, 1, 2, new Error('real exception'));
`, context);
const rejection = vm.runInContext(`({reason: new Error('real rejection')})`, context);
for (const listener of listeners.unhandledrejection) listener(rejection);
assert.equal(exceptions.length, 2);
assert.equal(exceptions[0].value, 'real exception');
assert.equal(exceptions[1].value, 'real rejection');
assert.equal(logs.length, enabled ? 4 : 0);
if (enabled) {
  assert.deepEqual(logs.map(l => l.level), ['info', 'warn', 'error', 'error']);
  assert.equal(logs[2].context.type, '');
  assert.equal(logs[2].context.stackFrames, '');
  assert.equal(logs[3].context.type, 'TypeError');
  assert.match(logs[3].context.stackFrames, /app\.js/);
}
process.stdout.write(JSON.stringify({logs, exceptions}));
