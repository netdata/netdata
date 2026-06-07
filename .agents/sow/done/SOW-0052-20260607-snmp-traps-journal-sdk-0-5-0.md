# SOW-0052 - SNMP Traps Journal SDK v0.5.0 Update

## Status

Status: completed

Sub-state: completed on 2026-06-07. SDK dependency update, module tidy, tests, benchmarks, SOW closeout, commit, and push are complete.

## Requirements

### Purpose

Keep SNMP trap journal ingestion on the current systemd-journal SDK release while preserving creation-time failure detection, journal queryability, retention behavior, and ingestion performance evidence.

### User Request

Use `github.com/netdata/systemd-journal-sdk/go v0.5.0` after the SDK release was updated.

### Assistant Understanding

Facts:

- The branch currently pins `github.com/netdata/systemd-journal-sdk/go v0.4.0` in `src/go/go.mod`.
- The module resolver lists `v0.5.0` as an available release.
- The local SDK tag `go/v0.5.0` exists at `netdata/systemd-journal-sdk @ 7ab81ec9a74cd4aea8dd5da8342a35d03febd390`.
- SNMP traps currently use the SDK writer through `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go`.
- The SDK public writer API still exposes `NewLog`, `LogConfig`, `Options`, `EntryOptions`, `Field`, `Append`, `AppendRaw`, `Sync`, `Close`, `EnforceRetention`, `JournalDirectory`, and `ActivePath` at tag `go/v0.5.0`.

Inferences:

- This is likely a surgical dependency bump with compile/test validation; code changes are only needed if the v0.5.0 compiler/API behavior exposes an integration mismatch.
- Because SOW-0045 performance work depended on SDK writer behavior, benchmark evidence should be refreshed after the bump.

Unknowns:

- Whether v0.5.0 changes runtime performance materially on this workstation; benchmark evidence must be re-measured.
- Whether transitive module changes from v0.5.0 require `go mod tidy` adjustments.

### Acceptance Criteria

- `src/go/go.mod` uses `github.com/netdata/systemd-journal-sdk/go v0.5.0`.
- SNMP trap journal writer compiles and preserves eager open, strict host identity, retention, rotation, raw-entry append, sync, active-path, and sanitized-field behavior.
- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` passes from `src/go`.
- `go mod tidy -diff` passes from `src/go`.
- Journal-focused benchmark evidence is re-run and recorded in this SOW.

## Analysis

Sources checked:

- `src/go/go.mod`
- `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go`
- `.agents/sow/done/SOW-0045-20260601-snmp-traps-ingestion-performance.md`
- `netdata/systemd-journal-sdk @ 7ab81ec9a74cd4aea8dd5da8342a35d03febd390` `go/API.md`
- `netdata/systemd-journal-sdk @ 7ab81ec9a74cd4aea8dd5da8342a35d03febd390` `go/journal/log.go`
- `netdata/systemd-journal-sdk @ 7ab81ec9a74cd4aea8dd5da8342a35d03febd390` `go/journal/writer.go`

Current state:

- Current dependency is `v0.4.0`.
- SDK `go/API.md` still says the expected consumable tag is `go/v0.3.0`; this appears stale because tags `go/v0.4.0` and `go/v0.5.0` both exist and the module resolver lists `v0.5.0`.
- The v0.5.0 API adds reader/explorer/function surfaces and file-mode documentation, while the writer surfaces consumed by SNMP traps remain present.

Risks:

- The SDK implementation changed substantially between v0.4.0 and v0.5.0, so compile success alone is insufficient.
- Journal ingestion throughput may change even if public APIs remain stable.
- Incorrect adaptation could weaken the required job-creation-time failure detection for journal directory/file creation.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- SNMP traps currently import SDK `v0.4.0`.
- The user requested moving to SDK `v0.5.0`.
- The likely root work is dependency drift, not a product behavior change: align the module version and validate the existing adapter against the new SDK implementation.

Evidence reviewed:

- `src/go/go.mod` pins `github.com/netdata/systemd-journal-sdk/go v0.4.0`.
- `go list -m -versions github.com/netdata/systemd-journal-sdk/go` lists `v0.1.0 v0.2.0 v0.3.0 v0.4.0 v0.5.0`.
- `netdata/systemd-journal-sdk @ 7ab81ec9a74cd4aea8dd5da8342a35d03febd390` exposes the writer functions and types consumed by `journal_writer.go`.
- SOW-0045 records the previous post-optimization benchmark evidence that must be refreshed after SDK changes.

Affected contracts and surfaces:

- Go module dependency graph under `src/go`.
- SNMP traps journal writer adapter and tests.
- Trap journal file creation/open behavior at job creation time.
- Trap journal retention/rotation behavior.
- Benchmark evidence in this SOW.

Existing patterns to reuse:

- Existing `NewJournalWriter` wrapper in `journal_writer.go`.
- Existing SDK eager-open and strict-identity configuration.
- Existing journal tests in `journal_sdk_test.go` and collector end-to-end tests.
- Existing SOW-0045 benchmark commands for apples-to-apples comparison.

Risk and blast radius:

- Blast radius should be limited to Go module dependency state and SNMP traps journal ingestion.
- Runtime risk is medium because the SDK writer implementation changed; validation must include journalctl queryability and benchmark row counts, not only unit tests.
- Operator-facing risk is low if no config/schema/docs behavior changes are made.

Sensitive data handling plan:

- Use only synthetic test traps, benchmark numbers, relative file paths, command names, module versions, and upstream commit IDs.
- Do not write raw SNMP communities, USM secrets, device identifiers, live public IPs, customer identifiers, bearer tokens, or private endpoints to SOWs, specs, docs, skills, instructions, or code comments.

Implementation plan:

1. Update `src/go` module dependency to `github.com/netdata/systemd-journal-sdk/go v0.5.0`.
2. Run `go mod tidy -diff`; apply only required module file changes.
3. Compile and test `snmp_traps`; adapt `journal_writer.go` only if v0.5.0 requires it.
4. Run focused journal writer tests and journalctl-backed benchmark evidence.
5. Update this SOW with validation, artifact gates, and close if successful.

Validation plan:

- `go mod tidy -diff`
- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s`
- `go test ./plugin/go.d/collector/snmp_traps -run '^TestJournalWriter|^TestCollectorReplayPcapThroughListenerToJournal' -count=1 -timeout 120s`
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^Benchmark(JournalTrapWriterDrain|JournalWriterWriteEntry|FullPacketToJournal)$' -benchmem -benchtime=30000x -count=3 -timeout 180s`
- `git diff --check`
- `.agents/sow/audit.sh`

