# SOW-20260613-snmp-trap-documentation - SNMP Trap Documentation

## Status

Status: in-progress

Sub-state: Artifact organization, documentation structure, and text-first
foundation drafting are approved. Foundation pages are being drafted from
implementation evidence. Runtime verification, screenshots, OTLP proof, and
benchmark/sizing evidence remain gated before those claims or visuals are added
to public docs.

## Requirements

### Purpose

Make SNMP trap documentation fit for network operators, NetOps, SecOps, and
SREs who need to deploy, validate, query, forward, and troubleshoot traps
without reading Go code or SOW research artifacts.

The documentation must be:

- operator-first and task-oriented;
- structurally consistent with the existing Network Flows documentation set;
- accurate to the implemented SNMP trap collector, profile, metrics, journal,
  and OTLP behavior;
- clear about what is stored locally, what can be queried with journalctl, and
  what can be forwarded to external log systems;
- explicit about source identity, deduplication, profile-defined metrics,
  cardinality limits, failure modes, and security-sensitive values;
- published through the Learn documentation map, not left as unmapped files.

### User Request

The user requested a new SOW for SNMP trap documentation after the implementation
agent completed the metrics work.

User-stated constraints and decisions:

- SNMP trap documentation should follow the Network Flows documentation pattern.
- The trap backend database is journal-compatible, so docs must explain
  journalctl and normal journal workflows for dumping, copying, and SIEM
  integration.
- OTLP forwarding exists for the same reason: pushing traps onward into log
  systems.
- Research under `.agents/sow/specs/snmp-traps/` must be separated from
  Netdata specs so it cannot be confused with product specification.
- The SNMP trap authoring skill must document this specs-vs-research
  organization.

### Assistant Understanding

Facts:

- Network Flows has a full documentation set under `docs/network-flows/`.
- Network Flows is published through `docs/.map/map.yaml` under the "Network
  Flows" sidebar section.
- Learn routing is controlled by `docs/.map/map.yaml`; filesystem path alone
  does not publish or route a page.
- Existing SNMP trap generated integration documentation lives at
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md`.
- SNMP trap profile format documentation lives at
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`.
- Current durable SNMP trap specs and research are mixed under
  `.agents/sow/specs/snmp-traps/`.
- The previous implementation SOW is present in `.agents/sow/active/` with
  `Status: completed`; it must not be merged as an active SOW file.

Inferences:

- A single collector integration page is not enough for this feature because
  SNMP traps include setup, profile authoring, metrics, logs, journal workflows,
  OTLP forwarding, validation, troubleshooting, and investigation workflows.
- Mirroring the Network Flows documentation structure is the long-term-best
  route because traps are another network-event ingestion surface with a Live
  view, local storage, operational charts, configuration, validation, and
  anti-patterns.
- Research files should remain durable but be visibly classified as research,
  not product contracts.

Resolved direction:

- The SNMP trap docs are a new top-level Learn section named "SNMP Traps".
- The proposed 16-page split is larger than Network Flows because SNMP traps
  add profile authoring, enrichment, journal-native querying, OTLP forwarding,
  metrics/alerts, and SIEM export concerns. Phase 3 may consolidate pages if
  the evidence-filtered draft makes any page too thin. A page is a consolidation
  candidate when it has fewer than 3 evidence-anchored subsections after
  roadmap and unsupported claims are removed.
- The first documentation pass is text-first. Screenshots are deferred until the
  text docs are ready, a representative trap journal is populated, and the user
  provides screenshots for filtering, histogram, search, and related Logs UI
  workflows. Screenshot insertion is a separate docs change that requires text
  pages to be ready and user-provided screenshots to be attached to the PR or a
  follow-up issue.
- Runtime evidence needs two synthetic trap datasets:
  - a high-rate benchmark feed to re-measure current trap processing throughput;
    the working expectation from previous benchmark evidence is about
    50K-100K traps/second, but public docs must cite only the new measured result
    and its hardware/test conditions;
  - a screenshot/demo feed with enough trap variety and time spread for Logs UI
    filters, histogram, search, and field examples. Normal received traps are
    timestamped at receive time (`collector.go:637`) and the journal writer uses
    that timestamp (`trapwriter_impl.go:165`), so screenshot preparation must
    either run a synthetic sender across the desired time window or use an
    evidence-only journal seeding helper that is never documented as a shipped
    product feature.
- Public docs describe what the shipped system does today. Missing features,
  roadmap items, and pending internal work are tracked internally or in follow-up
  SOWs, not documented as user-facing gaps.
- The unsupported `1 ms per PDU` decode-time claim has been corrected in the
  collector metadata, generated integration documentation, and durable Netdata
  trap spec. Public docs must not reintroduce decode-time language without new
  benchmark/runtime evidence.
- Historical SOW-reference cleanup is not part of this docs work. Old SOW files
  and references are treated as branch-local/history artifacts and must not block
  drafting the end-user documentation. They still must not be committed in active
  SOW storage at final merge.

Unknowns:

- Whether any implementation behavior discovered during final review should
  pause docs until fixed, or be documented as a current shipped-feature
  limitation.

### Docs UX Contract

The SNMP trap documentation is not a mechanical conversion of implementation
facts into pages. The documentation MUST be written around the operator's mental
model:

- Did traps arrive?
- What do the traps mean?
- Is this noise, a storm, a security signal, or an incident?
- Which source/device/vendor/trap type is involved?
- Are traps being dropped, rate-limited, deduplicated, unresolved, or forwarded?
- Can the operator query, copy, retain, and forward the data using normal journal
  and log-system workflows?
- What is the next action after the page answers the question?

Each public page MUST pass a page-level UX gate before drafting:

- **Reader question:** the concrete operator question this page answers.
- **Reader role:** the likely reader: NetOps, NOC, SRE, SecOps, platform owner,
  or MSP/operator.
- **Success path:** the shortest successful workflow the reader can complete
  from this page.
- **First signal:** the first Netdata row, field, chart, alert, command, or UI
  interaction the reader should look at.
- **Common failure:** the mistake or false assumption this page should prevent.
- **Evidence source:** implementation/spec/research evidence supporting each
  claim.
- **Screenshot/data need:** whether this page needs representative trap journal
  data, Logs UI screenshots, benchmark data, or only static examples.
- **Next action:** the page the reader should naturally visit next.

Every page MUST be organized around a user task, not around source files. Code,
collector configuration, specs, generated metadata, and research are evidence;
they are not the narrative structure.

### Page Subagent Drafting Model

Drafting SHOULD use one focused subagent per page or tightly coupled page pair.
The subagent's job is to create a useful operator page, not to dump every
available fact. Each page subagent MUST receive:

- the page's UX gate answers;
- the relevant Phase 1 implementation evidence entries;
- the relevant Phase 2 research gap entries;
- the public-doc guardrails: shipped behavior only, no missing topology/Flow
  correlation text, no roadmap language, no unsupported benchmark or screenshot
  claims;
- the expected cross-links to adjacent pages.

The work MUST NOT run as one massive blind parallel batch. Use small batches
with an integration pass after each batch:

1. **Foundation batch:** overview, installation, quick start, configuration.
2. **Meaning batch:** trap profiles, enrichment, usage/output, field reference.
3. **Operations batch:** journal/querying, forwarding to SIEM, metrics/alerts,
   validation.
4. **Incident batch:** investigation playbooks, troubleshooting, anti-patterns,
   sizing/capacity.

After each batch, the orchestrating agent MUST perform a cross-page UX review:

- duplicate or contradictory concepts are consolidated;
- terminology is consistent (`trap`, `inform`, `varbind`, `source`, `profile`,
  `journal`, `OTLP`, `severity`, `category`);
- each page has a clear entry point and next action;
- implementation facts are cited internally in the SOW, but public prose remains
  operator-centered;
- screenshots and benchmark numbers remain gated until runtime evidence exists.

### Page UX Briefs

These briefs are the drafting contract for page subagents. A subagent MAY refine
wording while drafting, but MUST NOT change a page's purpose without recording
the reason in this SOW and passing the integration review.

#### Foundation Batch

- **`README.md` / Overview**
  - **Reader question:** What are SNMP traps in Netdata, and what operational
    questions can they answer?
  - **Reader role:** NetOps, NOC, SRE, SecOps, platform owner, MSP/operator.
  - **Success path:** Reader understands traps as asynchronous network events
    stored as logs, summarized as metrics, and forwarded as logs when needed.
  - **First signal:** Trap rows in Logs plus receiver pipeline/severity charts.
  - **Common failure:** Treating traps as polling replacement, treating silence
    as health, or treating raw trap volume as incident severity.
  - **Evidence source:** Network Flows overview pattern, Phase 1 inventory for
    shipped collector behavior, research mental-model sections.
  - **Screenshot/data need:** Optional final overview screenshot after demo trap
    journal exists; no benchmark dependency.
  - **Next action:** `quick-start.md` for first receipt, or `installation.md`
    when the receiver is not installed/configured.
- **`installation.md`**
  - **Reader question:** What must exist on the host and network before Netdata
    can receive traps?
  - **Reader role:** Platform owner, NetOps, SRE.
  - **Success path:** Reader confirms Linux/runtime prerequisites, UDP reachability,
    port/capability handling, package availability, and where to configure jobs.
  - **First signal:** Configured listener job starts and receiver metrics/log
    source become available.
  - **Common failure:** Assuming traps are auto-detected, forgetting UDP/162
    privileges/firewall paths, or sending from a blocked source.
  - **Evidence source:** collector metadata, config schema, stock configuration,
    generated integration docs, implementation bind/preflight behavior.
  - **Screenshot/data need:** Static examples only.
  - **Next action:** `quick-start.md`.
- **`quick-start.md`**
  - **Reader question:** Can I get one known trap into Netdata and prove it
    arrived?
  - **Reader role:** NetOps, NOC, SRE.
  - **Success path:** Reader creates a minimal listener, sends or triggers a test
    trap, sees it in Logs, and confirms pipeline metrics moved.
  - **First signal:** `snmp:traps` / Logs source contains a trap row, and
    received/decoded/accepted/committed counters increment.
  - **Common failure:** Wrong destination IP/port, source allowlist miss,
    community/v3 mismatch, or expecting devices to send traps before device-side
    trap destinations are configured.
  - **Evidence source:** config schema, Function tests, journal writer behavior,
    pipeline metrics.
  - **Screenshot/data need:** Representative demo trap row and Logs UI screenshot
    after runtime evidence pass.
  - **Next action:** `configuration.md` for production hardening, or
    `usage-and-output.md` to read the row.
- **`configuration.md`**
  - **Reader question:** How do I safely control who can send traps, which
    SNMP versions are accepted, and where trap data goes?
  - **Reader role:** NetOps, SRE, SecOps, platform owner.
  - **Success path:** Reader configures listener endpoints, versions,
    communities or SNMPv3 users, allowlists, trusted relays, rate limits, dedup,
    journal, OTLP, retention, and profile metrics.
  - **First signal:** Job configuration validates; the pipeline accepts only
    expected sources and exposes the intended output backend.
  - **Common failure:** Leaving a listener broadly open, storing secrets inline
    instead of using Secrets Management, misunderstanding OTLP-only local-query
    behavior, or enabling high-cardinality profile metrics blindly.
  - **Evidence source:** config schema, `config.go`, Secrets Management docs,
    output-backend validation, rate-limit/dedup implementation.
  - **Screenshot/data need:** Static examples; runtime snippets only after
    Phase 1.5 verification.
  - **Next action:** `trap-profiles.md`, `journal-and-querying.md`, or
    `forwarding-to-siem.md`.

#### Meaning Batch

- **`trap-profiles.md`**
  - **Reader question:** How does Netdata turn raw trap OIDs into useful trap
    names, categories, severities, messages, tags, and metrics?
  - **Reader role:** NetOps, NOC, SRE, vendor/MIB owner.
  - **Success path:** Reader understands stock trap coverage, operator profile
    overrides, profile reload behavior, and profile-defined metric controls.
  - **First signal:** `TRAP_NAME`, `TRAP_CATEGORY`, `TRAP_SEVERITY`,
    `MESSAGE`, `TRAP_TAG_*`, and profile-derived charts.
  - **Common failure:** Expecting runtime MIB compilation, assuming unknown OIDs
    are broken ingestion, or creating high-cardinality metric labels.
  - **Evidence source:** profile-format docs, profile catalogue, profile loader,
    profile metric rules, metrics caps.
  - **Screenshot/data need:** Static examples first; profile-derived chart
    screenshot only after demo data exists.
  - **Next action:** `usage-and-output.md` and `field-reference.md`.
- **`enrichment.md`**
  - **Reader question:** What context does Netdata add to each trap, and how do I
    trust the source identity?
  - **Reader role:** NetOps, SRE, SecOps.
  - **Success path:** Reader understands source identity, UDP peer vs selected
    trap source, trusted relays, reverse DNS, device/vendor identity, and
    enrichment audit fields that are actually shipped.
  - **First signal:** Source fields, reverse-DNS/device/vendor fields, and
    `TRAP_ENRICHMENT`.
  - **Common failure:** Trusting relayed source data without configuring trusted
    relays, confusing UDP peer with device identity, or presenting target-state
    topology/correlation language as shipped behavior.
  - **Evidence source:** source attribution code, resolver/enrichment code,
    serialize fields, user decision forbidding missing topology/Flow language.
  - **Screenshot/data need:** Demo rows with multiple source identities; no
    benchmark dependency.
  - **Next action:** `field-reference.md` and `validation-and-data-quality.md`.
- **`usage-and-output.md`**
  - **Reader question:** How do I read a trap row and understand what happened?
  - **Reader role:** NOC, NetOps, SecOps, SRE.
  - **Success path:** Reader can interpret trap name, source, severity, category,
    message, varbinds, tags, JSON, binary-encoded fields, and dedup summaries.
  - **First signal:** `TRAP_NAME`, `TRAP_SOURCE_IP`, `TRAP_SEVERITY`,
    `TRAP_CATEGORY`, `TRAP_VAR_*`, `TRAP_JSON`, `TRAP_REPORT_TYPE`.
  - **Common failure:** Confusing device `sysUpTime` with wall-clock time,
    assuming every varbind is indexed as a top-level field, or overlooking
    binary/sensitive-value handling.
  - **Evidence source:** serializer, dedup summary, decode-error serialization,
    field filtering, Logs Function defaults.
  - **Screenshot/data need:** Logs row screenshot and field/facet examples after
    representative demo data exists.
  - **Next action:** `field-reference.md`, then `journal-and-querying.md`.
- **`field-reference.md`**
  - **Reader question:** What fields exist, what do they mean, and when should I
    expect them to be populated?
  - **Reader role:** NetOps, SecOps, SIEM engineer, SRE.
  - **Success path:** Reader can map field names to meaning, source, type,
    population conditions, query use, and sensitive-data cautions.
  - **First signal:** Canonical field table grouped by identity, trap meaning,
    source/enrichment, varbinds, tags, reports, and forwarding fields.
  - **Common failure:** Treating optional fields as always populated, exposing
    sensitive varbind values in examples, or using internal implementation names
    instead of journal/log fields.
  - **Evidence source:** serializer constants, OTLP mapping, generated metadata,
    Function default facets.
  - **Screenshot/data need:** Static table plus sample rows after demo data
    exists.
  - **Next action:** `journal-and-querying.md` or `forwarding-to-siem.md`.

#### Operations Batch

- **`journal-and-querying.md`**
  - **Reader question:** How do I query, dump, copy, retain, or integrate local
    trap data using journal-compatible workflows?
  - **Reader role:** SRE, platform owner, SIEM engineer, NetOps.
  - **Success path:** Reader can find per-job journal directories, use
    `journalctl --directory`, understand `snmp:traps` local sources, and know
    what changes in OTLP-only mode.
  - **First signal:** Per-job journal path, `__logs_sources`, and queryable
    `TRAP_*` fields.
  - **Common failure:** Expecting host journald entries instead of direct
    per-job journal files, or expecting local Logs sources when journal output is
    disabled.
  - **Evidence source:** journal writer, Function handler/tests, serializer,
    output-backend selection.
  - **Screenshot/data need:** Runtime-proven commands and copied-directory
    behavior; no invented output.
  - **Next action:** `forwarding-to-siem.md` or `field-reference.md`.
- **`forwarding-to-siem.md`**
  - **Reader question:** How do I send trap data onward to log and SIEM systems?
  - **Reader role:** SIEM engineer, SecOps, SRE, platform owner.
  - **Success path:** Reader understands journal-native export/copy/query
    workflows and OTLP log forwarding with resource attributes.
  - **First signal:** Journal export path or OTLP LogRecords with
    `service.name=netdata-snmptrap` and `service.instance.id=<job>`.
  - **Common failure:** Expecting Netdata to re-emit SNMP traps northbound, or
    treating OTLP as required for local storage.
  - **Evidence source:** OTLP writer, journal writer, Function tests, user
    requirement about journalctl/SIEM methodology.
  - **Screenshot/data need:** Runtime-proven OTLP receiver example and SIEM-safe
    sanitized payload.
  - **Next action:** `configuration.md` for backend settings and
    `field-reference.md` for field mapping.
- **`metrics-and-alerts.md`**
  - **Reader question:** How do I know the trap pipeline is healthy and alert on
    important conditions?
  - **Reader role:** NOC, SRE, NetOps.
  - **Success path:** Reader understands receiver pipeline charts, severity
    charts, error charts, dedup suppression, source charts, profile metrics, and
    shipped health alerts.
  - **First signal:** Received/decoded/accepted/committed/write-failed pipeline,
    severity rates, error dimensions, and alert templates.
  - **Common failure:** Using raw trap count as an incident KPI, ignoring drops
    or decode failures, or assuming unknown OIDs should always alert.
  - **Evidence source:** `charts.yaml`, health templates, metrics implementation,
    profile metric caps.
  - **Screenshot/data need:** Chart screenshots and alert examples after demo
    data exists.
  - **Next action:** `validation-and-data-quality.md` and `troubleshooting.md`.
- **`validation-and-data-quality.md`**
  - **Reader question:** How do I prove trap data is complete enough and not
    silently broken?
  - **Reader role:** SRE, NetOps, NOC.
  - **Success path:** Reader validates receipt, decode, accept, commit, unknown
    OID rate, allowlist/rate-limit drops, profile load health, write failures,
    and representative end-to-end test traps.
  - **First signal:** Pipeline deltas, error dimensions, last-seen/source counts,
    known test trap row, and relevant system UDP drop signals when applicable.
  - **Common failure:** Declaring success after one received trap while the
    pipeline is dropping, failing to decode, or failing to commit others.
  - **Evidence source:** pipeline metrics, error metrics, decode-error rows,
    journal writer failure paths, research validation playbook.
  - **Screenshot/data need:** Runtime evidence for test trap and pipeline checks.
  - **Next action:** `troubleshooting.md` for failed checks or
    `metrics-and-alerts.md` for continuous monitoring.

#### Incident Batch

- **`investigation-playbooks.md`**
  - **Reader question:** What should I do when traps indicate a real operational
    or security situation?
  - **Reader role:** NOC, NetOps, SecOps, incident responder, MSP/operator.
  - **Success path:** Reader follows short playbooks for trap storm, missing
    traps, auth failures, cold/warm start, link flap, unknown OID, device-source
    anomaly, and forwarding verification.
  - **First signal:** Time range, histogram, source filter, trap name/category,
    severity, top sources, and error dimensions.
  - **Common failure:** Assuming no trap means no incident, bulk-closing noisy
    traps without checking source pattern, or treating every vendor severity as
    operational truth.
  - **Evidence source:** research playbook, Phase 1 field/metric evidence,
    shipped Logs/metrics behavior.
  - **Screenshot/data need:** Demo trap journal with time spread, multiple
    severities/categories, and searchable varbinds.
  - **Next action:** `troubleshooting.md`, `validation-and-data-quality.md`, or
    `forwarding-to-siem.md`.
- **`troubleshooting.md`**
  - **Reader question:** Why am I not seeing the traps I expect, or why are traps
    noisy/broken?
  - **Reader role:** NetOps, NOC, SRE.
  - **Success path:** Reader follows a decision tree: no packet, no decode, no
    accept, no commit, no Logs source, no SIEM export, high drops, v3/auth/engine
    mismatch, profile errors, or unknown OID.
  - **First signal:** Receiver counters and error dimensions before deeper
    packet or config checks.
  - **Common failure:** Debugging Netdata first when the device never sent to the
    right destination, or debugging the device first when Netdata is dropping or
    rejecting accepted-source packets.
  - **Evidence source:** collector errors, health alerts, config schema,
    decode-error rows, OTLP failure paths, research diagnostic quick reference.
  - **Screenshot/data need:** Static decision tree first; runtime examples after
    evidence pass.
  - **Next action:** `configuration.md`, `validation-and-data-quality.md`, or
    `trap-profiles.md`.
- **`anti-patterns.md`**
  - **Reader question:** What trap practices create noise, blind spots, security
    risk, or scale failures?
  - **Reader role:** NetOps lead, SRE, SecOps, platform owner.
  - **Success path:** Reader avoids unsafe/open listeners, cleartext exposure on
    untrusted networks, enabling everything without policy, ignoring MIB/profile
    maintenance, no load testing, high-cardinality profile metrics, and treating
    traps as reliable metrics.
  - **First signal:** Configuration and operating-practice checklist.
  - **Common failure:** Copying a quick-start config into production without
    allowlists, secrets handling, rate limits, validation, or sizing.
  - **Evidence source:** research anti-patterns/failure modes, config/security
    implementation, metrics cardinality caps.
  - **Screenshot/data need:** None.
  - **Next action:** `configuration.md`, `validation-and-data-quality.md`, and
    `sizing-and-capacity.md`.
