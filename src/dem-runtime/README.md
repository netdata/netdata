# DEM browser runtime

This tree owns the complete implementation of DEM execution contract 1: JavaScript
adapters, exact component versions, private import paths and release tests. An
unchanged Agent can execute successive qualified runtime releases implementing
that contract. The Agent owns scheduling, supervised execution, contract validation,
metrics, history and verified capture publication. Component upgrades can change
browser behavior or Lighthouse scores; each run retains its immutable release identity.

`manifest.json` pins Node, npm packages and Debian's unbranded Chromium for this
release. These pins are not compiled into the Agent. `runtime.json` owns internal
locations and the contract/version; the local builder stamps its release version
over the source value `dev`. `runner/run.mjs` is the release entrypoint. The outer
trusted descriptor owns the archive SHA256, bounds and entrypoint. The archive
cannot embed its own digest. See `../../packaging/dem-runtime/README.md` for local
assembly and exact source inventory. Production publication and native packages
are separate delivery work; this remains an experimental, default-off integration.

Managed preparation and protected preloads use complete verified bundles. Node,
npm dependencies and system browsers cannot be selected independently. The runtime
resolves its browser to the prepared `chrome` wrapper, which applies the private
loader/font configuration. Execution does not modify installed files. Browser jobs
require an unprivileged plugin process on Linux with a compatible sandbox policy;
root RUM operation does not enable browser jobs. Playwright's bundled browser
revision is not the runtime delivery identity.

`runner/run.mjs --check` performs runtime-owned exact component version and private
import checks, including Chromium's bounded `--version` query. It opens no browser
session and reads no monitor request. Success is one bounded JSON object with
`contract`, `version`, `uid` and `gid`; failure exits nonzero with a bounded JSON
diagnostic on stderr. The Agent compares it with the acquired release identity
and consuming identity. This check does not replace real journey and Lighthouse
qualification through the supervised Agent execution path.

`runner/run.mjs` receives one JSON request on stdin. The Go caller supplies version 1,
kind, run_id, work_dir, timeout_ms, capture and
optional secrets. Journeys select exactly one of inline script and absolute
script_path; Lighthouse uses url. The Go owner creates the private working
folder. Node writes only generated configuration, script/cache/profile state
and output there. Resolved secrets enter only the controlled child environment
under DEM_SECRET_ names. Caller NODE_OPTIONS, proxies and other arbitrary
application variables are not inherited.

## Contract 1 semantics

There is one supported execution contract and no version adapters. Runtime-internal
dependency and browser locations are not request fields. The outer descriptor's
`contract` and request `version` are both 1; release versions are independent.

Journey discovery emits a `suite` event with `phase: discovery` and integer
`declared_tests`, including zero. Its human-readable title has no machine meaning.
Each `test` start/end event carries `test_id`, `expected_status` and `status`.
Start status is `running`; terminal test statuses are `passed`, `failed`,
`timedOut`, `skipped` or `interrupted`. The runtime maps its dependency's behavior
to these contract values. Go uses these events to retain observed partial counts
when a run terminates before a result. Step/error phases describe execution context
and are diagnostics; execution failures use `phase: execution`.

The terminal result status is `success`, `failed`, `timeout`, `cancelled`,
`inconclusive` or `error`. Journey counts are disjoint: `declared` equals the sum
of `passed`, `failed`, `timed_out`, `skipped`, `expected_failure` and `not_run`.
Both unexpected passes of expected-failing tests and expected failures belong to
`expected_failure`; interrupted or unstarted tests belong to `not_run`. A test
timeout takes precedence over observed unexpected failure, then executor failure.
The suite's timeout/cancellation applies next; only complete ordinary passing
coverage is success. Forced termination is classified by the Agent using its
supervisor and partial evidence, not by a missing reporter result.

Lighthouse success describes a completed desktop performance audit, without a
site-quality threshold. `performance` ranges from 0 to 100; `fcp_ms`, `lcp_ms`,
`tbt_ms` and `si_ms` are nonnegative milliseconds; `cls` is a nonnegative unitless
layout-shift score. Unavailable or invalid values are absent, never converted to
zero. Legitimate zero measurements remain present.

