// SPDX-License-Identifier: GPL-3.0-or-later
import test, {afterEach} from 'node:test';
import assert from 'node:assert/strict';
import {EventEmitter} from 'node:events';
import contextFixture from '../runner/browser-context.cjs';
import sample from '../runner/browser-sampler.cjs';
import Summary from '../runner/browser-summary.cjs';
import createStepView from '../runner/step-view.cjs';
import protocol from '../runner/protocol.cjs';
import Evidence from '../runner/failure-evidence.cjs';

afterEach(() => protocol.configureSecrets({}));
const tick = () => new Promise(resolve => setImmediate(resolve));
const categories = ['uncaught', 'console_error', 'transport', 'http_error'];
const zeros = () => Object.fromEntries(categories.map(category => [category, 0]));
function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return {promise, resolve, reject};
}
function handles(values, calls = [], pending) {
  return values.map(value => ({value, evaluate(fn, rest) {
    calls.push({first: this.value, rest: rest.map(handle => handle.value)});
    return pending?.promise || Promise.resolve(fn(this.value, rest.map(handle => handle.value)));
  }}));
}
const consoleMessage = (args = [], text = '', type = 'error') => ({type: () => type, args: () => args, text: () => text});
const request = (url = 'https://example.org/path?private=yes#fragment', method = 'GET', error = 'connection refused') => ({
  url: () => url, method: () => method, failure: () => ({errorText: error}),
});
const response = (status, req = request()) => ({status: () => status, request: () => req});
function observer() {
  const context = new EventEmitter(), annotations = [];
  const finish = contextFixture.observe(context, annotations);
  return {context, annotations, finish, read: () => JSON.parse(annotations[0].description)};
}
function annotation(samples = [], counts, omissions = []) {
  const totals = counts || zeros();
  if (!counts) for (const value of samples) totals[value.category]++;
  return {type: Summary.ANNOTATION, description: JSON.stringify({v: 1, state: 'observed', counts: totals, samples, omissions, rpc_calls: 0})};
}
const uncaught = message => ({category: 'uncaught', message});
const consoleText = text => ({category: 'console_error', state: 'resolved', text});
const http = (status = 500) => ({category: 'http_error', method: 'GET', url: 'https://example.org/path', status});
const transport = error => ({category: 'transport', method: 'GET', url: 'https://example.org/path', error});
function reporter() {
  const records = [], feature = new Summary(record => records.push(structuredClone(record)));
  return {feature, records, data: () => records.map(record => record.extension.data)};
}
const finishTest = (feature, id, ...annotations) => feature.test({id, annotations});

test('context observation counts every anomaly after candidate exhaustion and detaches synchronously', () => {
  const {context, finish, read} = observer();
  let userCalls = 0;
  const userListener = () => userCalls++;
  context.on('console', userListener);
  for (let n = 0; n < 10; n++) {
    context.emit('weberror', {error: () => ({message: 'uncaught ' + n})});
    context.emit('console', consoleMessage([], 'console ' + n));
    context.emit('requestfailed', request());
    context.emit('response', response(n === 9 ? 1000 : 400));
    context.emit('response', response(399));
    context.emit('console', consoleMessage([], 'ignored', 'log'));
  }
  finish();
  const snapshot = read();
  assert.deepEqual(snapshot.counts, {uncaught: 10, console_error: 10, transport: 10, http_error: 10});
  assert.equal(snapshot.samples.length, 12);
  assert.equal(snapshot.rpc_calls, 0);
  assert(snapshot.omissions.includes('candidate_limit'));
  assert.deepEqual(context.eventNames(), ['console']);
  assert.deepEqual(context.listeners('console'), [userListener]);
  assert.equal(userCalls, 20);
  context.emit('console', consoleMessage([], 'user still receives events'));
  assert.equal(userCalls, 21);
  context.emit('weberror', {error: () => ({message: 'after finalization'})});
  finish();
  assert.deepEqual(read(), snapshot);
});