- **`sizing-and-capacity.md`**
  - **Reader question:** How big should the receiver be, and what happens during
    storms?
  - **Reader role:** SRE, platform owner, NetOps lead.
  - **Success path:** Reader understands queue defaults, retention controls,
    backend choices, source/cardinality caps, UDP buffer considerations, and that
    published throughput numbers require benchmark evidence.
  - **First signal:** Receiver pipeline rate, write failures, queue/backend
    defaults, retention settings, source/cardinality limits, host CPU/memory/disk
    metrics.
  - **Common failure:** Assuming previous 50K-100K traps/second evidence applies
    to every host/configuration, or ignoring storage and UDP receive buffers.
  - **Evidence source:** implementation defaults, previous benchmark evidence as
    expectation only, new benchmark sub-phase results before publication.
  - **Screenshot/data need:** High-rate benchmark results and hardware/test
    conditions before numeric throughput claims.
  - **Next action:** `validation-and-data-quality.md` and `anti-patterns.md`.

### Acceptance Criteria

- A published SNMP trap documentation section is added to `docs/.map/map.yaml`.
- The docs mirror the useful Network Flows pattern without copying irrelevant
  flow-specific pages.
- Docs cover overview, installation/setup, quick start, configuration, trap
  profiles, profile-defined metrics, receiver/pipeline metrics, enrichment,
  Logs UI usage and output interpretation, journal/log storage,
  journalctl/SIEM workflows, OTLP forwarding, field reference, sizing and
  capacity planning, validation, troubleshooting, investigation playbooks,
  anti-patterns, and shipped-feature limitations.
- Public docs must not describe missing topology correlation, missing Network
  Flow correlation, or other roadmap work as product gaps. These items may be
  tracked internally as follow-up work, but the docs must stay focused on
  shipped behavior.
- The generated collector integration documentation and metadata remain
  synchronized with source docs.
- The trap profile format doc remains the authoritative profile schema
  reference and links cleanly from the operator docs.
- `.agents/sow/specs/snmp-traps/` contains Netdata specs and decisions only at
  the top level; research/playbooks/comparative studies move under
  `.agents/sow/specs/snmp-traps/research/`.
- The project SNMP trap profile authoring skill documents the
  specs-vs-research organization.
- All links to moved research files are updated.
- Documentation avoids raw secrets, SNMP communities, private customer data,
  customer-identifying public IPs, and proprietary incident details.
- Validation proves Markdown/MDX safety, map syntax, link integrity for changed
  docs, and sensitive-data safety for SOW/spec/skill/doc artifacts.

## Analysis

Sources checked:

- `.agents/sow/SOW.template.md`
- `.agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md`
- `.agents/skills/learn-site-structure/SKILL.md`
- `.agents/skills/sync-docs-specs-skills/SKILL.md`
- `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md`
- `docs/.map/map.yaml`
- `docs/.map/README.md`
- `docs/network-flows/README.md`
- `docs/network-flows/quick-start.md`
- `docs/network-flows/configuration.md`
- `docs/network-flows/visualization/dashboard-cards.md`
- `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md`
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`
- `.agents/sow/specs/snmp-traps/`

The Network Flows files above are representative structure and tone references,
not an exhaustive inventory of every Network Flows page.

Current state:

- Network Flows docs are a multi-page top-level section mapped in
  `docs/.map/map.yaml`.
- SNMP trap public docs currently appear mainly through generated integration
  docs and profile-format reference material.
- SNMP trap research, external-product studies, comparative matrices, and
  Netdata specs are mixed in one specs directory.
- The SNMP trap authoring skill references `.agents/sow/specs/snmp-traps/netdata.md`
  but does not yet define the research subdirectory rule.

Documentation information architecture:

- Add a mapped top-level `SNMP Traps` section, using source files under
  `docs/snmp-traps/`.
- Proposed page split:
  - `README.md`: feature overview, what traps are, what Netdata receives,
    decodes, stores, queries, forwards, and what traps can/cannot answer.
  - `installation.md`: prerequisites, listener setup, UDP/162 permissions,
    package/path expectations, and first receive checks.
  - `quick-start.md`: configure one trap source, verify receipt, open/query
    `snmp:traps`, and confirm metrics/alerts.
  - `configuration.md`: `go.d/snmp_traps.conf` jobs, sources, security,
    allowlists, rate limits, dedup, journal, OTLP, retention, and profile
    metrics knobs.
  - `trap-profiles.md`: operator-facing profile workflow, stock profiles,
    overrides, custom MIB conversion, and links to the authoritative
    profile-format reference.
  - `enrichment.md`: source identity, trusted relays, reverse DNS, vendor and
    interface enrichment, vnode attribution, `TRAP_ENRICHMENT`, and
    `TRAP_TAG_*`.
  - `usage-and-output.md`: existing Logs UI behavior, `snmp:traps`,
    `__logs_sources`, decoded trap rows, decode-error rows, dedup summary rows,
    and built-in/profile metric charts.
  - `journal-and-querying.md`: local direct journal storage, `snmp:traps`,
    `__logs_sources`, `TRAP_*`, `TRAP_VAR_*`, `TRAP_JSON`, Agent API, Cloud
    Logs, and `journalctl` workflows.
  - `forwarding-to-siem.md`: journal-native dump/copy/export workflows, SIEM
    integration patterns, OTLP forwarding, and the local-journal vs OTLP-only
    behavior differences.
  - `metrics-and-alerts.md`: receiver pipeline metrics, per-source metrics,
    profile-defined trap metrics, profile metric diagnostics, and health alerts.
  - `sizing-and-capacity.md`: sustained trap-rate planning, UDP receive buffer,
    CPU proportionality to trap rate, direct-journal retention/rotation, dedup
    cache sizing, OTLP queue/batch sizing, profile metric cardinality caps, and
    distributed deployment guidance.
  - `field-reference.md`: structured journal fields, profile labels, varbind
    indexing rules, decode-error rows, dedup summary rows, and OTLP field
    mapping where applicable.
  - `investigation-playbooks.md`: common NetOps/SecOps workflows such as top
    senders, critical traps, noisy categories, trap storms, decode errors, and
    before/after incident review.
  - `validation-and-data-quality.md`: missing profiles, unknown OIDs, malformed
    PDUs, duplicate storms, rate limiting, dropped entries, and how to verify
    received data quality.
  - `anti-patterns.md`: treating traps as state, alerting on every trap, using
    traps without profiles, ignoring dedup/rate limits, and expecting OTLP-only
    jobs to appear in local journal queries.
  - `troubleshooting.md`: listener startup, UDP/162 permissions, no incoming
    traps, journal write failures, OTLP export failures, profile load issues,
    retention/disk pressure, and debug commands.

Resolved page-structure refinements:

- Add `enrichment.md`. Source identity, trusted relays, reverse DNS, vendor and
  interface enrichment, vnode attribution, `TRAP_ENRICHMENT`, and `TRAP_TAG_*`
  need one clear operator-facing home.
- Add `usage-and-output.md`. This page explains what operators see in the
  existing Logs UI, `snmp:traps`, `__logs_sources`, decoded trap rows,
  decode-error rows, dedup summary rows, and built-in/profile metric charts.
- Keep security in `configuration.md` as a prominent section, with additional
  anti-pattern and playbook coverage where relevant. The docs must mention and
  link to go.d.plugin security providers for secrets and sensitive
  configuration management instead of duplicating that material.
- Keep shipped query, chart, and export workflows in
  `investigation-playbooks.md`, not in a standalone correlation page. Public
  docs may describe workflows that use shipped surfaces such as logs, journal
  queries, SIEM export, and implemented metrics when evidence supports them.
  Public docs must not mention missing topology correlation, missing Network
  Flow correlation, or other pending roadmap work.
- Define the `journal-and-querying.md` vs `forwarding-to-siem.md` boundary:
  local querying and field filtering must not be duplicated with SIEM export and
  OTLP forwarding guidance.
- Define the `configuration.md` vs `sizing-and-capacity.md` boundary:
  configuration documents syntax, defaults, and examples; sizing documents
  deployment shape, trap-rate planning, resource proportionality, and how to
  choose values.
- Define the `trap-profiles.md` vs `profile-format.md` boundary:
  `trap-profiles.md` is the operator workflow; `profile-format.md` remains the
  authoritative YAML/schema reference.
- Record that SNMP traps use the existing Logs UI and `journalctl`; there is no
  dedicated trap dashboard like Network Flows' visualizations unless the UI
  implementation changes.

Sizing-page rationale:

- A dedicated sizing page is required because SNMP trap operation exposes
  capacity decisions across several independent surfaces:
  - listener receive buffer (`receive_buffer`);
  - per-source rate limiting (`rate_limit`);
  - dedup fingerprint cache (`dedup.cache_max_entries`);
  - local direct-journal retention and rotation (`retention.*`);
  - OTLP buffering (`otlp.flush_interval`, `otlp.batch_size`,
    `otlp.queue_capacity`);
  - profile metric cardinality caps (`profile_metrics.limits.*`);
  - sustained trap-rate CPU cost and disk impact.
- These controls are too important to hide inside `configuration.md`; operators
  need one deployment-planning page similar to Network Flows sizing, but adapted
  to trap events and journal/log forwarding instead of flow rollup tiers.

Documentation analysis workflow recommendation:

- Source-of-truth hierarchy:
  1. Go source code and tests under `src/go/plugin/go.d/collector/snmp_traps/`
     are the runtime behavior source of truth.
  2. Collector consistency artifacts are the product-surface source of truth:
     `metadata.yaml`, `config_schema.json`, `charts.yaml`, `taxonomy.yaml`,
     `src/go/plugin/go.d/config/go.d/snmp_traps.conf`,
     `src/health/health.d/snmp_traps.conf`, and the generated integration page.
  3. `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` is
     the authoritative trap-profile schema reference.
  4. Durable Netdata specs under `.agents/sow/specs/snmp-traps/` explain design
     intent and accepted contracts; when they conflict with code, mark the item
     as implementation drift.
  5. Accepted architecture decisions under
     `.agents/sow/specs/snmp-traps/decisions/`, including
     `0001-go-process-and-trapwriter.md`, are internal design memory. They may
     support journal/trapwriter docs as secondary evidence, but public docs
     should cite implementation, generated collector artifacts, or shipped
     profile references first.
  6. Research files identify operator needs, industry patterns, and gaps; they
     do not prove Netdata behavior.
  7. Network Flows docs provide structure/tone patterns only.
- Phase 1: Implementation evidence inventory.
  - Build a section-by-section inventory for every proposed `docs/snmp-traps/`
    page.
  - Each inventory entry MUST use this schema:
    - page;
    - section;
    - user-facing fact to mention;
    - evidence source;
    - evidence location;
    - evidence type (`implementation`, `test`, `generated-doc`,
      `profile-reference`, `spec`, `research`, `network-flows-pattern`,
      `assumption`);
    - verification status (`proven`, `needs-runtime-verification`,
      `conflict`, `gap`);
    - security/sensitive-data notes;
    - page-boundary notes.
  - The implementation audit MUST include at least these surfaces:
    - listener and endpoint behavior: `listener.go`, `config.go`,
      `config_schema.json`;
    - job lifecycle and output backend validation: `collector.go`, `dynamic.go`;
    - journal storage and retention: `journal_writer.go`, `retention.go`,
      `retention_config.go`, `serialize.go`, `serialize_test.go`;
    - `snmp:traps` Function and `__logs_sources`: `snmptrapsfunc/func_logs.go`,
      `func_logs_test.go`;
    - OTLP export: `otlp.go`, `otlp_test.go`, `trapwriter_fanout.go`;
    - decode limits and decode errors: `decode.go`, `decode_error.go`,
      `decode_test.go`;
    - source identity, trusted relays, reverse DNS, enrichment, vnode scope:
      `enrich.go`, `source_identity.go`, `resolver.go`;
    - deduplication: `dedup.go`, `dedup_test.go`;
    - rate limiting and allowlists: `ratelimit.go`, `allowlist.go`;
    - profile loading and hot reload: `profile.go`, `load.go`,
      `profile_watcher.go`, `reload.go`;
    - profile-defined metrics and cardinality: `profile_metric.go`,
      `profile_metric_test.go`, `.agents/sow/specs/snmp-traps/trap-metrics-profiles.md`;
    - self-metrics, charts, and alerts: `metrics.go`, `charts.yaml`,
      `src/health/health.d/snmp_traps.conf`;
    - stock/operator profile pack: `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/`;
    - generated integration docs and metadata:
      `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml` and
      `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md`.
  - The inventory MUST surface top-level scoping facts explicitly:
    - direct journal storage is Linux-only;
    - OTLP-only jobs have no local direct journal and do not appear as
      `__logs_sources`;
    - the embedded `snmp:traps` Function is Cloud-required where the Function
      API marks it `RequireCloud: true`;
    - trap data can contain sensitive operational values, especially in
      `TRAP_JSON`, varbind fields, MESSAGE, and OTLP attributes.
- Phase 1.5: runtime verification of load-bearing workflows.
    - Run a local end-to-end verification or create a separate evidence SOW
      before public docs depend on unverified commands.
    - Every runtime evidence item MUST record the exact command/config,
      expected success condition, observed output/status, and sanitized example
      artifact or reason no artifact is retained.
    - Minimum runtime evidence:
      - send representative traps and query them with `journalctl --directory`
        using the effective per-job journal directory;
      - query direct-journal jobs through `snmp:traps` and verify
        `__logs_sources`;
      - verify OTLP export against a test receiver;
      - capture examples for a decoded trap, decode-error row, dedup summary,
        built-in metrics, alert-relevant metrics, and profile-defined metric.
    - Benchmark or sizing claims beyond conservative defaults require benchmark
      evidence. If evidence is missing, create a new SOW before publishing those
      claims.
  - After the first inventory is complete, run external reviewers `glm`,
    `minimax`, `kimi`, `mimo`, `deepseek`, and `qwen` against the full SOW and
    the inventory.
  - Reviewer prompt MUST ask for missing implementation facts, weak evidence,
    wrong page placement, misleading wording, security issues, unwanted side
    effects, documentation risks, and evidence that requires a separate SOW.
  - Iterate reviewers to convergence: fix or reject-with-evidence findings,
    rerun the same scope with fix notes, and repeat until no required findings
    remain.
- Phase 2: Research gap analysis.
  - Compare the Phase 1 inventory against:
    - `.agents/sow/specs/snmp-traps/research/playbooks/Skill-Distillation-SNMP-Traps-in-Network-Performance-Monitoring-NetOps-SecOps.md`;
    - `.agents/sow/specs/snmp-traps/research/playbooks/PLAYBOOK-Monitoring-SNMP-Traps-in-Modern-Enterprise-NPM-NetOps-SecOps.md`;
    - `.agents/sow/specs/snmp-traps/research/domain/snmp-traps-in-observability.md`;
    - synthesis files under `.agents/sow/specs/snmp-traps/research/comparison/`,
      especially `feature-matrix.md`, `operator-features.md`,
      `alerting-models.md`, `comparative-analysis.md`,
      `netdata-design-implications.md`, and `netdata-stress-test.md`;
    - external-system files only when the synthesis files point to a concrete
      operator need requiring source verification.
  - Before the gap table, classify research findings as:
    - Netdata-relevant operator needs;
    - generic SNMP knowledge that should be summarized or linked, not copied;
    - comparative design memory that should stay out of end-user docs unless it
      explains shipped Netdata behavior or an evidence-backed migration
      expectation.
  - Produce a research-to-doc mapping first. At minimum map:
    - mental model and maturity levels;
    - signal catalog;
    - composite failure patterns;
    - capacity and saturation;
    - operational edge cases;
    - security and integrity signals;
    - "what most teams get wrong";
    - anti-patterns;
    - SIEM/log forwarding;
    - shipped investigation workflows that use traps with logs, journal queries,
      SIEM export, and implemented metrics.
  - Treat research findings about topology correlation, Network Flow
    correlation, or other unimplemented/pending work as internal follow-up
    candidates only. Public docs MUST NOT describe these as missing product
    gaps.
  - Treat `.agents/sow/specs/snmp-traps/netdata-snmp-hub-architecture.md` as a
    target-state architecture note for hub co-location and correlation. Public
    docs MUST NOT use its topology, flow, or cross-signal correlation language
    as shipped trap behavior unless implementation evidence proves that behavior
    exists today.
  - Produce a section-by-section gap table with:
    - research/user need;
    - category (`Netdata-relevant`, `generic-SNMP`, `comparative`);
    - priority (`blocks-docs`, `enhances-docs`, `follow-up`);
    - implementation support;
    - docs placement;
    - evidence;
    - missing evidence or missing implementation;
    - disposition;
    - follow-up SOW requirement, if any.
  - Run the same external reviewers against the full SOW and the gap analysis,
    using the same iterate-to-convergence rule.
  - Create a new SOW before writing public docs that depend on:
    - benchmark or sizing data not already verified;
    - real `journalctl`, `snmp:traps`, or OTLP command behavior not yet proven;
    - screenshots or UI behavior not yet captured;
    - CVE/version/security posture claims that need external authoritative
      verification;
    - generated collector metadata corrections that remain unresolved;
    - implementation changes, not documentation changes.
  - Missing implementation evidence creates an internal follow-up candidate; it
    does not create public docs text about future or missing features.
- Phase 3: Documentation drafting and validation.
  - Draft each page from the Phase 1 inventory filtered through Phase 2
    dispositions, using the matching Network Flows page only as a tone and
    structure model.
  - Every public claim must trace back to an inventory item or a documented
    research-gap disposition.
  - Each page MUST be checked after drafting against:
    - implementation evidence;
    - collector consistency artifacts;
    - the profile-format reference;
    - research gap dispositions;
    - sensitive-data rules;
    - link and Learn map rules.
  - Collector metadata synchronization MUST be checked explicitly:
    - set `monitored_instance.link` to the published SNMP Traps documentation
      section once the page exists, or record why it remains empty;
    - update or intentionally keep
      `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml` fields such as
      `monitored_instance.link`, `related_resources`, and
      `info_provided_to_referring_integrations`;
    - update generated collector integration docs only where they should remain
      concise collector-facing entry points;
    - do not duplicate the full SNMP Traps documentation set inside generated
      integration prose.
  - This SOW does not add a Network-Flows-style `integration_placeholder` entry
    for SNMP traps. The generated collector integration remains under
    Collecting Metrics for this SOW, with manual cross-links from the public
    SNMP Traps section.
  - Run external reviewers on the completed docs set with the same full-scope
    reviewer iteration rule before the docs are considered ready.
- Recommendation: this workflow is the long-term-best route. It separates what
  Netdata actually implements from what operators may need, forces evidence
  before prose, and prevents the final documentation from becoming either
  implementation-notes-only or research-driven speculation.

## Phase 1 Implementation Evidence Inventory

Status: first external reviewer round completed. All six reviewers returned
`ACCEPT_WITH_CHANGES`; reviewer-required corrections are being folded into the
inventory before the same-scope reviewer rerun. Text-first docs may include
implementation-backed command forms, but SOW completion and merge readiness
remain blocked on Phase 1.5 runtime verification for representative trap data,
exact command output, OTLP receiver behavior, screenshots, and copied-directory
behavior.

Inventory rules:

- `proven` means the implementation, generated collector artifact, or profile
  reference supports the fact.
- `needs-runtime-verification` means the implementation supports the fact, but
  runtime output, ordering, screenshots, copied-directory behavior, or UI shape
  still need Phase 1.5 proof before SOW completion and merge readiness.
- `gap` means the fact needs new evidence or a follow-up SOW before public docs
  can make the claim.
- Public docs must describe shipped behavior only. Missing topology correlation,
  missing Network Flow correlation, richer integration search, and roadmap work
  remain internal follow-up topics.
- Reviewer round 1 found evidence-quality issues. Inventory entries below now
  prefer exact behavior lines, generated collector artifacts, and tests over
  broad source-file references.

### Cross-Page Load-Bearing Facts

- Page: all SNMP trap docs; section: output model; user-facing fact to mention:
  explicit jobs require at least one backend, direct journal is enabled by
  default, OTLP is optional, and direct journal requires Linux. Evidence source:
  `collector.go` and `config_schema.json`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:158`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:162`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:315`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:332`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: examples must not include real trap payloads,
  communities, USM keys, OTLP headers, or customer data. Page-boundary notes:
  local querying belongs in `journal-and-querying.md`; remote export belongs in
  `forwarding-to-siem.md`.
- Page: all SNMP trap docs; section: local logs access; user-facing fact to
  mention: direct-journal jobs expose the embedded `snmp:traps` logs Function,
  the Function is marked Cloud-required, and it supplies default facets, view
  keys, and `TRAP_NAME` histogram. Evidence source: `func_logs.go`; evidence
  location: `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:22`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:48`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:99`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:100`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:101`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:105`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:117`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:34`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:44`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:45`.
  Evidence type: implementation. Verification status: proven for configuration,
  needs-runtime-verification for example API payloads and UI screenshots.
  Security/sensitive-data notes: screenshots must use synthetic/sanitized trap
  data. Page-boundary notes: UI behavior goes in `usage-and-output.md`; query
  syntax goes in `journal-and-querying.md`.
- Page: all SNMP trap docs; section: direct journal model; user-facing fact to
  mention: trap entries are written to Netdata-created, systemd-journal-
  compatible files under the per-job trap directory; they are not submitted to
  the host systemd-journald daemon. Local CLI examples therefore use
  `journalctl --directory <per-job-dir>` rather than plain `journalctl`.
  Evidence source: `journal_writer.go`, `config_schema.json`, and generated
  integration doc; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:55`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:60`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:89`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:326`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:38`.
  Evidence type: implementation and generated-doc. Verification status: proven
  for path construction and `journalctl --directory` command form;
  representative command output and copied-directory behavior still need Phase
  1.5 runtime evidence before SOW completion.
  Security/sensitive-data notes: examples must use synthetic/sanitized trap
  rows. Page-boundary notes: this belongs in `journal-and-querying.md`; SIEM
  export details stay in `forwarding-to-siem.md`.
- Page: all SNMP trap docs; section: polling model; user-facing fact to
  mention: trap packet reception is event-driven; `update_every` controls the
  framework self-metrics interval and does not poll traps. Evidence source:
  `config_schema.json`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:8`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:10`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:13`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: none. Page-boundary notes: belongs in overview
  and configuration; do not imply traps are sampled at `update_every`.
