# SOW-0032 - SNMP Trap Support: Comparative Analysis of Monitoring Solutions

## Status

Status: in-progress

Sub-state: scope locked; template + reviewer protocol pending user confirmation; per-system analyses not yet started

## Requirements

### Purpose

Inform Netdata's design for SNMP trap support by producing a complete, evidence-based analysis of how every relevant monitoring solution implements SNMP traps end-to-end. The deliverable must let the user (and the team) take **informed** decisions about Netdata's trap architecture rather than guesses. Wrong assumptions about prior art at this stage produce permanent technical debt in Netdata's trap subsystem (UDP binding, MIB strategy, schema, dedup, alerting integration, UX).

### User Request

> Check mirrored repos for other monitoring solutions supporting SNMP traps and analyse, in extreme detail, exactly what features they support, how they implement them (summary), what tests/fixtures they have, what configurability the user has, how each feature is presented to users, what databases they maintain to support each feature. The goal is to identify end-user value due to the implementation, implementation architecture, flexibility for users to customize, and what is provided out of the box. 100% feature and implementation coverage per monitoring solution.

Output: one comprehensive file per system in `.agents/sow/specs/snmp-traps/`, then a full cross-system comparison, then a Netdata-design discussion.

### Assistant Understanding

Facts:

- Foundational spec exists: `.agents/sow/specs/snmp-traps/snmp-traps-in-observability.md` (1039 lines, validated synthesis from 4 advisors + web validation).
- Local source mirrors contain source for 13 OSS systems with first-class trap implementations (survey complete; see §Analysis).
- Three commercial systems (SolarWinds, Dynatrace, LogicMonitor) lack source mirroring but are influential enough to warrant docs-only research.
- User's coding style mandates: research first, no guessing, evidence-based recommendations, mirror existing patterns, brutal honesty.

Inferences:

- Some systems share base implementations (multiple Nagios-family use SNMPTT; Centreon also leans on snmptrapd; Zabbix and LibreNMS use snmptrapd as front-end). The analysis must distinguish "their unique contribution" from "what they inherit from `snmptrapd`."
- Sub-agent parallelism would compress wall time but the user has explicitly chosen sequential execution with per-system parallel external review, because uniform quality and verifiable evidence matter more than speed.

Unknowns:

- Whether some commercial systems publish enough detail (in public docs) for a faithful analysis — to be discovered during the per-system pass and called out explicitly when evidence is thin.

### Acceptance Criteria

- One per-system spec file exists for every system in the locked list (§Locked Scope), each following the Common Template (§Common Template), each verified by all six external reviewers, with reviewer findings addressed or explicitly recorded as accepted/rejected.
- Source-cited file:line evidence for every architectural claim in every per-system file. No unverifiable assertions.
- For systems with mirrored source: every claim about implementation traces to source code or repo tests/docs.
- For docs-only systems: every claim cites a vendor URL or official document, and the file labels the section as "Vendor-documented, not source-verified."
- An incrementally maintained `comparison/comparison-matrix.md` is updated after each per-system file is finalized.
- A final `comparison/comparative-analysis.md` document synthesizes the matrix into design lessons for Netdata.
- A design discussion document `comparison/netdata-design-implications.md` captures the conclusions about Netdata's trap support based on the analysis.

## Analysis

Sources checked:

- `.agents/sow/specs/snmp-traps/snmp-traps-in-observability.md` (full read)
- `/opt/baddisk/monitoring/scripts/find-repos.sh 'snmp'` and `'trap'`
- Local source-mirror directory scans for trap-related directories and files.
- Local source-mirror content scans for `snmptrap` implementation files.
- Cross-checked Icinga (no native trap support — uses snmptt or external integration), Prometheus (pull-only, no traps), OpenTelemetry (no native trap receiver in core), VictoriaMetrics/Grafana (no traps), Sumologic/Logzio/Observiq (no traps), Coralogix/Chronosphere/Edgedelta (no traps), Honeycomb (no), Mezmo (no).

Current state of mirrored evidence:

