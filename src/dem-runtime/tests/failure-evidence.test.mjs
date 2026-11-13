// SPDX-License-Identifier: GPL-3.0-or-later
import test from 'node:test';
import assert from 'node:assert/strict';
import Evidence from '../runner/failure-evidence.cjs';
import protocol from '../runner/protocol.cjs';
import extensions from '../runner/extensions.cjs';

const location = {file: '/journeys/main.spec.ts', line: 12, column: 3};
function fixture() {
  const records = [];
  const evidence = new Evidence({sourceDir: '/journeys'}, record => records.push(structuredClone(record.extension.data)));
  return {records, evidence};
}
const specimen = (id = 'test-1') => ({id, title: 'checkout', titlePath: () => ['shop', 'checkout'], expectedStatus: 'passed', location});
function step(title = 'pay', parent, category = 'test.step', error) {
  const value = {title, category, location, parent, steps: [], duration: 4, error};
  parent?.steps.push(value);
  return value;
}
const result = (status = 'failed', errors = [], steps = []) => ({status, duration: 5, errors, steps});

test('optional selection is exact, deduplicated and malformed hints fall back to baseline', () => {
  for (const value of [undefined, null, {}, 'failure_evidence.v1', [null], [''], ['failure_evidence.v1', 7]]) {
    assert.equal(extensions.selection({kind: 'journey', extensions: value}), undefined);
  }
  assert.deepEqual(extensions.selection({kind: 'journey', extensions: []}), []);
  assert.deepEqual(extensions.selection({kind: 'journey', extensions: ['future.v1']}), []);
  assert.deepEqual(extensions.selection({kind: 'lighthouse', extensions: ['failure_evidence.v1']}), []);
  assert.deepEqual(extensions.selection({kind: 'journey', extensions: ['failure_evidence.v1', 'failure_evidence.v1']}), ['failure_evidence.v1']);
});

test('optional malformed bodies are nonfatal while mixed and unknown envelopes stay strict', () => {
  const selected = ['failure_evidence.v1'];
  for (const extension of [undefined, null, 4, {}, {feature: 'failure_evidence.v1', data: false}]) {
    const record = {type: 'extension', ...(extension === undefined ? {} : {extension})};
    assert.deepEqual(extensions.optional(record, selected, false), record);
    assert.equal(extensions.optional(record, undefined, false), true);
    assert.equal(extensions.optional(record, selected, true), true);
  }
  assert.equal(extensions.optional({type: 'extension', extension: {feature: 'future.v1', data: {}}}, selected, false), true);
  for (const record of [{type: 'extension', extension: {}, result: {}}, {type: 'event', event: {}, extension: {}}, {type: 'unknown'}, null, [],
    ...['constructor', 'toString', '__proto__', ['extension'], ['extension_init'], ['event'], ['result']].map(type => ({type}))]) {
    assert.throws(() => extensions.optional(record, selected, false), /reporter record/);
  }
});

test('tests and concurrent siblings upsert stable identities and retain both begin and end', () => {
  const {evidence, records} = fixture(), t = specimen();
  evidence.test(t);
  const parent = step('checkout'), left = step('left request', parent), right = step('right request', parent);
  for (const s of [parent, left, right]) evidence.step(t, s);
  evidence.step(t, left, true);
  assert.equal([...evidence.steps.values()].filter(s => s.state === 'running').length, 2);
  evidence.step(t, right, true); evidence.step(t, parent, true);
  t.expectedStatus = 'failed'; // test.fail() is legal after onTestBegin.
  evidence.test(t, result('passed')); evidence.complete(); evidence.complete();
  assert.equal(records.filter(r => r.record === 'complete').length, 1);
  const tests = records.filter(r => r.record === 'test').map(r => r.test);
  assert.equal(tests.length, 2);
  assert.deepEqual({...tests[0], status: 'passed', expected_status: 'failed', duration_ms: 5, reporting_complete: true}, tests[1]);
  for (const [id, s] of evidence.steps) {
    const lifecycle = records.filter(r => r.record === 'step' && r.step.id === id).map(r => r.step);
    assert.equal(lifecycle.length, 2);
    assert.deepEqual({...lifecycle[0], state: 'passed', duration_ms: 4}, s);
  }
});

