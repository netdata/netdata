# SOW-0015 - Static Journal Facet Filtering

## Status

Status: completed

Sub-state: PR review cleanup completed.

## Requirements

### Purpose

Restore correct systemd journal facet filtering for static Netdata Agent builds that use the Rust journal provider, especially on distributions whose journal files contain LZ4-compressed data objects. Keep the fix small, source-verified, and isolated to the clean worktree created from `master`.

### User Request

The user reported that the prior session is from another server and must not be trusted as proof. The user clarified that dnf packages work because they use `libsystemd`, static installs fail, and all edits must happen in a new worktree from `master`, not in the dirty main checkout where another worker is active.

### Assistant Understanding

Facts:

- Clean worktree: `~/src/PRs/netdata-static-journal-facets`, branch `fix/static-journal-facets`, created from refreshed `origin/master`.
- The main checkout is dirty with unrelated netflow work and must not be edited.
- Static builds can use the Rust journal provider: `CMakeLists.txt:201`, `CMakeLists.txt:220`, `CMakeLists.txt:2908`.
- dnf builds on the user's tested server use native `libsystemd`; those are outside the failing path.

Inferences:

- The failing path is the Rust provider under `src/crates/jf/`, not native `sd-journal`.
- The bug affects filtered/faceted queries more strongly than unfiltered scans because facet slicing builds journal matches from unique field values before scanning rows.

Unknowns:

- None for the accepted scope. A local RHEL 8.10 static install was available for regression validation after the first PR iteration.

### Acceptance Criteria

- Rust journal provider can return decompressed payloads for LZ4/XZ/Zstd compressed data objects.
- Unique-value enumeration returns logical `field=value` bytes, not compressed payload bytes.
- Data-object lookup used by match filters can find compressed data objects when the caller provides logical `field=value`.
- Facet filter setup falls back to a full query if unique-value enumeration returns an error while constructing backend matches.
- Existing uncompressed data behavior remains unchanged.
- Focused Rust tests pass for `src/crates/jf/`.

## Analysis

Sources checked:

- `.agents/skills/project-writing-collectors/SKILL.md`
- `src/collectors/systemd-journal.plugin/systemd-journal.c`
- `src/collectors/systemd-journal.plugin/provider/netdata_provider.h`
- `src/collectors/systemd-journal.plugin/provider/rust_provider.h`
- `src/crates/jf/journal_reader_ffi/src/lib.rs`
- `src/crates/jf/journal_file/src/object.rs`
- `src/crates/jf/journal_file/src/file.rs`
- `src/crates/journal-core/src/file/object.rs`
- `src/crates/jf/Cargo.toml`
- `src/crates/jf/journal_file/Cargo.toml`
- Official systemd journal file format documentation: `https://systemd.io/JOURNAL_FILE_FORMAT/`

Current state:

- `systemd-journal.c:583-607` builds native journal matches from `NSD_JOURNAL_FOREACH_UNIQUE()` values, parses them as `field=value`, then passes the same bytes to `nsd_journal_add_match()`.
- `systemd-journal.c:661` skips a journal file when filters exist but no backend matches can be installed.
- `journal_reader_ffi/src/lib.rs:285-305` decompresses compressed entry data for normal row scans.
- `journal_reader_ffi/src/lib.rs:383-389` returns unique-value payload bytes without checking compression.
- `journal_file/src/object.rs:873-890` marks LZ4/XZ/Zstd as compression methods, but only implements Zstd decompression.
- `journal-core/src/file/object.rs:978-1021` already implements Zstd, LZ4, and XZ decompression in the newer shared journal core.
- `journal_file/src/file.rs:548-550` uses payload matching for data-object lookup; `file.rs:191` compares raw object payload bytes to the caller-provided payload, which cannot find compressed objects when the caller provides logical `field=value`.
- The official journal format says DATA objects contain `field=value` payloads, that XZ/LZ4/Zstd compression is signaled by object/header flags, and that the DATA object hash is computed from the payload. That matches the need to expose/deal with the logical payload even when the on-disk bytes are compressed.

Risks:

- A partial fix that only decompresses unique enumeration may still fail when the filter builder later looks up the compressed data object.
- Adding decompression to hash-table lookup can affect filtered query performance on compressed buckets. The lookup is only on candidate objects in one hash bucket and is necessary for correctness.
- Adding new Rust dependencies changes `src/crates/jf/Cargo.lock`; this must be validated with focused cargo tests.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Static builds use the Rust journal provider path when `ENABLE_NETDATA_JOURNAL_FILE_READER` is enabled. That provider must emulate `libsystemd` APIs expected by `systemd-journal.plugin`.
- Facet counters can be produced by row scans because `rsd_journal_enumerate_available_data()` already decompresses entry data before returning it.
- Facet filtering uses unique-value enumeration to install backend matches. The Rust unique path currently returns raw payloads and does not decompress. On compressed systemd journal data, `parse_journal_field()` cannot reliably see `field=value`, so selected facet matches are not installed.
- Even after returning decompressed unique data, filter construction must resolve the logical `field=value` back to the data object offset. The current data-object matcher compares raw on-disk payload bytes, so compressed objects remain unfindable by logical payload.
- The `src/crates/jf/` decompressor only supports Zstd despite carrying LZ4/XZ flags. The newer `src/crates/journal-core/` code provides a local pattern for LZ4 and XZ support.

Evidence reviewed:

- `src/collectors/systemd-journal.plugin/systemd-journal.c:583-607`
- `src/collectors/systemd-journal.plugin/systemd-journal.c:661`
- `src/crates/jf/journal_reader_ffi/src/lib.rs:285-305`
- `src/crates/jf/journal_reader_ffi/src/lib.rs:383-389`
- `src/crates/jf/journal_file/src/object.rs:857-890`
- `src/crates/jf/journal_file/src/file.rs:157-191`
- `src/crates/journal-core/src/file/object.rs:978-1021`
- `src/crates/jf/Cargo.toml:16-28`
- `src/crates/jf/journal_file/Cargo.toml:7-16`
- `https://systemd.io/JOURNAL_FILE_FORMAT/` sections "Structure", "Extensibility", "Data Objects", and "Reading".

Affected contracts and surfaces:

- Static-build systemd journal log queries and facet filters.
- Rust FFI compatibility with the C plugin's `sd-journal`-like expectations.
- Rust crate dependency lockfile for `src/crates/jf/`.
- No public configuration, schema, or user-facing documentation surface is expected to change.

