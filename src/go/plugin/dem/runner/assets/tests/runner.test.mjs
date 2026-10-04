// SPDX-License-Identifier: GPL-3.0-or-later
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {metricsFromLHR} from '../lighthouse.mjs';
const assets = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const dependencies = process.env.DEM_TEST_DEPENDENCIES || assets;

async function run(source, options = {}) {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'dem-runner-test-'));
  const request = {version: 1, kind: 'journey', run_id: 'test', dependencies_path: dependencies,
    browser_path: '/missing-prepared-browser', work_dir: dir, script: source, timeout_ms: 10000,
    capture: false, secrets: {}, ...options};
  const child = spawn(process.execPath, [path.join(assets, 'run.mjs')], {stdio: ['pipe', 'pipe', 'pipe'], env: {...process.env, NODE_OPTIONS: '', DEM_SECRET_UNPASSED: 'must-not-inherit'}});
  let out = '', err = '';
  child.stdout.on('data', b => out += b);
  child.stderr.on('data', b => err += b);
  child.stdin.end(JSON.stringify(request));
  const code = await new Promise((resolve, reject) => {child.on('error', reject); child.on('close', resolve);});
  assert.equal(err, '', 'bootstrap stderr is not an uncontrolled diagnostic path');
  const records = out.trim().split('\n').filter(Boolean).map(line => JSON.parse(line));
  const results = records.filter(r => r.type === 'result');
  assert.equal(results.length, 1, out);
  assert.ok(records.every(r => ['event', 'result'].includes(r.type)));
  return {code, result: results[0].result, events: records.filter(r => r.type === 'event').map(r => r.event), dir};
}
const counts = values => ({declared: 0, passed: 0, failed: 0, timed_out: 0, skipped: 0, expected_failure: 0, not_run: 0, ...values});
const prefix = "import {test, expect} from '@playwright/test';\n";

const outcomes = {
  success: ["test('pass', async()=>{expect(2).toBe(2)});", 'success', counts({declared:1,passed:1})],
  failure: ["test('fail', async()=>{expect(2).toBe(3)});", 'failed', counts({declared:1,failed:1})],
  'soft assertion': ["test('soft', async()=>{expect.soft(2).toBe(3)});", 'failed', counts({declared:1,failed:1})],
  'all skipped': ["test.skip('skip', async()=>{});", 'inconclusive', counts({declared:1,skipped:1})],
  'pass plus skip': ["test('pass', async()=>{}); test.skip('skip', async()=>{});", 'inconclusive', counts({declared:2,passed:1,skipped:1})],
  'failure plus skip': ["test('fail', async()=>{expect(1).toBe(2)}); test.skip('skip', async()=>{});", 'failed', counts({declared:2,failed:1,skipped:1})],
  'expected failure': ["test('xfail', async()=>{test.fail(); expect(1).toBe(2)});", 'inconclusive', counts({declared:1,expected_failure:1})],
  'unexpected pass': ["test('xpass', async()=>{test.fail(); expect(1).toBe(1)});", 'inconclusive', counts({declared:1,expected_failure:1})],
  empty: ['', 'inconclusive', counts({})],
  'test timeout': ["test('timeout', async()=>{test.setTimeout(100); await new Promise(()=>{})});", 'timeout', counts({declared:1,timed_out:1})],
  'teardown failure': ["test.afterEach(()=>{throw new Error('teardown-failure')}); test('pass',async()=>{});", 'failed', counts({declared:1,failed:1})],
};
for (const [name, [source,status,expected]] of Object.entries(outcomes)) {
  test(name, async () => {
    const actual = await run(prefix + source);
    assert.equal(actual.result.status,status);
    assert.deepEqual(actual.result.tests,expected);
    assert.equal(actual.result.capture_state,'disabled');
    if(status==='failed'||status==='timeout') assert.ok(actual.events.some(e=>e.kind==='error'));
  });
}

test('import and browser launch inability are errors', async () => {
  const imported = await run("import './missing-helper.mjs';");
  assert.equal(imported.result.status,'error');
  const browser = await run(prefix + "test('needs browser',async({page})=>{await page.goto('about:blank')});",{capture:true});
  assert.equal(browser.result.status,'error');
  assert.equal(browser.result.capture_state,'unavailable');
});

test('observed unexpected failure dominates a later browser startup error', async () => {
  const actual = await run(prefix + "test('fails',async()=>{expect(1).toBe(2)});test('cannot launch',async({page})=>{});");
  assert.equal(actual.result.status,'failed');
  assert.ok(actual.events.some(e=>e.kind==='error'&&e.message.includes('browserType.launch')));
});

test('Lighthouse startup inability preserves disabled capture policy', async () => {
  const actual = await run(undefined, {kind:'lighthouse',url:'http://127.0.0.1:1',capture:false});
  assert.equal(actual.result.status,'error');
  assert.equal(actual.result.capture_state,'disabled');
});

