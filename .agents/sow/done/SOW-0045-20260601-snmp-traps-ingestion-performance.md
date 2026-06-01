# SOW-0045 - SNMP Trap Ingestion Performance

## Status

Status: completed

Sub-state: completed on 2026-06-01. Implementation, local validation, commit, push, and final SOW-0039 close-gate review are complete.

## Requirements

### Purpose

Make SNMP trap ingestion fit for production by removing avoidable hot-path overhead between decoded trap entries and SDK journal append, while preserving journal field semantics, `journalctl --follow` visibility, creation-time failure detection, and the operator-facing trap query contract.

### User Request

The user asked to create a new SOW and optimize the SNMP trap ingestion pipeline because the SDK direct writer can write more than 150K entries/sec while the current ingestion pipeline is much slower.

### Assistant Understanding

Facts:

- The branch currently pins `github.com/netdata/systemd-journal-sdk/go v0.4.0`.
- Current direct SDK-oriented `BenchmarkJournalWriterWriteEntry` evidence is about 147K-179K entries/sec.
- Current queued writer evidence is about 52K-65K entries/sec.
- Current full packet-to-journal evidence is about 30K-38K persisted entries/sec on the latest repeat.
- The queued writer and serializer are Netdata code, not SDK code.
- `TRAP_JSON`, `TRAP_*` fields, creation-time writer preflight, and live journal queryability are part of the shipped trap contract.

Inferences:

- A significant share of the gap is avoidable in Netdata code: per-entry `[]JournalField` allocation, `TRAP_JSON` map/sort/JSON construction, label sorting, string/byte conversions, channel handoff, and single-worker serialization.
- SDK append/file-growth behavior still contributes to CPU time, but direct writer evidence shows the SDK alone is not the main full-pipeline bottleneck.

Unknowns:

- Portable throughput across slower storage and different CPUs remains unknown; local evidence is workstation benchmark evidence, not a hardware guarantee.

### Acceptance Criteria

- Preserve serialized journal field behavior for existing tests, including duplicate `TRAP_JSON` key suffixing, sorted deterministic JSON keys, sorted `TRAP_TAG_*` labels, summary records, and CWE-117 sanitized-field counting.
- Reduce allocations in `BenchmarkJournalTrapWriterDrain` and `BenchmarkFullPacketToJournal` versus the latest baseline.
- Improve or at minimum not regress persisted entries/sec in `BenchmarkJournalTrapWriterDrain` and `BenchmarkFullPacketToJournal` over repeated 30,000-entry runs.
- Keep `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` passing.
- Keep `journalctl --directory` validation in `BenchmarkFullPacketToJournal` passing with no loss of queryable rows beyond existing benchmark variability.

## Analysis

Sources checked:

- `src/go/plugin/go.d/collector/snmp_traps/trapwriter_impl.go`
- `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go`
- `src/go/plugin/go.d/collector/snmp_traps/serialize.go`
- `src/go/plugin/go.d/collector/snmp_traps/benchmark_test.go`
- `.agents/sow/current/SOW-0035-20260525-snmp-traps-foundation-mvp.md`
- `.agents/sow/current/SOW-0039-20260525-snmp-traps-bundle-facets-docs-merge-gate.md`

Current state:

- `journalTrapWriter` owns the queue and worker (`trapwriter_impl.go`).
- `writeOne()` serializes each `TrapEntry` into fields on every write, then calls `JournalWriter.WriteEntry()`.
- `serializeToJournalFields()` allocates a fresh field slice, builds `TRAP_JSON` through maps/slices/sorting/`json.Marshal`, and sorts labels.
- The SDK `Writer.Append()` consumes field bytes synchronously during append; it does not retain caller field slices after the call. This makes per-worker scratch reuse a safe optimization boundary.

Risks:

- Reusing buffers incorrectly could corrupt journal rows if the SDK retained caller bytes. Verified SDK append consumes payloads synchronously before returning.
- Rewriting JSON generation could alter `TRAP_JSON` byte order or duplicate-key suffix behavior. Existing serialization tests must remain the contract.
- Removing or delaying sync/publish behavior could break `journalctl --follow`; this SOW will not change the configured flush cadence or SDK live-publish semantics.
- Parallel writers for a single journal may violate SDK `Log` single-writer expectations and increase ordering complexity; this SOW does not introduce parallel journal writes.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The direct SDK writer can append prebuilt fields much faster than the current trap ingestion path.
- The current Netdata writer path rebuilds field slices and JSON payloads per trap and performs many small allocations before reaching SDK append.
- Profiling showed `serializeToJournalFields()` and `buildTrapJSON()` are large allocation sources, while SDK append remains significant but not sufficient to explain the full gap alone.