- Page: all SNMP trap docs; section: profile scale; user-facing fact to mention:
  the current stock trap profile pack contains 803 profile files, 6,121 MIB
  references, and 150,755 trap OIDs. Evidence source:
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json` and the
  default profile YAML pack; evidence location: `jq 'length'` returns `803`,
  `jq '[.[].mib_count] | add'` returns `6121`, `jq '[.[].trap_count] | add'`
  returns `150755`, and `find .../default -name '*.yaml' | wc -l` returns
  `803`. Evidence type: profile-reference. Verification status: proven.
  Security/sensitive-data notes: none. Page-boundary notes: public docs may
  state the scale, but richer vendor/MIB/trap search remains future internal
  work and must not be promised by this SOW. The count describes the bundled
  stock pack, not a guarantee that arbitrary vendor traps are already covered;
  link unsupported/override workflows to `profile-format.md`.
- Page: all SNMP trap docs; section: sensitive values; user-facing fact to
  mention: `snmpTrapCommunity` is skipped from indexed varbind fields and
  `TRAP_JSON`; `sysUpTime`, `snmpTrapOID`, `snmpTrapAddress`, and
  `snmpTrapEnterprise` are also skipped from indexed `TRAP_VAR_*` fields as
  redundant protocol varbinds; other varbinds, rendered `MESSAGE`, `TRAP_JSON`,
  and OTLP attributes can still contain sensitive operational values. Evidence source:
  `serialize.go`, `otlp.go`, and query skill safety notes; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:226`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:231`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:239`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:406`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:482`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:779`,
  `docs/netdata-ai/skills/query-snmp-traps/SKILL.md:52`. Evidence type:
  implementation. Verification status: proven. Security/sensitive-data notes:
  docs must warn before export/sharing examples; do not imply complete
  redaction. Page-boundary notes: main warning in `configuration.md`,
  repeated where exporting/querying is shown.
- Page: all SNMP trap docs; section: journal field safety; user-facing fact to
  mention: non-`MESSAGE` fields containing newline, NUL, DEL, invalid UTF-8, or
  other unsafe control bytes are binary-encoded for journal safety; this protects
  against field injection but does not make values non-sensitive. Evidence
  source: `cwe117.go`, `journal_writer.go`, and tests; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/cwe117.go:7`,
  `src/go/plugin/go.d/collector/snmp_traps/cwe117.go:39`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:181`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:216`,
  `src/go/plugin/go.d/collector/snmp_traps/cwe117_test.go:25`,
  `src/go/plugin/go.d/collector/snmp_traps/cwe117_test.go:82`.
  Evidence type: implementation and test. Verification status: proven; runtime
  examples need Phase 1.5 if public docs show an encoded-field row.
  Security/sensitive-data notes: document as injection protection, not
  redaction. Page-boundary notes: field details belong in `field-reference.md`;
  troubleshooting explains binary-encoded field metrics.
- Page: all SNMP trap docs; section: SNMPv3 INFORM state; user-facing fact to
  mention: v3 INFORM support uses a receiver-local engine ID and persisted
  engine-boots state under the Netdata lib directory; omitted `local_engine_id`
  is generated and persisted per job. Evidence source: `config_schema.json`,
  `engineboots.go`, `inform.go`, and tests; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:165`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:167`,
  `src/go/plugin/go.d/collector/snmp_traps/engineboots.go:26`,
  `src/go/plugin/go.d/collector/snmp_traps/engineboots.go:77`,
  `src/go/plugin/go.d/collector/snmp_traps/engineboots.go:90`,
  `src/go/plugin/go.d/collector/snmp_traps/engineboots.go:189`,
  `src/go/plugin/go.d/collector/snmp_traps/engineboots.go:200`,
  `src/go/plugin/go.d/collector/snmp_traps/engineboots.go:213`,
  `src/go/plugin/go.d/collector/snmp_traps/inform.go:14`,
  `src/go/plugin/go.d/collector/snmp_traps/local_engine_id_test.go:57`,
  `src/go/plugin/go.d/collector/snmp_traps/local_engine_id_test.go:77`,
  `src/go/plugin/go.d/collector/snmp_traps/local_engine_id_test.go:289`.
  Evidence type: implementation and test. Verification status: proven;
  end-to-end INFORM packet behavior still needs Phase 1.5 runtime evidence if
  public docs include a copyable INFORM example. Security/sensitive-data notes:
  examples must not include real SNMPv3 user names or keys. Page-boundary notes:
  setup belongs in `configuration.md`; troubleshooting covers unknown engine ID
  and INFORM response failures.
- Page: all SNMP trap docs; section: output fanout; user-facing fact to mention:
  when journal and OTLP are both enabled, journal is the primary writer and OTLP
  is secondary; secondary OTLP write failures increment OTLP error metrics but
  do not roll back a successful primary journal write. When OTLP is the only
  enabled backend, OTLP is the authoritative output and OTLP export failures are
  terminal write failures that increment pipeline/source write-failure metrics
  when the source is known; internally this is controlled by the OTLP writer's
  `terminalErrors` flag, set when no journal writer exists. Evidence source:
  `collector.go`,
  `trapwriter_fanout.go`, `otlp.go`, `metrics.go`, and generated integration
  doc; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:323`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:326`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:334`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:341`,
  `src/go/plugin/go.d/collector/snmp_traps/trapwriter_fanout.go:13`,
  `src/go/plugin/go.d/collector/snmp_traps/trapwriter_fanout.go:27`,
  `src/go/plugin/go.d/collector/snmp_traps/trapwriter_fanout.go:54`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:87`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:106`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:513`,
  `src/go/plugin/go.d/collector/snmp_traps/metrics.go:559`,
  `src/go/plugin/go.d/collector/snmp_traps/metrics.go:664`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:572`.
  Evidence type: implementation. Verification status: proven; OTLP failure
  example needs runtime verification. Security/sensitive-data notes: remote
  export can expose trap payloads. Page-boundary notes: configuration states
  semantics; forwarding page gives SIEM/export guidance.

### Page Inventory

- Page: `docs/snmp-traps/README.md`; section: overview; user-facing fact to
  mention: the collector is event-driven, receives UDP trap/INFORM PDUs, validates
  source/version/auth, resolves profiles, enriches entries, writes configured
  outputs, then updates self-metrics. Evidence source: `collector.go` and
  generated integration doc; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:482`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:502`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:622`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:637`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:666`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:677`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:80`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: use documentation IPs and synthetic names only.
  Page-boundary notes: do not present traps as polled state.
- Page: `docs/snmp-traps/README.md`; section: capabilities; user-facing fact to
  mention: Netdata supports profile-based decode/classification/rendering,
  direct journal storage, OTLP export, deduplication, profile-defined metrics,
  and self-metrics. Evidence source: generated integration doc and profile
  reference; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:31`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:34`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:35`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:16`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:204`.
  Evidence type: generated-doc. Verification status: proven.
  Security/sensitive-data notes: none. Page-boundary notes: the overview should
  point to task pages instead of becoming a config reference.

- Page: `docs/snmp-traps/installation.md`; section: listener setup;
  user-facing fact to mention: SNMP trap collection is not autodetected and an
  explicit `snmp_traps` job binds UDP endpoints, defaulting to
  `udp://0.0.0.0:162` with a 4 MiB receive buffer. Evidence source:
  stock configuration, `config_schema.json`, `collector.go`, `listener.go`, and
  generated integration doc; evidence location:
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:4`,
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:11`,
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:18`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:20`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:72`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:78`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:56`,
  `src/go/plugin/go.d/collector/snmp_traps/listener.go:40`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:114`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: default examples should include allowlists or
  explicitly call out open defaults. Page-boundary notes: package-specific
  commands need runtime validation before publication.
- Page: `docs/snmp-traps/installation.md`; section: permissions;
  user-facing fact to mention: binding UDP/162 requires
  `CAP_NET_BIND_SERVICE` or root, and Netdata packages grant this capability to
  `go.d.plugin`. Evidence source: `config_schema.json` and generated
  integration doc; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:72`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:106`.
  Evidence type: generated-doc. Verification status: proven.
  Security/sensitive-data notes: no raw secrets. Page-boundary notes: keep
  Linux service details here, not in quick start.
- Page: `docs/snmp-traps/installation.md`; section: platform behavior;
  user-facing fact to mention: direct journal startup validates the Netdata log
  root and the journal backend requires Linux. Evidence source: `collector.go`
  and `journal_writer.go`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:162`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:227`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:74`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: path examples must avoid private hostnames.
  Page-boundary notes: journal directory querying belongs in
  `journal-and-querying.md`.

- Page: `docs/snmp-traps/quick-start.md`; section: first job;
  user-facing fact to mention: the stock `snmp_traps.conf` is a commented
  template, so the quick start must tell users to create or uncomment an
  explicit job. A minimal job defaults to versions `v1` and `v2c`, an empty
  community list accepts all communities, and the default source allowlist is
  open to IPv4 and IPv6 unless narrowed. Evidence source: stock configuration,
  `collector.go`, `config_schema.json`, `allowlist.go`, and `init.go`; evidence
  location:
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:4`,
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:19`,
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:22`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:60`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:87`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:91`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:98`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:100`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:201`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:205`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:210`,
  `src/go/plugin/go.d/collector/snmp_traps/allowlist.go:15`,
  `src/go/plugin/go.d/collector/snmp_traps/init.go:22`,
  `src/go/plugin/go.d/collector/snmp_traps/init.go:248`,
  `src/go/plugin/go.d/collector/snmp_traps/init.go:249`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: recommended quick-start examples should use
  non-secret placeholder communities and source CIDR restrictions. Page-boundary
  notes: full security discussion stays in `configuration.md`.
- Page: `docs/snmp-traps/quick-start.md`; section: verify receipt;
  user-facing fact to mention: verify a received trap through `snmp:traps`, the
  direct journal, and receiver metrics. Evidence source: `func_logs.go`,
  `journal_writer.go`, and `charts.yaml`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:78`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:63`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:336`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:55`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:64`.
  Evidence type: implementation. Verification status:
  needs-runtime-verification for exact commands and example output.
  Security/sensitive-data notes: sample trap data must be synthetic. Page-boundary
  notes: do not add screenshots until the populated-journal pass.

- Page: `docs/snmp-traps/configuration.md`; section: option map;
  user-facing fact to mention: document all job-level option groups:
  `vnode`, `listen`, `versions`, `communities`, `usm_users`,
  `engine_id_whitelist`, `local_engine_id`, `dynamic_engine_id_discovery`,
  `dynamic_engine_id_max_pairs`, reverse DNS, allowlist, trusted relays, rate
  limit, dedup, journal, OTLP, retention, overrides, and `profile_metrics`
  including `identity` and `limits`. Evidence source: `config.go` and
  `config_schema.json`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/config.go:72`,
  `src/go/plugin/go.d/collector/snmp_traps/config.go:151`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:15`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:165`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:171`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:177`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:221`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:506`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:540`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: call out which values can be secret
  references. Page-boundary notes: configuration page covers syntax/defaults;
  sizing page covers how to choose values.
- Page: `docs/snmp-traps/configuration.md`; section: SNMPv3 security;
  user-facing fact to mention: SNMPv3 requires at least one `usm_users` entry
  and either static `engine_id_whitelist` or dynamic engine ID discovery;
  `auth_key` and `priv_key` are string fields whose shipped descriptions and
  stock examples tell operators to use Netdata secret references. Public docs
  should link to Secrets Management for resolver syntax and providers instead
  of treating the SNMP trap schema as the secret-provider specification.
  Evidence source: `collector.go`, `config_schema.json`, stock configuration,
  the Learn map, and Secrets Management reference; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:172`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:110`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:136`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:147`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:154`,
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:24`,
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:30`,
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:32`,
  `docs/.map/map.yaml:444`, `src/collectors/SECRETS.md:16`,
  `src/collectors/SECRETS.md:17`, `src/collectors/SECRETS.md:18`,
  `src/collectors/SECRETS.md:19`. Evidence type:
  implementation. Verification status: proven. Security/sensitive-data notes:
  link to Secrets Management; do not duplicate provider docs. Page-boundary
  notes: anti-patterns should warn against plaintext secrets.
- Page: `docs/snmp-traps/configuration.md`; section: SNMPv3 dynamic engine ID
  discovery; user-facing fact to mention: dynamic sender engine ID discovery is
  opt-in, requires an empty static engine whitelist, registers only authenticated
  non-INFORM v3 Trap `(engineID, username)` pairs, stores registrations in
  memory only, caps them at `dynamic_engine_id_max_pairs` (default 4096), and
  intentionally increments `unknown_engine_id` on the first accepted dynamic
  registration for operator visibility. Evidence source: `config_schema.json`,
  stock configuration, `collector.go`, `dynamic.go`, `init.go`, and generated
  integration doc; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:171`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:173`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:177`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:179`,
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:36`,
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:41`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:360`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:377`,
  `src/go/plugin/go.d/collector/snmp_traps/dynamic.go:20`,
  `src/go/plugin/go.d/collector/snmp_traps/dynamic.go:73`,
  `src/go/plugin/go.d/collector/snmp_traps/dynamic.go:135`,
  `src/go/plugin/go.d/collector/snmp_traps/dynamic.go:159`,
  `src/go/plugin/go.d/collector/snmp_traps/dynamic.go:165`,
  `src/go/plugin/go.d/collector/snmp_traps/dynamic.go:168`,
  `src/go/plugin/go.d/collector/snmp_traps/init.go:283`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:723`.
  Evidence type: implementation. Verification status: proven; runtime examples
  need Phase 1.5 if public docs include a working dynamic-discovery scenario.
  Security/sensitive-data notes: examples must not include real USM names or
  engine IDs from customer devices. Page-boundary notes: sizing covers the
  4096-pair cap; troubleshooting covers repeated `unknown_engine_id` increments.
- Page: `docs/snmp-traps/configuration.md`; section: source controls;
  user-facing fact to mention: source CIDR allowlists are pre-decode; trusted
  relays are the only peers allowed to override source identity via
  `snmpTrapAddress.0`; catch-all trusted relays are warned about. Evidence
  source: `config_schema.json`, `collector.go`, and `allowlist.go`; evidence
  location:
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:201`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:221`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:502`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:715`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:765`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: examples must not normalize open trusted relay
  configs. Page-boundary notes: enrichment page explains resulting source fields.
- Page: `docs/snmp-traps/configuration.md`; section: storm controls;
  user-facing fact to mention: rate limiting is optional and off by default,
  defaults to 1000 PPS per source in `drop` mode when enabled, and dedup is
  optional/off by default with a 5 second window and 100,000 fingerprint cache.
  Evidence source: `ratelimit.go`, `dedup.go`, and `config_schema.json`;
  evidence location: `src/go/plugin/go.d/collector/snmp_traps/ratelimit.go:14`,
  `src/go/plugin/go.d/collector/snmp_traps/ratelimit.go:18`,
  `src/go/plugin/go.d/collector/snmp_traps/ratelimit.go:39`,
  `src/go/plugin/go.d/collector/snmp_traps/dedup.go:19`,
  `src/go/plugin/go.d/collector/snmp_traps/dedup.go:83`,
  `src/go/plugin/go.d/collector/snmp_traps/dedup.go:90`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:241`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:274`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: none. Page-boundary notes: detailed capacity
  guidance goes in `sizing-and-capacity.md`.
- Page: `docs/snmp-traps/configuration.md`; section: output backends;
  user-facing fact to mention: direct journal stores local structured trap logs
  under the Netdata log directory, OTLP exports OTLP/gRPC LogRecords, the
  default OTLP endpoint is `http://127.0.0.1:4317`, and both outputs can be
  enabled at once. For OTLP-only operation, set `journal.enabled: false`
  because journal output is enabled by default for explicit jobs. OTLP endpoint
  scheme controls transport: bare `host:port` and `http://` use plaintext gRPC;
  `https://` uses TLS with system trust roots; plaintext loopback targets do not
  trigger the remote-plaintext warning. Evidence source: stock configuration,
  `config_schema.json`, `collector.go`, `otlp.go`, and `trapwriter_fanout.go`;
  evidence location:
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:62`,
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:69`,
  `src/go/plugin/go.d/config/go.d/snmp_traps.conf:71`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:315`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:326`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:332`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:338`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:353`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:357`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:323`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:334`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:758`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:30`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:87`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:106`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:177`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:188`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:212`,
  `src/go/plugin/go.d/collector/snmp_traps/trapwriter_fanout.go:13`,
  `src/go/plugin/go.d/collector/snmp_traps/trapwriter_fanout.go:27`,
  `src/go/plugin/go.d/collector/snmp_traps/trapwriter_fanout.go:54`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: OTLP headers may use secret references; remote
  plaintext OTLP is warned. Page-boundary notes: SIEM/export examples belong in
  `forwarding-to-siem.md`.

- Page: `docs/snmp-traps/trap-profiles.md`; section: profile purpose;
  user-facing fact to mention: trap profiles are declarative YAML files that map
  trap OIDs, varbinds, category, severity, message rendering, and optional
  metric rules without source-code changes. Evidence source: profile-format
  reference; evidence location:
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:7`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:16`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:24`.
  Evidence type: profile-reference. Verification status: proven.
  Security/sensitive-data notes: label examples must avoid user/customer
  identifiers. Page-boundary notes: link to `profile-format.md` for schema, do
  not duplicate it fully.
- Page: `docs/snmp-traps/trap-profiles.md`; section: locations/loading;
  user-facing fact to mention: stock profiles live under
  `snmp.trap-profiles/default/`, user profiles under `snmp.trap-profiles/`,
  stock profiles are lazy-loaded, and user profile changes are watched/reloaded.
  Evidence source: `load.go`, `profile_watcher.go`, and profile-format
  reference; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/load.go:38`,
  `src/go/plugin/go.d/collector/snmp_traps/load.go:71`,
  `src/go/plugin/go.d/collector/snmp_traps/load.go:103`,
  `src/go/plugin/go.d/collector/snmp_traps/profile_watcher.go:46`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:37`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: none. Page-boundary notes: custom MIB
  conversion commands need runtime/package-path validation before publication.
- Page: `docs/snmp-traps/trap-profiles.md`; section: custom profile workflow;
  user-facing fact to mention: operators with device-specific MIBs can use the
  installed `snmp-trap-profile-gen` helper to generate user profile YAML under
  `/etc/netdata/go.d/snmp.trap-profiles/`; stock OOB pack regeneration remains
  an internal maintainer workflow unless Phase 2 explicitly decides to document
  it. Evidence source: profile-format reference and project trap-profile
  authoring skill; evidence location:
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:37`,
  `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md:235`,
  `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md:250`,
  `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md:261`.
  Evidence type: profile-reference and runtime project skill. Verification
  status: proven for workflow shape, needs-runtime-verification for installed
  package paths and copyable commands. Security/sensitive-data notes: sample
  MIB paths must not contain customer or vendor-private path names.
  Page-boundary notes: do not turn public docs into stock-pack maintainer docs
  unless Phase 2 finds an operator need.
- Page: `docs/snmp-traps/trap-profiles.md`; section: taxonomy and overrides;
  user-facing fact to mention: categories and severities are closed 8-value sets,
  and jobs can override category, severity, and labels per OID. Evidence source:
  `profile.go`, `config_schema.json`, and generated integration doc; evidence
  location: `src/go/plugin/go.d/collector/snmp_traps/profile.go:17`,
  `src/go/plugin/go.d/collector/snmp_traps/profile.go:28`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:434`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:40`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: profile labels emit `TRAP_TAG_*`; avoid
  high-cardinality/sensitive labels. Page-boundary notes: profile authoring
  details stay in the profile-format reference.

- Page: `docs/snmp-traps/enrichment.md`; section: identity;
  user-facing fact to mention: Netdata starts from packet source identity,
  accepts relay-provided original source only from configured trusted relays,
  and records enrichment decisions in `TRAP_ENRICHMENT`. Evidence source:
  `decode.go`, `collector.go`, `enrich.go`, and `serialize.go`; evidence
  location: `src/go/plugin/go.d/collector/snmp_traps/decode.go:104`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:521`,
  `src/go/plugin/go.d/collector/snmp_traps/enrich.go:313`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:163`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: source values can identify networks; examples
  must use documentation IPs. Page-boundary notes: do not document missing or
  future topology workflows.
- Page: `docs/snmp-traps/enrichment.md`; section: reverse DNS;
  user-facing fact to mention: reverse DNS is optional/off by default, cached,
  best-effort, and emitted as `TRAP_REVERSE_DNS` without overriding
  authoritative identity. Evidence source: `config_schema.json` and `enrich.go`;
  evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:184`,
  `src/go/plugin/go.d/collector/snmp_traps/enrich.go:40`,
  `src/go/plugin/go.d/collector/snmp_traps/enrich.go:80`,
  `src/go/plugin/go.d/collector/snmp_traps/enrich.go:442`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: PTR names can reveal internal hostnames.
  Page-boundary notes: security warning links back to configuration.
