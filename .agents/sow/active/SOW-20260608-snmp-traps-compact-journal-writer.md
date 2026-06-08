# SOW-20260608-snmp-traps-compact-journal-writer - SNMP Trap Compact Journal Writer

## Status

Status: completed

Sub-state: explicit compact/no-FSS/no-compression writer options implemented
and post-change benchmarks measured.

## Requirements

### Purpose

Ensure SNMP trap journal files are created with the intended low-overhead storage
shape for high-volume trap ingestion: compact journal layout, no Forward Secure
Sealing, and no DATA compression.

### User Request

Set the journal SDK writer to create compact journal files with FSS disabled and
compression disabled, then measure performance after the change.

### Assistant Understanding

Facts:

- The SNMP trap writer currently constructs `sdkjournal.Options` with only
  `MachineID` and `BootID`.
- SDK `Options.Compact` defaults to false, so the branch currently creates
  regular journal files.
- SDK `Options.Seal` defaults to nil, so FSS is disabled today.
- SDK `Options.Compression` defaults to `CompressionNone`, so DATA compression
  is disabled today.

Inferences:

- The safest implementation is to set all three fields explicitly so future SDK
  default changes cannot silently change trap writer behavior.

Unknowns:

- Exact post-change performance must be measured locally after implementation.

### Acceptance Criteria

- SNMP trap journal writer passes `Compact: true` to the SDK.
- SNMP trap journal writer explicitly passes `CompressionNone`.
- SNMP trap journal writer explicitly leaves `Seal` nil.
- Regression test proves the created file has compact header flags and no
  compression or sealing flags.
- Focused Go tests pass.
- Relevant write-path benchmarks are run and reported.

## Analysis

Sources checked:

- `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go`
- `src/go/plugin/go.d/collector/snmp_traps/collector.go`
- `src/go/plugin/go.d/collector/snmp_traps/journal_sdk_test.go`
- `src/go/plugin/go.d/collector/snmp_traps/benchmark_test.go`
- `github.com/netdata/systemd-journal-sdk/go v0.5.1` module cache source

Current state:

- `NewJournalWriter()` uses `sdkjournal.LogConfig{Options: sdkjournal.Options{
  MachineID: machineID, BootID: bootID}}`.
- SDK `Options.Compact` is the compact-layout switch.
- SDK `Options.Seal` is the FSS switch.
- SDK `Options.Compression` is the DATA-compression switch.

Risks:

- Compact journal files cap offsets at 32 bits. The configured rotation size
  is already bounded well below that, so this is not expected to create a new
  production risk.
- Header parsing in tests must stay minimal and local to the tested flags.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The current branch relies on SDK defaults for compact/compression/FSS.
- That means compact is currently false because SDK defaults to regular layout.
- It also means compression and FSS are disabled today only by default, not by an
  explicit product contract in the integration.

Evidence reviewed:

- `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go` constructs
  `sdkjournal.Options` with only `MachineID` and `BootID`.
- SDK `writer.go` documents `Options.Compression` defaulting to
  `CompressionNone`, `Options.Compact` defaulting false, and `Options.Seal`
  enabling FSS only when non-nil.
- SDK `format.go` defines `CompressionNone = 0`,
  `incompatibleCompressedXZ/LZ4/ZSTD`, and `incompatibleCompact`.

Affected contracts and surfaces:

- SNMP trap journal file format and write-path performance.
- SNMP trap journal regression tests.
- Benchmarks for SDK-backed writing and full packet-to-journal ingestion.

Clean-end-state target:

- The SNMP trap writer explicitly configures compact/no-compression/no-FSS.
- A test locks the expected file flags.
- No user-facing configuration changes.

Existing patterns to reuse:

- Existing `journal_sdk_test.go` Linux/journalctl test guards.
- Existing `benchmark_test.go` write-path benchmarks.

Risk and blast radius:

- Low to medium. The change affects only SNMP trap journal files.
- Performance should improve or stay close; exact result must be measured.
- Existing readers must continue to read compact files; SDK and journalctl tests
  cover this path.

Sensitive data handling plan:

- Use synthetic benchmark data only.
- Do not write SNMP communities, device IPs, customer data, credentials,
  private endpoints, or production trap payloads to repo artifacts.

Implementation plan:

1. Set explicit SDK writer options in `NewJournalWriter()`.
2. Add a regression test that writes one entry and inspects the journal header
   flags for compact/no-compression/no-seal.
3. Run focused tests and benchmarks.

Validation plan:

- `go test ./plugin/go.d/collector/snmp_traps -run 'TestNewJournalWriter|TestJournalWriter'`
- `go test ./plugin/go.d/collector/snmp_traps -bench 'Benchmark(JournalWriterWriteEntry|JournalTrapWriterDrain|FullPacketToJournal)$'`

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: no public behavior spec update expected unless tests expose a broader
  contract gap.
- End-user/operator docs: no config or user workflow change expected.
- End-user/operator skills: no operator workflow change expected.
- SOW lifecycle: branch-local SOW only; do not merge to master.

Open-source reference evidence:

- None checked. This is a direct SDK option integration fix, not a protocol or
  ambiguity investigation.

Open decisions:

- None. User explicitly requested compact/no-FSS/no-compression and benchmark.

## Plan

1. Patch writer options and add regression test.
2. Validate with focused tests.
3. Run write-path benchmarks and report numbers.

## Execution Log

### 2026-06-08

- Created local SOW and started implementation.
- Updated `NewJournalWriter()` to pass `Compact: true`,
  `Compression: sdkjournal.CompressionNone`, and `Seal: nil` explicitly.
