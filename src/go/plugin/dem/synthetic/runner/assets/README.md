# DEM execution assets

The browser runtime requires an unprivileged plugin process on Linux. Root RUM
operation does not enable browser jobs. These immutable assets adapt prepared
Node 24+, Playwright Test 1.63.0 and
Lighthouse 13.4.1 to the DEM execution protocol. Install the dependencies from
`package-lock.json` into a separate prepared directory with `npm ci`; the plugin
never downloads packages, modifies the installed assets or updates the browser.
The prepared full Chromium is Playwright revision 1243, version 153.0.8010.12.
Browser sandboxing remains enabled and requires a compatible Linux namespace and
container security policy. The manifest describes the supported target; the Go
owner validates runtime availability and owns execution completion.

`run.mjs` receives one JSON request on stdin. The Go caller supplies version 1,
kind, run_id, dependencies_path, browser_path, work_dir, timeout_ms, capture and
optional secrets. Journeys select exactly one of inline script and absolute
script_path; Lighthouse uses url. The Go owner creates the private working
folder. Node writes only generated configuration, script/cache/profile state
and output there. Resolved secrets enter only the controlled child environment
under DEM_SECRET_ names. Caller NODE_OPTIONS, proxies and other arbitrary
application variables are not inherited.

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

Run `npm test` with dependencies prepared beside these assets, or set
DEM_TEST_DEPENDENCIES to a separately prepared dependency root before running
`node --test tests/*.test.mjs`. Tests use real non-browser Playwright execution
and synthetic fixtures. Real Linux browser/audit and process-tree cancellation
validation belongs to the Go-to-Node integration checks.