- Page: `docs/snmp-traps/enrichment.md`; section: device/interface fields;
  user-facing fact to mention: when registry or available enrichment data is
  unambiguous, Netdata can emit `_HOSTNAME`, `TRAP_DEVICE_VENDOR`,
  `ND_NIDL_NODE`, `TRAP_INTERFACE`, and `TRAP_NEIGHBORS`; conflicts or ambiguity
  are audited instead of silently replacing identity. Evidence source:
  `enrich.go` and `serialize.go`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/enrich.go:283`,
  `src/go/plugin/go.d/collector/snmp_traps/enrich.go:327`,
  `src/go/plugin/go.d/collector/snmp_traps/enrich.go:355`,
  `src/go/plugin/go.d/collector/snmp_traps/enrich.go:367`,
  `src/go/plugin/go.d/collector/snmp_traps/enrich.go:379`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:84`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:128`.
  Evidence type: implementation. Verification status: proven for emitted fields,
  needs-runtime-verification for screenshots/examples. Security/sensitive-data
  notes: hostnames and interface names may reveal topology. Page-boundary notes:
  public docs must describe the fields, not missing topology correlation.
- Page: `docs/snmp-traps/enrichment.md`; section: profile metric identity;
  user-facing fact to mention: profile metrics can use vnode host scope when
  enrichment is unambiguous, or bounded `source_id`/`source_kind` labels with a
  local hash by default. Evidence source: `source_identity.go` and
  profile-format reference; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/source_identity.go:28`,
  `src/go/plugin/go.d/collector/snmp_traps/source_identity.go:89`,
  `src/go/plugin/go.d/collector/snmp_traps/source_identity.go:122`,
  `src/go/plugin/go.d/collector/snmp_traps/source_identity.go:135`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:256`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: explain `raw` source labels only for accepted
  environments. Page-boundary notes: full `profile_metrics` config stays in
  `configuration.md`.

- Page: `docs/snmp-traps/usage-and-output.md`; section: Logs UI;
  user-facing fact to mention: `snmp:traps` exposes default facets
  `TRAP_CATEGORY`, `TRAP_DEVICE_VENDOR`, `TRAP_NAME`, `TRAP_SEVERITY`,
  `TRAP_SOURCE_IP`, `_HOSTNAME`, and `TRAP_JOB`, with default view keys and
  `TRAP_NAME` histogram. Evidence source: `func_logs.go`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:99`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:100`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:101`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:105`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:117`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:44`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:45`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:126`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:127`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:128`.
  Evidence type: implementation. Verification status: proven for defaults,
  needs-runtime-verification for UI screenshots. Security/sensitive-data notes:
  screenshots must use synthetic trap data. Page-boundary notes: no dedicated
  trap visualization page unless implementation changes.
- Page: `docs/snmp-traps/usage-and-output.md`; section: row types;
  user-facing fact to mention: rows can be normal traps, decode-error rows, or
  dedup summary rows. Evidence source: `serialize.go`, `decode_error.go`, and
  `dedup.go`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:94`,
  `src/go/plugin/go.d/collector/snmp_traps/decode_error.go:53`,
  `src/go/plugin/go.d/collector/snmp_traps/dedup.go:239`.
  Evidence type: implementation. Verification status: proven; example rows need
  runtime verification. Security/sensitive-data notes: decode-error rows contain
  SHA256 and sanitized error text, not raw packet bytes. Page-boundary notes:
  field definitions stay in `field-reference.md`.
- Page: `docs/snmp-traps/usage-and-output.md`; section: charts;
  user-facing fact to mention: users can inspect built-in receiver charts and,
  when enabled, profile-derived charts. Evidence source: `charts.yaml` and
  profile-format reference; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:64`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:87`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:112`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:137`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:204`.
  Evidence type: implementation. Verification status: proven; chart screenshots
  need runtime verification. Security/sensitive-data notes: profile chart labels
  may expose source identities if configured raw. Page-boundary notes: alert
  semantics stay in `metrics-and-alerts.md`.

- Page: `docs/snmp-traps/journal-and-querying.md`; section: storage path;
  user-facing fact to mention: direct journals are written under
  `${NETDATA_LOG_DIR}/traps/<job>` or `/var/log/netdata/traps/<job>` by default,
  with the effective directory returned by the journal writer. Evidence source:
  `journal_writer.go` and `config_schema.json`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:55`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:60`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:64`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:89`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:315`.
  Evidence type: implementation. Verification status: proven; exact
  `journalctl --directory` examples need runtime verification.
  Security/sensitive-data notes: do not show real journal payloads. Page-boundary
  notes: SIEM export details belong in `forwarding-to-siem.md`.
- Page: `docs/snmp-traps/journal-and-querying.md`; section: field filtering;
  user-facing fact to mention: fields include `MESSAGE`, `PRIORITY`,
  `_HOSTNAME`, `ND_LOG_SOURCE`, `TRAP_*`, `TRAP_TAG_*`, indexed `TRAP_VAR_*`,
  `TRAP_ENRICHMENT`, and `TRAP_JSON`. Evidence source: `serialize.go`; evidence
  location: `src/go/plugin/go.d/collector/snmp_traps/serialize.go:77`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:84`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:88`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:98`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:149`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:159`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:163`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:171`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: warn that `TRAP_JSON` and varbind fields may
  include operationally sensitive values. Page-boundary notes: full field table
  goes in `field-reference.md`.
- Page: `docs/snmp-traps/journal-and-querying.md`; section: local vs OTLP-only;
  user-facing fact to mention: OTLP-only jobs do not create local journal files
  and do not appear as local log sources. The `snmp:traps` Function surface is
  still registered, but local requests with no direct-journal source follow the
  documented 503 "direct journal output has no sources" path. Evidence source:
  generated integration doc, output-backend implementation, and Function tests;
  evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:38`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:158`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:334`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:20`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:29`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:34`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:138`.
  Evidence type: generated-doc. Verification status: proven for behavior,
  needs-runtime-verification for Function examples. Security/sensitive-data
  notes: none. Page-boundary notes: this is a shipped-feature limitation and may
  be documented because it affects local querying.

- Page: `docs/snmp-traps/forwarding-to-siem.md`; section: journal-native
  workflows; user-facing fact to mention: direct journal storage is compatible
  with journal methodologies such as dumping/copying/querying local journal
  files; `journalctl --directory` command forms are backed by implementation
  evidence and must be runtime-proven before SOW completion. Direct journal
  creation also depends on usable machine ID and boot ID values because the
  writer uses systemd-journal-compatible metadata.
  Evidence source: `journal_writer.go`, `func_logs.go`, tests, and user
  requirement; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:55`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:89`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:99`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:78`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:63`.
  Evidence type: implementation and test. Verification status: proven for
  command form and output path construction; needs-runtime-verification for
  representative command output, ordering, `_BOOT_ID`, and copied-directory
  behavior. Security/sensitive-data notes: SIEM examples must use synthetic
  payloads and warn about varbind data.
  Page-boundary notes: keep local querying in `journal-and-querying.md`; this
  page focuses on export/integration.
- Page: `docs/snmp-traps/forwarding-to-siem.md`; section: OTLP export;
  user-facing fact to mention: OTLP exports traps as LogRecords with resource
  attributes `service.name=netdata-snmptrap` and `service.instance.id=<job>`,
  severity mapping, event names, trap attributes, varbinds, labels, and dedup or
  decode-error details. Evidence source: `otlp.go`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:35`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:564`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:574`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:578`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:579`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:589`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:611`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:632`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:652`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:696`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:700`.
  Evidence type: implementation. Verification status: proven; receiver example
  needs runtime verification. Security/sensitive-data notes: remote endpoints
  should use TLS; headers may use secret references. Page-boundary notes: do not
  imply OTLP is a replacement for local Logs UI unless journal is disabled by
  design.

- Page: `docs/snmp-traps/metrics-and-alerts.md`; section: built-in charts;
  user-facing fact to mention: built-in charts cover `pipeline`, `events`,
  `severity`, `errors`, `dedup_suppressed`, `sources`, `source_attribution`,
  `source_pipeline`, `source_errors`, and `source_last_seen`. Built-in source
  receiver metrics are bounded separately from profile metrics: 2,000 active
  sources per job and inactive-source expiry after 60 successful collection
  cycles. Evidence source: `metrics.go`, `charts.yaml`, and generated
  integration doc; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/metrics.go:14`,
  `src/go/plugin/go.d/collector/snmp_traps/metrics.go:15`,
  `src/go/plugin/go.d/collector/snmp_traps/metrics.go:16`,
  `src/go/plugin/go.d/collector/snmp_traps/metrics.go:64`,
  `src/go/plugin/go.d/collector/snmp_traps/metrics.go:478`,
  `src/go/plugin/go.d/collector/snmp_traps/metrics.go:647`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:64`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:87`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:112`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:137`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:176`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:187`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:198`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:219`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:236`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:255`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:566`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:570`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: source labels may be sensitive. Page-boundary
  notes: examples of interpreting charts can be shared with playbooks.
- Page: `docs/snmp-traps/metrics-and-alerts.md`; section: profile metrics;
  user-facing fact to mention: profile metrics are off by default and have
  selection modes `none`, `auto`, `exact`, and `combined`. Evidence source:
  `config_schema.json`, `profile_metric.go`, and profile-format reference;
  evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:473`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:478`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:479`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:491`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:493`,
  `src/go/plugin/go.d/collector/snmp_traps/profile_metric.go:387`,
  `src/go/plugin/go.d/collector/snmp_traps/profile_metric.go:457`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:238`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: raw `source_id` can expose source values.
  Page-boundary notes: explain concepts here; schema stays in
  `profile-format.md`.
- Page: `docs/snmp-traps/metrics-and-alerts.md`; section: profile metrics;
  user-facing fact to mention: profile metrics are updated only after the
  configured writer path accepts the trap. Dedup-suppressed traps and
  synchronous write-failed traps do not update profile metrics; OTLP-only async
  export-failure behavior requires Phase 1.5 runtime verification before public
  docs state exact profile-metric side effects. Evidence source:
  `collector.go`, `otlp.go`, generated integration doc, and profile-format
  reference; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:666`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:677`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:399`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:513`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:572`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:418`.
  Evidence type: implementation. Verification status: proven for synchronous
  write path; needs-runtime-verification for OTLP-only async export failure
  examples and profile-metric side effects. Security/sensitive-data notes: raw
  `source_id` can expose source values. Page-boundary notes: explain concepts
  here; schema stays in `profile-format.md`.
- Page: `docs/snmp-traps/metrics-and-alerts.md`; section: default alerts;
  user-facing fact to mention: default alerts cover high severities, decode and
  template errors, malformed PDUs, allowlist drops, rate limits, auth/USM/engine
  issues, INFORM response failures, binary-encoded fields, profile load,
  journal write, OTLP export, listener read, and dedup storm symptoms. Evidence
  source: `src/health/health.d/snmp_traps.conf`; evidence location:
  `src/health/health.d/snmp_traps.conf:5`,
  `src/health/health.d/snmp_traps.conf:81`,
  `src/health/health.d/snmp_traps.conf:153`,
  `src/health/health.d/snmp_traps.conf:195`,
  `src/health/health.d/snmp_traps.conf:209`,
  `src/health/health.d/snmp_traps.conf:237`,
  `src/health/health.d/snmp_traps.conf:252`,
  `src/health/health.d/snmp_traps.conf:280`. Evidence type:
  implementation. Verification status: proven. Security/sensitive-data notes:
  no real trap samples in alert examples. Page-boundary notes: alert tuning is
  not a profile authoring topic. Phase 3 drafting must extract and list the
  actual health template names, because operators use template names for
  silencing, notification routing, and configuration review. Alert mapping to
  preserve in `metrics-and-alerts.md`:
  `snmp_trap_emergency_events` -> `snmp.trap.severity`/`emerg`;
  `snmp_trap_alert_events` -> `snmp.trap.severity`/`alert`;
  `snmp_trap_critical_events` -> `snmp.trap.severity`/`crit`;
  `snmp_trap_error_events` -> `snmp.trap.severity`/`err`;
  `snmp_trap_warning_event_storm` -> `snmp.trap.severity`/`warning`;
  `snmp_trap_decode_errors` -> `snmp.trap.errors`/`decode_failed`;
  `snmp_trap_template_unresolved` -> `snmp.trap.errors`/`template_unresolved`;
  `snmp_trap_malformed_pdus` -> `snmp.trap.errors`/`malformed_pdu`;
  `snmp_trap_allowlist_drops` -> `snmp.trap.errors`/`dropped_allowlist`;
  `snmp_trap_rate_limited` -> `snmp.trap.errors`/`rate_limited`;
  `snmp_trap_auth_failures` -> `snmp.trap.errors`/`auth_failures`;
  `snmp_trap_usm_failures` -> `snmp.trap.errors`/`usm_failures`;
  `snmp_trap_unknown_engine_id` -> `snmp.trap.errors`/`unknown_engine_id`;
  `snmp_trap_inform_response_failures` ->
  `snmp.trap.errors`/`inform_response_failed`;
  `snmp_trap_binary_encoded_fields` -> `snmp.trap.errors`/`binary_encoded`;
  `snmp_trap_profile_load_failures` -> `snmp.trap.errors`/`profile_load_failed`;
  `snmp_trap_journal_write_failures` ->
  `snmp.trap.errors`/`journal_write_failed`;
  `snmp_trap_otlp_export_failures` -> `snmp.trap.errors`/`otlp_export_failed`;
  `snmp_trap_listener_read_failures` ->
  `snmp.trap.errors`/`listener_read_failed`;
  `snmp_trap_high_dedup_suppression` ->
  `snmp.trap.dedup_suppressed`/`suppressed`.

- Page: `docs/snmp-traps/sizing-and-capacity.md`; section: listener limits;
  user-facing fact to mention: the listener reads datagrams up to 8 KiB plus one
  classification byte, defaults the UDP receive buffer to 4 MiB, and caps the
  requested receive buffer at 256 MiB. Evidence source: `listener.go`,
  `decode.go`, and `config_schema.json`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/listener.go:15`,
  `src/go/plugin/go.d/collector/snmp_traps/listener.go:87`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:305`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:36`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:41`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: none. Page-boundary notes: do not publish
  throughput claims without benchmark evidence.
- Page: `docs/snmp-traps/sizing-and-capacity.md`; section: decode limits;
  user-facing fact to mention: decode limits include 256 varbinds, BER nesting
  depth 8, BER length-field width 4 bytes, encoded OID length 128 bytes, and
  OctetString length 1024 bytes for relevant decoded values. Evidence source:
  `decode.go`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:36`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:37`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:38`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:39`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:40`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:331`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:410`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:645`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:655`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: none. Page-boundary notes: exact performance
  impact remains evidence-gated. Do not add wall-clock decode-deadline claims
  unless runtime or benchmark evidence proves them.
- Page: `docs/snmp-traps/sizing-and-capacity.md`; section: decode limits;
  user-facing fact to mention: the previous generated metadata, generated
  integration page, and durable Netdata trap spec contained an unsupported
  "decode time at 1 ms per PDU" / per-PDU decode-budget claim. The claim was
  removed in this SOW because current implementation evidence reviewed for this
  SOW did not find a decode deadline or timeout. Public docs must state only the
  implemented decoder guardrails above unless future benchmark/runtime evidence
  proves a time-bound behavior. Evidence source: generated metadata, generated
  integration doc, durable Netdata spec, and `decode.go`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml:119`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:119`,
  `.agents/sow/specs/snmp-traps/netdata.md:1264`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:69`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:73`,
  `src/go/plugin/go.d/collector/snmp_traps/decode.go:304`.
  Evidence type: generated-doc and implementation. Verification status:
  corrected. Security/sensitive-data notes: none. Page-boundary notes: Phase 3
  validation must grep public docs and generated integration docs to ensure the
  unsupported time-bound claim is not reintroduced.
- Page: `docs/snmp-traps/sizing-and-capacity.md`; section: buffering and
  retention; user-facing fact to mention: journal writer queue defaults to
  10,000 entries, flushes by 1,000 entries or 1 second, retention defaults to
  decimal 10 GB, and OTLP defaults to 512-record batches, 200 ms flush, and a
  10,000-record queue.
  Evidence source: `trapwriter_impl.go`, `retention.go`, `otlp.go`, and
  `config_schema.json`; evidence
  location:
  `src/go/plugin/go.d/collector/snmp_traps/trapwriter_impl.go:13`,
  `src/go/plugin/go.d/collector/snmp_traps/trapwriter_impl.go:47`,
  `src/go/plugin/go.d/collector/snmp_traps/retention.go:15`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:29`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:396`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: disk sizing examples must avoid real volumes.
  Page-boundary notes: benchmark-based sizing is a gap unless separate evidence
  is produced.
- Page: `docs/snmp-traps/sizing-and-capacity.md`; section: state caps;
  user-facing fact to mention: dynamic SNMPv3 engine ID discovery keeps
  in-memory `(engineID, username)` registrations capped by
  `dynamic_engine_id_max_pairs` with a default of 4096, and the internal
  rate-limit bucket table is capped at 10,000 sources with idle/oldest eviction.
  Evidence source: `init.go`, `collector.go`, `config_schema.json`,
  `dynamic.go`, and `ratelimit.go`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/init.go:19`,
  `src/go/plugin/go.d/collector/snmp_traps/init.go:289`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:363`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:177`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:179`,
  `src/go/plugin/go.d/collector/snmp_traps/dynamic.go:20`,
  `src/go/plugin/go.d/collector/snmp_traps/dynamic.go:73`,
  `src/go/plugin/go.d/collector/snmp_traps/ratelimit.go:20`,
  `src/go/plugin/go.d/collector/snmp_traps/ratelimit.go:83`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: dynamic-discovery examples must not expose real
  engine IDs or usernames. Page-boundary notes: configuration documents the
  `dynamic_engine_id_max_pairs` knob; rate-limit source-table cap is an
  implementation sizing fact, not a public config option.
- Page: `docs/snmp-traps/sizing-and-capacity.md`; section: profile metric
  cardinality; user-facing fact to mention: default caps are 500 rules, 2,000
  sources, 512 resources per source, 50,000 instances per job, with
  `drop_and_count` overflow. Evidence source: `profile_metric.go`,
  `config_schema.json`, and profile-format reference; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/profile_metric.go:51`,
  `src/go/plugin/go.d/collector/snmp_traps/profile_metric.go:434`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:540`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:544`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:286`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: source label selection can expose source
  values. Page-boundary notes: do not turn caps into recommended sizing without
  benchmark/runtime evidence.

- Page: `docs/snmp-traps/field-reference.md`; section: journal fields;
  user-facing fact to mention: document severity-to-`PRIORITY`, base fields,
  trap identity/source fields, dedup fields, decode-error fields, `TRAP_TAG_*`,
  `TRAP_VAR_*`, `TRAP_ENRICHMENT`, and `TRAP_JSON`. `TRAP_JSON` includes
  `netdata_packet_sequence` for normal traps and a compact summary object for
  dedup summary rows. Evidence source: `serialize.go`, `decode_error.go`, and
  `dedup.go`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:17`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:77`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:94`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:139`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:145`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:149`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:159`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:401`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:432`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:456`,
  `src/go/plugin/go.d/collector/snmp_traps/decode_error.go:53`,
  `src/go/plugin/go.d/collector/snmp_traps/dedup.go:261`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: warn against broad export of `TRAP_JSON`.
  Page-boundary notes: field reference should be concise but complete.
- Page: `docs/snmp-traps/field-reference.md`; section: indexed varbinds;
  user-facing fact to mention: `TRAP_VAR_*` fields are generated for
  non-sensitive, non-redundant varbinds, enum-backed fields also get `_RAW`.
  Generated field names stay within journald's 64-byte field-name limit using
  truncation plus an 8-character hash, while full varbind identity remains in
  `TRAP_JSON`. Evidence source: `serialize.go` and profile-format reference;
  evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:198`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:226`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:330`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:342`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:367`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:406`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:24`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: do not imply all sensitive vendor varbinds are
  known or filtered. Page-boundary notes: examples need synthetic values.
- Page: `docs/snmp-traps/field-reference.md`; section: OTLP mapping;
  user-facing fact to mention: OTLP maps each trap entry to a LogRecord with
  timestamps, severity, body, event name, standard and Netdata attributes,
  varbind list, and profile/operator labels. Evidence source: `otlp.go`;
  evidence location: `src/go/plugin/go.d/collector/snmp_traps/otlp.go:35`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:574`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:589`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:611`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:632`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:652`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:696`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:700`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:779`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: varbind attributes can contain operational
  data. Page-boundary notes: configuration of endpoint/headers stays in
  `configuration.md`.

- Page: `docs/snmp-traps/investigation-playbooks.md`; section: high severity;
  user-facing fact to mention: operators can investigate emergency, alert,
  critical, error, and warning trap rates through severity charts and default
  alerts. Evidence source: `charts.yaml` and health alerts; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:112`,
  `src/health/health.d/snmp_traps.conf:5`. Evidence type: implementation.
  Verification status: proven. Security/sensitive-data notes: playbook examples
  must use synthetic devices. Page-boundary notes: do not invent cross-feature
  workflows that are not shipped.
- Page: `docs/snmp-traps/investigation-playbooks.md`; section: trap storms;
  user-facing fact to mention: dedup-suppressed counts and dedup summary rows
  help identify duplicate trap storms when dedup is enabled. Evidence source:
  `dedup.go`, `charts.yaml`, and health alerts; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/dedup.go:198`,
  `src/go/plugin/go.d/collector/snmp_traps/dedup.go:239`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:176`,
  `src/health/health.d/snmp_traps.conf:280`. Evidence type: implementation.
  Verification status: proven; real row examples need runtime verification.
  Security/sensitive-data notes: no real storm data. Page-boundary notes: sizing
  of dedup cache stays in `sizing-and-capacity.md`.