test('independent caps bound ordinary emission and preserve admitted lifecycle ends and late first failure', () => {
  const {evidence, records} = fixture();
  for (let n = 0; n < 70; n++) evidence.test(specimen('t' + n));
  const t = specimen('late-failure'), admitted = [];
  for (const [category, count] of [['test.step', 300], ['pw:api', 300]]) {
    for (let n = 0; n < count; n++) {
      const s = step('action ' + n, undefined, category);
      evidence.step(t, s);
      if (evidence.ids.has(s)) admitted.push(s);
    }
  }
  const error = {message: 'payment failed', stack: 'actual payment source'};
  const failed = step('late payment', undefined, 'test.step', error);
  evidence.step(t, failed); evidence.step(t, failed, true);
  for (const s of admitted) { s.error = {message: '<&\u0001'.repeat(1000)}; evidence.step(t, s, true); }
  evidence.firstFailure(t, result('failed', [error], [failed])); evidence.complete();
  assert.equal(evidence.tests.size, 64);
  assert.equal(evidence.steps.size, 192);
  assert.ok(evidence.nextID <= 193, 'dropped steps never allocate identity metadata');
  assert.equal(records.filter(r => r.record === 'test').length, 64);
  assert.equal(records.filter(r => r.record === 'step').length, 384);
  assert.deepEqual(new Set(records.filter(r => r.record === 'truncated').map(r => r.scope)), new Set(['tests', 'user_steps', 'other_steps', 'text']));
  const failure = records.find(r => r.record === 'failure').failure;
  assert.equal(failure.test.id, t.id);
  assert.equal(failure.path[0].title, 'late payment');
  assert.equal(failure.message, 'payment failed');
  for (const r of records) {
    if (r.record === 'step') assert.ok(Evidence.encoded(r.step) <= 2048, Evidence.encoded(r.step));
    if (r.record === 'test') assert.ok(Evidence.encoded(r.test) <= 1024);
    assert.ok(Evidence.encoded(r) <= 32768);
  }
  assert.ok(records.reduce((bytes, r) => bytes + Evidence.encoded(r), 0) < 1024 * 1024);
});

test('first failure matches the actual uncaught error rather than an earlier caught error', () => {
  const {evidence, records} = fixture(), t = specimen();
  const caught = {message: 'same message', stack: 'caught source'};
  const actual = {message: 'same message', stack: 'actual source'};
  const root = step('root', undefined, 'test.step', actual);
  step('caught assertion', root, 'expect', caught);
  step('uncaught assertion', root, 'expect', {...actual});
  evidence.firstFailure(t, result('failed', [actual], [root]));
  assert.deepEqual(records.find(r => r.record === 'failure').failure.path.map(s => s.title), ['root', 'uncaught assertion']);
});

test('expected failures and caught errors do not consume the protected first failure', () => {
  const {evidence, records} = fixture(), t = specimen(), error = {message: 'expected', stack: 'expected source'};
  t.expectedStatus = 'failed'; evidence.firstFailure(t, result('failed', [error], [step('expected', undefined, 'expect', error)]));
  t.expectedStatus = 'passed'; evidence.firstFailure(t, result('passed', [], [step('caught', undefined, 'expect', error)]));
  assert.equal(records.some(r => r.record === 'failure'), false);
  evidence.firstFailure(t, result('timedOut', [{message: 'timeout'}]));
  assert.equal(records.find(r => r.record === 'failure').failure.message, 'timeout');
  assert.deepEqual(records.find(r => r.record === 'failure').failure.path, []);
});

test('unexpected passes retain evidence without inventing a failing step', () => {
  const {evidence, records} = fixture(), t = specimen(); t.expectedStatus = 'failed';
  evidence.firstFailure(t, result('passed', [], [step('successful action')]));
  const failure = records.find(r => r.record === 'failure').failure;
  assert.deepEqual(failure.path, []); assert.equal(failure.message, '');
});

test('deep paths preserve first eight and last eight source identities with explicit omissions', () => {
  const {evidence, records} = fixture(), t = specimen(), error = {message: 'deep failure', stack: 'deep source'};
  let root, leaf;
  for (let n = 0; n < 40; n++) { const s = step('nested ' + n, leaf, 'test.step', error); root ||= s; leaf = s; }
  evidence.firstFailure(t, result('failed', [error], [root]));
  const failure = records.find(r => r.record === 'failure').failure;
  assert.deepEqual(failure.path.map(s => s.title), [...Array.from({length: 8}, (_, n) => 'nested ' + n), ...Array.from({length: 8}, (_, n) => 'nested ' + (n + 32))]);
  assert.equal(failure.omitted_ancestors, 24);
  assert.equal(failure.path[8].parent_omitted, true);
  assert.ok(failure.path.every(s => s.message === undefined && s.duration_ms === undefined));
});