Artifact impact plan:

- AGENTS.md: no expected update; workflow and guardrails unchanged.
- Runtime project skills: no expected update; collector authoring workflow unchanged.
- Specs: no expected update unless v0.5.0 changes shipped trap behavior.
- End-user/operator docs: no expected update unless config, query, retention, or journal visibility behavior changes.
- End-user/operator skills: no expected update unless query workflow changes.
- SOW lifecycle: complete this SOW with implementation, validation, lessons, and follow-up mapping; move to `done/` with the implementation commit.

Open-source reference evidence:

- `netdata/systemd-journal-sdk @ 7ab81ec9a74cd4aea8dd5da8342a35d03febd390`:
  - `go/API.md`
  - `go/journal/log.go`
  - `go/journal/writer.go`

Open decisions:

- No blocking user decision. This is a surgical dependency update requested by the user.

## Implications And Decisions

1. Decision: perform a surgical update, not a local writer redesign.
   - Selected: yes.
   - Reason: the requested change is to use SDK v0.5.0; writer redesign would expand scope and risk.

2. Decision: refresh trap ingestion benchmarks after the bump.
   - Selected: yes.
   - Reason: SOW-0045 performance evidence depends on SDK writer behavior.

## Plan

1. Bump the SDK module and apply required `go.mod`/`go.sum` changes.
2. Run focused compile/tests; adapt code only on concrete compiler/runtime evidence.
3. Run benchmark evidence and update this SOW.
4. Close the SOW with artifact gates and validation evidence.

## Execution Log

### 2026-06-07

- Created this SOW after verifying the branch pinned `v0.4.0` and the module resolver exposed `v0.5.0`.
- Updated `src/go/go.mod` from `github.com/netdata/systemd-journal-sdk/go v0.4.0` to `v0.5.0`.
- Accepted the Go directive update from `go 1.26.0` to `go 1.26.3` because SDK `go/v0.5.0` declares `go 1.26.3`; the main module must not be lower than a dependency's declared Go version.
- Applied `go mod tidy`; stale `v0.4.0` checksums were removed from `src/go/go.sum`.
- No SNMP trap source-code adaptation was needed. The v0.5.0 writer API still matches the existing adapter.

## Validation

Acceptance criteria evidence:

- `src/go/go.mod` now uses `github.com/netdata/systemd-journal-sdk/go v0.5.0`.
- `src/go/go.sum` now contains only the v0.5.0 SDK checksums for this module.
- The existing `NewJournalWriter` adapter compiled and passed tests without code changes, preserving eager open, strict identity, retention/rotation, raw append, sync, active path, and sanitized-field behavior.

Tests or equivalent validation:

- `go mod tidy -diff` from `src/go` - passed.
- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` from `src/go` - passed.
- `go test ./plugin/go.d/collector/snmp_traps -run '^TestJournalWriter|^TestCollectorReplayPcapThroughListenerToJournal' -count=1 -timeout 120s` from `src/go` - passed.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^Benchmark(JournalTrapWriterDrain|JournalWriterWriteEntry|FullPacketToJournal)$' -benchmem -benchtime=30000x -count=3 -timeout 180s` from `src/go` - passed:
  - `BenchmarkJournalTrapWriterDrain`: 111.9K-116.8K entries/sec, 577 B/op, 7 allocs/op.
  - `BenchmarkJournalWriterWriteEntry`: 171.3K-177.7K entries/sec, 825 B/op, 5 allocs/op.
  - `BenchmarkFullPacketToJournal`: 93.7K-95.3K persisted entries/sec, 0-1 drops/run, 5,202 B/op, 128 allocs/op.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=100000x -count=3 -timeout 180s` from `src/go` - passed:
  - `BenchmarkFullPacketToJournal`: 74.9K-83.4K persisted entries/sec, 1-8 drops/run, 5,200-5,201 B/op, 128 allocs/op.
- `git diff --check` - passed.
- `.agents/sow/audit.sh` - passed with the repository's existing non-project skill classification warnings only.

Real-use evidence:

- The journalctl-backed tests and `BenchmarkFullPacketToJournal` validate the runnable trap path used here: synthetic SNMP packet decode, trap pipeline processing, SDK journal append, and `journalctl --directory` row visibility.

Reviewer findings:

- No external reviewers yet; this is a narrow dependency update. Run reviewers only if source changes expand beyond the dependency/module adaptation or a blocker appears.

Same-failure scan:

- `rg -n "github.com/netdata/systemd-journal-sdk/go v0\\.4\\.0|github.com/netdata/systemd-journal-sdk/go v0\\.5\\.0|go 1\\.26\\.3" src/go/go.mod src/go/go.sum .agents/sow/current/SOW-0052-20260607-snmp-traps-journal-sdk-0-5-0.md` confirmed the active module files reference v0.5.0 and `go 1.26.3`; remaining v0.4.0 references are SOW historical evidence only.

Sensitive data gate:

- Passed. Durable artifacts contain only module versions, checksums, command names, benchmark numbers, relative file paths, and upstream commit IDs. No SNMP communities, USM secrets, device identifiers, live public IPs, customer identifiers, bearer tokens, or private endpoints were written.

Artifact maintenance gate:

- AGENTS.md: no update needed; repository workflow and guardrails did not change.
- Runtime project skills: no update needed; collector authoring workflow did not change.
- Specs: no update needed; shipped trap behavior and public contract did not change.
- End-user/operator docs: no update needed; config, query, retention, and journal visibility behavior did not change.
- End-user/operator skills: no update needed; operator query workflow did not change.
- SOW lifecycle: this SOW was marked `Status: completed`, moved to `.agents/sow/done/`, and committed with the module changes.

Specs update:

- No spec update needed; the dependency update did not change product behavior or public trap schema.

Project skills update:

- No project skill update needed; no reusable workflow or guardrail changed.

End-user/operator docs update:

- No end-user/operator docs update needed; no operator-visible behavior changed.

End-user/operator skills update:

- No end-user/operator skills update needed; no query or operational workflow changed.

Lessons:

- SDK minor updates can still raise the main module Go directive when the dependency's `go.mod` raises its own directive. Check the SDK submodule `go.mod` before treating a Go directive change as accidental.

Follow-up mapping:

- No follow-up created. No source adaptation, docs changes, or new validation gaps were found.

## Outcome

SNMP traps now use `github.com/netdata/systemd-journal-sdk/go v0.5.0`. The existing journal adapter works unchanged, tests pass, and refreshed benchmarks show healthy persisted journal throughput.

## Lessons Extracted

- Record the dependency module's own Go directive as evidence when a dependency bump changes the main module `go` version.

## Followup

None yet.

## Regression Log

None yet.
