// SPDX-License-Identifier: GPL-3.0-or-later
"use strict";
const { FEATURE } = require("./failure-evidence.cjs");
const { FEATURE: BROWSER_SUMMARY } = require("./browser-summary.cjs");
const { FEATURE: CAPTURE_DETAILS } = require("./capture-details.cjs");

const FAILURE_TRACE = "failure_trace.v1";

function selection(request) {
  if (
    !Array.isArray(request.extensions) ||
    !request.extensions.every(
      (value) => typeof value === "string" && value.length,
    )
  )
    return undefined;
  return request.kind === "journey"
    ? [FEATURE, BROWSER_SUMMARY, CAPTURE_DETAILS, FAILURE_TRACE].filter(
        (feature) =>
          request.extensions.includes(feature) &&
          (![CAPTURE_DETAILS, FAILURE_TRACE].includes(feature) ||
            request.capture),
      )
    : [];
}
function family(record) {
  if (!record || Array.isArray(record) || typeof record !== "object")
    throw new Error("invalid reporter record");
  const member = record.type;
  if (
    !["event", "result", "extension", "extension_init"].includes(member) ||
    Object.keys(record).some((key) => key !== "type" && key !== member)
  )
    throw new Error("invalid or mixed reporter record");
  return member;
}
// Optional bodies belong to the consumer's best-effort validator. Shared framing
// and mixed envelopes remain errors even when the optional feature is unselected.
function optional(record, selected, terminal) {
  const kind = family(record);
  if (kind !== "extension" && kind !== "extension_init") return false;
  if (kind === "extension_init" && selected && !terminal) return record;
  if (kind === "extension" && selected?.length && !terminal) {
    const feature = record.extension?.feature;
    if (selected.includes(feature) || typeof feature !== "string")
      return record;
  }
  return true;
}
module.exports = { selection, family, optional, FAILURE_TRACE };
