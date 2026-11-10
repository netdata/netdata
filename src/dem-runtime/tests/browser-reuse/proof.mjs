// SPDX-License-Identifier: GPL-3.0-or-later
// Real Linux proof, run under a runtime use and nd-run; never against live sites.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {pathToFileURL, fileURLToPath} from 'node:url';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {createHash} from 'node:crypto';
import http from 'node:http';
import {manageBrowser} from '../../runner/managed-browser.mjs';

const [entry, work] = process.argv.slice(2);
assert(path.isAbsolute(entry) && path.isAbsolute(work));
assert.notEqual(process.getuid(), 0);
const {runtimePaths} = await import(pathToFileURL(path.join(path.dirname(entry), 'runtime.mjs')));
const runtime = await runtimePaths();
const api = await import(pathToFileURL(path.join(runtime.modules, 'playwright/index.mjs')));
const helper = pathToFileURL(path.resolve(import.meta.dirname, '../../runner/managed-browser.mjs')).href;
const emit = value => process.stdout.write(JSON.stringify(value) + '\n');
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const launchOptions = {executablePath: runtime.browser, chromiumSandbox: true};
const home = path.join(work, 'home');
await fs.mkdir(home, {recursive: true});
const env = {PATH: path.dirname(process.execPath) + ':/usr/bin:/bin', HOME: home, TMPDIR: home, LANG: 'C.UTF-8'};
let owner;
let retirements = 0;
let passes = 0;
const fixture = http.createServer((req, res) => {
  res.setHeader('Cache-Control', 'no-store');
  if (req.url === '/sw.js') {
    res.setHeader('Content-Type', 'text/javascript');
    res.end("self.addEventListener('install',()=>self.skipWaiting());self.addEventListener('activate',e=>e.waitUntil(clients.claim()));");
  } else if (req.url === '/download') {
    res.setHeader('Content-Disposition', 'attachment; filename=proof.txt');
    res.end('download evidence');
  } else {res.setHeader('Content-Type','text/html');res.end('<title>reuse proof</title><a href="/download">Download</a>');}
});
fixture.listen(0,'127.0.0.1');
await once(fixture,'listening');
const origin = 'http://127.0.0.1:' + fixture.address().port;
async function start() {
  owner = await api.chromium.launchServer({...launchOptions, headless: true, host: '127.0.0.1', env});
}
async function retire() {
  if (!owner) return;
  const process = owner.process();
  const closed = process.exitCode !== null || process.signalCode !== null ? Promise.resolve() : once(process, 'close');
  await owner.close();
  await closed;
  assert(process.exitCode !== null || process.signalCode !== null);
  owner = undefined;
  retirements++;
}
async function check(name, fn, dirty = false) {
  const started = performance.now();
  const browser = await api.chromium.connect(owner.wsEndpoint());
  try { await fn(browser); }
  finally { await browser.close(); }
  if (dirty) { await retire(); await start(); }
  emit({type: 'case', name, elapsed_ms: performance.now() - started, retired: dirty});
  passes++;
}

emit({type: 'metadata', runtime: runtime.metadata, node: process.version,
  guard_sha256: hash(await fs.readFile(new URL(helper))), proof_sha256: hash(await fs.readFile(new URL(import.meta.url)))});
