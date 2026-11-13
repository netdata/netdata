// SPDX-License-Identifier: GPL-3.0-or-later
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {EventEmitter} from 'node:events';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {journeyStop, journeyResult, monotonicMilliseconds} from '../runner/journey.mjs';
import {baselineRecords} from './contract-2.mjs';

const sourceRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
// Include two Node startups and filesystem preparation on a loaded test host.
// Exact deadline ordering is covered by the controlled-clock tests below.
const subprocessBudget = 5000;
const delayedExit = subprocessBudget + 1000;
function controlledStop(stopAt = 100) {
  const child = new EventEmitter();
  const signals = [], pending = new Map();
  child.kill = signal => { signals.push(signal); return true; };
  let now = 0, next = 0, terminal = false;
  const stop = journeyStop(child, stopAt, () => terminal, {
    now: () => now,
    setTimeout: (callback, delay) => { const id = ++next; pending.set(id, {callback, delay}); return id; },
    clearTimeout: id => pending.delete(id),
  });
  return {child, stop, signals, pending, terminal: () => {terminal = true;},
    advance(value) { now = value; const [id, timer] = pending.entries().next().value; pending.delete(id); timer.callback(); }};
}

test('journey soft stop signals its child once and preserves terminal-before-stop results', () => {
  const active = controlledStop();
  active.advance(100);
  assert.deepEqual(active.signals, ['SIGINT']);
  assert.equal(active.stop.requested, true);
  assert.equal(active.pending.size, 0);
  const complete = controlledStop();
  complete.terminal(); complete.advance(100);
  assert.deepEqual(complete.signals, []);
  assert.equal(complete.stop.requested, false);
});

test('journey stop clears on exit rather than waiting for pipe close', () => {
  const active = controlledStop();
  active.child.emit('exit', 0, null);
  assert.equal(active.pending.size, 0);
  assert.equal(active.stop.requested, false);
  assert.deepEqual(active.signals, []);
});

test('long monotonic deadlines clamp and rearm instead of overflowing Node timers', () => {
  const max = 2_147_483_647;
  const active = controlledStop(max + 1000);
  assert.equal([...active.pending.values()][0].delay, max);
  active.advance(max);
  assert.equal([...active.pending.values()][0].delay, 1000);
  assert.deepEqual(active.signals, []);
  active.advance(max + 1000);
  assert.deepEqual(active.signals, ['SIGINT']);
});

test('only an accepted parent budget stop forwards separate interrupted candidates', () => {
  const ordinary = {id:'screenshot-1'}, interrupted = {id:'screenshot-2'};
  const terminal = {status:'cancelled', termination_reason:'cancelled', tests:{not_run:1},
    error:'first assertion', artifacts:[ordinary], interrupted_artifacts:[interrupted]};
  const cancelled = journeyResult(terminal, false, true);
  assert.equal(cancelled.status, 'cancelled');
  assert.equal(cancelled.termination_reason, 'cancelled');
  assert.equal(cancelled.interrupted_artifacts, undefined);
  assert.deepEqual(cancelled.artifacts, [ordinary]);
  const budget = journeyResult(terminal, true, true);
  assert.equal(budget.status, 'timeout');
  assert.equal(budget.termination_reason, 'execution_budget');
  assert.equal(budget.error, 'first assertion');
  assert.deepEqual(budget.tests, {not_run:1});
  assert.deepEqual(budget.artifacts, [ordinary]);
  assert.deepEqual(budget.interrupted_artifacts, [interrupted]);
  assert.equal(journeyResult({status:'cancelled', interrupted_artifacts:[interrupted]}, false, true).capture_state, 'not_needed');
  assert.equal(journeyResult({status:'cancelled', interrupted_artifacts:[interrupted]}, true, true).capture_state, 'available');
  assert.match(journeyResult({}, true, true).error, /execution budget exhausted/);
});

