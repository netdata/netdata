// SPDX-License-Identifier: GPL-3.0-or-later
import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import extensions from "../runner/extensions.cjs";
import { monotonicMilliseconds } from "../runner/journey.mjs";
import { baselineRecords } from "./contract-2.mjs";

const sourceRoot = fileURLToPath(new URL("../", import.meta.url));
const traceFeature = extensions.FAILURE_TRACE;
const detailsFeature = "capture_details.v1";
const png = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);
const zip = Buffer.from([80, 75, 3, 4]);

test("trace selection requires a capture-enabled journey and an explicit request", () => {
  for (const [kind, capture, wanted] of [
    ["journey", true, [traceFeature]],
    ["journey", false, []],
    ["lighthouse", true, []],
  ])
    assert.deepEqual(
      extensions.selection({ kind, capture, extensions: [traceFeature] }),
      wanted,
    );
  assert.equal(
    extensions.selection({ kind: "journey", capture: true }),
    undefined,
  );
  assert.deepEqual(
    extensions.selection({
      kind: "journey",
      capture: true,
      extensions: [detailsFeature],
    }),
    [detailsFeature],
  );
});

function work(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "dt."));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.mkdirSync(path.join(dir, "output"));
  return dir;
}

function report(t, selected, capture = true) {
  const dir = work(t);
  fs.writeFileSync(path.join(dir, "output/good.png"), png);
  fs.writeFileSync(path.join(dir, "output/good.zip"), zip);
  fs.writeFileSync(path.join(dir, "output/bad.zip"), "not a ZIP");
  fs.writeFileSync(path.join(dir, "outside.zip"), zip);
  fs.symlinkSync(
    path.join(dir, "outside.zip"),
    path.join(dir, "output/escape.zip"),
  );
  fs.mkdirSync(path.join(dir, "output/directory.zip"));
  const program = `
    const Reporter=require(${JSON.stringify(path.join(sourceRoot, "runner/reporter.cjs"))});
    const dir=process.argv[1], selected=JSON.parse(process.argv[2]), capture=process.argv[3]==='true';
    const reporter=new Reporter({workDir:dir,extensions:selected,capture});
    const image={name:'screenshot',contentType:'image/png',path:dir+'/output/good.png'};
    const trace=name=>({name:'trace',contentType:'application/zip',path:dir+'/output/'+name});
    for(const [id,status] of [['failure','failed'],['interrupt','interrupted'],['pass','passed']]) {
      reporter.onTestEnd({id,title:id,expectedStatus:'passed'},{status,duration:1,steps:[],errors:[],attachments:[
        trace('good.zip'),image,trace('bad.zip'),trace('escape.zip'),trace('directory.zip'),trace('missing.zip'),
        {name:'trace',contentType:'application/zip',body:Buffer.from('inline')},
        {...trace('good.zip'),contentType:'text/plain'},
        {name:'screenshot',contentType:'image/png',body:Buffer.from('inline')}
      ]});
    }
    reporter.onEnd({status:'failed'});
  `;
  const result = spawnSync(
    process.execPath,
    ["-e", program, dir, JSON.stringify(selected), String(capture)],
    {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe", "pipe"],
    },
  );
  assert.equal(result.status, 0, result.stderr);
  return result.output[3].trim().split("\n").map(JSON.parse);
}

test("trace candidates reuse test links and never change PNG attachment counts", (t) => {
  const records = report(t, [detailsFeature, traceFeature]);
  const result = records.at(-1).result;
  assert.deepEqual(
    result.artifacts.map((a) => [a.id, a.kind, a.mime]),
    [
      ["trace-1", "trace", "application/zip"],
      ["screenshot-2", "screenshot", "image/png"],
    ],
  );
  assert.deepEqual(
    result.interrupted_artifacts.map((a) => a.id),
    ["trace-3", "screenshot-4"],
  );
  const detail = records
    .filter((r) => r.extension?.feature === detailsFeature)
    .map((r) => r.extension.data);
  assert.deepEqual(
    detail
      .filter((r) => r.record === "test")
      .map((r) => [r.test.id, r.test.accepted, r.test.unaccepted]),
    [
      ["failure", 1, 1],
      ["interrupt", 1, 1],
    ],
  );
  assert.deepEqual(
    detail.filter((r) => r.record === "capture").map((r) => [r.id, r.test_id]),
    [
      ["trace-1", "failure"],
      ["screenshot-2", "failure"],
      ["trace-3", "interrupt"],
      ["screenshot-4", "interrupt"],
    ],
  );
});

test("unrequested trace attachments never reach the frozen baseline", (t) => {
  const records = report(t, []);
  assert.deepEqual(
    baselineRecords(records.map(JSON.stringify).join("\n")),
    records,
  );
  assert.equal(records.at(-1).result.artifacts.length, 1);
  assert.equal(records.at(-1).result.artifacts[0].kind, "screenshot");
  const disabled = report(t, [traceFeature, detailsFeature], false);
  assert.equal(disabled.at(-1).result.capture_state, "disabled");
  assert.equal(disabled.at(-1).result.artifacts.length, 0);
});

