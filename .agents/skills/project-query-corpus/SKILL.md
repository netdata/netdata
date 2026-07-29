---
name: project-query-corpus
description: Developer contract for the query contract corpus (tests/query-corpus) — the black-box correctness suite for the Netdata query engine. Use when running the corpus, adding or extending corpus cases, authoring fixtures, changing an oracle or a byte-pin, adding a case for a query-engine bug, recording the fixing PR after it merges, or validating a query-engine fix branch against the corpus.
type: project
---

# Query Contract Corpus — developer contract

`tests/query-corpus/` is an end-to-end correctness suite for the query
engine of a completely **stock** `netdata` daemon. Fixtures are ingested
through the real streaming protocol by a fake child, queries run over the
normal HTTP API, and every response is checked against expectations
computed **outside** the daemon. `tests/query-corpus/README.md` describes
the layered ladder (L0 harness → L9 window/API surface); this skill is the
contract for working on the suite.

The suite is a self-contained Go module
(`github.com/netdata/netdata/tests/query-corpus`). Always run `go` commands
from inside `tests/query-corpus/`.

## Correctness model — why this suite means something

The founding rule: **expectations MUST be derived from the fixture
definitions, never captured from the engine.** A harness whose expected
values come from the system under test proves nothing. Every check in the
corpus belongs to one of three classes, with different rules:

- **Class A — first-principles oracles (the default).** Fixtures are
  literal Go definitions: charts, dimensions, explicit
  `(timestamp, collected value, SN flags)` points at the fixed epoch
  `fixture.T0 = 1700000000`. Expected values are computed in the test from
  those definitions (algorithm application, sums, averages, group-by folds,
  weights math, anomaly bit counts). New checks MUST be Class A unless the
  transform genuinely cannot be derived from first principles.