Existing patterns to reuse:

- Existing entry-data decompression branch in `rsd_journal_enumerate_available_data()`.
- Existing LZ4/XZ/Zstd decompression implementation in `src/crates/journal-core/src/file/object.rs`.
- Existing `JournalError::DecompressorError` / `UnknownCompressionMethod` handling.
- Existing `PayloadMatcher` bucket visitor pattern.

Risk and blast radius:

- Scope is confined to the Rust journal file reader used by the static provider.
- Native `libsystemd` builds should be unaffected.
- Main behavioral risk is filtered lookup performance on compressed hash buckets; lookup remains bounded to one bucket.
- Security risk is low; decompression must preserve existing error handling and must not panic on malformed compressed payloads.

Sensitive data handling plan:

- No raw logs, hostnames, IP addresses, customer identifiers, secrets, or journal payload samples will be written to durable artifacts.
- SOW evidence records only generic OS/compression behavior and source file references.

Implementation plan:

1. Add LZ4 and XZ dependencies to the `src/crates/jf/` workspace and port the established decompression logic from `journal-core`.
2. Make `rsd_journal_enumerate_available_unique()` mirror entry-data enumeration by returning decompressed payloads for compressed data objects.
3. Make data-object payload matching compare decompressed payloads for compressed data objects so filter construction can find the matching object offset.
4. Add focused Rust tests for LZ4 decompression and compressed payload matching.
5. Run focused formatting and tests for the `src/crates/jf/` workspace.

Validation plan:

- `cargo fmt` in `src/crates/jf`.
- `cargo test` in `src/crates/jf`.
- Same-failure scan for remaining raw `payload_bytes()` returns in FFI paths.
- Source review of all compressed data object reads in `src/crates/jf/`.

Artifact impact plan:

- AGENTS.md: no update expected; workflow rules unchanged.
- Runtime project skills: no update expected; collector-writing guidance remains valid.
- Specs: no update expected; this is a bug fix to match existing static-provider intent.
- End-user/operator docs: no update expected; no user-facing command/config changes.
- End-user/operator skills: no update expected; public AI skills are unaffected.
- SOW lifecycle: this SOW tracks the work and is completed/moved with the implementation in the same commit.

Open-source reference evidence:

- No external mirrored repository evidence used yet. The fix is based on two in-repository implementations of the same journal format.

Open decisions:

- None. The user has already specified the worktree constraint and the failing implementation path.

## Implications And Decisions

- No user decision is currently required. The evidence points to a bounded bug fix in the static Rust provider.

## Plan

1. Patch `src/crates/jf/journal_file` decompression support and data payload matching.
2. Patch `src/crates/jf/journal_reader_ffi` unique enumeration.
3. Update `src/crates/jf` Cargo manifests/lockfile.
4. Add focused tests.
5. Run focused validation and update this SOW.

## Execution Log

### 2026-05-08

- Created clean worktree from refreshed `origin/master`.
- Loaded project collector-writing skill.
- Verified source evidence and wrote pre-implementation gate.
- Added LZ4/XZ decompression support to the legacy `src/crates/jf/journal_file` reader, reusing the newer `journal-core` implementation pattern.
- Changed Rust FFI unique-value enumeration to return decompressed payloads for compressed DATA objects.
- Changed DATA hash-bucket payload matching to compare decompressed payloads when the on-disk object is compressed.
- Added focused LZ4 compressed payload matcher tests.
- Corrected an existing filter test expectation: the test writes 5,000 iterations and 2 matching rows per iteration, so the expected filtered count is `2 * iterations`, not `2`.

## Validation

Acceptance criteria evidence:

- LZ4/XZ/Zstd support: `src/crates/jf/journal_file/src/object.rs` now handles Zstd, LZ4 with the systemd 8-byte uncompressed-size prefix, and XZ.
- Unique enumeration: `src/crates/jf/journal_reader_ffi/src/lib.rs` now mirrors entry-data enumeration and decompresses before returning payload bytes.
- Match lookup: `src/crates/jf/journal_file/src/file.rs` now uses `DataPayloadMatcher`, which compares raw payload first and then decompressed payload for compressed DATA objects.
- Uncompressed behavior: raw `object.get_payload() == self.payload` matching remains the first path.
- Focused tests: `src/crates/jf/journal_file/src/file.rs` adds positive and negative LZ4 compressed payload matcher tests.

Tests or equivalent validation:

- `cargo fmt` in `src/crates/jf`: passed.
- `cargo test -q` in `src/crates/jf`: passed; 4 tests passed.
- `git diff --check`: passed.
- `.agents/sow/audit.sh`: status/directory checks passed for this SOW; the audit reported one pre-existing sensitive-data pattern in `.agents/skills/mirror-netdata-repos/SKILL.md:112`, which is public SSH clone syntax (`git@github.com:netdata/...`) in an unrelated existing file, not sensitive data from this work.

Real-use evidence:

- Initial implementation was not run against the local RHEL 8.10 static install before opening the PR.
- Regression validation below records live Function evidence from that static install after the reopened fix.

Reviewer findings:

- Initial implementation had no external reviewer pass before opening the PR; the user asked to distrust the prior session and verify locally from code.
- PR review iterations found Copilot comments on `netdata/netdata#22456`; each was verified before code changes, addressed in the same SOW, replied to in-thread, and resolved after commit/push.

Same-failure scan:

- `rg` over `src/crates/jf` for `payload_bytes()`, `decompress()`, `enumerate_available_unique`, `enumerate_available_data`, and `find_data_offset()` found the fixed FFI paths and the fixed match lookup. Remaining raw `payload_bytes()` use in `src/crates/jf/journal_file/src/filter.rs:246` is a debug dump path, not match construction or row enumeration.

Sensitive data gate:

- Durable artifacts contain no raw logs, secrets, credentials, bearer tokens, SNMP communities, customer names, personal data, non-private customer-identifying IPs, private endpoints, or proprietary incident details.

Artifact maintenance gate:

- AGENTS.md: no update needed; workflow and project guardrails did not change.
- Runtime project skills: no update needed; this did not change how agents should work on collectors.
- Specs: no update needed; this bug fix restores the intended static provider behavior and does not create a new product contract.
- End-user/operator docs: no update needed; no user-facing configuration, command, or workflow changed.
- End-user/operator skills: no update needed; public/operator AI skills are unaffected.
- SOW lifecycle: SOW status is `completed` and the file is moved to `.agents/sow/done/` with the implementation in the same commit.

