// SPDX-License-Identifier: GPL-3.0-or-later

export class ManagedBrowserError extends Error {
  constructor(reason) {
    super(`DEM browser reuse: ${reason}; use browser_mode: fresh for direct browser control`);
    this.name = 'ManagedBrowserError';
  }
}

// This guards a single connected worker. A clean result is only one part of the
// reuse proof: the Agent must also join the entire attempt's supervised tree.
export function manageBrowser(browser, onError = () => {}) {
  let sealed = false;
  let clean = false;
  let failure;
  let finishing;
  const creations = new Set();
  const contexts = new Map();
  const pages = new WeakSet();
  const localArtifacts = new WeakSet();
  const originalContexts = browser.contexts.bind(browser);

  const fail = reason => {
    if (!failure) {
      failure = new ManagedBrowserError(reason);
      onError(failure);
    }
    return failure;
  };
  const deny = (object, name) => {
    if (typeof object[name] === 'function') {
      object[name] = async () => { throw fail(`${name} is unavailable with a reused browser`); };
    }
  };
  function guardArtifact(artifact, kind) {
    if (!artifact || localArtifacts.has(artifact)) return artifact;
    localArtifacts.add(artifact);
    artifact.path = async () => { throw fail(`${kind}.path requires a fresh browser; use saveAs to save a copy`); };
    return artifact;
  }
  function guardPage(page) {
    if (pages.has(page)) return page;
    pages.add(page);
    if (typeof page.emit === 'function') {
      const emit = page.emit.bind(page);
      page.emit = (name, ...args) => {
        if (name === 'download') guardArtifact(args[0], 'download');
        return emit(name, ...args);
      };
    }
    if (typeof page.video === 'function') {
      const video = page.video.bind(page);
      page.video = () => guardArtifact(video(), 'video');
    }
    return page;
  }
  function guardContext(context) {
    if (contexts.has(context)) return context;
    const state = {close: context.close.bind(context), closing: undefined};
    contexts.set(context, state);
    context.close = (...args) => {
      // Joining a second Playwright close call is insufficient: it may return
      // while the first call is still flushing HAR/video or reporting failure.
      if (!state.closing) {
        state.closing = Promise.resolve().then(() => state.close(...args));
        state.closing.catch(() => fail('context cleanup failed'));
      }
      return state.closing;
    };
    if (typeof context.emit === 'function') {
      const emit = context.emit.bind(context);
      context.emit = (name, ...args) => {
        if (name === 'page') guardPage(args[0]);
        return emit(name, ...args);
      };
    }
    if (typeof context.pages === 'function') {
      const allPages = context.pages.bind(context);
      context.pages = () => allPages().map(guardPage);
      context.pages();
    }
    if (typeof context.newPage === 'function') {
      const newPage = context.newPage.bind(context);
      context.newPage = (...args) => newPage(...args).then(guardPage);
    }
    deny(context, 'newCDPSession');
    return context;
  }
  function creation(name, guard) {
    const original = browser[name].bind(browser);
    browser[name] = (...args) => {
      if (sealed) return Promise.reject(fail('browser creation after cleanup began'));
      let pending;
      try { pending = Promise.resolve(original(...args)).then(guard); }
      catch (error) { fail('browser creation failed'); return Promise.reject(error); }
      creations.add(pending);
      pending.then(() => creations.delete(pending), () => {
        creations.delete(pending);
        fail('browser creation failed');
      });
      return pending;
    };
  }

  // Guard before delivery, including prependListener and listeners added after
  // removeAllListeners. A regular context listener cannot guarantee that order.
  const emit = browser.emit.bind(browser);
  browser.emit = (name, ...args) => {
    if (name === 'context') guardContext(args[0]);
    if (name === 'disconnected' && !clean) fail('browser disconnected before cleanup');
    return emit(name, ...args);
  };
  browser.contexts = () => originalContexts().map(guardContext);
  browser.contexts();
  if (typeof browser.close === 'function') {
    const close = browser.close.bind(browser);
    browser.close = (...args) => {
      if (!clean) fail('browser closed before acknowledged cleanup');
      return close(...args);
    };
  }
  creation('newContext', guardContext);
  creation('newPage', page => { guardContext(page.context()); return guardPage(page); });
  for (const name of ['newBrowserCDPSession', 'startTracing', 'stopTracing', 'bind', 'unbind']) deny(browser, name);

  return {
    get error() { return failure; },
    finish(acknowledge = async () => {}) {
      if (finishing) return finishing;
      sealed = true;
      finishing = (async () => {
        await Promise.allSettled([...creations]);
        for (const context of originalContexts()) guardContext(context);
        await Promise.allSettled([...contexts.keys()].map(context => context.close()));
        if (originalContexts().length !== 0) fail('browser contexts remain after cleanup');
        if (!browser.isConnected()) fail('browser disconnected before cleanup');
        if (failure) throw failure;
        await acknowledge();
        if (failure) throw failure;
        clean = true;
      })();
      return finishing;
    },
  };
}

// The caller supplies an authenticated private loan and records its disposition.
// Only worker zero can borrow it; Test worker restarts get fresh browsers.
export function managedTest(api, {endpoint, launchOptions, claim, cleaned, failed}) {
  let borrowed = false;
  let policyFailure;
  let failureReported;
  const reportFailure = () => failureReported ??= Promise.resolve(failed());
  api.selectors.register = async () => {
    policyFailure ??= new ManagedBrowserError('custom selector registration is unavailable with a reused browser');
    await reportFailure();
    throw policyFailure;
  };
  const test = api.test.extend({
    _demReusePolicy: [async ({}, use) => {
      await use();
      if (policyFailure) { await reportFailure(); throw policyFailure; }
    }, {scope: 'worker', auto: true}],
    connectOptions: [async ({browserName, headless, channel, reuseContext, launchOptions: effective}, use, workerInfo) => {
      const keys = Object.keys(effective);
      const compatible = browserName === 'chromium' && headless && !channel &&
        keys.length === Object.keys(launchOptions).length &&
        keys.every(key => effective[key] === launchOptions[key]);
      if (workerInfo.workerIndex !== 0 || reuseContext || !compatible) return use(undefined);
      await claim();
      borrowed = true;
      await use({wsEndpoint: endpoint, timeout: 5000});
    }, {scope: 'worker'}],
    browser: [async ({browser}, use) => {
      if (!borrowed) return use(browser);
      const guard = manageBrowser(browser, error => { policyFailure ??= error; });
      try {
        if (browser.contexts().length !== 0) throw new ManagedBrowserError('browser already contains contexts before this attempt');
        await use(browser);
        await guard.finish(async () => {
          if (policyFailure) throw policyFailure;
          await cleaned();
        });
      } catch (error) {
        await reportFailure();
        throw error;
      }
    }, {scope: 'worker'}],
  });
  return test;
}
