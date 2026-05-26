# SOW-0039 - SNMP Trap Plugin: Collector Consistency Bundle + systemd-journal Facets + User Docs + SOW-0032 Closeout + Final Merge Gate

## Status

Status: open

Sub-state: queued in `.agents/sow/pending/`. Depends on SOW-0035 + SOW-0036 + SOW-0037 + SOW-0038 completion. **This SOW is the merge gate** — SOW-0035 through SOW-0038 land on a feature branch; the single PR sequence that becomes mergeable to `master` ends here.

## Requirements

### Purpose

Final SOW that takes the feature-branch trap subsystem (SOW-0035–0038) to merge-ready state:

1. **Collector consistency bundle** — the 7 artifacts AGENTS.md mandates "must move together": `metadata.yaml`, `config_schema.json`, stock `.conf`, `health.d/snmp_trap.conf`, `README.md`, `taxonomy.yaml`. Passes the fatal CI gates `check-markdown.yml` and `check_collector_taxonomy.py`.
2. **systemd-journal collector default-facet registration** — add the plugin-controlled `TRAP_*` field names to `SYSTEMD_KEYS_INCLUDED_IN_FACETS` in `src/collectors/systemd-journal.plugin/systemd-journal.c` so the Cloud Logs UI shows them as default facets. Operators can still pick any other field (including `TRAP_TAG_*` labels) via the UI facet selector — the macro is the default selection, not a whitelist.
3. **End-user AI skill** `query-snmp-traps` (`docs/netdata-ai/skills/query-snmp-traps/`) with how-tos catalog.
4. **User documentation** for the offline MIB-to-YAML conversion workflow (per spec §7 + §14) — operators run `tools/snmp-traps-profile-gen/` to convert custom MIBs and drop the resulting YAMLs into `/etc/netdata/go.d/snmp.trap-profiles/`.
5. **SOW-0032 closeout** — write `comparative-analysis.md` synthesizing shipped behavior across SOW-0035–0038 vs the 16 Phase A cohort systems; mark SOW-0032 `Status: completed`; move to `.agents/sow/done/`.
6. **Final merge** — single PR sequence from feature branch to master; SOW-0035 through SOW-0039 all marked `Status: completed` and moved to `.agents/sow/done/` in the merge commit (per AGENTS.md "one-commit close" rule).

### User Request

User-added 5th SOW per user request: "We need to add metadata.yaml and the rest — this can be done at the end as an extra SOW." The merge-gate framing resolves the SOW-0035 mergeability hazard that all 7 reviewers flagged in round 1.

### Assistant Understanding

Facts:

- AGENTS.md "Collector Consistency Requirements" mandates the 7 artifacts move together: code + `metadata.yaml` + `config_schema.json` + stock `.conf` + `health.d/*.conf` + `README.md` + `taxonomy.yaml`.
- `check_collector_taxonomy.py` (in `.github/workflows/check-markdown.yml`) is a fatal CI gate — `taxonomy.yaml` must resolve to real metadata.yaml contexts.
- systemd-journal facet registration is **100% static**: `SYSTEMD_KEYS_INCLUDED_IN_FACETS` macro at lines 99-192 of `src/collectors/systemd-journal.plugin/systemd-journal.c`. New facets require a code change. There is no runtime auto-discovery.
- The metric universe to document in `metadata.yaml` comes from SOW-0036 M4 (`snmp.trap.events`, `snmp.trap.errors.*` dimensions) + SOW-0037 M2 (`snmp.trap.dedup_suppressed`) + operator-opted-in per-OID metrics from SOW-0037 M4 (template/example).
- The conversion tools are already shipped under `tools/snmp-traps-profile-gen/` (commit `4056ffac1d`); this SOW's user docs explain the operator workflow but does not author new tools.
- SOW-0032 (`.agents/sow/current/SOW-0032-20260522-snmp-trap-comparative-analysis.md`) was intentionally held pending implementation; its synthesis is done here against shipped behavior.
- Public-skill convention per AGENTS.md: canonical under `docs/netdata-ai/skills/query-snmp-traps/`, symlink `.agents/skills/query-snmp-traps` → `../../docs/netdata-ai/skills/query-snmp-traps`.
- Public-skill catalog rule per AGENTS.md: `how-tos/INDEX.md` is live (assistants add new how-tos as they answer concrete operator questions).