Specs update:

- No spec update needed; behavior remains "static Rust provider should act like the native journal provider for DATA object payloads."

Project skills update:

- No project skill update needed; no new reusable workflow was discovered.

End-user/operator docs update:

- No docs update needed; this is a transparent bug fix.

End-user/operator skills update:

- No end-user/operator skill update needed; no public skill behavior changed.

Lessons:

- When fixing static-provider journal filtering, verify both the enumeration side and the data-offset lookup side. Returning decompressed `field=value` bytes is not enough if the filter builder still searches the DATA hash table by raw compressed bytes.

Follow-up mapping:

- No follow-up was needed for the initial fix; the later live regression is tracked in the appended regression section.

## PR Review Iteration - 2026-05-08

Findings:

- `PRRT_kwDOAKPxd86Asq8i`, `src/crates/jf/journal_reader_ffi/src/lib.rs:396`: valid. The decompression error path returned generic `-1` and printed to stderr instead of returning `JournalError::to_error_code()`. Same-class sweep found the same pattern in `rsd_journal_enumerate_available_data()`.
- `PRRT_kwDOAKPxd86Asq9F`, `src/crates/jf/journal_file/src/object.rs:899`: valid. The LZ4 branch trusted the on-disk uncompressed size prefix before `Vec::resize()`. Same-class sweep found the same LZ4 reader pattern in `src/crates/journal-core/src/file/object.rs`.
- CI signal before this iteration: one Docker armv7 job failed in its `Build Image` step while the workflow run was still in progress. GitHub did not expose logs yet because the overall run had not completed; all other available failure sources were either pending or passing.

Actions:

- Changed compressed DATA enumeration FFI paths to return `e.to_error_code()` for decompression errors.
- Added checked `u64` to `usize` conversion for the LZ4 uncompressed-size prefix.
- Added a DATA payload upper bound matching systemd's `DATA_SIZE_MAX` journal importer limit: 768 MiB.
- Used `try_reserve_exact()` before `Vec::resize()` so allocation failure returns `JournalError::DecompressorError` instead of panicking.
- Applied the same LZ4 bounds hardening to both `src/crates/jf/journal_file` and `src/crates/journal-core`.
- Added oversized LZ4 prefix regression tests in both readers.

Validation:

- `cargo fmt` in `src/crates/jf`: passed.
- `cargo fmt` in `src/crates`: passed; unrelated formatting churn in `src/crates/netdata-plugin/rt/src/lib.rs` was removed before staging.
- `cargo test -q` in `src/crates/jf`: passed; 5 tests passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 19 tests passed plus the existing ignored tests.

## PR Review Iteration 2 - 2026-05-08

Findings:

- `PRRT_kwDOAKPxd86AtF96`, `src/crates/journal-core/src/file/object.rs:1019`: valid. `try_reserve_exact()` was given a delta from capacity, but the API expects additional capacity from current length.
- `PRRT_kwDOAKPxd86AtF-U`, `src/crates/jf/journal_file/src/object.rs:911`: valid. Same fallible-reserve issue in the legacy reader.
- `PRRT_kwDOAKPxd86AtF-e`, `src/crates/jf/journal_file/src/object.rs:890`: valid. The Zstd streaming path still used unbounded `read_to_end()`; same-class sweep also covered XZ and `journal-core`.
- `PRRT_kwDOAKPxd86AtF-n`, `src/crates/jf/journal_file/src/object.rs:927`: valid. The newly added XZ decompression branch needed a focused positive test; same-class sweep added the same coverage to `journal-core`.

Actions:

- Corrected fallible LZ4 reserve logic to reserve the full required final length after `clear()`.
- Added bounded `read_limited_to_end()` helpers for streaming Zstd/XZ decompression.
- Applied the same bounded streaming decompression to both `src/crates/jf/journal_file` and `src/crates/journal-core`.
- Added fixed XZ compressed-payload fixtures and positive decompression tests for both readers without enabling the encoder feature in production dependencies.

Validation:

- `cargo fmt -p journal_file -p journal_reader_ffi` in `src/crates/jf`: passed.
- `cargo fmt -p journal-core` in `src/crates`: passed.
- `cargo test -q` in `src/crates/jf`: passed; 6 tests passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 20 tests passed plus the existing ignored tests.

## PR Review Iteration 3 - 2026-05-08

Findings:

- `PRRT_kwDOAKPxd86AtR3f`, `src/crates/jf/journal_file/src/file.rs:185`: partially valid. The slice comparison compiled in focused tests, but comparing by reference is clearer and avoids relying on implicit unsized slice comparison behavior.
- `PRRT_kwDOAKPxd86AtR33`, `src/crates/jf/journal_file/src/object.rs:787`: considered and not implemented as shared code. The duplicate helper exists in a legacy separate `src/crates/jf` workspace and the newer `src/crates/journal-core` workspace, with different error types. Centralizing via `journal-common` would add unrelated dependencies to the legacy static reader for a small private helper. Keeping the helpers local is lower risk for this bug-fix PR.
- `PRRT_kwDOAKPxd86AtR4H`, `src/crates/jf/journal_file/src/object.rs:796`: valid. `read_to_end()` can leave partial bytes in the reusable buffer before returning an error.
- `PRRT_kwDOAKPxd86AtR4a`, `src/crates/journal-core/src/file/object.rs:901`: valid. The stream-size cap needed a focused small-cap test that does not allocate hundreds of MiB.

Actions:

- Changed compressed payload comparison to compare slice references explicitly.
- Changed bounded stream reads to clear the reusable buffer on both over-limit and read-error paths.
- Added private small-cap helper entry points used by tests.
- Added small custom `Read` tests for over-limit stream handling in both readers.

Validation:

- `cargo fmt -p journal_file -p journal_reader_ffi` in `src/crates/jf`: passed.
- `cargo fmt -p journal-core` in `src/crates`: passed.
- `cargo test -q` in `src/crates/jf`: passed; 7 tests passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 21 tests passed plus the existing ignored tests.

## PR Review Iteration 4 - 2026-05-08

Findings:

