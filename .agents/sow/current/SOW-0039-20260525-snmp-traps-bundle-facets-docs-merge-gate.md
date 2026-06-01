# SOW-0039 - SNMP Trap Plugin: Collector Consistency Bundle + systemd-journal Facets + User Docs + SOW-0032 Closeout + Final Merge Gate

## Status

Status: in-progress

Sub-state: activated on 2026-05-28 after SOW-0035, SOW-0036, SOW-0037, and SOW-0038 reached implementation-complete state on the feature branch. **This SOW is the merge gate** — SOW-0035 through SOW-0038 land on a feature branch; the single PR sequence that becomes mergeable to `master` ends here.

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
- SOW-0032 was intentionally held pending implementation; its synthesis is done here against shipped behavior and the SOW is closed to `.agents/sow/done/SOW-0032-20260522-snmp-trap-comparative-analysis.md`.
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
- M5: user documentation for offline MIB-to-YAML conversion workflow shipped (plugin README/generated docs plus public skill how-to). Covers: run the installed Go helper `snmp-trap-profile-gen`, inspect generated catalogue/profile YAML, drop resulting YAML, trigger profile reload through the registered Function, and verify through trap journal queries. The legacy Python pipeline remains source-tree reference tooling only.
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
  - Run the installed `/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen` helper against operator-provided MIBs.
  - Inspect generated `profiles/catalogue.json` and YAML profile output.
  - Drop resulting YAML into `/etc/netdata/go.d/snmp.trap-profiles/`.
  - Trigger `snmp_traps:reload-profiles` (or re-apply/restart the trap job when no active job exists) to pick up new profiles.
  - Verify in Logs UI that previously-unknown OIDs now resolve with names + categories.
- Worked example using a well-known vendor MIB not in the OOB pack (e.g., a small open-source NMS MIB).

Cohort reference: spec §7 Custom MIB workflow section; existing `tools/snmp-traps-profile-gen/` documentation.

Reviewers: 3 rotating (group A: kimi/qwen/minimax).

### M6 — SOW-0032 closeout + final merge gate

- Write `.agents/sow/specs/snmp-traps/comparison/comparative-analysis.md` synthesizing shipped behavior across SOW-0035–0039 vs the 16 Phase A cohort systems. Cite spec §16 cohort-win audit + each cohort system's spec doc.
- Re-run the SNMP trap full packet-to-journal benchmark after each SDK bump or local hot-path change that can affect writer performance. SOW-0035's 2026-05-28 re-check with SDK `go/v0.3.0` measured about 55K-75K persisted traps/sec for the synthetic v2c profile-hit path on the workstation. The early 2026-06-01 `go/v0.4.0` repeat measured 30.5K-38.0K persisted traps/sec before local optimization. SOW-0045 then optimized the Netdata writer hot path and measured 62.5K-72.6K persisted traps/sec for 30,000-packet runs and 63.3K-66.0K for 100,000-packet runs.
- Update SOW-0032 (`.agents/sow/current/SOW-0032-20260522-snmp-trap-comparative-analysis.md`) `Status: completed`; move to `.agents/sow/done/`.
- Mark all five SOWs (0035-0039) `Status: completed`; move to `.agents/sow/done/`.
- Final merge: single commit (or commit sequence per AGENTS.md "one-commit close" rule) lands all of SOW-0035–0039's work to `master` with the consistency bundle satisfying the CI gate.

Cohort reference: 16 Phase A specs under `.agents/sow/specs/snmp-traps/`; spec §16; spec §17.

Reviewers: all 7 (final merge approval — gates production).

## Reviewer Protocol

- M2 + M6: all 7 reviewers (bundle consistency + final merge approval).
- M1, M3, M4, M5: 3 rotating reviewers per round from `glm`, `kimi`, `minimax`, and `qwen`.
- Fix-cycle: same reviewers as the round being fixed.
- `mimo` is unavailable due to quota exhaustion and is not used. `deepseek/deepseek-v4-pro` may be used for implementation work, but not with `--agent code-reviewer` because it needs write access.
- External assistant process rule: run with stdin closed via `</dev/null`.

## Pre-Implementation Gate

Status: passed for activation on 2026-05-28.

Problem/root-cause model:

- The SNMP trap runtime implementation exists on the feature branch, but it is not mergeable until the collector-facing artifacts, journal default facets, operator documentation, public AI skill, and comparative-analysis closeout are consistent with the shipped behavior.
- The root merge risk is not packet parsing. It is artifact drift: a collector can work locally while CI or users fail because `metadata.yaml`, `config_schema.json`, stock config, health alerts, README, taxonomy, docs, and journal facets disagree with the code.

Evidence reviewed:

- SOW-0035 is paused as implementation-complete for the foundation/MVP, with test isolation repaired and full ingestion throughput clarified on 2026-05-28.
- SOW-0036 is paused as implementation-complete for SNMPv3, INFORM, allowlist/rate-limit, DynCfg schema, and self-metrics.
- SOW-0037 is paused as implementation-complete for enrichment, opt-in dedup, profile reload, and operator metrics.
- SOW-0038 is completed and covers throughput benchmark harness, dynamic engineID, OTLP exporter, and metadata-handling regression repair.
- `src/go/plugin/go.d/collector/snmp_traps/collector.go`, `metrics.go`, `config_schema.json`, `charts.yaml`, `serialize.go`, and `otlp.go` are the current code contracts for M1/M2/M3 documentation and metadata.
- `src/collectors/systemd-journal.plugin/systemd-journal.c` owns the static default-facet registration for systemd-journal logs.
- AGENTS.md collector consistency rules require code, `metadata.yaml`, `config_schema.json`, stock config, health alerts, README, and `taxonomy.yaml` to move together before PR.

Affected contracts and surfaces:

- Go collector integration metadata and in-app configuration help.
- DynCfg schema presented to operators.
- Stock `/etc/netdata/go.d/snmp.trap.conf` example.
- Health alert templates for trap event/error/dedup rates.
- Dashboard taxonomy placement for trap plugin metric contexts.
- systemd-journal default facet list for Cloud Logs.
- Operator docs and public Netdata AI skills.
- SOW lifecycle for SOW-0032 and SOW-0035 through SOW-0039.

Existing patterns to reuse:

- Existing go.d collector `metadata.yaml`, `config_schema.json`, stock `.conf`, README, and taxonomy patterns.
- Existing systemd-journal facet macro style in `systemd-journal.c`.
- Existing public skills under `docs/netdata-ai/skills/query-netdata-cloud/` and `docs/netdata-ai/skills/query-netdata-agents/`.
- Existing integrations-lifecycle validation commands and taxonomy checker patterns.

Risk and blast radius:

- High operator-facing risk if docs/schema/examples imply defaults or fields the runtime does not implement.
- Medium CI risk around taxonomy and generated integration metadata.
- Medium UX risk if too many journal fields are default facets or if dynamic `TRAP_TAG_*` labels are incorrectly treated as static facets.
- Low runtime risk for M1/M2 docs/metadata edits, higher for M3 C facet registration because it touches the systemd-journal collector.

Sensitive data handling plan:

- Do not copy real trap payloads, SNMP communities, USM secrets, device hostnames, customer identifiers, public IPs, or local live journal contents into SOWs, docs, skills, tests, or examples.
- Use placeholders such as `${env:SNMP_COMMUNITY}`, `[DEVICE_IP]`, `[TRAP_OID]`, and sanitized example OIDs.
- Worked examples must use non-sensitive fixtures, documentation-only placeholders, or private RFC 5737 addresses.