Inferences:

- The CI gate failure mode for `check_collector_taxonomy.py` is the most likely cause of a merge attempt failing — taxonomy.yaml is small, easy to forget, and only validated at CI time.

Unknowns:

- Final list of public-skill how-tos to seed `INDEX.md` (settled in M4 against the operator-features.md cohort document).

### Acceptance Criteria

- M1: `metadata.yaml`, `config_schema.json`, stock `.conf` shipped under `src/go/plugin/go.d/...` (or Rust equivalent path per SOW-0035 M1 decision). `metadata.yaml` documents every metric from SOW-0036 M4 + SOW-0037 M2 + SOW-0037 M4 (operator opt-in example).
- M2: `health.d/snmp_trap.conf` + `README.md` + `taxonomy.yaml` shipped. `check-markdown.yml` + `check_collector_taxonomy.py` pass locally and in CI.
- M3: `SYSTEMD_KEYS_INCLUDED_IN_FACETS` macro in `systemd-journal.c` extended with the plugin-controlled `TRAP_*` field names (see milestone body for the explicit list). Verified: trap journal entries appear as default facets in the Cloud Logs UI; `TRAP_TAG_*` operator/profile labels appear in the UI's facet-picker for operator selection.
- M4: `docs/netdata-ai/skills/query-snmp-traps/SKILL.md` + `how-tos/INDEX.md` ship with at least 5 seeded how-tos (by category, by severity, by source IP, dedup-summary filter, varbind grep). Symlink `.agents/skills/query-snmp-traps` created (`ln -srfn`).
- M5: user documentation for offline MIB-to-YAML conversion workflow shipped (likely in plugin README and/or under `docs/`). Covers: install `tools/snmp-traps-profile-gen/` dependencies, run extract/classify/emit, drop resulting YAML, trigger DynCfg reload.
- M6: SOW-0032 `Status: completed`; `.agents/sow/specs/snmp-traps/comparison/comparative-analysis.md` ships; SOW file moved to `.agents/sow/done/` in the merge commit. All five SOWs (0035-0039) marked `Status: completed` and moved to `.agents/sow/done/` in the same merge commit per AGENTS.md.

## Milestones

### M1 — Collector consistency bundle (part 1): metadata.yaml + config_schema.json + stock conf

- `metadata.yaml` documents:
  - Every metric context from SOW-0036 M4: `snmp.trap.events`, `snmp.trap.errors` (full dimension universe per spec §12).
  - `snmp.trap.dedup_suppressed` from SOW-0037 M2 (when opt-in dedup is enabled).
  - Example operator-opted-in per-OID metric (`snmp.trap.cisco_config_changes` per spec §7.5).
  - Units, descriptions, default alerts for each metric.
- `config_schema.json` matches the DynCfg job-config schema from SOW-0036 M3 (per-job structure per spec §7.5).
- Stock `.conf` example file with `local` job (disabled by default per spec §5) commented out + explanatory comments.

Cohort reference: existing collector `metadata.yaml` examples; `integrations-lifecycle` skill.

Reviewers: 3 rotating (group B: glm/mimo/deepseek).

### M2 — Consistency bundle (part 2): health.d + README + taxonomy + CI gate pass

- `health.d/snmp_trap.conf` with default alerts:
  - `snmp.trap.errors.*` dimensions above thresholds (per-error-type alert templates).
  - Per-severity rate alarms on `snmp.trap.events` (e.g., crit rate sustained).
  - `snmp.trap.dedup_suppressed` rate (only fires when dedup is enabled).
- `README.md`:
  - What the plugin monitors (per-job listeners receiving SNMP traps).
  - How to configure (DynCfg per-job; reference §7.5 schema).
  - Per-job retention semantics.
  - Operator MIB workflow (forward-reference to M5 user docs).
  - Troubleshooting (port-binding failures, USM auth failures, etc.).
- `taxonomy.yaml` entry for the trap plugin's contexts (placement in the dashboard TOC).
- CI: `check-markdown.yml` + `check_collector_taxonomy.py` pass locally and in CI. All 7 consistency artifacts move together in this milestone's commit.

Cohort reference: existing collector bundles; `integrations-lifecycle` skill; `netdata-alerts` skill.