- `PRRT_kwDOAKPxd86AteMb`, `src/crates/journal-core/src/file/object.rs:900`: valid. The testable size-cap helper converted `usize` to `u64` with `as` and then added 1.
- `PRRT_kwDOAKPxd86AteNJ`, `src/crates/jf/journal_file/src/file.rs:204`: valid readability issue. The `BucketVisitor` implementation used an elided matcher lifetime.
- `PRRT_kwDOAKPxd86AteNi`, `src/crates/jf/journal_file/src/writer.rs:640`: valid. The corrected assertion message lost useful context.

Actions:

- Changed both bounded-read helpers to use `u64::try_from(max_size)` plus `checked_add(1)`.
- Made `DataPayloadMatcher`'s borrowed payload lifetime explicit in the `BucketVisitor` implementation.
- Restored context in the filter-count assertion message.

Validation:

- `cargo fmt -p journal_file -p journal_reader_ffi` in `src/crates/jf`: passed.
- `cargo fmt -p journal-core` in `src/crates`: passed.
- `cargo test -q` in `src/crates/jf`: passed; 7 tests passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 21 tests passed plus the existing ignored tests.

## PR Review Iteration 5 - 2026-05-08

Findings:

- `PRRT_kwDOAKPxd86Atn1r`, `src/crates/journal-core/src/file/object.rs:1047`: valid. The LZ4 decode-error path left the reusable buffer resized to the advertised uncompressed size.
- `PRRT_kwDOAKPxd86Atn2C`, `src/crates/jf/journal_file/src/object.rs:941`: valid. Same LZ4 decode-error buffer state issue in the legacy static reader, including the FFI caller reuse path.

Actions:

- Changed both LZ4 decode-error branches to replace the reusable buffer with a new empty `Vec`, so callers cannot observe stale decompressed bytes or retain the failed allocation.
- Added malformed LZ4 block regression tests for both readers.

Validation:

- `cargo fmt -p journal_file -p journal_reader_ffi` in `src/crates/jf`: passed.
- `cargo fmt -p journal-core` in `src/crates`: passed.
- `cargo test -q` in `src/crates/jf`: passed; 8 tests passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 22 tests passed plus the existing ignored tests.
- `git diff --check`: passed.

## PR Review Iteration 6 - 2026-05-08

Findings:

- `PRRT_kwDOAKPxd86Atzu9`, `src/crates/journal-core/src/file/object.rs:916`: valid. The bounded Zstd/XZ stream helper cleared the buffer on over-limit or read errors but retained the potentially large allocation.
- `PRRT_kwDOAKPxd86Atzv6`, `src/crates/jf/journal_file/src/object.rs:811`: valid. Same bounded stream allocation-retention issue in the legacy static reader.
- `PRRT_kwDOAKPxd86Atzvg`, `src/crates/journal-core/src/file/object.rs:1052`: valid. The LZ4 path left `buf.len()` at the advertised size if the decoder returned fewer bytes than the prefix.
- `PRRT_kwDOAKPxd86AtzwG`, `src/crates/jf/journal_file/src/object.rs:946`: valid. Same LZ4 size/length mismatch semantics in the legacy static reader.

Actions:

- Changed both bounded stream helpers to replace the reusable buffer with a new empty `Vec` on over-limit or read errors.
- Changed both LZ4 branches to accept success only when the decoded length matches the systemd uncompressed-size prefix; mismatch now resets the buffer and returns `JournalError::DecompressorError`.
- Added LZ4 size-mismatch regression tests in both readers and strengthened bounded stream tests to assert capacity release.

Validation:

- `cargo fmt -p journal_file -p journal_reader_ffi` in `src/crates/jf`: passed.
- `cargo fmt -p journal-core` in `src/crates`: passed.
- `cargo test -q` in `src/crates/jf`: passed; 9 tests passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 23 tests passed plus the existing ignored tests.
- `git diff --check`: passed.

## PR Review Iteration 7 - 2026-05-08

Findings:

- `PRRT_kwDOAKPxd86At82Y`, `src/crates/journal-core/src/file/object.rs:915`: valid. `read_to_end()` with `take(max + 1)` bounded decompressed bytes but still allowed `Vec` growth strategy to over-allocate beyond the intended cap.
- `PRRT_kwDOAKPxd86At821`, `src/crates/jf/journal_file/src/file.rs:583`: valid. Existing tests covered `DataPayloadMatcher::payload_matches()` directly but not the `find_data_offset()` hash-bucket traversal path that uses the matcher.

Actions:

- Replaced bounded streaming `read_to_end()` calls in both readers with a manual fixed-size stack-buffer loop that uses `try_reserve_exact()` for each chunk and fails as soon as the decompressed stream exceeds the configured cap.
- Added a temporary-journal test that writes a compressed DATA object into the data hash table and verifies `find_data_offset()` locates it by logical uncompressed payload.

Validation:

- `cargo fmt -p journal_file -p journal_reader_ffi` in `src/crates/jf`: passed.
- `cargo fmt -p journal-core` in `src/crates`: passed.
- `cargo test -q` in `src/crates/jf`: passed; 10 tests passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 23 tests passed plus the existing ignored tests.
- `git diff --check`: passed.

## PR Review Iteration 8 - 2026-05-08

Findings:

- `PRRT_kwDOAKPxd86AuG7E`, `src/crates/journal-core/src/file/object.rs:926`: valid. Per-read `try_reserve_exact()` avoided unbounded growth but could cause one allocation per read chunk for large decompressed payloads.
- `PRRT_kwDOAKPxd86AuG7c`, `src/crates/jf/journal_file/src/file.rs:1190`: reviewed and not changed. The direct matcher fixture's `ObjectHeader::size = header + payload.len()` matches the writer contract; object placement alignment is handled by `ObjectHeader::aligned_size()` and is covered by the new temporary-journal `find_data_offset()` test.

Actions:

- Changed both bounded stream helpers to reserve capacity in amortized chunks, growing up to the configured decompressed-size cap without using `read_to_end()`'s geometric growth.
- Kept the direct matcher helper unpadded because padding those raw bytes would become part of the payload passed to `DataObject::from_data()` and would diverge from the writer's unpadded `size` field.

Validation:

- `cargo fmt -p journal_file -p journal_reader_ffi` in `src/crates/jf`: passed.
- `cargo fmt -p journal-core` in `src/crates`: passed.
- `cargo test -q` in `src/crates/jf`: passed; 10 tests passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 23 tests passed plus the existing ignored tests.
- `git diff --check`: passed.

