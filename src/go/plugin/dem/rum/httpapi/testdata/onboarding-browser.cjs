// SPDX-License-Identifier: GPL-3.0-or-later
// Invoked by TestOnboardingBrowser. Uses an existing browser and puppeteer-core;
// never installs packages or downloads a browser.
const assert = require('node:assert/strict');
const {pathToFileURL} = require('node:url');
const options = JSON.parse(process.argv[2]);

async function until(description, observe, matches) {
  const deadline = Date.now() + 15000;
  let value;
  do {
    value = await observe();
    if (matches(value)) return value;
    await new Promise(resolve => setTimeout(resolve, 50));
  } while (Date.now() < deadline);
  assert.fail(`${description}: ${JSON.stringify(value)}`);
}

async function nativeSite(key) {
  const response = await fetch(options.receiver + '/function');
  assert.equal(response.status, 200);
  const result = await response.json();
  // Resolve the actual native Function schema, never presumed column positions.
  const rows = result.data.map(row => Object.fromEntries(
    Object.entries(result.columns).map(([key, column]) => [key, row[column.index]])
  ));
  const site = rows.find(row => row.site === key);
  assert.ok(site, `missing native site ${key}`);
  for (const obsolete of ['status', 'reachable', 'reach_detail', 'snippet_check', 'snippet_detail', 'beacon_url']) {
    assert.ok(!(obsolete in site), `obsolete installation claim: ${obsolete}`);
  }
  return site;
}

async function requestsFor(key) {
  const response = await fetch(options.receiver + '/requests');
  assert.equal(response.status, 200);
  return (await response.json() || []).filter(r => r.path.includes('/' + key + '/collect'));
}

