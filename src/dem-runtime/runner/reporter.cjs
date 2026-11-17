// SPDX-License-Identifier: GPL-3.0-or-later
"use strict";
const {
  text,
  event,
  write,
  captureState,
  captureCandidate,
  secretStream,
} = require("./protocol.cjs");
const FailureEvidence = require("./failure-evidence.cjs");
const BrowserSummary = require("./browser-summary.cjs");
const CaptureDetails = require("./capture-details.cjs");
const { FAILURE_TRACE } = require("./extensions.cjs");

module.exports = class DEMReporter {
  constructor(options) {
    this.options = options;
    this.stepView = require("./step-view.cjs")(options.observerFile);
    this.tests = new Map();
    this.artifacts = [];
    this.interruptedArtifacts = [];
    this.errors = [];
    this.executionError = false;
    this.confirmedFailure = false;
    this.finished = false;
    this.captureDetails =
      options.capture && options.extensions?.includes(CaptureDetails.FEATURE)
        ? new CaptureDetails((record) => write(3, record))
        : undefined;
    this.evidence = options.extensions?.includes(FailureEvidence.FEATURE)
      ? new FailureEvidence({ ...options, stepView: this.stepView }, (record) =>
          write(3, record),
        )
      : undefined;
    this.browserSummary = options.extensions?.includes(BrowserSummary.FEATURE)
      ? new BrowserSummary((record) => write(3, record))
      : undefined;
    this.stdio = Object.fromEntries(
      ["stdout", "stderr"].map((kind) => [
        kind,
        secretStream((message) =>
          this.emit(kind, { phase: "worker", message }),
        ),
      ]),
    );
  }
  // Suppress Playwright's automatic fallback terminal reporter.
  printsToStdio() {
    return true;
  }
  emit(kind, values) {
    write(3, { type: "event", event: event(kind, values) });
  }
  onBegin(config, suite) {
    for (const test of suite.allTests())
      this.tests.set(test.id, { test, result: null });
    if (
      config.workers !== 1 ||
      [...this.tests.values()].some(
        ({ test }) => test.retries !== 0 || test.repeatEachIndex !== 0,
      )
    ) {
      this.onError({
        message:
          "DEM journeys require one worker, no retries and no repetitions; script overrides are unsupported",
      });
      this.finish();
      // onBegin precedes worker startup. Do not execute an unsupported retry policy.
      process.exit(1);
    }
    this.emit("suite", {
      title: "Discovered " + this.tests.size + " tests",
      phase: "discovery",
      declared_tests: this.tests.size,
    });
  }
  onTestBegin(test) {
    this.evidence?.test(test);
    this.emit("test", {
      test_id: test.id,
      title: text(test.title),
      phase: "start",
      status: "running",
      expected_status: test.expectedStatus,
    });
  }
  onStepBegin(test, result, step) {
    if (!this.stepView.visible(step)) return;
    this.evidence?.beforeCleanup(test, result, step);
    this.evidence?.step(test, step);
  }
  onStepEnd(test, result, step) {
    if (!this.stepView.visible(step)) return;
    this.evidence?.step(test, step, true);
    this.emit("step", {
      test_id: test.id,
      title: text(step.title),
      phase: text(step.category),
      status: step.error ? "failed" : "passed",
      duration_ms: step.duration,
      ...(step.error
        ? { message: text(step.error.message || step.error.value) }
        : {}),
    });
  }
  onStdOut(chunk) {
    this.stdio.stdout.write(chunk);
  }
  onStdErr(chunk) {
    this.stdio.stderr.write(chunk);
  }
  onError(error) {
    const message = text(error.message || error.value || error);
    this.executionError = true;
    if (!this.errors.length) this.errors.push(message);
    this.emit("error", { phase: "execution", status: "error", message });
  }
  onTestEnd(test, result) {
    this.browserSummary?.test(test);
    this.evidence?.test(test, result);
    this.evidence?.firstFailure(test, result);
    this.evidence?.stopContext(test, result);
    this.tests.set(test.id, { test, result });
    const failureSteps = [];
    const visit = (steps) => {
      for (const step of this.stepView.children(steps)) {
        if (step.error) failureSteps.push(step);
        visit(step.steps || []);
      }
    };
    visit(result.steps || []);
    const phase = failureSteps.at(-1)?.category || "test";
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
      this.emit("error", {
        test_id: test.id,
        title: text(test.title),
        phase: text(phase),
        status: result.status,
        expected_status: test.expectedStatus,
        message,
      });
    }
    if (
      result.status === "failed" &&
      test.expectedStatus !== "failed" &&
      !launchOnly
    )
      this.confirmedFailure = true;
    this.emit("test", {
      test_id: test.id,
      title: text(test.title),
      phase: "end",
      status: result.status,
      expected_status: test.expectedStatus,
      duration_ms: result.duration,
    });
    if (
      this.options.capture &&
      ["failed", "timedOut", "interrupted"].includes(result.status)
    ) {
      const artifacts =
        result.status === "interrupted"
          ? this.interruptedArtifacts
          : this.artifacts;
      const first = artifacts.length;
      let unaccepted = 0;
      for (const attachment of result.attachments) {
        const screenshot =
          attachment.name === "screenshot" &&
          attachment.contentType === "image/png";
        const trace =
          this.options.extensions?.includes(FAILURE_TRACE) &&
          attachment.name === "trace" &&
          attachment.contentType === "application/zip";
        if (!screenshot && !trace) continue;
        if (!attachment.path) {
          if (screenshot) unaccepted++;
          continue;
        }
        try {
          const candidate = captureCandidate(
            attachment.path,
            this.options.workDir,
            this.artifacts.length + this.interruptedArtifacts.length + 1,
            screenshot ? "screenshot" : "trace",
          );
          if (candidate) artifacts.push(candidate);
          else if (screenshot) unaccepted++;
        } catch {
          if (screenshot) unaccepted++;
          /* Go also validates candidates; a missing capture does not hide the test result. */
        }
      }
      this.captureDetails?.test(test, result, artifacts, first, unaccepted);
    }
  }
  onEnd(fullResult) {
    this.finish(fullResult);
  }
  finish(fullResult) {
    if (this.finished) return;
    this.finished = true;
    for (const stream of Object.values(this.stdio)) stream.end();
    const counts = {
      declared: this.tests.size,
      passed: 0,
      failed: 0,
      timed_out: 0,
      skipped: 0,
      expected_failure: 0,
      not_run: 0,
    };
    for (const { test, result } of this.tests.values()) {
      if (!result || result.status === "interrupted") counts.not_run++;
      else if (result.status === "skipped") counts.skipped++;
      else if (result.status === "timedOut") counts.timed_out++;
      else if (test.expectedStatus === "failed") counts.expected_failure++;
      else if (result.status === "failed") counts.failed++;
      else if (result.status === "passed" && test.expectedStatus === "passed")
        counts.passed++;
      else counts.not_run++;
    }
    let status = "inconclusive",
      terminationReason;
    if (fullResult?.status === "timedout") {
      status = "timeout";
      terminationReason = "suite_timeout";
    } else if (fullResult?.status === "interrupted") {
      status = "cancelled";
      terminationReason = "cancelled";
    } else if (counts.timed_out) {
      status = "timeout";
      terminationReason = "test_timeout";
    } else if (this.confirmedFailure) status = "failed";
    else if (this.executionError) status = "error";
    else if (
      counts.declared > 0 &&
      counts.passed === counts.declared &&
      fullResult?.status === "passed"
    )
      status = "success";
    this.evidence?.complete();
    this.browserSummary?.complete();
    this.captureDetails?.complete();
    write(3, {
      type: "result",
      result: {
        status,
        tests: counts,
        ...(terminationReason ? { termination_reason: terminationReason } : {}),
        ...(this.errors.length ? { error: this.errors[0] } : {}),
        artifacts: this.artifacts,
        interrupted_artifacts: this.interruptedArtifacts,
        capture_state: captureState(
          this.options.capture,
          status,
          this.artifacts,
        ),
      },
    });
  }
};
