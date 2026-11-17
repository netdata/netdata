// SPDX-License-Identifier: GPL-3.0-or-later
import test, { afterEach } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import Details from "../runner/capture-details.cjs";
import protocol from "../runner/protocol.cjs";
import extensions from "../runner/extensions.cjs";
import { baselineRecords } from "./contract-2.mjs";

afterEach(() => protocol.configureSecrets({}));
const subject = (id) => ({ id, title: "Checkout", expectedStatus: "passed" });
const failed = { status: "failed" };
function collector() {
  const records = [];
  const feature = new Details((r) => records.push(r.extension.data));
  return { feature, records };
}

test("selection requires a capture-enabled journey and an explicit request", () => {
  for (const [kind, capture, wanted] of [
    ["journey", true, [Details.FEATURE]],
    ["journey", false, []],
    ["lighthouse", true, []],
  ]) {
    assert.deepEqual(
      extensions.selection({ kind, capture, extensions: [Details.FEATURE] }),
      wanted,
    );
  }
  assert.equal(
    extensions.selection({ kind: "journey", capture: true }),
    undefined,
  );
  assert.deepEqual(
    extensions.selection({
      kind: "journey",
      capture: true,
      extensions: ["failure_evidence.v1"],
    }),
    ["failure_evidence.v1"],
  );
});

test("one detail budget accommodates multi-page suites and explicitly ends coverage", () => {
  const { feature, records } = collector();
  for (let t = 0; t < 32; t++)
    feature.test(
      subject("t" + t),
      failed,
      Array.from({ length: 4 }, (_, i) => ({
        id: `s${t}_${i}`,
        kind: "screenshot",
      })),
      0,
      0,
    );
  assert.equal(records.length, 160);
  assert(!records.some((r) => r.record === "truncated"));
  for (let t = 32; t < 500; t++)
    feature.test(
      subject("t" + t),
      failed,
      [{ id: "s" + t, kind: "screenshot" }],
      0,
      1,
    );
  feature.complete();
  feature.complete();
  feature.test(subject("late"), failed, [], 0, 0);
  assert.equal(
    records.filter((r) => ["test", "capture"].includes(r.record)).length,
    Details.LIMIT,
  );
  assert.equal(records.filter((r) => r.record === "truncated").length, 1);
  assert.equal(records.filter((r) => r.record === "complete").length, 1);
  assert.equal(records.at(-1).record, "complete");
});

test("test titles are redacted before clipping and oversized identity is omitted explicitly", () => {
  protocol.configureSecrets({ token: "synthetic-secret" });
  const { feature, records } = collector();
  feature.test(
    {
      ...subject("t1"),
      titlePath: () => ["Suite", "synthetic-secret " + "x".repeat(400)],
    },
    failed,
    [],
    0,
    0,
  );
  assert(records[0].test.title.includes("[REDACTED]"));
  assert(!records[0].test.title.includes("synthetic-secret"));
  assert.equal(records[0].test.text_truncated, true);
  feature.test(
    subject("x".repeat(128)),
    failed,
    [{ id: "s1", kind: "screenshot" }],
    0,
    0,
  );
  assert.equal(records.at(-1).record, "truncated");
  assert(!records.some((r) => r.record === "capture"));
});

function report(t, selected, capture = true) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "dem-captures-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.mkdirSync(path.join(dir, "output"));
  fs.writeFileSync(
    path.join(dir, "output", "good.png"),
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
  );
  fs.writeFileSync(path.join(dir, "output", "bad.png"), "not a PNG");
  const program = `
    const Reporter=require(${JSON.stringify(fileURLToPath(new URL("../runner/reporter.cjs", import.meta.url)))});
    const dir=process.argv[1], capture=process.argv[2]==="true", selected=JSON.parse(process.argv[3]);
    const reporter=new Reporter({workDir:dir,capture,extensions:selected});
    for(const [id,status,expectedStatus] of [["mixed","failed","passed"],["absent","failed","passed"],["expected","failed","failed"],["timed","timedOut","passed"],["interrupted","interrupted","passed"],["expected-shot","failed","failed"],["unexpected","passed","failed"]]) {
      const image={name:"screenshot",contentType:"image/png",path:dir+"/output/good.png"};
      const attachments=id==="mixed" ? [
        image,
        {name:"screenshot",contentType:"image/png",path:dir+"/output/bad.png"},
        {name:"screenshot",contentType:"image/png",body:Buffer.from("image")},
        image
      ] : ["timed","interrupted","expected-shot"].includes(id) ? [image] : [];
      reporter.onTestEnd({id,title:id,expectedStatus},{status,duration:1,errors:status==="failed"?[{message:"assertion"}]:[],steps:[],attachments});
    }
    reporter.onEnd({status:"failed"});
  `;
  const r = spawnSync(
    process.execPath,
    ["-e", program, dir, String(capture), JSON.stringify(selected)],
    { encoding: "utf8", stdio: ["ignore", "pipe", "pipe", "pipe"] },
  );
  assert.equal(r.status, 0, r.stderr);
  return r.output[3].trim().split("\n").map(JSON.parse);
}

test("real reporter preserves candidate authority and exposes mixed attachment rejection", (t) => {
  const records = report(t, [Details.FEATURE]);
  const detail = records
    .filter((r) => r.type === "extension")
    .map((r) => r.extension.data);
  const observations = detail
    .filter((r) => r.record === "test")
    .map((r) => r.test);
  assert.deepEqual(
    observations.map((r) => [
      r.id,
      r.status,
      r.expected_status,
      r.accepted,
      r.unaccepted,
    ]),
    [
      ["mixed", "failed", "passed", 2, 2],
      ["absent", "failed", "passed", 0, 0],
      ["expected", "failed", "failed", 0, 0],
      ["timed", "timedOut", "passed", 1, 0],
      ["interrupted", "interrupted", "passed", 1, 0],
      ["expected-shot", "failed", "failed", 1, 0],
    ],
  );
  assert.deepEqual(
    detail.filter((r) => r.record === "capture"),
    [
      { record: "capture", id: "screenshot-1", test_id: "mixed" },
      { record: "capture", id: "screenshot-2", test_id: "mixed" },
      { record: "capture", id: "screenshot-3", test_id: "timed" },
      { record: "capture", id: "screenshot-4", test_id: "interrupted" },
      { record: "capture", id: "screenshot-5", test_id: "expected-shot" },
    ],
  );
  const { artifacts, interrupted_artifacts: interrupted } =
    records.at(-1).result;
  assert.deepEqual(
    artifacts.map((a) => a.id),
    ["screenshot-1", "screenshot-2", "screenshot-3", "screenshot-5"],
  );
  assert.deepEqual(
    interrupted.map((a) => a.id),
    ["screenshot-4"],
  );
  assert(
    [...artifacts, ...interrupted].every((a) => !Object.hasOwn(a, "test")),
  );
  const core = records.filter((r) => r.type !== "extension");
  baselineRecords(core.map(JSON.stringify).join("\n") + "\n");
  const normalize = (rows) =>
    rows.map((r) => (r.event ? { ...r, event: { ...r.event, at_ms: 0 } } : r));
  const without = report(t, []);
  // Private work roots differ; compare the relative core candidate paths.
  assert.deepEqual(normalize(core), normalize(without));
  const disabled = report(t, [Details.FEATURE], false);
  assert(!disabled.some((r) => r.type === "extension"));
  assert.equal(disabled.at(-1).result.capture_state, "disabled");
});
