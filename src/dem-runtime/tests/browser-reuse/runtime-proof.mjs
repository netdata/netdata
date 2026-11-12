// SPDX-License-Identifier: GPL-3.0-or-later
// Execute under supervise.go using a prepared runtime. This exercises the actual
// owner/attempt wire protocol; the Go integration proof owns individual receipts.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {fileURLToPath, pathToFileURL} from 'node:url';

const [installedEntry, work] = process.argv.slice(2);
const source = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const installed = await (await import(pathToFileURL(path.join(path.dirname(installedEntry), 'runtime.mjs')))).runtimePaths();
const root = path.join(work, 'runtime');
await fs.mkdir(root, {mode: 0o700});
await fs.cp(path.join(source, 'runner'), path.join(root, 'runner'), {recursive: true});
await fs.copyFile(path.join(source, 'manifest.json'), path.join(root, 'manifest.json'));
await fs.symlink(installed.dependencies, path.join(root, 'dependencies'));
await fs.symlink(installed.browser, path.join(root, 'chrome'));
await fs.writeFile(path.join(root, 'runtime.json'), JSON.stringify({contract: 2, version: 'reuse-proof', paths: {dependencies: 'dependencies', browser: 'chrome'}}));
const entry = path.join(root, 'runner/run.mjs');
const prefix = "import {test,expect} from '@playwright/test';\n";
const download = `await page.goto('data:text/html,<a download="fixture.txt" href="data:text/plain,fixture">save</a>');
  const pending=page.waitForEvent('download');await page.locator('a').click();const download=await pending;`;
let serial = 0;

function launch(args) {
  const child = spawn(process.execPath, [entry, ...args], {stdio: ['pipe', 'pipe', 'pipe']});
  let out = '', err = '';
  child.stdout.on('data', b => out += b);
  child.stderr.on('data', b => err += b);
  const ended = new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('close', (code, signal) => resolve({code, signal, out, err}));
  });
  return {child, ended};
}
async function owner() {
  const id = 'owner-' + ++serial;
  const dir = path.join(work, id);
  await fs.mkdir(dir, {mode: 0o700});
  const process = launch(['--browser-owner']);
  const lines = createInterface({input: process.child.stdout});
  const ready = Promise.race([
    new Promise(resolve => lines.once('line', line => resolve(JSON.parse(line)))),
    process.ended.then(result => { throw new Error('owner exited before readiness: ' + result.err); }),
  ]);
  process.child.stdin.write(JSON.stringify({version: 2, owner_id: id, work_dir: dir}) + '\n');
  const message = await ready;
  assert.equal(message.type, 'ready');
  assert.equal(message.owner_id, id);
  assert.ok(path.isAbsolute(message.endpoint), 'owner must use a private Unix socket');
  assert.ok(message.endpoint.startsWith(dir + path.sep));
  assert.equal((await fs.lstat(message.endpoint)).isSocket(), true);
  const files = await fs.readdir(path.join(dir, 'home'));
  assert.ok(files.some(name => name.startsWith('playwright_chromiumdev_profile-')), 'profile must be owned scratch');
  assert.ok(files.some(name => name.startsWith('playwright-artifacts-')), 'artifacts must be owned scratch');
  return {...process, id, endpoint: message.endpoint, epoch: 0,
    async stop(eof = false) {
      if (eof) process.child.stdin.end();
      else process.child.stdin.end('{"type":"stop"}\n');
      const result = await process.ended;
      assert.equal(result.code, 0, result.err);
    }};
}
async function run(name, source, browser, expected = 'success', state = 'clean', options = {}) {
  const dir = path.join(work, 'attempt-' + ++serial);
  await fs.mkdir(dir, {mode: 0o700});
  const request = {version: 2, kind: 'journey', run_id: name, work_dir: dir,
    script: prefix + source, timeout_ms: 15000, capture: false, secrets: {}, ...options};
  if (browser) request.browser = {owner_id: browser.id, epoch: String(++browser.epoch), endpoint: browser.endpoint};
  const process = launch([]);
  process.child.stdin.end(JSON.stringify(request));
  const result = await process.ended;
  await fs.writeFile(path.join(dir, 'wire.jsonl'), result.out);
  assert.equal(result.err, '', name + ': ' + result.err);
  const frames = result.out.trim().split('\n').filter(Boolean).map(line => JSON.parse(line));
  assert.equal(frames.filter(f => f.type === 'result').length, 1, name + ': ' + result.out);
  assert.equal(frames.at(-1).type, 'result', name);
  assert.equal(frames.at(-1).result.status, expected, name + ': ' + result.out);
  const disposition = frames.filter(f => f.type === 'browser');
  assert.equal(disposition.length, browser ? 1 : 0, name);
  if (browser) {
    assert.deepEqual(disposition[0].browser, {owner_id: browser.id, epoch: String(browser.epoch), state}, name + ': ' + result.out);
    assert.ok(!result.out.includes(browser.endpoint), 'owner capability leaked');
  }
  console.log(JSON.stringify({case: name, status: expected, disposition: state, passed: true}));
  return {frames, dir};
}

