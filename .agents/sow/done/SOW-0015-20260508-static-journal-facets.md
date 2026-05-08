# SOW-0015 - Static Journal Facet Filtering

## Status

Status: completed

Sub-state: implementation, focused validation, and SOW closure complete in clean worktree.

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

- Exact remote server journal contents are not available in this worktree. Validation must rely on source-level root cause, targeted unit tests, and Rust crate tests unless the user later asks for remote-system testing.

### Acceptance Criteria

- Rust journal provider can return decompressed payloads for LZ4/XZ/Zstd compressed data objects.
- Unique-value enumeration returns logical `field=value` bytes, not compressed payload bytes.
- Data-object lookup used by match filters can find compressed data objects when the caller provides logical `field=value`.
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

- Not run against the remote RHEL 8.10 server from this worktree. The user identified that server as separate, and this task was constrained to source edits in a clean local worktree.

Reviewer findings:

- Initial implementation had no external reviewer pass before opening the PR; the user asked to distrust the prior session and verify locally from code.
- PR review iteration found two Copilot comments on `netdata/netdata#22456`; both were verified as valid and addressed in the same SOW.

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

- No follow-up needed at this stage.

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

## Outcome

Implemented, validated, and prepared for commit in `~/src/PRs/netdata-static-journal-facets`.

## Lessons Extracted

- The legacy `src/crates/jf/` reader and the newer `src/crates/journal-core/` reader had diverged on compression support. For journal-format fixes, compare both implementations before changing one path.

## Followup

None yet.

## Regression Log

None yet.