async function realRun(t, script, options = {}) {
  const root = work(t);
  const dependencies = process.env.DEM_TEST_DEPENDENCIES || sourceRoot;
  fs.cpSync(path.join(sourceRoot, "runner"), path.join(root, "runner"), {
    recursive: true,
  });
  fs.symlinkSync(dependencies, path.join(root, "dependencies"));
  fs.symlinkSync(process.env.DEM_TEST_BROWSER, path.join(root, "chrome"));
  fs.writeFileSync(
    path.join(root, "runtime.json"),
    JSON.stringify({
      contract: 2,
      version: "test-traces",
      paths: { dependencies: "dependencies", browser: "chrome" },
    }),
  );
  const dir = work(t);
  const request = {
    version: 2,
    kind: "journey",
    run_id: "trace-test",
    work_dir: dir,
    script: "import {test,expect} from '@playwright/test';\n" + script,
    timeout_ms: 15000,
    stop_at_monotonic_ms: monotonicMilliseconds() + 20000,
    capture: true,
    extensions: ["failure_evidence.v1", detailsFeature, traceFeature],
    ...options,
  };
  const child = spawn(process.execPath, [path.join(root, "runner/run.mjs")], {
    stdio: ["pipe", "pipe", "pipe"],
    env: { ...process.env, NODE_OPTIONS: "" },
  });
  let stdout = "",
    stderr = "";
  child.stdout.on("data", (b) => (stdout += b));
  child.stderr.on("data", (b) => (stderr += b));
  child.stdin.end(JSON.stringify(request));
  const code = await new Promise((resolve, reject) => {
    child.on("error", reject);
    child.on("close", resolve);
  });
  assert.equal(stderr, "");
  const records = stdout.trim().split("\n").map(JSON.parse);
  const terminals = records.filter((r) => r.type === "result");
  assert.equal(terminals.length, 1, stdout);
  const config = fs.readFileSync(
    path.join(dir, "playwright.config.cjs"),
    "utf8",
  );
  return { code, records, result: terminals[0].result, dir, config };
}

const browser = {
  skip:
    !process.env.DEM_TEST_BROWSER &&
    "set DEM_TEST_BROWSER to the pinned sandbox browser",
};

test(
  "native Test traces retain per-test association across contexts and status policies",
  browser,
  async (t) => {
    const actual = await realRun(
      t,
      `
    test('two contexts',async({page,browser})=>{
      await page.setContent('<h1>First page</h1>');
      const other=await browser.newContext();const extra=await other.newPage();await extra.setContent('<h1>Second page</h1>');await other.close();
      expect(1).toBe(2);
    });
    test('later failure',async({page})=>{await page.setContent('<h1>Later</h1>');expect(1).toBe(2)});
    test('pass',async({page})=>{await page.setContent('<h1>Pass</h1>')});
    test('expected failure',async({page})=>{test.fail();await page.setContent('<h1>Expected</h1>');expect(1).toBe(2)});
    test('unexpected pass',async({page})=>{test.fail();await page.setContent('<h1>Unexpected</h1>')});
  `,
    );
    assert.equal(actual.result.status, "failed");
    assert.deepEqual(actual.result.tests, {
      declared: 5,
      passed: 1,
      failed: 2,
      timed_out: 0,
      skipped: 0,
      expected_failure: 2,
      not_run: 0,
    });
    const traces = actual.result.artifacts.filter((a) => a.kind === "trace");
    assert.equal(traces.length, 2);
    assert.equal(
      new Set(actual.result.artifacts.map((a) => a.id)).size,
      actual.result.artifacts.length,
    );
    for (const a of traces) {
      assert.equal(a.mime, "application/zip");
      assert(
        fs
          .readFileSync(path.join(actual.dir, a.path))
          .subarray(0, 4)
          .equals(zip),
      );
    }
    const detail = actual.records
      .filter((r) => r.extension?.feature === detailsFeature)
      .map((r) => r.extension.data);
    const tests = detail.filter((r) => r.record === "test").map((r) => r.test);
    const links = detail.filter((r) => r.record === "capture");
    const failingTests = tests.filter(
      (test) =>
        test.title.endsWith("two contexts") ||
        test.title.endsWith("later failure"),
    );
    assert.equal(failingTests.length, 2);
    assert.deepEqual(
      new Set(traces.map((a) => links.find((l) => l.id === a.id)?.test_id)),
      new Set(failingTests.map((test) => test.id)),
    );
    assert.equal(
      tests.reduce((n, t) => n + t.accepted, 0),
      actual.result.artifacts.filter((a) => a.kind === "screenshot").length,
    );
    assert(
      actual.config.includes('"sources":false') &&
        actual.config.includes('"attachments":false'),
    );
  },
);

test(
  "native timeouts and budget interruptions retain separate trace candidates",
  browser,
  async (t) => {
    for (const interrupted of [false, true]) {
      const actual = await realRun(
        t,
        `test('wait',async({page})=>{test.setTimeout(${interrupted ? 0 : 2500});await page.setContent('<h1>Waiting</h1>');await page.locator('#never').click()});`,
        interrupted
          ? { stop_at_monotonic_ms: monotonicMilliseconds() + 5000 }
          : {},
      );
      assert.equal(actual.result.status, "timeout");
      assert.equal(
        actual.result.termination_reason,
        interrupted ? "execution_budget" : "test_timeout",
      );
      const candidates = interrupted
        ? actual.result.interrupted_artifacts
        : actual.result.artifacts;
      assert.deepEqual(candidates.map((a) => a.kind).sort(), [
        "screenshot",
        "trace",
      ]);
      if (interrupted) assert.equal(actual.result.artifacts.length, 0);
    }
  },
);

test(
  "baseline requests configure trace off and discard script-requested trace attachments",
  browser,
  async (t) => {
    const actual = await realRun(
      t,
      `test.use({trace:'on'});test('failure',async({page})=>{await page.setContent('<h1>Failed</h1>');expect(1).toBe(2)});`,
      { extensions: undefined },
    );
    baselineRecords(actual.records.map(JSON.stringify).join("\n"));
    assert(actual.config.includes('"trace":"off"'));
    const files = fs.readdirSync(path.join(actual.dir, "output"), {
      recursive: true,
    });
    assert(files.some((file) => path.basename(file) === "trace.zip"));
    assert.deepEqual(
      actual.result.artifacts.map((a) => a.kind),
      ["screenshot"],
    );
  },
);