- **Class B — ports of engine algorithms.** Some transforms are engine
  design decisions, not derivable math: storage-number quantization
  (`fixture/sn.go`), tier rollups (`fixture/tier.go`), the time-grouping
  internals (`fixture/timegroup.go`), virtual-points interpolation
  (`fixture/viewpoints.go`). These are **reimplementations written from
  reading the C source** — never captures of engine output. Rules:
  - A port MUST cite the C source it mirrors (file:line in comments).
  - Every divergence found between a port and the engine MUST be resolved
    explicitly: either it is an engine bug (author a case for it) or an engine
    quirk adopted into the oracle **with a recorded pending ruling** in the
    manifest/SOW. Silently adjusting a port until the engine passes is the
    cardinal sin of this suite ("fit-to-engine") and is prohibited.
  - Where a port could hide drift, bound it independently (e.g. L2 pins the
    SN quantization error envelope against the ORIGINAL values).
  - A port MUST NOT be the oracle for a quantity that obeys an independent
    law. Where one exists — conservation, monotonicity, additivity — that
    law is derivable, so the check is Class A and the port has nothing to
    contribute but the engine's own opinion. `sum` is the worked example:
    `viewpoints.go` faithfully ports the virtual-points loop, which is the
    right model for average/min/max/stddev (each answers "the LEVEL at this
    instant"), and the wrong one for a volume — it served L9 a bucket
    holding 98.33 where the fixture had put 88.33, agreeing with the engine
    while both broke the L10/L11 totals. `fixture.ViewSumVolume` replaces it
    for `sum`. Ask of every Class B use: is there a law here the port could
    contradict? If yes, assert the law instead.
- **Fixtures MUST make the answer exact where the contract is exact.** A
  conservation check reads a difference, so anything else that moves the total
  is noise the check will report as a defect. Above tier 0 the loudest such
  source is resolution: when a window edge or a plan seam cuts a stored
  record, the part inside can only be estimated from the record's width, and
  with varying data that estimate is wrong by a share of the record — which
  is what tiers cost, not a defect (see "Tiered rollups are not bugs").
  - Grid-align the window so no edge falls inside a record, and where a
    boundary is discovered rather than chosen (a rotated tier head), carry a
    **flat dimension** alongside the varying ones: a constant survives rollup
    exactly, so its share of a cut record is its exact truth and any
    difference left is arithmetic.
  - Worked example: CASE-026's seam contract first read 0.49s of data
    "missing" across a plan switch, entirely from the seam cutting one
    random-valued tier1 record. On the flat dimension the same three windows
    answer 7,200,000 to the digit at every zoom. Had the noisy version been
    committed, the corpus would have reported accepted rollup as an engine
    bug — the mirror image of fit-to-engine, and just as damaging.
- **Class C — byte-pins and parity checks.** Formatter byte-pins (L7/L8,
  options) capture engine output once and pin **stability** — they detect
  contract regressions, not first-principles correctness. Parity checks
  (v2 vs v3, same-response) prove internal coherence only. Rules:
  - A Class C pin MUST be paired with independent validity checks where
    they exist (e.g. "the payload parses as JSON", "values equal the
    fixture-derived numbers inside the pinned envelope").
  - Updating a pinned byte string requires a justified contract change
    (a PR that deliberately changes the output format), never "the test
    started failing".

**Falsifiability discipline:** when an expectation and the engine disagree,
there are exactly two exits — the engine is wrong (the case stays as written
and joins the broken list until it is fixed) or a recorded ruling says the
behavior is intended (the case is rewritten to assert the ruled behavior,
with the quirk documented). There is no third option where the oracle is
quietly bent to match, and none where the disagreement is filed away as
acceptable.

## Architecture map

- `stream/stream.go` — the fixture child. Speaks plugins.d over the
  streaming socket: `CHART`/`DIMENSION`/`CLABEL`, live samples
  (`BEGIN2/SET2/END2`), v1 paced samples (`BEGIN/SET/END`), replication
  (`RBEGIN/RSET/REND`). Protocol words quote-switch per word (`qw()`):
  plugins.d accepts both `'` and `"` delimiters, so ids carrying an
  apostrophe ship double-quoted. `SET2` sends the value explicitly (the
  `#` shorthand truncates fractional values to integers on the parser side).
- `daemon/daemon.go` — the harness. Boots the stock binary with a scratch
  run dir, waits for the HTTP API, exposes query helpers (`DataV3`,
  `DataV1Raw`, `HostJSON`, …) and the settle primitive `WaitRetention(host,
  context, first, last, timeout)`.
- `fixture/` — the fixture model (`Chart`, `Dimension`, `Point`) and the
  Class B oracles (`sn.go`, `tier.go`, `timegroup.go`, `viewpoints.go`).
- `canon/` — canonical response comparison helpers.
- `*_test.go` — the ladder layers (`layerN_*.go`), surface files
  (`weights_`, `selectors_`, `options_`, `anomalybit_`, `resets_`,
  `rates_`, `updateevery_`), and per-bug files (`caseNNN_test.go`).
- `manifest.go` + `MANIFEST.md` — the ledger (below). Keep both in sync in
  the same commit.
- `reference-python/` — local-only cross-check implementation. It is NOT
  tracked and MUST NOT be committed.

## The manifest

Every contract case has an entry in `manifest.go`; its `Proves`, `Cloud`, and
optional `FixedBy` fields are mirrored as a row in `MANIFEST.md`.

- Register ordinary Go assertions with `trackContract(t, name)` before any
  operation that may fail or skip. Its cleanup records `Error`, `Fatal`, and
  `Skip`.
- Use `assertContract(t, name, held)` when the test computes an explicit
  contract verdict. Register the test up front as well, so an earlier
  `Fatal` cannot make the contract disappear from the summary.
- When independent test scopes jointly prove one contract, declare their
  names in `ManifestCase.Components` and register each with
  `trackContractComponent`. One component passing never substitutes for
  another component that did not run.

**A broken contract fails. Always.** On master, on a feature branch,
whether or not the break is already known.

- The manifest records NO expected outcome. There is no "known broken, and
  therefore fine" state, and adding one is prohibited: it makes a broken
  query engine report success, and this suite exists to name what is
  broken, not to keep a list of exceptions.
- An unfiltered root-package run ends with the deduplicated list of broken
  contracts and fails if any manifest contract or required component did
  not run. **That complete list is the corpus's answer** — the open-defect
  list produced by measurement rather than by hand.
- A filtered run reports how many contracts were fully evaluated and never
  claims the complete corpus holds. `-list` prints no contract verdict.
- `go test ./...` therefore exits non-zero while any contract is broken or
  the ledger is incomplete. That is the intended signal, not a problem to
  suppress. The corpus is not wired into CI.

A case name is `<layer-or-CASE-id>/<slug>`; `Proves` is one sentence a
maintainer can read as the contract claim. A case whose bug is fixed keeps
its test as the regression guard and records `FixedBy: "#PR"`.

## Running

- Build the daemon first: `ninja -C build netdata` from the repo root
  (prefer `nice -n 19` on shared workstations). The suite uses
  `../../build/netdata` by default; override with `QUERY_CORPUS_NETDATA=
  /path/to/netdata`.
- Full suite: `cd tests/query-corpus && go test ./... -count=1` (~6 min,
  one shared daemon plus per-scenario daemons).
- One test: `go test -count=1 -run 'TestName' .`
- Keep the daemon run dir for inspection: `QUERY_CORPUS_KEEP=1` (it is
  always kept on failure; the path is printed as `daemon run dir kept:`).
- Capture the verdict honestly: `go test ... ; echo "exit=$?"` — piping
  through `tail` masks the exit code.
- **Before every push of the corpus branch, run the full suite and compare
  the broken list to the previous run.** It must not grow, and no case may
  break that was holding before. The list being non-empty is expected while
  the query engine still has open defects.

## Authoring fixtures

- **Epoch**: all points anchor at `fixture.T0`. For `update_every > 1`,
  pre-align the series: `base := fixture.T0 - fixture.T0%int64(ue)` —
  storage keeps pushed timestamps exactly, but views re-grid onto absolute
  `update_every` multiples, so unaligned fixtures make expectations
  needlessly hard.
- **Host GUIDs**: `guid(n)` builds a deterministic machine GUID. `n` MUST
  be unique across the whole suite — hosts persist in the shared daemon for
  the entire run, so a collision silently cross-contaminates two tests.
  Before taking a number, `grep -n 'guid(' *_test.go` and pick an unused
  range; ranges used by loops (e.g. soak attempts) reserve their whole
  span.
- **Settle discipline**: after pushing, block on `td.WaitRetention(...)`
  before querying. Keep the pusher connection OPEN until after the settle
  barrier and assertions (`connect()` closes at test cleanup) — the
  receiver discards in-flight data when a child disconnects immediately
  after writing.
- **Weights fixtures**: rrdcontexts stamps retention ~1–2s after chart
  creation; weights queries return empty until then. Settle on the
  contexts `first_time_t` (see `weightsSettle`), not only on retention.
- **Tolerances**: exact comparison is the default. `Chart.ValueTolerance`
  is ONLY for quantization-probing fixtures, with the reason in a comment.
- **Tier window alignment**: `TierWindows(gran)` keys on ABSOLUTE multiples
  of the granularity, not on offsets from `T0`. `fixture.T0 % 60 == 20`, so
  the first tier-1 window ends at `T0+40` and a fixture whose shape is
  keyed on the sample index straddles two regimes per stored window. Anchor
  tier queries at `T0+40`, and let the oracle — never the fixture's index
  arithmetic — say what each window contains.
- **Forcing wide-point re-delivery**: ask for a view grid FINER than the
  stored data (`DataParamsTier(ctx, 1, after, before, buckets, ...)` with
  `buckets` a multiple of the stored window count). Each stored point is
  then delivered to several buckets, carrying its original start and an
  INTERPOLATED value — so any grouping that reads `value` instead of the
  window's own statistics answers differently per bucket. That is the only
  way to reach the repeat path from a query, and it is how
  CASE-023/tier-wide-point caught a constant window being judged on an
  interpolated blend of two windows.

## Adding a case

1. Author the fixture (Class A first; reach for a Class B oracle only when
   the transform requires it).
2. Push it (`pushLiveBurst` for live bursts; paced v1 or replication where
   the ingestion path is the thing under test), settle, query.
3. Compute expectations in Go from the fixture definition. Never paste a
   number you got from the engine.
4. Add the manifest entry and the `MANIFEST.md` row, then register the
   contract at the narrowest test/subtest scope that proves it.
5. Run the full suite; a new case MUST NOT destabilize existing cases
   (watch for GUID collisions and shared-host mutations).

## Adding a case for a bug (bug workflow)

A case for a known bug is written exactly like any other case: it states
the CORRECT behavior and fails while the engine gets it wrong. It is not
marked, excused, or inverted — it joins the broken list until the fix
lands, and the broken list is what the corpus is for.

1. Reproduce the divergence deterministically in its own `caseNNN_test.go`
   with a minimal fixture. The check asserts the CORRECT behavior and feeds
   the result into `assertContract`.
2. Add the manifest entry with a `Proves` sentence stating the contract
   precisely (what correct is), not the bug's symptoms.
3. Confirm it fails on today's daemon, and that the failure names the real
   defect — a case that fails for the wrong reason is worse than none.
4. The fix goes in its OWN branch/PR — never mixed into the corpus branch.
5. Validate the fix branch against the corpus before opening the PR:
   - build the fix branch, save the binary aside;
   - from the corpus checkout:
     `QUERY_CORPUS_NETDATA=<fix-binary> go test -count=1 -run '<the case
     plus neighboring pins>' .`
   - the case MUST now hold, and every other case that was holding MUST
     still hold (zero collateral).
6. When the fix merges: rebase the corpus branch onto the merge, record
   `FixedBy: "#PR"`, reword the case comment and `Proves` to describe the
   contract in force, run the full suite, push. The case lives on as the
   regression guard.
7. If the divergence is ruled intended behavior instead: change the case to
   assert the ruled behavior, document the quirk in the oracle comment and
   the `Proves` text, and record the ruling.

## Changing oracles, pins, and the harness

- An oracle change MUST cite its justification: the fixture math (Class A)
  or the C source being ported (Class B). "It makes the suite pass" is not
  a justification — that is fit-to-engine.
- A Class B port correction that changes expected values MUST state which
  divergence prompted it and why it is not an engine bug.
- Byte-pins change only with a deliberate output-contract change.
- Determinism: expectations MUST NOT depend on wall-clock time. Tests that
  must touch "now" (live edge, relative windows) assert ENVELOPES (bounded
  ranges, row-count bounds), not exact values.
- Protocol emitters (`stream/`) mirror the parser's actual grammar
  (`src/libnetdata/line_splitter/`); extend them when a fixture needs a
  protocol feature, citing the parser code.

## Known boundaries (extension points, not history)

Deliberately out of scope so far; extending into them is welcome and each
states what it takes:

- **KS2 exact tail values**: the ks2 weights oracle pins the engine's
  special cases; a full KSfbar port would make every ks2 weight exact.
- **Natural-points full oracle**: natural mode pins count/values and a
  two-candidate boundary check; a full oracle needs the natural-mode point
  walk ported.
- **Three-tier straddle windows**: plan switching is covered across two
  tiers; a three-tier straddle needs longer synthetic retention.
- **64-bit counter wrap**: unreachable through the signed text protocol;
  needs a different ingestion vector.
- **Float collected values on the reset path**: the v1 SET path parses
  integers; the reset/overflow pins use integer counters only.
- **`points` > 86400**: the API caps points; oversized-grid behavior is
  unpinned.
- **Cloud tier**: replaying the raw halves of L5/L6 through the real cloud
  aggregator is designed but lives outside this repo.

## Gotchas

- Run `go` commands from `tests/query-corpus/` (own module); running from
  the repo root fails with "go.mod not found".
- Values print through the engine's number formatter: a stored
  `22.000000000000004` prints as `22`. Compare parsed numbers, not
  strings, unless the check IS a byte-pin.
- The shared daemon serves ALL tests: never restart or reconfigure it from
  a test — scenario tests that need restarts boot their own daemon
  (`daemon.Start` with `t.TempDir()`).
- After system library upgrades, rebuild `build/netdata` before blaming a
  test failure on the suite.
- IDE diagnostics on the Go files can be stale; `go vet ./...` is the
  authority.
