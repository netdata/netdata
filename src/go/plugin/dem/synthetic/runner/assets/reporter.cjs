// SPDX-License-Identifier: GPL-3.0-or-later
'use strict';
const {text, event, write, captureState, screenshotCandidate, secretStream} = require('./protocol.cjs');

module.exports = class DEMReporter {
  constructor(options) {
    this.options = options;
    this.tests = new Map();
    this.artifacts = [];
    this.errors = [];
    this.executionError = false;
    this.confirmedFailure = false;
    this.finished = false;
    this.stdio = Object.fromEntries(['stdout', 'stderr'].map(kind => [kind,
      secretStream(message => this.emit(kind, {phase: 'worker', message}))]));
  }
  // Suppress Playwright's automatic fallback terminal reporter.
  printsToStdio() { return true; }
  emit(kind, values) { write(3, {type: 'event', event: event(kind, values)}); }
  onBegin(config, suite) {
    for (const test of suite.allTests()) this.tests.set(test.id, {test, result: null});
    if (config.workers !== 1 || [...this.tests.values()].some(({test}) => test.retries !== 0 || test.repeatEachIndex !== 0)) {
      this.onError({message: 'DEM journeys require one worker, no retries and no repetitions; script overrides are unsupported'});
      this.finish();
      // onBegin precedes worker startup. Do not execute an unsupported retry policy.
      process.exit(1);
    }
    this.emit('suite', {title: 'Discovered ' + this.tests.size + ' tests', phase: 'discovery'});
  }
  onTestBegin(test) {
    this.emit('test', {test_id: test.id, title: text(test.title), phase: 'start', status: 'running', expected_status: test.expectedStatus});
  }
  onStepEnd(test, result, step) {
    this.emit('step', {test_id: test.id, title: text(step.title), phase: text(step.category), status: step.error ? 'failed' : 'passed', duration_ms: step.duration, ...(step.error ? {message: text(step.error.message || step.error.value)} : {})});
  }
  onStdOut(chunk) { this.stdio.stdout.write(chunk); }
  onStdErr(chunk) { this.stdio.stderr.write(chunk); }
  onError(error) {
    const message = text(error.message || error.value || error);
    this.executionError = true;
    if (!this.errors.length) this.errors.push(message);
    this.emit('error', {phase: 'execution', status: 'error', message});
  }
  onTestEnd(test, result) {
    this.tests.set(test.id, {test, result});
    const failureSteps = [];
    const visit = steps => { for (const step of steps) { if (step.error) failureSteps.push(step); visit(step.steps || []); } };
    visit(result.steps || []);
    const phase = failureSteps.at(-1)?.category || 'test';
    let launchOnly = result.errors.length > 0;
    for (const error of result.errors) {
      const original = String(error.message || error.value || error);
      const message = text(original);
      // Classify original evidence; redaction must not change monitoring truth.
      if (/browserType\.launch(?:PersistentContext)?:/.test(original)) {
        this.executionError = true;
      } else {
        launchOnly = false;
      }
      if (!this.errors.length) this.errors.push(message);
      this.emit('error', {test_id: test.id, title: text(test.title), phase: text(phase), status: result.status, expected_status: test.expectedStatus, message});
    }
    if (result.status === 'failed' && test.expectedStatus !== 'failed' && !launchOnly) this.confirmedFailure = true;
    this.emit('test', {test_id: test.id, title: text(test.title), phase: 'end', status: result.status, expected_status: test.expectedStatus, duration_ms: result.duration});
    if (this.options.capture && ['failed', 'timedOut'].includes(result.status)) {
      for (const attachment of result.attachments) {
        if (attachment.name !== 'screenshot' || attachment.contentType !== 'image/png' || !attachment.path) continue;
        try {
          const candidate = screenshotCandidate(attachment.path, this.options.workDir, this.artifacts.length + 1);
          if (candidate) this.artifacts.push(candidate);
        } catch { /* Go also validates candidates; a missing capture does not hide the test result. */ }
      }
    }
  }
  onEnd(fullResult) { this.finish(fullResult); }
  finish(fullResult) {
    if (this.finished) return;
    this.finished = true;
    for (const stream of Object.values(this.stdio)) stream.end();
    const counts = {declared: this.tests.size, passed: 0, failed: 0, timed_out: 0, skipped: 0, expected_failure: 0, not_run: 0};
    for (const {test, result} of this.tests.values()) {
      if (!result || result.status === 'interrupted') counts.not_run++;
      else if (result.status === 'skipped') counts.skipped++;
      else if (result.status === 'timedOut') counts.timed_out++;
      else if (test.expectedStatus === 'failed') counts.expected_failure++;
      else if (result.status === 'failed') counts.failed++;
      else if (result.status === 'passed' && test.expectedStatus === 'passed') counts.passed++;
      else counts.not_run++;
    }
    let status = 'inconclusive';
    if (counts.timed_out) status = 'timeout';
    else if (this.confirmedFailure) status = 'failed';
    else if (this.executionError) status = 'error';
    else if (fullResult?.status === 'timedout') status = 'timeout';
    else if (fullResult?.status === 'interrupted') status = 'cancelled';
    else if (counts.declared > 0 && counts.passed === counts.declared && fullResult?.status === 'passed') status = 'success';
    write(3, {type: 'result', result: {status, tests: counts, ...(this.errors.length ? {error: this.errors[0]} : {}), artifacts: this.artifacts, capture_state: captureState(this.options.capture, status, this.artifacts)}});
  }
};
