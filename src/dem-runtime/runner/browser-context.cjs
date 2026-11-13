// SPDX-License-Identifier: GPL-3.0-or-later
"use strict";
const sample = require("./browser-sampler.cjs");
const Summary = require("./browser-summary.cjs");
const { encoded, clipped } = require("./failure-evidence.cjs");

function observe(context, annotations) {
  const annotation = {
    type: Summary.ANNOTATION,
    description: JSON.stringify({ v: 1, state: "open" }),
  };
  annotations.push(annotation);
  const counts = Summary.counts(),
    candidates = [],
    omissions = {};
  let finalized = false,
    rpcCalls = 0,
    invalid;
  function candidate(category, create) {
    if (finalized || invalid) return;
    if (counts[category] === Number.MAX_SAFE_INTEGER) {
      invalid = "counter_overflow";
      return;
    }
    counts[category]++;
    if (counts[category] > 3) {
      omissions.candidate_limit = true;
      return;
    }
    const value = create();
    value.category = category;
    candidates.push(value);
  }
  function metadata(value, limit, target, key) {
    const bounded = clipped(Summary.safeText(value), limit);
    target[key] = bounded.value;
    if (bounded.truncated) target.text_truncated = true;
  }
  function requestData(request) {
    const value = {};
    metadata(request.method(), 16, value, "method");
    const url = Summary.safeURL(request.url());
    if (encoded(url) <= 256) value.url = url;
    else {
      value.url_omitted = "too_long";
      omissions.too_long = true;
    }
    return value;
  }
  const listeners = {
    weberror: (error) =>
      candidate("uncaught", () => {
        const value = {};
        metadata(error.error().message, 512, value, "message");
        return value;
      }),
    requestfailed: (request) =>
      candidate("transport", () => {
        const value = requestData(request);
        metadata(request.failure()?.errorText || "", 128, value, "error");
        return value;
      }),
    response: (response) => {
      if (response.status() >= 400)
        candidate("http_error", () => ({
          ...requestData(response.request()),
          status: response.status(),
        }));
    },
    console: (message) => {
      if (message.type() !== "error") return;
      candidate("console_error", () => {
        const args = message.args(),
          value = { state: "pending" };
        if (!args.length) {
          const text = Summary.safeText(message.text());
          if (encoded(text) <= 512) return { state: "resolved", text };
          omissions.too_long = true;
          return { state: "unavailable", text_omitted: "too_long" };
        }
        if (args.length > 4) {
          value.args_omitted = args.length - 4;
          omissions.args_limit = true;
        }
        // Three permanent console slots imply at most three evaluations.
        rpcCalls++;
        sample(args).then(
          (values) => {
            if (finalized) return;
            value.state = "resolved";
            value.args = values;
            if (values.some((item) => item.omitted === "too_long"))
              omissions.too_long = true;
          },
          () => {
            if (!finalized) value.state = "unavailable";
          },
        );
        // Keep the same object in the bounded candidate list for async completion.
        return value;
      });
    },
  };
  for (const [name, listener] of Object.entries(listeners))
    context.on(name, listener);
  return () => {
    if (finalized) return;
    finalized = true;
    for (const [name, listener] of Object.entries(listeners))
      context.off(name, listener);
    for (const value of candidates) {
      if (value.state === "pending") omissions.pending = true;
      if (value.state === "unavailable") omissions.unavailable = true;
    }
    const observed = {
      v: 1,
      state: "observed",
      counts,
      samples: candidates,
      omissions: Object.keys(omissions),
      rpc_calls: rpcCalls,
    };
    if (!invalid && encoded(observed) > 16 * 1024) invalid = "annotation_limit";
    annotation.description = JSON.stringify(
      invalid ? { v: 1, state: "invalid", reason: invalid } : observed,
    );
  };
}

module.exports = (test) =>
  test.extend({
    context: [
      async ({ context }, use, testInfo) => {
        const finish = observe(context, testInfo.annotations);
        try {
          await use(context);
        } finally {
          finish();
        }
      },
      { box: "self" },
    ],
  });
module.exports.observe = observe;