test('console candidates issue only three evaluations with four handles and never refill', async () => {
  const {context, finish, read} = observer(), calls = [];
  for (let n = 0; n < 9; n++) context.emit('console', consoleMessage(handles(['same', 42, null, true, 'not sampled'], calls)));
  await tick();
  finish();
  const snapshot = read();
  assert.equal(calls.length, 3);
  assert(calls.every(call => call.rest.length === 3));
  assert.equal(snapshot.counts.console_error, 9);
  assert.equal(snapshot.samples.length, 3);
  assert.equal(snapshot.rpc_calls, 3);
  assert(snapshot.samples.every(value => value.args.length === 4 && value.args_omitted === 1));
  assert(snapshot.omissions.includes('args_limit'));
  assert(snapshot.omissions.includes('candidate_limit'));
});

test('pending and rejected samples remain explicit and late resolution cannot alter finalized annotations', async () => {
  const {context, finish, read} = observer(), pending = deferred(), rejected = deferred();
  context.emit('console', consoleMessage(handles(['pending'], [], pending)));
  context.emit('console', consoleMessage(handles(['rejected'], [], rejected)));
  rejected.reject(new Error('destroyed renderer'));
  await tick();
  finish();
  const snapshot = read();
  assert.deepEqual(snapshot.samples.map(value => value.state), ['pending', 'unavailable']);
  assert(snapshot.omissions.includes('pending'));
  assert(snapshot.omissions.includes('unavailable'));
  assert(snapshot.samples.every(value => !Object.hasOwn(value, 'args')));
  pending.resolve([{value: 'late'}]);
  await tick();
  assert.deepEqual(read(), snapshot);
});

test('primitive sampling never renders objects and retains whole values or explicit omissions', async () => {
  const hostile = new Proxy({}, {get() {throw new Error('object property read');}});
  assert.deepEqual(await sample(handles([hostile, Symbol('x'), () => {}, undefined])), [
    {omitted: 'object'}, {omitted: 'symbol'}, {omitted: 'function'}, {omitted: 'unrepresentable'},
  ]);
  assert.deepEqual(await sample(handles([1n, NaN, Infinity, -Infinity])), [
    {omitted: 'unrepresentable'}, {omitted: 'non_finite'}, {omitted: 'non_finite'}, {omitted: 'non_finite'},
  ]);
  assert.deepEqual(await sample(handles([null, false, 1.25, 'whole'])), [{value: null}, {value: false}, {value: 1.25}, {value: 'whole'}]);
  assert.deepEqual(await sample(handles(['a'.repeat(510), 'a'.repeat(511), '\u0001'.repeat(85), '\u0001'.repeat(86)])), [
    {value: 'a'.repeat(510)}, {omitted: 'too_long'}, {value: '\u0001'.repeat(85)}, {omitted: 'too_long'},
  ]);
  assert.deepEqual(await sample(handles(['😀'.repeat(128), 'a'.repeat(513)])), [{omitted: 'too_long'}, {omitted: 'too_long'}]);
});

test('secrets are redacted before whole-value byte checks and URL sanitization', async () => {
  protocol.configureSecrets({token: 'secret-token', expansion: '!'});
  assert.deepEqual(await sample(handles(['secret-token', '!'.repeat(100), 'see https://user:password@example.org/path?token=secret-token#fragment'])), [
    {value: '[REDACTED]'}, {omitted: 'too_long'}, {value: 'see https://example.org/path'},
  ]);
  const {context, finish, read} = observer();
  context.emit('requestfailed', request('https://user:password@example.org/path?q=secret-token#fragment', 'GET', 'secret-token'));
  context.emit('response', response(1000, request('https://example.org/' + 'z'.repeat(300))));
  context.emit('console', consoleMessage([], 'secret-token'));
  context.emit('console', consoleMessage([], 'a'.repeat(511)));
  finish();
  const snapshot = read();
  assert.equal(snapshot.samples[0].url, 'https://example.org/path');
  assert.equal(snapshot.samples[0].error, '[REDACTED]');
  assert.equal(snapshot.samples[1].status, 1000);
  assert.equal(snapshot.samples[1].url_omitted, 'too_long');
  assert(!Object.hasOwn(snapshot.samples[1], 'url'));
  assert.equal(snapshot.samples[2].text, '[REDACTED]');
  assert.equal(snapshot.samples[3].state, 'unavailable');
  assert.equal(snapshot.samples[3].text_omitted, 'too_long');
  assert(snapshot.omissions.includes('too_long'));
  assert(snapshot.omissions.includes('unavailable'));
});

