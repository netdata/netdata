// SPDX-License-Identifier: GPL-3.0-or-later
import assert from 'node:assert/strict';

// Frozen wire shape/enum guard, not a replacement for Go semantic validation.
// Baseline: 7278241848caed20c5a5d59b82e343bac5dbb037
// Sources: synthetic/{run,artifact}.go and synthetic/runner/protocol.go.
// Do not derive these definitions from evolving production types or widen them
// for extensions: unrequested output must still pass this baseline guard.
const counts = Object.fromEntries([
  'declared', 'passed', 'failed', 'timed_out', 'skipped', 'expected_failure', 'not_run',
].map(key => [key, 'integer']));
const metrics = Object.fromEntries([
  'performance', 'fcp_ms', 'lcp_ms', 'tbt_ms', 'si_ms', 'cls',
].map(key => [key, 'number']));
const capture = {id: 'string', kind: 'string', path: 'string', mime: 'string'};
const event = {
  declared_tests: 'integer', kind: 'string', at_ms: 'integer', test_id: 'string',
  title: 'string', phase: 'string', status: 'string', expected_status: 'string',
  duration_ms: 'number', message: 'string',
};
const result = {
  status: 'string', termination_reason: 'string', interrupted_artifacts: [capture],
  error: 'string', tests: counts, metrics, artifacts: [capture], capture_state: 'string',
};

function shape(value, fields, label) {
  assert.ok(value && typeof value === 'object' && !Array.isArray(value), `${label}: expected object`);
  for (const [key, item] of Object.entries(value)) {
    assert.ok(Object.hasOwn(fields, key), `${label}: unknown field ${key}`);
    // Go's decoder accepts null; semantic validators decide where it is valid.
    if (item === null) continue;
    const type = fields[key];
    const field = `${label}.${key}`;
    if (Array.isArray(type)) {
      assert.ok(Array.isArray(item), `${field}: expected array`);
      for (const child of item) shape(child, type[0], field);
    } else if (typeof type === 'object') {
      shape(item, type, field);
    } else if (type === 'integer') {
      assert.ok(Number.isSafeInteger(item), `${field}: expected integer`);
    } else {
      assert.equal(typeof item, type, `${field}: wrong type`);
      if (type === 'number') assert.ok(Number.isFinite(item), `${field}: nonfinite number`);
    }
  }
}

function member(value, values, label) {
  assert.ok(values.includes(value), `${label}: unsupported value ${value}`);
}

export function baselineRecords(wire) {
  const lines = wire.split('\n');
  if (lines.at(-1) === '') lines.pop();
  const records = [];
  let terminal = false;
  for (const line of lines) {
    assert.ok(Buffer.byteLength(line) <= 4 * 1024 * 1024, 'baseline frame exceeds 4 MiB');
    assert.ok(!terminal, 'baseline frame after terminal result');
    const record = JSON.parse(line);
    shape(record, {type: 'string', event, result}, 'frame');
    member(record.type, ['event', 'result'], 'frame.type');
    assert.ok(record[record.type], 'baseline frame lacks selected payload');
    assert.ok(!record[record.type === 'event' ? 'result' : 'event'], 'baseline mixed payloads');
    if (record.type === 'event') {
      member(record.event.kind, ['suite', 'test', 'step', 'error', 'stdout', 'stderr', 'audit'], 'event.kind');
    } else {
      const value = record.result;
      member(value.status, ['success', 'failed', 'timeout', 'cancelled', 'inconclusive', 'error'], 'result.status');
      member(value.capture_state, ['disabled', 'not_needed', 'available', 'unavailable'], 'result.capture_state');
      if (value.termination_reason != null) {
        member(value.termination_reason, ['', 'execution_budget', 'test_timeout', 'suite_timeout', 'cancelled'], 'result.termination_reason');
      }
      for (const item of [...(value.artifacts || []), ...(value.interrupted_artifacts || [])]) {
        member(`${item.kind}:${item.mime}`, ['screenshot:image/png', 'report:text/html'], 'capture kind/mime');
      }
      terminal = true;
    }
    records.push(record);
  }
  assert.ok(terminal, 'baseline stream lacks terminal result');
  return records;
}