test('retry override is rejected before the workflow executes', async () => {
  const marker=path.join(await fs.mkdtemp(path.join(os.tmpdir(),'dem-retry-')),'executed');
  const result=await run(prefix + "import fs from 'node:fs'; test.describe.configure({retries:2}); test('must not run',async()=>{fs.writeFileSync("+JSON.stringify(marker)+",'bad');expect(1).toBe(2)});");
  assert.equal(result.result.status,'error');
  assert.match(result.result.error,/no retries/);
  await assert.rejects(fs.access(marker));
});

test('secrets are controlled and stdout remains framed',async()=>{
  const source=prefix+"test('env',async()=>{expect(process.env.DEM_SECRET_TOKEN).toBe('synthetic-value');expect(process.env.DEM_SECRET_UNPASSED).toBeUndefined();expect(process.env.NODE_OPTIONS).toBeUndefined();console.log('not a protocol frame');console.error('diagnostic');await test.step('observed step',async()=>{});});";
  const result=await run(source,{secrets:{DEM_SECRET_TOKEN:'synthetic-value'}});
  assert.equal(result.result.status,'success');
  assert.ok(result.events.some(e=>e.kind==='stdout'&&e.message.includes('not a protocol frame')));
  assert.ok(result.events.some(e=>e.kind==='step'&&e.title==='observed step'));
  const files=await fs.readdir(result.dir);
  for(const name of files){const p=path.join(result.dir,name);if((await fs.stat(p)).isFile())assert.ok(!(await fs.readFile(p,'utf8')).includes('synthetic-value'));}
});

test('external formats use real helpers, hooks, and one prepared module instance',async()=>{
  for(const [extension,esm] of [['js',false],['js',true],['cjs',false],['mjs',true],['ts',false],['ts',true],['cts',false],['mts',true]]){
    const dir=await fs.mkdtemp(path.join(os.tmpdir(),'dem-external-'));
    await fs.writeFile(path.join(dir,'package.json'),JSON.stringify({type:esm?'module':'commonjs'}));
    const useRequire=!esm&&['js','cjs'].includes(extension);
    const helper='helper.'+extension;
    await fs.writeFile(path.join(dir,helper),useRequire?"const {test}=require('@playwright/test');exports.extended=test.extend({token:async({},use)=>use('ok')});":"import {test} from '@playwright/test';export const extended=test.extend({token:async({},use)=>use('ok')});");
    const pre=useRequire?"const {test:base,expect}=require('@playwright/test');const {test:second}=require('playwright/test');const {extended:test}=require('./"+helper+"');":"import {test as base,expect} from '@playwright/test';import {test as second} from 'playwright/test';import {extended as test} from './"+helper+"';";
    const selected=path.join(dir,'selected.'+extension);
    await fs.writeFile(selected,pre+"let n=0;test.beforeEach(()=>n++);test('selected',async({token})=>{expect(token).toBe('ok');expect(n).toBe(1);expect(base).toBe(second)});");
    const hostile="throw new Error('neighbor must not load');";
    await fs.writeFile(path.join(dir,'playwright.config.cjs'),hostile);
    await fs.writeFile(path.join(dir,'neighbor.spec.cjs'),hostile);
    await fs.writeFile(path.join(dir,'wrong.cjs'),hostile);
    await fs.writeFile(path.join(dir,'tsconfig.json'),JSON.stringify({compilerOptions:{baseUrl:dir,allowJs:true,paths:{'@playwright/test':['./wrong.cjs']}}}));
    const result=await run(undefined,{script_path:selected});
    assert.equal(result.result.status,'success',extension+' esm='+esm+' '+result.result.error);
    assert.equal(result.result.tests.declared,1);
  }
});

test('nullable Lighthouse measurements remain absent and legitimate zero survives',()=>{
  assert.deepEqual(metricsFromLHR({categories:{performance:{score:null}},audits:{'first-contentful-paint':{numericValue:null},'cumulative-layout-shift':{numericValue:0}}}),{cls:0});
  assert.deepEqual(metricsFromLHR({categories:{performance:{score:0}},audits:{'total-blocking-time':{numericValue:0},'largest-contentful-paint':{numericValue:123.4}}}),{performance:0,tbt_ms:0,lcp_ms:123.4});
});

test('resolved secret text is redacted before upstream clipping',async()=>{
  const secret='private-long-value-'.repeat(200);
  const actual=await run(prefix+"test('diagnostic',async()=>{console.log(process.env.DEM_SECRET_token)});",{secrets:{DEM_SECRET_token:secret}});
  assert.equal(actual.result.status,'success');
  assert.ok(actual.events.some(e=>e.kind==='stdout'));
  assert.ok(actual.events.every(e=>!(e.message||'').includes(secret.slice(0,80))));
});
