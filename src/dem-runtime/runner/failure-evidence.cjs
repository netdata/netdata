// SPDX-License-Identifier: GPL-3.0-or-later
"use strict";
const path = require("node:path");
const { redact } = require("./protocol.cjs");

const FEATURE = "failure_evidence.v1";
// Match encoding/json's UTF-8 and HTML escaping when enforcing history bounds.
const encoded = (value) =>
  Buffer.byteLength(
    JSON.stringify(value).replace(
      /[<>&\u2028\u2029]/g,
      (char) => "\\u" + char.charCodeAt(0).toString(16).padStart(4, "0"),
    ),
  );
function clipped(value, limit, characters = Infinity) {
  const safe = redact(value);
  let result = "",
    bytes = 2,
    count = 0;
  for (const char of safe) {
    const size = encoded(char) - 2;
    if (bytes + size > limit || count === characters)
      return { value: result, truncated: true };
    bytes += size;
    count++;
    result += char;
  }
  return { value: result, truncated: false };
}
const duration = (value) =>
  Number.isFinite(value) && value >= 0 ? { duration_ms: value } : {};
function sameError(left, right) {
  if (left === right) return true;
  const message = (error) => String(error?.message || error?.value || "");
  // A caught assertion with the same message at another source is not the failure.
  return (
    message(left) !== "" &&
    message(left) === message(right) &&
    typeof left?.stack === "string" &&
    left.stack !== "" &&
    left.stack === right?.stack
  );
}