async function fakeRuntime(cli) {
  const root = await fs.realpath(await fs.mkdtemp(path.join(os.tmpdir(), 'dem-deadline-')));
  await fs.cp(path.join(sourceRoot, 'runner'), path.join(root, 'runner'), {recursive:true});
  await fs.mkdir(path.join(root, 'dependencies/node_modules/playwright'), {recursive:true});
  await fs.writeFile(path.join(root, 'dependencies/node_modules/playwright/cli.js'), cli);
  await fs.writeFile(path.join(root, 'runtime.json'), JSON.stringify({contract:2, version:'test', paths:{dependencies:'dependencies', browser:'chrome'}}));
  return root;
}
async function runFake(cli, stopAfter, options = {}) {
  const root = await fakeRuntime(cli);
  const work = path.join(root, 'work'); await fs.mkdir(work);
  const child = spawn(process.execPath, [path.join(root, 'runner/run.mjs')], {stdio:['pipe','pipe','pipe']});
  let stdout = '', stderr = '';
  child.stdout.on('data', data => stdout += data); child.stderr.on('data', data => stderr += data);
  child.stdin.end(JSON.stringify({version:2, kind:'journey', work_dir:work, run_id:'test', timeout_ms:10000,
    stop_at_monotonic_ms:monotonicMilliseconds() + stopAfter, script:'unused', capture:true, ...options}));
  const started = monotonicMilliseconds();
  const code = await new Promise((resolve,reject) => {child.on('error',reject); child.on('close',resolve);});
  assert.equal(stderr, '');
  const negotiated = Array.isArray(options.extensions) && options.extensions.every(value => typeof value === 'string' && value);
  const records = negotiated ? stdout.trim().split('\n').map(line => JSON.parse(line)) : baselineRecords(stdout);
  if (negotiated) baselineRecords(records.filter(record => !['extension', 'extension_init'].includes(record.type)).map(record => JSON.stringify(record)).join('\n'));
  const results = records.filter(record => record.type === 'result'); assert.equal(results.length, 1, stdout);
  return {root, work, code, records, result:results[0].result, elapsed:monotonicMilliseconds() - started};
}

test('exhausted startup budget does not spawn selected user work', async () => {
  const actual = await runFake("require('node:fs').writeFileSync('started','bad')", -1);
  assert.equal(actual.code, 0);
  assert.equal(actual.result.status, 'timeout');
  assert.equal(actual.result.termination_reason, 'execution_budget');
  assert.equal(actual.result.tests, undefined);
  assert.equal(actual.result.capture_state, 'unavailable');
  await assert.rejects(fs.access(path.join(actual.work, 'started')));
});

test('short budget can stop a live CLI and preserves actual interrupted counts', async () => {
  const actual = await runFake(`const fs=require('node:fs');
    process.on('SIGINT',()=>{fs.writeSync(3,JSON.stringify({type:'result',result:{status:'cancelled',termination_reason:'cancelled',tests:{declared:1,not_run:1},interrupted_artifacts:[{id:'screenshot-1',kind:'screenshot',path:'output/failure.png',mime:'image/png'}]}})+'\\n');process.exit(0)});
    setInterval(()=>{},1000);`, subprocessBudget);
  assert.equal(actual.result.status, 'timeout');
  assert.equal(actual.result.termination_reason, 'execution_budget');
  assert.deepEqual(actual.result.tests, {declared:1,not_run:1});
  assert.deepEqual(actual.result.interrupted_artifacts, [{id:'screenshot-1',kind:'screenshot',path:'output/failure.png',mime:'image/png'}]);
  assert.ok(actual.elapsed >= subprocessBudget - 100, actual.elapsed);
});

test('budget stop before CLI reporting is ready still reports timeout without invented counts', async () => {
  const actual = await runFake('setInterval(()=>{},1000);', subprocessBudget);
  assert.equal(actual.result.status, 'timeout');
  assert.equal(actual.result.termination_reason, 'execution_budget');
  assert.equal(actual.result.tests, undefined);
  assert.match(actual.result.error, /without a terminal result \(SIGINT\)/);
});

test('accepted budget stop precedes a successful terminal with nonzero CLI exit', async () => {
  const actual = await runFake(`const fs=require('node:fs');process.on('SIGINT',()=>{
    fs.writeSync(3,JSON.stringify({type:'result',result:{status:'success',tests:{declared:1,passed:1}}})+'\\n');process.exit(130)});
    setInterval(()=>{},1000);`, subprocessBudget);
  assert.equal(actual.result.status, 'timeout');
  assert.equal(actual.result.termination_reason, 'execution_budget');
  assert.deepEqual(actual.result.tests, {declared:1,passed:1});
});

