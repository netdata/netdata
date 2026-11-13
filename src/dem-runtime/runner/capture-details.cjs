// SPDX-License-Identifier: GPL-3.0-or-later
"use strict";
const { clipped, encoded } = require("./failure-evidence.cjs");
const FEATURE = "capture_details.v1";
// One optional-detail budget, independent of capture publication. Allows e.g.
// 32 failed tests with four screenshots each (160 test/link records).
const LIMIT = 256;

module.exports = class CaptureDetails {
  constructor(emit) {
    this.emit = emit;
    this.records = 0;
    this.truncated = false;
    this.finished = false;
  }
  send(data) {
    this.emit({ type: "extension", extension: { feature: FEATURE, data } });
  }
  truncate() {
    if (!this.truncated) {
      this.truncated = true;
      this.send({ record: "truncated" });
    }
  }
  detail(data) {
    if (this.records === LIMIT) {
      this.truncate();
      return false;
    }
    this.records++;
    this.send(data);
    return true;
  }
  test(test, result, candidates, first, unaccepted) {
    if (this.finished) return;
    if (this.records === LIMIT) {
      this.truncate();
      return;
    }
    if (encoded(test.id) > 128) {
      this.truncate();
      return;
    }
    const title = clipped(
      typeof test.titlePath === "function"
        ? test.titlePath().filter(Boolean).join(" › ")
        : test.title,
      256,
    );
    let accepted = 0;
    for (let i = first; i < candidates.length; i++)
      if (candidates[i].kind === "screenshot") accepted++;
    if (
      !this.detail({
        record: "test",
        test: {
          id: test.id,
          title: title.value,
          ...(title.truncated ? { text_truncated: true } : {}),
          status: result.status,
          expected_status: test.expectedStatus,
          accepted,
          unaccepted,
        },
      })
    )
      return;
    for (let i = first; i < candidates.length; i++) {
      if (
        !this.detail({
          record: "capture",
          id: candidates[i].id,
          test_id: test.id,
        })
      )
        break;
    }
  }
  complete() {
    if (!this.finished) {
      this.send({ record: "complete" });
      this.finished = true;
    }
  }
};
module.exports.FEATURE = FEATURE;
module.exports.LIMIT = LIMIT;