- Added regression coverage for compact/no-compression/no-FSS journal header
  flags.
- Ran focused tests, full SNMP trap package tests, and write-path benchmarks.
- Added and ran real local UDP listener-to-journal benchmarks after the user
  asked whether end-to-end throughput can be tested.

## Validation

Acceptance criteria evidence:

- `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go` now passes:
  - `Compact: true`
  - `Compression: sdkjournal.CompressionNone`
  - `Seal: nil`
- `TestNewJournalWriterCreatesCompactUnsealedUncompressedJournal` writes an
  entry, reopens the active journal with the SDK reader, and checks:
  - compact incompatible flag is present;
  - compressed incompatible flags are absent;
  - sealed compatible flag is absent.

Tests or equivalent validation:

- `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -run 'TestNewJournalWriter|TestJournalWriter' -count=1 -timeout 120s`
  passed.
- `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -run 'TestNewJournalWriterCreatesCompactUnsealedUncompressedJournal' -count=1 -v -timeout 120s`
  passed.
- `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 180s`
  passed.

Post-change benchmark command:

- `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench 'Benchmark(JournalWriterWriteEntry|JournalTrapWriterDrain|FullPacketToJournal)$' -benchmem -benchtime=3s -count=3 -timeout 30m`

Post-change benchmark results:

- `BenchmarkJournalWriterWriteEntry`: `203517`, `202302`, `187149`
  entries/s; median `202302` entries/s.
- `BenchmarkJournalTrapWriterDrain`: `64857`, `75457`, `64627` entries/s;
  median `64857` entries/s.
- `BenchmarkFullPacketToJournal`: `57368`, `54311`, `48974`
  persisted entries/s; median `54311` persisted entries/s.

End-to-end UDP listener validation:

- `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -run '^TestCollectorReplayPcapThroughListenerToJournal$' -count=1 -v -timeout 120s`
  passed.
- Added `BenchmarkUDPPacketToJournal`, covering local UDP socket,
  `Listener.readLoop`, `Collector.handlePacket`, journal trap writer queue, and
  SDK-backed journal persistence.
- Added `BenchmarkUDPPacketToJournalPaced` for controlled local UDP rates.
- `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -run '^TestCollectorReplayPcapThroughListenerToJournal$' -bench '^BenchmarkUDPPacketToJournal(Paced)?$' -benchmem -benchtime=3s -count=1 -timeout 30m`
  passed.
- Burst run: `62129` persisted entries/s, but with `85.14%` drop rate because
  the sender flooded at `418114` packets/s.
- Paced `25000pps`: `24949` persisted entries/s, `0%` drop rate.
- Paced `50000pps`: `49821` persisted entries/s, `0%` drop rate.
- Paced `75000pps`: `58778` persisted entries/s, `15.68%` drop rate in the
  first run.
- Paced `100000pps`: `61202` persisted entries/s, `33.86%` drop rate.
- Confirmation command:
  `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkUDPPacketToJournalPaced/(50000|75000)pps$' -benchmem -benchtime=3s -count=2 -timeout 30m`
  passed.
- Confirmation results:
  - `50000pps`: `49835` persisted entries/s with `0.001328%` drop rate
    (`2` drops), then `49910` persisted entries/s with `0%` drop rate.
  - `75000pps`: `67784` persisted entries/s with `2.839%` drop rate, then
    `68038` persisted entries/s with `1.240%` drop rate.

End-to-end interpretation:

- Sustained local UDP listener-to-journal ingestion is confirmed at about
  `50k` traps/s with zero or near-zero loss on this workstation.
- Pushing the local paced sender to about `69k-70k` packets/s starts showing
  measurable loss.
- Flooding the local UDP path as fast as possible can persist about `62k`
  traps/s, but this is overload behavior, not a no-loss operating point.

Real-use evidence:

- Benchmarks write queryable journal rows and count them with
  `journalctl --directory`.

Reviewer findings:

- No external review requested for this small, focused fix.

Same-failure / validation-scope scan:

- The only SNMP trap journal writer construction path is `NewJournalWriter()`;
  collector init and benchmarks call through it.
- The previous `BenchmarkFullPacketToJournal` bypassed the UDP socket/listener
  path, so `BenchmarkUDPPacketToJournal` and
  `BenchmarkUDPPacketToJournalPaced` were added for real local UDP receive
  coverage.

Sensitive data gate:

- Durable artifacts contain only synthetic test/benchmark data and repo paths.

## Artifact Maintenance Gate

- AGENTS.md: no update needed; workflow unchanged.
- Runtime project skills: no update needed; no new repo workflow.
- Specs: no update needed; this locks an implementation detail in tests.
- End-user/operator docs: no update needed; no user-facing configuration or
  workflow changed.
- End-user/operator skills: no update needed; no operator workflow changed.
- SOW lifecycle: branch-local SOW only; delete before merge.

Specs update:

- No spec update needed; test now captures the writer option contract.

Project skills update:

- No project skill update needed.

End-user/operator docs update:

- No docs update needed.

End-user/operator skills update:

- No public skill update needed.

Lessons:

- Avoid relying on SDK defaults for product-level file format decisions.

Follow-up mapping:

- No code follow-up needed from this measurement.
- A separate real-use validation SOW already exists for lab/device validation
  outside this synthetic local UDP benchmark scope.

## Outcome

Implemented and benchmarked.

## Lessons Extracted

Writer format defaults that matter for performance and compatibility should be
explicit at integration boundaries and locked with a regression test.

## Follow-up Issues

- No code follow-up required from this measurement.
- A separate real-use validation SOW already exists for lab/device validation
  outside this synthetic local UDP benchmark scope.
