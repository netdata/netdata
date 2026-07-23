---
name: project-query-corpus
description: Developer contract for the query contract corpus (tests/query-corpus) — the black-box correctness suite for the Netdata query engine. Use when running the corpus, adding or extending corpus cases, authoring fixtures, changing an oracle or a byte-pin, adding a red case for a query-engine bug, flipping a red case green after a fix merges, or validating a query-engine fix branch against the corpus.
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

## Correctness model — why a green suite means something

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
    explicitly: either it is an engine bug (author a red case) or an engine
    quirk adopted into the oracle **with a recorded pending ruling** in the
    manifest/SOW. Silently adjusting a port until the engine passes is the
    cardinal sin of this suite ("fit-to-engine") and is prohibited.
  - Where a port could hide drift, bound it independently (e.g. L2 pins the
    SN quantization error envelope against the ORIGINAL values).
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
there are exactly two exits — a red case (engine bug) or a recorded ruling
(intended behavior, pinned green with the quirk documented). There is no
third option where the oracle is quietly bent to match.

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

## The manifest ledger

Every contract case has an entry in `manifest.go` (`Proves`, `Agent:
Green|Red`, optional `FixedBy`) mirrored as a row in `MANIFEST.md`. Tests
report through `expectAgentStatus(t, name, observedPass)`:

- **Green** = the contract holds; an observed failure is a regression and
  fails the suite.
- **Red** = a known, deterministic bug reproduction; the case PASSES while
  the bug reproduces and **fails loudly the moment the bug stops
  reproducing**, demanding the flip. A red case is therefore also the
  detector of its own fix.

A case name is `<layer-or-CASE-id>/<slug>`; `Proves` is one sentence a
maintainer can read as the contract claim. Cases flipped green keep their
test as the regression guard, with `FixedBy: "#PR"`.

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
- **The full suite MUST be green before every push of the corpus branch.**

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

## Adding a green case

1. Author the fixture (Class A first; reach for a Class B oracle only when
   the transform requires it).
2. Push it (`pushLiveBurst` for live bursts; paced v1 or replication where
   the ingestion path is the thing under test), settle, query.
3. Compute expectations in Go from the fixture definition. Never paste a
   number you got from the engine.
4. Add the manifest entry (`Agent: Green`) and the `MANIFEST.md` row.
5. Run the full suite; a new case MUST NOT destabilize existing cases
   (watch for GUID collisions and shared-host mutations).

## Adding a red case (bug workflow)

1. Reproduce the divergence deterministically in its own `caseNNN_test.go`
   with a minimal fixture. The check asserts the CORRECT behavior and
   feeds the observed result into `expectAgentStatus` — so the case passes
   (red-as-expected) on today's daemon and screams when the bug is fixed.
2. Add the manifest entry with `Agent: Red` and a `Proves` sentence that
   states the bug precisely (what is wrong, where, and what correct is).
3. The fix goes in its OWN branch/PR — never mixed into the corpus branch.
4. Validate the fix branch against the corpus before opening the PR:
   - build the fix branch, save the binary aside;
   - from the corpus checkout:
     `QUERY_CORPUS_NETDATA=<fix-binary> go test -count=1 -run '<the case
     plus neighboring pins>' .`
   - the red case MUST fail with "expected RED but the bug no longer
     reproduces", and every other pin MUST stay green (zero collateral).
5. When the fix merges: rebase the corpus branch onto the merge, flip the
   case (`Agent: Green, FixedBy: "#PR"`), reword the case comment and
   `Proves` to past tense, run the full suite, push. The case lives on as
   the regression guard.
6. If the divergence is ruled intended behavior instead: pin it green,
   document the quirk in the oracle comment and the `Proves` text, and
   record the ruling.

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
