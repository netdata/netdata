// SPDX-License-Identifier: GPL-3.0-or-later
"use strict";
const { encoded, clipped } = require("./failure-evidence.cjs");
const { redact } = require("./protocol.cjs");

const FEATURE = "browser_summary.v1",
  ANNOTATION = "netdata.dem.browser_summary";
const categories = ["uncaught", "console_error", "transport", "http_error"];
const workerFlags = [
  "args_limit",
  "rpc_budget",
  "candidate_limit",
  "pending",
  "unavailable",
  "too_long",
];
const argumentReasons = [
  "object",
  "symbol",
  "function",
  "unrepresentable",
  "non_finite",
  "too_long",
];
const counts = () => Object.fromEntries(categories.map((key) => [key, 0]));
const object = (value) =>
  value !== null && typeof value === "object" && !Array.isArray(value);
const safeInteger = (value) => Number.isSafeInteger(value) && value >= 0;
const boundedText = (value, limit) =>
  typeof value === "string" && encoded(value) <= limit;

function safeURL(value) {
  try {
    const url = new URL(redact(value));
    url.username = "";
    url.password = "";
    url.search = "";
    url.hash = "";
    return url.toString();
  } catch {
    return "";
  }
}
function safeText(value) {
  return redact(value).replace(/https?:\/\/[^\s<>"']+/gi, safeURL);
}
function metadata(value, limit, result, key) {
  if (!boundedText(value, limit)) throw new Error("invalid annotation text");
  const safe = clipped(safeText(value), limit);
  result[key] = safe.value;
  if (safe.truncated) result.text_truncated = true;
}
function readSample(input) {
  if (!object(input) || !categories.includes(input.category))
    throw new Error("invalid sample");
  const value = { category: input.category };
  if (
    input.text_truncated !== undefined &&
    typeof input.text_truncated !== "boolean"
  )
    throw new Error("invalid truncation flag");
  if (input.text_truncated) value.text_truncated = true;
  if (input.category === "console_error") {
    if (
      !["resolved", "pending", "unavailable"].includes(input.state) ||
      input.text_truncated !== undefined
    )
      throw new Error("invalid console state");
    value.state = input.state;
    if (input.text_omitted !== undefined) {
      if (input.state !== "unavailable" || input.text_omitted !== "too_long")
        throw new Error("invalid text omission");
      value.text_omitted = "too_long";
    }
    if (input.args_omitted !== undefined) {
      if (!safeInteger(input.args_omitted))
        throw new Error("invalid omitted arguments");
      if (input.args_omitted) value.args_omitted = input.args_omitted;
    }
    if (input.state !== "resolved") {
      if (input.args !== undefined || input.text !== undefined)
        throw new Error("unresolved arguments");
    } else if (input.text !== undefined) {
      if (input.args !== undefined || !boundedText(input.text, 512))
        throw new Error("invalid console text");
      const text = safeText(input.text);
      if (encoded(text) <= 512) value.text = text;
      else {
        value.state = "unavailable";
        value.text_omitted = "too_long";
      }
    } else {
      if (
        !Array.isArray(input.args) ||
        input.args.length < 1 ||
        input.args.length > 4
      )
        throw new Error("invalid arguments");
      value.args = input.args.map((item) => {
        if (
          !object(item) ||
          Object.hasOwn(item, "value") === Object.hasOwn(item, "omitted")
        )
          throw new Error("invalid argument");
        if (Object.hasOwn(item, "omitted")) {
          if (!argumentReasons.includes(item.omitted))
            throw new Error("invalid omission");
          return { omitted: item.omitted };
        }
        if (
          !(
            item.value === null ||
            typeof item.value === "boolean" ||
            (typeof item.value === "number" && Number.isFinite(item.value)) ||
            boundedText(item.value, 512)
          )
        )
          throw new Error("invalid primitive");
        const primitive =
          typeof item.value === "string" ? safeText(item.value) : item.value;
        return encoded(primitive) <= 512
          ? { value: primitive }
          : { omitted: "too_long" };
      });
    }
  } else if (input.category === "uncaught") {
    metadata(input.message, 512, value, "message");
  } else {
    metadata(input.method, 16, value, "method");
    if ((input.url === undefined) === (input.url_omitted === undefined))
      throw new Error("invalid url");
    if (input.url_omitted !== undefined) {
      if (input.url_omitted !== "too_long")
        throw new Error("invalid url omission");
      value.url_omitted = "too_long";
    } else {
      if (!boundedText(input.url, 256)) throw new Error("invalid url length");
      const url = safeURL(input.url);
      if (encoded(url) <= 256) value.url = url;
      else value.url_omitted = "too_long";
    }
    if (input.category === "transport")
      metadata(input.error, 128, value, "error");
    else {
      if (!safeInteger(input.status) || input.status < 400)
        throw new Error("invalid status");
      value.status = input.status;
    }
  }
  return value;
}
function readAnnotation(annotations) {
  const matches = annotations.filter(
    (value) => object(value) && value.type === ANNOTATION,
  );
  if (!matches.length) return { state: "no_summary" };
  if (matches.length !== 1)
    return { state: "invalid", reason: "duplicate_annotation" };
  try {
    const description = matches[0].description;
    if (
      typeof description !== "string" ||
      Buffer.byteLength(description) > 16 * 1024
    )
      throw new Error("annotation limit");
    const input = JSON.parse(description);
    if (!object(input) || input.v !== 1)
      throw new Error("invalid annotation version");
    if (input.state === "open") return { state: "incomplete" };
    if (input.state === "invalid") {
      if (!["counter_overflow", "annotation_limit"].includes(input.reason))
        throw new Error("invalid producer reason");
      return { state: "producer_invalid", reason: input.reason };
    }
    if (
      input.state !== "observed" ||
      !object(input.counts) ||
      !categories.every((key) => safeInteger(input.counts[key])) ||
      !safeInteger(input.rpc_calls) ||
      input.rpc_calls > 3 ||
      !Array.isArray(input.samples) ||
      input.samples.length > 12 ||
      !Array.isArray(input.omissions) ||
      input.omissions.some((value) => !workerFlags.includes(value)) ||
      new Set(input.omissions).size !== input.omissions.length
    )
      throw new Error("invalid observed annotation");
    const values = input.samples.map(readSample);
    for (const category of categories) {
      const size = values.filter((value) => value.category === category).length;
      if (size > 3 || size > input.counts[category])
        throw new Error("invalid candidate count");
    }
    return {
      state: "observed",
      counts: Object.fromEntries(
        categories.map((key) => [key, input.counts[key]]),
      ),
      samples: values,
      omissions: input.omissions,
    };
  } catch {
    return { state: "invalid", reason: "invalid_annotation" };
  }
}

class BrowserSummary {
  constructor(emit) {
    this.emit = emit;
    this.run = {
      tests: { observed: 0, incomplete: 0, invalid: 0, no_summary: 0 },
      counts: counts(),
    };
    this.omissions = new Set();
    this.tests = new Set();
    this.samples = [];
    this.truncated = new Set();
    this.finished = false;
  }
  send(data) {
    if (this.finished) return;
    this.emit({ type: "extension", extension: { feature: FEATURE, data } });
  }
  aggregate() {
    return {
      ...this.run,
      ...(this.omissions.size ? { omissions: [...this.omissions] } : {}),
    };
  }
  truncate(scope) {
    if (this.truncated.has(scope)) return;
    this.truncated.add(scope);
    this.send({ record: "truncated", scope, run: this.aggregate() });
  }
  invalid(reason) {
    this.send({ record: "invalid", reason });
    this.finished = true;
  }
  test(test) {
    if (this.finished) return;
    // Playwright owns exactly one test-end callback (retries/repetitions are
    // disabled). Remember only retained identities, not all clean test IDs.
    const annotation = readAnnotation(test.annotations || []);
    if (annotation.state === "producer_invalid") {
      this.invalid(annotation.reason);
      return;
    }
    const next = {
      tests: { ...this.run.tests },
      counts: { ...this.run.counts },
    };
    next.tests[annotation.state]++;
    if (annotation.state === "observed") {
      for (const category of categories)
        next.counts[category] += annotation.counts[category];
    }
    if (
      !Object.values(next.counts).every(safeInteger) ||
      !safeInteger(Object.values(next.tests).reduce((a, b) => a + b, 0))
    ) {
      this.invalid("counter_overflow");
      return;
    }
    this.run = next;
    for (const flag of annotation.omissions || []) this.omissions.add(flag);
    const interesting =
      annotation.state !== "no_summary" &&
      (annotation.state !== "observed" ||
        Object.values(annotation.counts).some(Boolean));
    if (!interesting) return;
    const rowAvailable = this.tests.size < 64;
    if (!rowAvailable) this.omissions.add("row_limit");
    const identity =
      typeof test.id === "string" &&
      /^[\w-]+$/.test(test.id) &&
      encoded(test.id) <= 128;
    const introduced = [];
    for (const candidate of annotation.samples || []) {
      const key = JSON.stringify(candidate);
      if (this.samples.some((value) => value.key === key)) continue;
      const size = this.samples.filter(
        (value) => value.sample.category === candidate.category,
      ).length;
      if (!rowAvailable && size) {
        this.omissions.add("row_limit");
        continue;
      }
      if (size === 4) {
        this.omissions.add("pool_full");
        continue;
      }
      if (!identity) {
        this.omissions.add("identity_limit");
        continue;
      }
      const value = {
        id: this.samples.length + 1,
        test_id: test.id,
        ...candidate,
      };
      this.samples.push({ key, sample: value });
      introduced.push(value);
    }
    if (!identity) {
      if (rowAvailable) this.omissions.add("identity_limit");
      this.truncate("tests");
    } else if (rowAvailable) {
      this.tests.add(test.id);
      const row = { id: test.id, state: annotation.state };
      if (annotation.counts) row.counts = annotation.counts;
      if (annotation.reason) row.reason = annotation.reason;
      if (introduced.length)
        row.sample_ids = introduced.map((value) => value.id);
      this.send({
        record: "test",
        test: row,
        run: this.aggregate(),
        ...(introduced.length ? { samples: introduced } : {}),
      });
    } else {
      for (const sample of introduced)
        this.send({ record: "late_sample", sample, run: this.aggregate() });
      this.truncate("tests");
    }
    if (
      [...this.omissions].some((flag) =>
        ["candidate_limit", "rpc_budget", "pool_full", "row_limit"].includes(
          flag,
        ),
      )
    )
      this.truncate("samples");
    if (
      introduced.some(
        (value) =>
          value.text_truncated ||
          value.text_omitted ||
          value.url_omitted ||
          value.args?.some((item) => item.omitted === "too_long"),
      )
    )
      this.truncate("text");
  }
  complete() {
    if (this.finished) return;
    this.send({ record: "complete", run: this.aggregate() });
    this.finished = true;
  }
}
module.exports = BrowserSummary;
Object.assign(module.exports, {
  FEATURE,
  ANNOTATION,
  counts,
  safeURL,
  safeText,
  readAnnotation,
});