## PR Review Iteration 9 - 2026-05-08

Findings:

- `PRRT_kwDOAKPxd86AuPUu`, `src/crates/journal-core/src/file/object.rs:947`: valid. The amortized `try_reserve_exact()` call still used a capacity delta, but the API takes additional capacity relative to `buf.len()`.
- `PRRT_kwDOAKPxd86AuPVH`, `src/crates/jf/journal_file/src/object.rs:842`: valid. Same reservation argument bug in the legacy reader.

Actions:

- Changed both amortized reservation paths to pass `target_capacity - buf.len()` to `try_reserve_exact()`.

Validation:

- `cargo fmt -p journal_file -p journal_reader_ffi` in `src/crates/jf`: passed.
- `cargo fmt -p journal-core` in `src/crates`: passed.
- `cargo test -q` in `src/crates/jf`: passed; 10 tests passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 23 tests passed plus the existing ignored tests.
- `git diff --check`: passed.

## Outcome

Implemented, validated, and prepared for commit in `~/src/PRs/netdata-static-journal-facets`.

## Lessons Extracted

- The legacy `src/crates/jf/` reader and the newer `src/crates/journal-core/` reader had diverged on compression support. For journal-format fixes, compare both implementations before changing one path.

## Followup

None for the completed PR review iteration; the appended regression section records the live validation repair.

## Regression Log

## Regression - 2026-05-08

What broke:

- Live static install testing on `[LOCAL_RHEL_8_10_TEST_AGENT]:19999` still returned empty rows when a systemd-journal facet value was selected.
- The previous validation only proved compressed DATA lookup/decompression at the Rust crate level. It did not prove the full C plugin Function path that receives facet selections from the UI, converts them into backend matches, and streams rows back to the dashboard.

Evidence:

- User report: static build copied and installed on a local RHEL 8.10 test Agent; selecting a facet returns empty responses.
- The regression is specific to facet filtering, not unfiltered journal access.
- Direct agent Function evidence from `[LOCAL_RHEL_8_10_TEST_AGENT]:19999`: unfiltered `systemd-journal` with `slice:true` returned 20 rows and non-empty facets for the last 4 hours; selecting advertised values `SYSLOG_IDENTIFIER=netdata`, `_SYSTEMD_UNIT=netdata.service`, or `PRIORITY=6` returned 0 rows with `slice:true`, while the same selections returned 20 rows with `slice:false`.
- Direct response stats for `SYSLOG_IDENTIFIER=netdata`, `slice:true`: request echoes the expected JSON `selections`, but `rows.evaluated=0`, `rows.matched=0`, and the journal file reports `rows_read=0`. This matches a failure before row scanning, during native match setup / filtered cursor resolution.
- Source evidence: `src/crates/jf/journal_file/src/filter.rs:339-357` builds filtered cursors by hashing selected `field=value` bytes and resolving them through `find_data_offset()`.
- Source evidence: `src/crates/jf/journal_file/src/hash.rs:4-8` still uses `twox_hash::XxHash64` behind a `FIXME` for the non-keyed Jenkins path.
- Source evidence: `systemd/systemd @ d0c912899a33436d6676b2564eb1ac506f378571`, `src/libsystemd/sd-journal/journal-file.c:1585-1600`, uses `siphash24()` only for keyed journal files and `jenkins_hash64()` otherwise.
- Source evidence: `systemd/systemd @ d0c912899a33436d6676b2564eb1ac506f378571`, `src/libsystemd/sd-journal/lookup3.h:14-20`, defines `jenkins_hash64()` as lookup3 `jenkins_hashlittle2()` with the primary value in the high 32 bits and the secondary value in the low 32 bits.
- In-repository pattern: `src/crates/journal-core/src/file/hash.rs:4-17` already uses `hashers::jenkins::Lookup3Hasher` and swaps the 32-bit halves to match systemd's `jenkins_hash64()`.

Repair plan:

- Replace the legacy `src/crates/jf` non-keyed hash fallback with the same lookup3/Jenkins implementation used by `journal-core`.
- Add reference-value tests for systemd-compatible Jenkins hashes.
- Re-run focused Rust tests and direct selected-facet queries on the local RHEL 8.10 static install.

Validation plan:

- Focused Rust crate tests for the fixed path.
- Direct API evidence from the local RHEL 8.10 static install showing the same facet values return rows after the fix.
- Static build/install validation using the user-provided local static-binary workflow if a code change is needed.

Why previous validation missed it:

- The first fix validated compressed DATA enumeration and compressed payload lookup, but did not validate the full selected-facet Function path on a real non-keyed journal file.
- The failing `slice:true` path builds filtered cursors before scanning rows; the incorrect hash function made `find_data_offset()` fail before row evaluation, so decompression tests alone could not catch it.

Actions:

- Replaced the legacy `src/crates/jf` non-keyed journal hash implementation with `hashers::jenkins::Lookup3Hasher`, matching the existing `journal-core` pattern and systemd lookup3 half ordering.
- Added the empty-payload lookup3 guard because `hashers` returns `0` for empty input while systemd lookup3 with zero seeds returns `0xdeadbeefdeadbeef`.
- Added systemd reference-value tests for the legacy reader hash path.
- Applied the same empty-payload guard and reference-value test to `src/crates/journal-core` so the legacy and newer journal readers remain consistent.

Validation:

- `cargo fmt -p journal_file -p journal_reader_ffi` in `src/crates/jf`: passed.
- `cargo fmt -p journal-core` in `src/crates`: passed.
- `cargo test -q` in `src/crates/jf`: passed; 11 tests passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 24 tests passed plus the existing ignored tests.
- Static build via the user-provided local static-binary workflow: passed; produced `artifacts/netdata-x86_64-latest.gz.run`.
- Static install on the local RHEL 8.10 test Agent: passed; installer restarted `netdata` through systemd.
- Direct Function validation after install:
  - baseline `slice:true`: `data_len=20`, `rows_evaluated=2019`, `rows_matched=2019`, `rows_read=2019`.
  - `SYSLOG_IDENTIFIER=netdata`, `slice:true`: `data_len=20`, `rows_evaluated=1499`, `rows_matched=1499`, `rows_read=1499`.
  - `_SYSTEMD_UNIT=netdata.service`, `slice:true`: `data_len=20`, `rows_evaluated=1714`, `rows_matched=1714`, `rows_read=1714`.
  - `PRIORITY=6`, `slice:true`: `data_len=20`, `rows_evaluated=1596`, `rows_matched=1596`, `rows_read=1596`.
  - `SYSLOG_IDENTIFIER=netdata`, `slice:false`: `data_len=20`, `rows_evaluated=2019`, `rows_matched=1499`, `rows_read=2019`.
