// SPDX-License-Identifier: GPL-3.0-or-later
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {metricsFromLHR} from '../runner/lighthouse.mjs';
const sourceRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const dependencies = process.env.DEM_TEST_DEPENDENCIES || sourceRoot;

async function fixtureRuntime(dependencyRoot, browser = 'missing-prepared-browser') {
  const root = await fs.realpath(await fs.mkdtemp(path.join(os.tmpdir(), 'dem-runtime-test-')));
  await fs.cp(path.join(sourceRoot, 'runner'), path.join(root, 'runner'), {recursive:true});
  await fs.symlink(dependencyRoot, path.join(root, 'dependencies'));
  const pins = JSON.parse(await fs.readFile(path.join(sourceRoot, 'manifest.json'), 'utf8'));
  pins.node_version = process.versions.node;
  await fs.writeFile(path.join(root, 'manifest.json'), JSON.stringify(pins));
  await fs.writeFile(path.join(root, 'runtime.json'), JSON.stringify({contract:1, version:'test-release',
    paths:{dependencies:'dependencies',browser}}));
  return root;
}
const runtimeRoot = await fixtureRuntime(dependencies);
const assets = path.join(runtimeRoot, 'runner');

async function run(source, options = {}, root = runtimeRoot) {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'dem-runner-test-'));
  const request = {version: 1, kind: 'journey', run_id: 'test',
    work_dir: dir, script: source, timeout_ms: 10000,
    capture: false, secrets: {}, ...options};
  const child = spawn(process.execPath, [path.join(root, 'runner/run.mjs')], {stdio: ['pipe', 'pipe', 'pipe'], env: {...process.env, NODE_OPTIONS: '', DEM_SECRET_UNPASSED: 'must-not-inherit'}});
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
    assert.equal(actual.events.find(e=>e.kind==='suite'&&e.phase==='discovery').declared_tests,expected.declared);
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