const reusable = await owner();
if (process.argv[4] === '--inspect') {
  await fs.writeFile(path.join(work, 'access-probe.json'), JSON.stringify({endpoint: reusable.endpoint, pid: reusable.child.pid}), {mode: 0o600});
  try {
    const deadline = Date.now() + 45000;
    for (;;) {
      try { await fs.access(path.join(work, 'access-checked')); break; }
      catch (error) { if (error.code !== 'ENOENT' || Date.now() > deadline) throw error; }
      await new Promise(resolve => setTimeout(resolve, 100));
    }
  } finally { await reusable.stop(); }
  console.log(JSON.stringify({case: 'external-access-inspection', passed: true}));
  process.exit(0);
}
if (process.argv[4] === '--soak') {
  const {chromium} = await import(pathToFileURL(path.join(installed.modules, 'playwright/index.mjs')));
  const {manageBrowser} = await import(pathToFileURL(path.join(source, 'runner/managed-browser.mjs')));
  const sample = async count => {
    const status = await fs.readFile(`/proc/${reusable.child.pid}/status`, 'utf8');
    console.log(JSON.stringify({connections: count, owner_rss_kib: Number(status.match(/^VmRSS:\s+(\d+)/m)[1]),
      owner_anon_kib: Number(status.match(/^RssAnon:\s+(\d+)/m)[1])}));
  };
  try {
    await sample(0);
    for (let i = 1; i <= 1000; i++) {
      const browser = await chromium.connect(reusable.endpoint);
      assert.equal(browser.contexts().length, 0, 'previous connection left a context');
      const guard = manageBrowser(browser);
      const context = await browser.newContext();
      const page = await context.newPage();
      await page.goto('about:blank');
      await guard.finish();
      await browser.close();
      if ([50, 100, 250, 500, 1000].includes(i)) await sample(i);
    }
  } finally { await reusable.stop(); }
  process.exit(0);
}
try {
  await run('hooks-fixtures-and-canonical-imports', `import {test as second} from 'playwright/test';
    const extended=test.extend({answer:async({},use)=>use(42)});let n=0;
    extended.beforeEach(()=>n++);extended('page',async({page,answer})=>{
    expect(test).toBe(second);expect(n).toBe(1);expect(answer).toBe(42);
    await page.goto('data:text/html,<h1>Ready</h1>');await expect(page.locator('h1')).toHaveText('Ready');});`, reusable);
  await run('new-process-secrets', `test('isolated',async({page})=>{
    expect(process.env.DEM_SECRET_TOKEN).toBe('proof-secret');
    await page.goto('about:blank');console.log(process.env.DEM_SECRET_TOKEN);});`, reusable, 'success', 'clean', {secrets: {DEM_SECRET_TOKEN: 'proof-secret'}});
  await run('next-process-has-no-secret', `test('isolated',async({page})=>{
    expect(process.env.DEM_SECRET_TOKEN).toBeUndefined();await page.goto('about:blank');});`, reusable);
  await run('remote-download-saveAs', `import fs from 'node:fs/promises';test('download',async({page},info)=>{
    ${download}const target=info.outputPath('saved.txt');await download.saveAs(target);expect(await fs.readFile(target,'utf8')).toBe('fixture');});`, reusable);
  await run('no-browser-is-unused', "test('math',()=>expect(2).toBe(2));", reusable, 'success', 'unused');
  for (let i = 0; i < 2; i++) {
    await run('context-reuse-is-fresh-' + i, `test.use({reuseContext:true});test('fresh',async({page})=>{
      await page.goto('about:blank');expect(await page.evaluate(()=>window.name)).toBe('');await page.evaluate(()=>window.name='prior-attempt');});`, reusable, 'success', 'unused');
  }
  await run('launch-override-is-fresh', `test.use({launchOptions:{executablePath:${JSON.stringify(path.join(root,"chrome"))},chromiumSandbox:true,args:["--disable-gpu"]}});test("fresh",async({page})=>{await page.goto("about:blank")});`, reusable, 'success', 'unused');
  await run('assertion-failure-still-cleans', "test('website',async({page})=>{await page.goto('about:blank');expect(1).toBe(2)});", reusable, 'failed');
  await run('worker-restart-is-fresh', "test('first',async({page})=>{expect(1).toBe(2)});test('next',async({page})=>{await page.goto('about:blank')});", reusable, 'failed');
  for (const [extension, esm] of [['js', false], ['js', true], ['cjs', false], ['mjs', true], ['ts', false], ['ts', true], ['cts', false], ['mts', true]]) {
    const dir = path.join(work, 'format-' + ++serial);
    await fs.mkdir(dir);
    await fs.writeFile(path.join(dir, 'package.json'), JSON.stringify({type: esm ? 'module' : 'commonjs'}));
    const useRequire = !esm && ['js', 'cjs'].includes(extension);
    const script = path.join(dir, 'journey.' + extension);
    await fs.writeFile(script, (useRequire ? "const {test,expect}=require('@playwright/test');" : prefix) + "test('page',async({page})=>{await page.goto('about:blank');expect(page.url()).toBe('about:blank')});");
    await run('format-' + extension + '-' + esm, '', reusable, 'success', 'clean', {script: undefined, script_path: script});
  }
} finally { await reusable.stop(); }