| Tier | System | Source mirror evidence |
|---|---|---|
| 1 | OpenNMS | `opennms/features/events/traps/`, `opennms-alarms/snmptrap-northbounder/`, `ui/src/components/TrapdConfiguration/`, `udpgen/trap_generator.cpp`, `TempNewMIBs/events/*-TRAP*.xml` |
| 1 | Zabbix | `zabbix/src/libs/zbxsnmptrapper/`, `zabbix/include/zbxsnmptrapper.h`, `zabbix/misc/snmptrap/zabbix_trap_receiver.pl`, `zabbix-docker/Dockerfiles/snmptraps/` |
| 1 | LibreNMS | `librenms/LibreNMS/Snmptrap/`, `librenms/snmptrap.php`, `librenms/config/snmptraps.php`, `librenms/tests/Feature/SnmpTraps/`, `librenms/LibreNMS/Interfaces/SnmptrapHandler.php` |
| 1 | Centreon | `centreon/centreon/centreon/bin/centreontrapd`, `centreon-collect/perl-libs/lib/centreon/trapd/`, `centreon/centreon/bin/centFillTrapDB`, `centreon/centreon/bin/centreon_trap_send`, `.github/docker/centreon-centreontrapd/` |
| 1 | Zenoss | `zenoss-prodbin/src/Products/ZenEvents/zentrap/`, `zenoss-prodbin/bin/zentrap`, `pynetsnmp/example/trapd.c`, `pynetsnmp/test/trap.py` |
| 1 | CheckMK | `checkmk` (Event Console / `mkeventd` handles traps), `checkmk-docs/images/ec_*trap*.png` for UX evidence; also `netapp-ontap-cmk/.../snmp_traphost.py` for trap destination registration |
| 1 | Nagios + SNMPTT/NSTI | `nagios/nsti/nsti/trapdumperdaemon.py`, `nagios/nsti/nsti/trapview.py`, `nagios/nsti/docs/snmpttsetup.rst`, `nagios/nsti/install/snmptt.sh` |
| 1 | Sensu | `sensu/snmptrapd2sensu/`, `sensu/sensu-snmp-trap-handler/`, `sensu/sensu-extensions-snmp-trap/lib/sensu/extensions/snmp-trap/` |
| 1 | Telegraf (InfluxData) | `influxdata/telegraf/plugins/inputs/snmp_trap/`; output: `influxdata/kapacitor/services/snmptrap/` |
| 1 | Logstash (Elastic) | `elastic/logstash-docs/docs/plugins/inputs/snmptrap.asciidoc`, `elastic/built-docs/.../plugins-inputs-snmptrap.html`; plugin source in upstream `logstash-plugins/logstash-input-snmptrap` (need to confirm presence) |
| 1 | Datadog Agent | `datadog-agent/comp/snmptraps/`, `datadog-agent/deps/snmp_traps/`, release notes for `traps-listener` defaults |
| 1 | Splunk Connect for SNMP | `splunk-connect-for-snmp/splunk_connect_for_snmp/traps.py`, `charts/.../templates/traps/`, `examples/traps_enabled_values.yaml`, `examples/polling_and_traps_v3.yaml` |
| 1 | Cribl | `cribl/cribl-control-plane-sdk-go/models/components/functionsnmptrapserialize.go`, `pipelinefunctionsnmptrapserialize.go`, `inputsnmpinput.go`, `outputsnmp.go` (in/out as a stream node) |
| 2 (docs-only) | SolarWinds NPM / Trap Service | No source mirror; vendor docs only |
| 2 (docs-only) | Dynatrace | No source mirror (only APM agents); vendor docs only |
| 2 (docs-only) | LogicMonitor | Only SDK refs mirrored; vendor docs only |

Out of scope (verified no native trap support, or out of relevance):