(async () => {
  const module = await import(pathToFileURL(process.env.DEM_PUPPETEER_MODULE).href);
  const puppeteer = module.default || module;
  const browser = await puppeteer.launch({
    executablePath: process.env.DEM_BROWSER_EXECUTABLE,
    headless: true,
    args: ['--disable-background-networking', '--no-first-run'],
  });
  console.log(JSON.stringify({browser: await browser.version(), pid: browser.process().pid}));
  try {
    for (const key of options.cases) {
      const before = await nativeSite(key);
      const canonical = options.receiver + '/proxy/rum/' + key + '.js';
      assert.ok(before.generation);
      assert.equal(before.script_url, canonical);
      assert.equal(before.snippet, `<script async src="${canonical}"></script>`);
      assert.equal(before.collection_enabled, key === 'disabled' ? 0 : 1);
      assert.equal(before.last_beacon_at, null);
      assert.equal(before.last_beacon_age_s, null);
      assert.equal(before.last_rejected_at, null);
      assert.equal(before.last_rejected_age_s, null);
      assert.equal(before.beacons_per_min, 0);
      const context = await browser.createBrowserContext();
      try {
        const page = await context.newPage();
        page.setDefaultTimeout(15000);
        page.setDefaultNavigationTimeout(15000);
        const requests = [], responses = [], warnings = [], errors = [], blocked = [];
        await page.setRequestInterception(true);
        page.on('request', request => {
          requests.push({url: request.url(), method: request.method()});
          const url = new URL(request.url());
          // Only designated failure cases intercept SDK requests. Positive cases
          // execute the exact production embedded assets through the receiver.
          if ((key === 'core-failure' && url.pathname.includes('/assets/faro-web-sdk-')) ||
              (key === 'tracing-failure' && url.pathname.includes('/assets/faro-web-tracing-'))) {
            void request.abort('failed');
          } else if ([options.website, options.receiver].includes(url.origin)) {
            void request.continue();
          } else {
            blocked.push(request.url());
            void request.abort('blockedbyclient');
          }
        });
        page.on('response', response => responses.push({url: response.url(), status: response.status()}));
        page.on('console', message => { if (message.type() === 'warn') warnings.push(message.text()); });
        page.on('pageerror', error => errors.push(error.message));
        await page.goto(options.website + '/' + key, {waitUntil: 'load'});
        if (key === 'clobbered-script') {
          assert.equal(await page.evaluate(() => document.currentScript.tagName), 'IMG', 'fixture must shadow the document property');
          assert.equal(await page.evaluate(() => window.unexpectedSDKExecuted === true), false, 'named HTML must not select executable SDK content');
          assert.ok(!requests.some(r => r.url.includes('/injected/rum/assets/')), 'named HTML must not redirect SDK requests');
        }
        if (key === 'isolated') assert.equal(await page.evaluate(() => crossOriginIsolated), true);
        const noReceipt = ['core-failure', 'disabled', 'wrong-origin', 'alias-bootstrap-only'].includes(key);
        if (!noReceipt) {
          await until(key + ' accepted POST', () => requestsFor(key), rows => rows.some(r => r.method === 'POST' && r.status === 202));
          const after = await nativeSite(key);
          assert.equal(after.generation, before.generation);
          assert.equal(typeof after.last_beacon_at, 'number');
          assert.ok(Math.abs(after.last_beacon_at / 1000 - Date.now()) < 30000, 'timestamp must use Unix microseconds');
          assert.ok(after.beacons_per_min > 0);
          assert.ok(after.observed_sessions > 0);
          assert.ok(after.pageviews_window > 0);
          assert.equal(after.last_rejected_at, null);
          const prefix = key === 'alias-coherent' ? '/coherent' : '/proxy';
          assert.ok(requests.some(r => r.url.startsWith(options.receiver + prefix + '/rum/' + key + '/collect')));
          assert.ok(responses.some(r => r.status === 200 && r.url.startsWith(options.receiver + prefix + '/rum/assets/faro-web-sdk-')));
          assert.ok(!requests.some(r => new URL(r.url).pathname.startsWith('/rum/')), 'proxy prefix lost');
          const sdkScripts = await page.evaluate(() => Array.from(document.scripts)
            .filter(s => s.src.includes('/assets/')).map(s => ({src: s.src, nonce: s.nonce})));
          if (['nonce-csp', 'strict-dynamic', 'dynamic', 'clobbered-script'].includes(key)) {
            assert.ok(sdkScripts.length > 0);
            assert.ok(sdkScripts.every(s => s.nonce === 'fixture-nonce'));
          }
          if (key === 'tracing-failure') {
            assert.equal(warnings.filter(w => w.includes('Netdata RUM: cannot load the tracing SDK')).length, 1);
          } else {
            assert.deepEqual(warnings.filter(w => w.startsWith('Netdata RUM:')), []);
          }
        } else if (key === 'wrong-origin') {
          const traffic = await until('rejected browser preflight', () => requestsFor(key), rows => rows.some(r => r.method === 'OPTIONS' && r.status === 403));
          assert.ok(!traffic.some(r => r.method === 'POST'), 'browser must not send payload after failed preflight');
          const after = await nativeSite(key);
          assert.equal(after.last_rejected_origin, options.website);
          assert.equal(typeof after.last_rejected_at, 'number');
          assert.ok(Math.abs(after.last_rejected_at / 1000 - Date.now()) < 30000);
          assert.ok(after.rejected_per_min > 0);
        } else if (key === 'disabled') {
          // Load completion includes parser and dynamically inserted async scripts.
          assert.ok(!requests.some(r => r.url.includes('/assets/')));
          assert.equal(await page.evaluate(() => typeof window.GrafanaFaroWebSdk), 'undefined');
          assert.deepEqual(await requestsFor(key), []);
        } else {
          await until(key + ' diagnostic', async () => warnings, rows => rows.some(w => w.includes('Netdata RUM: cannot load the core SDK')));
          assert.equal(warnings.filter(w => w.includes('Netdata RUM: cannot load the core SDK')).length, 1);
          assert.deepEqual(await requestsFor(key), []);
          if (key === 'alias-bootstrap-only') {
            assert.ok(responses.some(r => r.status === 302 && r.url.endsWith('/bootstrap-only/rum/' + key + '.js')));
            assert.ok(responses.some(r => r.status === 200 && r.url.endsWith('/proxy/rum/' + key + '.js')));
            assert.ok(requests.some(r => r.url.includes('/bootstrap-only/rum/assets/')),
              'SDK URL must follow original element src, not the redirect destination');
            // CORB can suppress the browser response event for a plain-text 404.
            // Verify the actual receiver request instead of depending on that event.
            const received = await (await fetch(options.receiver + '/requests')).json();
            assert.ok(received.some(r => r.status === 404 && r.path.includes('/bootstrap-only/rum/assets/')));
          }
        }
        if (noReceipt) {
          const after = await nativeSite(key);
          assert.equal(after.generation, before.generation);
          assert.equal(after.last_beacon_at, null);
          assert.equal(after.beacons_per_min, 0);
        }
        assert.deepEqual(errors, [], 'uncaught page exceptions');
        assert.deepEqual(blocked, [], 'external/CDN network request attempted');
        console.log(JSON.stringify({case: key, result: 'passed', native: await nativeSite(key), warnings}));
      } finally {
        await context.close();
      }
    }
  } finally {
    const pid = browser.process().pid;
    await browser.close();
    console.log(JSON.stringify({closedBrowserPID: pid, connected: browser.connected}));
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