test('secret redaction precedes encoded clipping and source labels never expose external paths', () => {
  protocol.configureSecrets({DEM_SECRET_TOKEN: 'secret-value-'.repeat(200)});
  try {
    const {evidence, records} = fixture(), t = specimen();
    t.title = 'unused'; t.titlePath = () => ['secret-value-'.repeat(200) + '<&\u0001😀'.repeat(400)];
    evidence.test(t);
    const s = step('secret-value-'.repeat(200)); s.location = {file: '/private/outside/secret.ts', line: 8, column: 1};
    evidence.step(t, s);
    const error = {message: 'secret-value-'.repeat(200) + '\nCall log:\n' + '<&\u0001😀'.repeat(2000)};
    evidence.firstFailure(t, result('failed', [error]));
    const json = JSON.stringify(records);
    assert.ok(!json.includes('secret-value-')); assert.ok(!json.includes('/private/outside'));
    assert.ok(json.includes('[REDACTED]')); assert.ok(json.includes('Call log:'));
    assert.equal(evidence.tests.get(t.id).location.file, 'main.spec.ts');
    assert.equal([...evidence.steps.values()][0].location.file, '[external]');
    const failure = records.find(r => r.record === 'failure').failure;
    assert.equal(failure.text_truncated, true);
    assert.ok(Evidence.encoded(failure.message) <= 12002);
    assert.ok(Evidence.encoded(failure) <= 32768);
  } finally { protocol.configureSecrets(); }
});

test('an unfinished producer keeps received running observations without claiming completion', () => {
  const {evidence, records} = fixture(), t = specimen();
  evidence.test(t); evidence.step(t, step('waiting for response'));
  assert.equal(records.find(r => r.record === 'test').test.reporting_complete, false);
  assert.equal(records.find(r => r.record === 'step').step.state, 'running');
  assert.equal(records.some(r => r.record === 'complete'), false);
});

test('first-failure scanning handles wide siblings linearly without allocating dropped identities', () => {
  const {evidence, records} = fixture(), t = specimen(), error = {message: 'last failure', stack: 'last source'};
  const root = step('wide parent', undefined, 'test.step', error);
  for (let n = 0; n < 20000; n++) step('sibling ' + n, root, 'test.step', n === 19999 ? error : undefined);
  root.steps.indexOf = () => { throw new Error('quadratic sibling search'); };
  evidence.firstFailure(t, result('failed', [error], [root]));
  assert.equal(records.find(r => r.record === 'failure').failure.path.at(-1).title, 'sibling 19999');
  assert.equal(evidence.steps.size, 0, 'scanning does not admit ordinary steps after caps');
  assert.ok(evidence.nextID <= 2);
});

test('failure traversal bounds deep ancestry and discloses an incomplete path while preserving the first error', () => {
  const {evidence, records} = fixture(), t = specimen(), error = {message: 'deep first error', stack: 'deep source'};
  let root, leaf;
  for (let n = 0; n < 1000; n++) { const s = step('nested ' + n, leaf, 'test.step', error); root ||= s; leaf = s; }
  evidence.firstFailure(t, result('failed', [error], [root]));
  const failure = records.find(record => record.record === 'failure').failure;
  assert.equal(failure.message, 'deep first error');
  assert.equal(failure.path_incomplete, true);
  assert.equal(failure.path.at(-1).title, 'nested 63');
  assert.ok(evidence.nextID <= 17, 'only retained first8/last8 path and omitted boundary receive IDs');
});

test('a later error with a matching step never replaces the actual first error', () => {
  const {evidence, records} = fixture(), t = specimen();
  const first = {message: 'first hook failure', stack: 'hook source'}, later = {message: 'later assertion', stack: 'assertion source'};
  evidence.firstFailure(t, result('failed', [first, later], [step('later step', undefined, 'expect', later)]));
  const failure = records.find(record => record.record === 'failure').failure;
  assert.equal(failure.message, first.message); assert.deepEqual(failure.path, []);
});

test('same-source errors in separate branches leave ambiguous ancestry incomplete', () => {
  const {evidence, records} = fixture(), t = specimen();
  const actual = {message: 'same assertion', stack: 'same source location'};
  const first = step('first branch', undefined, 'test.step');
  step('assertion', first, 'expect', {...actual});
  const second = step('second branch', undefined, 'test.step', {...actual});
  step('assertion', second, 'expect', {...actual});
  evidence.firstFailure(t, result('failed', [actual], [first, second]));
  const failure = records.find(r => r.record === 'failure').failure;
  assert.equal(failure.message, actual.message);
  assert.equal(failure.path_incomplete, true);
  assert.deepEqual(failure.path, []);
});