- Prometheus, OpenTelemetry, VictoriaMetrics, Grafana, Cortex, Thanos, Mimir (pull-only or no traps)
- Cloud-native APM: Honeycomb, Sentry, Lightstep, Dash0, Hyperdx, Coralogix, Logzio, Chronosphere, Sumologic, Edgedelta, Cardinalhq, Last9, Middleware
- Tracing-only: Jaeger, Zipkin, Skywalking
- Icinga (no native trap; uses snmptt as Nagios does)
- New Relic (only via Kentik partner — covered indirectly in SolarWinds/Kentik discussion if relevant)

Risks:

- A sub-agent could fabricate or hallucinate file:line evidence. Mitigation: every per-system file must list its evidence trail; reviewers cross-check.
- For docs-only systems, marketing material can lie. Mitigation: cite vendor URLs explicitly, mark sections as docs-only.
- 6 reviewers × 16 systems = 96 review runs. Mitigation: parallel reviewers per system; sequential systems.

## Implications And Decisions

### Decision 1 — Scope

Decided (user): All 13 tier-1 OSS systems + 3 commercial via docs (SolarWinds, Dynatrace, LogicMonitor). 16 systems total.

### Decision 2 — Depth

Decided (user): Comprehensive (5-15 pages each), with 100% feature and implementation coverage per the Content Requirements below.

### Decision 3 — Process (revised by user)

Decided (user): Three sub-agents run in parallel at a time. Each sub-agent owns one system end-to-end. Each sub-agent spawns its own six external reviewers in parallel (codex, glm, kimi, mimo, minimax, qwen). Each sub-agent ingests reviewer findings, judges importance, improves the document, and re-runs reviewers until there are no major findings. The sub-agent's judgement applies the balance: external reviewers will always find micro issues; iteration stops when only nits and minor stylistic findings remain.

Pilot first: run ONE sub-agent on OpenNMS, validate the entire flow, then scale to 3-at-a-time for the remaining 15 systems.

Iteration prompt (after the first reviewer pass): each subsequent reviewer pass receives the **same full reviewer prompt** as iteration 1 (defined under "External Reviewer Protocol" below), prepended with a one-line note: "This is iteration N — the previous iteration's findings have been addressed; please review the file again in whole." Reviewers must re-read the whole document and report ALL findings, not only ones related to the previous iteration's items. This is mandated to avoid scope-narrowing across iterations (CLAUDE.md rule: never narrow scope between repeated reviews).

### Decision 4 — Output Structure

Decided (user): Common template applied uniformly. Comparison matrix built progressively (after each system, the matrix is updated).

### Decision 5 — Content Requirements (per system)

Decided (user). Each per-system file MUST cover:

1. Exactly which features the system supports (capability inventory)
2. How they implement each feature (summary, plus deep architecture for key features)
3. What tests/fixtures exist to ensure each feature works
4. What configurability is exposed to the end user
5. How each feature is presented to the user (UI/CLI/API/files)
6. What database/store the system maintains to support each feature
7. End-user value derived from the implementation
8. Implementation architecture (components, processes, deployment model)
9. Flexibility for users to customize
10. What is provided out of the box (defaults, bundled MIBs, bundled rules)

## Locked Scope (System Order)

Order chosen so that the most architecturally distinct systems are analysed early (their patterns will inform what to look for in the rest):

1. **OpenNMS** — most comprehensive open-source NMS; full lifecycle including northbound forwarding, alarm correlation, MIB-driven event mapping, UI for trap configuration
2. **Zenoss** — Python-native dedicated `zentrap` daemon; ZenPacks model for vendor extensions
3. **CheckMK** — Event Console approach (treat traps as one of many event sources); unique architecture
4. **Centreon** — `centreontrapd` DB-driven mapping; classic snmptrapd front-end pattern
5. **Zabbix** — native C trapper + Perl receiver bridge; widely deployed
6. **LibreNMS** — PHP handler-per-OID model; also ALERT TRANSPORT (emits traps northbound)
7. **Nagios + SNMPTT/NSTI** — the de-facto translator pattern for the classic NMS family
8. **Sensu** — three different community integration patterns in one ecosystem; instructive comparison
9. **Telegraf (InfluxData)** — modern Go stream input plugin; minimalist approach; pairs with Kapacitor output
10. **Logstash (Elastic)** — log-pipeline ingestion model
11. **Datadog Agent** — modern Go cloud-agent integrated component
12. **Splunk Connect for SNMP** — Kubernetes-native modern pipeline
13. **Cribl** — stream-processor unique angle (trap as input AND output transformation)
14. **SolarWinds** (docs-only) — dominant commercial NMS for traps
15. **Dynatrace** (docs-only) — modern observability platform with trap support via OneAgent / ActiveGate
16. **LogicMonitor** (docs-only) — cloud-NMS with trap support via Collector