test('whole browser-text omission marks only published text and does not invent arguments', () => {
  const {context, finish, annotations} = observer();
  context.emit('console', consoleMessage([], 'x'.repeat(600)));
  finish();
  const {feature, data} = reporter();
  finishTest(feature, 'text', ...annotations);
  feature.complete();
  const value = data().find(item => item.record === 'test').samples[0];
  assert.equal(value.state, 'unavailable');
  assert.equal(value.text_omitted, 'too_long');
  assert(!Object.hasOwn(value, 'args'));
  assert(data().some(item => item.record === 'truncated' && item.scope === 'text'));
});

test('reporter aggregates clean and missing tests without rows and deduplicates attributable representatives', () => {
  const {feature, data} = reporter();
  finishTest(feature, 'clean', annotation());
  finishTest(feature, 'missing');
  finishTest(feature, 'first', annotation([uncaught('same')]));
  finishTest(feature, 'second', annotation([uncaught('same')]));
  feature.complete();
  const rows = data().filter(value => value.record === 'test');
  assert.equal(rows.length, 2);
  assert.deepEqual(rows[0].test.sample_ids, [1]);
  assert.equal(rows[0].samples[0].test_id, 'first');
  assert(!Object.hasOwn(rows[1].test, 'sample_ids'));
  assert(!Object.hasOwn(rows[1], 'samples'));
  assert.deepEqual(data().at(-1).run.tests, {observed: 3, incomplete: 0, invalid: 0, no_summary: 1});
  assert.equal(data().at(-1).run.counts.uncaught, 2);
});

test('row exhaustion keeps aggregate truth and admits only the first absent category in the existing pool', () => {
  const {feature, data} = reporter();
  for (let n = 0; n < 64; n++) finishTest(feature, 't' + n, annotation([uncaught('same')]));
  finishTest(feature, 'after-row-cap', annotation([uncaught('new'), consoleText('first console'), consoleText('second console'), http(), transport('failed')]));
  finishTest(feature, 'still-counted', annotation([consoleText('third console')]));
  feature.complete();
  assert.equal(data().filter(value => value.record === 'test').length, 64);
  const late = data().filter(value => value.record === 'late_sample');
  assert.equal(late.length, 3);
  assert.deepEqual(late.map(value => value.sample.category), ['console_error', 'http_error', 'transport']);
  assert(late.every(value => value.sample.test_id === 'after-row-cap'));
  assert.equal(new Set(late.map(value => value.sample.id)).size, 3);
  const run = data().at(-1).run;
  assert.equal(run.tests.observed, 66);
  assert.deepEqual(run.counts, {uncaught: 65, console_error: 3, transport: 1, http_error: 1});
  assert(run.omissions.includes('row_limit'));
  assert(!run.omissions.includes('pool_full'));
  assert.equal(data().filter(value => value.record === 'truncated' && value.scope === 'tests').length, 1);
});

test('sample pool exhaustion and oversized identities omit detail without losing valid counts', () => {
  const {feature, data} = reporter();
  for (let n = 0; n < 6; n++) finishTest(feature, 't' + n, annotation([uncaught('unique ' + n)]));
  finishTest(feature, 'z'.repeat(127), annotation([http()]));
  finishTest(feature, 'z'.repeat(127), annotation());
  feature.complete();
  const bodies = data().flatMap(value => value.samples || []);
  assert.equal(bodies.length, 4);
  const run = data().at(-1).run;
  assert.equal(run.counts.uncaught, 6);
  assert.equal(run.counts.http_error, 1);
  assert.equal(run.tests.observed, 8);
  assert(run.omissions.includes('pool_full'));
  assert(run.omissions.includes('identity_limit'));
  const clean = reporter();
  finishTest(clean.feature, 'z'.repeat(127), annotation());
  clean.feature.complete();
  assert(!clean.data().at(-1).run.omissions);
});