test('stopped tests retain a protected pre-cleanup snapshot independently of errors and detail caps', () => {
  for (const status of ['interrupted', 'timedOut']) {
    const {evidence, records} = fixture(), t = specimen();
    const earlier = {message: 'earlier assertion', stack: 'earlier source'};
    evidence.firstFailure(specimen('earlier'), result('failed', [earlier], [step('earlier', undefined, 'expect', earlier)]));
    for (const category of ['test.step', 'pw:api']) {
      for (let n = 0; n < 200; n++) evidence.step(t, step('noise', undefined, category));
    }
    const caught = step('caught', undefined, 'expect', {message: 'caught', stack: 'caught'});
    const parent = step('Checkout'), child = step('Submit', parent), action = step('Click', child, 'pw:api');
    for (const s of [parent, child, action]) s.duration = -1;
    const cleanup = step('After Hooks', undefined, 'hook'); cleanup.duration = -1;
    const r = result(status, [{message: 'generic timeout'}], [caught, parent, cleanup]);
    evidence.beforeCleanup(t, r, cleanup);
    for (const s of [parent, child, action]) { s.duration = 100; s.error = {message: 'Test ended'}; }
    step('cleanup failure', cleanup, 'expect', {message: 'cleanup failure'});
    evidence.stopContext(t, r);
    evidence.stopContext(specimen('later'), result('timedOut'));
    const stops = records.filter(r => r.record === 'stop');
    assert.equal(stops.length, 1);
    assert.deepEqual(stops[0].stop.path.map(s => s.title), ['Checkout', 'Submit', 'Click']);
    assert.ok(stops[0].stop.path.every(s => s.state === 'running'));
    assert.equal(stops[0].stop.path_incomplete, undefined);
    assert.equal(stops[0].stop.test.status, status);
    assert.equal(stops[0].stop.message, '');
    assert.equal(records.find(r => r.record === 'failure').failure.message, 'earlier assertion');
    assert.ok(Evidence.encoded(stops[0]) < 32768);
    assert.ok(records.reduce((n, r) => n + Evidence.encoded(r), 0) < 1024 * 1024);
  }
});

test('pre-cleanup context discloses concurrent, missing and deep observations without guessing', () => {
  for (const mode of ['siblings', 'disjoint', 'no steps', 'no callback', 'deep']) {
    const {evidence, records} = fixture(), t = specimen();
    const root = step('Checkout'); root.duration = -1;
    const left = step('left', root), right = step('right', mode === 'disjoint' ? undefined : root);
    left.duration = right.duration = -1;
    if (mode === 'deep') {
      root.steps = []; let parent = root;
      for (let n = 0; n < 80; n++) { parent = step('level ' + n, parent); parent.duration = -1; }
    }
    const cleanup = step('After Hooks', undefined, 'hook'); cleanup.duration = -1;
    const roots = mode === 'no steps' ? [] : mode === 'disjoint' ? [root, right] : [root];
    const r = result('timedOut', [], [...roots, cleanup]);
    if (mode !== 'no callback') evidence.beforeCleanup(t, r, cleanup);
    evidence.stopContext(t, r);
    const stop = records.find(r => r.record === 'stop').stop;
    assert.equal(stop.path_incomplete, true, mode);
    assert.ok(stop.path.length <= 16);
    if (mode === 'siblings') assert.deepEqual(stop.path.map(s => s.title), ['Checkout']);
    else if (mode !== 'deep') assert.deepEqual(stop.path, [], mode);
  }
});

test('successful tests release pending observations and do not claim stop context', () => {
  const {evidence, records} = fixture(), t = specimen();
  const pending = step('unawaited'); pending.duration = -1;
  const cleanup = step('After Hooks', undefined, 'hook');
  evidence.beforeCleanup(t, result('passed', [], [pending, cleanup]), cleanup);
  evidence.stopContext(t, result('passed'));
  assert.equal(records.some(r => r.record === 'stop'), false);
  evidence.stopContext(specimen('next'), result('timedOut'));
  assert.deepEqual(records.find(r => r.record === 'stop').stop.path, []);
});

test('candidate clipping announces text loss only when the stop snapshot is published', () => {
  for (const status of ['passed', 'failed', 'timedOut', 'interrupted']) {
    const {evidence, records} = fixture(), t = specimen();
    for (let n = 0; n < 128; n++) {
      const s = step('ordinary ' + n); evidence.step(t, s); evidence.step(t, s, true);
    }
    const pending = step('unawaited '.repeat(100)); pending.duration = -1;
    evidence.step(t, pending); // This identity is outside the ordinary detail pool.
    const cleanup = step('After Hooks', undefined, 'hook');
    const r = result(status, [], [pending, cleanup]);
    evidence.beforeCleanup(t, r, cleanup);
    assert.equal(records.some(r => r.record === 'truncated' && r.scope === 'text'), false);
    evidence.stopContext(t, r);
    const stop = records.find(r => r.record === 'stop');
    const textLoss = records.filter(r => r.record === 'truncated' && r.scope === 'text');
    if (['timedOut', 'interrupted'].includes(status)) {
      assert.equal(stop.stop.text_truncated, true);
      assert.equal(stop.stop.path[0].text_truncated, true);
      assert.equal(textLoss.length, 1);
    } else {
      assert.equal(stop, undefined);
      assert.deepEqual(textLoss, []);
    }
  }
});