test('transported secrets are not persisted and stdout remains framed',async()=>{
  const source=prefix+"test('env',async()=>{expect(process.env.DEM_SECRET_TOKEN).toBe(['synthetic','value'].join('-'));expect(process.env.DEM_SECRET_UNPASSED).toBeUndefined();expect(process.env.NODE_OPTIONS).toBeUndefined();console.log('not a protocol frame');console.error('diagnostic');await test.step('observed step',async()=>{});});";
  const result=await run(source,{secrets:{DEM_SECRET_TOKEN:'synthetic-value'}});
  assert.equal(result.result.status,'success');
  assert.ok(result.events.filter(e=>e.kind==='stdout').map(e=>e.message).join('').includes('not a protocol frame'));
  assert.ok(result.events.some(e=>e.kind==='step'&&e.title==='observed step'));
  for(const name of await fs.readdir(result.dir,{recursive:true})){const p=path.join(result.dir,name);if((await fs.stat(p)).isFile())assert.ok(!(await fs.readFile(p,'utf8')).includes('synthetic-value'),name);}
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


test('fragmented worker and CLI output cannot reconstruct controlled secrets', async()=>{
  const secret='fixture-only-secret-0123456789';
  const writes="process.stdout.write(process.env.DEM_SECRET_TOKEN.slice(0,20));await new Promise(r=>setTimeout(r,30));process.stdout.write(process.env.DEM_SECRET_TOKEN.slice(20)+'\\n');";
  for (const [source,scope] of [[prefix+"test('stream',async()=>{"+writes+"});",'worker'],[prefix+"import fs from 'node:fs';"+writes.replaceAll('process.stdout.write(', 'fs.writeSync(1,')+"test('pass',async()=>{});",'cli']]) {
    const actual=await run(source,{secrets:{DEM_SECRET_TOKEN:secret}});
    assert.equal(actual.result.status,'success');
    assert.ok(actual.events.some(e=>e.kind==='stdout'&&e.phase===scope),scope);
    const recovered=actual.events.filter(e=>e.kind==='stdout').map(e=>e.message).join('');
    assert.ok(!recovered.includes(secret),recovered);
    assert.ok(!recovered.includes('fixture-only-secret-'),recovered);
    assert.ok(recovered.includes('[REDACTED]'),recovered);
  }
});

test('actual reporter protects fragmented buffers and unfinished secret prefixes', async()=>{
  const script=`const p=require(${JSON.stringify(path.join(assets,'protocol.cjs'))});
    p.configureSecrets({DEM_SECRET_TOKEN:'fixture-only-secret-0123456789',DEM_SECRET_UTF:'秘密🔐value'});
    const Reporter=require(${JSON.stringify(path.join(assets,'reporter.cjs'))});const r=new Reporter({});
    r.onStdOut('fixture-only-secret-');r.onStdOut('0123456789\\n');
    const b=Buffer.from('秘密🔐value\\n');for(const byte of b)r.onStdErr(Buffer.from([byte]));
    r.onStdOut('safe-prefix fixture-only-secret-');r.finish();`;
  const child=spawn(process.execPath,['-e',script],{stdio:['ignore','ignore','pipe','pipe']});
  let wire='';child.stdio[3].on('data',b=>wire+=b);
  assert.equal(await new Promise(r=>child.on('close',r)),0);
  const records=wire.trim().split('\n').map(JSON.parse);
  const recovered=records.filter(r=>r.type==='event').map(r=>r.event.message||'').join('');
  assert.ok(!recovered.includes('fixture-only-secret-'),recovered);
  assert.ok(!recovered.includes('秘密'),recovered);
  assert.ok(recovered.includes('safe-prefix'),recovered);
  assert.ok(recovered.includes('[REDACTED]'),recovered);
});


test('optional Lighthouse report write failure preserves completed measurements', async()=>{
  const dir=await fs.mkdtemp(path.join(os.tmpdir(),'dem-report-failure-'));
  const modules=path.join(dir,'node_modules');
  for(const packageName of ['lighthouse','chrome-launcher']) {
    await fs.mkdir(path.join(modules,packageName),{recursive:true});
    await fs.writeFile(path.join(modules,packageName,'package.json'),'{"type":"module"}');
  }
  await fs.mkdir(path.join(modules,'lighthouse/core/config'),{recursive:true});
  await fs.mkdir(path.join(modules,'chrome-launcher/dist'),{recursive:true});
  await fs.writeFile(path.join(modules,'lighthouse/core/index.js'),`import assert from 'node:assert/strict';export default async(url,options,config)=>{assert.equal(config.profile,'desktop');assert.deepEqual(options.onlyCategories,['performance']);return {lhr:{categories:{performance:{score:0.88}},audits:{'largest-contentful-paint':{numericValue:123}}},report:['{}','<html>fixture</html>']}};`);
  await fs.writeFile(path.join(modules,'lighthouse/core/config/lr-desktop-config.js'),"export default {profile:'desktop'};");
  const closed=path.join(dir,'closed');
  await fs.writeFile(path.join(modules,'chrome-launcher/dist/index.js'),`import assert from 'node:assert/strict';import fs from 'node:fs/promises';export const launch=async options=>{assert.deepEqual(options.chromeFlags,['--headless=new','--disable-dev-shm-usage']);return {port:1,kill:async()=>fs.writeFile(${JSON.stringify(closed)},'closed')}};`);
  await fs.mkdir(path.join(dir,'output/lighthouse.html'),{recursive:true});
  const fixture=await fixtureRuntime(dir,'chrome');
  await fs.symlink(process.execPath,path.join(fixture,'chrome'));
  const child=spawn(process.execPath,[path.join(fixture,'runner/lighthouse.mjs')],{stdio:['pipe','ignore','pipe','pipe']});
  let wire='';child.stdio[3].on('data',b=>wire+=b);
  child.stdin.end(JSON.stringify({work_dir:dir,url:'http://fixture.invalid',timeout_ms:1000,capture:true}));
  await new Promise(r=>child.on('close',r));
  const records=wire.trim().split('\n').map(JSON.parse);
  const result=records.find(r=>r.type==='result').result;
  assert.equal(result.status,'success');
  assert.deepEqual(result.metrics,{performance:88,lcp_ms:123});
  assert.equal(result.capture_state,'unavailable');
  assert.deepEqual(result.artifacts,[]);
  assert.ok(records.some(r=>r.event?.phase==='capture'&&r.event.message.includes('EISDIR')));
  assert.equal(await fs.readFile(closed,'utf8'),'closed');
});


test('known stream prefixes stay protected for every byte partition', async()=>{
  const {default:protocol}=await import('../runner/protocol.cjs');
  protocol.configureSecrets({DEM_SECRET_A:'abc',DEM_SECRET_B:'abcdef',DEM_SECRET_UTF:'秘密🔐value',DEM_SECRET_NEWLINE:'first\nsecond',DEM_SECRET_SHORT:'!'});
  try {
    const input=Buffer.from('ordinary 界 abc abcdef 秘密🔐value first\nsecond ! tail abcde');
    for(let width=1;width<=input.length;width++) {
      let recovered='';const stream=protocol.secretStream(value=>recovered+=value);
      for(let i=0;i<input.length;i+=width)stream.write(input.subarray(i,i+width));
      stream.end();
      assert.equal(recovered,'ordinary 界 [REDACTED] [REDACTED] [REDACTED] [REDACTED] [REDACTED] tail [REDACTED]',String(width));
    }
  } finally {protocol.configureSecrets();}
});


test('secret redaction cannot change browser-start failure classification',async()=>{
  const actual=await run(prefix+"test('browser',async({page})=>{});",{secrets:{DEM_SECRET_SHORT:'a'}});
  assert.equal(actual.result.status,'error');
});


test('assertion failure takes precedence over launch inability in the same test',async()=>{
  const actual=await run("import {test,expect,chromium} from '@playwright/test'; test('mixed',async()=>{expect.soft(1).toBe(2);await chromium.launch({executablePath:'/missing-prepared-browser'});});");
  assert.equal(actual.result.tests.failed,1);
  assert.equal(actual.result.status,'failed');
});

test('mixed stream chunks preserve byte ordering and empty UTF-8 boundaries',async()=>{
  const {default:protocol}=await import('../runner/protocol.cjs');
  for(const chunks of [[Buffer.from([0xe7]),'',Buffer.from([0x95,0x8c])],[Buffer.from([0xe7]),'middle',Buffer.from([0x95,0x8c])]]){
    let recovered='';const stream=protocol.secretStream(value=>recovered+=value);
    for(const chunk of chunks)stream.write(chunk);
    stream.end();
    assert.equal(recovered,Buffer.concat(chunks.map(chunk=>Buffer.isBuffer(chunk)?chunk:Buffer.from(chunk))).toString('utf8'));
  }
  protocol.configureSecrets({DEM_SECRET_UTF:'界'});
  try{
    let recovered='';const stream=protocol.secretStream(value=>recovered+=value);
    stream.write(Buffer.from([0xe7]));stream.write('');stream.write(Buffer.from([0x95,0x8c]));stream.end();
    assert.equal(recovered,'[REDACTED]');
  }finally{protocol.configureSecrets();}
});

test('monitor requests cannot select component paths', async()=>{
  for(const field of ['dependencies_path','browser_path']) {
    const actual=await run(prefix+"test('pass',async()=>{});",{[field]:'/tmp/alternate'});
    assert.equal(actual.result.status,'error');
    assert.match(actual.result.error,/component paths are not request fields/);
    assert.ok(!actual.events.some(event=>event.phase==='discovery'));
  }
});

test('journey launches the runtime browser with sandbox enabled', async()=>{
  const root=await fixtureRuntime(dependencies,'chrome');
  await fs.writeFile(path.join(root,'chrome'),'#!/bin/sh\nprintf \'%s\\n\' "$@" > "$(dirname "$0")/browser-args"\nexit 1\n',{mode:0o755});
  const actual=await run(prefix+"test('browser',async({page})=>{});",{},root);
  assert.equal(actual.result.status,'error');
  const args=(await fs.readFile(path.join(root,'browser-args'),'utf8')).trim().split('\n');
  assert.ok(args.some(arg=>arg.startsWith('--headless')));
  assert.ok(!args.includes('--no-sandbox'));
  assert.ok(!args.includes('--disable-setuid-sandbox'));
});

async function check(root) {
  const child=spawn(process.execPath,[path.join(root,'runner/run.mjs'),'--check'],{
    stdio:['ignore','pipe','pipe'],env:{PATH:process.env.PATH}});
  let stdout='',stderr='';
  child.stdout.on('data',chunk=>stdout+=chunk);
  child.stderr.on('data',chunk=>stderr+=chunk);
  const code=await new Promise((resolve,reject)=>{child.on('error',reject);child.on('close',resolve)});
  return {code,stdout,stderr};
}

test('self-check verifies runtime-owned versions and private imports without a session', async()=>{
  const root=await fixtureRuntime(dependencies,'chrome');
  const pins=JSON.parse(await fs.readFile(path.join(root,'manifest.json'),'utf8'));
  await fs.writeFile(path.join(root,'chrome'),`#!/bin/sh\n[ "$#" = 1 ] && [ "$1" = --version ] || exit 27\nprintf 'Chromium ${pins.chromium.version}\\n'\n`,{mode:0o755});
  const good=await check(root);
  assert.equal(good.code,0,good.stderr);
  assert.equal(good.stderr,'');
  assert.deepEqual(JSON.parse(good.stdout),{contract:1,version:'test-release',uid:process.geteuid(),gid:process.getegid()});
  assert.ok(Buffer.byteLength(good.stdout)<4096);
  for(const [changed,message] of [
    [{...pins,packages:{...pins.packages,playwright:'0.0.0'}},/package version mismatch/],
    [{...pins,node_version:'0.0.0'},/Node or execution contract mismatch/],
    [{...pins,chromium:{...pins.chromium,version:'0.0.0'}},/Chromium version mismatch/],
  ]) {
    await fs.writeFile(path.join(root,'manifest.json'),JSON.stringify(changed));
    const bad=await check(root);
    assert.equal(bad.code,1);
    assert.equal(bad.stdout,'');
    assert.match(JSON.parse(bad.stderr).error,message);
  }
});

test('self-check rejects missing private imports before invoking the browser', async()=>{
  const dependencies=await fs.mkdtemp(path.join(os.tmpdir(),'dem-check-modules-'));
  const pins=JSON.parse(await fs.readFile(path.join(sourceRoot,'manifest.json'),'utf8'));
  for(const [name,version] of Object.entries(pins.packages)) {
    const dir=path.join(dependencies,'node_modules',name);
    await fs.mkdir(dir,{recursive:true});
    await fs.writeFile(path.join(dir,'package.json'),JSON.stringify({type:'module',version}));
  }
  for(const name of ['@playwright/test/index.mjs','playwright/index.mjs','lighthouse/core/index.js']) {
    const target=path.join(dependencies,'node_modules',name);
    await fs.mkdir(path.dirname(target),{recursive:true});
    await fs.writeFile(target,'export default {};');
  }
  const root=await fixtureRuntime(dependencies,'chrome');
  await fs.writeFile(path.join(root,'chrome'),'#!/bin/sh\ntouch "$(dirname "$0")/browser-called"\n',{mode:0o755});
  const actual=await check(root);
  assert.equal(actual.code,1);
  assert.equal(actual.stdout,'');
  assert.match(JSON.parse(actual.stderr).error,/lr-desktop-config/);
  await assert.rejects(fs.access(path.join(root,'browser-called')));
});