for (const [name, source, expected] of [
  ['caught-cdp', "test('policy',async({browser})=>{await browser.newBrowserCDPSession().catch(()=>{})});", 'error'],
  ['caught-selector', "import {selectors} from '@playwright/test';test('policy',async({page})=>{await selectors.register('x',()=>({query(){},queryAll(){return []}})).catch(()=>{})});", 'error'],
  ['failure-before-policy', "test('policy',async({browser})=>{expect.soft(1).toBe(2);await browser.newBrowserCDPSession().catch(()=>{})});", 'failed'],
  ['caught-download-path', `test('policy',async({page})=>{${download}await download.path().catch(()=>{});});`, 'error'],
  ['caught-selector-skipped', "import {selectors} from '@playwright/test';await selectors.register('x',()=>({query(){},queryAll(){return []}})).catch(()=>{});test.skip('skipped',()=>{});", 'error'],
  ['caught-selector-empty', "import {selectors} from '@playwright/test';await selectors.register('x',()=>({query(){},queryAll(){return []}})).catch(()=>{});", 'error'],
  ['unawaited-browser-close', "test('policy',async({browser})=>{void browser.close().catch(()=>{})});", 'error'],
]) {
  const browser = await owner();
  try { await run(name, source, browser, expected, 'retire'); }
  finally { await browser.stop(); }
}
await run('explicit-fresh-cdp', "test('advanced',async({browser})=>{const cdp=await browser.newBrowserCDPSession();await cdp.send('Browser.getVersion');await cdp.detach()});", undefined);
const helperOwner = await owner();
try {
  const directory = path.join(work, 'early-helper');
  await fs.mkdir(directory);
  await fs.writeFile(path.join(directory, 'package.json'), '{"type":"module"}');
  await fs.writeFile(path.join(directory, 'helper.mjs'), "import {selectors} from 'playwright';export const register=selectors.register.bind(selectors);");
  const entry = path.join(directory, 'test.mjs');
  await fs.writeFile(entry, "import {register} from './helper.mjs';import {test} from '@playwright/test';test('cached',async()=>{await register('x',()=>({query(){},queryAll(){return []}})).catch(()=>{})});");
  await run('early-helper-cannot-bypass-selector-policy', '', helperOwner, 'error', 'retire', {script: undefined, script_path: entry});
} finally { await helperOwner.stop(); }
const eof = await owner();
await eof.stop(true);
console.log(JSON.stringify({case: 'owner-eof-shutdown', passed: true}));
const dead = await owner();
await dead.stop();
await run('dead-endpoint-is-executor-error', "test('unavailable',async({page})=>{});", dead, 'error', 'retire');
const early = launch(['--browser-owner']);
early.child.stdin.end(JSON.stringify({version: 2, owner_id: 'early', work_dir: path.join(work, 'early')}) + '\n{"type":"stop"}\n');
assert.equal((await early.ended).code, 0);
console.log(JSON.stringify({case: 'stop-before-readiness', passed: true}));
const crashed = await owner();
const {chromium} = await import(pathToFileURL(path.join(installed.modules, 'playwright/index.mjs')));
const connection = await chromium.connect(crashed.endpoint);
const cdp = await connection.newBrowserCDPSession();
await cdp.send('Browser.close').catch(() => {});
assert.equal((await crashed.ended).code, 1, 'idle browser death must end the owner');
console.log(JSON.stringify({case: 'idle-browser-death', passed: true}));
console.log(JSON.stringify({complete: true}));