test('missing, open, malformed and duplicate annotations remain distinct from producer failure', () => {
  const {feature, data} = reporter();
  finishTest(feature, 'missing');
  finishTest(feature, 'open', {type: Summary.ANNOTATION, description: '{"v":1,"state":"open"}'});
  finishTest(feature, 'malformed', {type: Summary.ANNOTATION, description: '{'});
  finishTest(feature, 'duplicate', annotation(), annotation());
  feature.complete();
  assert.deepEqual(data().at(-1).run.tests, {observed: 0, incomplete: 1, invalid: 2, no_summary: 1});
  assert.deepEqual(data().filter(value => value.record === 'test').map(value => value.test.state), ['incomplete', 'invalid', 'invalid']);
  const producer = reporter();
  finishTest(producer.feature, 'valid', annotation([uncaught('keep')]));
  finishTest(producer.feature, 'invalid', {type: Summary.ANNOTATION, description: '{"v":1,"state":"invalid","reason":"annotation_limit"}'});
  producer.feature.complete();
  finishTest(producer.feature, 'after-terminal', annotation([uncaught('ignore')]));
  assert.equal(producer.data().length, 2);
  assert.equal(producer.data()[0].samples[0].message, 'keep');
  assert.deepEqual(producer.data()[1], {record: 'invalid', reason: 'annotation_limit'});
});

test('malformed unrelated annotations do not interrupt browser reporting', () => {
  const {feature, data} = reporter();
  finishTest(feature, 'missing', null, undefined, 3, 'unrelated');
  finishTest(feature, 'observed', null, annotation([uncaught('keep')]), {});
  feature.complete();
  assert.deepEqual(data().at(-1).run.tests, {observed: 1, incomplete: 0, invalid: 0, no_summary: 1});
  assert.equal(data()[0].samples[0].message, 'keep');
});

test('malformed candidate shapes and unsafe numbers never enter aggregates', () => {
  const malformed = [
    annotation([consoleText('a'.repeat(511))]),
    annotation([uncaught('a'), uncaught('b'), uncaught('c'), uncaught('d')]),
    annotation([http(399)]), annotation([http(1.5)]),
    annotation([{category: 'console_error', state: 'pending', args: [{value: 'invented'}]}]),
    annotation([{category: 'console_error', state: 'resolved', args: [{value: {}}]}]),
    annotation([uncaught('a')], zeros()),
    annotation([], {...zeros(), uncaught: Number.MAX_SAFE_INTEGER + 1}),
    annotation([], {...zeros(), uncaught: -1}),
    annotation([], zeros(), ['future_flag']),
    {type: Summary.ANNOTATION, description: ' '.repeat(16 * 1024 + 1)},
  ];
  for (const [index, input] of malformed.entries()) {
    const {feature, data} = reporter();
    finishTest(feature, 'bad' + index, input);
    feature.complete();
    assert.equal(data()[0].test.state, 'invalid', 'case ' + index);
    assert.deepEqual(data().at(-1).run.counts, zeros());
  }
});

test('checked aggregate overflow emits a feature terminal without replacing earlier valid evidence', () => {
  const {feature, data} = reporter();
  finishTest(feature, 'maximum', annotation([uncaught('keep')], {...zeros(), uncaught: Number.MAX_SAFE_INTEGER}));
  finishTest(feature, 'overflow', annotation([uncaught('extra')]));
  feature.complete();
  assert.equal(data().length, 2);
  assert.equal(data()[0].run.counts.uncaught, Number.MAX_SAFE_INTEGER);
  assert.equal(data()[0].samples[0].message, 'keep');
  assert.deepEqual(data()[1], {record: 'invalid', reason: 'counter_overflow'});
});