Reviewers: all 7 (final consistency-bundle pass — gates merge).

### M3 — systemd-journal collector facet registration

- Edit `src/collectors/systemd-journal.plugin/systemd-journal.c` `SYSTEMD_KEYS_INCLUDED_IN_FACETS` macro (lines 99-192 area) to add the DEFAULT facet selection for trap entries — the operator can still pick any other field as a facet via the Logs UI selector, but this list is what the UI shows by default:
  - **Plugin-controlled `TRAP_*` fields**: `TRAP_REPORT_TYPE`, `TRAP_OID`, `TRAP_NAME`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_PDU_TYPE`, `TRAP_VERSION`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`, `TRAP_NEIGHBORS`.
  - **Standard fields the trap writer sets**: `_HOSTNAME` (source device hostname) is already in the existing macro; verify it appears in default facets so operators can quickly filter by source device.
  - **Existing Netdata fields the trap plugin populates** (no change needed — already in the macro): `ND_LOG_SOURCE`, `ND_NIDL_NODE`.
  - **`TRAP_TAG_*` (profile + operator labels)**: NOT pre-registered because the keys are dynamic (each profile / each operator picks different label keys). Operators discover them via the Logs UI facet selector and pin the ones they care about.
  - **Summary-only fields** (`TRAP_SUPPRESSED_COUNT`, `TRAP_SUPPRESSED_FINGERPRINTS`, `TRAP_REPORT_PERIOD_SEC`, `TRAP_JSON`): NOT in default facets — they are payload/metadata not useful as facets.
- Optional: register field transformations in `systemd_journal_register_transformations()` for display formatting (e.g., severity slug → colored label). Not required for first release.
- Verification: write trap entries to a test journal, open in Logs UI, confirm: (a) default facets appear from the registered list above; (b) `TRAP_TAG_*` operator labels appear in the UI's "add facet" picker.

Cohort reference: existing facet registrations in `systemd-journal.c`; commit `6a515000ac` (fix to facet-fetching path).

Reviewers: 3 rotating (group A: kimi/qwen/minimax).

### M4 — End-user AI skill `query-snmp-traps`

- `docs/netdata-ai/skills/query-snmp-traps/SKILL.md` + `how-tos/INDEX.md` per AGENTS.md public-skill convention.
- Seeded how-tos:
  1. Find recent security traps from a specific device.
  2. Filter by severity (e.g., crit + emerg across the fleet).
  3. Identify the top trap senders in the last hour.
  4. Inspect dedup summary entries during a flap storm.
  5. Grep `TRAP_JSON` for a specific varbind value.
- Uses existing `query-netdata-cloud` and `query-netdata-agents` skills as transport.
- Symlink: `ln -srfn ../../docs/netdata-ai/skills/query-snmp-traps .agents/skills/query-snmp-traps`. Verify with `readlink -f .agents/skills/query-snmp-traps`.

Cohort reference: existing public skills under `docs/netdata-ai/skills/`.

Reviewers: 3 rotating (group B: glm/mimo/deepseek).

### M5 — User documentation: offline MIB-to-YAML conversion workflow

- Documentation in the plugin `README.md` (M2) + a dedicated section under `docs/` or inside the skill from M4 covering:
  - Install `tools/snmp-traps-profile-gen/` dependencies (Python venv, pysmi).
  - Run extract/classify/emit pipeline against operator-provided MIBs.
  - Drop resulting YAML into `/etc/netdata/go.d/snmp.trap-profiles/`.
  - Trigger DynCfg reload (or restart plugin) to pick up new profile.
  - Verify in Logs UI that previously-unknown OIDs now resolve with names + categories.
- Worked example using a well-known vendor MIB not in the OOB pack (e.g., a small open-source NMS MIB).

Cohort reference: spec §7 Custom MIB workflow section; existing `tools/snmp-traps-profile-gen/` documentation.

Reviewers: 3 rotating (group A: kimi/qwen/minimax).

### M6 — SOW-0032 closeout + final merge gate