test('a budget stop does not excuse invalid reporter protocol', async () => {
  const actual = await runFake("require('node:fs').writeSync(3,'invalid JSON\\n');setInterval(()=>{},1000);", subprocessBudget);
  assert.equal(actual.result.status, 'error');
  assert.equal(actual.result.termination_reason, undefined);
});

test('terminal report before soft stop survives delayed CLI exit without signal', async () => {
  const actual = await runFake(`const fs=require('node:fs');
    process.on('SIGINT',()=>fs.writeFileSync('signalled','bad'));
    fs.writeSync(3,JSON.stringify({type:'result',result:{status:'success',tests:{declared:1,passed:1}}})+'\\n');
    setTimeout(()=>process.exit(0),${delayedExit});`, subprocessBudget);
  assert.equal(actual.result.status, 'success');
  assert.equal(actual.result.termination_reason, undefined);
  assert.ok(actual.elapsed >= delayedExit, actual.elapsed);
  await assert.rejects(fs.access(path.join(actual.work, 'signalled')));
});

test('CLI exit before soft stop still joins inherited reporter pipe EOF', async () => {
  const actual = await runFake(`const fs=require('node:fs');
    fs.writeSync(3,JSON.stringify({type:'result',result:{status:'success'}})+'\\n');
    require('node:child_process').spawn(process.execPath,['-e','setTimeout(()=>{},${delayedExit})'],{stdio:['ignore','inherit','inherit',3]}).unref();
    process.exit(0);`, subprocessBudget);
  assert.equal(actual.result.status, 'success');
  assert.ok(actual.elapsed >= delayedExit, actual.elapsed);
});

test('journey request requires positive safe monotonic handoff', async () => {
  for (const value of [undefined, 0, -1, 0.5, Number.MAX_SAFE_INTEGER + 1]) {
    const actual = await runFake('process.exit(27)', 10000, {stop_at_monotonic_ms:value});
    assert.equal(actual.result.status, 'error');
    assert.match(actual.result.error, /stop_at_monotonic_ms/);
  }
});

async function report(script) {
  const child = spawn(process.execPath, ['-e', `const Reporter=require(${JSON.stringify(path.join(sourceRoot, 'runner/reporter.cjs'))});${script}`], {stdio:['ignore','ignore','pipe','pipe']});
  let wire = '', stderr = '';
  child.stdio[3].on('data', data => wire += data); child.stderr.on('data', data => stderr += data);
  assert.equal(await new Promise(resolve => child.on('close', resolve)), 0, stderr);
  return baselineRecords(wire).find(record => record.type === 'result').result;
}

test('native suite timeout beats onError and earlier assertion while retaining first error', async () => {
  const result = await report(`const r=new Reporter({});const t={id:'one',title:'first',expectedStatus:'passed'};
    r.onTestEnd(t,{status:'failed',duration:1,errors:[{message:'first assertion'}],steps:[]});
    r.onError({message:'global timeout'});r.onEnd({status:'timedout'});`);
  assert.equal(result.status, 'timeout');
  assert.equal(result.termination_reason, 'suite_timeout');
  assert.equal(result.error, 'first assertion');
  assert.equal(result.tests.failed, 1);
});

test('reporter retains ordinary and interrupted PNG provenance with unique IDs', async () => {
  const work = await fs.realpath(await fs.mkdtemp(path.join(os.tmpdir(), 'dem-interrupted-capture-')));
  await fs.mkdir(path.join(work, 'output'));
  const png = path.join(work, 'output/failure.png'); await fs.writeFile(png, Buffer.from([137,80,78,71,13,10,26,10]));
  const result = await report(`const r=new Reporter({capture:true,workDir:${JSON.stringify(work)}});
    for(const [id,status] of [['first','failed'],['second','interrupted']])r.onTestEnd({id,title:id,expectedStatus:'passed'},
    {status,duration:1,errors:[{message:id+' error'}],steps:[],attachments:[{name:'screenshot',contentType:'image/png',path:${JSON.stringify(png)}}]});
    r.onEnd({status:'interrupted'});`);
  assert.equal(result.status, 'cancelled');
  assert.equal(result.termination_reason, 'cancelled');
  assert.equal(result.error, 'first error');
  assert.equal(result.tests.failed, 1); assert.equal(result.tests.not_run, 1);
  assert.equal(result.artifacts[0].id, 'screenshot-1');
  assert.equal(result.interrupted_artifacts[0].id, 'screenshot-2');
});