test('generated records and whole-run wire remain within structural byte ceilings', () => {
  for (const character of ['x', '"', '\\', '\u0001', '<', '\u2028', '😀']) {
    const fill = budget => {
      let value = '';
      while (Evidence.encoded(value + character) <= budget) value += character;
      while (Evidence.encoded(value + 'a') <= budget) value += 'a';
      return value;
    };
    const {feature, records, data} = reporter();
    const counters = Object.fromEntries(categories.map(category => [category, Math.floor(Number.MAX_SAFE_INTEGER / 70)]));
    for (let n = 0; n < 64; n++) {
      const suffix = String(n), text = fill(500) + suffix;
      const samples = [];
      for (let k = 0; k < 3; k++) samples.push(
        uncaught(text + k),
        {category: 'console_error', state: 'resolved', args: Array.from({length: 4}, () => ({value: text + k})), args_omitted: Number.MAX_SAFE_INTEGER},
        {category: 'transport', method: fill(10) + suffix + k, url: 'https://example.org/' + 'a'.repeat(230), error: fill(120) + suffix + k, text_truncated: true},
        {category: 'http_error', method: fill(10) + suffix + k, url: 'https://example.org/' + 'a'.repeat(230), status: Number.MAX_SAFE_INTEGER, text_truncated: true},
      );
      finishTest(feature, ('t' + n).padEnd(126, 'a'), annotation(samples, counters, ['candidate_limit', 'too_long']));
    }
    feature.complete();
    assert.equal(data().filter(value => value.record === 'test').length, 64);
    const published = data().flatMap(value => value.samples || []);
    assert.equal(published.length, 16);
    assert.equal(new Set(published.map(value => value.id)).size, 16);
    for (const category of categories) assert.equal(published.filter(value => value.category === category).length, 4);
    assert(data().every(value => Evidence.encoded(value) <= 16 * 1024));
    const acknowledgement = {type: 'extension_init', extension_init: {selected: ['failure_evidence.v1', Summary.FEATURE]}};
    const wireBytes = [acknowledgement, ...records].reduce((sum, value) => sum + Evidence.encoded(value) + 1, 0);
    assert(wireBytes <= 72 * 1024, 'generated wire bytes: ' + wireBytes);
    assert.equal(data().at(-1).record, 'complete');
  }
});

test('worker finalization bounds a full twelve-candidate annotation before transport', async () => {
  const {context, annotations, finish, read} = observer();
  for (let n = 0; n < 3; n++) {
    context.emit('weberror', {error: () => ({message: '\u0001'.repeat(512)})});
    context.emit('console', consoleMessage(handles(Array(4).fill('a'.repeat(510)))));
    context.emit('requestfailed', request('https://example.org/' + 'a'.repeat(230), 'a'.repeat(50), '\u0001'.repeat(128)));
    context.emit('response', response(Number.MAX_SAFE_INTEGER, request('https://example.org/' + 'a'.repeat(230), 'a'.repeat(50))));
  }
  await tick();
  finish();
  assert.equal(read().state, 'observed');
  assert.equal(read().samples.length, 12);
  assert(read().samples.some(value => value.text_truncated));
  assert(Evidence.encoded(read()) <= 16 * 1024);
  assert(Evidence.encoded(annotations[0]) <= 33 * 1024);
});

test('shared step view excludes only canonical observer API steps and preserves original trees', () => {
  const source = '/runner/browser-sampler.cjs', view = createStepView(source);
  const root = {category: 'test.step', title: 'user', steps: []};
  const hidden = {category: 'pw:api', location: {file: source}, parent: root, steps: []};
  const child = {category: 'test.step', title: 'evaluateHandle', parent: hidden, steps: []};
  const lookalike = {category: 'pw:api', title: 'evaluateHandle', location: {file: '/journey/browser-sampler.cjs'}, parent: root, steps: []};
  const otherCategory = {category: 'test.step', location: {file: source}, parent: root, steps: []};
  root.steps.push(hidden, lookalike, otherCategory); hidden.steps.push(child);
  assert.equal(view.visible(hidden), false);
  assert.equal(view.visible(lookalike), true);
  assert.equal(view.visible(otherCategory), true);
  assert.deepEqual([...view.children(root.steps)], [child, lookalike, otherCategory]);
  assert.equal(view.parent(child), root);
  assert.equal(child.parent, hidden);
  assert.deepEqual(root.steps, [hidden, lookalike, otherCategory]);
  assert.deepEqual([...createStepView().children(root.steps)], root.steps);
});