- Page: `docs/snmp-traps/investigation-playbooks.md`; section: data quality and
  auth failures; user-facing fact to mention: decode errors, malformed PDUs,
  auth failures, USM failures, unknown engine IDs, template unresolved, and
  profile load failures are surfaced as metrics/alerts and sometimes as
  decode-error rows. Evidence source: `decode_error.go`, `metrics.go`, and
  health alerts; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/decode_error.go:29`,
  `src/go/plugin/go.d/collector/snmp_traps/metrics.go:19`,
  `src/health/health.d/snmp_traps.conf:81`,
  `src/health/health.d/snmp_traps.conf:153`,
  `src/health/health.d/snmp_traps.conf:223`. Evidence type:
  implementation. Verification status: proven. Security/sensitive-data notes:
  decode-error rows omit raw packet bytes. Page-boundary notes: detailed
  remediation belongs in `troubleshooting.md`.
- Page: `docs/snmp-traps/investigation-playbooks.md`; section: SIEM/log
  workflows; user-facing fact to mention: playbooks may show implemented
  workflows using local journal queries, `snmp:traps`, OTLP-forwarded logs, and
  built-in metrics. Evidence source: `func_logs.go`, `func_logs_test.go`,
  `journal_writer.go`, `otlp.go`, and `charts.yaml`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:48`,
  `src/go/plugin/go.d/collector/snmp_traps/snmptrapsfunc/func_logs.go:78`,
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:63`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:89`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:564`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:64`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:112`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:137`. Evidence type:
  implementation. Verification status: needs-runtime-verification for exact
  queries. Security/sensitive-data notes: use sanitized examples. Page-boundary
  notes: no public text about missing topology/Flow correlation.

- Page: `docs/snmp-traps/validation-and-data-quality.md`; section: receiver
  quality signals; user-facing fact to mention: validation should check received
  vs decoded vs accepted vs committed counts, unknown OIDs, dropped allowlist,
  rate-limited packets, profile load failures, template unresolved, and write
  failures. Evidence source: `collector.go`, `metrics.go`, and `charts.yaml`;
  evidence location: `src/go/plugin/go.d/collector/snmp_traps/collector.go:482`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:506`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:622`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:641`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:666`,
  `src/go/plugin/go.d/collector/snmp_traps/metrics.go:64`,
  `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:64`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: none. Page-boundary notes: remediation steps
  go in `troubleshooting.md`.
- Page: `docs/snmp-traps/validation-and-data-quality.md`; section: decode-error
  rows; user-facing fact to mention: accepted-source decode failures can be
  written as sanitized `TRAP_REPORT_TYPE=decode_error` rows with packet size,
  SHA256, source port, listener, sniffed version, and safe engine ID when
  extractable; raw packet bytes are not written. Evidence source:
  `decode_error.go` and generated integration doc; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/decode_error.go:47`,
  `src/go/plugin/go.d/collector/snmp_traps/decode_error.go:53`,
  `src/go/plugin/go.d/collector/snmp_traps/decode_error.go:140`,
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:120`.
  Evidence type: implementation. Verification status: proven; example rows need
  runtime verification. Security/sensitive-data notes: do not show raw packet
  bytes. Page-boundary notes: field table stays in `field-reference.md`.

- Page: `docs/snmp-traps/anti-patterns.md`; section: operational mistakes;
  user-facing fact to mention: avoid treating traps as continuously polled
  state; trap-derived samples/states are last-reported values and profile
  metrics do not update for dedup-suppressed or write-failed traps. Evidence
  source: profile-format reference; evidence location:
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:418`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:425`.
  Evidence type: profile-reference. Verification status: proven.
  Security/sensitive-data notes: none. Page-boundary notes: this is a shipped
  behavior limitation and should be documented because it affects interpretation.
- Page: `docs/snmp-traps/anti-patterns.md`; section: security mistakes;
  user-facing fact to mention: avoid empty community/source allowlists in
  production, catch-all trusted relay CIDRs, plaintext remote OTLP, raw
  profile-metric source labels where sensitive, and alerting every trap without
  dedup/rate-limit thinking. Evidence source: `allowlist.go`,
  `config_schema.json`, `collector.go`, `otlp.go`, and health alerts; evidence
  location: `src/go/plugin/go.d/collector/snmp_traps/allowlist.go:15`,
  `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:201`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:758`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:765`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:177`,
  `src/health/health.d/snmp_traps.conf:5`. Evidence type: implementation.
  Verification status: proven. Security/sensitive-data notes: this page should
  link back to Secrets Management and configuration. Page-boundary notes: do not
  become a generic SNMP security guide.

- Page: `docs/snmp-traps/troubleshooting.md`; section: startup and bind
  failures; user-facing fact to mention: troubleshooting must cover invalid
  job/listen config, bind errors, receive-buffer errors, missing log root,
  non-Linux direct journal, and no output backend. Evidence source:
  `collector.go`, `listener.go`, and `journal_writer.go`; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:111`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:158`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:162`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:227`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:229`,
  `src/go/plugin/go.d/collector/snmp_traps/listener.go:56`,
  `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:74`.
  Evidence type: implementation. Verification status: proven.
  Security/sensitive-data notes: examples must not include real paths with
  customer names. Page-boundary notes: keep first-receive validation in
  quick start.
- Page: `docs/snmp-traps/troubleshooting.md`; section: no traps or bad traps;
  user-facing fact to mention: troubleshooting must cover source allowlist,
  version/community mismatch, SNMPv3 USM/engine errors, INFORM response
  failures, rate limiting, decode errors, malformed PDUs, unknown OIDs, and
  template/profile failures. Evidence source: `collector.go`, `inform.go`,
  `decode_error.go`, and health alerts; evidence
  location: `src/go/plugin/go.d/collector/snmp_traps/collector.go:502`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:515`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:579`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:588`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:602`,
  `src/go/plugin/go.d/collector/snmp_traps/collector.go:615`,
  `src/go/plugin/go.d/collector/snmp_traps/inform.go:14`,
  `src/go/plugin/go.d/collector/snmp_traps/decode_error.go:53`,
  `src/health/health.d/snmp_traps.conf:81`. Evidence type: implementation.
  Verification status: proven. Security/sensitive-data notes: do not print real
  communities or USM keys in examples. Page-boundary notes: root-cause logic can
  link to validation and playbooks.
- Page: `docs/snmp-traps/troubleshooting.md`; section: storage/export problems;
  user-facing fact to mention: troubleshooting must cover journal write
  failures, retention/disk pressure, OTLP preflight/export failures, listener
  read failures, binary-encoded fields, and high dedup suppression. Evidence
  source: `trapwriter_impl.go`, `retention.go`, `retention_config.go`,
  `otlp.go`, `cwe117.go`, and health alerts; evidence location:
  `src/go/plugin/go.d/collector/snmp_traps/trapwriter_impl.go:47`,
  `src/go/plugin/go.d/collector/snmp_traps/retention.go:45`,
  `src/go/plugin/go.d/collector/snmp_traps/retention_config.go:40`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:112`,
  `src/go/plugin/go.d/collector/snmp_traps/otlp.go:293`,
  `src/go/plugin/go.d/collector/snmp_traps/cwe117.go:7`,
  `src/health/health.d/snmp_traps.conf:209`,
  `src/health/health.d/snmp_traps.conf:237`,
  `src/health/health.d/snmp_traps.conf:252`,
  `src/health/health.d/snmp_traps.conf:266`,
  `src/health/health.d/snmp_traps.conf:280`. Evidence type:
  implementation. Verification status: proven; exact diagnostic commands need
  runtime verification. Security/sensitive-data notes: no real endpoints or
  headers. Page-boundary notes: SIEM recovery workflows stay in
  `forwarding-to-siem.md`.

### Phase 1.5 Runtime Evidence Required Before Public Drafting

Blocking labels:

- `BLOCKS_PUBLIC_DRAFT`: proof required before any public SNMP trap docs are
  drafted from the affected inventory.
- `BLOCKS_SPECIFIC_PAGE:<page>`: proof required only before that page includes
  copyable commands, screenshots, exact failure output, or example rows for the
  fact.

Runtime evidence checklist:

- `BLOCKS_PUBLIC_DRAFT`: run runtime verification on Linux, or record a
  separate evidence path for any direct-journal command because the direct
  journal backend is Linux-only.
- `BLOCKS_PUBLIC_DRAFT`: populate a local direct journal with representative
  synthetic traps: decoded trap, unknown OID, decode error, dedup summary,
  high-severity trap, and profile-metric-producing trap.
- `BLOCKS_PUBLIC_DRAFT`: verify `snmp:traps` requests, `__logs_sources`,
  default facets, default view keys, and histogram behavior with populated data.
- `BLOCKS_PUBLIC_DRAFT`: verify built-in chart data, alert-relevant metrics,
  and at least one profile-defined metric.
- `BLOCKS_PUBLIC_DRAFT`: verify profile metric identity behavior for
  vnode-attributed source metrics, fallback `source_id`/`source_kind` labels,
  hash vs raw source ID configuration, and a resource metric with bounded
  `resource_class`/`resource_id`.
- `BLOCKS_SPECIFIC_PAGE:journal-and-querying.md`: verify exact
  `journalctl --directory` commands against the effective per-job journal
  directory.
- `BLOCKS_SPECIFIC_PAGE:forwarding-to-siem.md`: verify OTLP export against a
  test receiver and capture the emitted LogRecord shape, including resource
  attributes, event name, severity mapping, varbinds, labels, and
  decode-error/dedup fields.
- `BLOCKS_SPECIFIC_PAGE:forwarding-to-siem.md`: verify OTLP preflight behavior.
  The empty `ExportLogsServiceRequest` sent during job creation is source-proven
  by `otlp.go:293`; runtime evidence is still required for exact startup
  failure text and operator-visible output for an unreachable or invalid
  receiver.
- `BLOCKS_SPECIFIC_PAGE:forwarding-to-siem.md`: verify both-output fanout
  behavior: journal commit remains queryable when secondary OTLP export fails,
  and OTLP failure metrics increment.
- `BLOCKS_SPECIFIC_PAGE:forwarding-to-siem.md`: verify OTLP-only behavior with
  `journal.enabled: false`: no local direct journal is created, `snmp:traps` /
  `__logs_sources` local Function examples follow the missing-source path, OTLP
  export failures increment terminal pipeline/source write-failure metrics,
  committed/event/severity counters match the documented "authoritative output
  commitment" semantics, and profile-metric side effects are documented only
  after the async export path is proven.
- `BLOCKS_SPECIFIC_PAGE:configuration.md`: verify URL-scheme transport behavior
  for OTLP examples: bare/`http://` means plaintext gRPC, `https://` means TLS.
- `BLOCKS_SPECIFIC_PAGE:configuration.md`: verify SNMPv3 INFORM behavior with
  local engine ID and persisted engine-boots state if public docs include INFORM
  examples.
- `BLOCKS_SPECIFIC_PAGE:configuration.md`: verify SNMPv3 dynamic engine ID
  discovery with synthetic v3 Trap senders if public docs include
  dynamic-discovery examples, including the first accepted pair's
  `unknown_engine_id` visibility increment and the configured pair cap.
- `BLOCKS_SPECIFIC_PAGE:troubleshooting.md`: verify direct-journal failure
  behavior for missing/invalid machine ID and boot ID only if public
  troubleshooting docs include those exact failure paths.
- `BLOCKS_SPECIFIC_PAGE:field-reference.md`: verify binary-encoded field
  behavior with a synthetic unsafe value if public docs show a row or metric
  example for CWE-117 protection.
- `BLOCKS_SPECIFIC_PAGE:sizing-and-capacity.md`: verify journal retention and
  rotation behavior only to the extent public docs provide operational commands
  or example outputs; otherwise document configuration semantics from
  implementation evidence only.
- `BLOCKS_SPECIFIC_PAGE:journal-and-querying.md`: verify local/no-direct-journal
  behavior for `snmp:traps` and `__logs_sources`, including the 503 "direct
  journal output has no sources" response path from a missing direct-journal
  root, if public docs include a local API example.
- `BLOCKS_SPECIFIC_PAGE:usage-and-output.md`: capture user-provided screenshots
  only after representative data exists.
- `BLOCKS_SPECIFIC_PAGE:sizing-and-capacity.md`: create a follow-up SOW for
  benchmark-based sizing if public docs need trap-rate throughput, CPU, memory,
  disk growth, or high-volume deployment numbers beyond conservative defaults
  and implementation limits.

### Phase 1 Reviewer Round 1 Findings And Disposition

Reviewers:

- `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, and `qwen` all returned
  `ACCEPT_WITH_CHANGES` on the Phase 1 inventory.

Required corrections folded into this SOW:

- Added missing facts for event-driven reception vs `update_every`, stock
  `snmp_traps.conf`, open default source allowlist, SNMPv3 local engine ID and
  engine-boots persistence, INFORM response handling, OTLP preflight and
  scheme-driven TLS/plaintext behavior, fanout writer semantics, direct journal
  machine/boot ID requirements, CWE-117 binary journal encoding, full
  protocol-varbind skip behavior, and field-name truncation/hash behavior.
- Strengthened weak evidence lines for `snmp:traps` Cloud-required behavior,
  default facets/view keys/histogram, Secrets Management map entry, decode
  limits, profile metric caps, retention units, OTLP resource attributes, and
  journal/SIEM workflows.
- Recorded runtime verification additions for `journalctl --directory`,
  `snmp:traps`, `__logs_sources`, OTLP receiver/preflight behavior, INFORM,
  profile metric identity, binary-encoded fields, journal rotation/retention,
  and copied-journal/SIEM workflows.
- Reasserted public-doc boundaries: field tables belong in `field-reference.md`;
  configuration owns syntax/defaults; sizing owns deployment-choice guidance;
  journal querying is separate from SIEM forwarding; public docs do not mention
  missing topology or Network Flow correlation.

Rejected or deferred reviewer items:

- Public docs will not describe missing topology correlation, missing Network
  Flow correlation, or richer integration search as product gaps because the
  user explicitly scoped public docs to shipped behavior only.
- A profile-pack-local README under
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/` is not added in this
  phase. It is a possible follow-up documentation artifact after the public
  docs and profile-format cross-links are designed.
- Benchmark or high-volume sizing claims remain gated behind a separate evidence
  SOW unless runtime/benchmark evidence exists before drafting.

Rerun requirement:

- Run the same six reviewers again against the full SOW and Phase 1 inventory
  with a short fix note list. Do not narrow the review scope to only the
  corrections above.

Risks:

- If docs describe intended behavior instead of implemented behavior, operators
  will trust workflows that do not work.
- If journalctl/SIEM workflows are under-documented, users may think trap data
  is locked inside Netdata instead of usable with standard journal tooling.
- If OTLP forwarding is documented as equivalent to local journal storage
  without caveats, users may misunderstand local Logs UI behavior and retention.
- If research remains beside specs, future agents can treat external product
  research or speculative analysis as Netdata product contract.
- If pages are created without `docs/.map/map.yaml`, they will not publish
  correctly on Learn.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The implementation branch has substantial SNMP trap behavior that must be
  documented for operators.
- The documentation source material is split across generated collector docs,
  profile-format reference docs, Netdata specs, SOW research, and implementation
  behavior.
- The durable spec directory currently mixes product specs with research
  artifacts, creating a real risk of future agents treating research as
  authoritative Netdata specification.

Evidence reviewed:

- `docs/.map/map.yaml` contains the Network Flows section and every published
  flow page.
- `docs/network-flows/` contains the pattern to reuse: overview, setup,
  configuration, field reference, validation, troubleshooting, investigation
  playbooks, anti-patterns, and visualization docs.
- `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md`
  is generated collector-facing integration documentation, not a complete
  feature documentation set.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` is the
  schema/reference page for trap profiles.
- `.agents/sow/specs/snmp-traps/` currently contains both Netdata specs and
  files named for external systems, playbooks, skill distillation, and
  comparison research.

Affected contracts and surfaces:

- Learn sidebar and routing through `docs/.map/map.yaml`.
- New or updated docs under `docs/`.
- Generated integration docs and collector metadata under
  `src/go/plugin/go.d/collector/snmp_traps/`.
- Trap profile format docs under
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/`.
- Durable specs and research under `.agents/sow/specs/snmp-traps/`.
- Runtime project skill
  `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md`.
- Active SOW lifecycle and final delete-before-merge requirement.

Clean-end-state target:

- SNMP trap docs are a mapped, coherent operator documentation section modeled
  after Network Flows.
- The section uses the documented `docs/snmp-traps/` page split, including a
  dedicated sizing and capacity page because trap-rate, journal retention, OTLP
  buffering, dedup cache, and profile metric cardinality are deployment-sizing
  decisions.
- Top-level `.agents/sow/specs/snmp-traps/` contains Netdata-owned specs,
  decisions, and architecture references.
- `.agents/sow/specs/snmp-traps/research/` contains research inputs:
  external-product studies, research playbooks, comparative matrices, stress
  tests, and research-derived evidence inventories.
- Internal links and citation conventions point to the new research paths.
- The SNMP trap profile authoring skill tells future agents where specs and
  research belong.
- The hub architecture spec is clearly marked as target-state architecture for
  topology/flow/cross-signal correlation, not as proof of shipped trap behavior
  for public documentation.
- Removed as redundant (i): duplicated SNMP trap operator prose in generated
  integration docs if the same prose is moved to the new docs section; stale
  references to old research locations after files move.
- Excluded coupled items (ii): implementation fixes are excluded unless final
  documentation review proves a behavior is unsafe or undocumented behavior
  would mislead users. In that case, pause for a user decision before changing
  code.
- Reviewer-discovered coupled cleanup: the pre-existing legacy `SOW-NNNN`
  references that keep `.agents/sow/audit.sh` failing, plus the completed
  previous SOW still under `.agents/sow/active/`, must be fixed, explicitly
  user-excluded with reason, or tracked before this SOW is completed. Current
  legacy-reference scope is 62 matches across 9 durable files:
  `.agents/sow/specs/snmp-traps/decisions/0001-go-process-and-trapwriter.md`,
  `.agents/sow/specs/snmp-traps/netdata.md`,
  `.agents/sow/specs/snmp-traps/research/comparison/comparative-analysis.md`,
  `.agents/sow/specs/snmp-traps/research/comparison/comparison-matrix.md`,
  `.agents/sow/specs/snmp-traps/research/external-systems/centreon.md`,
  `.agents/sow/specs/snmp-traps/research/external-systems/nagios-snmptt.md`,
  `.agents/sow/specs/snmp-traps/research/netdata-existing/netdata-existing-netflow.md`,
  `.agents/sow/specs/snmp-traps/research/netdata-existing/netdata-existing-netipc.md`,
  and
  `.agents/sow/specs/snmp-traps/research/netdata-existing/netdata-existing-snmp.md`.
  Only the first two are outside `research/`. The completed predecessor SOW is
  `.agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md`.
- Reference search: required before moving research files and before declaring
  docs complete. Search must include `.agents/`, `docs/`, and relevant
  `src/go/plugin/go.d/collector/snmp_traps/` paths.

Existing patterns to reuse:

- `docs/network-flows/` page split and operator tone.
- `docs/.map/map.yaml` publication pattern for a top-level feature section.
- `docs/.map/README.md` publishing rules.
- Generated collector integration docs from `metadata.yaml`.
- Trap profile schema reference in `profile-format.md`.
- `sync-docs-specs-skills` classification rules for docs vs specs vs skills.

Risk and blast radius:

- Documentation-only changes still affect public user behavior because users may
  configure traps, storage, forwarding, and SIEM exports from these pages.
- Moving research files can break internal links unless all references are
  searched and updated.
- Map changes can move Learn URLs if labels or paths are chosen poorly.
- Journal examples can accidentally expose or normalize unsafe secret handling
  if they include real communities, customer names, or public IPs.
- OTLP examples can imply guaranteed delivery or local availability unless
  carefully scoped to the implemented behavior.

Sensitive data handling plan:

- Use only private documentation examples such as `192.0.2.10`,
  `198.51.100.10`, `203.0.113.10`, `router-1`, `site-a`, and
  `[REDACTED_SECRET]`.
- Do not include real SNMP communities, USM keys, bearer tokens, customer names,
  non-private customer-identifying IPs, private endpoints, or proprietary
  incident details in SOWs, specs, docs, skills, or examples.
- Show journalctl and OTLP commands with placeholders for hosts, tokens, and
  endpoints.

Implementation plan:

1. Approve documentation information architecture and artifact organization.
2. Move research artifacts under `.agents/sow/specs/snmp-traps/research/` and
   update internal references.
3. Update the SNMP trap authoring skill with the specs-vs-research rule.
4. Review the completed SNMP trap implementation enough to document actual
   behavior, not intended behavior.
5. Add the SNMP trap docs section and map entries following the Network Flows
   pattern.
6. Update generated integration metadata/docs and profile-format cross-links so
   users can navigate between high-level docs and schema reference.
7. Validate docs, map, links, sensitive data scan, and implementation/doc
   consistency.

Validation plan:

- `rg` reference search for moved research paths before and after the move.
- `git diff --check` on changed docs/specs/skills/SOW files.
- `.agents/sow/scan-sensitive.sh` on changed SOW/spec/skill/doc files.
- YAML validation for `docs/.map/map.yaml` if project tooling is available.
- Markdown/link checks for changed docs where project tooling supports it.
- Targeted implementation review of SNMP trap config, metrics, journal, OTLP,
  and profile behavior before writing final operator docs.
- Phase 1 inventory validation:
  - every inventory entry has the required schema;
  - every implementation fact cites file/line evidence;
  - every config default/limit cites `config.go`, `config_schema.json`, generated
    integration docs, or tests;
  - every metric/alert claim cites `metrics.go`, `charts.yaml`,
    `metadata.yaml`, or `src/health/health.d/snmp_traps.conf`;
  - every field-reference claim cites `serialize.go`, `trapentry.go`,
    `otlp.go`, specs, or tests.
- Runtime validation:
  - `journalctl --directory` works against the effective per-job direct-journal
    directory;
  - `snmp:traps` returns direct-journal jobs and `__logs_sources`;
  - OTLP export works against a test receiver;
  - representative decoded trap, decode-error, dedup-summary, metric, alert, and
    profile-defined metric evidence is captured.
