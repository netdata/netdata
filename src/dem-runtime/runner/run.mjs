// SPDX-License-Identifier: GPL-3.0-or-later
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import protocol from "./protocol.cjs";
import extensions from "./extensions.cjs";
import BrowserSummary from "./browser-summary.cjs";
import { runtimePaths, checkRuntime } from "./runtime.mjs";
import {
  monotonicMilliseconds,
  journeyStop,
  journeyResult,
} from "./journey.mjs";
const { text, event, write } = protocol;
const here = path.dirname(fileURLToPath(import.meta.url));
const emit = (record) => write(1, record);
let request;

function absolute(value, name) {
  if (typeof value !== "string" || !path.isAbsolute(value))
    throw new Error(name + " must be an absolute path");
}
function validate(r) {
  if (!r || r.version !== 2 || !["journey", "lighthouse"].includes(r.kind))
    throw new Error("unsupported runner request");
  absolute(r.work_dir, "work_dir");
  if ("dependencies_path" in r || "browser_path" in r)
    throw new Error("runtime component paths are not request fields");
  if (typeof r.run_id !== "string" || !r.run_id)
    throw new Error("run_id is required");
  if (!Number.isSafeInteger(r.timeout_ms) || r.timeout_ms <= 0)
    throw new Error("timeout_ms must be positive");
  if (typeof r.capture !== "boolean")
    throw new Error("capture must be boolean");
  if (r.kind === "journey") {
    if (
      !Number.isSafeInteger(r.stop_at_monotonic_ms) ||
      r.stop_at_monotonic_ms <= 0
    )
      throw new Error("stop_at_monotonic_ms must be a positive safe integer");
    if (Boolean(r.script) === Boolean(r.script_path))
      throw new Error("set exactly one of script and script_path");
    if (r.script && typeof r.script !== "string")
      throw new Error("script must be source text");
    if (r.script_path) {
      absolute(r.script_path, "script_path");
      if (!/\.[cm]?[jt]s$/.test(r.script_path))
        throw new Error("script_path must be a JS or TS file");
    }
  } else if (
    typeof r.url !== "string" ||
    !/^https?:$/.test(new URL(r.url).protocol)
  )
    throw new Error("url must use HTTP or HTTPS");
  for (const [name, value] of Object.entries(r.secrets || {})) {
    if (
      !/^DEM_SECRET_\w+$/.test(name) ||
      typeof value !== "string" ||
      value.includes("\0")
    )
      throw new Error("invalid controlled secret environment entry");
  }
}
async function main() {
  if (process.argv.length !== 2) {
    if (process.argv.length !== 3 || process.argv[2] !== "--check")
      throw new Error("unsupported runtime operation");
    write(1, await checkRuntime());
    return;
  }
  const runtime = await runtimePaths();
  const chunks = [];
  for await (const chunk of process.stdin) chunks.push(chunk);
  request = JSON.parse(Buffer.concat(chunks).toString("utf8"));
  validate(request);
  protocol.configureSecrets(request.secrets);
  const r = request;
  const selected = extensions.selection(r);
  if (selected) emit({ type: "extension_init", extension_init: { selected } });
  for (const dir of ["output", "tmp", "home", "cache"])
    await fs.mkdir(path.join(r.work_dir, dir), {
      recursive: true,
      mode: 0o700,
    });
  const env = {
    PATH: path.dirname(process.execPath) + ":/usr/bin:/bin",
    HOME: path.join(r.work_dir, "home"),
    TMPDIR: path.join(r.work_dir, "tmp"),
    XDG_CONFIG_HOME: path.join(r.work_dir, "home", ".config"),
    XDG_CACHE_HOME: path.join(r.work_dir, "cache"),
    PWTEST_CACHE_DIR: path.join(r.work_dir, "cache", "transform"),
    CI: "1",
    NO_COLOR: "1",
    LANG: "C.UTF-8",
    ...r.secrets,
  };
  let args;
  if (r.kind === "journey") {
    let entry = r.script_path;
    if (!entry) {
      const input = path.join(r.work_dir, "input");
      await fs.mkdir(input, { mode: 0o700 });
      await fs.writeFile(
        path.join(input, "package.json"),
        '{"type":"module"}\n',
        { mode: 0o600 },
      );
      entry = path.join(input, "journey.spec.ts");
      await fs.writeFile(entry, r.script, { mode: 0o600 });
    }
    // Playwright reports canonical paths; the Agent supplies a short work alias.
    const sourceDir = path.dirname(await fs.realpath(entry).catch(() => entry));
    const modules = runtime.modules;
    let wrapper;
    const browserSummary = selected?.includes(BrowserSummary.FEATURE);
    if (browserSummary) {
      wrapper = path.join(r.work_dir, "browser-test.mjs");
      const real = JSON.stringify(
        pathToFileURL(path.join(modules, "playwright/test.mjs")).href,
      );
      const observe = JSON.stringify(
        pathToFileURL(path.join(here, "browser-context.cjs")).href,
      );
      await fs.writeFile(
        wrapper,
        `export * from ${real};\nimport { test as base } from ${real};\nimport observe from ${observe};\nexport const test = observe(base);\nexport default test;\n`,
        { mode: 0o600 },
      );
    }
    const config = {
      testDir: path.dirname(entry),
      workers: 1,
      retries: 0,
      repeatEach: 1,
      fullyParallel: false,
      forbidOnly: true,
      timeout: r.timeout_ms,
      globalTimeout: 0,
      respectGitIgnore: false,
      outputDir: path.join(r.work_dir, "output"),
      reporter: [
        [
          path.join(here, "reporter.cjs"),
          {
            workDir: r.work_dir,
            sourceDir,
            capture: r.capture,
            ...(selected?.length ? { extensions: selected } : {}),
            ...(browserSummary
              ? {
                  observerFile: await fs.realpath(
                    path.join(here, "browser-sampler.cjs"),
                  ),
                }
              : {}),
          },
        ],
      ],
      projects: [
        {
          name: "chromium",
          use: {
            browserName: "chromium",
            headless: true,
            launchOptions: {
              executablePath: runtime.browser,
              chromiumSandbox: true,
            },
            screenshot: r.capture ? "only-on-failure" : "off",
            trace: selected?.includes(extensions.FAILURE_TRACE)
              ? {
                  mode: "retain-on-failure",
                  snapshots: true,
                  screenshots: true,
                  sources: false,
                  attachments: false,
                }
              : "off",
            video: "off",
          },
        },
      ],
    };
    const configPath = path.join(r.work_dir, "playwright.config.cjs");
    const escaped = entry.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    await fs.writeFile(
      configPath,
      "module.exports = Object.assign(" +
        JSON.stringify(config) +
        ", {testMatch: new RegExp(" +
        JSON.stringify("^" + escaped + "$") +
        ")});\n",
      { mode: 0o600 },
    );
    const tsconfig = path.join(r.work_dir, "tsconfig.json");
    await fs.writeFile(
      tsconfig,
      JSON.stringify({
        compilerOptions: {
          allowJs: true,
          baseUrl: runtime.dependencies,
          paths: {
            "@playwright/test": [
              wrapper || path.join(modules, "@playwright/test/index.mjs"),
            ],
            "playwright/test": [
              wrapper || path.join(modules, "playwright/test.mjs"),
            ],
            playwright: [path.join(modules, "playwright/index.mjs")],
          },
        },
      }),
      { mode: 0o600 },
    );
    args = [
      path.join(modules, "playwright/cli.js"),
      "test",
      "--config",
      configPath,
      "--tsconfig",
      tsconfig,
      "--pass-with-no-tests",
    ];
  } else {
    args = [path.join(here, "lighthouse.mjs")];
  }
  // Configuration and filesystem awaits consume the same admitted work budget.
  if (
    r.kind === "journey" &&
    monotonicMilliseconds() >= r.stop_at_monotonic_ms
  ) {
    emit({ type: "result", result: journeyResult({}, true, r.capture) });
    return;
  }
  const child = spawn(process.execPath, args, {
    cwd: r.work_dir,
    env,
    stdio: [
      r.kind === "lighthouse" ? "pipe" : "ignore",
      "pipe",
      "pipe",
      "pipe",
    ],
  });
  if (r.kind === "lighthouse")
    child.stdin.end(JSON.stringify({ ...r, secrets: undefined }));
  let terminal;
  const stop =
    r.kind === "journey"
      ? journeyStop(child, r.stop_at_monotonic_ms, () => Boolean(terminal))
      : undefined;
  let protocolError;
  for (const [kind, stream] of [
    ["stdout", child.stdout],
    ["stderr", child.stderr],
  ]) {
    const safe = protocol.secretStream((message) =>
      emit({ type: "event", event: event(kind, { phase: "cli", message }) }),
    );
    stream.on("data", (chunk) => safe.write(chunk));
    stream.on("end", () => safe.end());
  }
  const lines = createInterface({ input: child.stdio[3], crlfDelay: Infinity });
  lines.on("line", (line) => {
    try {
      if (Buffer.byteLength(line) > 4 * 1024 * 1024)
        throw new Error("reporter record exceeds framing limit");
      const record = JSON.parse(line);
      const optional = extensions.optional(record, selected, terminal);
      if (optional) {
        if (optional !== true) emit(optional);
        return;
      }
      if (record.type === "event" && record.event) emit(record);
      else if (record.type === "result" && record.result && !terminal) {
        terminal = record.result;
        stop?.clear();
      } else throw new Error("invalid or duplicate reporter record");
    } catch (error) {
      protocolError = text(error.message);
    }
  });
  const ended = new Promise((resolve) => lines.once("close", resolve));
  const exit = await new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("close", (code, signal) => resolve({ code, signal }));
  });
  await ended;
  if (protocolError) throw new Error(protocolError);
  if (!terminal) {
    const error =
      "Runner exited without a terminal result (" +
      (exit.signal || exit.code) +
      ")";
    if (!stop?.requested) throw new Error(error);
    terminal = { error };
  }
  if (terminal.status === "success" && exit.code !== 0 && !stop?.requested)
    throw new Error("Runner exited unsuccessfully after a successful report");
  emit({
    type: "result",
    result:
      r.kind === "journey"
        ? journeyResult(terminal, stop.requested, r.capture)
        : terminal,
  });
}
main().catch((error) => {
  if (process.argv.includes("--check")) {
    write(2, { error: text(error.message) });
    process.exitCode = 1;
    return;
  }
  emit({
    type: "event",
    event: event("error", {
      phase: "execution",
      status: "error",
      message: text(error.message),
    }),
  });
  emit({
    type: "result",
    result: {
      status: "error",
      error: text(error.message),
      capture_state: request?.capture ? "unavailable" : "disabled",
    },
  });
  process.exitCode = 1;
});
