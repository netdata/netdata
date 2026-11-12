// SPDX-License-Identifier: GPL-3.0-or-later
import test from 'node:test';
import assert from 'node:assert/strict';
import {EventEmitter} from 'node:events';
import {manageBrowser} from '../runner/managed-browser.mjs';

const deferred = () => Promise.withResolvers();
function fixture() {
  const browser = new EventEmitter();
  const live = new Set();
  let connected = true;
  browser.contexts = () => [...live];
  browser.isConnected = () => connected;
  browser.newContext = async () => {
    const context = {browser: () => browser, close: async () => live.delete(context), newCDPSession: async () => 'unsafe'};
    live.add(context);
    browser.emit('context', context);
    return context;
  };
  browser.newPage = async () => { const context = await browser.newContext(); return {context: () => context}; };
  browser.newBrowserCDPSession = async () => 'unsafe';
  browser.disconnect = () => { connected = false; browser.emit('disconnected'); };
  return {browser, live};
}

test('cleanup joins delayed creation and closes the eventual context', async () => {
  const {browser, live} = fixture();
  const release = deferred();
  const create = browser.newContext;
  browser.newContext = async () => { await release.promise; return create(); };
  const guard = manageBrowser(browser);
  const pending = browser.newContext();
  let finished = false;
  const done = guard.finish().then(() => { finished = true; });
  await new Promise(setImmediate);
  assert.equal(finished, false);
  release.resolve();
  await pending;
  await done;
  assert.equal(live.size, 0);
});

test('an accepted newPage can finish its nested context creation during cleanup', async () => {
  const {browser, live} = fixture();
  const guard = manageBrowser(browser);
  const page = browser.newPage();
  await guard.finish();
  await page;
  assert.equal(live.size, 0);
});

test('cleanup joins the original close even after the context disappears', async () => {
  const {browser, live} = fixture();
  const context = await browser.newContext();
  const release = deferred();
  let closes = 0;
  context.close = async () => { closes++; live.delete(context); await release.promise; };
  const guard = manageBrowser(browser);
  const originalClose = context.close();
  assert.equal(context.close(), originalClose);
  let finished = false;
  const done = guard.finish().then(() => { finished = true; });
  await new Promise(setImmediate);
  assert.equal(live.size, 0);
  assert.equal(finished, false);
  release.resolve();
  await done;
  assert.equal(closes, 1);
});

test('caught original close failure remains fatal to reuse', async () => {
  const {browser, live} = fixture();
  const context = await browser.newContext();
  context.close = async () => { live.delete(context); throw new Error('HAR write failed'); };
  const guard = manageBrowser(browser);
  await context.close().catch(() => {});
  await assert.rejects(guard.finish(), /context cleanup failed/);
});

test('caught creation failure remains fatal to reuse', async () => {
  const {browser} = fixture();
  browser.newContext = async () => { throw new Error('aborted'); };
  const guard = manageBrowser(browser);
  await browser.newContext().catch(() => {});
  await assert.rejects(guard.finish(), /browser creation failed/);
});

test('context is guarded before even a prepended context listener', async () => {
  const {browser} = fixture();
  const guard = manageBrowser(browser);
  let assertion;
  browser.prependListener('context', context => {
    assertion = assert.rejects(context.newCDPSession(), /unavailable/);
  });
  await browser.newContext();
  await assertion;
  await assert.rejects(guard.finish(), /unavailable/);
});

test('late creation is refused without making a context', async () => {
  const {browser, live} = fixture();
  const guard = manageBrowser(browser);
  await guard.finish();
  await assert.rejects(browser.newContext(), /after cleanup/);
  assert.equal(live.size, 0);
});

test('disconnect is only expected after acknowledged cleanup', async () => {
  const first = fixture();
  const dirty = manageBrowser(first.browser);
  first.browser.disconnect();
  await assert.rejects(dirty.finish(), /disconnected/);
  const second = fixture();
  const clean = manageBrowser(second.browser);
  await clean.finish();
  second.browser.disconnect();
  assert.equal(clean.error, undefined);
});

test('disconnect during acknowledgment write prevents reuse', async () => {
  const {browser} = fixture();
  const guard = manageBrowser(browser);
  const writing = deferred();
  const release = deferred();
  const done = guard.finish(async () => { writing.resolve(); await release.promise; });
  await writing.promise;
  browser.disconnect();
  release.resolve();
  await assert.rejects(done, /disconnected before cleanup/);
});

test('unawaited browser close is sticky before disconnected notification', async () => {
  const {browser} = fixture();
  browser.close = async () => {};
  const guard = manageBrowser(browser);
  void browser.close();
  await assert.rejects(guard.finish(), /closed before acknowledged cleanup/);
});