- Docs-class sensitive-data rules:
  - examples use documentation IPs and placeholders only;
  - no real SNMP communities, USM keys, OTLP headers, endpoints, tokens, customer
    names, public customer-identifying IPs, or proprietary incident details;
  - `TRAP_JSON`, `TRAP_VAR_*`, MESSAGE, and OTLP examples are synthetic or
    sanitized and include sharing/export caution.
  - Learn/map validation:
    - new pages are present in `docs/.map/map.yaml`;
    - map ordering matches the intended sidebar;
    - links to generated integration docs and `profile-format.md` resolve;
    - Learn ingest/preview or equivalent map validation is run when available.
  - Draft leakage guardrails:
    - `rg -n '1 ms|1ms' docs/snmp-traps src/go/plugin/go.d/collector/snmp_traps/metadata.yaml src/go/plugin/go.d/collector/snmp_traps/integrations .agents/sow/specs/snmp-traps/netdata.md`
      must return no unsupported time-bound decode claim, or the hit must be
      explicitly justified with implementation or benchmark evidence. This check
      applies after `docs/snmp-traps/` exists in Phase 3 and after generated
      integration metadata is synchronized;
    - public docs must not use `correlation`, `topology position`, or
      `NetFlow records` language from the hub architecture spec unless the
      relevant shipped behavior is proven by implementation evidence.
- Post-draft consistency:
  - each public page is checked against its Phase 1 inventory and Phase 2 gap
    dispositions;
  - reviewer findings are iterated to convergence using the same review scope;
  - any rejected reviewer finding records evidence and rationale.

Artifact impact plan:

- AGENTS.md: no expected update unless SOW lifecycle or repo-wide docs rules are
  changed.
- Runtime project skills: update
  `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md`.
- Specs: move research under `.agents/sow/specs/snmp-traps/research/`; keep
  Netdata specs and decisions at top level.
- End-user/operator docs: add a mapped SNMP trap docs section under `docs/`.
- End-user/operator skills: update only if docs introduce operator workflows
  that need portable AI-agent instructions.
- SOW lifecycle: this file is branch-local working memory and must be deleted
  before merge after durable knowledge is transferred.

Open-source reference evidence:

- No new external OSS references are required to create this SOW. Existing
  research files already contain external product evidence; this SOW focuses on
  organizing and converting that evidence into Netdata documentation.

Decision status:

- Decision 1: SNMP trap Learn placement and initial page split. Approved A.
- Decision 2: Documentation analysis workflow. Approved A, with reviewer-required
  refinements recorded in this SOW.
- Decision 3: Specs vs research organization. Approved A.
- Decision 4: Skill organization rule. Approved A.
- Decision 5: Documentation accuracy and missing-evidence gate. Approved A for
  evidence-first docs and separate SOWs for missing evidence; implementation
  defects that require code changes still pause for user decision.
- Decision 6: Screenshot scope. Approved: text-first now; user-provided
  screenshots after the text docs are ready and representative trap data exists
  in the journal.
- Decision 7: Reviewer-suggested page-structure refinements. Approved B:
  standalone `enrichment.md` and `usage-and-output.md`; security remains a
  major `configuration.md` section; shipped query, chart, and export workflows
  remain in `investigation-playbooks.md`.
- Decision 8: Security treatment. Approved B: keep security in
  `configuration.md`, mention and link to go.d.plugin Secrets Management, and
  avoid duplicating secret-provider documentation.
- Decision 9: Correlation treatment. Approved B with constraint: document only
  shipped query, chart, and export workflows. Do not mention missing topology
  correlation, missing Network Flow correlation, or other pending roadmap work
  in public docs.
- Decision 10: Generated integration placement. Approved A: keep the generated
  collector integration page under Collecting Metrics and cross-link manually
  from the SNMP Traps docs. Do not add a new `integration_placeholder` entry for
  SNMP traps in this SOW. Richer vendor/MIB/trap search is pending future work.
- Decision 11: Missing evidence and limitations policy. Approved:
  identify missing evidence and plan follow-up work, but public end-user docs
  describe what the system does today. Limitations are documented only when they
  affect shipped features.
- Approved scope now in progress: build Phase 1 implementation evidence
  inventory, then run runtime verification and research gap analysis before
  drafting public docs.

## Implications And Decisions

1. Documentation structure.

   Options:

   - A. Long-term-best: create a top-level `docs/snmp-traps/` section modeled
     after Network Flows, with multiple task/reference/playbook pages and
     `docs/.map/map.yaml` entries.
     Pros: complete, navigable, scalable, consistent with Network Flows.
     Cons: larger docs change; needs more validation.
     Risks: map/link mistakes if not validated.
   - B. Surgical: expand only the generated collector integration page and
     profile-format reference.
     Pros: smaller diff.
     Cons: too much material in one page; weaker discovery; does not match the
     requested Network Flows pattern.
     Risks: users miss journalctl, OTLP, troubleshooting, and metrics guidance.

   Recommendation: A, long-term-best. This feature is broad enough to need a
   real documentation section.

   User decision: A. The user accepted adding the concrete page structure to
   the SOW.

   Page structure to implement:

   - `docs/snmp-traps/README.md`
   - `docs/snmp-traps/installation.md`
   - `docs/snmp-traps/quick-start.md`
   - `docs/snmp-traps/configuration.md`
   - `docs/snmp-traps/trap-profiles.md`
   - `docs/snmp-traps/enrichment.md`
   - `docs/snmp-traps/usage-and-output.md`
   - `docs/snmp-traps/journal-and-querying.md`
   - `docs/snmp-traps/forwarding-to-siem.md`
   - `docs/snmp-traps/metrics-and-alerts.md`
   - `docs/snmp-traps/sizing-and-capacity.md`
   - `docs/snmp-traps/field-reference.md`
   - `docs/snmp-traps/investigation-playbooks.md`
   - `docs/snmp-traps/validation-and-data-quality.md`
   - `docs/snmp-traps/anti-patterns.md`
   - `docs/snmp-traps/troubleshooting.md`

2. Documentation analysis workflow.

   Options:

   - A. Long-term-best: build an evidence-backed section inventory first, run
     external reviewer validation, then perform a research gap analysis against
     the distillation and playbook documents with the same reviewer validation.
     Create follow-up SOWs for missing benchmark, sizing, UI, or operational
     evidence before public docs depend on those claims.
     Pros: public docs are grounded in implementation and user needs; missing
     evidence is surfaced before writing; reviewers reduce blind spots.
     Cons: more analysis work before drafting.
     Risks: may expose missing implementation evidence or benchmark gaps that
     delay docs.
   - B. Surgical: draft docs directly from implementation docs, specs, and
     existing research, then review the draft.
     Pros: faster first draft.
     Cons: harder to prove completeness; gaps are discovered later when prose is
     already written.
     Risks: inaccurate or incomplete public docs, especially around sizing,
     SIEM forwarding, journal workflows, and operational playbooks.

   Recommendation: A, long-term-best. SNMP traps touch deployment, security,
   storage, forwarding, metrics, and incident workflows; the docs need an
   evidence inventory and research gap analysis before prose drafting.

   User decision: A. The user explicitly requested this staged analysis and
   reviewer validation. Six reviewers returned `ACCEPT_WITH_CHANGES`; their
   required refinements are folded into the workflow above.

3. Specs vs research organization.

   Options:

   - A. Long-term-best: keep Netdata specs and decisions at
     `.agents/sow/specs/snmp-traps/`; move external product studies,
     comparison artifacts, playbooks, and research outputs under
     `.agents/sow/specs/snmp-traps/research/`.
     Pros: clear artifact semantics; future agents cannot confuse research with
     Netdata specs.
     Cons: requires reference updates.
     Risks: broken links if the move is not searched thoroughly.
   - B. Surgical: leave files where they are and add a README warning.
     Pros: fewer file moves.
     Cons: warning can be missed; directory still mixes concerns.
     Risks: research can still be treated as product contract.

   User decision: A. The user explicitly requested moving research under
   `.agents/sow/specs/snmp-traps/research/`.

   Classification to apply:

   - Netdata specs and decisions that stay under `.agents/sow/specs/snmp-traps/`:
     `netdata.md`, `trap-metrics-profiles.md`,
     `netdata-snmp-hub-architecture.md`, and `decisions/`.
   - Domain research that moves under `research/domain/`:
     `snmp-traps-in-observability.md`.
   - Research playbooks that move under `research/playbooks/`:
     `PLAYBOOK-Monitoring-SNMP-Traps-in-Modern-Enterprise-NPM-NetOps-SecOps.md`
     and
     `Skill-Distillation-SNMP-Traps-in-Network-Performance-Monitoring-NetOps-SecOps.md`.
   - Internal Netdata current-state inventories that move under
     `research/netdata-existing/`: `netdata-existing-netflow.md`,
     `netdata-existing-netipc.md`, and `netdata-existing-snmp.md`.
   - External-product studies that move under `research/external-systems/`:
     `centreon.md`, `checkmk.md`, `cribl.md`, `datadog-agent.md`,
     `dynatrace.md`, `librenms.md`, `logicmonitor.md`, `logstash.md`,
     `nagios-snmptt.md`, `opennms.md`, `sensu.md`, `solarwinds.md`,
     `splunk-sc4snmp.md`, `telegraf.md`, `zabbix.md`, and `zenoss.md`.
   - Comparative research that moves as a folder under `research/comparison/`.

4. Skill organization rule.

   Options:

   - A. Long-term-best: update the SNMP trap authoring skill to define the
     `snmp-traps/` top-level spec vs `snmp-traps/research/` research boundary.
     Pros: future profile/spec edits follow the new organization.
     Cons: small durable skill update required.
     Risks: none beyond normal review.
   - B. Do not update the skill.
     Pros: no skill diff.
     Cons: future agents will not know the rule.
     Risks: directory drift returns.

   User decision: A. The user explicitly requested fixing the skill too.

5. Documentation accuracy gate.

   Options:

   - A. Long-term-best: review the implemented SNMP trap behavior before writing
     final docs; if behavior is broken or misleading, pause and decide whether
     to fix code or document a limitation.
     Pros: prevents documenting intended behavior as fact.
     Cons: adds review time before docs writing.
     Risks: can surface implementation issues that delay docs.
   - B. Surgical: write docs from specs and implementation notes only.
     Pros: faster docs drafting.
     Cons: higher risk of inaccurate public docs.
     Risks: users follow incorrect examples or trust unsupported behavior.

   Recommendation: A, long-term-best.

   User decision: A for evidence gaps. The user explicitly said that if evidence
   is missing, a new SOW should gather it before public docs depend on it.
   Implementation defects that need code changes remain user-owned pause
   decisions.

6. Screenshot and UI evidence scope.

   Evidence:

   - Phase 1.5 requires representative trap rows for `journalctl`,
     `snmp:traps`, OTLP, metrics, alerts, and profile-defined metrics before
     public commands or UI workflows depend on them.
   - Empty Logs UI screenshots would not document a useful operator workflow.

   Options:

   - A. Include screenshots in the first text draft.
     Pros: visual docs ship at once.
     Cons: requires trap data and screenshots before prose can stabilize.
     Risks: screenshots show empty or unrepresentative states.
   - B. Long-term-best: write text docs first, then populate a representative
     trap journal and add user-provided screenshots for filtering, histogram,
     search, and related Logs UI workflows.
     Pros: keeps prose grounded first; screenshots can show real workflows.
     Cons: screenshots become a follow-on docs step.
     Risks: final visual polish waits for the screenshot pass.

   Recommendation: B, long-term-best.

   User decision: B.

7. Reviewer-suggested page-structure refinements.

   Evidence:

   - Reviewers found that enrichment, usage/output, security, and correlation
     needed clear page ownership before the inventory is built.
   - The approved page list is modeled on Network Flows, but SNMP traps use the
     existing Logs UI and journal workflows instead of a dedicated flow-like
     visualization section.

   Options:

   - A. Add standalone pages for enrichment, usage/output, security, and
     correlation.
     Pros: maximum separation.
     Cons: larger docs set; security/correlation pages could be thin or imply
     features not yet shipped.
     Risks: public docs may overemphasize missing or future work.
   - B. Long-term-best: add standalone `enrichment.md` and
     `usage-and-output.md`; keep security in `configuration.md`; keep shipped
     query, chart, and export workflows in `investigation-playbooks.md`.
     Pros: clear ownership without creating thin roadmap-shaped pages.
     Cons: security and correlation are not top-level pages.
     Risks: those sections must be prominent enough to be discoverable.
   - C. Keep the original 14-page split.
     Pros: smaller page set.
     Cons: known ambiguity remains.
     Risks: inventory and docs may scatter key topics.

   Recommendation: B, long-term-best.

   User decision: B.

8. Security treatment.

   Evidence:

   - `docs/.map/map.yaml` maps a "Secrets Management" section at lines 444-449.
   - `src/collectors/SECRETS.md` documents `${env:...}`, `${file:...}`,
     `${cmd:...}`, and `${store:...}` secret references.
   - `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md`
     already tells users to use Netdata secret references for SNMPv3 keys and
     OTLP headers.

   Options:

   - A. Create a standalone `security.md`.
     Pros: easy to find.
     Cons: duplicates existing Secrets Management docs and can become a generic
     SNMP security essay.
     Risks: stale duplicated guidance.
   - B. Long-term-best: keep security in `configuration.md`, link to Secrets
     Management for providers and resolver syntax, and add security-relevant
     warnings in anti-patterns and playbooks.
     Pros: keeps configuration and security together; avoids duplicating secret
     provider docs.
     Cons: requires careful anchors and links.
     Risks: security guidance must remain visible inside a larger page.

   Recommendation: B, long-term-best.

   User decision: B.

9. Correlation treatment.

   Evidence:

   - The shipped trap implementation stores/query traps as logs and can forward
     via OTLP; journalctl/SIEM workflows are in scope.
   - The user confirmed trap-to-topology work has not been done yet, and that
     Network Flow correlation is unclear.

   Options:

   - A. Create a standalone correlation page.
     Pros: room for future workflows.
     Cons: suggests a product surface that is not clearly implemented today.
     Risks: public docs could document missing roadmap work.
   - B. Long-term-best: include only shipped query, chart, and export workflows
     in `investigation-playbooks.md`, such as logs, journal queries, SIEM
     export, and implemented metrics workflows when evidence supports them.
     Pros: docs stay truthful to shipped behavior.
     Cons: future topology/flow correlation will need a later docs update.
     Risks: none if missing roadmap items stay internal.

   Recommendation: B, long-term-best.

   User decision: B, with explicit constraint that public docs must not mention
   missing topology correlation, missing Network Flow correlation, or pending
   roadmap work. Profile-defined metrics and receiver/pipeline metrics are
   shipped behavior and remain in scope; only missing topology correlation,
   missing Network Flow correlation, and richer integration search are excluded
   from public docs in this SOW.

10. Generated integration page placement.

   Evidence:

   - Generated SNMP trap integration docs live at
     `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md`.
   - Network Flows uses `docs/.map/map.yaml` integration placeholders for
     generated integration content.
   - The user stated the current generated page understates the supported
     vendor/MIB/trap scale and that richer searchable vendor/MIB/trap discovery
     is future work.

   Options:

   - A. Long-term-best for this SOW: keep the generated integration page under
     Collecting Metrics and add manual cross-links from the SNMP Traps docs.
     Pros: avoids treating the current generated page as the main feature docs;
     keeps searchable vendor/MIB/trap work separate.
     Cons: requires clear cross-links.
     Risks: generated integration page remains less expressive until a later
     integration-search project.
   - B. Add the generated integration page into the new SNMP Traps section now.
     Pros: one sidebar location.
     Cons: current generated page may understate capability in the new section.
     Risks: users may judge the feature from incomplete generated copy.

   Recommendation: A, long-term-best for the current docs SOW.

   User decision: A.

   Explicit routing decision: no `integration_placeholder` is added for SNMP
   traps in this SOW. The generated collector integration remains in its current
   Collecting Metrics placement, and the new public SNMP Traps docs will link to
   it manually where useful.

11. Missing evidence and public limitations policy.

   Evidence:

   - Phase 2 gap analysis compares implementation evidence against research and
     can expose missing benchmark data, runtime proof, screenshots, or missing
     implementation.
   - Public docs are end-user/operator documentation, not an internal roadmap or
     gap tracker.

   Options:

   - A. Block every page section until all related evidence and future work are
     complete.
     Pros: avoids partial documentation.
     Cons: would delay documenting shipped behavior.
     Risks: useful current functionality remains undocumented.
   - B. Long-term-best: document shipped behavior with evidence, create
     follow-up SOWs for missing evidence or future work, and document only
     limitations that affect shipped features.
     Pros: accurate public docs now; internal gaps remain tracked.
     Cons: requires discipline during Phase 2 dispositions.
     Risks: reviewers may keep trying to push roadmap gaps into public docs; the
     SOW must reject that explicitly.
   - C. Use conservative wording and avoid follow-up SOWs.
     Pros: less process.
     Cons: loses traceability for important missing evidence.
     Risks: important work disappears.

   Recommendation: B, long-term-best.

   User decision: B.

## Plan

1. Reorganize `.agents/sow/specs/snmp-traps/` into specs and research, updating
   all references. Status: completed.
2. Update the SNMP trap authoring skill with the organization rule. Status:
   completed.
3. Record and review the documentation analysis workflow. Status: completed.
4. Resolve IA decisions: screenshots deferred, page refinements approved,
   security/correlation placement approved, generated integration placement
   approved, and missing-evidence policy approved. Status: completed.
5. Build the Phase 1 implementation evidence inventory with the required
   template, source hierarchy, implementation-file audit, collector-consistency
   audit, and page-boundary notes. Status: completed.
6. Run external reviewers on the Phase 1 inventory and iterate to convergence.
   Status: completed for Phase 1. Five same-scope rerun reviewers completed
   after the latest fixes; all returned `ACCEPT_WITH_CHANGES` with no blockers
   for Phase 1.5 or Phase 2. Required corrections were folded into the SOW. The
   sixth reviewer (`qwen`) became unresponsive and was terminated by exact PID
   after verification that the process command matched this SOW.
7. Run Phase 1.5 runtime verification for journalctl, `snmp:traps`, OTLP,
   representative trap rows, metrics, alerts, and profile-defined metrics; create
   evidence-gathering SOWs for missing benchmark/UI/runtime evidence before
   public docs depend on those claims. Status: pending.
8. Build the Phase 2 research gap analysis against playbooks, domain research,
   comparison synthesis, and targeted external-system evidence. Status: pending.
9. Run external reviewers on Phase 2 and iterate to convergence.
   Status: pending.
10. Draft the SNMP trap documentation set and map entries from approved inventory
    and gap-analysis dispositions. Status: completed for the text-first
    end-user page set. Runtime screenshots, representative demo data, and
    throughput evidence remain gated and are not included as public claims.
11. Synchronize generated integration docs, profile-format links, specs, and
    end-user/operator skills. Status: pending.
12. Resolve SOW-system cleanup before this SOW is completed: either remove the
    completed predecessor SOW from `.agents/sow/active/` with explicit user
    approval, and fix or formally track the legacy `SOW-NNNN` durable-reference
    audit failures. Status: pending.
13. Validate docs, links, Learn map, runtime examples, sensitive-data safety,
    reviewer convergence, and SOW lifecycle cleanup before completion.
    Status: pending.