## Common Template (per-system file structure)

Each file at `.agents/sow/specs/snmp-traps/<system-slug>.md` follows these sections **in this exact order**. Sections that genuinely do not apply must be present and marked `Not applicable — <evidence-backed reason>` (never omitted).

```
# <System Name> — SNMP Trap Support: Complete Implementation Analysis

## 0. Document Metadata
- System: <name>
- Version analysed: <git commit / docs version>
- Source evidence: mirrored | docs-only
- Repository root analysed: owner/repo @ commit
- Author: assistant
- Reviewer pass: pending | in-progress | accepted

## 1. System Overview & Lineage
- What is it (1 paragraph), license, primary audience, age, ownership
- Where SNMP traps fit in its broader product (one signal of many vs primary use case)
- Relationship to upstream tools (snmptrapd, Net-SNMP, SNMPTT, etc.)

## 2. Trap-Subsystem Architecture
- Components (daemons/services/processes/containers); diagram (ASCII)
- Deployment model (single-node, distributed, HA, container, Kubernetes)
- Languages and key libraries
- Inter-component IPC (sockets, DB, message bus)

## 3. Trap Reception (UDP/162 Ingress)
- Listener implementation (own socket vs delegated to snmptrapd)
- SNMP version support (v1, v2c, v3 USM, TLSTM/DTLS)
- Performance / concurrency model
- Privileged-port handling
- Horizontal scaling pattern
- HA / clustering

## 4. MIB Management
- MIB store location, layout, formats accepted
- Compilation / load pipeline
- Bundled MIBs out-of-the-box (vendor coverage)
- User workflow for adding/updating MIBs
- Dependency resolution
- Version management vs firmware
- Fallback behaviour for unknown OIDs

## 5. Trap Processing Pipeline
- Parse (BER decode, varbind extraction)
- OID-to-name resolution
- Source identification (IP → device mapping; agent-addr handling for v1)
- Enrichment (varbind decoration, lookup tables, topology join)
- Normalization (vendor severity → internal severity; unit conversion)
- Deduplication / suppression (keys, windows, rate limits)
- Routing (where the processed event goes)
- Error handling for malformed PDUs, unknown OIDs, decode failures

## 6. Data Model & Persistent Storage
- Per feature, identify the storage: which database (RDBMS / TSDB / message bus / files); schema (tables, fields); retention policy; indexing
- Tables/streams for: raw traps, processed events/alarms, MIB definitions, OID-to-event mappings, dedup state, suppression rules, severity rules, device inventory, topology, audit/log
- Migration / upgrade handling

## 7. Configuration UX
- All configuration surfaces: config files (paths, formats), CLI, GUI/dashboard, REST API
- What the operator sees by default
- Discoverability of options (defaults documented? auto-completion? validation?)
- Live reload vs restart
- Multi-tenancy / RBAC

## 8. Integration with Other Signals
### 8.1 Metrics
- Are traps converted to metrics? counters? gauges?
- Are traps used as annotations on metric dashboards?
### 8.2 Alerting / Notifications
- How traps become alerts / tickets / pages
- Alert routing, escalation, deduplication policies
- Acknowledgement / clear semantics
### 8.3 Topology
- Is there a topology graph? Are traps mapped onto it? L2/L3/L7?
- Is topology-aware suppression supported?
### 8.4 Logs / Events
- Trap-as-event in a unified event store
- Searchability, retention, schema
### 8.5 Northbound Forwarding
- Can it forward traps to upstream NMS? formats? (SNMPv2c trap, REST, syslog, OTLP)

## 9. Severity Model
- Where vendor severity comes from
- How it is mapped to system severity
- Customization surface

## 10. Storm / Volume Handling
- Per-source rate limits
- Dedup keys and windows
- Circuit breakers
- Storm detection
- Backpressure / queue management

## 11. Security
- SNMPv3 USM support (auth, priv algorithms)
- DTLS / TLSTM support
- Credential storage
- Access control on the trap subsystem itself
- Audit logging

## 12. Trap Simulation & Testing (in-source evidence)
- Unit tests (paths, count, what they cover)
- Integration tests (containers? fixtures? real PDU bytes?)
- Sample trap fixtures included (file paths)
- Tools shipped for trap simulation
- CI workflow for trap pipeline

## 13. Out-of-the-Box Coverage (defaults)
- MIBs bundled
- Severity rules bundled
- Dedup defaults
- Vendor packs / integration packages
- Sample/preset dashboards or reports

## 14. User Customization Surface
- How users add custom OID handlers
- Custom MIBs
- Custom severity rules
- Custom dedup rules
- Plugin / extension model (e.g. ZenPacks, Nagios plugins, Telegraf processors)
- API surface for automation

## 15. End-User Value Analysis
- What an operator gets day-1 with default config
- What requires customization
- Learning curve
- Operational toil
- Visibility into the pipeline's own health

## 16. Strengths
- Design wins, with file:line evidence

## 17. Weaknesses / Gaps
- Documented design tradeoffs that hurt users, with evidence (issues, bugs, mailing-list complaints, code smell)

## 18. Notable Code or Configuration Examples
- 3-6 quotable evidence blocks: file:line + small extract demonstrating a key design decision

## 19. Sources Examined
- Repository paths (relative), commits, key files
- For docs-only: vendor URLs with retrieval date

## 20. Evidence Confidence
- Per major section, rate: high (source-verified) | medium (docs-only but consistent) | low (single source / unverifiable)
```

