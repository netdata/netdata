// SPDX-License-Identifier: GPL-3.0-or-later
import test from 'node:test';
import assert from 'node:assert/strict';
import {baselineRecords} from './contract-2.mjs';

// Synthetic baseline transcripts exercise every nested wire model. Real producer
// output is checked separately by runner.test.mjs through the same frozen guard.
const journey = [
  {type: 'event', event: {kind: 'suite', at_ms: 1, phase: 'discovery', declared_tests: 1}},
  {type: 'event', event: {kind: 'test', at_ms: 2, test_id: 'checkout', title: 'Checkout', phase: 'start', status: 'running', expected_status: 'passed'}},
  {type: 'event', event: {kind: 'step', at_ms: 3, test_id: 'checkout', title: 'Submit', phase: 'test.step', status: 'failed', duration_ms: 1.5, message: 'fixture failure'}},
  {type: 'result', result: {status: 'timeout', termination_reason: 'execution_budget', error: 'fixture failure',
    tests: {declared: 1, passed: 0, failed: 0, timed_out: 0, skipped: 0, expected_failure: 0, not_run: 1},
    artifacts: [], interrupted_artifacts: [{id: 'capture-1', kind: 'screenshot', path: 'output/failure.png', mime: 'image/png'}], capture_state: 'available'}},
];
const lighthouse = [
  {type: 'event', event: {kind: 'audit', at_ms: 1, phase: 'start', status: 'running'}},
  {type: 'result', result: {status: 'success', metrics: {performance: 88, fcp_ms: 100, lcp_ms: 200, tbt_ms: 0, si_ms: 300, cls: 0.01},
    artifacts: [{id: 'report', kind: 'report', path: 'output/lighthouse.html', mime: 'text/html'}], capture_state: 'available'}},
];
const wire = records => records.map(record => JSON.stringify(record)).join('\n') + '\n';

test('frozen baseline accepts journey and Lighthouse wire models', () => {
  for (const records of [journey, lighthouse]) assert.deepEqual(baselineRecords(wire(records)), records);
});

const incompatible = {
  'outer field': records => { records[0].extra = true; },
  'event field': records => { records[0].event.details = {}; },
  'result field': records => { records.at(-1).result.details = {}; },
  'counts field': records => { records.at(-1).result.tests.retries = 1; },
  'capture field': records => { records.at(-1).result.interrupted_artifacts[0].bytes = 1; },
  'event kind': records => { records[0].event.kind = 'extension'; },
  'frame type': records => { records[0].type = 'extension'; },
  'outcome': records => { records.at(-1).result.status = 'new-outcome'; },
  'reason': records => { records.at(-1).result.termination_reason = 'new-reason'; },
  'capture state': records => { records.at(-1).result.capture_state = 'new-state'; },
  'capture type': records => { records.at(-1).result.interrupted_artifacts[0].kind = 'trace'; },
  'wrong scalar type': records => { records[0].event.at_ms = '1'; },
  'mixed payloads': records => { records[0].result = {status: 'error'}; },
  'missing payload': records => { delete records[0].event; },
  'late frame': records => { records.push(records[0]); },
  'missing terminal': records => { records.pop(); },
};
for (const [name, mutate] of Object.entries(incompatible)) {
  test(`frozen baseline rejects ${name}`, () => {
    const records = structuredClone(journey);
    mutate(records);
    assert.throws(() => baselineRecords(wire(records)));
  });
}
test('frozen baseline rejects an added Lighthouse metric', () => {
  const records = structuredClone(lighthouse);
  records.at(-1).result.metrics.new_metric = 1;
  assert.throws(() => baselineRecords(wire(records)), /unknown field new_metric/);
});
test('frozen baseline rejects blank records and oversized frames', () => {
  assert.throws(() => baselineRecords('\n' + wire(journey)));
  assert.throws(() => baselineRecords(JSON.stringify({type:'event',event:{kind:'stdout',at_ms:1,message:'x'.repeat(4*1024*1024)}})), /exceeds 4 MiB/);
});