14. Run production-readiness external reviews page-by-page for the complete
    text-first documentation set. Status: in-progress. Each page is reviewed by
    `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, and `qwen` in parallel for
    missing information, misleading details, confusing or ambiguous language,
    wrong examples or directives, non-end-user content, scope drift, and
    documentation quality score. The same page is rerun after fixes until all
    responding reviewers vote `PRODUCTION GRADE`.

## Execution Log

### 2026-06-13

- Created the documentation SOW in planning state.
- Recorded user decisions about Network Flows alignment, journalctl/SIEM and
  OTLP documentation, research relocation, and skill update.
- Reclassified `.agents/sow/specs/snmp-traps/`:
  - top-level Netdata specs/decisions now contain `netdata.md`,
    `trap-metrics-profiles.md`, `netdata-snmp-hub-architecture.md`,
    `README.md`, and `decisions/`;
  - research inputs moved under `research/domain/`, `research/playbooks/`,
    `research/netdata-existing/`, `research/external-systems/`, and
    `research/comparison/`;
  - `.agents/sow/specs/README.md`, top-level SNMP trap specs, and stale
    references were updated to point at the new research paths;
  - `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` now records
    the specs-vs-research organization rule.
- Added the concrete SNMP Traps documentation information architecture and
  sizing-page rationale to this SOW.
- Added the reviewer-validated documentation analysis workflow:
  - Phase 1 implementation evidence inventory;
  - Phase 1.5 runtime verification;
  - Phase 2 research gap analysis;
  - Phase 3 docs drafting and validation.
- Ran six external reviewers (`glm`, `minimax`, `kimi`, `mimo`, `deepseek`,
  `qwen`) against the SOW and analysis workflow. All six returned
  `ACCEPT_WITH_CHANGES`; required refinements are now recorded in this SOW.
- Recorded user decisions that:
  - screenshots are deferred until text docs are ready and representative trap
    journal data exists;
  - `enrichment.md` and `usage-and-output.md` are added;
  - security remains in `configuration.md` and links to go.d.plugin Secrets
    Management;
  - shipped query, chart, and export workflows stay in
    `investigation-playbooks.md`;
  - generated integration docs stay under Collecting Metrics with manual
    cross-links;
  - missing topology, Network Flow correlation, and other pending roadmap work
    are internal follow-up items, not public docs gaps.
- Built the Phase 1 implementation evidence inventory:
  - page-by-page facts now cite implementation, generated collector docs,
    profile-reference docs, charts, health alerts, and config schema evidence;
  - runtime-only claims are marked `needs-runtime-verification`;
  - public docs boundaries explicitly exclude missing topology correlation,
    missing Network Flow correlation, richer integration search, and roadmap
    work;
  - stock profile scale was verified from repository artifacts: 803 profile
    files, 6,121 MIB references, and 150,755 trap OIDs.
- Ran six external reviewers against the Phase 1 implementation evidence
  inventory. All returned `ACCEPT_WITH_CHANGES`.
- Folded reviewer-required Phase 1 corrections into this SOW:
  - stronger line evidence for `snmp:traps`, OTLP, decode limits, journal
    storage, profile metrics, retention, and chart facts;
  - added missing facts for event-driven reception, source allowlist defaults,
    stock config, SNMPv3 INFORM/local engine state, fanout semantics, CWE-117
    journal-field protection, and varbind field skip/hash behavior;
  - expanded Phase 1.5 runtime verification requirements before public drafting.
- Folded Phase 1 reviewer rerun corrections into this SOW:
  - marked hub-architecture correlation language as target-state architecture,
    not proof of shipped trap behavior for public docs;
  - split Phase 1.5 runtime checks into `BLOCKS_PUBLIC_DRAFT` and
    `BLOCKS_SPECIFIC_PAGE:<page>` categories;
  - added explicit alert-template to metric-dimension mapping for
    `metrics-and-alerts.md`;
  - corrected imprecise `func_logs_test.go` and allowlist line references;
  - added OTLP preflight source evidence and output-backend OTLP/fanout line
    evidence;
  - added draft guardrails for unsupported decode-time claims and target-state
    correlation language.
- Folded convergence-review corrections into this SOW:
  - split profile metrics into separate config/mode and commit-path inventory
    entries;
  - tightened remaining `func_logs_test.go`, `serialize.go`,
    `health.d/snmp_traps.conf`, OTLP resource-attribute, and Secrets Management
    evidence anchors;
  - recorded the exact sensitive-data scan command;
  - clarified that the `1 ms` grep guardrail is a Phase 3 post-draft check and
    must cover both `docs/snmp-traps` and generated integration docs;
  - removed the unsupported decode-budget/decode-time claims from the generated
    integration page, collector metadata, and durable Netdata trap spec.
- Drafted the foundation SNMP trap docs batch with one page-focused subagent
  per page and an orchestrator integration pass:
  - `docs/snmp-traps/README.md`
  - `docs/snmp-traps/installation.md`
  - `docs/snmp-traps/quick-start.md`
  - `docs/snmp-traps/configuration.md`
  - `docs/.map/map.yaml`
- Folded integration corrections into the foundation batch:
  - removed links to planned pages that do not exist yet;
  - verified package wording against `netdata-plugin-go` packaging evidence;
  - replaced the SNMPv2c example community with scanner-approved fake value
    `example`;
  - split the `journalctl --directory` example to avoid OID false positives in
    the sensitive-data scanner;
  - added the 64-character job-name cap from collector validation;
  - verified the local `snmptrap` command syntax while recording that this is
    not full receiver runtime proof.
- Updated the SNMP Traps overview to make the stock decode coverage a first
  viewport value point: 800+ stock vendor profiles, 6,000+ MIBs, and 150,000+
  trap definitions. Evidence comes from
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json` counts
  recorded in this SOW.
- Corrected `docs/snmp-traps/installation.md` wording that incorrectly said the
  collector "does not auto-detect devices." Public docs now distinguish:
  - trap receiving is explicit and requires a configured `snmp_traps` listener
    job;
  - automatic source-device enrichment works when the same Netdata Agent has a
    successful SNMP collector job for the trap source and the source maps to one
    unambiguous SNMP device registry entry;
  - interface and LLDP/CDP neighbor enrichment require the same Agent's
    `snmp_topology` job to have a successful topology cache for the same
    SNMP-polled device, plus an `ifIndex` in the trap.
  Evidence: `src/go/plugin/go.d/collector/snmp_traps/enrich.go` calls the SNMP
  device registry and topology enrichment paths before writing the trap entry;
  `src/go/plugin/go.d/collector/snmp/ddsnmp/device_registry.go` defines the
  registry as populated by SNMP collector jobs;
  `src/go/plugin/go.d/collector/snmp_topology/topology_trap_enrich.go`
  documents that interface/neighbor enrichment uses the trap `ifIndex` only
  after one source IP matches one local topology cache.
- Tightened `docs/snmp-traps/README.md` to say listener jobs are explicit
  instead of saying trap collection is "not auto-detected." Focused search now
  finds no `auto-detected` or `autodetect` wording in `docs/snmp-traps/`.
- Corrected UDP/162 permission wording in `docs/snmp-traps/installation.md`.
  Public docs now say standard Netdata packages, static installs, and the
  official Docker image handle low-port binding, while custom or hardened
  deployments may need a high port or `CAP_NET_BIND_SERVICE`. Evidence:
  `system/systemd/netdata.service.in` includes `CAP_NET_BIND_SERVICE` in the
  service capability bounding set for `go.d/snmp_traps`;
  `packaging/cmake/pkg-files/deb/plugin-go/postinst` grants
  `cap_net_bind_service` to `go.d.plugin`; `packaging/makeself/install-or-update.sh`
  does the same for static installs; `packaging/docker/Dockerfile` marks
  `go.d.plugin` with setuid permissions in the official image, and
  `packaging/docker/run.sh` starts Netdata from the official entrypoint before
  dropping runtime user privileges.
- Drafted and integrated the meaning docs batch with one page-focused worker per
  page and an orchestrator consistency pass:
  - `docs/snmp-traps/trap-profiles.md`
  - `docs/snmp-traps/enrichment.md`
  - `docs/snmp-traps/usage-and-output.md`
  - `docs/snmp-traps/field-reference.md`
- Folded meaning-batch integration corrections into public docs:
  - `field-reference.md` now identifies `TRAP_DEVICE_VENDOR` as local SNMP
    device registry or topology enrichment, not profile-file metadata;
  - public docs explicitly state that profile-rendered text is stored in the
    journal `MESSAGE` field and that no separate `TRAP_MESSAGE` field exists.
- Drafted and integrated the operations docs batch with one page-focused worker
  per page and an orchestrator consistency pass:
  - `docs/snmp-traps/journal-and-querying.md`
  - `docs/snmp-traps/forwarding-to-siem.md`
  - `docs/snmp-traps/metrics-and-alerts.md`
  - `docs/snmp-traps/validation-and-data-quality.md`
- Folded operations-batch integration corrections into public docs:
  - binary journal encoding wording now points to control characters, invalid
    UTF-8, or binary payload values instead of unsupported field names;
  - sensitive-data cautions avoid screenshot/customer wording and use support
    artifacts, organization names, and environment-identifying public IPs.
- Drafted and integrated the incident docs batch with one page-focused worker
  per page and an orchestrator consistency pass:
  - `docs/snmp-traps/investigation-playbooks.md`
  - `docs/snmp-traps/troubleshooting.md`
  - `docs/snmp-traps/anti-patterns.md`
  - `docs/snmp-traps/sizing-and-capacity.md`
- Expanded `docs/.map/map.yaml` so Learn publishes the complete SNMP Traps
  section with 16 mapped pages:
  - Overview, Installation, Quick Start, Configuration;
  - Trap Profiles, Enrichment, Usage and Output, Field Reference;
  - Journal and Querying, Forwarding to SIEM, Metrics and Alerts, Sizing and
    Capacity Planning, Validation and Data Quality;
  - Investigation Playbooks, Anti-patterns, Troubleshooting.
- Started the production-readiness external review loop requested by the user.
  Review protocol:
  - review one `docs/snmp-traps/*.md` page at a time;
  - run the six configured reviewers in parallel for that page;
  - require an explicit `PRODUCTION GRADE` or `NOT PRODUCTION GRADE` vote;
  - ask reviewers to classify findings as missing information/detail,
    misleading information/detail, confusing or ambiguous information/detail,
    wrong examples/configuration directives, non-end-user information, scope
    drift, and quality score for tone, language, completeness, simplicity,
    friendliness, and professionalism;
  - validate findings against implementation evidence before editing docs;
  - rerun the same page review after fixes until all responding reviewers vote
    `PRODUCTION GRADE`.

## Validation

Acceptance criteria evidence:

- Organization phase complete:
  - Netdata specs remain at `.agents/sow/specs/snmp-traps/`.
  - Research moved under `.agents/sow/specs/snmp-traps/research/`.
  - `.agents/sow/specs/snmp-traps/README.md` documents the spec boundary.
  - `.agents/sow/specs/snmp-traps/research/README.md` documents the research
    boundary.
  - `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` documents
    the organization rule for future agents.
- Foundation documentation batch drafted:
  - `docs/snmp-traps/README.md` introduces traps as pushed network events,
    local journal log entries, optional OTLP log export, and receiver metrics.
  - `docs/snmp-traps/installation.md` covers Linux/go.d prerequisites, explicit
    listener jobs, UDP reachability, UDP/162 capability handling, package
    availability, output preflight, and the exact requirements for automatic
    device, interface, and neighbor enrichment.
  - `docs/snmp-traps/quick-start.md` provides a local UDP/9162 test workflow
    using a synthetic SNMPv2c `coldStart` trap, Logs verification, receiver
    pipeline metrics, and `journalctl --directory` inspection.
  - `docs/snmp-traps/configuration.md` documents listener endpoints, versions,
    communities, SNMPv3 users, source controls, rate limiting, dedup, direct
    journal, OTLP, retention, overrides, and profile metrics.
  - `docs/.map/map.yaml` now publishes the SNMP Traps section with Overview,
    Installation, Quick Start, and Configuration entries.
- Complete text-first documentation page set drafted and mapped:
  - `docs/snmp-traps/README.md`
  - `docs/snmp-traps/installation.md`
  - `docs/snmp-traps/quick-start.md`
  - `docs/snmp-traps/configuration.md`
  - `docs/snmp-traps/trap-profiles.md`
  - `docs/snmp-traps/enrichment.md`
  - `docs/snmp-traps/usage-and-output.md`
  - `docs/snmp-traps/field-reference.md`
  - `docs/snmp-traps/journal-and-querying.md`
  - `docs/snmp-traps/forwarding-to-siem.md`
  - `docs/snmp-traps/metrics-and-alerts.md`
  - `docs/snmp-traps/sizing-and-capacity.md`
  - `docs/snmp-traps/validation-and-data-quality.md`
  - `docs/snmp-traps/investigation-playbooks.md`
  - `docs/snmp-traps/anti-patterns.md`
  - `docs/snmp-traps/troubleshooting.md`

Tests or equivalent validation:

- `git diff --check -- .agents/sow/specs .agents/skills/project-snmp-trap-profiles-authoring/SKILL.md .agents/sow/active/SOW-20260613-snmp-trap-documentation.md`
  passed.
- `git diff --check -- .agents/sow/active/SOW-20260613-snmp-trap-documentation.md`
  passed after the Phase 1 inventory update.
- Sensitive-data scan passed:
  `.agents/sow/scan-sensitive.sh .agents/sow/active/SOW-20260613-snmp-trap-documentation.md .agents/sow/specs/README.md .agents/sow/specs/snmp-traps/README.md .agents/sow/specs/snmp-traps/research/README.md .agents/sow/specs/snmp-traps/netdata-snmp-hub-architecture.md .agents/skills/project-snmp-trap-profiles-authoring/SKILL.md`.
- Foundation docs validation passed:
  - `python3 docs/.map/validate_map_schema.py`;
  - `git diff --check -- docs/snmp-traps docs/.map/map.yaml`;
  - `rg -nP "[^\\x00-\\x7F]" docs/snmp-traps` returned no hits;
  - `rg -n "1 ms|1ms|per-PDU|roadmap|future|not implemented|Network Flow|network flow|correlation|benchmark|screenshot" docs/snmp-traps`
    returned no hits. The shipped topology-enrichment prerequisite is allowed
    in `installation.md`.
  - `.agents/sow/scan-sensitive.sh docs/snmp-traps/README.md docs/snmp-traps/installation.md docs/snmp-traps/quick-start.md docs/snmp-traps/configuration.md docs/.map/map.yaml .agents/sow/active/SOW-20260613-snmp-trap-documentation.md`.
- Local command syntax check passed:
  - `timeout 10 snmptrap -v 2c -c example 127.0.0.1:9162 '' 1.3.6.1.6.3.1.1.5.1`
    exited 0. It printed local Net-SNMP persistent-directory warnings in this
    workstation environment; this verifies command syntax, not full Netdata
    receiver runtime behavior.
- Stock profile scale commands:
  - `find src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default -type f -name '*.yaml' | wc -l`
    returned `803`;
  - `jq 'length' src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json`
    returned `803`;
  - `jq '[.[].mib_count] | add' src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json`
    returned `6121`;
  - `jq '[.[].trap_count] | add' src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json`
    returned `150755`.
- Focused validation after the trap listener/device-enrichment wording fix
  passed:
  - `python3 docs/.map/validate_map_schema.py`;
  - `git diff --check -- docs/snmp-traps/README.md docs/snmp-traps/installation.md .agents/sow/active/SOW-20260613-snmp-trap-documentation.md`;
  - `rg -nP "[^\\x00-\\x7F]" docs/snmp-traps/README.md docs/snmp-traps/installation.md`
    returned no hits;
  - `rg -n "does not auto-detect devices|auto-detect devices|auto-detected|autodetect" docs/snmp-traps`
    returned no hits;
  - `rg -n "1 ms|1ms|per-PDU|roadmap|future|not implemented|Network Flow|network flow|correlation|benchmark|screenshot" docs/snmp-traps`
    returned no hits;
  - `.agents/sow/scan-sensitive.sh docs/snmp-traps/README.md docs/snmp-traps/installation.md .agents/sow/active/SOW-20260613-snmp-trap-documentation.md`.
- Full text-first documentation validation passed:
  - `python3 docs/.map/validate_map_schema.py`;
  - `git diff --check -- docs/snmp-traps docs/.map/map.yaml .agents/sow/active/SOW-20260613-snmp-trap-documentation.md`;
  - `rg -nP "[^\\x00-\\x7F]" docs/snmp-traps` returned no hits;
  - `rg -n "1 ms|1ms|per-PDU|roadmap|future|not implemented|Network Flow|network flow|correlation|benchmark|screenshot|customer" docs/snmp-traps`
    returned no hits;
  - `.agents/sow/scan-sensitive.sh docs/snmp-traps/*.md docs/.map/map.yaml .agents/sow/active/SOW-20260613-snmp-trap-documentation.md`;
  - map/link consistency checks found no missing `docs/snmp-traps/*.md` targets,
    no missing H1 headings, and no missing page-specific `custom_edit_url`
    metadata.
- Final post-review validation after `enrichment.md` and `forwarding-to-siem.md`
  reruns passed:
  - `python3 docs/.map/validate_map_schema.py`;
  - `rg -nP "[^\\x00-\\x7F]" docs/snmp-traps .agents/sow/active/SOW-20260613-snmp-trap-documentation.md`
    returned no hits;
  - `rg -n "[ \\t]+$" docs/snmp-traps docs/.map/map.yaml .agents/sow/active/SOW-20260613-snmp-trap-documentation.md`
    returned no hits;
  - `rg -n "1 ms|1ms|per-PDU|roadmap|future|not implemented|Network Flow|network flow|correlation|benchmark|screenshot|customer" docs/snmp-traps`
    returned no hits;
  - `.agents/sow/scan-sensitive.sh docs/snmp-traps/*.md docs/.map/map.yaml .agents/sow/active/SOW-20260613-snmp-trap-documentation.md`;
  - custom metadata/link sweep found no missing `docs/snmp-traps/*.md` targets, no
    missing H1 headings, and no missing page-specific `custom_edit_url` metadata.
- `.agents/sow/audit.sh` still fails on pre-existing legacy `SOW-0035`-style
  references in durable SNMP trap specs/research. The organization-specific
  checks in the audit passed, but the remaining legacy-reference failure spans
  both top-level specs and moved research files and must be fixed,
  user-excluded, or tracked before SOW completion. The audit also reports the
  expected branch-local active-SOW warning, including the older completed
  `.agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md`.

Real-use evidence:

- Pending.

Reviewer findings:

- Six-reviewer workflow review completed for the analysis workflow and for
  Phase 1 inventory round 1.
- Workflow refinements folded in:
  - add a fixed Phase 1 inventory template;
  - add implementation-file-to-page traceability;
  - define a source-of-truth hierarchy for evidence conflicts;
  - add runtime verification for `journalctl`, `snmp:traps`, OTLP, representative
    trap rows, metrics, alerts, and profile-defined metrics;
  - expand Phase 2 beyond the two playbooks to include domain and comparison
    synthesis, with targeted external-system verification;
  - add research classification, priority, and follow-up SOW fields;
  - iterate external reviewers to convergence, not a single pass;
  - add Phase 3 docs drafting and post-draft consistency validation;
  - resolve page boundaries for enrichment, usage/output, security, implemented
    investigation workflows, journal querying, SIEM forwarding, configuration,
    sizing, trap profiles, and profile-format reference;
  - surface Linux-only direct journal behavior, Cloud-required Function behavior,
    OTLP-only local-query caveats, and sensitive `TRAP_JSON`/varbind/export
    handling;
  - define missing-evidence triggers that create a new SOW before public docs
    depend on unverified claims.
- Phase 1 inventory round 1 corrections folded in:
  - added missing implementation facts for stock config, open source allowlist,
    event-driven reception vs `update_every`, SNMPv3 INFORM/local engine ID,
    persisted engine-boots state, OTLP preflight and TLS/plaintext scheme
    behavior, fanout writer semantics, direct-journal machine/boot ID
    prerequisites, CWE-117 binary encoding, protocol-varbind skip behavior, and
    hashed/truncated journal field names;
  - corrected or strengthened evidence for Function `RequireCloud`, default
    facets/view keys/histogram, Secrets Management map line, decode limits,
    retention units, profile metric caps, OTLP resource attributes, and SIEM/log
    workflow anchors;
  - expanded runtime verification for INFORM, OTLP preflight/failure, fanout
    behavior, copied-journal/journalctl behavior, profile metric identity,
    binary-encoded fields, and local no-journal Function behavior.
- Phase 1 inventory reviewer rerun completed with five responding reviewers
  (`glm`, `minimax`, `kimi`, `mimo`, `deepseek`). All five accepted the SOW for
  Phase 1.5/Phase 2 with changes; required SOW corrections were folded. `qwen`
  was unavailable/unresponsive for this pass and did not return a review.
- Reviewer-discovered cleanup to resolve before SOW completion:
  - legacy `SOW-NNNN` references still make `.agents/sow/audit.sh` fail across
    62 matches in 9 files, including 7 moved research files and 2 top-level
    Netdata-owned spec/decision files;
  - `SOW-20260612-snmp-trap-metrics-docs.md` remains under
    `.agents/sow/active/` even though its top-level status is completed.
  - no file deletion or durable-reference rewrite is performed in this pass;
    cleanup requires explicit user approval or a separately tracked follow-up
    before this SOW can be completed.
- Page-level external review, `docs/snmp-traps/README.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in the final round;
  - required corrections folded before convergence:
    - clarified direct-journal local querying as Linux-only and using
      `journalctl --directory <per-job-dir>`;
    - removed the misleading general Agent Function API claim and documented the
      `snmp:traps` Function as Cloud-required;
    - clarified direct journal vs OTLP-only output behavior;
    - clarified SNMPv3 optional privacy and INFORM acknowledgements;
    - added a sensitive-operational-data warning for trap rows and OTLP exports;
    - expanded the overview navigation and shipped capability inventory,
      including the 800+ vendor / 6,000+ MIB / 150,000+ trap profile pack.
- Page-level external review, `docs/snmp-traps/installation.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in round 7;
  - required and reviewer-suggested corrections folded before convergence:
    - clarified that the default `listen.endpoints` value is an inert commented
      template until a job is uncommented, created, or applied with Dynamic
      Configuration;
    - documented that SNMPv3 listener jobs need `usm_users` plus exactly one
      sender engine ID policy, and that static SNMPv3 jobs using
      `engine_id_whitelist` require `usm_users[].engine_id`;
    - clarified static validation failure vs Dynamic Configuration apply
      failure behavior;
    - used SNMP "community strings" terminology;
    - documented the default-open `allowlist.source_cidrs` behavior and the
      production need to restrict expected sender or relay CIDRs;
    - made OTLP preflight wording distinguish static job startup failure from
      Dynamic Configuration apply failure;
    - verified all package examples include `sudo systemctl restart netdata`.