- Same-failure scan: `rg -n "twox_hash|XxHash64|jenkins_hash|Lookup3Hasher|hashers" src/crates/jf src/crates/journal-core src/crates/Cargo.toml src/crates/Cargo.lock` found no remaining `twox_hash`/`XxHash64` use in the legacy journal hash path, and found both journal readers using `Lookup3Hasher`.
- `git diff --check`: passed.
- `.agents/sow/audit.sh`: SOW status/directory checks passed for this SOW; audit still exits non-zero for the pre-existing public SSH clone syntax false positive in `.agents/skills/mirror-netdata-repos/SKILL.md:112`, unrelated to this work.

Sensitive data handling:

- Bearer tokens, Cloud token values, claim identifiers, node identifiers, raw journal rows, and private endpoint names were not written to this SOW.
- Raw response JSON from live validation was kept under `.local/audits/`, which is gitignored and not staged.

Artifact updates:

- AGENTS.md: no update needed; workflow and project guardrails did not change.
- Runtime project skills: no update needed; the static-build skill worked as operational context and the fix did not change how agents should work on collectors.
- Specs: no update needed; this restores the intended systemd-compatible journal hash behavior.
- End-user/operator docs: no update needed; no user-facing configuration, command, or workflow changed.
- End-user/operator skills: no update needed; public/operator skill behavior was not changed by this PR.
- SOW lifecycle: SOW is moved back to `.agents/sow/done/` with `Status: completed` in the same commit as the regression repair.

Follow-up mapping:

- No follow-up remains for the static journal facet filtering regression.

## Core Library Extension - 2026-05-09

Question answered:

- The user asked whether the same compatibility-layer changes were also needed in the underlying `journal-core` library.

Evidence:

- `src/crates/journal-core/src/file/hash.rs:4-64` already had the systemd-compatible Jenkins lookup3 hash implementation and reference-value test from the regression repair.
- `src/crates/journal-core/src/file/object.rs:1049-1103` already had bounded Zstd/LZ4/XZ DATA decompression support from the earlier PR review iterations.
- `src/crates/journal-core/src/file/file.rs:39-79` still used the generic `PayloadMatcher`, which compared `object.raw_payload() == self.payload`.
- `src/crates/journal-core/src/file/file.rs:485-488` used that raw-only matcher in `find_data_offset()`.
- `src/crates/journal-core/src/file/filter.rs:283` and `src/crates/journal-core/src/file/filter.rs:297` build filter expressions through `find_data_offset()`, so compressed DATA objects could still fail filter construction in core-library users.

Actions:

- Added `DataPayloadMatcher` to `src/crates/journal-core/src/file/file.rs`, matching the `src/crates/jf` repair pattern.
- Kept the raw-payload comparison as the first path for uncompressed DATA objects.
- Added a compressed-payload comparison path that decompresses only compressed DATA objects in the selected hash bucket.
- Changed `journal-core` `find_data_offset()` to use `DataPayloadMatcher`.
- Added focused `journal-core` tests for direct LZ4 compressed matching, negative compressed matching, and the `find_data_offset()` hash-bucket traversal path.

Validation:

- `cargo fmt -p journal-core` in `src/crates`: passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 27 tests passed plus the existing ignored tests.
- `cargo test -q` in `src/crates/jf`: passed; 11 tests passed.
- Same-failure scan: `rg -n "raw_payload\\(\\) == self\\.payload|DataPayloadMatcher|find_data_offset\\(|payload_matches" src/crates/jf src/crates/journal-core` confirmed both readers now use `DataPayloadMatcher` for DATA hash-bucket lookup; the remaining raw-payload comparison is the generic field-object matcher path, which is correct because FIELD objects are not compressed DATA payloads.

Artifact updates:

- AGENTS.md: no update needed; workflow and project guardrails did not change.
- Runtime project skills: no update needed; this is a code-path consistency fix, not a workflow change.
- Specs: no update needed; this preserves the intended journal-format behavior.
- End-user/operator docs: no update needed; no user-facing configuration, command, or workflow changed.
- End-user/operator skills: no update needed; public/operator skill behavior was not changed.
- SOW lifecycle: same SOW reopened for the same-class core-library gap and moved back to `.agents/sow/done/` with `Status: completed` in the same commit.

Follow-up mapping:

- No follow-up remains for the underlying `journal-core` compressed DATA lookup gap.

## External Review Follow-up - 2026-05-09

Why reopened:

- The user requested external review of the performance and side effects of the changes before merge.
- Reviewers agreed the original static facet fix is correct, but flagged same-family raw-payload reads outside the original `find_data_offset()` lookup path.

Confirmed findings:

- `src/crates/journal-core/src/file/file.rs:595` read remapping entry DATA bytes with `raw_payload()` after the marker lookup had become compressed-aware.
- `src/crates/journal-core/src/file/reader.rs:428` copied remapping entry DATA bytes with `raw_payload()`.
- `src/crates/journal-index/src/field_types.rs:233` parsed source timestamp DATA bytes with `raw_payload()`, with callers in `src/crates/journal-index/src/file_index.rs:390` and `src/crates/journal-index/src/file_indexer.rs:437`.
- The `hashers` crate contains suspicious optimized alignment branches, but local inspection showed its `offset_to_align()` helper never returns `0` for normal alignments, so the crate falls back to the byte path used by the passing reference-value tests and the live RHEL 8.10 validation. This is not a current PR blocker.

Actions:

- Added `DataObject::logical_payload()` in `src/crates/journal-core/src/file/object.rs`.
- Updated `journal-core` remapping reads to use logical payload bytes and return `JournalError::InvalidField` instead of panicking on non-UTF-8 data.
- Updated `journal-index` source timestamp parsing to use logical payload bytes, while preserving a scratch-buffer variant to avoid repeated allocations in loops.
- Added focused `journal-core` tests for logical raw payloads and LZ4-compressed logical payloads.

