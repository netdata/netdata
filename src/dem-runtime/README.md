# DEM browser runtime

This tree owns the complete implementation of DEM execution contract 2: JavaScript
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
cannot embed its own digest. See `../../packaging/dem-runtime/README.md` for
assembly, native package recipes and exact source inventory. Ordinary Linux Agent
installations include DEM, but browser runtime installation remains explicit.
Production publication requires confirmed infrastructure, source/license review
and platform qualification.

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

`runner/run.mjs` receives one JSON request on stdin. The Go caller supplies version 2,
kind, run_id, work_dir, timeout_ms, capture and
optional secrets. Journeys select exactly one of inline script and absolute
script_path and require a positive safe-integer stop_at_monotonic_ms; Lighthouse
uses url. The Go owner creates the private working
folder. Node writes only generated configuration, script/cache/profile state
and output there. Resolved secrets enter only the controlled child environment
under DEM_SECRET_ names. Caller NODE_OPTIONS, proxies and other arbitrary
application variables are not inherited.

## Contract 2 semantics

The current implementation accepts execution contract 2 and request version 2.
Runtime-internal dependency and browser locations are not request fields.
Release versions are independent. See [Compatibility policy](#compatibility-policy)
for the rules governing future additions; those rules do not enable new wire
messages or request versions by themselves.

The Agent retains the admission-based hard execution deadline, including an earlier
caller deadline. Journeys reserve min(5 seconds, one fifth of that effective admitted
duration) inside this budget for reporting. The Agent sends the resulting absolute
Linux CLOCK_MONOTONIC soft stop as stop_at_monotonic_ms. The pinned Linux Node
24.20.0 runtime uses the same clock for process.hrtime.bigint(); a shared origin is
not a portable Node API guarantee. Configuration and startup consume this budget.
If startup exhausts it, no CLI/user work starts. Otherwise the journey parent sends
SIGINT once to its exact owned CLI child at the soft stop, unless a terminal report
has already arrived or that child has exited. Long timer delays are clamped and
rechecked. Child exit clears the timer, while inherited pipes still join through EOF.

timeout_ms keeps its native per-test semantics, including script test-timeout
overrides; the competing generated Playwright globalTimeout is disabled. Lighthouse
keeps its existing timeout semantics and has no journey soft-stop timer. A blocked
worker or teardown may still require the hard supervisor deadline and lose final
reporting. Neither the soft stop nor reporter completion proves process drainage.

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
timeout or native suite timeout reports timeout. Native suite timeout takes precedence
over earlier assertion and executor errors; a native interrupted suite reports
cancelled. Ordinary test timeout precedes unexpected failure, then executor failure.
Only complete ordinary passing coverage is success. Forced termination is
classified by the Agent using its
supervisor and partial evidence, not by a missing reporter result.

Optional termination_reason is independent of the first error. Runtime reasons are
test_timeout, suite_timeout, cancelled and execution_budget; ordinary outcomes omit
it. execution_budget means the parent requested its graceful stop before the terminal
report, without asserting which OS signal caused each reported observation. This
sets overall timeout while preserving the first error and actual interrupted test
status/counts. The parent supplies a budget message if no error was reported. The
Agent can supersede this with caller_deadline, execution_deadline or cancelled.

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
overrides are rejected before workers start. DEM configures video off and enables
tracing only when `failure_trace.v1` is selected; see [Optional journey failure traces](#optional-journey-failure-traces).
Captured diagnostic events describe test/step phases, errors and worker stdio;
the reporter does not promise browser console or network-event instrumentation.
Only all declared ordinary tests passing is success. Skipped, expected-failure
and incomplete coverage is inconclusive unless an unexpected failure/timeout
or executor error occurs. Import and managed-browser launch failures are
execution errors. CLI exit alone does not determine monitoring success.

Optional failure screenshots use Playwright's own capture path. PNG screenshot attachments inside output are
candidate artifacts; negotiated failure traces can also supply ZIPs. Failed/timedOut-test
candidates use artifacts; interrupted-test candidates use a separate
interrupted_artifacts list with IDs unique across both lists. The parent forwards
interrupted candidates only when its budget stop preceded the terminal report.
Go preserves this provenance and rejects interrupted candidates if a caller
cancellation or hard deadline supersedes the report; earlier ordinary-failure
candidates remain eligible. capture_state reflects both eligible lists. They may
be absent
on forced termination. Lighthouse uses its actual desktop preset and retains
nullable metrics; a missing observation never becomes zero. Failure to save its
optional report preserves completed measurements and adds an unavailable-capture
diagnostic. Its optional HTML
report, like screenshots, may contain sensitive page content and is not claimed
to be sanitized. Capture paths are relative to work_dir and begin with output/;
Go validates and publishes them after verified drainage.

## Compatibility policy

The execution contract defines shared request, result and safety semantics. A new
runtime release MUST preserve those semantics for existing supported requests, or
use a new execution contract. A core contract bump is required only when existing
required semantics cannot remain correct through baseline behavior or an explicitly
negotiated path. A break affecting even one supported request matters.

Contract 2 has two baseline compatibility rules:

- Unknown advisory request members MUST be ignored. Existing required fields and
  explicitly forbidden component-path fields retain their validation. A sender
  MUST NOT rely on an unknown field to enforce a required execution control.
- Without explicit opt-in, runtime output MUST remain inside the baseline
  contract-2 schema on every path, including failure, timeout, cancellation and
  capture reporting. Older Agents reject unknown nested response fields, frame
  types and event kinds. An optional new output field is therefore not automatically
  compatible just because its value could be omitted.

An unsupported request version MUST be rejected before script loading, generated
execution files, browser launch or workflow actions. Reading runtime identity and
starting the Node process are not workflow execution. Refusal still uses the
baseline error event/result and does not authorize replay with weaker settings.

### Additions and required capabilities

Output extensions MUST be explicitly requested by an Agent that understands
them and selected by the runtime for that attempt. The neutral `extensions` request
member and `extension_init` acknowledgement implement this gate for `failure_evidence.v1`,
the opt-in `browser_summary.v1`, capture-enabled `capture_details.v1`, and opt-in `failure_trace.v1`. The execution contract remains 2.

| Change | Compatible evolution rule |
|---|---|
| Safe-to-ignore request hint | Add an advisory member; retain baseline validation and behavior when absent. |
| New output field, event, metric or enum value | Emit only to a peer that opted in; retain truthful baseline output otherwise. Missing measurements are absent, not zero. |
| Optional diagnostic format | Use an exact format ID; permit additive fields or records only when older readers can safely ignore them. |
| Changed field meaning, unit, required record or completeness rule | Introduce a new field/format/capability; never silently change the existing meaning. |
| Required execution control | Honor it or refuse that attempt before workflow work; absence of an optional acknowledgement is not enforcement. |
| Incompatible execution, cancellation, capture trust or cleanup semantics | Change the core contract if baseline semantics cannot be preserved through a safe negotiated path. |
| Component update with unchanged protocol semantics | Change the immutable runtime release identity, not the core contract. |

Optional evidence cannot change authoritative outcomes/counts, authorize artifact
publication, prove process drainage or complete persistence. Invalid content inside
an identifiable best-effort extension MUST invalidate that evidence without
cancelling an otherwise valid attempt. Shared framing, mixed core/extension
envelopes and core result validation remain strict. Absence, unsupported formats,
partial reports and retention truncation MUST stay distinguishable; none implies
zero errors or complete evidence. Unknown record variants are safely ignorable only
when the format explicitly preserves known lifecycle and completeness semantics.

Required controls need a separate must-understand admission rule. A reserved design
is a new request dialect (for example version 3 with required capability IDs): an
old runtime rejects it before workflow work, while a newer runtime can still serve
baseline version-2 requests under execution contract 2. This separates the request
dialect from the runtime's shared baseline contract. It is not implemented, and
version 3 is currently unsupported. Introduce and validate it with the first real
required control, not as unused compatibility machinery. Never retry an unsupported
control as an advisory field or infer enforcement from optional output selection.

The extension owner directly routes typed Failure-evidence, browser summary and capture-detail consumers.
There is no generic registry, version-range solver, capability cache or extra probe
process in this design. Producer emission, parsing and retention must be bounded:
optional traffic shares the result stream and must be measured near execution
deadlines, not merely clipped after it has already delayed core reporting.

These rules do not make contract-1 runtimes executable by contract-2 Agents. Runtime
selection, cross-contract upgrade order, rollback and native package compatibility
remain separate work in [#24720](https://github.com/netdata/netdata/issues/24720).
New runtime content still requires a new immutable release version/digest.

## Optional journey evidence wire format

A journey request adds `"extensions":["failure_evidence.v1"]`. An absent or malformed
member preserves baseline-only output. A well-formed array of nonempty strings opts
in, including an empty array; duplicate requests normalize. After core validation and
redaction setup, before workflow work, the parent sends:

```json
{"type":"extension_init","extension_init":{"selected":["failure_evidence.v1"]}}
```

Selection may be empty (including Lighthouse). An old runtime ignores the advisory
request field and emits baseline output; the Agent records negotiation as unavailable (absence alone cannot establish support).
No negotiation probe, retry or replay is performed. An Agent without the request
receives only the frozen core event/result format from a new runtime.

Selected data uses `{"type":"extension","extension":{"feature":"failure_evidence.v1","data":{...}}}`.
The Agent accepts one initialization before its first core stdout record. Duplicate,
late or malformed initialization disables optional reporting for that attempt.
Unselected format IDs are discarded without allocating per-ID state. The initialization and extension envelope bodies have frozen keys; unknown keys invalidate optional reporting.
Unknown additive feature-data fields are discarded; unknown record variants invalidate this format. Identifiable
optional-body failures are nonfatal, including missing/null/scalar bodies. Invalid JSON,
unknown outer fields/types, mixed payload families, invalid core records and any Agent
stdout record after the core result remain protocol errors. Optional records cannot
clear the stop timer or become a terminal result. The parent ignores late reporter
extensions after its accepted core terminal, without claiming their completion.

| `data.record` | Typed payload and lifecycle |
|---|---|
| `test` | `test`: `id`, `title`, optional `location`, `status`, `expected_status`, optional `duration_ms`, `reporting_complete`, optional `text_truncated`. Begin then end updates one retained test. Skipped tests may report end only. Dynamic `test.fail()` may change expected status at end. |
| `step` | `step`: `id`, `test_id`, optional `parent_id`/`parent_omitted`, `title`, `category`, optional `location`, `state`, optional `duration_ms`/`message`/`text_truncated`. Begin/end update one retained identity; a step error alone does not establish an unexpected test failure. |
| `failure` | `failure`: a self-contained completed `test`, `path` of step identities, `omitted_ancestors`, optional `path_incomplete`, first error `message`, optional error `location`/`text_truncated`. Emitted once for the first unexpected failed/timed-out test or unexpected pass. It has independent retention even when detail pools are full. |
| `stop` | `stop`: the same bounded path/test shape as `failure`, with an empty message. Emitted once for the first `timedOut` or `interrupted` test. The path is pending activity observed before cleanup, independently reserved from the first failure and ordinary detail pools. |
| `truncated` | `scope`: `tests`, `user_steps`, `other_steps` or `text`. A scope is emitted at most once. |
| `complete` | Reporter finished this feature stream. Per-test completeness and truncation remain independent. Later feature records invalidate reporting. |

Test statuses are `running`, `passed`, `failed`, `timedOut`, `skipped`, `interrupted`;
expected statuses are `passed`, `failed`, `skipped`. Step states are `running`, `passed`,
`failed`. Location is `{file,line,column}`, with positive source positions, script-relative
file labels or `[external]`. IDs are opaque generated identity, not titles. Parent links
and retention order support concurrent sibling steps; they are not a single active stack.
The first-failure path matches the actual first test error. When repeated errors share
message and stack, only their common ancestry is retained and `path_incomplete` is set;
the runtime does not guess which sibling assertion was caught. Unexpected passes do not invent an error or failed step. Error messages may contain
the public Playwright call-log excerpt; no private call-log API or raw stack is retained.

The stop context snapshots public reporter steps whose duration is `-1` when the root
`hook` named `After Hooks` begins. Playwright 1.63 reports this boundary before user cleanup
and fixture teardown; real-runtime tests pin that ordering. The observation does not identify
the cause or exact location of the eventual timeout: an unawaited body action may remain
pending while cleanup itself times out. Caught, already-ended steps and later cleanup errors
are not selected. Concurrent pending branches retain only common ancestry with `path_incomplete`.
Missing boundary, no pending step, or depth uncertainty is explicitly incomplete; cleanup-only
timeouts can have test-only context. Path states describe the observation before cleanup,
even if those steps later complete. Test status describes the final reporter observation.
Summary source locations identify the displayed step or, without a known step, the test
declaration; they are not timeout locations.

### Feature budgets

Arbitrary journey scripts can generate more detail than history or interactive diagnosis
should retain. These are diagnostic quotas, not execution/test limits or journal SDK limits:

- 64 test details, 128 user (`test.step`) details and 64 other step details in independent
  pools. A retained ID reserves its end update; ordinary caps never suppress that update.
  Step-pool exhaustion makes last-running observations incomplete. Core test counts continue.
- Encoded JSON string budgets (including quotes and Go-compatible HTML/control escaping):
  test ID 128 bytes, step/parent ID 64, title/source file 256, category 64, step error 1024.
  Secret redaction precedes clipping. Explicit text flags disclose clipping.
- Each test is at most 1024 serialized bytes; each step at most 2048. A separate first failure
  retains up to 16 ancestry nodes (first eight and last eight), exact omitted-ancestor count,
  and a 2000-character error excerpt. First-error search uses at most 64 ancestor cursors;
  deeper unexplored subtrees mark `path_incomplete`, without discarding the error.
  Ambiguous error matches also mark the retained path incomplete.
- Stop context has its own single reserve with the same 16-node/64-depth path bounds.
  Its message is empty; it cannot replace or change first-failure attribution.
- Optional payloads are at most 32 KiB, far below the unchanged 4 MiB shared frame ceiling.
  At most two records per retained test/step, one first failure, one stop context and fixed loss/completion
  markers bound ordinary feature emission below 1 MiB and retained typed data below 544 KiB.

These quotas cover ordinary short journeys while reserving late failure evidence independently
of preceding API noise. Deep or large scripts get explicitly incomplete detail, not fabricated
absence. Earlier independently valid observations survive malformed later optional records;
a contradictory update removes its disputed item. Raw extension JSON is never persisted.
The typed snapshot is copied before outcome branching, including hard-stop and core-error paths.
Process drainage, capture validation and persistence remain the execution owners' responsibilities.

## Optional browser-error summary

The journey job option `browser_error_summary` defaults to false. When enabled,
the Agent requests `browser_summary.v1` alongside Failure-evidence. Both use the
same selection acknowledgement and 32 KiB envelope limit; malformed feature data
invalidates only that feature. Malformed shared framing/initialization affects
both. Missing support is explicit and never triggers a replay.

The selected runtime maps both canonical Playwright test imports to one boxed
context-fixture wrapper. It observes public context console, web-error, response
and failed-request events. Coverage is `fixture_context`: fixture pages and popups
are observed; manually created/replaced contexts and workers have no general
coverage guarantee. Tests without the fixture report `no_summary`. An open
annotation at test end is incomplete; missing test-end callbacks remain unknown.
Reporter completion means reporting ended, not that every browser scope was seen.

Only `console.error`, uncaught errors, failed transport and HTTP status >=400 are
counted. Counts continue after sampling stops. Each test has three permanent
candidate opportunities per category, at most three public sampler evaluations,
and at most four handles per evaluation. Duplicate/rejected/pending samples do
not refill slots. Console values are finite JSON primitives or explicit omissions:
`object`, `symbol`, `function`, `unrepresentable`, `non_finite`, `too_long`.
`unrepresentable` covers undefined and provider conversion loss, including BigInt
on Playwright 1.63. No object rendering, request bodies, headers or cookies are
captured. Primitive strings are whole values or omitted, never prefixes.

Sampling starts eagerly; fixture teardown takes a synchronous snapshot without
waiting for evaluations. Pending/unavailable states are explicit and late
callbacks cannot change it. Whole browser-text omission uses `text_omitted: too_long`
on an unavailable sample, without inventing arguments. The sampler has one canonical
file; the shared step view excludes only its `pw:api` operations from core diagnosis and every
Failure-evidence reader. Original provider step objects remain untouched and raw
Playwright steps can contain those operations. User steps/API calls remain visible.
This is a supported-script boundary, not isolation against direct private imports.

Secrets are redacted before bounding text. Structured URLs lose credentials, query
and fragment; an oversized URL is omitted whole and an unusable URL is empty.
Recognized embedded HTTP(S) URLs receive best-effort stripping. Arbitrary
application text is not guaranteed anonymous. The renderer guards strings at 512
UTF-16 units; worker/Go retention requires <=512 Go-encoded JSON bytes, including
quotes. Metadata fields may clip with an explicit marker. Method/URL/transport
error limits are 16/256/128 encoded bytes.

The worker sends one `netdata.dem.browser_summary` annotation (`v:1`) with state
`open`, `observed`, or `invalid`. Observed annotations contain counts, candidates,
omission flags and `rpc_calls`; the description boundary is 16 KiB. Counter
arithmetic uses nonnegative safe integers. Overflow terminates this feature with
`counter_overflow`; an oversized producer annotation uses `annotation_limit`.

The reporter retains at most 64 interesting test rows and four distinct samples
per category (16 total). Clean observed tests contribute only to aggregates.
Samples are stored once with their first test owner; later duplicates cannot claim
that attribution. After row exhaustion, at most four `late_sample` records can
introduce one representative of a category absent from that same pool. The records
are `test`, `late_sample`, `truncated` (once per tests/samples/text scope), and one
`complete` or `invalid`. Every ordinary record carries a cumulative `run` aggregate;
terminal invalid preserves earlier evidence without replacing its counts.

Omission flags describe limits, not lost-event counts: `args_limit`, `rpc_budget`,
`candidate_limit`, `pending`, `unavailable`, `too_long`, `pool_full`, `row_limit`,
`identity_limit`. Only `args_omitted` counts omitted arguments. No count-minus-sample
loss estimate is made. Actual published clipping controls text truncation.
Test/sample identities are never clipped. Known fields are validated atomically;
additive fields are discarded, and conflicting identities or terminal ordering
invalidate this feature. Missing aggregates are not zeros; absent counters inside
a validated aggregate are zero.

Structural tests prove <=16 KiB generated record data, <=33 KiB escaped annotation
contribution, <=72 KiB generated feature traffic per run and <=40 KiB retained
JSON. Those are derived guarantees, not extra byte ledgers or cumulative raw-input
policies. The annotation/envelope boundaries and field/collection quotas enforce
the bounds. Provider dispatch/handles and all other Playwright IPC are outside
these owned-state guarantees. See the DEM `PERFORMANCE.md` for measured opt-in cost.
No summary event changes test outcomes, deadlines, retries, process drainage or
artifact publication.

## Tests

Run `npm ci --ignore-scripts` and `npm test` in this source directory, or set
DEM_TEST_DEPENDENCIES to an absolute, separately prepared dependency root before running
`node --test tests/*.test.mjs`. Set DEM_TEST_BROWSER to the pinned sandbox browser
for the real-browser summary and native trace cases. Tests also use real non-browser Playwright execution
and synthetic fixture runtimes, without adding component path overrides to the
execution request. Fixtures stamp the test Node version; production archives use
the release pin. Real Linux browser/audit and process-tree cancellation validation
belongs to the Go-to-Node integration checks. Candidate installation qualification
must exercise both journeys and Lighthouse with deterministic local content.

`tests/contract-2.mjs` freezes baseline wire fields, types and enum values from
`7278241848caed20c5a5d59b82e343bac5dbb037`. It checks real runner output without
importing evolving production models. Do not expand that baseline to accommodate
new output: future opt-in tests must retain baseline-only checks for old requests.
This shape/enum guard complements the Go semantic validators; it does not prove
process drainage, capture provenance or extended-peer compatibility. The tests also
pin advisory request tolerance and unsupported-version refusal before workflow work.
The shared `dem-runtime-node-tests.yml` workflow runs this suite with the manifest's
Node pin for both DEM PR checks and runtime release builds; a failing suite blocks
release assembly. These non-browser tests do not replace target qualification.

## Optional capture details

Capture-enabled journeys request `capture_details.v1` alongside other optional features. The parent selects it only
when capture is enabled. An old runtime or a peer without this selection keeps existing capture behavior; no feature
probe or extra job option is needed. The frozen core candidate objects and terminal lists are unchanged.

The reporter emits these records after processing an eligible test's capture attachments:

| `data.record` | Payload |
|---|---|
| `test` | `test`: `id`, `title`, optional `text_truncated`, `status`, `expected_status`, `accepted`, `unaccepted`. Counts cover usable PNG candidates and unusable PNG attachments named `screenshot`, including mixed results. |
| `capture` | `id`, `test_id`: links one accepted candidate to a preceding test observation. |
| `truncated` | Detail was omitted; emitted at most once. |
| `complete` | Reporter finished this stream; does not imply untruncated coverage or publication. |

Eligibility retains existing actual statuses `failed`, `timedOut`, `interrupted`. Expected failures may have no
screenshot; unexpected passes remain outside this capture loop. The runtime cannot infer why Playwright supplied no
attachment. Association is to the test, not a page, step or screenshot instant. No extra browser calls are made.
Only file-backed attachments below `output/` are candidates. Playwright's `testInfo.attach` with `body` retains inline
bytes without a file path; a PNG named `screenshot` in that form contributes to `unaccepted`, not publication.

One budget permits 256 test/link records per attempt (32 tests with four screenshots use 160). Both producer emission
and consumer retention enforce it independently of capture publication. Later terminal candidates remain eligible,
with association explicitly unrecorded. Existing encoded ID/title budgets (128/256 bytes) and the 32 KiB extension
body limit apply; exact-secret redaction precedes title clipping. There is no per-test capture quota or byte ledger.

Invalid bodies or unknown variants invalidate this feature alone and retain earlier valid observations; conflicting
identities lose their disputed association. Shared envelopes and core records remain strict. Reporting states are
`negotiation_not_reported`, `unsupported`, `partial`, `complete`, `invalid`; truncation is independent. An absent stored
report means not recorded. Lighthouse has Go publication dispositions but no journey feature reporting state.

Go joins labels only onto eligible terminal candidates. Metadata IDs never authorize files. A delivered association
without an accepted final candidate is `not_in_final_report`; interrupted candidates visible to Go but rejected by
its actual stop policy are `not_eligible`. Without terminal authority, earlier screenshots are not published even if
their metadata arrived. Publication still requires verified drainage. Go skips valid candidates initially above
5 MiB and publishes the retained subset atomically. Those omissions are `too_large`, without general run-error text;
if all are omitted, capture state is `unavailable` and no empty manifest is created. Invalid input, copy growth and
I/O failures still reject the entire set. An identified hard size-error offender is `too_large`; other candidates,
including earlier size skips, are `publication_failed`.

Published artifact association is retained in manifest version 2 and run history. Missing association means no label;
Go-authored stored records use ordinary typed JSON decoding. Optional-wire validation remains isolated from core
outcomes and publication. Historical omissions remain in `run.captures`, never in empty manifests; current availability
and byte access remain owned by the artifact store. Functions derive labels
from artifact kind and display test titles separately. Titles outlive history when their captures do; redaction of
metadata does not sanitize PNG, ZIP or HTML content.

## Optional journey failure traces

`trace_on_failure` is off by default and requires `screenshot_on_failure`. Enabling the option requests
`failure_trace.v1`; selecting it enables tracing in the generated Playwright configuration and allows the reporter
to offer trace candidates. It adds no extension body records and does not change execution contract 2. Old Agent
requests retain baseline output; an older runtime can decline the new feature without changing screenshot behavior.
Core trace-kind admission depends on the Agent's
request, never on optional `extension_init` validity.

DEM configures `retain-on-failure`, snapshots/screenshots enabled, sources/attachments disabled. Trusted scripts can
override these settings with `test.use({ trace: 'on' })`, invoke tracing APIs or supply their own trace attachments.
Script-directed recording can occur even when DEM tracing is off or Configuration Test is used. Configuration Test
never publishes captures; monitoring attempts publish only requested, eligible captures.

Existing failed/timedOut/interrupted attachment eligibility applies. Under the configured defaults, expected failures
produce no automatic trace; unexpected passes may write a ZIP that DEM does not offer and drained workspace cleanup
removes. One test can merge multiple
browser contexts into its native ZIP. Missing attachment/export is best effort, not proof of a clean page.

The reporter accepts attachments named `trace` with MIME `application/zip`, checks the same output-root/regular-file
boundary as PNGs and a ZIP local-file-header signature, then offers `trace-<index>` / kind `trace` candidates through
the existing normal or interrupted list. IDs share the existing per-attempt index. Capture-detail test links apply to
both kinds, while `accepted` and `unaccepted` remain PNG-only. No trace rejection counter or inferred absence reason
is introduced. Existing Go omissions describe candidates rejected by terminal eligibility or publication.

When requested, `run.captures.trace` describes negotiation: `supported`, `unsupported`, `negotiation_not_reported` or
`invalid`; absent when disabled. It describes capability, not whether a trace was produced or retained. Bad
capture-detail payloads do not change this independent capability state. Invalid shared negotiation affects optional
reporting only, never outcomes or request-authorized core candidates.

Trace ZIPs retain raw data, including resolved DEM credentials, cookies, form values, headers, bodies, object details
and source fragments. Text/step redaction does not rewrite ZIPs. Source and attachment exclusion is not redaction.
Recording costs CPU, memory and temporary space even on passing tests. Export precedes the terminal report and can
exhaust its existing timeout reserve; captures remain best effort with no deadline extension, retry or early salvage.

The shared 5 MiB publication/fetch limit applies after recording: valid ZIPs over the limit at the initial size check
are omitted with `too_large` while valid PNG siblings remain. Growth beyond the limit during copying is a hard error
that aborts the entire set, as do other hard publication errors. Existing administrator-equivalent byte access and
atomic set retention apply. Download
and base64-decode `synthetics-artifact` bytes to a ZIP, then use the standard local Playwright Trace Viewer.

Manifest version remains 2. An Agent without trace-kind support rejects mixed sets, including their PNGs, and reports
them as unclassified outside retention cleanup. Preserved files become readable again after re-upgrade. This is a
known downgrade limitation; no alternate set format or compatibility adapter is added.