const fakeResult = {type: 'result', result: {status: 'success', tests: {declared: 1, passed: 1}}};
const fakeCLI = records => "const fs=require('node:fs');" + records.map(record => 'fs.writeSync(3,' + JSON.stringify(JSON.stringify(record) + '\n') + ');').join('');

test('parent acknowledges selected extensions first and relays malformed optional bodies without outcome failure', async () => {
  const malformed = {type: 'extension', extension: {feature: 'failure_evidence.v1', data: null}};
  const actual = await runFake(fakeCLI([malformed, fakeResult]), 10000, {extensions: ['failure_evidence.v1', 'failure_evidence.v1']});
  assert.equal(actual.result.status, 'success');
  assert.deepEqual(actual.records[0], {type: 'extension_init', extension_init: {selected: ['failure_evidence.v1']}});
  assert.deepEqual(actual.records[1], malformed);
  assert.equal(actual.records.at(-1).type, 'result');
});

test('legacy parent relay discards recognized optional records and ignores unselected feature IDs', async () => {
  const optional = {type: 'extension', extension: {feature: 'failure_evidence.v1', data: {record: 'complete'}}};
  const legacy = await runFake(fakeCLI([optional, fakeResult]), 10000);
  assert.equal(legacy.result.status, 'success');
  assert.deepEqual(legacy.records, [legacy.records.at(-1)]);
  const unknown = await runFake(fakeCLI([{type: 'extension', extension: {feature: 'future.v1', data: null}}, fakeResult]), 10000,
    {extensions: ['failure_evidence.v1']});
  assert.equal(unknown.result.status, 'success');
  assert.equal(unknown.records.some(record => record.type === 'extension'), false);
});

test('parent refuses mixed families and duplicate results but never emits late optional completion', async () => {
  for (const records of [
    [{type: 'extension', extension: null, result: {}}, fakeResult],
    [fakeResult, fakeResult],
    ...['constructor', 'toString', '__proto__', ['extension'], ['extension_init'], ['event'], ['result']].map(type => [{type}, fakeResult]),
  ]) {
    const actual = await runFake(fakeCLI(records), 10000, {extensions: ['failure_evidence.v1']});
    assert.equal(actual.result.status, 'error');
    assert.match(actual.result.error, /invalid or (mixed|duplicate) reporter record/);
  }
  const actual = await runFake(fakeCLI([fakeResult, {type: 'extension', extension: {feature: 'failure_evidence.v1', data: {record: 'complete'}}}]), 10000,
    {extensions: ['failure_evidence.v1']});
  assert.equal(actual.result.status, 'success');
  assert.equal(actual.records.some(record => record.type === 'extension'), false);
});

test('unexpected reporter initialization is relayed for optional invalidation only to opted-in peers', async () => {
  const init = {type: 'extension_init', extension_init: {selected: ['failure_evidence.v1']}};
  const gated = await runFake(fakeCLI([init, fakeResult]), 10000, {extensions: ['failure_evidence.v1']});
  assert.equal(gated.result.status, 'success');
  assert.equal(gated.records.filter(record => record.type === 'extension_init').length, 2);
  const legacy = await runFake(fakeCLI([init, fakeResult]), 10000);
  assert.equal(legacy.result.status, 'success');
  assert.equal(legacy.records.some(record => record.type === 'extension_init'), false);
});

// The parent withholds its terminal until exit/EOF; late core observations still
// precede the Agent terminal as in the baseline runtime. Optional completion does not.
test('parent preserves late core observations while waiting for reporter exit', async () => {
  const late = {type: 'event', event: {kind: 'stdout', at_ms: 1, message: 'late'}};
  const actual = await runFake(fakeCLI([fakeResult, late]), 10000);
  assert.equal(actual.result.status, 'success');
  assert.deepEqual(actual.records[0], late);
  assert.equal(actual.records.at(-1).type, 'result');
});