- Write `.agents/sow/specs/snmp-traps/comparison/comparative-analysis.md` synthesizing shipped behavior across SOW-0035–0039 vs the 16 Phase A cohort systems. Cite spec §16 cohort-win audit + each cohort system's spec doc.
- Re-run the SNMP trap SDK-backed journal benchmarks after the SDK benchmark/profile/optimize work is either completed or explicitly accepted as a tracked risk. SOW-0035 measured fast queue acceptance but only about 3K-6K SDK-backed journal appends/sec on the workstation; the final merge gate must decide whether this is acceptable for release or must wait for `systemd-journal-sdk/.agents/sow/pending/SOW-0009-20260523-benchmark-profile-optimize.md`.
- Update SOW-0032 (`.agents/sow/current/SOW-0032-20260522-snmp-trap-comparative-analysis.md`) `Status: completed`; move to `.agents/sow/done/`.
- Mark all five SOWs (0035-0039) `Status: completed`; move to `.agents/sow/done/`.
- Final merge: single commit (or commit sequence per AGENTS.md "one-commit close" rule) lands all of SOW-0035–0039's work to `master` with the consistency bundle satisfying the CI gate.

Cohort reference: 16 Phase A specs under `.agents/sow/specs/snmp-traps/`; spec §16; spec §17.

Reviewers: all 7 (final merge approval — gates production).

## Reviewer Protocol

- M2 + M6: all 7 reviewers (bundle consistency + final merge approval).
- M1, M3, M4, M5: 3 rotating reviewers per round (alternating groups A/B).
- Fix-cycle: same reviewers as the round being fixed.

## Pre-Implementation Gate

Status: blocked

Reason: depends on SOW-0035 + SOW-0036 + SOW-0037 + SOW-0038 completion. Full gate filled at activation — depends on the metric universe + config schema + journal field set + OTLP shape delivered by the prior SOWs.

## Plan

Sequential M1 → M6. M6 is the final merge commit; SOW-0035–0039 close together.

## Execution Log

### 2026-05-25

SOW created as the 5th SOW in the revised lineup, owning the collector consistency bundle + systemd-journal facets + user docs + SOW-0032 closeout + final merge gate. Resolves the SOW-0035 mergeability hazard flagged by all 7 reviewers in round 1.

## Validation

Acceptance criteria evidence: pending.
Tests or equivalent validation: pending — CI must pass on `check-markdown.yml` and `check_collector_taxonomy.py`.
Real-use evidence: pending — Logs UI must show new facets after M3.
Reviewer findings: pending.
Same-failure scan: pending.
Sensitive data gate: pending — no secrets in user docs or worked examples.
Artifact maintenance gate: pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

Tracked items to schedule after SOW-0039 closes:

- **Align NetFlow plugin retention default with the trap plugin's intentional deviation** (spec §11): NetFlow currently defaults `duration_of_journal_files` to `7d`; the trap plugin uses `null` (size-only eviction) because trap forensic data should not age out by time alone. Create a follow-up SOW to update NetFlow's `default_retention_duration_of_journal_files()` in `src/crates/netflow-plugin/src/plugin_config/defaults.rs` to `null` so the two plugins agree on the size-only default. Coordinate with NetFlow maintainers.
- **SDK-backed journal append throughput gate**: SOW-0035's SDK-backed benchmarks showed excellent queue acceptance but actual writer append/drain throughput below the original tens-of-thousands/sec target on the workstation. Before final merge, consume the SDK SOW-0009 benchmark/optimization result or record an explicit release decision accepting the measured throughput with scaling guidance.
- **`display_hint` rendering for varbinds**: the field is documented as reserved-future in `profile-format.md`. When the plugin renderer is taught to honor `display_hint` (MAC, IPv4, custom formatters), the extractor must learn to pull `DISPLAY-HINT` from TEXTUAL-CONVENTION definitions in the same regeneration cycle. Track as a post-0039 enhancement.
- **`decode_error_summary` report type**: `TRAP_REPORT_TYPE` enum reserves this slot per §11 but no SOW in the 5-SOW lineup implements it. When decode-error rates become operator-visible enough to warrant batched summary entries (analogous to the dedup summary), open a follow-up SOW.
- **`dimension_from_varbind` cardinality table**: SOW-0037 M4 rejects unbounded-cardinality varbinds at config load. The reference cardinality table (which varbind types are bounded vs unbounded) needs an authoritative source — likely embedded in the profile schema or shipped as a separate `varbind-cardinality.yaml`. Open a follow-up SOW.

## Regression Log

None yet.