module.exports = class FailureEvidence {
  constructor(options, emit) {
    this.emit = emit;
    this.view =
      options.stepView || require("./step-view.cjs")(options.observerFile);
    this.sourceDir = options.sourceDir || options.workDir || process.cwd();
    this.tests = new Map();
    this.steps = new Map();
    this.ids = new WeakMap();
    this.nextID = 0;
    this.userSteps = 0;
    this.otherSteps = 0;
    this.truncated = new Set();
    this.failure = false;
    this.stopped = false;
    this.pendingStop = undefined;
    this.finished = false;
  }
  send(data) {
    if (this.finished) return;
    // Candidate snapshots may be discarded; only published items announce loss.
    const item = data.test || data.step || data.failure || data.stop;
    if (item?.text_truncated) this.truncate("text");
    this.emit({ type: "extension", extension: { feature: FEATURE, data } });
  }
  truncate(scope) {
    if (this.truncated.has(scope)) return;
    this.truncated.add(scope);
    this.send({ record: "truncated", scope });
  }
  string(value, limit, target, key, characters) {
    const result = clipped(value, limit, characters);
    target[key] = result.value;
    if (result.truncated) {
      target.text_truncated = true;
    }
  }
  location(value, target) {
    if (
      !value ||
      !Number.isSafeInteger(value.line) ||
      value.line <= 0 ||
      !Number.isSafeInteger(value.column) ||
      value.column <= 0 ||
      typeof value.file !== "string"
    )
      return;
    const relative = path.relative(this.sourceDir, value.file);
    const file =
      !relative ||
      /[:\\]/.test(relative) ||
      path.isAbsolute(relative) ||
      relative === ".." ||
      relative.startsWith(".." + path.sep)
        ? "[external]"
        : relative.split(path.sep).join("/");
    const result = clipped(file, 256);
    // Clipping a filename could manufacture a different source identity.
    target.location = {
      file: result.truncated ? "[external]" : result.value,
      line: value.line,
      column: value.column,
    };
    if (result.truncated) {
      target.text_truncated = true;
    }
  }
  testValue(test, result) {
    const value = {
      id: test.id,
      status: result?.status || "running",
      expected_status: test.expectedStatus,
      reporting_complete: Boolean(result),
      ...(result ? duration(result.duration) : {}),
    };
    this.string(
      typeof test.titlePath === "function"
        ? test.titlePath().filter(Boolean).join(" › ")
        : test.title,
      256,
      value,
      "title",
    );
    this.location(test.location, value);
    return value;
  }
  test(test, result) {
    const existing = this.tests.get(test.id);
    if (!existing && (this.tests.size === 64 || encoded(test.id) > 128)) {
      this.truncate("tests");
      return;
    }
    if (existing?.reporting_complete) return;
    const value = existing
      ? {
          ...existing,
          status: result?.status || "running",
          expected_status: test.expectedStatus,
          reporting_complete: Boolean(result),
          ...(result ? duration(result.duration) : {}),
        }
      : this.testValue(test, result);
    if (!existing && encoded(value) > 1024) {
      this.truncate("tests");
      return;
    }
    this.tests.set(test.id, value);
    this.send({ record: "test", test: value });
  }
  id(step) {
    let value = this.ids.get(step);
    if (!value) {
      value = "s" + ++this.nextID;
      this.ids.set(step, value);
    }
    return value;
  }
  stepValue(test, step) {
    const value = { id: this.id(step), test_id: test.id, state: "running" };
    this.string(step.title, 256, value, "title");
    // Playwright's controlled category is protocol metadata, not user text. In
    // particular, redacting a short secret must not move test.step to another pool.
    if (
      [
        "test.step",
        "expect",
        "fixture",
        "hook",
        "pw:api",
        "test.attach",
      ].includes(step.category)
    )
      value.category = step.category;
    else this.string(step.category, 64, value, "category");
    this.location(step.location, value);
    const parent = this.view.parent(step);
    if (parent) {
      value.parent_id = this.id(parent);
      if (!this.steps.has(value.parent_id)) value.parent_omitted = true;
    }
    return value;
  }
  step(test, step, end = false) {
    const knownID = this.ids.get(step),
      existing = this.steps.get(knownID);
    if (existing) {
      if (!end || existing.state !== "running") return;
      const value = {
        ...existing,
        state: step.error ? "failed" : "passed",
        ...duration(step.duration),
      };
      if (step.error)
        this.string(
          step.error.message || step.error.value,
          1024,
          value,
          "message",
        );
      // Admission reserves space for the largest end update; never drop it.
      this.steps.set(knownID, value);
      this.send({ record: "step", step: value });
      return;
    }
    // End-only steps are not new admissions. Playwright supplies onStepBegin.
    if (end) return;
    const user = step.category === "test.step",
      scope = user ? "user_steps" : "other_steps";
    if (
      (user ? this.userSteps === 128 : this.otherSteps === 64) ||
      encoded(test.id) > 128
    ) {
      this.truncate(scope);
      return;
    }
    const value = this.stepValue(test, step);
    if (
      encoded({
        ...value,
        state: "failed",
        duration_ms: Number.MAX_VALUE,
        message: "x".repeat(1022),
        text_truncated: true,
      }) > 2048
    ) {
      if (user) this.userSteps = 128;
      else this.otherSteps = 64;
      this.truncate(scope);
      return;
    }
    this.steps.set(value.id, value);
    if (user) this.userSteps++;
    else this.otherSteps++;
    this.send({ record: "step", step: value });
  }
  firstFailure(test, result) {
    if (
      this.failure ||
      !(
        result.status === "timedOut" ||
        (result.status === "failed" && test.expectedStatus !== "failed") ||
        (result.status === "passed" && test.expectedStatus === "failed")
      )
    )
      return;
    this.failure = true;
    const error = result.errors?.[0];
    const selection = this.matchingPath(
      result.steps || [],
      (step) => step.error && error && sameError(step.error, error),
    );
    const failure = {
      test: this.testValue(test, result),
      ...this.pathDetail(test, selection),
    };
    this.string(
      error?.message || error?.value || "",
      12002,
      failure,
      "message",
      2000,
    );
    this.location(
      error?.location || selection.matchedPath.at(-1)?.location,
      failure,
    );
    if (
      failure.path.some((node) => node.text_truncated) ||
      failure.test.text_truncated
    )
      failure.text_truncated = true;
    this.send({ record: "failure", failure });
  }
  matchingPath(steps, matches) {
    let matchedPath = [],
      ambiguous = false,
      pathIncomplete = false;
    // Bound temporary ancestry even for arbitrarily deep user-authored steps.
    // Skipping a deeper subtree is explicit; the actual first error survives.
    const cursors = [{ iterator: this.view.children(steps), step: undefined }];
    while (cursors.length) {
      const cursor = cursors.at(-1);
      const next = cursor.iterator.next();
      if (next.done) {
        pathIncomplete ||= cursor.iterator.incomplete;
        cursors.pop();
        continue;
      }
      const step = (cursor.step = next.value);
      if (matches(step)) {
        const candidate = cursors.map((cursor) => cursor.step);
        let shared = 0;
        while (
          shared < matchedPath.length &&
          shared < candidate.length &&
          matchedPath[shared] === candidate[shared]
        )
          shared++;
        if (!ambiguous && shared === matchedPath.length)
          matchedPath = candidate;
        else {
          // Repeated assertions can share message and stack. Preserve only
          // common ancestry instead of attributing the error to a caught sibling.
          matchedPath.length = shared;
          ambiguous = true;
          pathIncomplete = true;
        }
      }
      if (step.steps?.length) {
        if (cursors.length === 64) pathIncomplete = true;
        else
          cursors.push({
            iterator: this.view.children(step.steps),
            step: undefined,
          });
      }
    }
    return { matchedPath, pathIncomplete };
  }
  pathDetail(test, { matchedPath, pathIncomplete }) {
    const first = [],
      last = [];
    let ancestors = 0;
    const matched = matchedPath.at(-1);
    for (let step = matched; step; step = this.view.parent(step)) {
      if (last.length < 8) last.push(step);
      else {
        first.push(step);
        if (first.length > 8) first.shift();
      }
      ancestors++;
    }
    const nodes = [...first.reverse(), ...last.reverse()].map((step) => ({
      ...this.stepValue(test, step),
      state:
        step.duration === -1 ? "running" : step.error ? "failed" : "passed",
    }));
    for (let i = 0; i < nodes.length; i++) {
      const previous = nodes[i - 1];
      if (nodes[i].parent_id && nodes[i].parent_id !== previous?.id)
        nodes[i].parent_omitted = true;
      else delete nodes[i].parent_omitted;
    }
    return {
      path: nodes,
      omitted_ancestors: Math.max(0, ancestors - nodes.length),
      ...(pathIncomplete ? { path_incomplete: true } : {}),
    };
  }
  beforeCleanup(test, result, step) {
    // Playwright 1.63 begins this root hook before user cleanup and fixture teardown.
    // Its public result tree still includes pending steps omitted by our detail caps.
    if (
      this.stopped ||
      this.view.parent(step) ||
      step.category !== "hook" ||
      step.title !== "After Hooks"
    )
      return;
    const selection = this.matchingPath(
      result.steps || [],
      (candidate) => candidate !== step && candidate.duration === -1,
    );
    if (!selection.matchedPath.length) selection.pathIncomplete = true;
    this.pendingStop = { testID: test.id, ...this.pathDetail(test, selection) };
  }
  stopContext(test, result) {
    const pending = this.pendingStop;
    this.pendingStop = undefined;
    if (this.stopped || !["timedOut", "interrupted"].includes(result.status))
      return;
    this.stopped = true;
    const stop = {
      test: this.testValue(test, result),
      ...(pending?.testID === test.id
        ? {
            path: pending.path,
            omitted_ancestors: pending.omitted_ancestors,
            ...(pending.path_incomplete ? { path_incomplete: true } : {}),
          }
        : { path: [], omitted_ancestors: 0, path_incomplete: true }),
      message: "",
    };
    if (
      stop.test.text_truncated ||
      stop.path.some((node) => node.text_truncated)
    )
      stop.text_truncated = true;
    // This is an observation before cleanup, never an assertion about the stop cause.
    this.send({ record: "stop", stop });
  }
  complete() {
    if (!this.finished) {
      this.send({ record: "complete" });
      this.finished = true;
    }
  }
};
module.exports.FEATURE = FEATURE;
module.exports.encoded = encoded;
module.exports.clipped = clipped;