## External Reviewer Protocol

Each completed per-system file is sent to **six** reviewers in parallel.

Reviewers:

1. `codex` — `timeout 1800 codex exec "<prompt>" --skip-git-repo-check`
2. `glm` — `timeout 1800 opencode run -m "llm-netdata-cloud/glm-5.1" --agent code-reviewer "<prompt>"`
3. `kimi` — `timeout 1800 opencode run -m "llm-netdata-cloud/kimi-k2.6" --agent code-reviewer "<prompt>"`
4. `mimo` — `timeout 1800 opencode run -m "llm-netdata-cloud/mimo-v2.5-pro" --agent code-reviewer "<prompt>"`
5. `minimax` — `timeout 1800 opencode run -m "llm-netdata-cloud/minimax-m2.7-coder" --agent code-reviewer "<prompt>"`
6. `qwen` — `timeout 1800 opencode run -m "llm-netdata-cloud/qwen3.6-plus" --agent code-reviewer "<prompt>"`

The exact reviewer prompt (shown to user before first run and unchanged across systems):

```
YOU ARE BEING RUN BY ANOTHER ASSISTANT, FOR A SECOND OPINION.

CONTEXT
We are producing a comparative analysis of how monitoring systems implement
SNMP traps, to inform design decisions in the Netdata project. The
foundational spec for the entire effort is at:

  .agents/sow/specs/snmp-traps/snmp-traps-in-observability.md

The Statement of Work governing this comparative analysis is:

  .agents/sow/current/SOW-0032-20260522-snmp-trap-comparative-analysis.md

This SOW defines a uniform per-system template (sections 0-20) and the
acceptance criteria. Please read both files first.

TASK
Review the per-system analysis file at:

  .agents/sow/specs/snmp-traps/<SYSTEM_SLUG>.md

Verify:

1. Accuracy: every claim about implementation traces to source evidence
   (file:line) or to a cited vendor doc URL. Flag any claim that is not
   verifiable from the cited evidence, or that contradicts the cited evidence.
2. Completeness: every section of the Common Template (in the SOW) is
   present and meaningfully filled. Sections marked "Not applicable" must
   carry an evidence-backed reason.
3. Coverage: the 10 Content Requirements in the SOW (features supported,
   implementation summary, tests/fixtures, configurability, presentation,
   databases, end-user value, architecture, customisation flexibility,
   out-of-the-box defaults) are fully addressed.
4. Faithfulness: the file does not overstate, understate, or romanticise
   the system. Brutal honesty is required. Flag marketing language.
5. Source coverage: identify important code paths, tests, fixtures, or
   documentation that the analysis missed and should have included.
6. Comparability: identify any place where the framing is so system-specific
   that it cannot be compared with other systems following the same template.

For systems with source mirrored locally:
please explore the source where useful to validate claims. For docs-only
systems, validate against the cited vendor URLs.

OUTPUT FORMAT
- Findings as a numbered list, each finding includes: severity (blocker /
  major / minor / nit), section reference, evidence, and a concrete fix.
- An overall verdict: accept | accept-with-fixes | reject.
- A "missed content" appendix listing genuinely material content not
  present in the file but found in source.

MANDATORY RULES
- DO NOT MAKE CHANGES, DO NOT CREATE/MODIFY/DELETE FILES, DO NOT STOP
  PROCESSES OR SERVICES.
- DO NOT ASK FOR PERMISSIONS - THIS IS A NON-INTERACTIVE SESSION.
- DO NOT RUN OTHER EXTERNAL ASSISTANTS. RISK OF INFINITE RECURSION.

THIS IS A READ-ONLY REQUEST. PROVIDE YOUR REVIEW.
```

