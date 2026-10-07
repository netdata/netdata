// SPDX-License-Identifier: GPL-3.0-or-later
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const bundle = fs.readFileSync(require('node:path').join(__dirname, '../assets/faro-web-sdk.iife.js'), 'utf8');
const scripts = JSON.parse(fs.readFileSync(0, 'utf8'));
const source = 'https://receiver.example/prefix/rum/proxy/rum/shop.js?cache=1#fragment';
function run(options = {}) {
  const warnings = [], loads = [], configs = [];
  const script = options.missing ? null : {
    src: options.src ?? source, nonce: 'fixture-nonce', getAttribute() { return ''; },
  };
  const c = {
    URL, URLSearchParams, TextEncoder,
    console: { warn: value => warnings.push(value) },
    location: { href: 'https://site.example/nested/page', hostname: 'site.example' },
    navigator: { userAgent: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36', webdriver: false },
    addEventListener() {},
  };
  c.window = c;
  c.document = {
    location: c.location, currentScript: script,
    createElement() {
      if (options.throwAt === 'create') throw new Error('private create detail');
      return options.throwAt === 'assign' ? { set src(_) { throw new Error('private assignment detail'); } } : {};
    },
    head: { appendChild(s) {
      if (options.throwAt === 'append') throw new Error('private insertion detail');
      loads.push({src:s.src, nonce:s.nonce});
      if (options.failCore || (options.failTracing && s.src.includes('faro-web-tracing'))) { s.onerror(); return; }
      if (s.src.includes('faro-web-sdk')) {
        vm.runInContext(bundle, c);
        c.GrafanaFaroWebSdk.initializeFaro = cfg => {
          if (options.failInit) throw new Error('private initialization detail');
          configs.push(cfg);
          return { api: { pushEvent() {} } };
        };
      }
      s.onload();
    } },
  };
  require('./script-context.cjs')(c);
  if (options.clobbered) c.document.currentScript = { src: 'https://untrusted.example/rum/shop.js', getAttribute() { return ''; } };
  if (options.missingNativeGetter) c.Document = class Document {};
  vm.createContext(c);
  assert.doesNotThrow(() => vm.runInContext(scripts[options.variant || 'core'], c));
  assert.equal(typeof c.netdataRum.setUser, 'function');
  assert.ok(warnings.every(message => message.startsWith('Netdata RUM: ') && !message.includes('private')));
  return { warnings, loads, configs };
}
const normal = run();
assert.equal(run({clobbered:true}).loads[0].src, normal.loads[0].src);
assert.equal(normal.loads.length, 1);
assert.equal(normal.loads[0].nonce, 'fixture-nonce');
assert.ok(normal.loads[0].src.startsWith('https://receiver.example/prefix/rum/proxy/rum/assets/faro-web-sdk-2.11.0-'));
assert.equal(normal.configs[0].transports[0].options.url, 'https://receiver.example/prefix/rum/proxy/rum/shop/collect');
assert.equal(normal.warnings.length, 0);
for (const options of [{missing:true}, {missingNativeGetter:true}, {src:''}, {src:'data:text/javascript,void 0'}, {src:'https://secret:private@receiver.example/rum/shop.js'}]) {
  const r = run(options); assert.equal(r.loads.length, 0); assert.equal(r.warnings.length, 1);
}
for (const throwAt of ['create','assign','append']) {
  const r = run({throwAt}); assert.equal(r.configs.length, 0); assert.equal(r.warnings.length, 1);
}
let r = run({failCore:true}); assert.equal(r.configs.length, 0); assert.equal(r.warnings.length, 1);
r = run({failInit:true}); assert.equal(r.configs.length, 0); assert.equal(r.warnings.length, 1);
r = run({variant:'tracing', failTracing:true});
assert.equal(r.loads.length, 2); assert.equal(r.configs.length, 1); assert.equal(r.warnings.length, 1);
assert.equal(r.loads[1].nonce, 'fixture-nonce');
r = run({variant:'disabled', missing:true});
assert.equal(r.loads.length, 0); assert.equal(r.configs.length, 0); assert.equal(r.warnings.length, 0);
process.stdout.write('loader contracts passed\n');