- Page-level external review, `docs/snmp-traps/quick-start.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in round 4;
  - required and reviewer-suggested corrections folded before convergence:
    - made the `edit-config` command use the standard Netdata configuration
      directory fallback;
    - split Cloud Logs verification from standalone local verification with
      `journalctl`;
    - kept local `journalctl` verification as a first-class path, not an
      optional afterthought;
    - separated the test trap OID from listener-match fields so the listener
      job only matches destination, SNMP version, community, and source CIDR;
    - kept SNMP INFORM wording only where it describes the protocol, not as a
      listener configuration keyword;
    - added links to usage/output, field reference, and production
      configuration next steps;
    - corrected `journalctl --directory` examples to use the effective
      per-job machine-id child directory, including the same-failure fixes in
      adjacent SNMP trap docs.
- Page-level external review, `docs/snmp-traps/configuration.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in final rerun 3; `glm` and
    `deepseek` required retries because the first saved runs did not produce
    usable votes;
  - required and reviewer-suggested corrections folded before convergence:
    - documented direct-journal output as Linux-only and separated local
      `snmp:traps` Function sources from journal row `ND_LOG_SOURCE=snmp-trap`;
    - clarified backend defaults versus common deployment scenarios for direct
      journal, OTLP, both-backend, and OTLP-only jobs;
    - added SNMPv3 dynamic engine ID discovery behavior, privacy-without-auth
      validation, source CIDR empty/omitted behavior, and rate-limit source
      bucket limits;
    - documented OTLP header restrictions, request-timeout preflight/apply
      behavior, and plaintext remote-endpoint warning;
    - clarified retention `rotation_size: 0`, override OID format,
      `profile_metrics.include` validation, raw `source_id` privacy, and
      `drop_and_count` overflow behavior;
    - documented that stock trap profiles provide decode coverage only, and
      profile-derived charts require loaded operator profile YAMLs with
      `metrics:` and `charts:`;
    - clarified that `profile_metrics.include` is ignored when
      `profile_metrics.enabled` is `false`, and documented the exact
      missing-rule and disabled-rule validation messages;
    - documented profile metric update semantics: metrics update only after the
      authoritative output backend accepts the trap, and OTLP export failures do
      not roll back already updated metrics;
    - added trap-payload sensitivity guidance for `MESSAGE`, `TRAP_VAR_*`,
      `TRAP_JSON`, and OTLP attributes;
    - clarified the `journalctl --directory` machine-id child directory as the
      canonical 32-character lowercase hexadecimal machine ID;
    - aligned Secrets Management and SNMP Trap Profile Format cross-page links
      with Learn source-link rules and mapped the profile-format reference under
      the SNMP Traps section.
- Page-level external review, `docs/snmp-traps/trap-profiles.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in final5;
  - required and reviewer-suggested corrections folded before convergence:
    - removed the misleading stock-pack claim that shipped stock profiles include
      trap-to-metric rules or chart definitions;
    - documented that profile metrics come from loaded profile YAML rules and
      produce no charts when no loaded profile defines metric rules;
    - added `TRAP_OID` to the profile output list and clarified enum-backed
      `TRAP_VAR_*_RAW` fields;
    - documented stock profile lazy loading on first matching trap OID;
    - clarified exact-first trap OID lookup plus single `.0.` insertion or
      removal around the final OID arc;
    - documented same-filename operator profile replacement, `extends:` filename
      validation, operator-first resolution, 32-level depth limit, and circular
      `extends:` rejection;
    - documented label-key syntax, bounded templated label sources, and
      profile-load-time rejection of unbounded label templates;
    - linked profile-load-failure diagnostics to Metrics and Alerts;
    - summarized `profile_metrics` selection modes, missing/disabled include-rule
      validation, profile metric update semantics, and OTLP export-failure
      non-rollback behavior;
    - verified that repo-root profile-format links are handled by the Learn
      pipeline's source-link rewrite and the SNMP Trap Profile Format map entry.
- Page-level external review, `docs/snmp-traps/enrichment.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in rerun 3; earlier `kimi` and
    other partial runs were treated as incomplete or stale after page edits;
  - required and reviewer-suggested corrections folded before convergence:
    - changed the opening from unconditional enrichment language to conditional
      local-context language;
    - added `_HOSTNAME` to the first-fields list;
    - clarified that `_HOSTNAME` can come from registry enrichment, topology
      enrichment, or source-address fallback in serialized log rows;
    - documented topology `ND_NIDL_NODE` conflict behavior as
      `TRAP_ENRICHMENT.topology.status=conflict` with reason `vnode_mismatch`;
    - listed common `TRAP_ENRICHMENT.source.rejected_candidates` reason codes for
      operators validating relayed source identity;
    - documented `TRAP_ENRICHMENT.source.udp_peer` and
      `TRAP_ENRICHMENT.source.snmp_trap_address`;
    - tightened `TRAP_ENRICHMENT.source.method` wording to normal production
      values plus the defensive `entry_source` fallback;
    - documented the open default `allowlist.source_cidrs` behavior in the source
      trust model;
    - mapped common audit `method` values to the audit record that emits them;
    - clarified rejected-candidate format as `snmpTrapAddress.0:<reason>`;
    - clarified reverse-DNS cache-miss behavior: current rows are not backfilled,
      later traps may use cached PTR results;
    - sharpened registry wording around the local Netdata device registry and
      selected-source lookup;
    - documented `_HOSTNAME` fallback order and the normal-row write requirement
      for a device hostname, selected source IP, or UDP peer.
- Page-level external review, `docs/snmp-traps/usage-and-output.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in round 2;
  - required and reviewer-suggested corrections folded before convergence:
    - clarified Cloud Logs through the Cloud-required `snmp:traps` Function
      versus local `journalctl --directory` access to direct-journal files;
    - documented exact decode-error kind values and made
      `TRAP_SOURCE_UDP_PORT` conditional on the source port being known;
    - added decode-error `TRAP_CATEGORY` and `TRAP_SEVERITY` behavior;
    - documented `_HOSTNAME` fallback order and called it inventory data;
    - added `PRIORITY`, `ND_NIDL_NODE`, `TRAP_INTERFACE`, and `TRAP_NEIGHBORS`
      to the normal trap row table;
    - clarified pre-write drops as receiver metrics only, not trap rows;
    - corrected binary-encoded guidance to the `binary_encoded` dimension in the
      SNMP trap processing errors chart and selector
      `snmp_trap_errors_binary_encoded`.
- Page-level external review, `docs/snmp-traps/field-reference.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in round 2;
  - required and reviewer-suggested corrections folded before convergence:
    - clarified that `TRAP_SOURCE_IP` and `TRAP_SOURCE_UDP_PEER` are populated on
      `trap` and `decode_error` rows when known;
    - removed duplicate `TRAP_SOURCE_UDP_PORT` placement from Source identity and
      kept it only under Packet audit as decode-error-only;
    - documented that `TRAP_JSON` keeps non-sensitive protocol-control varbinds
      while `TRAP_VAR_*` omits them;
    - documented that `unknown_engine_id` decode errors map to
      `TRAP_CATEGORY=auth`;
    - linked decode-error fields with Packet audit fields so operators see
      `TRAP_LISTENER`, `TRAP_ENGINE_ID`, `TRAP_PACKET_*`, and
      `TRAP_SOURCE_UDP_PORT` as decode-error row fields;
    - tightened `TRAP_INTERFACE` source language to local topology context;
    - clarified OTLP `network.peer.address` fallback and the three
      `snmp.varbinds` payload shapes for normal traps, decode errors, and dedup
      summaries.
- Page-level external review, `docs/snmp-traps/journal-and-querying.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in the final round;
  - required and reviewer-suggested corrections folded before convergence:
    - replaced misleading local-Function wording with Netdata Cloud Logs and the
      Cloud-required `snmp:traps` Function;
    - clarified that OTLP-only jobs do not create local journal files and do not
      appear as `snmp:traps` job sources;
    - kept `journalctl --directory` as the local shell-query path for
      direct-journal jobs;
    - narrowed decode-error raw-packet rationale to community strings or binary
      payloads;
    - documented that `TRAP_JSON` can include protocol-control varbinds that are
      not indexed as `TRAP_VAR_*`;
    - verified `journalctl` examples against local `journalctl --help` and
      standard journald match semantics.
- Page-level external review, `docs/snmp-traps/forwarding-to-siem.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in the final rerun after the
    sensitive-scan placeholder cleanup;
  - required and reviewer-suggested corrections folded before convergence:
    - clarified Cloud-required `snmp:traps` Function source behavior and the
      local-journal implications of OTLP-only mode;
    - documented that direct journal output is Linux-only and fails validation on
      non-Linux when `journal.enabled: true`;
    - corrected local journal command/path examples to use the machine-id child,
      `tr -d '-' < /etc/machine-id`, `sudo`, and `--no-pager`;
    - removed the inaccurate single-file journal filename form and kept the
      shipped `snmp-traps` source prefix with chain naming and an at-sign
      separator;
    - removed the email-like journal filename placeholder that triggered the
      sensitive-data scanner;
    - fixed the Configuration anchor to `#direct-journal-retention`;
    - added OTLP `headers: null` defaults and clarified gRPC metadata header
      validation, secret references, and reserved `grpc-` key rejection;
    - expanded OTLP startup/preflight, gRPC-only, batching, retry, queue
      overflow, no-backoff, no-max-retry, non-durable queue, TLS/system-trust,
      and plaintext-warning behavior;
    - clarified that failed OTLP export batches remain pending and are retried in
      normal operation because `exportPending()` clears the batch only after
      successful export;
    - clarified journal-and-OTLP fanout behavior: the journal backend accepts the
      record before the OTLP backend, so OTLP failures do not remove records
      already accepted by the journal backend;
    - clarified OTLP-only queue-full behavior as terminal loss because there is
      no local journal backstop;
    - expanded OTLP mapping highlights for report types, event names, source and
      UDP peer fallback, decode-error fields, dedup summary fields,
      `network.peer.port`, `snmp.varbinds`, and the dedup-summary omission of
      the `snmp.trap.severity` attribute;
    - clarified `snmpTrapCommunity` omission from indexed `TRAP_VAR_*`,
      `TRAP_JSON`, and `snmp.varbinds`;
    - added full metric selector names for `otlp_export_failed` and
      `journal_write_failed`, and clarified that `otlp_export_failed` covers
      queue-overflow drops as well as gRPC export errors.
- Page-level external review, `docs/snmp-traps/metrics-and-alerts.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in the final rerun after the
    troubleshooting consistency edit and reviewer-requested precision fixes;
  - required and reviewer-suggested corrections folded before convergence:
    - added the profile metric diagnostics chart and dimensions
      `rule_missed`, `extraction_failed`, `attribution_failed`,
      `overflow_dropped`, and `source_transitions`;
    - documented `template_unresolved` health alert coverage and expanded the
      default-alert table to all 20 shipped health templates with exact
      contexts, dimensions, thresholds, and windows;
    - corrected `dropped` semantics so the page states decode-error rows can
      increment `dropped` in parallel with their decode/error dimension;
    - clarified dedup naming: receiver pipeline dimension `dedup_suppressed`
      versus dedup chart dimension `suppressed`;
    - fixed the `binary_encoded` processing-error row to render as a valid
      two-column table row and documented direct-journal-only semantics with
      OTLP-only jobs keeping the counter at zero;
    - changed `decoded` from an "allowed" packet to a parsed trap or INFORM PDU,
      separated pre-decode source/sniff/auth/malformed causes from post-decode
      version/community/engine/rate-limit causes, and documented first-time
      dynamic engine ID registrations as `unknown_engine_id` visibility signals;
    - clarified `write_failed` by output mode and separated pipeline and error
      dimensions: journal-only and journal+OTLP failures increment
      `write_failed` with `journal_write_failed`; OTLP-only synchronous
      queue-full and asynchronous export failures increment `write_failed` with
      `otlp_export_failed`; journal+OTLP secondary OTLP enqueue or export
      failures raise `otlp_export_failed` without raising `write_failed`;
    - documented built-in source receiver cap behavior as 2000 active sources
      with 60 successful collection cycles before inactive-source expiry, and
      separated it from profile-defined metric caps;
    - documented profile metric selection modes `none`, `auto`, `exact`, and
      `combined`, and clarified profile metric update timing after writer
      acceptance, including direct-journal, OTLP-only queued export, and
      synchronous write-failure behavior.
- Page-level external review, `docs/snmp-traps/sizing-and-capacity.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in the final round;
  - required and reviewer-suggested corrections folded before convergence:
    - removed unsupported throughput and wall-clock decode claims pending a
      separate benchmark evidence SOW;
    - clarified pipeline outcome counters versus diagnostic/error counters so
      operators do not treat them as one mutually exclusive breakdown;
    - documented `dropped` as the broad non-committed packet counter, including
      allowlist drops, decode or malformed packets, auth/USM failures, unknown
      engine IDs, rate-limit drop-mode discards, and packet-handling failures;
    - clarified output-mode write-failure semantics for direct journal,
      OTLP-only, and journal-plus-OTLP fanout jobs;
    - documented that `rate_limit.mode: sample` increments `rate_limited` while
      still allowing over-limit traps through the rest of the receiver pipeline;
    - clarified that INFORM acknowledgements are attempted before the rate-limit
      gate;
    - documented dynamic SNMPv3 engine ID table behavior, including first-time
      accepted registrations, full-registry rejection, and the counters to watch;
    - corrected profile metric chart cap configuration to
      `chart_meta.lifecycle.max_instances`;
    - separated source pipeline counters from source attribution overflow
      diagnostics and profile metric cardinality limits;
    - documented direct-journal-only `binary_encoded` behavior and OTLP-only
      zero-counter behavior.
- Page-level external review, `docs/snmp-traps/validation-and-data-quality.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in round 6;
  - required and reviewer-suggested corrections folded before convergence:
    - added first-deployment workflow links to Quick Start and Journal and
      Querying;
    - clarified source/community validation ordering and trusted-relay source
      audit fields, including `TRAP_ENRICHMENT.source.snmp_trap_address`;
    - documented source-health metrics, profile-load health metrics, and
      `MESSAGE` as the rendered trap message field;
    - tightened `TRAP_JSON` sensitivity wording so operators treat it as
      structured payload and audit detail, not as non-sensitive data;
    - labeled `rate_limit.mode` as job configuration and kept `rate_limited` as
      the `snmp.trap.errors` dimension;
    - documented direct-journal validation as Linux-only and added OTLP-only
      source attributes `snmp.source.ip` and `network.peer.address`;
    - corrected enrichment status wording to the implemented `conflict` status
      and added decode-error inspection fields such as `TRAP_SOURCE_UDP_PORT`.
- Page-level external review, `docs/snmp-traps/investigation-playbooks.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in round 2;
  - reviewer-suggested corrections folded before convergence:
    - added an OTLP-only reminder that points operators to downstream log
      attributes and the Field Reference OTLP mapping;
    - added Field Reference to the related-page list;
    - clarified `rate_limited` as the `snmp.trap.errors` dimension in the trap
      storm playbook;
    - changed restart examples to exact `TRAP_NAME=SNMPv2-MIB::coldStart` and
      `TRAP_NAME=SNMPv2-MIB::warmStart`;
    - linked the unknown-OID next action to Trap Profiles;
    - added `TRAP_JOB` to the incident-ticket checklist.
- Page-level external review, `docs/snmp-traps/troubleshooting.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in round 2;
  - required and reviewer-suggested corrections folded before convergence:
    - corrected the pipeline-order decision table so unknown-OID, template, and
      profile issues are documented as data-quality signals on accepted traps,
      not as blockers before the accepted counter;
    - corrected SNMPv1/v2c version and community mismatch troubleshooting to
      separate pre-decode disallowed-version drops from post-decode allowlist
      drops;
    - aligned `metrics-and-alerts.md` accepted-flat wording with the same
      verified pipeline ordering;
    - documented all `snmp.trap.errors` dimensions and clarified
      non-loss diagnostic dimensions such as `inform_response_failed`,
      `binary_encoded`, and `listener_read_failed`;
    - clarified that `edge-traps` in commands is a placeholder listener job
      name;
    - clarified dedup chart evidence as
      `snmp.trap.dedup_suppressed:suppressed`;
    - clarified OTLP-only terminal failures versus journal-plus-OTLP secondary
      export failures.
- Page-level external review, `docs/snmp-traps/anti-patterns.md`:
  - reviewers: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, `qwen`;
  - result: all six voted `PRODUCTION GRADE` in round 3;
  - required and reviewer-suggested corrections folded before convergence:
    - clarified default catch-all `allowlist.source_cidrs` behavior and the
      difference between open-allowlist authentication/decode symptoms and
      allowlist-dependent `dropped_allowlist`;
    - clarified that RFC 5737 ranges are documentation placeholders that must be
      replaced with real deployment ranges;
    - replaced an invalid `netdata:sm_secret:...` example with documented
      `${store:<kind>:<name>:<operand>}` secret-reference syntax;
    - clarified profile-metric symptom framing and the full
      `profile_metrics.identity.source_id_privacy` path;
    - added explicit `rate_limit.mode: drop` validation risk and
      `rate_limit.mode: sample` guidance;
    - replaced editorial system-of-record phrasing with concrete local-journal,
      OTLP-only, and journal-plus-OTLP backend behavior;
    - added accepted SNMP `versions`, `communities`, and SNMPv3 `usm_users` to
      the production checklist.

Same-failure scan:

- Stale old-path scans passed with no hits:
  - `rg -n "\\.agents/sow/specs/snmp-traps/(PLAYBOOK|Skill-Distillation|centreon|checkmk|cribl|datadog-agent|dynatrace|librenms|logicmonitor|logstash|nagios-snmptt|opennms|sensu|snmp-traps-in-observability|solarwinds|splunk-sc4snmp|telegraf|zabbix|zenoss|netdata-existing|comparison/)"`
  - `rg -n "snmp-traps/(PLAYBOOK|Skill-Distillation|centreon|checkmk|cribl|datadog-agent|dynatrace|librenms|logicmonitor|logstash|nagios-snmptt|opennms|sensu|snmp-traps-in-observability|solarwinds|splunk-sc4snmp|telegraf|zabbix|zenoss|netdata-existing|comparison/)"`
- Empty-directory scan found no empty directories under
  `.agents/sow/specs/snmp-traps/`.
- `netdata-existing-netipc` same-failure scan:
  - `rg -n "netdata-existing-netipc" .agents/sow/specs/snmp-traps -g '!**/research/**'`
    returned no hits;
  - `rg -n "netdata-existing-netipc" .agents/sow/specs/snmp-traps`
    returned only research-internal references under
    `.agents/sow/specs/snmp-traps/research/comparison/`.
- Legacy SOW reference scan:
  - `rg -n '\bSOW-[0-9]{4}\b|\.agents/sow/(current|pending|done)/' .agents/sow/specs/snmp-traps`
    returned 62 matches across 9 files;
  - the same command with `-g '!**/research/**'` returned 2 non-research files:
    `.agents/sow/specs/snmp-traps/decisions/0001-go-process-and-trapwriter.md`
    and `.agents/sow/specs/snmp-traps/netdata.md`.

Sensitive data gate:

- Passed for the organization phase. No raw secrets or user-identifying data
  were introduced.

## Artifact Maintenance Gate

- AGENTS.md: no update needed for this organization phase.
- Runtime project skills:
  `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` updated.
- Specs: `.agents/sow/specs/README.md`,
  `.agents/sow/specs/snmp-traps/README.md`, and
  `.agents/sow/specs/snmp-traps/research/README.md` updated.
- End-user/operator docs: complete text-first SNMP trap page set drafted under
  `docs/snmp-traps/` and mapped in `docs/.map/map.yaml`.
- End-user/operator skills: pending.
- SOW lifecycle: pending; this active SOW must be deleted before merge.

Specs update:

- Organization complete for SNMP trap specs and research.

Project skills update:

- SNMP trap profile authoring skill updated with the specs-vs-research rule.

End-user/operator docs update:

- Text-first page set drafted:
  - `docs/snmp-traps/README.md`
  - `docs/snmp-traps/installation.md`
  - `docs/snmp-traps/quick-start.md`
  - `docs/snmp-traps/configuration.md`
  - `docs/snmp-traps/trap-profiles.md`
  - `docs/snmp-traps/enrichment.md`
  - `docs/snmp-traps/usage-and-output.md`
  - `docs/snmp-traps/field-reference.md`
  - `docs/snmp-traps/journal-and-querying.md`
  - `docs/snmp-traps/forwarding-to-siem.md`
  - `docs/snmp-traps/metrics-and-alerts.md`
  - `docs/snmp-traps/sizing-and-capacity.md`
  - `docs/snmp-traps/validation-and-data-quality.md`
  - `docs/snmp-traps/investigation-playbooks.md`
  - `docs/snmp-traps/anti-patterns.md`
  - `docs/snmp-traps/troubleshooting.md`
- Learn map updated:
  - `docs/.map/map.yaml` adds the SNMP Traps section and maps all text-first
    pages.

End-user/operator skills update:

- Pending.

Lessons:

- Pending.

Follow-up mapping:

- Pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Follow-up Issues

- Generated collector metadata/spec correction: completed in this SOW. The
  unsupported `metadata.yaml`, generated integration markdown, and
  `.agents/sow/specs/snmp-traps/netdata.md` "decode time at 1 ms per PDU" /
  per-PDU decode-budget claims were removed. Phase 3 validation MUST run
  `rg -n '1 ms|1ms' docs/snmp-traps src/go/plugin/go.d/collector/snmp_traps/metadata.yaml src/go/plugin/go.d/collector/snmp_traps/integrations .agents/sow/specs/snmp-traps/netdata.md`
  after drafting, allowing only unrelated test-fixture matches outside public
  docs.
- SOW audit cleanup: explicitly excluded from this docs work by user decision.
  The 62 legacy `SOW-NNNN` / legacy SOW-path references across 9 durable SNMP
  trap spec/research files are historical logs of completed work and must not
  block drafting the end-user documentation. If these durable specs are prepared
  for commit and the audit gate must pass, handle this as a separate
  spec-hygiene cleanup.
- SOW lifecycle cleanup: remove the completed
  `.agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md` from active SOW
  storage with explicit user approval before completion/merge readiness.
- Runtime evidence SOW or sub-phase: gather Phase 1.5 evidence for
  `journalctl --directory`, `snmp:traps`, `__logs_sources`, OTLP, representative
  trap rows, alerts, and profile-defined metrics before public docs depend on
  copyable commands or screenshots. This phase must include a screenshot/demo
  trap feed with enough time spread and field variety for Logs UI filters,
  histogram, and search examples.
- Benchmark/sizing evidence SOW: re-measure throughput before publishing trap
  rate, CPU, memory, disk-growth, or high-volume deployment numbers beyond
  conservative defaults and implementation limits. The benchmark should test the
  current implementation against the expected 50K-100K traps/second processing
  range and record hardware, configuration, trap corpus, backend mode, and
  success/failure counters.