Stdout is newline-delimited event/result JSON, never raw user output. The
Playwright reporter uses a separate child descriptor; ordinary child stdout and
stderr are diagnostic events. Worker and CLI streams have separate redaction
state across callbacks and UTF-8 reads; stream events describe that source rather
than assigning delayed bytes to a test. Structured test/step/error events retain
their test identity and remain incremental. Exact resolved values, including
short values and values longer than the text budget, are removed before clipping.
An unfinished matching prefix is withheld or redacted on stream termination.
The carry per stream is bounded by the longest supplied value plus one read;
resolved values themselves are bounded by the 4 MiB request transport limit.

Go assembles diagnostic lines before applying its common credential patterns.
A line longer than 2000 characters is omitted with an explicit truncation marker,
then input is discarded through newline; its remainder cannot become a fresh
unredacted diagnostic. PEM blocks are suppressed across lines without buffering
the block. Incomplete lines may wait until stream completion, and forced process
termination can lose buffered output. Structured diagnosis is clipped to 2000
characters after redaction. The Go reader owns the 500-event retention limit
and dropped count, including line/truncation events. These measures cover exact
resolved text and recognized credential shapes; they do not promise to scrub
arbitrarily encoded or transformed credentials. Reporter completion is
not process completion: Go must join the Linux supervised tree before publishing
artifacts, releasing admission or replacing the job.

## Journey authoring

Scripts use real Playwright assertions, hooks, fixtures and TypeScript loading.
Use named imports or destructured CommonJS require:

```js
import { test, expect } from '@playwright/test';
// CommonJS: const { test, expect } = require('@playwright/test');
```

Explicit generated tsconfig paths map canonical imports to the prepared real
ESM exports. This keeps file-based scripts and relative helpers at their original
locations, without requiring adjacent node_modules. The callable CommonJS
shorthand `const test = require('@playwright/test'); test(...)` is unsupported;
use destructuring. Neighboring Playwright configuration, tsconfig aliases and
unselected test files are not loaded. Imported helpers still execute normally;
this is one selected entry plus its imports, not a hostile-code sandbox.
Additional package installation and arbitrary project build configuration are
not provided. Inline source is a generated TypeScript module; relative helper
imports belong in file-based scripts.

One Chromium project and worker run without retries or repetition. Script retry
overrides are rejected before workers start. Tracing and video are disabled.
Captured diagnostic events describe test/step phases, errors and worker stdio;
the reporter does not promise browser console or network-event instrumentation.
Only all declared ordinary tests passing is success. Skipped, expected-failure
and incomplete coverage is inconclusive unless an unexpected failure/timeout
or executor error occurs. Import and managed-browser launch failures are
execution errors. CLI exit alone does not determine monitoring success.

Optional failure screenshots use Playwright's own capture path. Only PNG
screenshot attachments inside output are candidate artifacts. They may be absent
on forced termination. Lighthouse uses its actual desktop preset and retains
nullable metrics; a missing observation never becomes zero. Failure to save its
optional report preserves completed measurements and adds an unavailable-capture
diagnostic. Its optional HTML
report, like screenshots, may contain sensitive page content and is not claimed
to be sanitized. Capture paths are relative to work_dir and begin with output/;
Go validates and publishes them after verified drainage.

## Tests

Run `npm ci --ignore-scripts` and `npm test` in this source directory, or set
DEM_TEST_DEPENDENCIES to an absolute, separately prepared dependency root before running
`node --test tests/*.test.mjs`. Tests use real non-browser Playwright execution
and synthetic fixture runtimes, without adding component path overrides to the
execution request. Fixtures stamp the test Node version; production archives use
the release pin. Real Linux browser/audit and process-tree cancellation validation
belongs to the Go-to-Node integration checks. Candidate installation qualification
must exercise both journeys and Lighthouse with deterministic local content.