Reviewer-finding handling per system (per sub-agent):

- Severity classification by the sub-agent: blocker | major | minor | nit.
- **Iterate while any major (or blocker) finding is present**: revise the document and re-run all six reviewers with the simplified prompt "Read and review the completeness and accuracy of file X."
- **Stop when only minor/nit findings remain**: the sub-agent records the surviving minor findings inline in the file's "Reviewer pass" log and returns.
- The sub-agent must NOT chase asymptotic perfection. Reviewers will always find micro issues; the sub-agent applies judgement and stops at "no major findings remain."
- The sub-agent must NOT narrow scope between iterations — each re-review runs all six reviewers, never just the ones that complained.
- The sub-agent appends a per-system review log to its returned summary, including: iterations count, findings per iteration, classification decisions and reasoning.

## Comparison Matrix (incremental)

After each per-system file passes review, `.agents/sow/specs/snmp-traps/comparison/comparison-matrix.md` is updated. Columns are the systems analysed so far; rows are common dimensions (reception, MIB management, normalization, dedup, storage, alerting integration, topology integration, simulation, security, defaults, customization API). After the last system, this matrix becomes the input for `comparative-analysis.md` and then `netdata-design-implications.md`.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- We are not patching a bug; we are doing foundational analysis to inform a future design. The root question is whether the chosen scope, template, system order, and reviewer protocol will yield decision-grade evidence.

Evidence reviewed:

- The foundational spec `.agents/sow/specs/snmp-traps/snmp-traps-in-observability.md`
- Local mirrored repositories (presence survey complete)
- SOW template `.agents/sow/SOW.template.md`

Affected contracts and surfaces:

- New files under `.agents/sow/specs/snmp-traps/` (per-system files + comparison subdir)
- No source code changes
- No public docs changes (until final design discussion phase)

Existing patterns to reuse:

- The foundational spec sets the vocabulary and section structure for analysing trap subsystems; per-system templates align with it.
- Project rules in CLAUDE.md / AGENTS.md (open-source reference citation, no absolute paths in evidence, redaction of sensitive data).

Risk and blast radius:

- Risk: hallucinated file:line citations. Mitigation: external reviewers spot-check; the assistant must paste a small verbatim extract for every notable code/config example (§18 of template) so reviewers can validate.
- Risk: marketing language for docs-only systems. Mitigation: explicit "Vendor-documented, not source-verified" labelling.
- Risk: scope creep into adjacent topics (polling, flow telemetry). Mitigation: §1 Scope & Applicability of the foundational spec sets the boundary; per-system files must respect it.

Sensitive data handling plan:

- No SNMP communities, bearer tokens, customer names, or proprietary incident details will appear in per-system files. Where vendor public docs use example community strings, those will appear verbatim (they are vendor-supplied public examples).

Implementation plan:

1. Confirm SOW with user (this step).
2. Create the empty per-system file skeletons for all 16 systems using the Common Template. Each starts with status `pending`.
3. System #1 (OpenNMS): write full analysis → run 6 reviewers in parallel → integrate findings → mark accepted → update matrix.
4. Iterate steps 3 for each subsequent system.
5. After all per-system files accepted: write `comparative-analysis.md` synthesising the matrix.
6. After comparative analysis: write `netdata-design-implications.md` discussing how Netdata should approach trap support.
7. Final user review of comparative analysis and design implications.

Validation plan:

- Per-system: six-reviewer parallel verification.
- Cross-system: the matrix surfaces inconsistencies between files; revisions issued to resolve them.
- Final: user review.

Artifact impact plan:

- AGENTS.md: no expected change (this is research, not framework change).
- Runtime project skills: no expected change.
- Specs: substantial additions under `.agents/sow/specs/snmp-traps/`.
- End-user/operator docs: no expected change.
- End-user/operator skills: no expected change.
- SOW lifecycle: this SOW will be moved to `done/` only after the design implications document is accepted by user.

Open-source reference evidence:

- All evidence will use the `owner/repo @ commit` plus repo-relative paths convention. The mirrored monolith roots map to upstream repositories with public histories.

Open decisions (require user confirmation before per-system work starts):

1. **OD-1 — System order**: the order proposed in §Locked Scope. Adjust if you want a different sequence (e.g., start with a smaller system to validate the template, or with a system whose architecture is most analogous to Netdata).
2. **OD-2 — Common template sections**: 20 sections proposed. Adjust if any section is missing or should be split/merged.
3. **OD-3 — Reviewer prompt**: confirm wording (shown verbatim above).
4. **OD-4 — Per-system file size cap**: the user said "5-15 pages each". Confirm or adjust. (Long files cost reviewer attention; very short files cost analytical depth.)
5. **OD-5 — Reviewer rerun policy**: blocking findings trigger a full 6-reviewer re-review (per CLAUDE.md "never narrow scope between iterations"). Minor findings can be fixed and a single pass-through of all 6 done at the end of the system. Confirm.
6. **OD-6 — Commercial scope**: SolarWinds, Dynatrace, LogicMonitor are proposed. Add/drop (e.g., add IBM Tivoli Netcool, BMC TrueSight Network Automation, ManageEngine OpManager) before we lock the list.

## Execution Log

### 2026-05-22

- Read foundational spec `.agents/sow/specs/snmp-traps/snmp-traps-in-observability.md`.
- Surveyed mirrored repos; identified 13 first-class OSS trap implementations + 3 commercial doc-only candidates.
- Authored SOW with locked scope, template, reviewer protocol, and pre-implementation gate.
- The user confirmed scope, depth, template, reviewer set, and content requirements; revised process to: 3 sub-agents in parallel; each sub-agent spawns its own 6 reviewers in parallel and iterates until no major findings remain.
- Decision: pilot OpenNMS with one sub-agent first; after pilot accepted, scale to 3 parallel sub-agents for remaining 15 systems.
- Verified `codex` and `opencode` CLIs are installed and runnable from this host.
- Pilot sub-agent launched for OpenNMS.

## Validation

Pending.

Sensitive data gate:

Pending. Before this SOW can close, all durable artifacts produced by it must be checked for raw secrets, SNMP communities, customer identifiers, personal data, non-documentation IPs, private endpoints, and workstation-local paths.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