Validation:

- `cargo fmt -p journal-core -p journal-index` in `src/crates`: passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 29 tests passed plus the existing ignored doc tests.
- `cargo test -q -p journal-index` in `src/crates`: passed; 66 tests passed across the package test binaries.

Follow-up mapping:

- Stale uncompiled `src/crates/jf/journal_file/src/journal_file.rs` still contains a raw-payload lookup helper with no callers. This is not runtime behavior for this PR and should be handled only if that stale module is removed or revived.
- Zstd/XZ integration tests through `find_data_offset()` would improve coverage but do not block this RHEL LZ4 regression fix because object-level XZ and bounded streaming behavior are already tested, and the logical matcher delegates to the same decompression API.

## External Review Rerun Closure - 2026-05-09

Why updated:

- The user asked to run the external reviewers again after the first follow-up fixes.
- The rerun found no evidence that the live RHEL 8.10 static facet failure remained, but it did identify two local hardening issues in the same code path.

Confirmed findings:

- `src/crates/journal-core/src/file/file.rs:629` still used `expect("utf8 data")` for FIELD names in the same `load_fields()` path where DATA payload parsing had already been converted to `JournalError::InvalidField`.
- `src/collectors/systemd-journal.plugin/provider/rust_provider.h:13` exposed unique enumeration through a foreach macro that stops on `<= 0`; when `rsd_journal_enumerate_available_unique()` returned a negative decompression error, `src/collectors/systemd-journal.plugin/systemd-journal.c:583` could silently stop without incrementing `failures`, so the fallback at `systemd-journal.c:621` would not run.
- `src/crates/journal-index/src/file_index.rs:493` manually decompressed regex payloads even though `DataObject::logical_payload()` now exists.

Reviewed and rejected as non-blocking for this SOW:

- `src/crates/journal-index/src/file_indexer.rs:39-43` documents that compressed values are skipped by the bitmap indexer; `file_indexer.rs:296-309` implements that existing index-size policy. Changing it would require a separate product/performance decision because it would decompress and index every unique compressed value. This PR keeps the documented indexing limit unchanged.
- A reviewer claimed the Zstd object flag should be `1 << 3`. That finding was false. Current systemd source has `OBJECT_COMPRESSED_ZSTD = 1 << 2` and `HEADER_INCOMPATIBLE_COMPRESSED_ZSTD = 1 << 3`; Netdata's object/header constants match that split. Evidence: `https://raw.githubusercontent.com/systemd/systemd/main/src/libsystemd/sd-journal/journal-def.h`, lines 62-64 and 186.
- The stale uncompiled `src/crates/jf/journal_file/src/journal_file.rs` helper still has no runtime callers. It is rejected for this SOW because changing dead code would increase review surface without changing shipped behavior.
- Zstd/XZ `find_data_offset()` integration tests and Zstd object-level tests are useful coverage, but they are rejected for this SOW because the production regression is the RHEL LZ4 path and the shared decompression paths are already covered by object-level LZ4/XZ and bounded-stream tests.

Actions:

- Converted the remaining FIELD-name panic in `journal-core` `load_fields()` to `JournalError::InvalidField`.
- Added `nsd_journal_enumerate_available_unique()` to the provider abstraction.
- Replaced the filter builder's `NSD_JOURNAL_FOREACH_UNIQUE()` macro use with an explicit restart/enumerate loop that counts negative `query_unique()` and enumeration returns as setup failures, preserving the existing full-query fallback.
- Replaced `_BOOT_ID` annotation unique enumeration with the same explicit wrapper and logged negative enumeration returns.
- Replaced the manual regex-path decompression in `journal-index` with `DataObject::logical_payload()`.

Validation:

- `curl -fsSL https://raw.githubusercontent.com/systemd/systemd/main/src/libsystemd/sd-journal/journal-def.h | rg -n "OBJECT_COMPRESSED_(XZ|LZ4|ZSTD)|HEADER_INCOMPATIBLE_COMPRESSED_ZSTD"`: verified object Zstd is `1 << 2` and header incompatible Zstd is `1 << 3`.
- `cargo fmt -p journal-core -p journal-index` in `src/crates`: passed.
- `git diff --check`: passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 29 tests passed plus the existing ignored doc tests.
- `cargo test -q -p journal-index` in `src/crates`: passed; 66 tests passed across the package test binaries.
- `cargo test -q --all-targets` in `src/crates/jf`: passed; 11 tests passed.
- `./packaging/makeself/build-static.sh x86_64`: passed; produced `artifacts/netdata-x86_64-latest.gz.run` after compiling `systemd-journal.plugin`, `journal_reader_ffi`, `journal-core`, and `journal-index` in the static musl build.
- `.agents/sow/audit.sh`: SOW status/directory checks passed for this SOW; the audit still exits non-zero on the pre-existing public SSH clone syntax pattern in `.agents/skills/mirror-netdata-repos/SKILL.md:112`, unrelated to this work and not staged by this PR.

Follow-up mapping:

- No follow-up remains for the static journal facet filtering regression.

## Libsystemd Compatibility Follow-up - 2026-05-09

Why reopened:

- The user asked whether the new explicit C call could break old `libsystemd` builds if older `SD_JOURNAL_FOREACH_UNIQUE()` macros did not use `sd_journal_enumerate_available_unique()`.

Evidence:

- systemd v245 `src/systemd/sd-journal.h` declares `sd_journal_enumerate_unique()` and defines `SD_JOURNAL_FOREACH_UNIQUE()` with `sd_journal_enumerate_unique()`.
- systemd v246 `src/systemd/sd-journal.h` declares `sd_journal_enumerate_available_unique()` and defines `SD_JOURNAL_FOREACH_UNIQUE()` with `sd_journal_enumerate_available_unique()`.
- The current systemd manual records `sd_journal_query_unique()`, `sd_journal_enumerate_unique()`, `sd_journal_restart_unique()`, and `SD_JOURNAL_FOREACH_UNIQUE()` as added in version 195; `sd_journal_enumerate_available_unique()` was added in version 246.

Conclusion:

- The user's concern was valid. The previous C wrapper would have broken builds against libsystemd headers older than v246 when `HAVE_SD_JOURNAL_RESTART_FIELDS` was set.

Actions:

- Added `HAVE_SD_JOURNAL_ENUMERATE_AVAILABLE_UNIQUE` detection in `packaging/cmake/Modules/NetdataDetectSystemd.cmake`.
- Added the generated config define to `packaging/cmake/config.cmake.h.in`.
- Updated `nsd_journal_enumerate_available_unique()` so:
  - Rust provider builds call `rsd_journal_enumerate_available_unique()`.
  - Modern libsystemd builds call `sd_journal_enumerate_available_unique()`.
  - Older libsystemd builds fall back to `sd_journal_enumerate_unique()`, matching the old `SD_JOURNAL_FOREACH_UNIQUE()` macro behavior.

Validation:

- systemd v245 header check: confirmed `SD_JOURNAL_FOREACH_UNIQUE()` uses `sd_journal_enumerate_unique()`.
- systemd v246 header check: confirmed `SD_JOURNAL_FOREACH_UNIQUE()` uses `sd_journal_enumerate_available_unique()`.
- Current systemd manual check: confirmed `sd_journal_enumerate_available_unique()` was added in version 246.
- Compiled `src/collectors/systemd-journal.plugin/provider/netdata_provider.c` with a temporary config where `HAVE_SD_JOURNAL_RESTART_FIELDS` is defined and `HAVE_SD_JOURNAL_ENUMERATE_AVAILABLE_UNIQUE` is not defined: passed, proving the old-libsystemd branch compiles.
- Compiled the same provider file with both `HAVE_SD_JOURNAL_RESTART_FIELDS` and `HAVE_SD_JOURNAL_ENUMERATE_AVAILABLE_UNIQUE` defined: passed, proving the modern-libsystemd branch compiles.
- `git diff --check`: passed.

Artifact updates:

- AGENTS.md: no update needed; workflow and project guardrails did not change.
- Runtime project skills: no update needed; this is a compatibility guard in project code.
- Specs: no update needed; this preserves existing libsystemd compatibility.
- End-user/operator docs: no update needed; no user-facing configuration, command, or workflow changed.
- End-user/operator skills: no update needed; public/operator skill behavior was not changed.
- SOW lifecycle: SOW reopened for this compatibility fix and will be moved back to done with `Status: completed` in the same commit.

Follow-up mapping:

- No follow-up remains for libsystemd unique-enumeration symbol compatibility.

## PR Review Cleanup - 2026-05-09

Why reopened:

- The user reported that old PR comments/reviews were still unresolved.
- Current PR review state showed three unresolved bot review threads.

Findings:

- `PRRT_kwDOAKPxd86AyMvL`, `src/crates/journal-core/src/file/object.rs:1085`: valid. Several `DataObject::decompress()` error paths can return without clearing the caller-provided scratch buffer, including short LZ4 prefix, oversized LZ4 prefix, LZ4 reserve failure, Zstd decoder creation failure, and unknown compression method.
- `PRRT_kwDOAKPxd86AyMwE`, `src/crates/jf/journal_file/src/object.rs:971`: valid. The legacy static reader has the same scratch-buffer error-path issue.
- `PRRT_kwDOAKPxd86Aye5J`, `src/crates/journal-index/src/field_types.rs:242`: valid. `get_timestamp_field()` scans all entry DATA objects for the configured timestamp field. A decompression failure in an unrelated compressed DATA object can abort the search instead of being treated like a non-match and allowing `get_entry_timestamp()` to fall back to the entry realtime timestamp.

Planned actions:

- Reset decompression scratch buffers to a new empty `Vec` on every `DataObject::decompress()` error path that occurs before the bounded stream reader or LZ4 decoder already clears it.
- Add regression tests in both readers for short LZ4 prefixes and stale-buffer oversized prefixes.
- Treat compressed-payload decompression failures as timestamp-field non-matches during timestamp parsing, while preserving other journal errors.
- Run focused Rust validation and GitHub review sync before commit/push.

Actions:

- Changed `src/crates/journal-core/src/file/object.rs` and `src/crates/jf/journal_file/src/object.rs` so early decompression failures reset the scratch buffer to a new empty `Vec`.
- Added short LZ4-prefix regression tests in both readers.
- Strengthened the oversized LZ4-prefix tests in both readers to start with a stale buffer and assert capacity release.
- Changed `src/crates/journal-index/src/field_types.rs` so `JournalError::DecompressorError` and `JournalError::UnknownCompressionMethod` become `IndexError::InvalidFieldPrefix` for timestamp parsing. Other journal errors still propagate.
- Added a focused timestamp error-classification test.

Validation:

- `cargo fmt -p journal-core -p journal-index` in `src/crates`: passed.
- `cargo fmt -p journal_file -p journal_reader_ffi` in `src/crates/jf`: passed.
- `cargo test -q -p journal-core` in `src/crates`: passed; 30 tests passed plus existing ignored doc tests.
- `cargo test -q -p journal-index` in `src/crates`: passed; 67 tests passed across package test binaries.
- `cargo test -q --all-targets` in `src/crates/jf`: passed; 12 tests passed.
- `git diff --check`: passed.
- Same-failure scan: checked the affected decompression error returns and confirmed the remaining early returns in both `DataObject::decompress()` implementations clear the scratch buffer before returning; bounded-stream and LZ4 decoder failure paths already clear internally.
- PR sync barrier: `fetch-all.sh 22456` still showed the same three unresolved threads and no newer unresolved threads.
- Sonar sync barrier: `fetch-sonar-findings.sh 22456` reported 0 issues and 0 hotspots.
- CI sync barrier: `ci-status.sh 22456` reported 0 failing checks and 94 running checks before this push.
- `.agents/sow/audit.sh`: SOW status/directory checks passed; audit still exits non-zero on the pre-existing public SSH clone syntax pattern in `.agents/skills/mirror-netdata-repos/SKILL.md:112`, unrelated to this work and not staged by this PR.

Artifact updates:

- AGENTS.md: no update needed; workflow and project guardrails did not change.
- Runtime project skills: no update needed; this is a code review cleanup in an existing workflow.
- Specs: no update needed; this preserves intended journal error resilience.
- End-user/operator docs: no update needed; no user-facing configuration, command, or workflow changed.
- End-user/operator skills: no update needed; public/operator skill behavior was not changed.
- SOW lifecycle: SOW reopened for unresolved PR comments and will be moved back to done with `Status: completed` in the same commit.

Follow-up mapping:

- No follow-up remains for these unresolved review comments.