try {
  await start();
  await check('unclosed context is acknowledged and removed', async browser => {
    const guard = manageBrowser(browser);
    const context = await browser.newContext();
    const page = await context.newPage();
    await page.goto('data:text/html,<title>reuse</title>');
    await guard.finish();
    assert.equal(browser.contexts().length, 0);
  });
  await check('unawaited newPage is joined during cleanup',async browser=>{
    const guard=manageBrowser(browser);
    const page=browser.newPage();
    await guard.finish();
    await page;
    assert.equal(browser.contexts().length,0);
  });
  await check('delayed creation blocks cleanup', async browser => {
    const release = Promise.withResolvers();
    const entered = Promise.withResolvers();
    const create = browser.newContext.bind(browser);
    browser.newContext = async (...args) => {
      const context = await create(...args);
      entered.resolve();
      await release.promise;
      return context;
    };
    const guard = manageBrowser(browser);
    const pending = browser.newContext();
    await entered.promise;
    let finished = false;
    const done = guard.finish().then(() => { finished = true; });
    await new Promise(resolve => setTimeout(resolve, 30));
    assert.equal(finished, false);
    release.resolve();
    await pending;
    await done;
    assert.equal(browser.contexts().length, 0);
  });
  await check('original close remains joined after context removal', async browser => {
    const context = await browser.newContext();
    const close = context.close.bind(context);
    const release = Promise.withResolvers();
    const removed = Promise.withResolvers();
    let calls = 0;
    context.close = async () => { calls++; await close(); removed.resolve(); await release.promise; };
    const guard = manageBrowser(browser);
    const original = context.close();
    assert.equal(context.close(), original);
    await removed.promise;
    assert.equal(browser.contexts().length, 0);
    let finished = false;
    const done = guard.finish().then(() => { finished = true; });
    await new Promise(resolve => setTimeout(resolve, 30));
    assert.equal(finished, false);
    release.resolve();
    await done;
    assert.equal(calls, 1);
  });
  await check('caught close failure forces retirement', async browser => {
    const context = await browser.newContext();
    const close = context.close.bind(context);
    context.close = async () => { await close(); throw new Error('controlled post-close failure'); };
    const guard = manageBrowser(browser);
    await context.close().catch(() => {});
    await assert.rejects(guard.finish(), /context cleanup failed/);
  }, true);
  await check('caught actual creation rejection forces retirement', async browser => {
    const guard = manageBrowser(browser);
    await assert.rejects(browser.newContext({viewport: {width: 'invalid', height: 100}}));
    await assert.rejects(guard.finish(), /browser creation failed/);
  }, true);
  await check('early context event is guarded', async browser => {
    const guard = manageBrowser(browser);
    let checked;
    browser.prependListener('context', context => {
      checked = assert.rejects(context.newCDPSession(), /unavailable/);
    });
    await browser.newContext();
    assert(checked, 'real Playwright delivered a context event');
    await checked;
    await assert.rejects(guard.finish(), /unavailable/);
  }, true);
  await check('late newPage cannot create resources', async browser => {
    const guard = manageBrowser(browser);
    await guard.finish();
    await assert.rejects(browser.newPage(), /after cleanup/);
    assert.equal(browser.contexts().length, 0);
  });
  await check('HAR flush failure remains fatal after context removal', async browser => {
    const blocked = path.join(work,'not-a-directory');
    await fs.writeFile(blocked,'file');
    const guard = manageBrowser(browser);
    const context = await browser.newContext({recordHar:{path:path.join(blocked,'capture.har')}});
    await context.close().catch(() => {});
    await assert.rejects(guard.finish(),/context cleanup failed/);
  },true);
  await check('downloads HAR tracing and service workers drain', async browser => {
    const guard = manageBrowser(browser);
    const context = await browser.newContext({recordHar:{path:path.join(work,'capture.har')}});
    await context.tracing.start({screenshots:true,snapshots:true});
    const page = await context.newPage();
    await page.goto(origin);
    await page.evaluate(async()=>{
      localStorage.setItem('prior','dirty');
      await caches.open('prior');
      await navigator.serviceWorker.register('/sw.js');
      await navigator.serviceWorker.ready;
    });
    await context.addCookies([{name:'prior',value:'dirty',url:origin}]);
    const downloadEvent = page.waitForEvent('download');
    await page.getByText('Download').click();
    const download = await downloadEvent;
    await download.saveAs(path.join(work,'download.txt'));
    assert.equal(await fs.readFile(path.join(work,'download.txt'),'utf8'),'download evidence');
    await context.tracing.stop({path:path.join(work,'trace.zip')});
    await page.screenshot({path:path.join(work,'page.png')});
    await guard.finish();
    assert.equal(browser.contexts().length,0);
    assert.equal(JSON.parse(await fs.readFile(path.join(work,'capture.har'),'utf8')).log.version,'1.2');
    assert((await fs.stat(path.join(work,'trace.zip'))).size>0);
    assert((await fs.readFile(path.join(work,'page.png'))).subarray(1,4).equals(Buffer.from('PNG')));
  });
  await check('next connection has no preceding storage or workers',async browser=>{
    const guard=manageBrowser(browser);
    const context=await browser.newContext();const page=await context.newPage();await page.goto(origin);
    assert.deepEqual(await context.cookies(),[]);
    assert.deepEqual(await page.evaluate(async()=>({storage:localStorage.getItem('prior'),caches:await caches.keys(),workers:(await navigator.serviceWorker.getRegistrations()).length})),{storage:null,caches:[],workers:0});
    await guard.finish();
  });
  await check('dead browser cannot acknowledge cleanup',async browser=>{
    const guard=manageBrowser(browser);await browser.newContext();
    const disconnected=once(browser,'disconnected');
    owner.process().kill('SIGKILL');
    await disconnected;
    await assert.rejects(guard.finish(),/disconnected|cleanup failed/);
  },true);
  await check('disconnect during acknowledgment forces retirement',async browser=>{
    const guard=manageBrowser(browser);
    await assert.rejects(guard.finish(async()=>{await browser.close();}),/disconnected/);
  },true);

  // Exercise the real Test CLI and workers, not just direct library calls.
  for (const [name, source, expectedCode, expectedClean, disposition='claimed'] of [
    ['fixtures', `import {test,expect} from '@playwright/test';
      import {test as second} from 'playwright/test';
      test.beforeEach(async ({page})=>{await page.goto('data:text/html,<title>fixture</title>');});
      const extended=test.extend({answer:async({},use)=>use(42)});
      extended('isolated',async({page,answer,browser})=>{expect(test).toBe(second);expect(answer).toBe(42);await expect(page).toHaveTitle('fixture');const c=await browser.newContext();await c.newPage();});`, 0, true],
    ['restart', `import {test,expect} from '@playwright/test';
      test('failure',async({page},info)=>{expect(info.workerIndex).toBe(0);expect(1).toBe(2)});
      test('new worker',async({page,browser},info)=>{expect(info.workerIndex).toBeGreaterThan(0);const cdp=await browser.newBrowserCDPSession();await cdp.detach();});`, 1, true],
    ['timeout', `import {test} from '@playwright/test';test('timeout',async({page})=>{test.setTimeout(100);await new Promise(()=>{});});`, 1, true],
    ['fresh-options',`import {test} from '@playwright/test';test.use({launchOptions:${JSON.stringify({...launchOptions,args:['--disable-extensions']})}});test('fresh',async({browser})=>{const cdp=await browser.newBrowserCDPSession();await cdp.detach();});`,0,false,'unused'],
    ['caught-CDP',`import {test} from '@playwright/test';test('caught',async({page,context})=>{try{await context.newCDPSession(page);}catch{}});`,1,false,'failed'],
    ['caught-selector',`import {test,selectors} from '@playwright/test';test('caught',async({page})=>{try{await selectors.register('custom',{create:()=>({query:()=>null,queryAll:()=>[]})});}catch{}});`,1,false,'failed'],
    ['killed-CLI',`import {test} from '@playwright/test';test('waiting',async({page})=>{await page.goto('data:text/html,waiting');await new Promise(()=>{});});`,null,false,'killed'],
  ]) {
    const started = performance.now();
    const dir = path.join(work, name);
    await fs.mkdir(dir);
    const paths = Object.fromEntries(['claim','clean','failed'].map(k => [k,path.join(dir,k+'.json')]));
    const shim = path.join(dir, 'test-api.mjs');
    const real = path.join(runtime.modules, '@playwright/test/index.mjs');
    await fs.writeFile(shim, `import * as api from ${JSON.stringify(real)};
      import fs from 'node:fs/promises';
      import {managedTest} from ${JSON.stringify(fileURLToPath(helper))};
      export * from ${JSON.stringify(real)};
      export const test=managedTest(api,{endpoint:${JSON.stringify(owner.wsEndpoint())},launchOptions:${JSON.stringify(launchOptions)},
        claim:()=>fs.writeFile(${JSON.stringify(paths.claim)},'{}',{flag:'wx'}),
        cleaned:()=>fs.writeFile(${JSON.stringify(paths.clean)},'{}',{flag:'wx'}),
        failed:()=>fs.writeFile(${JSON.stringify(paths.failed)},'{}',{flag:'wx'})});
      export default test;`, {mode:0o600});
    await fs.writeFile(path.join(dir,'journey.spec.ts'), source);
    await fs.writeFile(path.join(dir,'tsconfig.json'), JSON.stringify({compilerOptions:{baseUrl:dir,paths:{'@playwright/test':[shim],'playwright/test':[shim]}}}));
    await fs.writeFile(path.join(dir,'playwright.config.cjs'), 'module.exports='+JSON.stringify({testDir:dir,outputDir:path.join(dir,'output'),workers:1,retries:0,repeatEach:1,fullyParallel:false,timeout:10000,globalTimeout:20000,reporter:[['json',{outputFile:path.join(dir,'report.json')}]],use:{browserName:'chromium',headless:true,launchOptions}}));
    const child = spawn(process.execPath,[path.join(runtime.modules,'playwright/cli.js'),'test','--config',path.join(dir,'playwright.config.cjs'),'--tsconfig',path.join(dir,'tsconfig.json')],{cwd:dir,env:{...env,HOME:dir,TMPDIR:dir,CI:'1'},stdio:['ignore','pipe','pipe']});
    const closed = once(child,'close');
    let diagnostic='';
    child.stdout.on('data',b=>{diagnostic+=b;});child.stderr.on('data',b=>{diagnostic+=b;});
    if(disposition==='killed') {
      const deadline=Date.now()+10000;
      while(true){try{await fs.access(paths.claim);break;}catch{assert(Date.now()<deadline,'worker did not claim');await new Promise(r=>setTimeout(r,10));}}
      child.kill('SIGKILL');
    }
    const [code] = await closed;
    await fs.writeFile(path.join(dir,'cli.log'),diagnostic);
    assert.equal(code,expectedCode,`CLI ${name}: ${diagnostic.slice(-1000)}`);
    if(disposition==='unused') await assert.rejects(fs.access(paths.claim));
    else await fs.access(paths.claim);
    if(expectedClean) await fs.access(paths.clean);
    else await assert.rejects(fs.access(paths.clean));
    if(disposition==='failed') await fs.access(paths.failed);
    else await assert.rejects(fs.access(paths.failed));
    let report;
    if(disposition!=='killed') {
      report = JSON.parse(await fs.readFile(path.join(dir,'report.json'),'utf8'));
      if(disposition!=='failed') assert.equal(report.errors.length,0,JSON.stringify(report.errors));
    }
    emit({type:'case',name:'Test CLI '+name,elapsed_ms:performance.now()-started,stats:report?.stats});passes++;
    if(name==='timeout'||disposition==='failed'||disposition==='killed') {await retire();await start();}
  }
  await retire();
  emit({type:'complete',passes,retirements});
} finally {
  await retire();
  await new Promise(resolve=>fixture.close(resolve));
}
