# SOW-20260614-netflow-tier-offload-followups - NetFlow tier-offload follow-up items

## Status

Status: in-progress

Sub-state: items extracted from the completed tier-offload SOW (shipped as
PR #22703). The requested prerequisite netflow published journal SDK refresh to
0.6.4 is implemented and validated. Follow-up item disposition is still pending;
each item below is independent work, and this SOW is the tracking record until
each item is implemented, rejected with evidence, or filed as a GitHub issue.

## Requirements

### Purpose

Track and dispose of the follow-up items surfaced by the netflow tier-commit
offload work (PR #22703) and its 4 external-review rounds, so none are lost.
Fit-for-purpose: each item must end as implemented, rejected-with-evidence,
or a tracked GitHub issue before this SOW closes.

### User Request

After confirming the tier-offload work was externally reviewed, the user
asked to close the current SOW and "create an SOW for the pending items and
let's address them."

### Assistant Understanding

Facts:

- The tier-offload work is merged-pending as PR #22703 (rebased onto master,
  487 tests green, MERGEABLE).
- The items below were identified during implementation/review and explicitly
  recorded as out-of-scope-for-the-offload-PR follow-ups (Scope discipline:
  each is independent — the offload clean end state is complete without it).

Unknowns:

- Which items the user wants done now vs. deferred vs. rejected (this is the
  decision this SOW exists to capture).

### Acceptance Criteria

- Every item below reaches a terminal disposition (implemented / rejected
  with evidence / tracked GitHub issue), recorded here.
- Prerequisite SDK refresh: `netflow-plugin` depends on published
  `systemd-journal-sdk-*` crates version 0.6.4, the lockfile resolves those
  crates at 0.6.4, and netflow validation passes before item-1 work starts.

## Pending Items

Each item: source, evidence, type, blast radius, recommendation.

1. **Facet state persistence holds a Mutex across disk writes** (pre-existing
   defect, independent of offload).
   - Evidence: `src/crates/netflow-plugin/src/facet_runtime.rs` —
     `observe_rotation` holds the facet Mutex across `write_sidecar_files` +
     `persist_state_locked` (disk I/O under the lock). Surfaced repeatedly in
     review as the heaviest facet-contention case.
   - Type: bug fix, behavioral. Blast radius: facet runtime, shared with the
     query task and the new tier workers.
   - Recommendation (long-term-best): fix it — move the disk writes outside
     the lock (snapshot under lock, write unlocked). Own PR; needs its own
     review round. **Strongest candidate to do now.**

2. **Stage B: move the raw-rotation fsync off the receive thread.**
   - Evidence: the last inline disk barrier on the receive thread is the raw
     journal's sync-on-archive; removing it needs an SDK opt-out
     (`with_sync_on_archive(false)`) plus a `nf-raw-sync` worker behind the
     `Rotated` lifecycle event.
   - Type: feature, BLOCKED on an upstream systemd-journal-sdk release.
   - Recommendation: defer until the SDK opt-out ships; track as a GitHub
     issue so it is not lost.

3. **Consolidate the remaining vendored journal crates** (carried from the
   migration SOW / #22665).
   - Evidence: `netdata-log-viewer` and the OTEL plugins still use the
     in-tree `journal-*` crates; only netflow moved to
     `systemd-journal-sdk-*` (updated to 0.6.4 as a prerequisite in this SOW).
   - Type: refactor/migration, cross-plugin. Blast radius: large (other
     plugins). Recommendation: separate effort, own SOW; track as issue.

4. **Timing-independent CI test for the per-row lock-drop property.**
   - Evidence: the per-row-read-lock gate is `#[ignore]` (timing-sensitive),
     so CI cannot catch a regression that "optimizes" it into one batch-wide
     guard (deepseek round 3). Idea: a two-thread forward-progress assertion
     proving the read lock is released between rows (logical, not timing).
   - Type: test infrastructure, low risk. Recommendation: nice-to-have; do
     opportunistically or track as issue.

5. **Dead code: `ready_notify` set up but never awaited** (pre-existing,
   outside the offload diff).
   - Evidence: `src/crates/netflow-plugin/src/facet_runtime.rs` —
     `ready_notify: Notify` and `mark_ready()` exist; `notify_waiters()` is a
     no-op because nothing waits (minimax round 4).
   - Type: trivial cleanup. Recommendation: fold into the item-1 facet PR if
     done, else a tiny standalone cleanup.

6. **Upstream SDK comment fix.**
   - Evidence: `systemd-journal-sdk` `GuardedCell` comment claims "NOT Send
     or Sync"; it is wrong about `Send` (`UnsafeCell` removes only `Sync`).
     Our compile probe proves `Log: Send`.
   - Type: one-line upstream PR (separate repo). Recommendation: low priority;
     file upstream when convenient.

7. **Backward clock step > bucket width can re-create committed receive-time
   buckets** (pre-existing; not a regression).
   - Evidence: today's inline flush has the same behavior; over-counts on the
     overlapped wall-clock span, no data loss. Recorded in the offload SOW
     risk register.
   - Type: known limitation. Recommendation: reject as not-worth-fixing
     (recommend NTP slewing in docs), or track as a low-priority issue.

## Recommended disposition (for user decision)

- **Do now (own PRs, own review rounds):** item 1 (facet lock — real defect),
  optionally bundling item 5 (dead code) into it.
- **Track as GitHub issues, defer:** items 2 (SDK-blocked), 3 (cross-plugin),
  4 (test infra), 6 (upstream).
- **Reject with evidence (doc-only):** item 7.

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

## Pre-Implementation Gate

Status: prerequisite SDK refresh completed; follow-up item implementation still
begins only after the user picks the items to address and the chosen item's gate
is filled.

Problem / root-cause model:

- The netflow plugin is intentionally isolated onto the published
  `systemd-journal-sdk-*` crates while log-viewer and OTEL still use the
  vendored workspace journal crates.
- Current netflow pins are 0.6.2, while crates.io reports 0.6.4 as the current
  SDK release set. Starting item-1 facet work on the old SDK would mix a
  dependency update with the facet behavioral fix.

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
- No public Netdata Function schema, chart context, config, or docs contract is
  expected to change.

Clean-end-state target:

- `netflow-plugin` uses the published `systemd-journal-sdk-*` 0.6.4 crates for
  all six aliases.
- `src/crates/Cargo.lock` resolves those exact SDK packages to 0.6.4.
- Removed as redundant (i): no code/config/docs/tests become redundant from a
  version-only SDK refresh.
- Excluded coupled items (ii): consolidating `netdata-log-viewer` and OTEL from
  vendored journal crates remains pending item 3 because it is cross-plugin,
  high-blast-radius migration work and not required for this prerequisite.
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

1. Update the six published SDK aliases in
   `src/crates/netflow-plugin/Cargo.toml` from 0.6.2 to 0.6.4.
2. Refresh `src/crates/Cargo.lock` for only the SDK crate set.
3. Run targeted netflow validation and reference searches for stale 0.6.2 SDK
   pins.

Validation plan:

- `cargo update -p systemd-journal-sdk-common --precise 0.6.4` from
  `src/crates`; Cargo updates the whole interdependent SDK crate set.
- `cargo test -p netflow-plugin`
- `cargo test -p netflow-plugin journal_log_is_send`
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
- SOW lifecycle: record prerequisite validation here; follow-up items still need
  terminal disposition before the SOW can complete.

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
2. Validate netflow compile/tests and SDK-sensitive probes.
3. Record validation and stale-reference search results in this SOW.

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

## Validation

Acceptance criteria evidence:

- `cargo tree -p netflow-plugin --depth 1 | rg -n "systemd-journal-sdk|journal"`
  reports all six direct SDK dependencies at 0.6.4.
- `git diff --stat` for implementation files is limited to
  `src/crates/netflow-plugin/Cargo.toml` and `src/crates/Cargo.lock`
  (18 insertions, 18 deletions).

Tests or equivalent validation:

- `cargo test -p netflow-plugin journal_log_is_send`: passed; 1 test passed,
  511 filtered, plus the filtered gRPC test target.
- `cargo test -p netflow-plugin`: passed; 487 tests passed, 25 ignored manual
  gates, and `tests/grpc_build.rs` passed 1/1.
- Both test commands emitted the pre-existing Rust warning that
  `OpenTierRow::{timestamp_usec, flow_ref, metrics}` are never read.

Same-failure scan:

- `rg -n 'systemd-journal-sdk-[^"]+", version = "0\.6\.2"'
  src/crates/netflow-plugin/Cargo.toml`: no matches.
- `awk '/^\[\[package\]\]/{pkg=""} /^name = "systemd-journal-sdk-/{pkg=$0}
  pkg && /^version = "0\.6\.2"/{print FILENAME ":" FNR ":" pkg " " $0}'
  src/crates/Cargo.lock`: no matches.
- Generic `version = "0.6.2"` still exists in `src/crates/Cargo.lock` for
  unrelated `rustls-platform-verifier`; it is not an SDK package.

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
- SOW lifecycle: prerequisite evidence recorded here; follow-up items still need
  terminal disposition before this SOW can complete.