Evidence reviewed:

- `trapwriter_impl.go`: single queue worker, per-entry `serializeToJournalFields()`, sync every `defaultFlushEntries`.
- `serialize.go`: fresh fields, JSON map, duplicate tracking map, sorted keys, per-value `json.Marshal`.
- `journal_writer.go`: adapter counts sanitized fields then calls SDK `Log.Append`.
- `benchmark_test.go`: direct writer precomputes fields once; queued writer serializes each entry.
- SOW-0035/SOW-0039 latest benchmark evidence: full path about 30K-38K/sec latest, direct writer about 147K-179K/sec latest.

Affected contracts and surfaces:

- Go SNMP trap collector internal writer implementation.
- Journal field serialization contract and tests.
- Performance benchmark evidence in SOW-0035/SOW-0039 and comparative spec.
- No public configuration, schema, docs, or operator workflow changes are intended.

Existing patterns to reuse:

- Existing focused benchmarks in `benchmark_test.go`.
- Existing serialization tests in `serialize_test.go`.
- Existing `TrapWriter` abstraction.
- Single worker ownership of serialization state, which allows simple scratch reuse without `sync.Pool`.

Risk and blast radius:

- Blast radius is limited to SNMP trap journal writer serialization and benchmark tests.
- Main regression risk is journal row content drift. Mitigation: preserve existing tests and add focused tests if needed.
- Performance measurements are noisy on the workstation. Mitigation: run repeated benchmark batches and record ranges, not single best numbers.
- Sensitive data risk is low because tests use synthetic/documentation IPs and no secrets.

Sensitive data handling plan:

- Durable SOW/spec/docs will contain only synthetic benchmark data, relative file paths, and command names.
- No SNMP communities, real device IPs, customer identifiers, bearer tokens, or private endpoints will be written to SOWs, specs, docs, skills, code comments, or tests.

Implementation plan:

1. Add a reusable serializer state owned by `journalTrapWriter`'s single worker.
2. Rewrite journal field construction to reuse field slices, JSON buffers, duplicate tracking, and sort slices while preserving output semantics.
3. Avoid `fmt.Sprintf` and generic `json.Marshal` in hot scalar paths where simple appends are sufficient.
4. Preserve SDK append, sync cadence, creation-time validation, and journal queryability.
5. Re-run focused tests and repeated benchmarks; update SOW/spec performance evidence with the real measured result.

Validation plan:

- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s`
- `go test ./plugin/go.d/collector/snmp_traps -run '^TestSerializeToJournalFields|^TestJournalWriter|^TestCollectorReplayPcapThroughListenerToJournal' -count=1 -timeout 120s`
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^Benchmark(JournalTrapWriterDrain|JournalWriterWriteEntry|FullPacketToJournal)$' -benchmem -benchtime=30000x -count=3 -timeout 120s`
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=30000x -count=3 -timeout 120s` repeated if first result is noisy.
- `go mod tidy -diff`
- `git diff --check`

Artifact impact plan:

- AGENTS.md: no expected update; project workflow is unchanged.
- Runtime project skills: no expected update unless a durable trap-writer performance lesson is extracted.
- Specs: update SNMP trap comparative/performance spec evidence if benchmark numbers change materially.
- End-user/operator docs: no expected update; no operator-visible behavior/config changes.
- End-user/operator skills: no expected update; query workflow unchanged.
- SOW lifecycle: complete this SOW with implementation, validation, lessons, and follow-up mapping when accepted.

Open-source reference evidence:

- None checked. This SOW optimizes local hot-path implementation already constrained by Netdata's journal field contract and SDK API; external monitoring implementations do not inform this internal allocation path.

Open decisions:

- No blocking user decisions. The user explicitly requested optimization and the implementation preserves existing behavior.

## Implications And Decisions

1. Decision: preserve `TRAP_JSON` and all current journal fields.
   - Selected: yes.
   - Reason: removing fields would be faster but would break the operator query and documentation contract.

2. Decision: do not change SDK live-publish/sync semantics in this SOW.
   - Selected: yes.
   - Reason: write speed is important, but users must be able to see traps promptly through `journalctl --follow` and Netdata logs functions.

3. Decision: optimize single-worker serialization before considering parallel writers.
   - Selected: yes.
   - Reason: the current bottleneck is avoidable allocation/serialization work; parallel writers would add ordering, locking, and SDK contract risks.

## Plan

1. Establish baseline and implementation constraints from current benchmarks/profiles.
2. Add reusable serializer state and route `journalTrapWriter.writeOne()` through it.
3. Keep the public `serializeToJournalFields()` function as the simple allocation-returning API for tests and non-hot callers.
4. Run focused tests and benchmarks; iterate on regressions.
5. Update SOW/spec performance evidence.

## Execution Log

### 2026-06-01

- Created this SOW after current benchmark evidence showed the full trap ingestion path remained far below direct SDK append throughput.
- Added `JournalWriter.WriteRawEntry()` so the trap writer can pass prebuilt `KEY=value` payloads to the SDK without allocating a `[]JournalField` per trap.
- Added a reusable `journalHotSerializer` owned by the single `journalTrapWriter` worker. It reuses payload, byte, label-key, JSON-entry, and duplicate-key state across trap entries.
- Replaced the hot path's `buildTrapJSON()` map/`json.Marshal` construction with direct JSON object emission for ordinary trap rows while keeping the existing `buildTrapJSON()` path for summary rows.
- Added a hot-serializer parity test against `serializeToJournalFields()` covering ordinary traps, duplicate JSON keys, summary rows, sanitized fields, JSON escaping, bytes, floats, bools, nil values, and invalid UTF-8.
- Removed one full-path varbind copy by letting `trapEntryFromPDU()` take ownership of the decoded PDU varbind slice and resolve profile metadata in place before queueing the immutable entry.
- Profiled the optimized path. Current writer-side allocations are no longer the dominant full-path allocation source; `gosnmp` decode and Netdata varbind conversion dominate full packet-to-journal allocation, while SDK append/live publication remains the largest writer-side CPU component.

## Validation

Acceptance criteria evidence:

- Serialized journal field behavior is preserved by `TestJournalHotSerializerMatchesSerializeToJournalFields`.
- `BenchmarkJournalTrapWriterDrain` improved from the latest pre-SOW-0045 repeat of 52.1K-64.6K entries/sec, 3,242 B/op, 42 allocs/op to 61.9K-74.2K entries/sec, 577 B/op, 7 allocs/op in the final isolated 30,000-entry run.
- `BenchmarkFullPacketToJournal` improved from the latest pre-SOW-0045 repeat of 30.5K-38.0K persisted entries/sec, 11,588-11,590 B/op, 192 allocs/op to 62.5K-72.6K persisted entries/sec, 5,202 B/op, 128 allocs/op in the final isolated 30,000-packet run.
- The longer 100,000-packet full-path run measured 63.3K-66.0K persisted entries/sec, 5,200-5,201 B/op, 128 allocs/op.

Tests or equivalent validation:

- `go test ./plugin/go.d/collector/snmp_traps -run '^TestJournalHotSerializerMatchesSerializeToJournalFields$|^TestSerializeToJournalFields' -count=1 -timeout 120s` — passed.
- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` — passed.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^Benchmark(TrapWriterWrite|JournalTrapWriterDrain|JournalWriterWriteEntry|FullPacketToJournal)$' -benchmem -benchtime=30000x -count=3 -timeout 120s` — passed.
  - `BenchmarkTrapWriterWrite`: 204.3-297.0 ns/op, 3.37M-4.89M entries/sec, 0 B/op, 0 allocs/op.
  - `BenchmarkJournalTrapWriterDrain`: 13.47-16.15 us/op, 61.9K-74.2K entries/sec, 577 B/op, 7 allocs/op.
  - `BenchmarkJournalWriterWriteEntry`: 5.45-6.01 us/op, 166K-184K entries/sec, 825 B/op, 5 allocs/op.
  - `BenchmarkFullPacketToJournal`: 13.78-16.01 us/op, 62.5K-72.6K persisted entries/sec, 0 drops, 5,202 B/op, 128 allocs/op.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=100000x -count=3 -timeout 180s` — passed.
  - 100,000 packets/run: 15.16-15.81 us/op, 63.3K-66.0K persisted entries/sec, 0-1 drops/run, 5,200-5,201 B/op, 128 allocs/op.

Real-use evidence:

- `BenchmarkFullPacketToJournal` remains the real-use proxy for this optimization: it verifies synthetic packet decode through journal append and uses `journalctl --directory` row-count validation after the timed section.

Reviewer findings:

- Final SOW-0039 close-gate review covered this SOW together with SOW-0035, SOW-0036, SOW-0037, and SOW-0039.
- `qwen` found no blockers and accepted the SOW-0045 performance evidence as sufficient.
- `glm` found no SOW-0045 performance blocker; its only close blocker was an RPM file-list gap for `snmp-trap-profile-gen`, fixed in the SOW-0039 closeout.
- `kimi` found no blockers and listed only cosmetic code-quality notes.
- `minimax` did not produce a usable final review after starting a read-only review, so it is recorded as no final output.

Same-failure scan:

- Profile scan after optimization shows the previous same-failure class, per-entry hot writer serialization allocation, is no longer dominant. Remaining allocation hot spots are mostly `gosnmp` decode, Netdata varbind conversion, and SDK metadata payload append.

Sensitive data gate:

- Passed. Only synthetic documentation IPs, benchmark numbers, relative paths, and command names were written to durable artifacts.

Artifact maintenance gate:

- AGENTS.md: no update needed; workflow and guardrails unchanged.
- Runtime project skills: no update needed; this is a local implementation optimization, and the existing collector skill already covers hot-path allocation discipline.
- Specs: updated SNMP trap design/spec evidence and ADR notes to record the new measured performance and hot writer boundary.
- End-user/operator docs: no update needed; no configuration, workflow, field, or operator-visible behavior changed.
- End-user/operator skills: no update needed; trap query workflow and fields are unchanged.
- SOW lifecycle: this SOW is completed and moved to `.agents/sow/done/` with the final closeout. No required deferred work is hidden.

Specs update:

- Updated in this branch to replace stale 30K-50K v0.4.0 performance interpretation with SOW-0045 evidence.

Project skills update:

- No project skill update needed; no durable developer workflow changed.

End-user/operator docs update:

- No operator docs update needed; no operator-facing behavior changed.

End-user/operator skills update:

- No operator skill update needed; no query workflow or public skill behavior changed.

Lessons:

- The useful optimization boundary was the single trap writer worker: it can own reusable serialization state without `sync.Pool` and without cross-goroutine aliasing.
- Direct SDK append benchmarks are not full ingestion benchmarks. After SOW-0045, the remaining gap is mostly SDK append/live publication plus packet decode/varbind conversion, not the previous per-entry journal-field/JSON allocation pattern.

Follow-up mapping:

- No required follow-up for this SOW. Optional future work, if a higher target is required, should be a new SOW focused on SDK trusted/batched append APIs or replacing/reducing `gosnmp` decode allocations.

## Outcome

Implemented and locally validated.

The SNMP trap full packet-to-journal path is now roughly back in the SDK `go/v0.3.0` historical range while using SDK `go/v0.4.0`, but with much lower allocation:

- Latest pre-SOW-0045 `go/v0.4.0` repeat: 30.5K-38.0K persisted entries/sec, 11,588-11,590 B/op, 192 allocs/op.
- Final SOW-0045 30,000-packet run: 62.5K-72.6K persisted entries/sec, 5,202 B/op, 128 allocs/op.
- Final SOW-0045 100,000-packet run: 63.3K-66.0K persisted entries/sec, 5,200-5,201 B/op, 128 allocs/op.

## Lessons Extracted

- The trap writer's single worker is a good place for scratch-state ownership. It avoids `sync.Pool`, avoids locks, and keeps SDK append synchronous.
- `TRAP_JSON` compatibility needs explicit tests because direct JSON emission can silently drift from `encoding/json` behavior.
- The remaining path to 150K/sec is not another small local serializer tweak. It likely needs SDK API changes, live-publish cadence changes, or decode-path work with a larger blast radius.

## Followup

No required follow-up.

Optional future SOW candidates if a materially higher target is required:

- SDK trusted append or batch append API that avoids per-entry field validation/metadata allocation while preserving journal queryability.
- Decode-path allocation reduction or a lower-allocation SNMP trap parser, because `gosnmp` decode and varbind conversion now dominate full-path allocation.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
