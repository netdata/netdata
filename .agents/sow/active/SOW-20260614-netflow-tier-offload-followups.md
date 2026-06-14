# SOW-20260614-netflow-tier-offload-followups - NetFlow tier-offload follow-up items

## Status

Status: in-progress

Sub-state: items extracted from the completed tier-offload SOW (shipped as
PR #22703). The requested prerequisite netflow published journal SDK refresh to
0.6.4 is implemented and validated. User decision on 2026-06-14: no deferred
follow-ups for this PR; every finding below must be fixed in PR #22703 or
rejected with evidence in this SOW.

## Requirements

### Purpose

Track and dispose of the follow-up items surfaced by the netflow tier-commit
offload work (PR #22703) and its 4 external-review rounds, so none are lost.
Fit-for-purpose: each item must end as implemented, rejected-with-evidence,
before this SOW closes.

### User Request

After confirming the tier-offload work was externally reviewed, the user
asked to close the current SOW and "create an SOW for the pending items and
let's address them."

Latest user decision: address all issues for long-term-best in PR #22703.
No deferral: either dismiss a finding with evidence or fix it in this PR.

### Assistant Understanding

Facts:

- The tier-offload work is open as PR #22703.
- The original follow-up list mixed direct offload findings with broader
  migration debt and pre-existing limitations; this SOW now separates those
  classes and gives each one a terminal disposition.
- A later PR review against `upstream/master` found two direct tier-worker
  follow-ups: the cubic startup-leak comment is valid, and worker-mode tier
  sync attempts are not counted in tier sync telemetry.

Unknowns:

- None for scope. Implementation may still reject a finding if investigation
  proves it is not a valid PR #22703 issue or cannot be implemented in this
  repository without violating the approved dependency boundary.

### Acceptance Criteria

- Every item below reaches a terminal disposition (implemented / rejected
  with evidence), recorded here. No item may be left as deferred or only
  tracked externally.
- Prerequisite SDK refresh: `netflow-plugin` depends on published
  `systemd-journal-sdk-*` crates version 0.6.4, the lockfile resolves those
  crates at 0.6.4, and netflow validation passes before follow-up
  implementation starts.
- Fixed items have targeted regression coverage, and `cargo test -p
  netflow-plugin` passes.

## Pending Items

Each item: source, evidence, type, blast radius, recommendation.

1. **Tier workers are spawned before UDP bind succeeds** (cubic PR comment;
   direct offload bug).
   - Evidence: `src/crates/netflow-plugin/src/ingest/service/runtime.rs:7`
     calls `spawn_tier_commit_workers()` before
     `src/crates/netflow-plugin/src/ingest/service/runtime.rs:10-12` binds the
     UDP socket.
   - Evidence: `src/crates/netflow-plugin/src/ingest/service/runtime.rs:45`
     calls `finish_shutdown()` only after the receive loop; the bind-error path
     returns before `src/crates/netflow-plugin/src/ingest/service/runtime.rs:242`
     can call `tier_handoff.begin_shutdown()`.
   - Type: bug fix, behavioral. Blast radius: ingest service startup and tier
     worker lifecycle.
   - Disposition target: **fix in PR #22703**.
   - Recommendation (surgical): fix before merge by moving worker spawn after
     successful bind and receive-buffer setup, or by adding an explicit
     bind-error shutdown path. Add a regression test that binds an already-used
     UDP address and proves `run()` returns the bind error without leaving tier
     workers alive.

2. **Worker-mode tier sync attempts are not counted in telemetry** (direct
   offload observability bug).
   - Evidence: pre-worker tier sync increments `tier_journal_syncs` at
     `src/crates/netflow-plugin/src/ingest/service/runtime.rs:369-371`.
   - Evidence: worker sync calls `writer.sync()` at
     `src/crates/netflow-plugin/src/ingest/tier_commit.rs:360-382`, and final
     drain sync calls `writer.sync()` at
     `src/crates/netflow-plugin/src/ingest/tier_commit.rs:415-421`, but both
     paths only increment `tier_journal_sync_errors` on failure.
   - Type: bug fix, telemetry correctness. Blast radius: tier commit metrics
     and materialized-tier charts/API snapshots.
   - Disposition target: **fix in PR #22703**.
   - Recommendation (surgical): increment `tier_journal_syncs` for every
     worker sync attempt, including final drain if final drain is part of the
     intended sync-call metric. Add a focused test that worker commits advance
     tier sync-call telemetry.

3. **Facet state persistence holds a Mutex across disk writes** (pre-existing
   defect, independent of offload).
   - Evidence: `src/crates/netflow-plugin/src/facet_runtime.rs` —
     `observe_rotation` holds the facet Mutex across `write_sidecar_files` +
     `persist_state_locked` (disk I/O under the lock). Surfaced repeatedly in
     review as the heaviest facet-contention case.
   - Additional PR-review evidence:
     `src/crates/netflow-plugin/src/facet_runtime.rs:331-349` still performs
     sidecar/state disk writes while holding the facet lock. With tier workers,
     this can now contend across worker and receive threads, so it remains a
     real isolation risk even though the defect pre-dates offload.
   - Type: bug fix, behavioral. Blast radius: facet runtime, shared with the
     query task and the new tier workers.
   - Disposition target: **fix in PR #22703**.
   - Recommendation (long-term-best): fix it — move the disk writes outside
     the lock (snapshot under lock, write unlocked). Own PR; needs its own
     review round. **Strongest candidate to do now.**

4. **Stage B: move the raw-rotation fsync off the receive thread.**
   - Evidence: the last inline disk barrier on the receive thread is the raw
     journal's sync-on-archive; removing it needs an SDK opt-out
     (`with_sync_on_archive(false)`) plus a `nf-raw-sync` worker behind the
     `Rotated` lifecycle event.
   - Additional evidence: published `systemd-journal-sdk-log-writer` 0.6.4
     `src/log/config.rs` exposes `Config` fields for rotation, retention,
     compression, compact layout, open/identity mode, strict naming,
     live-publish cadence, field policy, and file mode; there is no
     `sync_on_archive` / `with_sync_on_archive` option.
   - Additional evidence: published `systemd-journal-sdk-log-writer` 0.6.4
     `src/log/mod.rs` syncs the old active file inside
     `rotate_existing_active_file` before archiving it.
   - Type: feature, not implementable in this repository while netflow uses the
     published SDK boundary.
   - Disposition target: **reject for PR #22703 with evidence**. Implementing
     this now would require a new upstream SDK release or vendoring/patching
     the SDK in this PR, which would undo the just-completed published-SDK
     boundary and mix dependency ownership with tier offload.

5. **Consolidate the remaining vendored journal crates** (carried from the
   migration SOW / #22665).
   - Evidence: `netdata-log-viewer` and the OTEL plugins still use the
     in-tree `journal-*` crates; only netflow moved to
     `systemd-journal-sdk-*` (updated to 0.6.4 as a prerequisite in this SOW).
   - Additional evidence: PR #22703's tier-offload runtime links only
     `netflow-plugin`; the remaining vendored journal consumers are
     `netdata-log-viewer` and `netdata-otel/otel-plugin` manifests.
   - Type: historical cross-plugin migration debt, not a defect introduced by
     PR #22703's tier offload.
   - Disposition target: **reject as a PR #22703 finding with evidence**.
     This is real repository debt, but not a valid offload correctness issue.

6. **Timing-independent CI test for the per-row lock-drop property.**
   - Evidence: the per-row-read-lock gate is `#[ignore]` (timing-sensitive),
     so CI cannot catch a regression that "optimizes" it into one batch-wide
     guard (deepseek round 3). Idea: a two-thread forward-progress assertion
     proving the read lock is released between rows (logical, not timing).
   - Type: test infrastructure, low risk.
   - Disposition target: **fix in PR #22703** by adding a deterministic
     non-ignored test hook that proves a write lock can be acquired between row
     emissions during `commit_batch`.

7. **Dead code: `ready_notify` set up but never awaited** (pre-existing,
   outside the offload diff).
   - Evidence: `src/crates/netflow-plugin/src/facet_runtime.rs` —
     `ready_notify: Notify` and `mark_ready()` exist; `notify_waiters()` is a
     no-op because nothing waits (minimax round 4).
   - Type: trivial cleanup.
   - Disposition target: **fix in PR #22703** together with item 3.

8. **Upstream SDK comment fix.**
   - Evidence: `systemd-journal-sdk` `GuardedCell` comment claims "NOT Send
     or Sync"; it is wrong about `Send` (`UnsafeCell` removes only `Sync`).
     Our compile probe proves `Log: Send`.
   - Type: upstream-comment defect, not a Netdata PR #22703 code finding.
   - Disposition target: **reject as a PR #22703 finding with evidence**.
     The Netdata-side contract is covered by the `Log: Send` compile probe.

9. **Backward clock step > bucket width can re-create committed receive-time
   buckets** (pre-existing; not a regression).
   - Evidence: today's inline flush has the same behavior; over-counts on the
     overlapped wall-clock span, no data loss. Recorded in the offload SOW
     risk register.
   - Additional evidence: published `systemd-journal-sdk-log-writer` 0.6.4
     documents and implements strict monotonic clamping for entry realtime
     (`src/log/mod.rs`: entry realtime and monotonic overrides are clamped to
     `last + 1us` floors).
   - Type: pre-existing edge case, not an offload regression.
   - Disposition target: **reject with evidence**. Adding another local
     receive-time monotonic layer would create future-dated flow records after
     a backward wall-clock step, which is worse for time-window queries.

10. **Torn-entry tolerant test reader can count a partially unreadable entry**
    (cubic re-review; test correctness bug).
    - Evidence: `src/crates/netflow-plugin/src/main_tests.rs:817-823` breaks
      only the inner data-object loop on `data_ref()` or decompression failure;
      `src/crates/netflow-plugin/src/main_tests.rs:832-833` can still count a
      timestamp read earlier from the same partially unreadable entry.
    - Type: test correctness. Blast radius: crash-recovery regression tests and
      confidence in torn journal recovery.
    - Disposition target: **fix in PR #22703**.
    - Recommendation (surgical): treat any unreadable data object or malformed
      `_SOURCE_REALTIME_TIMESTAMP` in the current entry as a torn entry and
      stop reading that journal file before counting it.

11. **Ignored crash test deadline is ineffective while stdout read blocks**
    (cubic re-review; manual test robustness bug).
    - Evidence: `src/crates/netflow-plugin/src/main_tests.rs:2850` blocks in
      `reader.lines()`; the deadline check at
      `src/crates/netflow-plugin/src/main_tests.rs:2858-2860` runs only after a
      line arrives or EOF occurs.
    - Type: test harness robustness. Blast radius: ignored manual SIGKILL crash
      test.
    - Disposition target: **fix in PR #22703**.
    - Recommendation (surgical): read child stdout on a helper thread and use a
      timed channel receive in the parent loop so the deadline is enforced even
      when the child stops producing lines.

## Approved Disposition

- **Fix in PR #22703:** items 1, 2, 3, 6, 7, 10, and 11.
- **Reject with evidence in this SOW:** items 4, 5, 8, and 9.
- **No deferred-only items:** per user decision on 2026-06-14.

## Analysis

Sources checked:

- Completed SOW `SOW-20260609-netflow-tier-commit-offload.md` (follow-up
  mapping section).
- The 4 external-review-round transcripts under
  `.local/audits/netflow-journal-sdk/`.
- `src/crates/netflow-plugin/Cargo.toml` pins the six published SDK aliases to
  0.6.2.
- `cargo search systemd-journal-sdk --limit 20` reports
  `systemd-journal-sdk-*` 0.6.4 as the current crates.io release set.
- `cargo info` for the six SDK crates reports installed/resolved 0.6.2 and
  latest 0.6.4.
- docs.rs `systemd-journal-sdk-core/latest` resolves to
  `systemd-journal-sdk-core` 0.6.4 and shows sibling SDK dependencies on
  `^0.6.4`.
- `.agents/sow/specs/netflow-tier-commit-workers.md` records the SDK-sensitive
  tier-worker contract, including the `Log: Send` compile probe.
- PR review against `upstream/master` confirmed the cubic startup-leak comment
  and identified a worker-mode sync telemetry gap.
- `systemd-journal-sdk-log-writer` 0.6.4 source checked locally from the
  published crate: no archive-sync opt-out exists; rotation still syncs the
  archived active file inside the SDK.
- `rg -n "journal-(common|core|engine|index|log-writer|registry)|systemd-journal-sdk"
  src/crates -g 'Cargo.toml'` shows remaining vendored journal consumers are
  outside `netflow-plugin`.

## Pre-Implementation Gate

Status: in-progress for the user-approved no-deferral follow-up scope.

Problem / root-cause model:

- The netflow plugin is intentionally isolated onto the published
  `systemd-journal-sdk-*` crates while log-viewer and OTEL still use the
  vendored workspace journal crates.
- Current netflow pins are 0.6.2, while crates.io reports 0.6.4 as the current
  SDK release set. Starting follow-up implementation on the old SDK would mix a
  dependency update with the facet behavioral fix.
- PR #22703's worker offload is correct only if worker lifecycle, telemetry,
  and facet-side disk work do not reintroduce receive-path stalls or lifecycle
  leaks.
- Some pending items are not legitimate PR #22703 findings: they are blocked
  by the published SDK API boundary, belong to other binaries, or are already
  mitigated by SDK behavior.

Evidence reviewed:

- `src/crates/netflow-plugin/Cargo.toml`: six published SDK aliases pinned to
  0.6.2.
- `src/crates/Cargo.lock`: six `systemd-journal-sdk-*` packages resolved to
  0.6.2.
- `cargo tree -p netflow-plugin --depth 1`: netflow directly depends on the six
  SDK crates at 0.6.2.
- `cargo search systemd-journal-sdk --limit 20`: current SDK release set is
  0.6.4.
- `cargo info systemd-journal-sdk-{common,core,engine,index,log-writer,registry}`:
  local resolution is 0.6.2 and latest is 0.6.4.
- `docs.rs/systemd-journal-sdk-core/latest`: latest docs resolve to 0.6.4.

Affected contracts and surfaces:

- Rust dependency manifest: `src/crates/netflow-plugin/Cargo.toml`.
- Rust lockfile: `src/crates/Cargo.lock`.
- Runtime surface: netflow raw journal writes, registry scans, index/query code,
  and tier-worker commit code that uses the SDK aliases.
- Ingest service startup and shutdown lifecycle.
- Tier worker commit telemetry exposed through `IngestMetrics` and charts.
- Facet runtime state/sidecar persistence and query autocomplete.
- No public Netdata Function schema, chart context, config, or docs contract is
  expected to change.

Clean-end-state target:

- `netflow-plugin` uses the published `systemd-journal-sdk-*` 0.6.4 crates for
  all six aliases.
- `src/crates/Cargo.lock` resolves those exact SDK packages to 0.6.4.
- PR #22703 starts tier workers only after the UDP listener is successfully
  bound and configured.
- Worker sync attempts are reflected in `tier_journal_syncs`.
- Facet runtime does not hold its state mutex across sidecar/state file disk
  writes in the rotation/update paths touched by this PR.
- Dead `ready_notify` state is removed.
- CI has a deterministic test for the per-row flow-index read-lock release
  property.
- Rejected items are recorded with concrete evidence and are not left as
  deferred follow-ups.
- Removed as redundant (i): no code/config/docs/tests become redundant from a
  version-only SDK refresh. `ready_notify` becomes redundant when item 7 is
  fixed and must be removed.
- Excluded coupled items (ii): consolidating `netdata-log-viewer` and OTEL from
  vendored journal crates is rejected as a PR #22703 finding because it is
  outside the tier-offload binary and was not introduced by this PR.
- Reference search: `rg -n "0\\.6\\.2|systemd-journal-sdk" src/crates/Cargo.toml
  src/crates/Cargo.lock src/crates/netflow-plugin/Cargo.toml
  src/crates/netflow-plugin -g 'Cargo.toml' -g 'Cargo.lock' -g '*.rs'` found
  only the netflow manifest pins and existing lockfile resolutions for the SDK
  version contract.

Existing patterns to reuse:

- Keep the dependency aliases (`journal-common`, `journal-core`,
  `journal-engine`, `journal-index`, `journal-log-writer`, `journal-registry`)
  so call sites continue to use existing `journal_*` imports.
- Preserve the existing scope split: only netflow uses the published SDK crates;
  cross-plugin journal consolidation stays tracked separately.

Risk and blast radius:

- Behavioral risk: SDK 0.6.4 may change journal writer, registry, index, or
  query behavior used by netflow.
- Compatibility risk: if 0.6.4 changes public APIs, compilation fails and the
  bump must stop for investigation.
- Performance risk: journal write/query hot paths can shift even without API
  changes; netflow crate tests and SDK-sensitive probes are required before item
  work starts.
- Security and sensitive-data risk: low; the work updates dependency metadata
  and must not record runtime logs or customer data in durable artifacts.

Sensitive data handling plan:

- Do not copy raw runtime journal entries, flow payloads, IPs, secrets, tokens,
  customer identifiers, private endpoints, or proprietary incident details into
  SOWs, specs, docs, skills, instructions, or code comments.
- Validation evidence records only commands, pass/fail status, and sanitized
  summaries.

Implementation plan:

1. Completed prerequisite: update the six published SDK aliases in
   `src/crates/netflow-plugin/Cargo.toml` from 0.6.2 to 0.6.4.
2. Completed prerequisite: refresh `src/crates/Cargo.lock` for only the SDK
   crate set.
3. Fix startup lifecycle: bind/configure the UDP socket before spawning tier
   workers; add a bind-failure regression test.
4. Fix worker telemetry: count worker sync attempts; extend worker tests.
5. Fix facet runtime lock scope: snapshot under lock, write sidecars/state
   outside the mutex, and clear dirty state only when the written snapshot still
   matches current state.
6. Remove unused `ready_notify` state and notify call.
7. Add deterministic per-row lock-drop CI coverage.
8. Record evidence-based rejections for items 4, 5, 8, and 9.

Validation plan:

- `cargo update -p systemd-journal-sdk-common --precise 0.6.4` from
  `src/crates`; Cargo updates the whole interdependent SDK crate set.
- `cargo test -p netflow-plugin`
- `cargo test -p netflow-plugin journal_log_is_send`
- Targeted tests for worker startup lifecycle, worker sync telemetry, facet
  persistence, and per-row lock release.
- `rg -n "systemd-journal-sdk-.*0\\.6\\.2|version = \"0\\.6\\.2\""
  src/crates/netflow-plugin/Cargo.toml src/crates/Cargo.lock`

Artifact impact plan:

- AGENTS.md: no update expected; dependency version refresh does not change
  repository operating rules.
- Runtime project skills: no update expected; collector guidance remains valid.
- Specs: no update expected beyond this SOW; the tier-worker contract remains
  unchanged if tests pass.
- End-user/operator docs: no update expected; no public config or behavior
  contract changes are intended.
- End-user/operator skills: no update expected.
- SOW lifecycle: terminal dispositions are recorded in this SOW; the active SOW
  remains only as branch-local PR working memory until final cleanup before
  merge.

Open-source reference evidence:

- External source check used crates.io/docs.rs metadata for the SDK release set;
  no local mirrored third-party implementation was relevant to a version-only
  dependency refresh.

Open decisions:

- Resolved by user request: update the netflow published journal SDK aliases to
  0.6.4 before starting the follow-up item work.

## Implications And Decisions

1. Prerequisite SDK refresh.
   - Decision: update `netflow-plugin` only to `systemd-journal-sdk-*` 0.6.4
     before implementing follow-up items.
   - Recommendation class: surgical.
   - Implication: the SDK bump is isolated from the facet-lock behavioral fix,
     making any dependency regression easier to identify.
   - Risk: if 0.6.4 changes SDK semantics, validation may fail and item work
     must pause for investigation.

## Plan

1. Update netflow SDK manifest pins and lockfile to 0.6.4.
2. Fix tier-worker startup lifecycle so bind failure cannot leave workers alive.
3. Fix worker-mode tier sync telemetry.
4. Move facet state and sidecar disk writes outside the facet state mutex, while
   preserving stale-snapshot safety.
5. Remove dead `ready_notify` state.
6. Add deterministic CI coverage for the per-row flow-index read-lock release.
7. Reject non-PR findings with evidence: raw-rotation fsync blocked by the
   published SDK API, remaining vendored journal crates outside netflow,
   upstream SDK comment, and backward clock-step behavior.
8. Fix cubic re-review test-harness findings: stop counting partially
   unreadable torn entries, and make the ignored crash test deadline
   independent of blocking stdout reads.
9. Validate with targeted regression tests, full `netflow-plugin` tests, SOW
   hygiene checks, PR comment/CI checks, then commit and push.

## Execution Log

### 2026-06-14

- Updated `src/crates/netflow-plugin/Cargo.toml` so all six published SDK
  aliases use `systemd-journal-sdk-*` 0.6.4.
- Ran `cargo update -p systemd-journal-sdk-common --precise 0.6.4` from
  `src/crates`; Cargo updated exactly the six SDK packages to 0.6.4 and
  reported 104 unrelated dependencies left behind latest.
- Updated `src/crates/Cargo.lock` with the 0.6.4 versions and checksums for
  `systemd-journal-sdk-common`, `systemd-journal-sdk-core`,
  `systemd-journal-sdk-engine`, `systemd-journal-sdk-index`,
  `systemd-journal-sdk-log-writer`, and `systemd-journal-sdk-registry`.
- Moved tier worker startup behind successful UDP bind and receive-buffer
  setup in `src/crates/netflow-plugin/src/ingest/service/runtime.rs`.
- Added `e2e_ingest_bind_failure_does_not_start_tier_workers` to prove bind
  failure does not start tier workers.
- Counted worker sync attempts in `sync_with_failure_policy()` and final worker
  drain sync in `src/crates/netflow-plugin/src/ingest/tier_commit.rs`.
- Extended tier-worker E2E tests to assert `tier_journal_syncs` advances for
  worker commits and shutdown drains.
- Reworked `src/crates/netflow-plugin/src/facet_runtime.rs` persistence so
  sidecar deletion/writes and state-file writes happen after releasing the
  facet state mutex.
- Added a dedicated facet persistence lock plus dirty-generation re-check so
  stale snapshots cannot overwrite newer persisted facet state.
- Fixed adjacent facet reconciliation correctness: clearing non-empty active
  contributions now marks the state dirty, so a reload cannot resurrect stale
  published facets.
- Removed unused `ready_notify` and its `tokio::sync::Notify` import.
- Added deterministic tests for facet disk-write lock scope, stale snapshot
  skipping, cleared active contribution persistence, and per-row tier index
  read-lock release.
- Rejected items 4, 5, 8, and 9 with evidence in the pending-item list instead
  of deferring them.
- Fixed cubic re-review item 10 by making `readable_timestamp_counts()` stop
  the current journal-file scan before counting an entry when any data object in
  that entry is unreadable, cannot decompress, or has a malformed
  `_SOURCE_REALTIME_TIMESTAMP`.
- Fixed cubic re-review item 11 by reading the ignored crash test child's stdout
  on a helper thread and enforcing the deadline through timed channel receives.
- Fixed an E2E fixture race found during validation: `wait_for_ingest_progress()`
  now waits for the raw-entry counter to become stable across two polls before
  canceling the ingest service, so shutdown cannot race between raw write
  accounting and tier observation.

## Validation

Acceptance criteria evidence:

- `cargo tree -p netflow-plugin --depth 1 | rg -n "systemd-journal-sdk|journal"`
  reports all six direct SDK dependencies at 0.6.4.
- `git diff --stat` for implementation files is limited to
  the active SOW plus:
  `src/crates/netflow-plugin/src/facet_runtime.rs`,
  `src/crates/netflow-plugin/src/ingest/service/runtime.rs`,
  `src/crates/netflow-plugin/src/ingest/tier_commit.rs`, and
  `src/crates/netflow-plugin/src/main_tests.rs`.

Tests or equivalent validation:

- `cargo test -p netflow-plugin journal_log_is_send`: passed; 1 test passed,
  516 filtered, plus the filtered gRPC test target.
- `cargo test -p netflow-plugin runtime_persistence_skips_stale_snapshots`:
  passed.
- `cargo test -p netflow-plugin
  runtime_reconcile_persists_cleared_active_contributions`: passed.
- `cargo test -p netflow-plugin
  runtime_rotation_does_not_hold_state_lock_during_disk_writes`: passed.
- `cargo test -p netflow-plugin
  commit_batch_releases_flow_index_read_lock_between_rows`: passed.
- `cargo test -p netflow-plugin
  e2e_ingest_bind_failure_does_not_start_tier_workers`: passed.
- `cargo test -p netflow-plugin e2e_tier_commit_workers`: passed; both worker
  E2E tests passed.
- `cargo test -p netflow-plugin
  e2e_timestamp_source_first_switched_is_persisted_as_source_timestamp --
  --nocapture`: failed once after the cubic re-review patches, exposing the
  fixture cancellation race; passed after strengthening
  `wait_for_ingest_progress()`.
- `cargo test -p netflow-plugin`: passed; 492 tests passed, 25 ignored manual
  gates, and `tests/grpc_build.rs` passed 1/1.
- Cargo test commands emitted the pre-existing Rust warning that
  `OpenTierRow::{timestamp_usec, flow_ref, metrics}` are never read.

Same-failure scan:

- `rg -n 'systemd-journal-sdk-[^"]+", version = "0\.6\.2"'
  src/crates/netflow-plugin/Cargo.toml`: no matches.
- `awk '/^\[\[package\]\]/{pkg=""} /^name = "systemd-journal-sdk-/{pkg=$0}
  pkg && /^version = "0\.6\.2"/{print FILENAME ":" FNR ":" pkg " " $0}'
  src/crates/Cargo.lock`: no matches.
- Generic `version = "0.6.2"` still exists in `src/crates/Cargo.lock` for
  unrelated `rustls-platform-verifier`; it is not an SDK package.
- `rg -n "ready_notify|tokio::sync::Notify" src/crates/netflow-plugin/src`:
  no matches.
- `git diff --check`: passed.
- `.agents/sow/audit.sh`: failed on pre-existing legacy `SOW-NNNN` references
  under `.agents/sow/specs/snmp-traps/`; the audit reported this SOW has the
  required sections and the sensitive-data scan passed.

Sensitive data gate:

- No runtime logs, flow payloads, IP addresses, credentials, tokens, customer
  identifiers, private endpoints, or proprietary incident details were copied
  into durable artifacts.

## Artifact Maintenance Gate

- AGENTS.md: no update needed; repository operating rules are unchanged.
- Runtime project skills: no update needed; collector guidance remains valid.
- Specs: no update needed; the tier-worker contract remains unchanged and the
  `Log: Send` compile probe passed on SDK 0.6.4.
- End-user/operator docs: no update needed; no public config, Function schema,
  chart, or operator-facing behavior changed.
- End-user/operator skills: no update needed.
- SOW lifecycle: all follow-up items now have terminal dispositions here. The
  active SOW file is still branch-local working memory and must be removed
  before final merge.