Implementation plan:

1. M1: audit current runtime config/metrics and implement the collector consistency part 1 artifacts (`metadata.yaml`, `config_schema.json`, stock config).
2. M2: add health alerts, README, taxonomy, and run local consistency/taxonomy validation.
3. M3: add default journal facets for plugin-controlled static `TRAP_*` fields.
4. M4: add the public `query-snmp-traps` skill and `.agents/skills/` symlink.
5. M5: document the offline MIB-to-YAML workflow.
6. M6: close SOW-0032, rerun benchmark/validation, and close SOW-0035 through SOW-0039 together when merge-ready.

Validation plan:

- Focused Go tests for `snmp_traps` after metadata/schema changes that affect embedded config schema.
- JSON/schema validation for `config_schema.json`.
- Integration/taxonomy validation using the repository's existing scripts.
- `git diff --check`.
- Focused C build or static validation for the systemd-journal facet edit where a narrow command exists.
- External review rounds on meaningful batches with `glm`, `kimi`, `minimax`, and `qwen`.

Artifact impact plan:

- `AGENTS.md`: updated to add the `query-snmp-traps` public-skill index entry after M4 introduced a new public/operator skill.
- Runtime project skills: update only if this SOW exposes a reusable developer workflow gap.
- Specs: update `.agents/sow/specs/snmp-traps/` if shipped behavior differs from the spec.
- End-user/operator docs: update README and any docs page added for the MIB conversion workflow.
- End-user/operator skills: add `docs/netdata-ai/skills/query-snmp-traps/` plus the required symlink.
- SOW lifecycle: move this SOW from `pending/` to `current/` at activation; close SOW-0032 and SOW-0035 through SOW-0039 only at final merge gate.

Open decisions:

- None for M1. If implementation uncovers a real product decision, record options in this SOW before code changes.

## Plan

Sequential M1 -> M6. M6 is the final merge commit; SOW-0035-0039 close together.

## Execution Log

### 2026-05-25

SOW created as the 5th SOW in the revised lineup, owning the collector consistency bundle + systemd-journal facets + user docs + SOW-0032 closeout + final merge gate. Resolves the SOW-0035 mergeability hazard flagged by all 7 reviewers in round 1.

### 2026-05-28

Activated after the user requested continuing implementation. Pre-implementation gate filled with current dependency state, SDK `go/v0.3.0` throughput clarification, and updated reviewer constraints (`mimo` unavailable; external assistants run with stdin closed).

### 2026-05-28 — M1 implementation

Created `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml`.

Evidence of coverage against M1 acceptance criteria:

- **Every static metric context from charts.yaml**: `snmp.trap.events` (8 dimensions: state_change, config_change, security, auth, license, mobility, diagnostic, unknown), `snmp.trap.errors` (14 dimensions: unknown_oid through otlp_export_failed), `snmp.trap.dedup_suppressed` (1 dimension: suppressed). All documented in `metrics.scopes[0].metrics` with per-dimension descriptions, units (`events/s`, `errors/s`), and chart types (`stacked`, `line`).
- **Operator opt-in dynamic contexts**: `dynamic_context_prefixes` entry for `snmp.trap.` with explanation of the `metrics` config section, matching the SNMP collector pattern in `src/go/plugin/go.d/collector/snmp/metadata.yaml:1148-1150`.
- **Config options**: All 18 properties from `config_schema.json` are listed in `setup.configuration.options`, documented with groups (Collection, Listener, SNMP, SNMPv1/2c, SNMPv3, Enrichment, Security, Rate limiting, Deduplication, OTLP export, Retention, Overrides, Per-OID metrics, Virtual node), default values, and detailed descriptions matching the JSON schema constraints. Four config examples: Basic (v1/v2c), SNMPv3 with static USM, Dynamic engine ID discovery, With dedup and per-OID metrics.
- **Sanitized examples**: All examples use secret-reference placeholders, wildcard listener addresses, and stock-profile trap OIDs. No real communities, USM keys, device hostnames, or customer identifiers.
- **Style**: Follows the SNMP metadata.yaml pattern (meta/overview/setup/metrics/troubleshooting structure), ping metadata.yaml compact metric definitions, and go.d conventions for `plugin_name`, `module_name`, file paths, categories, and keywords.

Config schema and stock config audit:

- `config_schema.json`: Already present and comprehensively covers all Config struct fields. JSON passes `python3 -c "import json; json.load(...)"` validation. No changes needed.
- Stock config (`src/go/plugin/go.d/config/go.d/snmp_traps.conf`): Already ship-complete with all options documented as commented-out YAML, including v3 USM with secret references, dynamic engine ID, dedup, OTLP, retention, and per-OID metrics. No changes needed.

Validation executed:

- `python3 -c "import yaml; yaml.safe_load(...)"` — metadata.yaml parses cleanly.
- `python3 -c "import json; json.load(...)"` — config_schema.json parses cleanly.
- `git diff --check` — no whitespace issues.
- Config option consistency cross-check (metadata vs config_schema.json vs Config struct): zero mismatches — all 18 properties in metadata exactly match the 18 properties in config_schema.json.
- Dimension cross-check (metadata vs charts.yaml): events has 8 dims (match), errors has 14 dims (match), dedup_suppressed has 1 dim (match).
- Coordinator follow-up fixed metadata schema parent indentation, corrected the trap profile-format link to `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`, and replaced the arbitrary synthetic per-OID metric example with stock-profile Cisco trap OIDs.
- `python3 integrations/gen_integrations.py` passed after the metadata schema fixes.

### 2026-05-28 — M2 health alert decision required

M2 asks for default alerts on `snmp.trap.errors.*`, per-severity event rates, and `snmp.trap.dedup_suppressed`.

Evidence:

- `src/go/plugin/go.d/collector/snmp_traps/metrics.go` exposes `trapEvents` by category only (`state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, `unknown`).
- `src/go/plugin/go.d/collector/snmp_traps/charts.yaml` defines `snmp.trap.events` with category dimensions only.
- Trap severity is present on journal/log entries as `TRAP_SEVERITY`, but there is no `snmp.trap.severity` metric context for health alerts.

Decision:

- A accepted by the user on 2026-05-28. Add runtime severity counters and a dedicated chart context now, then implement per-severity default alerts.

Reason:

- The broad SNMP traps design spec already required event counters by severity.
- SOW-0036 narrowed the implemented metric slice to category-only event counters.
- SOW-0039's health-alert milestone correctly expects severity-aware alert inputs, so this SOW repairs the spec-to-implementation drift instead of weakening the health alert requirement.

Implementation direction:

- Keep `snmp.trap.events` as category-rate metrics.
- Add a separate bounded `snmp.trap.severity` metric context for the closed 8-severity taxonomy (`emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug`).
- Wire health alerts to `snmp.trap.severity`, not to mixed category/severity dimensions in `snmp.trap.events`.

### 2026-05-28 — M2 severity counters + health alerts implementation

Implemented the accepted option A:

- **metrics.go** (`metrics.go:40-48`): Added `trapSeverities` struct with 8 uint64 fields (`emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug`). Added `severities trapSeverities` field to `perJobMetrics`. Added `incSeverity(Severity)` method with bounded 8-way switch + default-to-notice fallback for unknown values. Added package-level `incTrapSeverity(jobName, severity)` and collector-level `c.incTrapSeverity(severity)` mirrors. Added `collectSeverities()` function emitting `snmp_trap_severity_*` counter IDs via the existing `metrix.CollectorStore` pattern. Wired `collectSeverities` into `collectMetrics` alongside `collectEvents`/`collectErrors`/`collectDedup`.

- **collector.go** (`collector.go:543`): Added `c.incTrapSeverity(entry.Severity)` in the trap write success path, immediately after the existing `c.incTrapEvents(cat)`. Severity comes from the entry after profile resolution and override application. Normal profile loading rejects empty or unknown severities; `incSeverity` still keeps a defensive bounded fallback to `notice` for impossible or test-injected values.

- **charts.yaml**: Added 8 severity metric IDs (`snmp_trap_severity_emerg` through `snmp_trap_severity_debug`) to the flat metrics list. Added a new `severity` chart with `context: severity`, `units: events/s`, `type: stacked`, `by_labels: [job_name]`, and one dimension per severity. The resulting full chart context is `snmp.trap.severity`.

- **metadata.yaml**: Updated self-metrics overview to mention severity counters. Updated runtime step 6 to include severity. Updated metrics description from "three static metric contexts" to "four". Added `snmp.trap.severity` scope with all 8 dimensions and their descriptions (matching the severity taxonomy table in the overview section). Replaced `alerts: []` with entries for every stock `snmp_trap.conf` alert template.

- **pipeline_test.go**: Added 3 focused severity tests:
  1. `TestCollectorHandlePacketIncrementsSeverityMetric` — proves a successful trap write with severity `"warning"` increments `metrics.severities.warning` to 1 and does not touch other severity buckets.
  2. `TestPerJobMetricsIncSeverityFallsBackToNotice` — proves impossible or test-injected empty/unknown severity values still land in the bounded `notice` bucket.
  3. `TestCollectMetricsEmitsSeverityCounters` — proves the `collectSeverities` collector-store path emits all 8 `snmp_trap_severity_*` metric IDs, with expected non-zero values for `crit`, `warning`, and `info` and zero values for the other buckets.
  Updated `TestCollectorHandlePacketDedupSuppressesDuplicates`, `TestCollectorHandlePacketDedupPreservesHealthErrorCounters`, and `TestCollectorHandlePacketDedupRollsBackFingerprintAfterWriteFailure` to also verify severity counters alongside existing event/error/dedup checks.

- **src/health/health.d/snmp_trap.conf**: Created stock health alert templates:
  - Severity-rate alerts for `emerg`, `alert`, `crit`, `err`, and high-rate `warning`; `notice`, `info`, and `debug` do not alert by default.
  - One alert template for every `snmp.trap.errors` dimension (`unknown_oid`, `decode_failed`, `template_unresolved`, `malformed_pdu`, `dropped_allowlist`, `rate_limited`, `auth_failures`, `usm_failures`, `unknown_engine_id`, `inform_response_failed`, `sanitized`, `profile_load_failed`, `journal_write_failed`, `otlp_export_failed`).
  - `snmp_trap_high_dedup_suppression` on `snmp.trap.dedup_suppressed`.
  All alerts use `average` lookups on rate chart dimensions with specific dimension selectors. Thresholds are conservative to avoid alert noise from ordinary informational traps.

- **README.md + generated integration page**: Ran `python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps`. This created `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md` from `metadata.yaml` and the `README.md` symlink to it.

- **taxonomy.yaml**: Added `src/go/plugin/go.d/collector/snmp_traps/taxonomy.yaml` under `remote-devices.snmp` with a heads grid for events, severity, errors, and dedup suppression; explicit groups for trap events and pipeline health; and an operator-metrics selector for `snmp.trap.*` dynamic contexts. Adjusted `src/go/plugin/go.d/collector/snmp/taxonomy.yaml` to exclude `snmp.trap.` from the existing broad SNMP profile selector so the new trap contexts are not double-owned.

Validation executed:

- `gofmt -w metrics.go collector.go pipeline_test.go` — clean, no format issues.
- `python3 -c 'import yaml, pathlib; ...'` — `metadata.yaml` and `charts.yaml` parse cleanly after indentation repair.
- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` from `src/go/` — all tests pass (including the 3 new severity tests and the 2 updated dedup tests).
- `python3 integrations/gen_integrations.py` from repo root — passed after metadata alert-list and severity-context updates.
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps` from repo root — passed and generated the collector integration page plus README symlink.
- `python3 integrations/check_collector_taxonomy.py` from repo root — passed after excluding `snmp.trap.` from the broad SNMP selector and excluding stock contexts from the trap dynamic selector.
- `python3 integrations/gen_taxonomy.py` from repo root — passed.
- `timeout 120 ./build/netdata -W unittest` from repo root — full internal unit suite passed. This is broader than the SOW needs and created `/var/cache/netdata/unittest-dbengine`; do not treat it as a targeted health-only validation.
- `git diff --check` — no whitespace issues.

### 2026-05-28 — M2 external review round and cleanup

External review round:

- Reviewers: `glm`, `kimi`, `minimax`, and `qwen`.
- Scope: whole M1/M2 consistency batch, including this SOW file, severity counters, health alerts, `metadata.yaml`, generated README/integration doc, trap taxonomy, SNMP taxonomy exclusion, and tests.
- Result: all four reviewers returned `PRODUCTION GRADE`.

Reviewer findings handled:

- `kimi` found a low-severity pre-existing inefficiency in `perJobMetrics.addError`: the fallback path looped `n` times for non-OTLP dimensions even though the current production caller only uses `otlp_export_failed`. Fixed by making `incError` call `addError(dim, 1)` and making `addError` directly `atomic.AddUint64(..., n)` for every known error dimension. Unknown dimensions still no-op as before.
- `kimi` found low-severity test coverage gaps: severity tests checked the touched buckets but not every untouched bucket, and dedup error subtests did not assert severity counters. Fixed by adding `assertSeverityCounters()` and using it across severity, dedup, fallback, and collector-store tests.
- Alert sensitivity for `emerg`, auth/USM, unknown-engine-ID, and sanitized-field alerts was reviewed by all reviewers and accepted as conservative operator-facing behavior, not a code defect. No threshold change made.

Validation executed after cleanup:

- `gofmt -w src/go/plugin/go.d/collector/snmp_traps/metrics.go src/go/plugin/go.d/collector/snmp_traps/pipeline_test.go` — clean.
- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` from `src/go/` — passed.
- `python3 integrations/check_collector_taxonomy.py` from repo root — passed.
- `python3 integrations/gen_integrations.py` from repo root — passed.
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps` from repo root — passed.
- `python3 integrations/gen_taxonomy.py` from repo root — passed.
- `git diff --check` from repo root — passed.

Second-pass reviewer findings handled:

- `kimi` flagged the `unknown_engine_id` alert wording: successful dynamic SNMPv3 engine ID registration intentionally increments `unknown_engine_id` once per first accepted `(engineID, username)` pair per SOW-0038 and `.agents/sow/specs/snmp-traps/netdata.md`. The counter behavior is spec-compliant, but the M2 alert and generated docs described it only as a static-whitelist failure. Fixed the alert info text and `metadata.yaml` troubleshooting text to explain both modes:
  - static mode: sender engine ID is outside `engine_id_whitelist`;
  - dynamic mode: the first accepted pair is expected visibility, while repeated or rejected increments indicate cap exhaustion, invalid sender state, or an unauthorized sender.

Validation executed after the wording fix:

- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps` from repo root — passed and regenerated `integrations/snmp_trap_listener.md` plus README symlink target.
- `python3 integrations/gen_integrations.py` from repo root — passed.
- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` from `src/go/` — passed.
- `python3 integrations/check_collector_taxonomy.py` from repo root — passed.
- `python3 integrations/gen_taxonomy.py` from repo root — passed.
- `git diff --check` from repo root — passed.

Third-pass review after the wording fix:

- `glm`: `PRODUCTION GRADE`. It initially saw a stale generated-doc snippet, but current filesystem evidence after regeneration shows both `README.md` and `integrations/snmp_trap_listener.md` contain the corrected dual-mode `unknown_engine_id` wording and are identical.
- `minimax`: `PRODUCTION GRADE`. It also initially reported the same stale generated-doc wording, but no runtime, metric, taxonomy, or test issue.
- `kimi`: `PRODUCTION GRADE`. It independently reran `go test`, `go vet`, `check_collector_taxonomy.py`, `gen_integrations.py`, `gen_docs_integrations.py`, `gen_taxonomy.py`, JSON/YAML parses, and `git diff --check`; no issues found.
- `qwen`: final-pass harness failure. First attempt ran for the full `timeout 1800` window and exited `124` without a final review after initial reads. A compact retry repeated the same silent failure mode and was stopped by targeted termination of the specific retry PIDs. Earlier Qwen rounds before the wording-only doc fix returned `PRODUCTION GRADE`; the final wording change is covered by the other third-pass reviewers plus current file evidence.

Current generated-doc evidence:

- `src/go/plugin/go.d/collector/snmp_traps/README.md` is a symlink to `integrations/snmp_trap_listener.md`.
- Both the alert table and troubleshooting section now describe `unknown_engine_id` in static and dynamic discovery modes.
- `cmp -s src/go/plugin/go.d/collector/snmp_traps/README.md src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md` — passed.

### 2026-05-28 — M3 systemd-journal facet registration

Implemented default facet registration for SNMP trap journal fields:

- **`src/collectors/systemd-journal.plugin/systemd-journal.c:192-205`**: Added `/* --- NETDATA SNMP TRAPS --- */` block to `SYSTEMD_KEYS_INCLUDED_IN_FACETS`.
- Added exactly the 12 plugin-controlled static trap fields required by M3:
  - `TRAP_REPORT_TYPE`
  - `TRAP_OID`
  - `TRAP_NAME`
  - `TRAP_CATEGORY`
  - `TRAP_SEVERITY`
  - `TRAP_PDU_TYPE`
  - `TRAP_VERSION`
  - `TRAP_SOURCE_IP`
  - `TRAP_SOURCE_UDP_PEER`
  - `TRAP_DEVICE_VENDOR`
  - `TRAP_INTERFACE`
  - `TRAP_NEIGHBORS`
- Verified the already-existing standard/Netdata fields required by M3 remain present:
  - `_HOSTNAME` in `SYSTEMD_KEYS_INCLUDED_IN_FACETS`
  - `ND_LOG_SOURCE` in `SYSTEMD_KEYS_INCLUDED_IN_FACETS`
  - `ND_NIDL_NODE` in `SYSTEMD_KEYS_INCLUDED_IN_FACETS`
- Deliberately did **not** add dynamic labels or payload/summary fields as default facets:
  - no `TRAP_TAG_*`
  - no `TRAP_JSON`
  - no `TRAP_SUPPRESSED_COUNT`
  - no `TRAP_SUPPRESSED_FINGERPRINTS`
  - no `TRAP_REPORT_PERIOD_SEC`

Implementation evidence:

- `src/go/plugin/go.d/collector/snmp_traps/serialize.go:83-115` writes every registered field as a journal field.
- `src/go/plugin/go.d/collector/snmp_traps/serialize.go:123-139` writes `TRAP_JSON`, dedup summary fields, and `TRAP_TAG_*` labels separately; these remain outside the default facet macro.
- `src/collectors/systemd-journal.plugin/systemd-journal.c:1160-1161` passes `SYSTEMD_KEYS_INCLUDED_IN_FACETS` and `SYSTEMD_KEYS_EXCLUDED_FROM_FACETS` to `lqs_facets_create()`, so this code path controls default facet selection for the systemd-journal Logs query path.

Validation executed:

- Static macro check: all 12 required fields present; unwanted `TRAP_TAG_*`, `TRAP_JSON`, `TRAP_SUPPRESSED_COUNT`, `TRAP_SUPPRESSED_FINGERPRINTS`, and `TRAP_REPORT_PERIOD_SEC` absent — passed.
- `git diff --check -- src/collectors/systemd-journal.plugin/systemd-journal.c` — passed.
- `sudo ninja -C build systemd-journal.plugin` — passed. The first non-sudo attempt failed because the existing `build/` directory is root-owned and CMake needed to update its glob marker after the working tree changed. The successful compile emitted only existing const-qualifier warnings in nearby code paths.

External implementation/review:

- `deepseek/deepseek-v4-pro` implementation run: verified the existing M3 patch, made no edits, and confirmed all 12 required fields are present while excluded fields are absent.
- `glm`: `PRODUCTION GRADE`; confirmed exact field match against `serialize.go`, no excluded fields, `_HOSTNAME`/`ND_LOG_SOURCE`/`ND_NIDL_NODE` already present, and no macro/exclude interaction issue.
- `kimi`: `PRODUCTION GRADE`; confirmed byte-for-byte field-name alignment, exact-match facet pattern behavior, and the Logs UI picker semantics. It noted that `TRAP_JSON` remains operator-selectable because it is not in `SYSTEMD_KEYS_EXCLUDED_FROM_FACETS`. No change made: current SOW and spec intentionally define `SYSTEMD_KEYS_INCLUDED_IN_FACETS` as the default selection, not a whitelist.
- `minimax`: `PRODUCTION GRADE`; confirmed registered fields match `serialize.go` and spec, excluded fields are absent, and default facet registration is static and isolated.
- `qwen`: `PRODUCTION GRADE`; confirmed the one-consumer macro path, expected conditional field presence, excluded fields, and no regression to journal write, health alert, taxonomy, or OTLP code paths.

### 2026-05-28 — M4 public `query-snmp-traps` skill

Implementation:

- Added `docs/netdata-ai/skills/query-snmp-traps/SKILL.md`.
- Added `docs/netdata-ai/skills/query-snmp-traps/how-tos/INDEX.md`.
- Added seeded operator how-tos:
  - `recent-security-traps-from-device.md`
  - `filter-by-severity-across-fleet.md`
  - `top-trap-senders-last-hour.md`
  - `inspect-dedup-summary-entries.md`
  - `search-varbind-value-in-trap-json.md`
- Added `.agents/skills/query-snmp-traps` symlink to `../../docs/netdata-ai/skills/query-snmp-traps`.
- Updated `AGENTS.md` public-skill index with the new skill trigger, symlink, and live status.

Implementation evidence:

- The skill documents the SNMP trap journal fields emitted by `src/go/plugin/go.d/collector/snmp_traps/serialize.go`, including `MESSAGE`, `TRAP_PDU_TYPE`, and `TRAP_VERSION`.
- `TRAP_REPORT_TYPE=decode_error_summary` is documented as reserved-future only. Current code defines the report type but does not write decode-summary journal entries.
- The fleet severity how-to now deletes stale `severity-*.json` files before a run, continues when one reachable node fails, avoids `jq -s` hanging on an empty glob, and includes a local-only node row-count listing.

Validation executed:

- Relative Markdown link resolver over `docs/netdata-ai/skills/query-snmp-traps/` — passed.
- Extracted `bash` code blocks from the new skill/how-tos and ran `bash -n` — passed.
- Sensitive-data grep for raw bearer/API-token examples, SNMP secrets, public IP literals, and personal names in the new skill — passed.
- `readlink .agents/skills/query-snmp-traps` and `readlink -f .agents/skills/query-snmp-traps` — passed.
- Relative Markdown link resolver over `docs/netdata-ai/skills/query-snmp-traps/` after reviewer fixes — passed.
- Extracted all 25 indented and non-indented `bash` code blocks from the new skill/how-tos and ran `bash -n` after reviewer fixes — passed.
- Sensitive-data grep over the new skill, `AGENTS.md`, and this SOW after reviewer fixes — passed.
- `readlink .agents/skills/query-snmp-traps` and `readlink -f .agents/skills/query-snmp-traps` after reviewer fixes — passed.
- `git diff --check -- AGENTS.md docs/netdata-ai/skills/query-snmp-traps .agents/skills/query-snmp-traps .agents/sow/current/SOW-0039-20260525-snmp-traps-bundle-facets-docs-merge-gate.md` after reviewer fixes — passed.

External implementation/review:

- `deepseek/deepseek-v4-pro` implementation run: added missing `MESSAGE`, `TRAP_PDU_TYPE`, and `TRAP_VERSION` field-table coverage, improved Cloud room node extraction to `.[] | select(.state=="reachable") | .nd`, and added reachable-node guidance. A draft wording change that implied `decode_error_summary` is emitted today was corrected to reserved-future wording based on current code evidence.
- `glm`: `PRODUCTION GRADE`; verified journal field table, row decoding, facet aggregation, symlink, links, token safety, and public/operator boundary. No required change.
- `minimax`: `PRODUCTION GRADE`; verified field/reference accuracy, default facet alignment, row decoding, how-to structure, privacy guidance, links, and bash syntax. No required change.
- `qwen`: `NEEDS WORK` before fixes; required adding the new `AGENTS.md` public-skill index entry. Also flagged stale severity glob cleanup and empty-result handling. Fixed in this M4 batch.
- `kimi`: `NEEDS WORK` before fixes; required making the fleet loop continue on per-node Function call failures, avoiding `jq -s` reading stdin when no severity files exist, and adding an actual node row-count listing because the how-to question asks which nodes matched. Fixed in this M4 batch.

Final review after fixes:

- `glm`: `PRODUCTION GRADE`; confirmed exact source-field alignment, wrapper usage, Cloud room-node path, `set -euo pipefail` behavior, severity taxonomy, symlink, links, `AGENTS.md` entry, and SOW consistency. No findings.
- `kimi`: `PRODUCTION GRADE`; confirmed all prior findings fixed. It noted that `shopt -s nullglob` remains active in the operator shell and that the row-count arithmetic assumes `jq length` returns a number; both were classified as acceptable because the how-to uses local shell snippets and `jq 'length'` returns a non-negative integer for the guarded array expression.
- `minimax`: `PRODUCTION GRADE`; confirmed all prior findings fixed, all bash snippets syntax-check, token-safety self-test passes in the reused wrapper library, and no audience-boundary or sensitive-data issue remains.
- `qwen`: `PRODUCTION GRADE`; confirmed all prior findings fixed. It suggested optional documentation polish for `PRIORITY` and the `last`/facet-scan distinction and raised a `tonumber? // 0` maintainability concern while also confirming the current jq works with the journal writer. No change made in this batch because no functional or operator-safety issue was demonstrated.

### 2026-05-28 — Severity source clarification

The user clarified that no new severity product decision is needed because loaded trap profiles already carry severity.

Evidence:

- `.agents/sow/specs/snmp-traps/netdata.md` already defines the trap profile entry as carrying required `severity`, applies the profile entry during OID resolution, and requires counters by category and severity.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` defines `severity` as a required trap-entry field and maps the closed 8-severity set to journal `PRIORITY`.
- `src/go/plugin/go.d/collector/snmp_traps/pipeline.go` sets the event severity from the resolved profile entry.
- `src/go/plugin/go.d/collector/snmp_traps/collector.go` applies configured per-OID overrides before serializing and counting the event.

Decision recorded:

- Runtime severity counters, health alerts, journal `TRAP_SEVERITY`, and OTLP severity continue to use the effective resolved trap severity: profile severity first, then operator override if configured, then unknown-OID fallback to `notice`.
- No separate severity inference layer is added in this SOW.

### 2026-05-28 — M5 user documentation: offline MIB-to-YAML conversion workflow

Implementation:

- Added the custom-MIB conversion guide to `docs/netdata-ai/skills/query-snmp-traps/SKILL.md` and `docs/netdata-ai/skills/query-snmp-traps/how-tos/INDEX.md`.
- Added `docs/netdata-ai/skills/query-snmp-traps/how-tos/convert-custom-mibs-to-trap-profiles.md`.
- Updated `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml` and regenerated `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md` so the generated README content documents the installed helper path.
- Updated `tools/snmp-traps-profile-gen/README.md` to direct operators to the installed Go helper first and keep the legacy Python pipeline as source-tree/reference material.

Behavior documented:

- The supported operator path is `/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen generate`, not the legacy Python pipeline.
- Operators should test one MIB module first, inspect `profiles/catalogue.json` and generated YAML, then run `--all` if needed.
- Generated YAML is installed under `/etc/netdata/go.d/snmp.trap-profiles/`.
- Active jobs reload profiles through the registered Function `snmp_traps:reload-profiles`; if no trap job is active, the operator re-applies the DynCfg job or restarts the Netdata Agent so job creation validates and loads the files.
- Verification is through the existing `systemd-journal` Function, filtering `ND_LOG_SOURCE=snmp-trap` and checking that trap rows now expose `TRAP_NAME`, `TRAP_CATEGORY`, and `TRAP_SEVERITY`.

Worked example evidence:

- Open-source source: `nagios-plugins/nagios-mib @ 725c67605058094bedc5f1ddd8c2a911e263093a`, `MIB/NAGIOS-NOTIFY-MIB:576-615`.
- The checked `NAGIOS-NOTIFY-MIB` contains four NOTIFICATION-TYPE definitions: `nHostEvent`, `nHostNotify`, `nSvcEvent`, and `nSvcNotify`.
- Local validation with the Go helper emitted `profiles/nagios.yaml` containing four traps and 25 varbinds, plus `profiles/catalogue.json`, `traps.jsonl`, and `extraction-report.json`.

Validation executed:

- Relative Markdown link resolver over the public skill, its how-tos, the tool README, and the generated integration doc — passed (`10` files).
- Extracted `bash`/`sh` code blocks from the same files with a line-based fenced-code parser and ran `bash -n` — passed (`49` shell blocks).
- YAML parse for `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml` and `src/go/plugin/go.d/collector/snmp_traps/taxonomy.yaml` — passed.
- `python3 integrations/gen_integrations.py` then `python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps` — passed; generation order matters because the docs generator reads the generated integration JSON.
- `python3 integrations/check_collector_taxonomy.py` — passed.
- `git diff --check -- docs/netdata-ai/skills/query-snmp-traps tools/snmp-traps-profile-gen/README.md src/go/plugin/go.d/collector/snmp_traps/metadata.yaml src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md .agents/sow/current/SOW-0039-20260525-snmp-traps-bundle-facets-docs-merge-gate.md` — passed.
- Stale reload-command scan confirmed the invalid `{"method":"reload-profiles"}` Function body pattern is absent. Current docs use `--function snmp_traps:reload-profiles`.

External review:

- `glm`: `PRODUCTION GRADE`; verified the installed Go helper path, legacy Python wording, Function registration path (`snmp_traps:reload-profiles` via the module/method framework), severity source chain, generated-doc consistency, links, token safety, and audience boundary.
- `kimi`: `PRODUCTION GRADE`; verified the installed helper, reload Function name and empty-body execution, profile/override/notice severity chain, generated README symlink consistency, shell snippets, and secret-safe examples. It noted the `{"info":true}` discovery call is redundant for a no-param method but consistent with the generic Function workflow, so no change was needed.
- `minimax`: `PRODUCTION GRADE`; verified helper path, legacy pipeline positioning, reload Function name, no stale method-body pattern, severity chain, generated documentation consistency, link paths, and no sensitive data leaks.
- `qwen`: `PRODUCTION GRADE`; verified helper path, reload Function protocol, severity chain, generated-doc consistency, token-safe wrapper usage, shell snippets, and absence of audience-boundary violations.

### 2026-05-28 — M6 SOW-0032 closeout, SDK v0.3.0 alignment, and full packet-to-journal benchmark

Implementation:

- Added `.agents/sow/specs/snmp-traps/comparison/comparative-analysis.md`, synthesizing the shipped Netdata behavior from SOW-0035 through SOW-0039 against the 16-system Phase A cohort.
- Added `.agents/sow/specs/snmp-traps/comparison/comparison-matrix.md` as a compatibility pointer because the actual final matrix is `.agents/sow/specs/snmp-traps/comparison/feature-matrix.md`.
- Updated SOW-0032 to `Status: completed` and moved it from `.agents/sow/current/` to `.agents/sow/done/`.
- Bumped the committed Go dependency to `github.com/netdata/systemd-journal-sdk/go v0.3.0` in `src/go/go.mod` and `src/go/go.sum`.
- Added committed `BenchmarkFullPacketToJournal` to `src/go/plugin/go.d/collector/snmp_traps/benchmark_test.go`.
- Updated `.agents/sow/specs/snmp-traps/netdata.md`, ADR-0001, and SOW-0035 to record SDK `go/v0.3.0` as the then-current integration version.

Current OOB profile catalogue evidence:

- `jq 'keys | length' src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json` -> 437 profile files.
- `jq '[.[] .mib_count] | add' src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json` -> 3131 MIB modules.
- `jq '[.[] .trap_count] | add' src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json` -> 71787 trap definitions.
- `jq '[.[] .varbind_count] | add' src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json` -> 44462 varbind definitions.

Validation executed:

- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` after SDK bump — passed.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=30000x -count=3 -timeout 120s`
  - 30,000 packets/run: 16.97-18.54 us/op, 53.9K-58.9K packets/sec, 53.9K-58.9K persisted entries/sec, 1-5 dropped packets/run, 12,624-12,625 B/op, 206 allocs/op.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=100000x -count=3 -timeout 180s`
  - 100,000 packets/run: 16.18-16.55 us/op, 60.4K-61.8K packets/sec, 60.4K-61.8K persisted entries/sec, 1 dropped packet/run, 12,580-12,581 B/op, 206 allocs/op.

### 2026-05-31 — SDK v0.4.0 alignment

Implementation:

- Bumped the committed Go dependency to `github.com/netdata/systemd-journal-sdk/go v0.4.0` in `src/go/go.mod` and `src/go/go.sum`.
- Checked the SDK tag list (`go list -m -versions github.com/netdata/systemd-journal-sdk/go`) and confirmed `v0.4.0` is published.
- Scanned the local SDK tag diff from `go/v0.3.0` to `go/v0.4.0`; writer/reader internals changed, but the adapter's used writer API surface remained source-compatible.
- Updated `.agents/sow/specs/snmp-traps/netdata.md`, ADR-0001, the comparative analysis, and SOW-0035 to record SDK `go/v0.4.0` as the current integration version.

Validation executed:

- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` — passed.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=30000x -count=3 -timeout 120s`
  - Final repeated 30,000-packet runs: 20.18-24.41 us/op, 41.0K-49.5K packets/sec, 41.0K-49.5K persisted entries/sec, 0-6 dropped packets/run, 11,587-11,590 B/op, 191-192 allocs/op.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^Benchmark(JournalTrapWriterDrain|JournalWriterWriteEntry)$' -benchmem -benchtime=30000x -count=3 -timeout 120s`
  - Queued `JournalTrapWriterDrain`: 22.89-25.85 us/op, 38.7K-43.7K entries/sec, 3,242 B/op, 42 allocs/op.
  - Direct `JournalWriterWriteEntry`: 5.25-5.62 us/op, 178K-191K entries/sec, 825 B/op, 5 allocs/op.
- `go mod tidy -diff` — clean after applying the tidy module graph.
- `git diff --check` — clean.
- `./.agents/sow/audit.sh` — exit 0; existing non-project skill classification warnings remain unrelated to this SDK bump.

### 2026-06-01 — SDK v0.4.0 benchmark repeat

Validation executed:

- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=30000x -count=3 -timeout 120s`
  - Pass 1: 26.29-32.75 us/op, 30.5K-38.0K persisted entries/sec, 0-2 dropped packets/run, 11,588-11,590 B/op, 192 allocs/op.
  - Pass 2: 26.77-28.64 us/op, 34.9K-37.4K persisted entries/sec, 1-3 dropped packets/run, 11,588-11,589 B/op, 192 allocs/op.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^Benchmark(JournalTrapWriterDrain|JournalWriterWriteEntry)$' -benchmem -benchtime=30000x -count=3 -timeout 120s`
  - Queued `JournalTrapWriterDrain`: 15.48-19.20 us/op, 52.1K-64.6K entries/sec, 3,242 B/op, 42 allocs/op.
  - Direct `JournalWriterWriteEntry`: 5.58-6.81 us/op, 147K-179K entries/sec, 825 B/op, 5 allocs/op.

Throughput gate decision:

- The early SDK v0.4.0 repeat exposed a local Netdata hot-path bottleneck and led to SOW-0045.
- After SOW-0045, the committed branch measures about 62K-73K persisted traps/sec for the synthetic v2c profile-hit packet-to-journal path on the workstation for 30,000-packet runs, and about 63K-66K persisted traps/sec for 100,000-packet runs.
- This satisfies the first-release merge gate as local measured evidence, not as a portable hardware guarantee. The remaining gap to direct SDK append is mostly SDK append/live publication plus packet decode/varbind conversion, not the previous per-entry journal-field/JSON allocation pattern.

### 2026-06-01 — SOW-0045 local hot-path optimization

Validation executed:

- `go test ./plugin/go.d/collector/snmp_traps -run '^TestJournalHotSerializerMatchesSerializeToJournalFields$|^TestSerializeToJournalFields' -count=1 -timeout 120s` — passed.
- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` — passed.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^Benchmark(TrapWriterWrite|JournalTrapWriterDrain|JournalWriterWriteEntry|FullPacketToJournal)$' -benchmem -benchtime=30000x -count=3 -timeout 120s`
  - Queued `JournalTrapWriterDrain`: 13.47-16.15 us/op, 61.9K-74.2K entries/sec, 577 B/op, 7 allocs/op.
  - Direct `JournalWriterWriteEntry`: 5.45-6.01 us/op, 166K-184K entries/sec, 825 B/op, 5 allocs/op.
  - Full packet-to-journal: 13.78-16.01 us/op, 62.5K-72.6K persisted entries/sec, 0 drops, 5,202 B/op, 128 allocs/op.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=100000x -count=3 -timeout 180s`
  - 100,000 packets/run: 15.16-15.81 us/op, 63.3K-66.0K persisted entries/sec, 0-1 drops/run, 5,200-5,201 B/op, 128 allocs/op.

Installed-Agent validation:

- The local installed `go.d.plugin` binary initially did not match `build/go.d.plugin`, did not have `cap_net_bind_service`, and was not bound to UDP/162 even though `/etc/netdata/go.d/snmp_traps.conf` configured a `local` trap job on `0.0.0.0:162`.
- Rebuilt and installed the current branch artifacts with:
  - `sudo ninja -C build go.d.plugin systemd-journal.plugin`
  - `sudo install -o root -g [PLUGIN_GROUP] -m 0750 build/go.d.plugin /usr/libexec/netdata/plugins.d/go.d.plugin`
  - `sudo install -o root -g [PLUGIN_GROUP] -m 0750 build/systemd-journal.plugin /usr/libexec/netdata/plugins.d/systemd-journal.plugin`
  - `sudo setcap 'cap_dac_read_search+epi cap_net_admin=eip cap_net_raw=eip cap_net_bind_service=eip' /usr/libexec/netdata/plugins.d/go.d.plugin`
  - `sudo setcap 'cap_dac_read_search=eip' /usr/libexec/netdata/plugins.d/systemd-journal.plugin`
  - `sudo systemctl restart netdata`
- Post-install evidence:
  - `sha256sum /usr/libexec/netdata/plugins.d/go.d.plugin build/go.d.plugin` -> both `6136a4ed22e739647ef81bf6a2e872618453ff8f65ce6230aae5e13ea12d00f1`.
  - `getcap /usr/libexec/netdata/plugins.d/go.d.plugin` -> `cap_dac_read_search,cap_net_bind_service,cap_net_admin,cap_net_raw=eip`.
  - `sudo ss -lunp | rg ':(162|9162)\b|go\.d\.plugin'` -> `go.d.plugin` listening on UDP/162.
  - Netdata log line: `collector=snmp_traps job=local` reported `check success` and `started (v2), data collection interval 1s`.
- Sent one synthetic loopback SNMPv2c trap with `snmptrap -v 2c -c public 127.0.0.1:162 '' 1.3.6.1.6.3.1.1.5.1`.
- `journalctl --directory=/var/cache/netdata/traps/local/929c259028424a2da82cc8cd09e0bca1 --since '2026-05-28 21:54:18' -o json` returned one `ND_LOG_SOURCE=snmp-trap` entry with:
  - `MESSAGE="Device reinitializing and configuration may have been altered on 127.0.0.1."`
  - `TRAP_VERSION=v2c`
  - `TRAP_PDU_TYPE=trap`
  - `TRAP_OID=1.3.6.1.6.3.1.1.5.1`
  - `TRAP_NAME=SNMPv2-MIB::coldStart`
  - `TRAP_CATEGORY=state_change`
  - `TRAP_SEVERITY=notice`
  - `PRIORITY=5`
  - `TRAP_REPORT_TYPE=trap`
  - `TRAP_JSON` containing the expected `sysUpTime.0` and `snmpTrapOID.0` varbinds.
- Agent chart evidence:
  - `/api/v1/charts` exposes `snmp_traps_local.events_local`, `snmp_traps_local.severity_local`, and `snmp_traps_local.errors_local`.
  - `/api/v1/data?chart=snmp_traps_local.events_local&after=-120&points=5&format=json` showed a non-zero `state_change` rate for the validation bucket.
  - `/api/v1/data?chart=snmp_traps_local.severity_local&after=-120&points=5&format=json` showed a non-zero `notice` rate for the validation bucket.
  - `/api/v1/data?chart=snmp_traps_local.errors_local&after=-120&points=5&format=json` showed zeroes for all error dimensions in the validation bucket.
- Logs UI / Function limitation:
  - `/api/v1/functions` lists `systemd-journal` with access `signed-in`, `same-space`, and `sensitive-data`.
  - Direct unauthenticated calls to `/api/v3/function?function=systemd-journal` and `/api/v1/function?function=systemd-journal...` returned HTTP 412: signed-in Cloud SSO or an agent token is required.
  - The local dashboard Logs tab displayed the same authorization gate. Therefore the SOW can claim installed-Agent trap receive, journal persistence, metric counters, and static facet-code registration, but cannot honestly claim a signed-in Logs UI visual facet check until a token or signed-in session is available.

## Validation

Acceptance criteria evidence:

- M1/M2 collector consistency bundle implemented: `metadata.yaml`, `config_schema.json`, stock config, health alerts, README/generated integration doc, taxonomy, severity counters, and generated docs are present.
- M3 default systemd-journal trap facets implemented in `src/collectors/systemd-journal.plugin/systemd-journal.c`; installed-Agent trap journal entries now contain the same plugin-controlled `TRAP_*` fields. A signed-in Logs UI visual facet check is blocked by local Function authorization and is not claimed.
- M4 public skill implemented at `docs/netdata-ai/skills/query-snmp-traps/` with `.agents/skills/query-snmp-traps` symlink and seeded how-tos.
- M5 installed-helper custom-MIB workflow documented in generated collector docs, public skill how-to, and `tools/snmp-traps-profile-gen/README.md`.
- M6 comparative closeout implemented: SOW-0032 is completed and moved to `.agents/sow/done/`; `comparison/comparative-analysis.md` and `comparison/comparison-matrix.md` exist.
- SDK dependency truth corrected: `src/go/go.mod` pins `github.com/netdata/systemd-journal-sdk/go v0.4.0`.
- Full packet-to-journal benchmark is committed and rerun against the committed dependency.

Tests or equivalent validation:

- `go vet ./plugin/go.d/collector/snmp_traps/...` — passed.
- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` — passed after SDK v0.3.0 bump and committed benchmark addition.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=30000x -count=3 -timeout 120s` — passed; 53.9K-58.9K persisted entries/sec.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=100000x -count=3 -timeout 180s` — passed; 60.4K-61.8K persisted entries/sec.
- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` — passed after SDK v0.4.0 bump.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=30000x -count=3 -timeout 120s` — passed after SDK v0.4.0 bump and final repeated validation; 41.0K-49.5K persisted entries/sec.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^Benchmark(JournalTrapWriterDrain|JournalWriterWriteEntry)$' -benchmem -benchtime=30000x -count=3 -timeout 120s` — passed after SDK v0.4.0 bump; queued writer 38.7K-43.7K entries/sec, direct writer 178K-191K entries/sec.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=30000x -count=3 -timeout 120s` — passed on 2026-06-01 repeat; 30.5K-38.0K persisted entries/sec in pass 1 and 34.9K-37.4K in pass 2.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^Benchmark(JournalTrapWriterDrain|JournalWriterWriteEntry)$' -benchmem -benchtime=30000x -count=3 -timeout 120s` — passed on 2026-06-01 repeat; queued writer 52.1K-64.6K entries/sec, direct writer 147K-179K entries/sec.
- `go test ./plugin/go.d/collector/snmp_traps -run '^TestJournalHotSerializerMatchesSerializeToJournalFields$|^TestSerializeToJournalFields' -count=1 -timeout 120s` — passed after SOW-0045.
- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` — passed after SOW-0045.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^Benchmark(TrapWriterWrite|JournalTrapWriterDrain|JournalWriterWriteEntry|FullPacketToJournal)$' -benchmem -benchtime=30000x -count=3 -timeout 120s` — passed after SOW-0045; full packet-to-journal 62.5K-72.6K persisted entries/sec, queued writer 61.9K-74.2K entries/sec.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=100000x -count=3 -timeout 180s` — passed after SOW-0045; 63.3K-66.0K persisted entries/sec.
- `go mod tidy -diff` — clean after applying the tidy module graph.
- `git diff --check` — clean.
- `./.agents/sow/audit.sh` — exit 0; existing non-project skill classification warnings remain unrelated to this SDK bump.
- `python3 integrations/gen_integrations.py` — passed.
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps` — passed.
- `python3 integrations/check_collector_taxonomy.py` — passed.
- `python3 integrations/gen_taxonomy.py` — passed.
- Markdown link validation over the new comparison docs, public skill docs, tool README, and generated integration doc — passed (`12` files).
- Shell syntax extraction over the public skill docs, tool README, and generated integration doc — passed (`49` shell blocks).
- `git diff --check` — passed.
- `.agents/sow/audit.sh` — sensitive-data scan passed after wording repair. The audit still reports pre-existing non-project skill classification warnings unrelated to this SOW.

Real-use evidence:

- `BenchmarkFullPacketToJournal` verifies the synthetic packet-to-journal path and confirms `journalctl --directory` sees queryable rows after the timed section.
- Installed-Agent validation on the local node received a synthetic SNMPv2c `coldStart` trap on UDP/162, persisted the trap to `/var/cache/netdata/traps/local/.../*.journal`, exposed it through `journalctl --directory`, and updated the `snmp.trap.events`, `snmp.trap.severity`, and `snmp.trap.errors` charts as expected.
- Direct Agent Function and local Logs UI validation are currently blocked by the Agent's signed-in/sensitive-data authorization gate (HTTP 412 without a Cloud SSO session or agent token). Do not claim signed-in Logs UI facet validation complete until this is checked with an authorized session.

Reviewer findings:

- M2 reviewed in multiple rounds. `glm`, `kimi`, and `minimax` returned final `PRODUCTION GRADE` after the last wording fix; `qwen` returned `PRODUCTION GRADE` in earlier rounds but timed out on the final wording-only pass.
- M3 reviewed by `glm`, `kimi`, `minimax`, and `qwen`; all returned `PRODUCTION GRADE`. DeepSeek reviewed the M3 implementation with write access available and made no edits.
- M4 first review returned `PRODUCTION GRADE` from `glm` and `minimax`; `qwen` and `kimi` found actionable documentation/how-to robustness issues that were fixed. Final M4 review after fixes returned `PRODUCTION GRADE` from `glm`, `kimi`, `minimax`, and `qwen`.
- M5 review returned `PRODUCTION GRADE` from `glm`, `kimi`, `minimax`, and `qwen`.
- M6 final-batch review after SDK v0.3.0, installed-Agent validation, and SOW update:
  - `kimi`: `PRODUCTION GRADE`. Verified all validation commands, accepted the signed-in Logs UI limitation as correctly documented, and found no code/docs/security/test/performance blocker. It noted SOW-0035/0036/0037 remain in `current/paused` by design until the merge commit closes SOW-0035 through SOW-0039 together.
  - `qwen`: `PRODUCTION GRADE`. Verified alert/template consistency, config-schema/metadata consistency, taxonomy, generated docs, tests, and installed-Agent evidence. It treated the remaining SOW lifecycle moves and this final review recording as merge-commit actions, not implementation blockers.
  - `glm`: `NOT PRODUCTION GRADE`, lifecycle-only. It found M1-M6 implementation, tests, docs, generated artifacts, installed-Agent validation, and sensitive-data handling acceptable, but blocked on SOW-0035/0036/0037/0039 still not being marked `completed` and moved to `done/`, and on this SOW still saying M6 review was pending before this paragraph was added.
  - `minimax`: `NOT PRODUCTION GRADE`, lifecycle-only. It found the implementation production-grade and blocked only on SOW-0035/0036/0037 still being `current/paused`, SOW-0039 still `in-progress`, and `Outcome`/`Lessons Extracted` still pending.

Same-failure scan:

- Stale SDK-version scan found old current-version references in `netdata.md`, ADR-0001, SOW-0035, and comparative docs; these were updated to `go/v0.4.0` where they describe the current integration. Historical `go/v0.1.0` and `go/v0.3.0` evidence remains explicitly labeled as historical.
- Stale invalid reload-command scan found no `{"method":"reload-profiles"}` docs; current docs use `snmp_traps:reload-profiles`.
- SOW lifecycle audit found SOW-0032 stale in `current/`; it is now completed and moved to `done/`.
- Profile catalogue count was recomputed from committed `catalogue.json`; comparative docs use the current 437 profile files / 3,131 MIB modules / 71,787 traps / 44,462 varbinds figures.

Sensitive data gate:

- SOW/user docs use placeholders and public open-source MIB evidence only.
- No raw SNMP communities, USM secrets, API tokens, customer names, customer identifiers, live device hostnames, private endpoints, or live public IPs were added.
- `.agents/sow/audit.sh` sensitive-data guardrail now reports no sensitive-data patterns in durable artifacts.

Artifact maintenance gate:

- `AGENTS.md`: updated with the new public `query-snmp-traps` skill index entry.
- Runtime project skills: no reusable developer workflow change beyond existing project-writing and SNMP trap profile authoring skills.
- Specs: `netdata.md`, ADR-0001, SOW-0032 closeout, comparative analysis, and comparison pointer updated.
- End-user/operator docs: collector generated docs/README and `tools/snmp-traps-profile-gen/README.md` updated for trap listener and custom MIB workflow.
- End-user/operator skills: new `docs/netdata-ai/skills/query-snmp-traps/` plus `.agents/skills/query-snmp-traps` symlink.
- SOW lifecycle: SOW-0039 moved from pending to current; SOW-0032 moved current to done; SOW-0035 remains current/paused until final installed-path validation and final close.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

Tracked items to schedule after SOW-0039 closes:

- **Align NetFlow plugin retention default with the trap plugin's intentional deviation** (spec §11): NetFlow currently defaults `duration_of_journal_files` to `7d`; the trap plugin uses `null` (size-only eviction) because trap forensic data should not age out by time alone. Create a follow-up SOW to update NetFlow's `default_retention_duration_of_journal_files()` in `src/crates/netflow-plugin/src/plugin_config/defaults.rs` to `null` so the two plugins agree on the size-only default. Coordinate with NetFlow maintainers.
- **`display_hint` rendering for varbinds**: the field is documented as reserved-future in `profile-format.md`. When the plugin renderer is taught to honor `display_hint` (MAC, IPv4, custom formatters), the extractor must learn to pull `DISPLAY-HINT` from TEXTUAL-CONVENTION definitions in the same regeneration cycle. Track as a post-0039 enhancement.
- **`decode_error_summary` report type**: `TRAP_REPORT_TYPE` enum reserves this slot per §11 but no SOW in the 5-SOW lineup implements it. When decode-error rates become operator-visible enough to warrant batched summary entries (analogous to the dedup summary), open a follow-up SOW.
- **`dimension_from_varbind` cardinality table**: SOW-0037 M4 rejects unbounded-cardinality varbinds at config load. The reference cardinality table (which varbind types are bounded vs unbounded) needs an authoritative source — likely embedded in the profile schema or shipped as a separate `varbind-cardinality.yaml`. Open a follow-up SOW.

## Regression Log

None yet.
