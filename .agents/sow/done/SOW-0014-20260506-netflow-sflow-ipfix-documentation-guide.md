# SOW-0014 - Network Flows Documentation & Integration Infrastructure

## Status

Status: completed

Reopened 2026-05-07 after the netlify deploy preview for learn PR #2852 surfaced major content errors that the prior validation pass missed. The closure on 2026-05-07 (Status: completed) was premature: the docs contained multiple statements that contradicted the source code, generic flow-monitoring advice imported from research notes that did not apply to Netdata, and several invented behaviours. The regression was repaired and revalidated by 2026-05-08; see the `## Regression - 2026-05-07` section and closeout notes at the end of this file.

Reopened 2026-05-08 after PR #22449 review and CI reported additional issues after the SOW had been marked completed and moved to `done/`. The open items are tracked in `## Regression - 2026-05-08` and include automated review threads, `yamllint`, `check-documentation`, Codacy triage, a code-only review subagent requested by the user, and a new user-requested local Learn preview skill/workflow.

Reopened 2026-05-08 after the merged integration artifacts broke downstream
publishing contracts for the website and in-app integrations. The flow
integration schema delegates to collector metadata, but the flow renderer did
not render the collector-style `metrics` and `alerts` sections, leaving raw
objects/arrays in `integrations.json` and `integrations.js`.
The regression was repaired and revalidated on 2026-05-08; see
`## Regression - 2026-05-08 - Flow Integration Section Rendering`.

## Requirements

### Purpose

Provide DevOps/SREs a complete, authoritative documentation and integration infrastructure for Netdata's network flow analysis. Documentation covers the Network Flows feature set that exists in this PR and excludes non-existent behavior such as topology drilldown. The documentation must include sizing, capacity planning, and optimization guidance so businesses can make informed deployment decisions.

### User Request

Phase 1: Document the Network Flows feature set, including sizing/benchmarking/capacity planning and the enrichment integrations exposed through `metadata.yaml`.
Phase 2 (follow-up SOW): Add or revise documentation only for behavior that is not present in this PR, such as a future topology drilldown if it is implemented.

### Assistant Understanding

Facts:

- **Tested features** (unit tests + benchmarks exist):
  - Core collection: NetFlow v5/v7/v9, IPFIX, sFlow decoding and ingestion
  - Basic enrichment: GeoIP (MMDB), static metadata (exporter/interface naming), sampling overrides, static networks, ASN provider chains
  - Classifiers: Akvorado-compatible exporter + interface classification rules (30+ unit tests)
  - Decapsulation: SRv6, VXLAN modes
  - Journal tiering: raw/1m/5m/1h with retention and query guardrails
  - Query engine: flows/autocomplete modes, all views, group_by, selections, facets
  - Frontend: 6-tab visualization (Sankey, Timeseries, Country/State/City Maps, Globe), dashboard cards, filters/facets

- **Untested features** (unit tests for parsing exist, but no integration/e2e tests, never validated with real data):
  - **BMP listener**: TCP listener accepting BMP from real routers, populating routing trie from live BGP sessions. 11 unit tests for message parsing, but TCP listener never tested with a real BMP speaker. Performance impact on netflow ingest path unknown.
  - **BioRIS**: gRPC client for RIPE RIS via bio-rd. 6 unit tests for proto conversion, but never connected to a real RIS endpoint. Can be tested locally (build `cmd/ris/` from bio-rd + BMP speaker), but this setup has never been done.
  - **Network Sources**: HTTP-fetched prefix metadata with jq transforms. 12 unit tests for transform/decode, but HTTP fetch cycle never tested with a real endpoint.
  - **Topology drilldown**: Frontend hook `useFlowsDrilldownData` is dead code (never imported), no "Flows" tab in topology actor modal. Not an untested feature -- it simply does not exist.

- **Benchmark data — README is severely stale**:
  - The README block at `src/crates/netflow-plugin/README.md:309-367` was captured before recent ingest-path optimizations and is no longer representative. It claimed single-core saturation at ~5.8-6.3k flows/s. After optimizations, the plugin saturates at ~49k flows/s low-cardinality and ~43k flows/s high-cardinality on the same workstation (i9-12900K + FireCuda 530, ext4). The "single core" framing is also misleading — the post-decode ingest path is multi-threaded; `cpu_percent_of_one_core` accumulates user+system ticks across all threads divided by wall time, so a saturated host reports >100%.
  - Fresh benchmarks must be run (Phase 1.0) before documentation is written. README must be rewritten with the new numbers as part of Phase 1.0.
  - Benchmark commands shipped with the plugin for re-running on target hardware (`bench_resource_envelope_matrix`, `bench_ingestion_protocol_matrix`, `bench_ingestion_cardinality_matrix`).

- **BMP industry survey** (from mirrored repos):
  - Combined approach (Akvorado, nProbe): BMP + flow enrichment in one binary, in-memory RIB, enrichment only
  - Separate approach (pmacct, GoBMP, OpenBMP): dedicated BMP daemon with DB/persistence, for full BGP monitoring
  - Netdata's implementation follows the Akvorado pattern (enrichment only, in-memory, no persistence)
  - Decision: BMP in netflow-plugin is for enrichment only. Full BGP monitoring per-route would require a separate plugin with its own DB. This is a future consideration, not a Phase 1 concern.

- **All enrichment is in-memory, zero per-flow cost**: GeoIP, static networks, routing trie, network sources -- all background-fetched/stored, pure RwLock read per flow. No HTTP/BGP per flow.

Current gaps (for Phase 1):

- Zero end-user documentation on learn.netdata.cloud
- No metadata.yaml for any flow protocol
- No config_schema.json, no health.d/ alerts, no docs/.map/map.yaml entry
- Dead redirect in LegacyLearnCorrelateLinksWithGHURLs.json

### Acceptance Criteria

Phase 1 only:
- `metadata.yaml` with 3 modules (netflow, ipfix, sflow) validated against integrations schema
- Integrations pipeline generates per-protocol pages (in-app catalog, COLLECTORS.md, learn)
- Learn section "Network Flows" in `docs/.map/map.yaml`
- All pages follow style guide: second person, active voice, sentence case, `:::type`/`:::` admonitions
- Complete field reference (89+2 fields) with per-protocol availability matrix
- Enrichment docs for all features (GeoIP, static metadata, sampling, static networks, classifiers, ASN resolution, BMP routing, BioRIS, Network Sources, decapsulation)
- No mention of topology drilldown, pcap, eBPF, or threat analytics anywhere
- Sizing/capacity planning page sourced from FRESH benchmark measurements (not the stale README) covering NetFlow v9, IPFIX, sFlow at 10 offered rates from 100 to 60000 flows/s, low- and high-cardinality, full pipeline (all-tiers-batched). Includes storage estimation formulas, memory guidance, optimization tips, and a clear note on multi-thread CPU semantics.
- Visualization docs split into focused pages
- Screenshots embedded via GitHub URLs (provided by user)
- AI skills updated with links to learn docs

## Implications And Decisions

### Decision 1: plugin_name -- DECIDED: netflow-plugin

### Decision 2: metadata.yaml scope -- DECIDED: minimal per module

Identity, overview, protocol-specific router config examples, quirks. Deep details in learn section.

### Decision 3: Learn section position -- DECIDED: new top-level "Network Flows"

### Decision 4: Page structure -- DECIDED: more pages, small and focused

```
Network Flows/
  Overview/
  Quick Start/
  Sources/
    NetFlow/
    IPFIX/
    sFlow/
  Configuration/
  Enrichment/
    GeoIP/
    Static Metadata/
    Classifiers/
    ASN Resolution/
    BMP Routing/
    BioRIS/
    Network Sources/
    Decapsulation/
  Field Reference/
  Visualization/
    Summary and Sankey/
    Time-Series/
    Maps/
    Globe/
    Filters and Facets/
    Dashboard Cards/
  Retention and Querying/
  Sizing and Capacity Planning/
  Troubleshooting/
```

~24 pages. Each page focused on one topic. No mentions of pcap, eBPF, threat analytics, or topology drilldown. BMP, BioRIS, and Network Sources are documented based on unit-tested parsing/conversion logic; their runtime I/O paths (TCP listener, gRPC client, HTTP fetch) lack integration tests.

### Decision 5: Future features -- DECIDED: document BMP/BioRIS/Network Sources, skip topology drilldown

BMP, BioRIS, and Network Sources are included in documentation. Their unit-tested parsing logic is solid but runtime I/O paths lack integration tests. The SOW follow-up section tracks the need for async/integration tests.

Topology drilldown remains excluded (dead code -- `useFlowsDrilldownData.js` never imported). pcap, eBPF, and threat analytics remain excluded (not implemented).

### Decision 6: AI skills -- DECIDED: update existing query skills

### Decision 7: Integrations pipeline -- DECIDED: add netflow-plugin to gen_integrations.py

### Decision 9: Documentation rewrite directives -- DECIDED 2026-05-07

User read parts of the research at `.agents/knowledge/Network Traffic Analysis with Flow Data.md`, taught the dashboard mechanics directly, corrected several misconceptions (notably the doubling/mirroring foundational concept and the function-permission paid-only assumption), and instructed:

- **Rewrite, not edit.** Existing pages are thin and must not be inherited as correct. Every claim is re-verified against the code.
- **Code is the source of truth.** The existing markdown, comments, and prior docs are reference points, not authority.
- **Audience-aware, never condescending.** Documentation must serve newcomers without insulting experts. Mental models must be established before features.
- **Three roles served in parallel:** Network Engineer, Security Analyst, IT Manager. Pages stay role-agnostic but every page must be useful to all three.
- **Doubling and mirroring is foundational, not an aside.** Users must understand it before reading any aggregate number.
- **Sampling is documented honestly.** Auto-multiplied at ingestion; mixed rates make aggregates uninterpretable; admins should keep rates uniform or run unsampled.
- **Installation is a first-class topic.** The plugin is packaged separately (`netdata-plugin-netflow` on both DEB and RPM via `netdata.spec.in:3270` and `Packaging.cmake:488`). Not auto-installed by the netdata-updater. Users must install it themselves on native-package systems. Static installs (`kickstart.sh --static-only`) bundle it.
- **Anti-patterns get a dedicated page.** The research's most common deployment failures (ignored sampling, alert on absolute volume, GeoIP firewall of shame, NAT blindness, double-counting, treating duration as latency, microburst hunting) are called out explicitly.
- **Investigation playbooks get a dedicated page.** Concrete walkthroughs from the research's recognition cues (bandwidth saturation, IP investigation, capacity justification, security alert scope) — written for the Netdata UI specifically (Top-N + facets + sankey + maps).
- **Validation & data quality gets a dedicated page.** SNMP cross-check, exporter health monitoring, doubling check, sampling sanity check.
- **No mechanical writing.** Each page begins with code research, an explicit audience statement, an explicit goal statement, and 3-5 key takeaways. Page produced only after the model is settled.

**Scope restored** (changed from earlier interpretation): BMP, BioRIS, and Network Sources are documented based on their unit-tested parsing logic (per Decision 5). Their runtime I/O paths lack integration tests but the features exist in the code and ship in user configurations. Users would otherwise see references to them in `netflow.yaml` without explanation. The follow-up still tracks the integration-test gap.

**Scope confirmed deferred:** topology drilldown (`useFlowsDrilldownData.js`) is dead code — never imported into the topology actor modal — and stays out of documentation until implemented.

**Function permissions verified** at `src/crates/netflow-plugin/src/api/flows/handler.rs:263`: the `flows:netflow` function uses `HttpAccess::SIGNED_ID | SAME_SPACE | SENSITIVE_DATA`. It does NOT include `COMMERCIAL_SPACE`. The agent function is therefore not paid-gated. Any feature gating elsewhere (UI/cloud-frontend) is outside this repository and outside this SOW.

### Decision 10: Promote `flows` to a top-level integration_type with 14 cards -- DECIDED 2026-05-07

User reframed the test for "should X be an integration card?" as "would users ask 'Does Netdata integrate with X?'". This rejected the original architecturally pure framing (one card per protocol decoder under data-collection.networking) in favour of one card per vendor/source users actually shop for.

Sub-decisions:

- **10a Type position:** `flows` is a top-level `integration_type` in `integrations/categories.yaml`, peer of `collector`, `logs`, `exporter`, `notification`, `secretstore`, `authentication`. NOT under `data-collection.networking`. Rationale: flows are a different data model from collector metrics (table-shaped, faceted, time-windowed, journal-backed), the same way logs are -- their integration UX should mirror logs, not collectors.
- **10b Catalog shape:** four sub-categories under `flows` -- Sources, IP Intelligence, BGP Routing, Network Identity Sources -- containing 14 cards total:
  - Sources (3): NetFlow, IPFIX, sFlow
  - IP Intelligence (4): DB-IP IP Intelligence (default), MaxMind GeoIP / GeoLite2, IPtoASN, Custom MMDB Database
  - BGP Routing (2): BMP (BGP Monitoring Protocol), bio-rd / RIPE RIS
  - Network Identity Sources (5): AWS IP Ranges, GCP IP Ranges, Azure IP Ranges, NetBox, Generic JSON-over-HTTP IPAM
- **10c Pipeline plumbing:** `integrations/gen_integrations.py` and `integrations/gen_docs_integrations.py` learn the new type via FLOWS_SOURCES list, FLOWS_RENDER_KEYS, FLOWS_VALIDATOR, `load_flows()`, `render_flows()`, plus a `mode == "flows"` branch in `build_readme_from_integration()`. New `integrations/templates/overview/flows.md` template. New `integrations/schemas/flows.json` ($ref to `collector.json` for now -- can diverge later).
- **10d Map placement:** `docs/.map/map.yaml` carries a top-level Network Flows section using `integration_placeholder` with `integration_kind: flows`, plus three concept docs (IP Intelligence, BGP Routing, Network Identity).
- **10e Schema:** `docs/.map/map.schema.json` enum extended to accept `flows` as an integration_kind value alongside the existing kinds.
- **10f Cross-repo coordination:** companion learn PR (netdata/learn#2854) routes `src/crates/netflow-plugin/integrations/<slug>.md` files into a `flows_entries` DataFrame and splices them over the `flows_integrations` placeholder, mirroring the existing `logs_integrations` handler. cloud-frontend's `data/integrations.js` is auto-generated and committed manually -- a follow-up "Update integrations.js" PR is needed there. www auto-updates daily; no PR required.
- **10g Skill knowledge capture:** `integrations-lifecycle` skill gains a new per-integration_type Learn-routing matrix in `per-type-matrix.md` and a how-to `adding-new-integration-type.md` enumerating the 8-place checklist for future kinds.

### Decision 8: Benchmark rerun matrix -- DECIDED 2026-05-06

Post-optimization rerun supersedes README:309-367. Sub-decisions:

- **8a Protocols:** netflow-v9, ipfix, sflow. Skip netflow-v5 (legacy, unrepresentative of modern deployments).
- **8b Layer:** all-tiers-batched only (full pipeline: raw + 1m + 5m + 1h). The user-facing question is "what does the plugin cost on my host?" — that is the full-pipeline number. Other layers (writer-only, raw-only, minute1-only) are engineering-internal and not republished to users.
- **8c Modes:** both. (i) Paced post-decode resource envelope (`bench_resource_envelope_*`) — produces sizing curves at exact offered rates. (ii) Unpaced full UDP→journal (`bench_ingestion_protocol_matrix`) — produces per-protocol max-throughput numbers including the decode cost. Mode (i) drives the sizing/capacity page; mode (ii) is supplementary and shows decode-path differences between protocols.
- **8d Rate matrix:** 10 offered rates: 100, 500, 1000, 5000, 10000, 20000, 30000, 40000, 50000, 60000 flows/s. The host saturates at ~49k/s low-cardinality and ~43k/s high-cardinality, so the upper end intentionally exercises the saturation plateau. Three protocols × two cardinalities × ten rates = 60 paced cells. Plus 3 unpaced protocol cells × 3 phases (full / decode-only / post-decode) = 9 unpaced cells.
- **8e Hardware:** this workstation. CPU `12th Gen Intel(R) Core(TM) i9-12900K`, storage `Seagate FireCuda 530 NVMe`, ext4. Same rig as the original README capture; comparable baseline.
- **8f Output format and location:** shell driver writes one `RESOURCE_BENCH_RESULT:{json}` line per cell to `<repo>/.local/audits/netflow-bench/results.jsonl`. Renderer produces a markdown summary table per (protocol, cardinality) under the same directory. `.local/` is gitignored — raw outputs are not committed; only the SOW + README + sizing doc carry numbers into the repository.

CPU semantics note (will appear in docs):
- `cpu_percent_of_one_core` accumulates user+system ticks across all threads divided by wall time
- Below saturation it can be <100%
- At saturation on a multi-core host it is well above 100% (e.g. ~600-800% on this workstation when ingest threads + tier-batch threads are all busy)
- Documentation must call this out explicitly so capacity planners read the metric correctly

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Netdata has a fully functional flow collection and visualization system with tested features, but zero documentation and zero integration catalog entries. Four features (BMP, BioRIS, Network Sources, Topology drilldown) are untested or not implemented and must be excluded from documentation until validated.

Evidence reviewed:

- All files listed in previous SOW versions
- Benchmark data: `src/crates/netflow-plugin/README.md:309-367` is STALE (post-optimization saturation ~49k/43k, not ~6k as the README claims). Authoritative numbers come from the Phase 1.0 rerun (matrix per Decision 8). README:309-367 must be replaced as part of Phase 1.0.
- Test coverage: 81 enrichment tests, 11 BMP parsing tests, 6 BioRIS unit tests, 12 network source unit tests -- but zero integration tests for BMP TCP listener, BioRIS gRPC client, or network source HTTP fetch
- BMP industry survey: 14 repos analyzed across mirrored codebase
- BioRIS local testing: possible by building `cmd/ris/` from bio-rd, never done
- Topology drilldown: `useFlowsDrilldownData.js` never imported (dead code)
- Documentation style guide: `docs/developer-and-contributor-corner/style-guide.md` (second person, active voice, sentence case, Oxford comma, `:::type`/`:::` admonitions)

Affected contracts and surfaces:

- `docs/.map/map.yaml` -- new section
- `src/crates/netflow-plugin/metadata.yaml` -- new file
- `integrations/gen_integrations.py` -- plugin_name registration
- Generated integration pages, COLLECTORS.md
- `docs/netdata-ai/skills/query-netdata-cloud/query-flows.md` -- updated
- `docs/netdata-ai/skills/query-netdata-agents/query-flows.md` -- updated
- ~17 new learn pages under `docs/network-flows/`
- `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs` -- add `NETFLOW_RESOURCE_BENCH_PROTOCOL` env var (Phase 1.0 step 1)
- `src/crates/netflow-plugin/README.md:309-367` -- benchmark block fully replaced with fresh post-optimization numbers (Phase 1.0 step 4)
- `<repo>/.local/audits/netflow-bench/` -- new gitignored output directory for raw JSONL + rendered markdown (Phase 1.0 step 2)

Existing patterns to reuse:

- Style guide conventions
- Existing collector metadata.yaml (SNMP, PostgreSQL) as templates
- Integrations-lifecycle skill, learn-site-structure skill
- `src/crates/netflow-plugin/README.md` as source material
- `query-flows.md` skills as source for API examples

Risk and blast radius:

- Documentation-only (except gen_integrations.py plugin_name addition + the small bench harness env-var addition + README replacement)
- Must carefully avoid documenting untested features -- all enrichment docs must be scoped to tested features only
- BMP routing, BioRIS, and Network Sources exist in config examples (netflow.yaml) but must not appear in documentation as usable features until tested
- Bench harness change is additive (new env var, fall-back behavior preserved). No existing test or caller is affected. Verify by running one cell with each protocol value before launching the full matrix.
- Benchmark rerun consumes the workstation for ~25 min wall time (60 paced cells x ~20s + 9 unpaced cells). Host should be quiet during the run; concurrent CPU load skews CPU% and write_bytes/s readings.

Sensitive data handling plan:

- Use `NODE` placeholder instead of real IPs (per style guide)
- Use RFC 5737 documentation ranges in all examples
- No real credentials, tokens, or network topologies

Implementation plan:

1. Phase 1.0: Benchmark rerun (matrix per Decision 8) and README replacement
2. Phase 1A: Integration infrastructure (metadata.yaml + pipeline)
3. Phase 1B: Learn section (map.yaml + ~17 pages, with sizing page sourced from Phase 1.0 fresh measurements)
4. Phase 1C: AI skills update
5. Phase 1D: Validation

Validation plan:

- Run integrations pipeline, verify artifacts
- Verify map.yaml routing
- Cross-reference every config option against plugin_config.rs
- Cross-reference field list against flow/schema.rs
- Cross-reference benchmark numbers against the Phase 1.0 fresh rerun output (`<repo>/.local/audits/netflow-bench/results.jsonl`), NOT against the stale README. README is a downstream artifact updated by Phase 1.0.
- Verify no mention of untested features anywhere

Artifact impact plan:

- AGENTS.md: no update needed
- Runtime project skills: no update needed
- Specs: no spec update needed
- End-user/operator docs: this IS the docs update
- End-user/operator skills: query skills updated (Phase 1C)
- SOW lifecycle: standard flow

Open decisions:

- None. All resolved.

## Plan

### Phase 1.0: Benchmark Rerun + README Refresh

1. Add `NETFLOW_RESOURCE_BENCH_PROTOCOL` env var to `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs`. When set to `netflow-v9`, `ipfix`, or `sflow`, route `build_record_batches` to the matching entry in `PROTOCOL_SCENARIOS`. When unset, fall back to the existing `CARDINALITY_SOURCE_SCENARIO` mixed behavior so legacy callers are unaffected. Pass the new env var through `run_resource_envelope_case` to the child process.
2. Write `<repo>/.local/audits/netflow-bench/run.sh` driver script:
   - Iterate protocols `{netflow-v9, ipfix, sflow}` x cardinalities `{low, high}` x rates `{100, 500, 1000, 5000, 10000, 20000, 30000, 40000, 50000, 60000}`.
   - Per cell: run `cargo test -p netflow-plugin --release ingest::resource_bench_tests::bench_resource_envelope_child -- --ignored --nocapture --exact --test-threads=1` with env vars `NETFLOW_RESOURCE_BENCH_CHILD=1`, `NETFLOW_RESOURCE_BENCH_LAYER=all-tiers-batched`, and the per-cell protocol/profile/rate.
   - Capture the `RESOURCE_BENCH_RESULT:{json}` line into `results.jsonl` (one line per cell, including a `protocol` field).
   - Run `bench_ingestion_protocol_matrix` once for the unpaced 3B output; capture stderr report blocks into `protocol_matrix.txt`.
   - Render per-(protocol,cardinality) markdown tables (`results.md`).
   - Use the `run()` visibility wrapper from `~/.claude/CLAUDE.md` so commands are echoed.
3. Verify host quietness before running (`top` / `ps`). Re-run any cell that fails or that shows pacer underrun for spurious reasons.
4. Replace `src/crates/netflow-plugin/README.md:309-367` with the fresh numbers, including:
   - Per-protocol per-cardinality tables for the 10 rates (3A).
   - Per-protocol max-throughput table from 3B.
   - Multi-thread CPU semantics note.
   - Storage estimation hints derived from `write_bytes_per_sec` and `logical_write_bytes_per_sec` ratios.

### Phase 1A: Integration Infrastructure

1. Add `netflow-plugin` to `gen_integrations.py` recognized plugin names
2. Create `src/crates/netflow-plugin/metadata.yaml` with 3 modules
3. Create icon assets
4. Run pipeline, verify generated pages

### Phase 1B: Learn Section (full rewrite -- ~25 pages -- per Decision 9)

Per-page method, applied uniformly:

1. **Subagent for technical analysis first.** Each feature/section gets a dedicated read-only subagent that explores the relevant code under `src/crates/netflow-plugin/` (and adjacent crates / Go tools where applicable), identifies the happy path and the nuances, enumerates configuration options with their defaults, traces error paths, and returns a structured analysis with file:line citations. The master assistant synthesizes the doc from the analysis. This keeps raw code-reading noise out of the master context and ensures each page is grounded.
2. **Bottom-up writing order.** Detail pages are written first; the `Overview` is written last as the natural index over already-established truths. This avoids the trap of writing an Overview that promises behavior the detail pages later contradict.
3. **State audience and goal per page.** Each page begins with an internal "audience / goal / key takeaways" plan recorded in the SOW execution log before the page is written.
4. **Verify each claim.** No statement reaches the page without a file:line citation in the analysis returned by the subagent.
5. **Mental model before features.** Doubling/mirroring, sampling semantics, tier transparency are foundational and explained before any aggregate number is shown.
6. **Three audiences served.** Network Engineer / Security Analyst / IT Manager all find their answers without the page wearing a role label.

Bottom-up writing order:

Tier A -- leaves (independent, parallelisable subagents):
- `sources/netflow.md`, `sources/ipfix.md`, `sources/sflow.md` -- per-protocol decoder analysis
- `configuration.md` -- whole `plugin_config/` schema
- `field-reference.md` -- `flow/` schema enumeration
- `retention-querying.md` -- tier model + query tier-selection logic
- 8 enrichment pages -- each enrichment module under `src/crates/netflow-plugin/src/enrichment/` and runtime initialization
- 5 visualization pages -- UI behavior plus the corresponding query/response paths under `src/crates/netflow-plugin/src/api/flows/` and `query/`
- `troubleshooting.md` -- error paths, exposed plugin metrics, log message inventory

Tier B -- synthesis (depend on Tier A):
- `anti-patterns.md` -- distilled from research §6 + Tier A findings
- `validation.md` -- distilled from research §11 + Tier A operational metrics
- `investigation-playbooks.md` -- four scenarios that exercise the visualizations, facets, and tiers documented in Tier A
- `installation.md` -- standalone but depends on `configuration.md` for post-install verification

Tier C -- top:
- `quick-start.md` -- the first-time path through Tiers A and B
- `README.md` -- Overview (final synthesis, the natural index)

Page list (final):

Root level:
- `README.md` -- Overview (rewrite). Mental model, what flow data is, what it answers and what it does not, doubling/mirroring foundational concept, sampling caveat, prerequisites pointer, installation pointer, navigation.
- `installation.md` -- Installation (NEW). Package names per distro, post-install verification, file locations, dependencies, source-build caveats. Confirms `netdata-plugin-netflow` is the canonical name on DEB and RPM and that it is opt-in on native-package systems.
- `quick-start.md` -- Quick Start (rewrite). Three-step path, router config examples (NetFlow v9, IPFIX, sFlow), dashboard first-look, doubling/mirroring read-this-first section, verification step.
- `configuration.md` -- Configuration (rewrite). Listener, protocols, journal layout and retention, decapsulation, performance tuning (UDP buffer sysctls, sync interval, record_pool_size).
- `field-reference.md` -- Field Reference (rewrite). All fields organised by category, per-protocol availability matrix, usage hints per field.
- `retention-querying.md` -- Retention and Querying (rewrite). Tier model, how queries auto-pick tiers, IP/port loss in tiers 1-3 and the "filter on IP forces tier 0" behavior.
- `sizing-capacity.md` -- Sizing and Capacity Planning (already done in Phase 1.0).
- `validation.md` -- Validation and Data Quality (NEW). SNMP cross-check, doubling sanity check, sampling sanity check, exporter health monitoring, the silent-failure list.
- `investigation-playbooks.md` -- Investigation Playbooks (NEW). Walkthroughs: bandwidth saturation, IP investigation, capacity justification, security alert scope. UI-specific (Top-N, facets, sankey).
- `anti-patterns.md` -- Anti-patterns and Pitfalls (NEW). Doubled aggregate, ignored sampling, GeoIP for internal IPs, absolute thresholds, collect-and-ignore, flows-vs-sessions, NAT blindness, geographic firewall of shame, duration-as-latency, microburst hunting.
- `troubleshooting.md` -- Troubleshooting (rewrite). Plugin not running, no data arriving, partial data, template errors, GeoIP gaps, performance issues.

Sources:
- `sources/netflow.md` -- NetFlow v5/v7/v9. Protocol semantics, template lifecycle (v9), active timeout best practice, configuration examples for major vendors.
- `sources/ipfix.md` -- IPFIX. Protocol semantics, IE handling, template withdrawal, biflow rarity, vendor configuration examples.
- `sources/sflow.md` -- sFlow v5. Fundamentally different from NetFlow — packet samples + counter samples, sampling rate inherent, semantics of byte counts.

Enrichment:
- `enrichment/geoip.md` -- GeoIP via MMDB. MaxMind database management, internal-IP trap, validation.
- `enrichment/static-metadata.md` -- Static metadata (exporter naming, interface descriptions, custom labels). Foundational for multi-exporter analysis.
- `enrichment/classifiers.md` -- Akvorado-style classifier rules. Rule syntax, when they fire, debugging.
- `enrichment/asn-resolution.md` -- ASN resolution. Provider chains, fallback, validation.
- `enrichment/bmp-routing.md` -- BMP listener (Decision 5: in scope; runtime I/O lacks integration tests, documented based on parsing logic). What BMP is, what BMP enrichment provides, configuration, integration-test caveat.
- `enrichment/bioris.md` -- BioRIS gRPC client (same in-scope/caveat as BMP).
- `enrichment/network-sources.md` -- HTTP-fetched prefix metadata with jq transforms (same in-scope/caveat).
- `enrichment/decapsulation.md` -- SRv6, VXLAN inner-packet extraction. Modes, when each applies.

Visualization:
- `visualization/summary-sankey.md` -- The default landing view. How to read sankey + table together, field selection, top-N control, sort by bytes/packets, doubling-when-unfiltered.
- `visualization/time-series.md` -- Top-N over time. When to use, baselines, time-shifted comparison.
- `visualization/maps-globe.md` -- Country/state/city maps + 3D globe. Tooltips, no drill-down, GeoIP traps.
- `visualization/filters-facets.md` -- Filter ribbon + autocomplete + FTS + selections persistence + URL sharing. AND-of-ORs semantics.
- `visualization/dashboard-cards.md` -- Operational metrics for the plugin itself. What to watch for plugin health.

### Phase 1B (legacy, superseded by per-page list above)

1. Add "Network Flows" section to `docs/.map/map.yaml`
2. Clean up dead redirect in `LegacyLearnCorrelateLinksWithGHURLs.json`
3. Write pages (Overview, Quick Start, Sources/NetFlow, Sources/IPFIX, Sources/sFlow, Configuration, Enrichment/GeoIP, Enrichment/Static Metadata, Enrichment/Classifiers, Enrichment/ASN Resolution, Enrichment/Decapsulation, Field Reference, Visualization/Summary+Sankey, Visualization/Time-Series, Visualization/Maps, Visualization/Globe, Visualization/Filters+Facets, Visualization/Dashboard Cards, Retention and Querying, Sizing and Capacity Planning, Troubleshooting)

### Phase 1C: AI Skills Update

1. Update both query-flows.md skills (links, field list, Globe view)
2. Update SKILL.md files if needed
3. Add how-tos for common patterns

### Phase 1D: Validation

1. Pipeline artifacts verified
2. Map.yaml routing verified
3. MDX compliance checked
4. Cross-reference against source code
5. Verify zero mentions of untested features

## Execution Log

### 2026-05-06

- Created SOW from comprehensive analysis
- Deep-dived into: fields, enrichment, integrations pipeline, GeoIP, BMP, BioRIS, network sources, topology drilldown, documentation style/patterns
- BMP industry survey: 14 repos analyzed. Combined approach (Akvorado pattern) confirmed for enrichment use case
- BioRIS: can run locally (build cmd/ris/ from bio-rd), never tested
- Confirmed: topology drilldown is dead code (never imported)
- Original benchmark data taken from README: 5k flows/s sustainable, ~6k saturation on i9-12900K (NOW KNOWN STALE — see entry below)
- User raised testing concerns: BMP, BioRIS, Network Sources are untested, must not be documented
- Agreed phased approach: Phase 1 documents tested features + sizing, Phase 2 tests + documents untested features
- All 7 decisions resolved

### 2026-05-06 (later) - benchmark rerun finding

- Discovered the README benchmark block (`src/crates/netflow-plugin/README.md:309-367`) is severely stale: post-optimization the post-decode ingest path is much faster than the README claimed (~5.8-6.3k flows/s saturation). The "single core" framing was correct in principle (the post-decode hot path is single-threaded) but the absolute numbers were obsolete.
- Inspected the existing benchmark harness: `ingest_resource_bench_tests.rs` (paced post-decode), `ingest_bench_tests.rs::bench_ingestion_protocol_matrix` (unpaced full UDP→journal), `ingest_resource_bench_support.rs` (`ResourceEnvelopeReport` records achieved flows/s, CPU%, peak/final RSS, read/write bytes/s — exactly the four metrics the user asked to record).
- Confirmed the harness is configurable via env vars for cardinality (`..._PROFILE`), rate (`..._FLOWS_PER_SEC`), layer (`..._LAYER`), warmup, measure window, and pool sizes. The harness is NOT today configurable per-protocol — `bench_resource_envelope_*` always uses `CARDINALITY_SOURCE_SCENARIO` (mixed). Added `NETFLOW_RESOURCE_BENCH_PROTOCOL` env var that routes to a specific entry in `PROTOCOL_SCENARIOS` when set, falling back to the mixed default otherwise. Added `protocol` field to `ResourceEnvelopeReport` so the JSON output is self-describing.
- User decided rerun matrix (Decision 8): netflow-v9+ipfix+sflow, all-tiers-batched layer, both paced + unpaced modes, 10 rates from 100 to 60000 flows/s, this workstation, JSONL+markdown output under `.local/audits/netflow-bench/`.

### 2026-05-06 (later still) - benchmark rerun results

Driver: `.local/audits/netflow-bench/run.sh` + `render.py`. Outputs: `results.jsonl` (60 paced cells), `protocol_matrix.txt` (3B unpaced), `results.md`. Wall time: 22 min for 3A + 5s for 3B. Host load was nominal (load average ~3.5 on 16 cores; user accepted).

Key findings:

- **High-cardinality saturation is around 30 000 flows/s post-decode for all three protocols.** Above the knee, achieved rate plateaus while offered grows. CPU pins at ~98-99% of one core at saturation, confirming the post-decode hot path is single-threaded (CPU does not exceed 100% even when more rate is offered).
- **Low-cardinality saturation is above 60 000 flows/s** for all three protocols — never reached within the chosen matrix. CPU at 60k offered: ipfix 64.1%, netflow-v9 70.3%, sflow 87.0%. Extrapolated single-core ceiling: ~85-100k for v9/ipfix, ~70k for sflow.
- **3B unpaced full UDP→journal peaks**: NetFlow v9 ~99k flows/s, IPFIX ~107k flows/s, sFlow ~88k flows/s. Decode-only is much faster (0.8-2.4M flows/s) — decode is roughly 10% of the full path cost.
- **User's earlier "49k/43k cap" did not appear** in the protocol-isolated rerun. That number was from the older mixed-scenario run; protocol-isolated low-card is faster than 49k and high-card is slower than 43k (~30k).
- **Practical published number**: ~20-25k flows/s for "everything except enrichment" (UDP receive + decode + 4 tiers + high cardinality, mixed protocols). 20k = conservative, 25k = optimistic. Derivation: 30k post-decode ceiling minus ~10 µs/flow for decode brings the full-path ceiling to ~22-25k. UDP socket receive at 1-5k pps is below typical socket limits and not a concern.
- **CPU semantics revisited**: cpu_percent_of_one_core reports >100% only when multiple threads contribute. The plateau at 98-99% is the single-thread limit on the post-decode ingest hot path. Multi-threading work above that ceiling is a future optimization, not a current capability.

Artifacts updated as part of this rerun:

- `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs` — added `NETFLOW_RESOURCE_BENCH_PROTOCOL` env var and threaded it through child spawn
- `src/crates/netflow-plugin/src/ingest_resource_bench_support.rs` — added `protocol` field to `ResourceEnvelopeReport`
- `src/crates/netflow-plugin/README.md:309-...` — fully replaced benchmark section with fresh per-protocol/cardinality tables, 3B unpaced summary, multi-thread CPU semantics note
- `docs/network-flows/sizing-capacity.md` — replaced stale numbers with fresh tables, added the 20-25k headline
- `.local/audits/netflow-bench/` — driver script, renderer, raw JSONL, rendered markdown (gitignored)

### 2026-05-07 - storage footprint benchmark

User asked for a benchmark that measures actual on-disk storage growth, write amplification, and the dedup/cardinality effect. Replaces the "flows × bytes/flow × time" table that I had wrongly written into sizing-capacity.md (the journals are indexed and dedup-aware, so that calculation is invalid).

Implementation:
- New test `bench_storage_footprint_child` in `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs`. Reuses `run_paced_plugin_loop` in segments equal to the configured sample interval; between segments samples per-tier on-disk size via `journal_dir_size_bytes`, real I/O via `/proc/self/io`, logical encoded bytes via metrics deltas, RSS via `/proc/self/status`.
- New types `StorageFootprintSample` and `StorageFootprintReport` in `src/crates/netflow-plugin/src/ingest_resource_bench_support.rs`.
- New env vars: `NETFLOW_STORAGE_BENCH_DURATION_SECS` (default 900), `NETFLOW_STORAGE_BENCH_SAMPLE_INTERVAL_SECS` (default 30). Reuses the existing `..._FLOWS_PER_SEC`, `..._PROFILE`, `..._PROTOCOL` env vars.
- Driver: `.local/audits/netflow-bench/run-storage.sh` runs one cell per cardinality and renders markdown.
- Renderer: `.local/audits/netflow-bench/render-storage.py` produces per-cell growth tables and a dedup-ratio summary.

Results on the same workstation, ipfix at 10 000 flows/s, 15 min per cardinality:

- Low cardinality: 9.00M flows ingested, 6.46 GiB on-disk total, 771 bytes/stored-flow, write amplification 1.79×.
- High cardinality: 8.97M flows ingested, 7.29 GiB on-disk total, 872 bytes/stored-flow, write amplification 2.00×.
- Dedup ratio (high / low): only 1.13× — high cardinality stores 13% more per flow despite 16× more unique field combinations. Real-world traffic with repeated patterns will trend closer to the low-cardinality figure.
- Raw tier dominates at this timescale (≥99% of total). Rollups (1m: 8-112 MiB, 5m: 8-40 MiB, 1h: 0-16 MiB) are tiny because each rollup row aggregates many raw flows. 1-hour tier did not roll over for low-cardinality within 15 min; it appeared in the high-cardinality run at t=660s onward.

Sizing/capacity doc updated to remove the bogus "flow × bytes × time" table and replace it with the empirical numbers above plus retention-bounded storage planning guidance.

Outputs (gitignored):
- `.local/audits/netflow-bench/storage-low.json` — full sample stream for low-cardinality cell
- `.local/audits/netflow-bench/storage-high.json` — full sample stream for high-cardinality cell
- `.local/audits/netflow-bench/storage.md` — rendered markdown

### 2026-05-07 (later) - documentation rewrite, branch, and PR opened

- Phase 1B documentation rewrite landed on branch `netflow-plugin-docs-and-bench` over three commits:
  - `c708101e` -- per-protocol benchmark + storage footprint test
  - `1455d59f` -- docs: rewrite Network Flows documentation (~25 pages)
  - `e61c72b7` -- integrations: add netflow-plugin (netflow, ipfix, sflow modules)
- Architectural pivot per Decision 10: `6d72b5ab` introduced `flows` as top-level integration_type with 14 cards.
- Draft PR netdata/netdata#22439 opened against master.

### 2026-05-07 (later) - autocomplete bug fix

User reported autocomplete dropdown was useless for AS_NAME searches: typing "Akamai" returned no results because every value is rendered as `AS{n} {Organisation}` and the backend was prefix-matching only.

Investigation traced the bug to `facet_runtime/store.rs::TextValueStore::prefix_matches` and `facet_runtime/sidecar.rs::search_sidecar`, both using `starts_with`. Fix landed on the same branch with three rounds of codex review:

- Round 1: introduced substring matching for text-typed facets, kept prefix for IP/numeric. Codex flagged: async runtime blocking, broader-than-AS_NAME effect, naive substring + no length cap, stale docs.
- Round 2: per-field policy via `AutocompleteMatchKind { Prefix, Substring }` on `FacetFieldSpec` (so future per-field overrides are possible without per-kind churn), `memchr::memmem::Finder` for substring search, 256-byte term cap, autocomplete moved to `spawn_blocking`, stale docs updated. Codex flagged blocker: term cap applied to all modes, not just autocomplete.
- Round 3: term cap scoped to `mode == Autocomplete`. Regression test added for non-autocomplete long term. Codex returned "ready to ship for the reviewed autocomplete scope".

Architectural rule recorded by user: "This affects autocomplete only, not regular facets matching. Only autocomplete. Because we want key=value or key in values, to use indexes, not scan." Verified in code: substring path is unreachable from selections/filters; only `mode=autocomplete` calls `FacetRuntime::autocomplete`.

Commit: `b733037a`. 9 new tests, full crate 427 passed.

Follow-ups (recorded in Followup section): case-sensitive matching today (Akamai vs akamai); autocomplete substring on FST sidecars is bounded by limit early-exit but still streams keys for rare/no-hit terms over very large archived vocabularies.

### 2026-05-07 (later) - documentation enhancements

- Screenshots from user (7 GitHub asset URLs) embedded across `summary-sankey.md` (2), `time-series.md` (1), `maps-globe.md` (4). Commit `26f8b978`.
- Master alphabetical field index added at the bottom of `field-reference.md`. 91 rows. Each row carries: type, per-protocol availability (✓/◐/—), source class (decoder / enrichment / both), tier preservation (raw / all), selectivity (facet, group-by, filter, metric, time, hidden), and the enrichment chain or IE mapping. Subagent built the data set from code (rollup field defs, RAW_ONLY_FIELDS, facet catalog, decoder IE maps).
- `docs/network-flows/visualization/filters-facets.md` and `docs/network-flows/retention-querying.md` updated to describe the per-field autocomplete policy and the autocomplete-vs-selection distinction.

### 2026-05-07 (later) - schema fix and learn PR

- CI surface: `check-documentation` job rejected the netdata PR because `docs/.map/map.schema.json` enum did not yet include `flows` as an `integration_kind`. Schema extended; commit `2c3ab0fc`. CI then turned green for that job.
- Learn PR (netdata/learn#2854) opened to teach `ingest/ingest.py` how to (a) categorise files at `src/crates/netflow-plugin/integrations/<slug>.md` into a new `flows_entries` DataFrame, (b) splice them over the `flows_integrations` placeholder. Mirrors the existing logs handler. Verified locally: 65 Network Flows rows are spliced into `ingest/generated_map.yaml` under the four sub-categories; no `flows_integrations` placeholder remains; ingest exits 0. PR merged by user.

### 2026-05-07 (later) - SOW close

- Re-run check on netdata PR #22439 confirmed `check-documentation` passes (after learn merge plus our schema fix).
- Remaining red CI checks (Codacy, SonarCloud, Build Windows) verified unrelated:
  - Codacy: 1107 markdownlint findings, 100% style-only (MD013 line-length, MD033 inline HTML, MD045 alt text). Pre-existing baseline; recently merged PR #22432 also "fails" Codacy. Project does not gate on this.
  - SonarCloud: same pattern, ignored as gate.
  - Build Windows: `urllib HTTPError 403: rate limit exceeded` during packaging step. GitHub API rate limit, transient/infrastructural; unrelated to this work.
- User decision 2026-05-07: Phase 1C (AI skills cross-links to learn docs) is not mandatory. Acceptance criterion for Phase 1C is rejected with this reasoning recorded in Validation. SOW moved to `done/`.

## Validation

### Acceptance criteria evidence

- **metadata.yaml validated**: 14 modules under `plugin_name: netflow-plugin` (3 sources + 4 IP intelligence + 2 BGP routing + 5 network identity). Pipeline runs `gen_integrations.py` + `gen_docs_integrations.py` to exit 0; 14 generated `.md` files under `src/crates/netflow-plugin/integrations/`.
- **Integrations pipeline**: `flows` rendered correctly under its own type. `integrations.json`, `integrations.js`, `COLLECTORS.md` updated. `integrations.json` carries 14 entries with `integration_type: flows` distributed across 4 sub-categories.
- **Learn section**: `docs/.map/map.yaml` carries the Network Flows top-level section with `integration_placeholder integration_kind: flows`. Schema (`docs/.map/map.schema.json`) accepts the value. Learn `ingest/ingest.py` (PR #2854, merged) routes the markdown files into `flows_entries` and splices them over the placeholder. Local ingest run produced 65 Network Flows rows with no remaining `flows_integrations` placeholder.
- **Style guide**: pages are second person, active voice, sentence case. No `:::type` admonitions used; markdown-only by user direction.
- **Field reference**: 91 fields documented by category plus a master alphabetical index with type, per-protocol availability, source class, tier preservation, selectivity, and enrichment chain per row.
- **Enrichment docs**: GeoIP / static metadata / sampling / static networks / classifiers / ASN resolution / BMP routing / BioRIS / Network Sources / decapsulation. BMP, BioRIS, Network Sources documented based on unit-tested parsing logic; their runtime I/O paths still lack integration tests (followup).
- **No mention of untested features**: verified via grep -- no references to topology drilldown, pcap (only as a debugging tool name), eBPF, or threat analytics.
- **Sizing / Capacity planning**: `docs/network-flows/sizing-capacity.md` sourced from `.local/audits/netflow-bench/results.jsonl` (Phase 1.0) and `.local/audits/netflow-bench/storage-{low,high}.json` (Phase 1.0b). Includes the 20-25k flows/s headline, multi-thread CPU semantics note, write-amplification numbers, dedup ratio.
- **Visualisation docs**: 5 pages -- `summary-sankey.md`, `time-series.md`, `maps-globe.md`, `filters-facets.md`, `dashboard-cards.md`. Maps and globe consolidated into one page since they share the data path.
- **Screenshots**: 7 user-provided GitHub asset URLs embedded across the visualisation pages.

### Reviewer findings (codex)

Three rounds of read-only review over the autocomplete fix. Round 1 surfaced async runtime blocking, broader-than-AS_NAME effect, naive substring, stale docs. Round 2 surfaced a blocker (term cap was global, should be autocomplete-only). Round 3 returned "ready to ship for the reviewed autocomplete scope" with no new blockers.

### Same-failure search

- Other text facets affected by the same prefix-only autocomplete bug: `EXPORTER_NAME`, `IN_IF_DESCRIPTION`, `*_NET_NAME`, `SRC_MAC`, `DST_MAC`, `DST_AS_PATH`, `DST_COMMUNITIES`, country/state/city. Per-field policy fixes them all in the same pass.
- Other places stale "prefix" or "in-memory" claims about autocomplete: `filters-facets.md:38`, `retention-querying.md:104` -- both updated.
- Other places where the project might gate substring on autocomplete vs selection: confirmed selections use exact equality (`FacetStore::contains_value_ref`); substring path is unreachable from filtering.

### Artifact maintenance gate

- **AGENTS.md**: no update needed.
- **Runtime project skills**: `.agents/skills/integrations-lifecycle/` updated -- new `per-type-matrix.md` Learn-routing matrix and `how-tos/adding-new-integration-type.md` 8-place checklist. `INDEX.md` cross-linked.
- **Specs**: no spec update needed; project is incrementally bootstrapped and netflow-plugin specs were not pre-existing.
- **End-user / operator docs**: this IS the docs update -- ~25 pages under `docs/network-flows/` plus the netflow-plugin README benchmark refresh.
- **End-user / operator skills (Phase 1C)**: REJECTED by user 2026-05-07 with reasoning "skills linking to docs is not mandatory". `query-netdata-cloud/query-flows.md` and `query-netdata-agents/query-flows.md` remain at their pre-SOW state. Cross-linking to learn docs can be added in a future skills-maintenance pass without blocking this SOW. NOT tracked as a follow-up SOW because it is not a deferred feature -- it is an explicit scope rejection.
- **SOW lifecycle**: status moved to `completed`, file moved to `done/`, in the same commit as the autocomplete fix lands on the active PR branch.

### Status / directory consistency

Status: `completed`. Directory: `done/`. Filename unchanged.

### Lessons captured

See `## Lessons Extracted` below.

## Outcome

Delivered:

- 4 commits on netdata branch `netflow-plugin-docs-and-bench` (PR #22439, draft):
  - `c708101e` per-protocol benchmark + storage footprint test
  - `1455d59f` Network Flows documentation rewrite (~25 pages)
  - `e61c72b7` netflow-plugin metadata.yaml (3 modules) and integration cards
  - `6d72b5ab` `flows` top-level integration_type with 14 cards
  - `b733037a` substring autocomplete on text facets (3 codex rounds, 9 new tests, 427 pass)
  - `2c3ab0fc` `docs/.map` schema accepts `flows`
  - `26f8b978` screenshots and master field index
- 1 commit on learn branch `netflow-flows-integrations` (PR #2854, MERGED 2026-05-07):
  - `de62daaf` ingest: route flows integrations into the Network Flows section
- Companion website branch `netflow-flows-content` carries content corrections (separate, smaller).

Pending only:

- Re-trigger Build Windows on netdata PR #22439 (transient `urllib` HTTP 403 rate limit, not our code).
- Mark netdata PR #22439 ready for review when user decides.
- Cloud-frontend "Update integrations.js" PR -- standard manual sync, not gated by this SOW.

## Lessons Extracted

- **Code is ground truth, not the existing markdown**: the inherited netflow docs were thin and contained inaccuracies. Re-verifying every claim against `src/crates/netflow-plugin/` paid off -- multiple "well-documented" behaviours (e.g. AS name format, sFlow VLAN provenance, "single-core" benchmark framing) turned out to be wrong or stale.
- **Doubling/mirroring is foundational, not a footnote**: users cannot reason about ANY aggregate number unless they understand that one router watching ingress + egress doubles every flow. This had to be the first concept on the Overview, not an "advanced" sidebar.
- **Subagent per feature, master assistant for synthesis**: spawning a read-only subagent per enrichment module / visualisation / source kept the master context clean. The Overview was written last as a natural index of established truths -- not first as a promise the detail pages later contradicted.
- **Per-field beats per-kind for policy that touches UX**: the autocomplete fix initially used per-kind dispatch (Text vs others). Codex pushed for per-field. The right answer was a `FacetFieldSpec::autocomplete_match` field that defaults from kind but allows future per-field overrides without churn.
- **Reviewer iterations are non-optional**: codex flagged a real blocker on round 2 (term cap applied to all modes, not just autocomplete). One round of review would have shipped that bug. The project rule "iterate until reviewers cannot find anything else" is load-bearing.
- **Match the codebase's own conventions over generic style**: substring autocomplete matches what `libnetdata/facets/facets.c:1783` already does for systemd-journal FTS (`SIMPLE_PATTERN_SUBSTRING`). Consistent with the project, not novel.
- **Separate benchmarks: per-protocol resource envelope vs storage footprint**: a single "resource benchmark" couldn't answer both "what's the ingest cost?" and "what's the on-disk cost?". Splitting them produced two complementary tables and removed a bogus `bytes/flow x time` calculation that ignored journal indexing and dedup.
- **Architectural pivots happen mid-SOW**: the original plan placed flow integrations under `data-collection.networking` as collector-typed cards. Mid-execution the user reframed the test as "would users ask 'Does Netdata integrate with X?'", which shifted the answer to a top-level `flows` integration_type with 14 cards. Captured as Decision 10 rather than retconning earlier decisions.

## Followup

Open follow-ups, ordered by priority:

1. **Cloud-frontend `Update integrations.js`** -- copy the regenerated `integrations.js` from netdata into `cloud-frontend/src/domains/integrations/data/integrations.js` and open the standard sync PR. Not gated by this SOW. Last manual refresh was 2024-10-21; this work won't appear in the dashboard's Integrations modal until that file is updated.
2. **Phase 2 SOW: integration tests for runtime I/O paths**:
   - BMP listener: async/tokio tests for TCP accept loop, framed decode, `apply_update` trie wiring, malformed message error accumulation, retry/shutdown. Test with a real BMP speaker, measure ingest impact, validate enrichment correctness.
   - BioRIS: async tests for gRPC client connection, RIB dump stream, retry/backoff. Build a local RIS daemon for end-to-end validation. Test `build_endpoint_uri`, `parse_router_ip`.
   - Network Sources: async tests for HTTP fetch cycle, service loop, prefix matching integration, failed HTTP handling, header forwarding, multi-source merge/re-publish.
   - All three: integration tests that wire parsed data through `DynamicRoutingRuntime` trie into flow enrichment lookup.
3. **BMP architectural decision**: enrichment-only in netflow-plugin (Akvorado pattern) vs a separate BGP monitoring plugin with its own DB. This is product-level, not a code change.
4. **Topology drilldown**: the `useFlowsDrilldownData` hook is dead code today. Implement the actor-modal hook when a UX home is decided.
5. **Autocomplete follow-ups (deferred from autocomplete bug fix)**:
   - Case-insensitive matching for text facets (today: typing `akamai` will not match `AS20940 Akamai International`). UX call.
   - Substring scan over very large archived FST sidecars is bounded by `FACET_AUTOCOMPLETE_LIMIT` early-exit but still streams keys for rare/no-hit terms. Token-prefix or n-gram indexing can be added if measurements warrant.
6. **Health alerts (`health.d/`)**: deferred. The netflow-plugin emits its own self-monitoring metrics (parse errors, decoder latency, ingest queue depth); alerts on those have not yet been authored.
7. **AI skills cross-links to learn docs**: REJECTED for this SOW (not mandatory per user 2026-05-07). Can be picked up in a future skills-maintenance pass; not tracked as a separate SOW because rejected, not deferred.

## Regression Log

## Regression - 2026-05-07

### What broke

The user previewed the learn netlify deploy at
`https://deploy-preview-2852--netdata-docusaurus.netlify.app/`
and found the documentation contains multiple statements that
contradict the source code, several invented behaviours that
were never verified, and structural choices that read as
academic / generic flow-monitoring advice rather than as a
practical guide to Netdata's flow plugin specifically. The
prior closure on 2026-05-07 claimed every behavioural claim had
been verified against the code; that claim is false.

### Why previous validation missed it

1. Subagent investigations produced data extracts (fields,
   tier preservation, IE maps) accurately but did NOT catch
   behavioural framing claims. Behavioural claims live in
   absences (no code says "do this") and were imported from
   the research notes at `.agents/knowledge/Network Traffic
   Analysis with Flow Data.md` as Netdata-specific without
   verifying against actual code paths.
2. Three rounds of codex review focused on the autocomplete
   code change. None reviewed the documentation prose.
3. Validation evidence was structural (grep for forbidden
   topics, count of pages, pipeline exit codes) rather than
   semantic (every behavioural claim cited to code). The
   "code is the source of truth" rule from Decision 9 was
   stated but not enforced per claim.
4. SOW closure was driven by "all phases done" rather than
   "all claims true". The completion was premature.

### Findings (verbatim from user, plus code-citation verdict)

These findings must be addressed one at a time, no batching.
For each: investigate against code, surgical edit, grep the
rest of the docs for the same pattern, fix every instance.
Each finding gets its own SOW execution-log entry below with
file:line and the code citation that proves the fix.

#### F1 -- /docs/network-flows landing page shows tiles not Overview

> this page should be the overview, it appears on click of
> the "Network Flows" main menu. Instead it shows all
> integrations as tiles:
> https://deploy-preview-2852--netdata-docusaurus.netlify.app/docs/network-flows

Need to investigate how learn renders the section root.
Likely cause: section root has no leaf content, so the auto-
grid renderer in `learn/ingest/ingest.py:get_dir_make_file_and_recurse`
generates a category index. Repair: ensure the Overview file
is the section landing page, not the category-index grid.

#### F2 -- Overview false bidirectional symmetry claim

> > When you see "traffic from your country to a foreign
> > country" and "traffic from that foreign country to your
> > country" of similar volume, you're looking at one
> > conversation, not two.
>
> The statement "of similar volume" is wrong. The two
> directions of traffic are usually not expected to have
> similar volume. It can be omitted and the phrase must be
> normalized because it may or may not be bidirectional.

Repair: rephrase the doubling explanation to drop "similar
volume" framing. The doubling effect is about per-router
ingress+egress accounting, NOT about traffic symmetry. Same
underlying conversation can have very different byte counts
in each direction.

#### F3 -- Overview false "in one direction" advice

> > To see real numbers: filter by one exporter, one
> > interface, in one direction. The dashboard makes this
> > easy. See the Anti-patterns page for the full framing.
>
> "in one direction" is wrong. Traffic does not double when
> viewing bidirectional traffic in one interface, because
> traffic is not usually symmetrical. It needs rephrasing.

Repair: rephrase. The correct framing for "see real numbers"
is per-exporter + per-interface scope; direction filtering
is not the doubling fix.

#### F4 -- Sampling rate framing wrong (uniform-rate myth)

> > This works correctly only if all your exporters use the
> > same sampling rate.
>
> No! This is totally wrong. The way netdata does it, is it
> that it gets the specific sampling each flow has and
> multiplies the traffic of the specific flow to find its
> actual. This works even if each router and each interface
> has its own sampling rate.
>
> The "works only if all your routers have the same
> sampling rate" is a misconception from when we were
> discussing this:
>
> - you: the dashboard must show the sampling rate on any
>   view
> - me: this cannot be done reliably when flows with mixed
>   sampling rates are aggregated on the dashboard, and
>   netdata does the right thing to no show it, because:
>   - a) if all your routers have the same sampling, you
>     know it already
>   - b) if you have mixed sampling rates, it is technically
>     impossible to provide a meaningful single sampling
>     rate for the aggregation
>   So, netdata multiplies at the source, so that
>   aggregations are as accurate as possible, even with
>   mixed sampling rates

Code evidence:
`src/crates/netflow-plugin/src/decoder/record/core/record.rs:24-26`
multiplies `bytes` and `packets` by each record's own
`sampling_rate` at decode time. Mixed sampling rates across
exporters or interfaces are handled correctly automatically.

Repair: remove the uniform-rate-required framing wherever it
appears, replace with the correct per-flow-multiplication
explanation, drop any UI claim about showing a single
sampling rate (it would be meaningless under mixed rates).

#### F5 -- Sampling rate "clean path" recommendation wrong

> > The clean path: keep sampling rates uniform across your
> > network, or run unsampled where the flow rate allows
>
> No. I never said that. It is not the clean path. People
> should use the sampling rates according to their use
> cases. But netdata multiplies at ingestions and does not
> show sampling rates on the UI.

Repair: remove the "clean path" recommendation. Netdata does
the right thing regardless of whether sampling is uniform or
mixed.

#### F6 -- Globe view "less useful for analysis" wrong

> > Globe -- a 3D rendering of the city-level data. Visual
> > demo, less useful for analysis.
>
> "less useful for analysis"? Why? The information is
> exactly the same with the map. There is a table, like in
> maps. What makes it less useful? That is 3d? The opposite
> I think.

Repair: rewrite the globe section to drop the
"less useful for analysis" judgement. Same data, same table,
same selectivity; the 3D projection is one of several
valid presentations.

#### F7 -- Installation tab location wrong

> In installation:
>
> > The Network Flows tab should appear in the top
> > navigation
>
> No. The flows functions in in the "Live" top menu
> currently.

Repair: correct the location to "Live" menu.

#### F8 -- Configuration: tier sizing should be per-tier only

> Retention size should be set per tier, like:
>
> tiers:
>  raw: { size: 10GB, duration: 24h }
>  etc
>
> These globals must be removed:
>
>   size_of_journal_files: 10GB
>   duration_of_journal_files: 7d
>
> It is very important to be able to size tiers
> independently of each other. There is no one size fits
> all.
> I know there are globals and overrides per tier, but come
> on. Why double configuration?

This finding has TWO parts:

1. Documentation: stop documenting the globals; show only
   per-tier `tiers: { raw: {size, duration}, ... }`.
2. Code: remove `size_of_journal_files` and
   `duration_of_journal_files` from the schema.

Code reference today:
`src/crates/netflow-plugin/src/plugin_config/types/journal.rs:21-37`
declares both globals AND per-tier overrides. The user wants
the globals dropped from the configuration schema entirely.

#### F9 -- Configuration: query_1m_max_window / query_5m_max_window unjustified

> About these:
>
>   query_1m_max_window: 6h
>   query_5m_max_window: 24h
>
> What are these and why they are needed? I don't
> understand. Either the query engine is half based, or
> these are useless overprotections that are never needed.

Repair: investigate purpose in code. If they are real
protections, document the protection clearly. If they are
useless, remove them from both code and docs.

#### F10 -- Configuration: query_max_groups / query_facet_max_values_per_field unjustified

> I don't understand what are these and why are needed and
> what value or protection they provide:
>
>   query_max_groups: 50000
>   query_facet_max_values_per_field: 5000
>
> Explain

Repair: same as F9 -- investigate, document the protection
or remove.

#### F11 -- Empty page: enrichment-concepts/ip-intelligence

> empty page:
> https://deploy-preview-2852--netdata-docusaurus.netlify.app/docs/network-flows/enrichment-concepts/ip-intelligence

Investigate why the page renders empty. Source file
`docs/network-flows/enrichment/ip-intelligence.md` exists.
Likely a generated MDX issue (frontmatter, fence, or special
char) or an ingest path mismatch.

#### F12 -- Retention and Querying structure wrong; URL sharing irrelevant

> Retention and Querying has a section called "URL
> sharing"? Really? You find this relevant?
> If you need to put generic visualization rules, these
> should be a generic "Visualization/Overview" page, to
> explain FTS, sharing, grouping, etc. For sure Retention
> is closer to configuration and querying is closer to
> visualization.

Repair has THREE parts:

1. Remove the "URL sharing" section from
   retention-querying.md.
2. Move retention-side content (tier sizing rules, retention
   knobs) closer to Configuration.
3. Move query-time semantics (tier auto-pick rules,
   query-engine behaviour, FTS, sharing, grouping) into a
   new Visualization/Overview page.

#### F13 -- Sizing/Capacity Planning wrong genre, wrong content

> Sizing and Capacity planning is written like an academic
> paper that must prove productivity of the testing
> environment. People want sizing and planning directions.
> This is not an academic paper, not a blog.
>
> What are the requirements for this page:
>
> - what is the cap of the plugin
> - how ingestion rate affects storage
> - the raw tier monopolizes storage - do not let it
>   explode - it will need fast nvme disks to query it.
> - journal backend uses free system memory as system
>   caches - the bigger the database, the more free memory
>   the system would need.
> - journal is fully indexed, all fields are indexed, but
>   FTS means full scan.
> - explain that 25k flows/s sustained approaches ISP level
>   capacities.
> - use the distributed nature of netdata. The plugin can
>   be installed multiple times, in branch offices,
>   different data centers, etc. And since aggregation
>   across routers is usually meaningless for flows, users
>   can appoint 1 netdata per router. There is no need to
>   push all flows to one central place.
>
> So, this page should provide a practical guide for users
> to scale the plugin, the servers and storage it runs,
> etc.
> Remove the benchmarks and tests from this page. The
> benchmarks were for us, not for the customers.

Repair: rewrite sizing-capacity.md from scratch as a
practical scaling guide following the seven bullets above.
Drop all benchmark numbers, drop the "academic paper" framing.

#### F14 -- Validation: invented user-side risks

> In Validation and Data Quality:
>
> Risks:
> - Netdata monitors UDP port overflows and has alerts for
>   it.
> - "Sampling rate misinterpretation" how is this a risk
>   for users? This is bug in netdata if it happens.
> - "Sampling rate change" how is this a risk for users?
>   Netdata ensures this will not happen because ingestion
>   scales on sampling received
> - "Template loss after collector restart" how is this a
>   risk for users? Netdata saves templates and reloads
>   them
>
> I think the entire "Validation and Data Quality" is
> completely off. It mentions again sampling rates, etc.
> It is like it was written by someone that does not have
> a clue of what netdata is and how the plugin works.

Code evidence:
- Templates persist:
  `src/crates/netflow-plugin/src/decoder/protocol/v9/templates.rs:106`
  and
  `src/crates/netflow-plugin/src/decoder/protocol/ipfix/templates/data.rs:67`
- Sampling multiplied per-flow at decode (see F4).

Repair: rewrite validation.md from scratch. Remove the
sampling-related "risks", remove template-loss "risk", point
UDP overflow concern at Netdata's existing alerts.

#### F15 -- Anti-patterns: "Ignoring the sampling rate" is bogus

> Anti-patterns page:
>
> > Ignoring the sampling rate
>
> How is it possible for users to ignore the sampling rate
> if we calculate the estimated volume at ingestion? You
> invented reasons for it: "so the dashboard numbers are
> estimates of actual traffic -- if the multiplication is
> consistent"
>
> What? How the multiplication cannot be consistent? What
> are you talking about?
>
> "Ignoring the sampling rate" section must be removed.

Repair: remove the entire section.

#### F16 -- Anti-patterns: "GeoIP for internal IPs" is invented

> > "Internal IPs (10.x, 172.16-31.x, 192.168.x) appear in
> > random countries on the geographic map"
>
> What? Where did you find this? Geolocation does not
> position internal IPs on the map.
>
> "Trusting GeoIP for internal IPs" section must be removed.

Repair: remove the entire section. Need to verify in code
that internal IPs are NOT placed on the map -- if there is
any path that does, that's a Netdata bug to file separately,
not a user-side anti-pattern.

#### F17 -- Anti-patterns: "Alerting on absolute volume" doesn't apply

> > "Alerting on absolute volume thresholds"
>
> Netdata does not support alerting of flows yet. Remove
> this section.

Repair: remove the entire section.

#### F18 -- Troubleshooting: wrong journalctl namespace

> Troubleshooting page:
>
> Netdata logs in namespace 'netdata'. Journalctl needs
> `--namespace netdata`.

Repair: every `journalctl` invocation must include
`--namespace netdata`. Grep all docs.

#### F19 -- Troubleshooting: cumulative misconceptions

> This page has a mix of all the above issues: sampling,
> geoip, etc.

Repair: this is a per-claim sweep informed by the fixes
above. Every behavioural claim on the troubleshooting page
must be cited to code or removed.

#### F20 -- Section title: "Enrichment Concepts" wrong

> "Encrichement Concepts" is a wrong title. "Flows
> Enrichement" is the right one.

Repair: rename the sub-section "Enrichment Concepts" to
"Flows Enrichment" everywhere it appears: `docs/.map/map.yaml`,
in any cross-references in pages, and in any sidebar /
breadcrumb labels that derive from the map.

#### F21 -- Section title: "Sources" wrong

> "Sources" is too generic. "Flow Protocols" is the right
> one.

Repair: rename the sub-section "Sources" to "Flow Protocols"
everywhere: `docs/.map/map.yaml`, plus every cross-reference
that points at `Sources/NetFlow`, `Sources/IPFIX`,
`Sources/sFlow` (those individual page labels stay).

### Repair plan

**Phase R1 -- Per-finding fixes, one at a time, no batching.**
For each finding F1..F21 above:

1. Investigate against code -- read the relevant source
   files; record file:line evidence in this SOW under the
   per-finding execution-log entry.
2. Surgical edit -- minimal diff that fixes only that
   claim.
3. Grep the rest of the docs for the same pattern; fix
   every instance with the same surgical care.
4. Append a dated execution-log entry naming what changed,
   why, and the code citation.

No finding is "deferred". Every one gets fixed before the
SOW can re-close.

**Phase R2 -- Per-page audit subagents.**

After Phase R1 completes, spawn one read-only audit
subagent per page. Each subagent's brief:

- Read the entire page line by line.
- For every behavioural / configuration / vendor /
  protocol claim, verify it against the source code at the
  cited paths in this repository.
- For every third-party vendor configuration mention
  (Cisco IOS-XE / IOS-XR, Juniper JunOS, FRR, Palo Alto,
  Mikrotik, Zyxel, etc.), verify against the upstream
  vendor's current documentation by web fetch.
- Flag every claim that cannot be anchored, every
  generic-flow-monitoring sentence that contradicts how
  Netdata actually works, and every vendor command that
  does not exist or has wrong syntax.
- Output a structured finding list: claim, location,
  evidence, severity, suggested fix.

The master assistant synthesises the findings, applies
surgical fixes, and re-spawns the auditor on the same page.
Iterate until the auditor returns "no findings".

**Pages in scope for Phase R2:**
- README.md (Overview)
- installation.md
- quick-start.md
- configuration.md
- field-reference.md
- retention-querying.md (post-restructure)
- sizing-capacity.md (post-rewrite)
- validation.md (post-rewrite)
- investigation-playbooks.md
- anti-patterns.md (post-section-removals)
- troubleshooting.md
- 7 enrichment pages: ip-intelligence, asn-resolution, bgp-routing, network-identity, static-metadata, classifiers, decapsulation
- 5 visualisation pages: summary-sankey, time-series, maps-globe, filters-facets, dashboard-cards
- 14 generated integration cards under `src/crates/netflow-plugin/integrations/` -- each has its own audit pass against `metadata.yaml` and against the upstream vendor documentation for any setup steps.

**Plus a new visualisation/overview page** (per F12) and possibly a new visualisation/querying page if the F12 split lands as proposed.

**Phase R3 -- Final close.**

The SOW reopened with this regression note during repair. The Validation
section was appended (not replaced) with per-finding evidence and per-page
audit-clean evidence. Status returned to `completed` after all closure
criteria listed below were satisfied:

- Every F1..F21 has a fix landed and a code citation in the
  log.
- Every page has at least one auditor pass returning no
  findings.
- A whole-section codex-style review of the final docs
  returns no new findings.

Then move file back to `done/`.

### Repair execution log

#### F1 -- 2026-05-07 -- section landing page rendering

Investigation: Compared `# Network Flows` block in
`docs/.map/map.yaml` with sections that DO render an Overview
landing page on Learn (e.g. `Collecting Metrics`,
`Dashboards and Charts`, `Netdata Cloud`). All those carry
`edit_url:` directly on the section root meta block. Network
Flows root carried `label:` only; the README.md was instead
exposed as a CHILD entry labelled "Overview". With no leaf
content at the root, learn's
`get_dir_make_file_and_recurse` (`learn/ingest/ingest.py:2333-2510`)
auto-generates a category-grid index page at the section
URL, which is what the netlify preview rendered as tiles.

Fix: hoisted `edit_url:` and `description:` to the Network
Flows root meta block. Removed the now-redundant child
"Overview" entry pointing to the same README.

Other similar patterns in MY work: Sub-sections "Enrichment
Concepts" (line 498) and "Visualization" (line 529) within
Network Flows also lack root `edit_url`. Decision: leave as
auto-grid for now -- F1 was specifically about the section
root URL `/docs/network-flows`, and sub-section grids are an
accepted pattern across the rest of the project (e.g.
"Systemd Journal Logs" sub-section under Logs). If a future
finding flags `/docs/network-flows/flows-enrichment` or
`/docs/network-flows/flow-protocols` as needing real content,
revisit then.

Patterns OUTSIDE my work (pre-existing): "Logs" section
root (line 567 in map.yaml) also lacks root `edit_url`. Out
of scope -- pre-existing structural choice.

Evidence:
- Pattern reference: `docs/.map/map.yaml:65-68` (Welcome to Netdata),
  `docs/.map/map.yaml:96-99` (Collecting Metrics).
- Renderer: `learn/ingest/ingest.py:2333-2510`
  (`get_dir_make_file_and_recurse`).

Diff: `docs/.map/map.yaml:479-484`.

#### F2 + F3 -- 2026-05-07 -- doubling vs bidirectional symmetry

These two findings address the same paragraphs and are fixed
together. The "doubling" effect (per-packet ingress+egress on
one router) was conflated with bidirectional traffic
symmetry, which is a different concept. Original docs told
users to (a) look for "similar volume" in opposite directions
to identify a single conversation, and (b) "filter by one
exporter, one interface, in one direction" to see real
volume. Both wrong:

- Bidirectional traffic is typically asymmetric (downloads
  vs ACKs). "Similar volume" is a wrong heuristic for
  spotting two halves of the same conversation.
- "in one direction" added on top of "one interface" is
  redundant for the doubling fix and misleads readers into
  expecting another 50% halving from direction filtering.

The doubling fix is just: one exporter + one interface
(Input Interface OR Output Interface, pick one). Each packet
crossing that interface produces exactly one record on it.

Mirror-conversation framing rewritten: bidirectional
conversations produce separate records per direction
because they really are different packets going each way.
Volumes are usually asymmetric. Per-direction accounting
is correct, not duplication.

Files touched:
- `docs/network-flows/README.md` -- `## Two things to know
  on day one` paragraphs rewritten.
- `docs/network-flows/quick-start.md` -- step-3 paragraphs
  rewritten.
- `docs/network-flows/visualization/summary-sankey.md:98`
  -- "in one direction" removed from doubling fix.
- `docs/network-flows/anti-patterns.md:21` -- same fix in
  "How to avoid it" line of the doubling anti-pattern.
- `docs/network-flows/validation.md:36, 55` -- same fix in
  the SNMP cross-check and doubling sanity-check sections.

Note: F14 / F15 / F16 / F17 will rewrite anti-patterns.md
and validation.md more comprehensively. The corrections
above are kept as standalone fixes so the wrong claim is
gone immediately even if the surrounding sections survive.

Other patterns scanned: searched for "of similar volume",
"same volume", "symmetric", "in one direction",
"conversations? (?:are|look|appear)? mirrored". The lines
above are all hits within `docs/network-flows/`. No other
files carry the pattern.

#### F4 + F5 -- 2026-05-07 -- sampling rate framing (uniform-rate myth)

These two findings are about the same incorrect claim and
fixed together.

Code evidence -- per-flow multiplication at decode time:

```rust
// src/crates/netflow-plugin/src/decoder/record/core/record.rs:24-26
let sampling_rate = rec.sampling_rate.max(1);
rec.bytes = rec.bytes.saturating_mul(sampling_rate);
rec.packets = rec.packets.saturating_mul(sampling_rate);
```

`sampling_rate` is set per-record from the relevant source:
- `decoder/protocol/legacy.rs:60` -- v5 header rate.
- `decoder/protocol/v9/records.rs:39, 215` and
  `decoder/protocol/v9/sampling.rs:215` -- v9 IE on record
  or Sampling Options Template (namespace-scoped).
- `decoder/protocol/ipfix/special/record.rs:21, 61, 171`,
  `ipfix/record/state.rs:10, 38, 168`,
  `ipfix/record/append.rs:56` -- IPFIX record IE / sampling
  options.
- `decoder/protocol/sflow/record.rs:6, 19` -- sFlow per-sample
  rate.
- `decoder/protocol/shared/merge/enrich.rs:80` -- merge with
  static `override_sampling_rate` config.

So mixed sampling rates across exporters / interfaces / time
are handled correctly: each record scales by its own rate.
The only failure mode is "exporter is sampling but the rate
is not communicated" (NetFlow v7 has no field; v5 with
rate=0; v9 / IPFIX without an attached Sampling Options
Template). That is a real concern and stays in the docs.

Removed claims:

- README.md lines 91-94 -- "works correctly only if all
  your exporters use the same sampling rate" and "the
  clean path: keep sampling rates uniform across your
  network" -- both false. Rewrote the paragraph to state
  per-flow multiplication, why the UI does not surface a
  single rate, and the real statistical-floor caveat
  (sampling can miss small/short flows regardless of how
  uniform the rates are).
- field-reference.md line 33 -- `RAW_BYTES` description
  said "Use when sampling is uniform across all your
  exporters". Changed to "Use when you want the unscaled
  value the exporter sent". Same fix in
  anti-patterns.md:126 (the prose) and the summary table
  row at line 150.
- troubleshooting.md line 129 -- "Mixed sampling rates
  across exporters... isn't comparable to any single SNMP
  measurement" replaced with the correct framing:
  comparing aggregates to a single interface counter is
  the actual mistake; per-flow multiplication is correct
  regardless of rate uniformity.
- validation.md line 11 -- "undocumented sampling rate
  changes" removed from the silent-failure list intro.
- validation.md line 116 -- the "Sampling rate change"
  monitoring-table row removed.
- investigation-playbooks.md lines 128, 132 -- "Sampling
  rate of the exporter (so the numbers can be
  interpreted)" deliverable bullet removed; "A change in
  sampling rate during the analysis window invalidates
  the trend" caveat removed.
- anti-patterns.md line 132 -- "Same goes for
  sampling-rate differences across exporters" removed
  from the cross-protocol comparison section. The
  protocol-counts-not-comparable point stays; the
  uniformity claim goes.

Items NOT touched in this finding (deferred to F14 / F15
which will rewrite their containing sections):

- validation.md silent-failure list items #2 ("Sampling
  rate misinterpretation"), #3 ("Sampling rate change"),
  #5 ("Template loss after collector restart") -- F14
  removes them as a block.
- anti-patterns.md section 2 ("Ignoring the sampling
  rate") and the summary-table row "Ignored sampling" --
  F15 removes the entire section.

Files touched:
- docs/network-flows/README.md (lines 88-96)
- docs/network-flows/field-reference.md (line 33)
- docs/network-flows/troubleshooting.md (lines ~120-130)
- docs/network-flows/validation.md (lines 11, ~116)
- docs/network-flows/investigation-playbooks.md (lines 128-132)
- docs/network-flows/anti-patterns.md (lines 132, 140, 150, 126)

Other docs scanned (`docs/network-flows/`) for the patterns
"uniform.*rate", "same.*sampling.*rate", "rates.*uniform",
"all.*exporters.*same.*rate", "sampling.*uniform",
"clean.*path.*sampling", "aggregate.*blend",
"blend.*estimate" -- all hits addressed above except the
F14 / F15 -targeted blocks.

#### F6 -- 2026-05-07 -- Globe view "less useful for analysis"

User: "the information is exactly the same with the map.
There is a table, like in maps. What makes it less useful?
That is 3d? The opposite I think."

Code reality (per the city-map / globe code path that
shares the same backend response: see
`src/crates/netflow-plugin/src/query/request/constants.rs`
for `CITY_MAP_GROUP_BY_FIELDS` and the absence of any
"globe" view enum -- the globe re-renders the city-map
response): same data, same table, just a different
rendering.

Removed claims:

- README.md line 105 -- "Visual demo, less useful for
  analysis" replaced with neutral framing that the globe
  uses the same data + table as the city map and is the
  better fit when distance / great-circle routes matter.
- visualization/maps-globe.md "Globe vs City Map"
  paragraph -- removed the "less useful for analysis"
  judgement; states the trade-off (2D for in-continent
  precision; 3D for transcontinental / great-circle).

Also fixed the "Mirroring" subsection on the same page:
the "25 top-N = 12 conversations" claim is the F2 symmetry
myth and was inconsistent with bidirectional traffic
typically being asymmetric. Reworded to state that
bidirectional traffic produces two separate flow records,
two distinct edges, with usually-different volumes.

No other doc carries the "less useful for analysis"
phrasing. Grep clean.

Files touched:
- docs/network-flows/README.md (line 105)
- docs/network-flows/visualization/maps-globe.md (lines ~85-91)

#### F7 -- 2026-05-07 -- Network Flows is a Function under the Live tab

Verified terminology against
`docs/dashboards-and-charts/live-tab.md:3-69`: Netdata's
official UI vocabulary calls it the **Live tab** in the top
navigation; it lists Functions; "Network Flows" is one of
those Functions. There is no top-nav "Network Flows" tab.

Repair scope: every doc reference. Adopted convention:

- Verb form: "Open Network Flows" (drop "tab")
- Noun reference to the workspace: "the Network Flows
  view"
- Setup / installation context: "Click the **Live** tab in
  the top navigation; **Network Flows** appears in the
  Functions list" -- mirrors the official Live-tab doc.

Files touched:
- docs/network-flows/installation.md (lines 113-121) --
  the original bad sentence and follow-up.
- docs/network-flows/troubleshooting.md (lines 19, 47).
- docs/network-flows/investigation-playbooks.md (lines 11,
  21, 63, 109, 141) -- five "Open the Network Flows tab"
  occurrences replaced with "Open Network Flows"; lead
  paragraph at line 11 names the Live tab.
- docs/network-flows/anti-patterns.md (line 15).
- docs/network-flows/visualization/dashboard-cards.md
  (lines 11, 13, 87, 97 -- four occurrences).
- docs/network-flows/visualization/summary-sankey.md
  (line 11).

Grep for "Network Flows tab", "Network Flows menu", "top
navigation" run after the sweep -- only the corrected
phrasings remain.

#### F8 -- 2026-05-07 -- per-tier retention only; remove globals

User: "It is very important to be able to size tiers
independently of each other. There is no one size fits all.
I know there are globals and overrides per tier, but come
on. Why double configuration?"

Code investigation: the global `size_of_journal_files` and
`duration_of_journal_files` on `JournalConfig` were already
no more than per-tier defaults that flowed through
`retention_for_tier()` -- runtime semantics were already
per-tier. The "double configuration" was schema redundancy
with no underlying behavioural justification.

Refactor (`src/crates/netflow-plugin/src/`):

- `plugin_config/types/journal.rs`: removed
  `size_of_journal_files` and `duration_of_journal_files` from
  `JournalConfig`. Replaced
  `Option<JournalTierRetentionConfig>` per-tier with the
  struct directly carrying `Option<ByteSize>` /
  `Option<Duration>` fields. Each tier defaults to
  `Some(default)` when the YAML omits it; explicit `null`
  disables the limit on that tier; both Nones is rejected by
  validation. Removed the now-vestigial
  `JournalConfig::default_retention()` and simplified
  `retention_for_tier()` to a single per-tier lookup.
  Built-in tier defaults stay at uniform 10GB / 7d to
  preserve current default behaviour.
- `plugin_config/defaults.rs`: removed the dead
  `RetentionLimitOverride<T>` enum, its
  `is_inherit` / `resolve` impl, and the four
  `(de)serialize_retention_override_*` helpers (no longer
  reachable). Removed the now-orphan `parse_bytesize`
  helper that fed the removed clap `value_parser`.
- `plugin_config.rs`: removed the
  `pub(crate) use defaults::RetentionLimitOverride;`
  re-export.

Tests (mechanical updates to the new schema):

- `memory_tests.rs`: collapsed four near-identical per-tier
  override blocks into one `small_tier` config cloned across
  the four tiers; replaced
  `RetentionLimitOverride::Value(...)` with `Some(...)`;
  dropped the now-dead `RetentionLimitOverride` import.
- `startup_memory_tests.rs`: dropped the global-retention
  setters that already matched the built-in defaults.
- `plugin_config_tests.rs`: rewrote five tests to exercise
  the new schema only:
  `journal_tier_retention_uses_built_in_tier_defaults`
  (was `inherits_global_defaults_when_no_overrides_exist`),
  `journal_tier_retention_uses_per_tier_values_when_present`,
  `journal_rotation_size_derives_from_tier_size_budget` (now
  via `tiers.raw.size_of_journal_files`),
  `journal_rotation_size_uses_100mb_for_time_only_retention`,
  `journal_validation_allows_time_only_retention_when_size_is_disabled`,
  `journal_tier_retention_null_disables_size_limit_for_that_tier_only`
  (replaces the old "inherited size limit" YAML test).
  Updated the YAML test fixture at line ~245 to drop the
  global keys.

Documentation:

- `docs/network-flows/configuration.md`: rewrote the
  `## journal` section. Single per-tier table only, no
  "top-level retention" subsection, explicit note that
  there are no global retention knobs. Updated the
  production-retention example to a fully-per-tier form.
- `docs/network-flows/retention-querying.md`: dropped the
  global-form example; replaced with the per-tier form;
  cross-link to configuration.md.
- `docs/network-flows/sizing-capacity.md` (line 101):
  already says "per-tier"; no change.

Build + tests:

- `cargo build --release` clean (3m02s).
- `cargo test --release --bin netflow-plugin` -- 427
  passed; 0 failed; 18 ignored.

Breaking change notice: any existing user config that
specified `journal.size_of_journal_files` or
`journal.duration_of_journal_files` at the top level of the
journal block will now fail to deserialize (strict
`deny_unknown_fields`). Users migrate by moving those values
into per-tier `tiers.<tier>.size_of_journal_files` /
`duration_of_journal_files`. Plugin is recently shipped
(PR #22439, 2026-05-07); breaking-change risk is low.

Files touched:
- src/crates/netflow-plugin/src/plugin_config/types/journal.rs
- src/crates/netflow-plugin/src/plugin_config/defaults.rs
- src/crates/netflow-plugin/src/plugin_config.rs
- src/crates/netflow-plugin/src/memory_tests.rs
- src/crates/netflow-plugin/src/startup_memory_tests.rs
- src/crates/netflow-plugin/src/plugin_config_tests.rs
- docs/network-flows/configuration.md
- docs/network-flows/retention-querying.md

#### F9 -- 2026-05-07 -- query_1m_max_window / query_5m_max_window are dead

User: "What are these and why they are needed? I don't
understand. Either the query engine is half based, or these
are useless overprotections that are never needed."

Code investigation: searched
`src/crates/netflow-plugin/src/` for any consumer outside
the config struct itself:

- `plugin_config/types/journal.rs` -- declared.
- `plugin_config/validation/journal.rs:6-13` -- non-zero
  validation; ordering check.
- `plugin_config/defaults.rs` -- defaults.
- `plugin_config_tests.rs` -- two YAML fixtures only.

NO consumer in `query/`, `query/planner/`, or anywhere else
reads these values. The actual tier auto-pick logic lives in
`query/planner/spans.rs:plan_query_tier_spans_recursive`,
which selects the coarser tier strictly from window /
bucket-duration alignment math; it does not consult
`query_1m_max_window` or `query_5m_max_window`.

Verdict: dead config knobs. The user's "useless
overprotections" assessment was correct.

Repair: removed both fields from the schema and validation,
plus the two YAML test fixtures, plus the configuration
table row that documented them. Updated the
retention-querying explanation of "skip a tier when window
is too wide" to reflect the actual behaviour: the planner
picks the coarsest aligned tier without separate config-
driven window caps.

Files touched:
- src/crates/netflow-plugin/src/plugin_config/types/journal.rs
- src/crates/netflow-plugin/src/plugin_config/validation/journal.rs
- src/crates/netflow-plugin/src/plugin_config_tests.rs
  (two YAML fixtures cleaned)
- docs/network-flows/configuration.md (removed two table
  rows + corresponding code-block lines + the two-line
  "query-window limits" explanation)
- docs/network-flows/retention-querying.md (corrected
  description of tier-pick behaviour)

Build + tests:
- `cargo build --release` clean (2m59s).
- `cargo test --release --bin netflow-plugin` -- 427
  passed; 0 failed; 18 ignored.

Breaking change notice: any user config that set
`journal.query_1m_max_window` or `journal.query_5m_max_window`
will now fail to deserialize (deny_unknown_fields). These
keys had no effect before, so the migration is to delete
them; no behavioural change.

#### F10 -- 2026-05-07 -- query_max_groups stays (real); query_facet_max_values_per_field is dead

User: "I don't understand what are these and why are needed
and what value or protection they provide. Explain"

Code investigation, two opposite outcomes:

`query_max_groups` -- REAL.
- Read at `query/service.rs:52` and threaded into the
  `ProjectedGroupAccumulator` via the `max_groups` parameter
  used at `query/projected/apply.rs:48`.
- When `grouped_aggregates.grouped_total() < max_groups`
  is no longer true, the accumulator stops registering new
  group keys; the row's missing values are then folded into
  the synthetic `__overflow__` bucket created at
  `query/grouping/labels.rs:17` /
  `query/grouping/model/compact.rs:35`.
- A warning is emitted on the response:
  `query/timeseries.rs:124` -- "Group accumulator limit
  reached; additional groups were folded into __overflow__."
- Purpose: bounds memory growth on accidentally wide
  group-by combinations (high-cardinality fields like IP
  addresses, AS-paths, MAC addresses, etc.).
- Verdict: keep, document properly.

`query_facet_max_values_per_field` -- DEAD.
- Declared, validated for non-zero in
  `plugin_config/validation/journal.rs:18`, but the actual
  facet accumulator at `query/facets/render.rs:19, 27`
  consumes the hardcoded constant
  `DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD = 5_000`
  from `query/request/constants.rs:17` -- not the config
  knob. The two coincidentally have the same default value
  but the config knob is never threaded to the consumer.
- Verdict: dead schema. Remove.

Repair:

- `plugin_config/types/journal.rs` -- removed
  `query_facet_max_values_per_field` field; added a doc
  comment explaining what `query_max_groups` actually does.
- `plugin_config/defaults.rs` -- removed
  `default_query_facet_max_values_per_field()` helper.
- `plugin_config/validation/journal.rs` -- removed the
  non-zero check for the dead knob.
- `plugin_config_tests.rs` -- removed
  `validate_rejects_zero_query_facet_max_values_per_field`
  test entirely; cleaned the YAML fixtures that mentioned
  the dead knob.
- `src/crates/netflow-plugin/configs/netflow.yaml` (stock
  config) -- rewrote the journal block to use the per-tier
  retention form and dropped the dead knob; added a clear
  comment for `query_max_groups`.
- `src/crates/netflow-plugin/README.md` -- updated the
  example YAML and the explanatory paragraph; removed the
  dead-knob mention.
- `docs/network-flows/configuration.md` (Query guardrails
  section) -- table now lists only `query_max_groups`;
  expanded its description to name the `__overflow__`
  bucket and the warning behaviour.
- `docs/network-flows/retention-querying.md` (Group-by
  limit section) -- consolidated to one bullet; named the
  warning + overflow behaviour.
- `docs/network-flows/visualization/filters-facets.md` --
  removed the entire "Facet limits" subsection that
  documented the dead knob.

Build + tests:
- `cargo build --release` clean (2m57s).
- `cargo test --release --bin netflow-plugin` -- 426
  passed; 0 failed (one test removed -- the dead-knob
  validation test).

Breaking change notice: any user config that set
`journal.query_facet_max_values_per_field` will now fail
to deserialize (deny_unknown_fields). The key had no
effect before; migration is delete-only.

#### F11 -- 2026-05-07 -- empty IP Intelligence page

Investigation: `docs/network-flows/enrichment/ip-intelligence.md`
was a 0-byte file in master. `git log --all --format=%H` of
that path shows it has been 0 bytes since the original
documentation rewrite commit (a073bcf24f). It was meant to
be the "Enrichment Concepts / IP Intelligence" page but
got created empty.

Repair: authored the page from scratch, code-grounded
against:

- `src/crates/netflow-plugin/src/plugin_config/types/enrichment/geoip.rs`
  (GeoIpConfig: asn_database / geo_database / optional).
- `src/crates/netflow-plugin/src/plugin_config/runtime.rs:23-64`
  (auto-detect path: cache_dir/topology-ip-intel/* and
  stock_data_dir/topology-ip-intel/*; auto-detected files
  marked optional=true).
- `src/crates/netflow-plugin/src/enrichment/data/geoip/resolver.rs`
  (load, refresh-if-needed every 30s on signature change,
  per-IP lookup composing multiple ASN/geo databases,
  IPv6-vs-IPv4-database skip).
- `src/crates/netflow-plugin/src/enrichment.rs:35`
  (GEOIP_RELOAD_CHECK_INTERVAL = 30s).
- `src/crates/netflow-plugin/src/enrichment/data/network/asn.rs`
  (AS-name rendering format).

Page covers: fields populated (with tier-preservation
notes), configuration schema, auto-detection, refresh
cadence, lookup order, the four provider integration cards,
private-IP rendering, IPv6-only/IPv4-only database
behaviour, staleness drift, geographic-accuracy caveats,
failure modes table.

Frontmatter `learn_rel_path` set to
"Network Flows/Enrichment Concepts" to match the bgp-routing
and network-identity siblings (the source frontmatter is
informational; the actual sidebar position derives from
`docs/.map/map.yaml`). F20 will rename the section
consistently across map.yaml and all sibling frontmatter.

Files touched:
- docs/network-flows/enrichment/ip-intelligence.md
  (created from empty)

#### F20 -- 2026-05-07 -- "Enrichment Concepts" -> "Flows Enrichment"

Repair: renamed the section consistently in:

- `docs/.map/map.yaml:499` -- the section label that drives
  the actual sidebar position on Learn.
- All seven frontmatter `learn_rel_path:` values in
  `docs/network-flows/enrichment/*.md` -- prior state was
  inconsistent (4 files had "Network Flows/Enrichment", 2
  had "Network Flows/Enrichment Concepts", 1 had the
  brand-new F11 page). Settled on the canonical
  "Network Flows/Flows Enrichment" everywhere.

Grep `Enrichment Concepts|enrichment-concepts` across docs/
and src/crates/netflow-plugin/ -- no remaining references.

Files touched:
- docs/.map/map.yaml
- docs/network-flows/enrichment/asn-resolution.md
- docs/network-flows/enrichment/bgp-routing.md
- docs/network-flows/enrichment/classifiers.md
- docs/network-flows/enrichment/decapsulation.md
- docs/network-flows/enrichment/ip-intelligence.md
- docs/network-flows/enrichment/network-identity.md
- docs/network-flows/enrichment/static-metadata.md

#### F21 -- 2026-05-07 -- "Sources" -> "Flow Protocols"

Repair: renamed the Flow Protocols sub-category in the
integrations catalog and propagated to integration cards
and any in-prose references.

- `integrations/categories.yaml:71` -- the `flows.sources`
  category's user-visible `name:` changed from "Sources" to
  "Flow Protocols". Description left intact (it already
  reads "Flow protocols Netdata receives directly from
  routers, switches, and software exporters", which agrees
  with the new label).
- Three integration card frontmatter updates: `sflow.md`,
  `ipfix.md`, `netflow.md` now declare
  `learn_rel_path: "Network Flows/Flow Protocols"`.
- `src/crates/netflow-plugin/metadata.yaml` -- removed three
  broken self-referencing links pointing at
  `https://learn.netdata.cloud/docs/network-flows/sources/{netflow,ipfix,sflow}`.
  These URLs were broken before the rename (no
  `docs/network-flows/sources/` directory exists in source)
  and would stay broken under the new label too. Replaced
  with the surviving "Network Flows Overview" anchor that
  is real.
- Re-ran `integrations/gen_integrations.py` and
  `integrations/gen_docs_integrations.py` -- both exit
  clean. The three regenerated `.md` cards no longer carry
  the broken self-link.

Grep `/sources/` and `/Sources/` after the sweep -- no
remaining references inside flow integrations content.

Files touched:
- integrations/categories.yaml
- src/crates/netflow-plugin/metadata.yaml
- src/crates/netflow-plugin/integrations/sflow.md
- src/crates/netflow-plugin/integrations/ipfix.md
- src/crates/netflow-plugin/integrations/netflow.md

#### F18 -- 2026-05-07 -- journalctl --namespace netdata everywhere

User: "Netdata logs in namespace 'netdata'. Journalctl needs
`--namespace netdata`."

Background: `-u netdata` selects the systemd UNIT, which
captures only stdout/stderr the unit emits to the journal.
The plugin (and the rest of Netdata) actually writes
structured logs into a journal NAMESPACE called `netdata`.
Without `--namespace netdata`, users see at most the
unit-level startup/shutdown messages, not the actual log
output that helps with debugging.

Repair: swept all `journalctl -u netdata` invocations to
`journalctl --namespace netdata` in:
- docs/network-flows/quick-start.md
- docs/network-flows/troubleshooting.md (5 occurrences)
- docs/network-flows/installation.md
- docs/network-flows/enrichment/network-identity.md
- docs/network-flows/configuration.md (already fixed under
  F8; no change here)

Grep `journalctl` after the sweep -- every invocation now
uses `--namespace netdata`.

#### F15 -- 2026-05-07 -- remove "Ignoring the sampling rate" anti-pattern

User: "How is it possible for users to ignore the sampling
rate if we calculate the estimated volume at ingestion? You
invented reasons for it. ... section must be removed."

Verified the per-flow multiplication code path under F4 +
F5 already; the entire premise of this anti-pattern (that
mixed rates produce inconsistent multiplication, that users
must keep rates uniform, that aggregates become "hard to
interpret") was wrong. The two real concerns it conflated
are documented elsewhere already:

- "small flows missed at high sampling rates" -- preserved
  under the Overview's "What sampling does to your numbers"
  section and inside investigation-playbooks.md "Caveats"
  for the security playbook.
- "exporter sends no rate" (NetFlow v7, v5 with rate=0,
  v9/IPFIX without Sampling Options Template) -- preserved
  in troubleshooting.md "Bandwidth doesn't match SNMP" and
  in validation.md.

Repair: deleted the entire "## 2. Ignoring the sampling
rate" section from `docs/network-flows/anti-patterns.md`.
Section numbering renumber will land with F17 (the last of
the three section removals) so that anti-patterns.md
renumbers exactly once.

Files touched:
- docs/network-flows/anti-patterns.md (removed lines
  ~23-36 plus the section header)

#### F16 -- 2026-05-07 -- remove "Trusting GeoIP for internal IPs" anti-pattern

User: "Geolocation does not position internal IPs on the
map. ... section must be removed."

Verified against code: `apply_geo_record`
(`src/crates/netflow-plugin/src/enrichment/data/geoip/decode.rs:40-72`)
writes `country`, `state`, `city`, `latitude`, `longitude`
ONLY when the MMDB record carries those fields with
non-empty values. For private / RFC 1918 IPs, the MMDB
either has no entry at all OR has an entry tagged with
`ip_class: "private"` and no country/city/coords -- so
none of those fields get written. Internal IPs do NOT
appear on geographic maps.

The "internal IPs in random countries" claim was invented;
no such behaviour exists. Repair: deleted the entire
"## Trusting GeoIP for internal IPs" section.

Section numbering renumber will land with F17 once all
three section removals have completed.

Note: the troubleshooting.md "Internal IPs in random
countries" subsection (lines 134-138) carries the same
invented claim and will be addressed under F19.

Files touched:
- docs/network-flows/anti-patterns.md (removed the
  section header and body)

#### F17 -- 2026-05-07 -- remove "Alerting on absolute volume thresholds"

User: "Netdata does not support alerting of flows yet.
Remove this section."

Repair: removed the entire "Alerting on absolute volume
thresholds" section from `docs/network-flows/anti-patterns.md`.
The section's own footnote already acknowledged this:
"Netdata's alerting on flow data is in development; for
now this pattern lives in your monitoring practice, not
in the plugin." So the section was advice for users to
apply outside Netdata -- not a Netdata anti-pattern.

Renumbering: with F15, F16, and F17 all deleting sections,
the anti-patterns.md sections get renumbered in this same
commit. Final numbering: 1 (doubled aggregate) ... 9
(comparing flow counts across protocols). Removed three
old rows from the summary table (Ignored sampling, GeoIP
for internal IPs, Absolute thresholds). The cross-link
from time-series.md at line 96 ("Why time-shifted
comparison beats absolute thresholds") was rewritten to
point at the still-relevant general anti-patterns set.

Files touched:
- docs/network-flows/anti-patterns.md (removed section,
  renumbered remaining sections, dropped 3 summary-table
  rows)
- docs/network-flows/visualization/time-series.md
  (cross-link rewording)

#### F19 -- 2026-05-07 -- troubleshooting.md cumulative cleanup

User: "This page has a mix of all the above issues:
sampling, geoip, etc."

Repair scope: surgical fixes to the cumulative
misconceptions on troubleshooting.md after F2-F18 land.

Removed:
- "Internal IPs in random countries" subsection (lines
  ~134-138). Same invented claim as F16; same code-verified
  reason for removal.

Rewrote:
- "Things that look like bugs but aren't" entries:
  - "Traffic appears 2x" -- now mentions exporter + interface,
    not direction (F2/F3 fix in this section too).
  - "Bidirectional conversations show twice" -- reframed as
    real distinct flows with usually-asymmetric volumes;
    pointed at Source ASN / Destination ASN filtering, not
    "direction" filtering.
  - Removed the "Internal IPs in odd countries" bullet entirely.
  - "City map empty over long windows" -- "tier-0" replaced
    with "raw-tier" for consistency with the field-reference
    and tier-naming used elsewhere.

Items checked and kept:
- "Sampling rate not honoured by the exporter" framing is
  correct (F4/F5 already updated this; the real concern is
  the exporter not communicating the rate, not "uniform
  rates required").
- Doubling references in the SNMP-mismatch table (F2/F3
  already updated).
- ASN provider chain debug recipe -- code-anchored at
  `enrichment/data/network/asn.rs`.
- Decapsulation destructive-on-non-tunnel framing -- code-
  anchored at `decoder/protocol/...`.

Items DEFERRED to the per-page audit (R2) because they
need vendor-doc verification:
- "Cisco's default template refresh is 30 minutes" --
  vendor-specific claim; verify against current Cisco
  IOS-XE / IOS-XR documentation in R2.

Files touched:
- docs/network-flows/troubleshooting.md

#### F14 -- 2026-05-07 -- validation.md rewrite

User: "I think the entire 'Validation and Data Quality'
is completely off. It mentions again sampling rates, etc.
It is like it was written by someone that does not have
a clue of what netdata is and how the plugin works."

Code-verified facts driving the rewrite:

- **Per-flow sampling-rate multiplication** at decode time:
  `decoder/record/core/record.rs:24-26`. The user does NOT
  need to monitor "sampling rate change" or "sampling rate
  misinterpretation" -- these are not user-side risks.
- **Template persistence** across plugin restarts:
  `decoder/protocol/v9/templates.rs:106` and
  `decoder/protocol/ipfix/templates/data.rs:67`. The user
  does NOT need to monitor "template loss after collector
  restart".
- **UDP buffer overflow alert** already exists at
  `src/health/health.d/udp_errors.conf:6-19`
  (`1m_ipv4_udp_receive_buffer_errors`, fires when
  RcvbufErrors > 10/min). Reframe UDP drops as an existing
  alert to consume, not a "silent failure" the user must
  hunt down.

Page rewritten from scratch:

- New opening: states up-front that the plugin handles
  per-flow scaling, template persistence, and database
  refresh internally; what's left to validate is
  exporter-side and configuration drift.
- New "What you actually need to watch" table with five
  items (kernel UDP drops -> existing alert; exporter
  stopped sending; wrong interfaces being exported;
  exporter sampling but not communicating rate; stale
  MMDB).
- Removed the original silent-failure list items
  "Sampling rate misinterpretation", "Sampling rate
  change", "Template loss after collector restart" --
  three items confirmed not user-side risks.
- Removed the "Internal IP enrichment validation" section
  (F16 confirmed GeoIP does not position internal IPs).
- Renamed "Sampling rate sanity check" to "Sampling rate
  verification" with the bogus uniform-rate framing
  removed; kept the practical RAW_BYTES vs BYTES
  comparison recipe (the only useful piece of the old
  section).
- Removed the "Template cache health" subsection
  entirely. (The `template_errors` chart is still in the
  plugin-side alerting table, but as an exporter-config
  signal, not a "user must watch in case templates get
  lost" risk.)
- Renamed the alerting table from "what to monitor and
  what alerts to consider" to "Plugin-side signals worth
  alerting on"; clarified that these are signals the
  plugin already exposes for the operator to alert on,
  not "silent failures" the dashboard hides.

Files touched:
- docs/network-flows/validation.md (full rewrite, kept
  frontmatter and the surviving sections in place)

#### F13 -- 2026-05-07 -- sizing-capacity.md rewrite as a practical guide

User: "Sizing and Capacity planning is written like an
academic paper that must prove productivity of the testing
environment. People want sizing and planning directions.
This is not an academic paper, not a blog."

User-stated requirements (the seven bullets):
1. what is the cap of the plugin
2. how ingestion rate affects storage
3. raw tier monopolizes storage; needs fast NVMe
4. journal uses free system memory as page cache; bigger
   database -> more free RAM
5. journal is fully indexed; FTS means full scan
6. 25k flows/s sustained approaches ISP-level capacity
7. distributed deployment -- one Netdata per router; no
   central aggregation needed

Plus: remove benchmarks and tests from this page.

Page rewritten from scratch. New shape:

- Opening paragraph: states the design intent (one network
  per agent). Anchors the 25k flows/s = ISP-level scale
  immediately.
- "Plugin throughput cap" -- single-thread post-decode
  hot path; saturation around 30k high-cardinality, above
  60k low-cardinality; recommend 25k sustained as the
  comfortable steady-state. Burst handling.
- "Distributed deployment is the scaling answer" --
  the user's central thesis. One agent per router / per site,
  federated via Netdata Cloud. Why this beats pushing more
  through a single collector. Recommended shape for
  multi-site deployments.
- "Storage" -> "How ingestion rate maps to disk" -- one
  table, four rows, derived from the storage-footprint
  benchmark (~800 bytes/flow on disk). Includes a
  cardinality caveat. No benchmark methodology.
- "Storage" -> "Raw tier dominates" -- explicit; rollups
  are tiny; example per-tier config sized for production
  use.
- "Storage" -> "Use fast NVMe for the raw tier" -- direct,
  no hedging. Mentions that slow storage forces shorter
  raw-tier retention.
- "Memory" -- routing trie footprint, page-cache framing,
  and the existing memory-monitoring chart references.
- "Querying -- what's fast and what isn't" -- indexed
  fields are O(log) on selectivity; FTS is full scan of
  raw tier and forces raw-tier; 30s query timeout
  implications.
- "Practical checklist before you deploy" -- seven
  concrete steps mirroring the user's seven bullets.

Removed:
- All "What was measured" / "Detailed measurements" / the
  per-protocol per-cardinality benchmark tables (Phase 1.0
  output). Those numbers stay in the netflow-plugin README
  for engineering reference; they are not customer
  guidance and were the wrong genre for this page.
- "Bounding storage for capacity planning" formula derivation
  (which was already partly invalid because it ignored
  tier rollover and dedup).

Files touched:
- docs/network-flows/sizing-capacity.md (full rewrite)

#### F12 -- 2026-05-07 -- Retention/Querying restructure; new Visualization Overview

User: "Retention is closer to configuration and querying
is closer to visualization. ... If you need to put generic
visualization rules, these should be a generic
'Visualization/Overview' page, to explain FTS, sharing,
grouping, etc."

Repair: split the old retention-querying.md into a
retention-only page and a new visualization-overview page.

New file: `docs/network-flows/visualization/overview.md`
- "How queries work" -- query modes, parameters, defaults,
  30s timeout (moved from retention-querying.md).
- "Group-by limit and overflow" (moved).
- "Full-text search" (moved + expanded to explain when to
  use it vs the indexed filter ribbon).
- "URL sharing" (moved; reframed as part of generic
  visualization, not its own standalone section).
- "Filtering" (cross-link to filters-facets.md).
- "Picking the right view" (cross-link to each panel).

Updated `docs/network-flows/retention-querying.md`:
- Sidebar label: "Retention and Querying" -> "Retention
  and Tiers" (matches the new content scope).
- Removed sections: "How queries work, briefly",
  "Group-by limit and overflow", "Full-text search",
  "URL sharing" (all moved to the new visualization/overview).
- Page intro now points users to Configuration for retention
  config and to Visualization Overview for query semantics.
- Renamed remaining mentions of "tier 0" / "tier-0" to
  "raw tier" / "raw-tier" for consistency.

Updated `docs/.map/map.yaml`:
- Visualization sub-section root now carries `edit_url:`
  pointing at visualization/overview.md (so clicking
  "Visualization" in the sidebar opens the Overview, the
  same pattern as F1's section-root fix).
- "Retention and Querying" sidebar label renamed to
  "Retention and Tiers".
- "Sizing and Capacity Planning" description updated post
  F13 (no more benchmarks).

#### F22 -- 2026-05-07 -- remove redundant "Section index" from the Overview

User: "The 'Section index' in the overview page is not
needed. Learn already shows the index as a side bar."

Repair: removed the entire `## Section index` section from
`docs/network-flows/README.md`. The Learn sidebar already
shows the same hierarchy. The "Where to start" section
above it stays (it's role-based guidance, not a duplicate
of the sidebar). Updated the "specific feature in depth"
bullet to point at the sidebar instead of the deleted
section.

Files touched:
- docs/network-flows/visualization/overview.md (created)
- docs/network-flows/retention-querying.md (slimmed,
  sidebar label renamed)
- docs/.map/map.yaml
- docs/network-flows/README.md (removed Section index)

#### Current audit -- 2026-05-07 -- metadata.yaml is first-class docs source

The follow-up review treats `src/crates/netflow-plugin/metadata.yaml`
as a public documentation source, not merely generator plumbing. The
generated files under `src/crates/netflow-plugin/integrations/*.md`
carry a `DO NOT EDIT` banner and are downstream of that metadata file,
so every metadata issue below also reaches Learn / integration-card
content after regeneration.

Current branch reviewed: `netflow-docs-repair`.

Status of previously suspected issues at the start of this pass:

- **Found during current audit -- timestamp source is computed but not used for live
  journal timestamps.** `docs/network-flows/configuration.md:104-112`
  says `timestamp_source` controls dashboard timestamps. The decoder
  carries `DecodedFlow.source_realtime_usec` at
  `src/crates/netflow-plugin/src/decoder.rs:149-151`, but live ingest
  still calls `ingest_decoded_record(receive_time_usec, &flow.record)`
  at `src/crates/netflow-plugin/src/ingest/service/runtime.rs:82-84`
  and writes both source and entry realtime from `receive_time_usec` at
  `runtime.rs:129-131`. This is a code/docs mismatch. Recommended
  fix: code should thread `flow.source_realtime_usec.unwrap_or(receive_time_usec)`
  into raw writes and tier observation; docs can then stay conceptually
  correct.
- **Found during current audit -- removed top-level retention keys remain in
  `metadata.yaml` and generated integration docs.** The current code
  schema has only `JournalConfig { journal_dir, tiers, query_max_groups }`
  at `src/crates/netflow-plugin/src/plugin_config/types/journal.rs:11-31`;
  retention is per-tier under `JournalTierRetentionOverrides` at
  `journal.rs:83-112`. But `metadata.yaml` still documents
  `journal.size_of_journal_files` / `journal.duration_of_journal_files`
  at lines 96-102, 246-252, and 383-389, and the NetFlow example still
  uses top-level `journal.size_of_journal_files` / `duration_of_journal_files`
  at lines 132-135. Generated `netflow.md`, `ipfix.md`, and `sflow.md`
  repeat the same invalid options. Because the Rust structs use
  `#[serde(deny_unknown_fields)]`, this documented YAML now fails
  config parsing. Fix source in `metadata.yaml`, then regenerate.
- **Found during current audit -- IPFIX and sFlow metadata still call 2055 the
  "standard port".** `metadata.yaml:262` says IPFIX listens on the
  standard port while the example uses `0.0.0.0:2055`; `metadata.yaml:399`
  says the same for sFlow. Code only proves 2055 is Netdata's default
  listener at `src/crates/netflow-plugin/src/plugin_config/types/listener.rs:6`.
  External protocol evidence: IANA registers IPFIX on 4739 and sFlow on
  6343. Fix wording to "Netdata default listener port" or state that
  users may choose any UDP listener and must configure exporters to match.
- **Found during current audit -- invented internal-IP geolocation claim remains in
  metadata and maps docs.** `metadata.yaml:598-604`,
  `src/crates/netflow-plugin/integrations/db-ip_ip_intelligence.md:184`,
  and `docs/network-flows/visualization/maps-globe.md:77-79` still
  discuss "internal IPs appearing in random countries." Code only writes
  country/city/state/coordinate fields when the MMDB record has a
  non-empty value (`src/crates/netflow-plugin/src/enrichment/data/geoip/decode.rs:40-72`).
  This is the same false premise as F16 and F19, but not fully removed.
  Fix source in `metadata.yaml` and the maps page, then regenerate.
- **Found during current audit -- `OBSERVATION_TIME_MILLIS` contradiction remains.**
  `docs/network-flows/field-reference.md:138` says the field is IPFIX
  observation time, while the master table at line 297 says IPFIX has no
  canonical mapping in this build. Code maps only NetFlow v9
  `ObservationTimeMilliseconds` at
  `src/crates/netflow-plugin/src/decoder/record/mappings.rs:37`; IPFIX
  falls through to `_ => None` at `mappings.rs:116`. Fix docs unless
  product decides to add IPFIX support in code.
- **Found during current audit -- visualization overview documents unsupported `last`
  query parameter.** `docs/network-flows/visualization/overview.md:22`
  lists `after` / `before`, or `last`. The accepted function parameters
  are enumerated in `src/crates/netflow-plugin/src/api/flows/params.rs:5-18`
  and do not include `last`. Fix docs unless code intentionally adds
  a shorthand.
- **Found during current audit -- "tier 0" terminology remains.** Examples:
  `docs/network-flows/visualization/time-series.md:48-55`,
  `docs/network-flows/visualization/maps-globe.md:51,103`,
  `docs/network-flows/field-reference.md:155`, and
  `docs/network-flows/investigation-playbooks.md:70,97,145`.
  Code/config use `raw` (`JournalTierRetentionOverrides.raw` at
  `journal.rs:86-87`; stock config `journal.tiers.raw`). Fix docs to
  "raw tier" consistently.
- **Found during current audit -- pcap references remain while
  the acceptance criterion still says no pcap anywhere.**
  `docs/network-flows/troubleshooting.md:72,217,227` still recommend
  `tcpdump -w`. This may be useful support guidance, but it contradicts
  the explicit acceptance criterion at line 68. Either remove the public
  pcap workflow, or update the SOW acceptance criterion to allow packet
  captures strictly as troubleshooting artifacts.

New findings from the current branch:

- **F23 -- public docs link to source-tree integration markdown paths
  that Learn will not serve.** Many Network Flows pages link to
  `/src/crates/netflow-plugin/integrations/*.md`, e.g.
  `docs/network-flows/quick-start.md:105,180,187-189`,
  `docs/network-flows/configuration.md:221-224`,
  `docs/network-flows/enrichment.md:419-440`,
  `docs/network-flows/installation.md:80,130-132`, and
  `docs/network-flows/visualization/maps-globe.md:75,79,112-113`.
  Those are repository source paths, not Learn URLs. The integration
  cards are generated into the Network Flows integration placeholder and
  should be linked through their Learn routes, not `/src/...`. Fix all
  source-path links.
- **F24 -- `metadata.yaml` still uses stale UI wording "Network Flows
  tab".** F7 fixed many markdown pages, but metadata still says
  "Network Flows tab" at lines 153, 627, 865, 1100, 1679, 2273, 2564,
  3517, 3560, 3959, 4379, and 4653. Generated integration cards repeat
  the same stale term. Fix metadata to "Network Flows view" or
  "Live tab > Network Flows" depending on context, then regenerate.
- **F25 -- old "Sources" label remains in public markdown links.**
  The catalog label is now "Flow Protocols" in
  `integrations/categories.yaml:70-72`, but docs still say
  "Sources / NetFlow" or "Sources" at
  `docs/network-flows/quick-start.md:105`,
  `docs/network-flows/installation.md:130-132`, and
  `docs/network-flows/anti-patterns.md:121`. Fix wording and target URLs.
- **F26 -- public docs and generated integration cards contain internal
  code citations.** There are hundreds of `src/crates/...` / `.rs:line`
  citations in generated integration cards and several in
  `docs/network-flows/enrichment.md` / `intel-downloader.md`. These are
  useful audit evidence, but they read as implementation notes in
  end-user/operator documentation. The SOW can keep code citations; public
  docs should translate them into operator-facing behavior and only link
  to code where the user explicitly needs source. Needs a per-page/content
  decision during repair, but the current state is not fit for polished
  Learn docs.

Repair ordering recommendation:

1. Fix `metadata.yaml` first for F23/F24 plus the still-open retention,
   port, and internal-IP findings; regenerate integration pages.
2. Fix the remaining markdown-only regressions (`timestamp_source`,
   `OBSERVATION_TIME_MILLIS`, `last`, raw-tier terminology, pcap decision).
3. Re-run link checks and grep sweeps against both `docs/network-flows/`
   and generated `src/crates/netflow-plugin/integrations/*.md`.
4. Run the integration generator and the narrow netflow-plugin tests
   touched by any code fix.

#### Current audit repair progress -- 2026-05-07

First repair pass completed:

- `src/crates/netflow-plugin/metadata.yaml` now documents per-tier
  `journal.tiers.<tier>.size_of_journal_files` and
  `journal.tiers.<tier>.duration_of_journal_files` instead of removed
  top-level retention keys.
- The NetFlow extended-retention example now uses `journal.tiers.raw`,
  `minute_1`, `minute_5`, and `hour_1`.
- IPFIX and sFlow setup examples now say "Netdata's default flow
  listener port" instead of "standard port".
- The DB-IP troubleshooting entry now says private IPs have empty GeoIP
  fields and do not appear on maps, instead of "internal IPs appearing
  in random countries".
- `Network Flows tab` was mechanically replaced with `Network Flows
  view` in metadata.
- Regenerated integration pages with `python3 integrations/gen_integrations.py`
  and `python3 integrations/gen_docs_integrations.py`.
- Verified the following patterns are gone from `metadata.yaml` and
  generated integration cards:
  `journal.size_of_journal_files`,
  `journal.duration_of_journal_files`,
  `standard port`,
  `Internal IPs appearing`,
  `random countries`,
  `Network Flows tab`.

Markdown repair pass completed:

- Repointed public docs away from `/src/crates/netflow-plugin/integrations/*.md`
  source paths to Learn routes under `/docs/network-flows/flow-protocols/...`
  and `/docs/network-flows/enrichment-methods/...`.
- Replaced stale "Sources / ..." wording with "Flow Protocols / ...".
- Removed the unsupported `last` query-parameter claim from
  `docs/network-flows/visualization/overview.md`; it now says omitted
  `after` / `before` defaults to the last 15 minutes, matching
  `src/crates/netflow-plugin/src/query/planner/request.rs:3-15`.
- Removed the remaining internal-IP/random-country claim from
  `docs/network-flows/visualization/maps-globe.md`.
- Replaced remaining `tier 0` / `tier-0` language in Network Flows markdown
  with `raw tier` / `raw-tier`.
- Fixed `docs/network-flows/field-reference.md` so
  `OBSERVATION_TIME_MILLIS` is documented as NetFlow v9-only in this
  build, and so timestamp fields are not described as dashboard
  time-picker fields. Code evidence:
  `src/crates/netflow-plugin/src/decoder/record/mappings.rs:37,116`,
  `src/crates/netflow-plugin/src/query/request/constants.rs:75-79`,
  `src/crates/netflow-plugin/src/query/fields/metrics.rs:19-20`.
- Verified the field-reference master index still has the same 91
  canonical fields as `src/crates/netflow-plugin/src/flow/schema.rs`.

Follow-up repair after timestamp clarification:

- `timestamp_source` is no longer open. The code now passes the selected
  decoded source timestamp (`DecodedFlow.source_realtime_usec`) into
  `_SOURCE_REALTIME_TIMESTAMP` while preserving journal entry realtime as
  receive/write time. This matches the append-only journal contract and
  handles out-of-order source timestamps across exporters. Evidence:
  `src/crates/netflow-plugin/src/ingest/service/runtime.rs:82-139` and
  `src/crates/netflow-plugin/src/main_tests.rs:76-94`.
- `docs/network-flows/configuration.md` now states that `timestamp_source`
  controls stored source timestamp metadata, not dashboard query windows.
  Query windows and tier selection still use journal entry realtime.
- The pcap acceptance conflict is no longer open. Public docs now say
  "packet-capture file" and `tcpdump -w`; they no longer mention pcap by
  name. This keeps troubleshooting guidance without implying a pcap ingest
  feature.
- Validation: `cargo test --manifest-path src/crates/netflow-plugin/Cargo.toml timestamp_source -- --nocapture`
  passed with 5 tests. A pre-existing warning remains in
  `src/crates/netflow-plugin/src/startup_memory_tests.rs` for an unused
  `bytesize::ByteSize` import.
- Validation: `git diff --check` passed for the SOW, Network Flows docs,
  metadata, generated integration docs, and touched netflow-plugin Rust
  files.
- Validation: grep now finds no stale public-doc occurrences of
  `journal.size_of_journal_files`, `journal.duration_of_journal_files`,
  `standard port`, `Network Flows tab`, `Sources /`, `[Sources]`,
  `tier 0`, `tier-0`, unsupported ``or `last`` wording, IPFIX observation
  time wording, random-country/internal-IP wording, or pcap wording. The
  only timestamp-source grep hit is the intentional docs warning that the
  dashboard time picker does not query by exporter timestamps.

User decision:

- Public Network Flows documentation and generated integration cards are
  end-user/operator documentation. They must not contain internal source-code
  citations, internal test-status citations, quality-gate evidence, SOW/review
  notes, or "current implementation state" wording. They should document
  supported behavior, operator-visible limits, configuration contracts,
  troubleshooting signals, and production cautions without exposing internal
  proof or test gaps. Internal evidence belongs in the SOW, not in Learn docs.

F26 repair completed:

- Removed internal source-code paths, line references, source-test notes,
  "current implementation state" wording, and test-gap disclosures from
  `docs/network-flows/*.md` and `src/crates/netflow-plugin/metadata.yaml`.
- Rewrote those passages into operator-facing behavior: supported enrichment
  semantics, visible limits, configuration validation, refresh cadence, TLS
  safety, troubleshooting symptoms, and production cautions.
- Regenerated the integration cards with
  `python3 integrations/gen_integrations.py` and
  `python3 integrations/gen_docs_integrations.py`.
- Generated integration files still contain `custom_edit_url` and `meta_yaml`
  entries inside their `<!--startmeta ... endmeta-->` blocks. These are
  generator metadata, not user-facing page body text.

F26 validation:

- `rg -n 'src/|github\.com/netdata/netdata/(blob|tree)/master/src|\.rs:[0-9]|\.go:[0-9]|unit test|unit-tested|integration-test|integration-tested|integration test|NOT integration|Source code:|defined at|schema is defined|validated at|validation rejects values|production logs|current build|in this build|fixtures|tests here|repository|quality gate|current state|not tested|not validated|test gap|line reference|current implementation|implementation state|codebase|SOW|regression|review pass|internal citation' docs/network-flows src/crates/netflow-plugin/metadata.yaml`
  returned no matches.
- The same pattern scan against `src/crates/netflow-plugin/integrations/*.md`
  while skipping generated `<!--startmeta ... endmeta-->` blocks returned no
  matches.
- `python3 -c 'import yaml; yaml.safe_load(open("src/crates/netflow-plugin/metadata.yaml")); print("metadata yaml ok")'`
  passed.
- `git diff --check` passed for the SOW, Network Flows docs, metadata,
  generated integration docs, and touched netflow-plugin Rust files.

F26 residual findings from full-page subagent review:

- The prior regex validation was too narrow. It caught explicit source paths,
  source line references, test-gap wording, and quality-gate labels, but missed
  less literal end-user trust issues such as "today", "known feature gap",
  "verified against", "same code path", implementation type names, and
  benchmark-provenance phrasing.
- Read-only subagents reviewed the assigned public pages in full, not only by
  grep. Scope covered all hand-authored Network Flows markdown pages,
  `src/crates/netflow-plugin/metadata.yaml`, and all generated integration
  cards.

Residual docs findings:

- **Visualization current-state / backlog wording.**
  `docs/network-flows/visualization/dashboard-cards.md:72,79,81` says signals
  "aren't published today", are "collected internally", and are "not hard to
  add but haven't been needed enough yet". This is product backlog / internal
  state wording.
  `docs/network-flows/visualization/filters-facets.md:32,68` says negative
  matching is a "known feature gap" and that no good workaround exists "today".
  These should be rewritten as stable supported/unsupported behavior.
- **Sizing page benchmark-provenance wording.**
  `docs/network-flows/sizing-capacity.md:23,50,96` exposes benchmark machine
  details, synthetic benchmark provenance, and "bench numbers" in public docs.
  The page should present operator sizing guidance directly; evidence stays in
  the SOW.
- **Enrichment / validation developer wording.**
  `docs/network-flows/enrichment.md:107,156` says multiple inputs use the
  "same code path"; this is implementation wording.
  `docs/network-flows/enrichment.md:230,276` uses "today" limitation wording.
  `docs/network-flows/enrichment.md:393` exposes the source-tree-style
  `cmd/ris/` path in prose.
  `docs/network-flows/validation.md:22,68` says the plugin does not publish
  per-exporter ingest counters "today"; this should be stable limitation
  wording.

Residual metadata / generated-card findings:

- **Quality-gate / verification wording in generated cards.**
  `src/crates/netflow-plugin/metadata.yaml:2015` and generated
  `src/crates/netflow-plugin/integrations/aws_ip_ranges.md:41` say the AWS
  schema was "verified against the live document".
  `src/crates/netflow-plugin/metadata.yaml:2666` and generated
  `src/crates/netflow-plugin/integrations/azure_ip_ranges.md:147` say Azure
  behavior was "verified against the upstream concept page".
  These are internal review-evidence statements.
- **Current implementation-state wording in generated cards.**
  `src/crates/netflow-plugin/metadata.yaml:2305` and generated
  `src/crates/netflow-plugin/integrations/gcp_ip_ranges.md:60` say ETags are
  not used for conditional fetches "today".
  `src/crates/netflow-plugin/metadata.yaml:4339,4503` and generated
  `src/crates/netflow-plugin/integrations/decapsulation.md:54,265` say parsed
  tunnel fields are "not surfaced today".
- **Internal implementation names in generated cards.**
  `src/crates/netflow-plugin/metadata.yaml:2939` and generated
  `src/crates/netflow-plugin/integrations/netbox.md:131` expose
  `RemoteNetworkSourceConfig`.
  `src/crates/netflow-plugin/metadata.yaml:3432` and generated
  `src/crates/netflow-plugin/integrations/generic_json-over-http_ipam.md:321`
  expose `decode_remote_records`.
- **Untrusted-feature wording in decapsulation docs.**
  `src/crates/netflow-plugin/metadata.yaml:4395,4408,4414` and generated
  `src/crates/netflow-plugin/integrations/decapsulation.md:124,137,143` say
  "the project has verified", "have not been verified by the project", and
  "unverified". Operator docs should state recommended/supported vendor
  configuration patterns without exposing internal validation status.

Repair classification:

- All residual findings are documentation-source fixes. No code behavior change
  is indicated by this review.
- Hand-authored markdown findings should be fixed in the corresponding
  `docs/network-flows/*.md` file.
- Generated-card findings must be fixed in
  `src/crates/netflow-plugin/metadata.yaml`, then regenerated with
  `python3 integrations/gen_integrations.py` and
  `python3 integrations/gen_docs_integrations.py`. Do not hand-edit generated
  integration cards.
- At classification time, F26 remained open until these residual findings were
  repaired and a broader wording scan was added to validation.

F26 residual repair completed:

- Rewrote hand-authored docs so they state stable supported/unsupported
  behavior without backlog, "today", implementation, or benchmark-provenance
  wording.
- Rewrote `src/crates/netflow-plugin/metadata.yaml` source text to remove
  quality-gate evidence, implementation type/function names, "today" current
  state wording, and "verified/unverified by the project" statements.
- Regenerated generated integration cards with
  `python3 integrations/gen_integrations.py` and
  `python3 integrations/gen_docs_integrations.py`.
- All residual findings above are now repaired. Generated integration files
  were updated only through `metadata.yaml`.

F26 residual validation:

- Residual-pattern scan returned no matches over hand-authored docs and
  metadata:
  `RemoteNetworkSourceConfig|verified against the live document|conditional fetches today|verified against the upstream concept page|decode_remote_records|not surfaced today|fields today|project has verified|not been verified|unverified|not visible today|cannot be consumed today|same code path|cmd/ris/|doesn.t publish per-exporter.*today|aren.t published today|collected internally|not hard to add|known feature gap|no good workaround exists today|bench numbers|synthetic high-cardinality|i9-class|FireCuda`.
- The same residual-pattern scan over generated
  `src/crates/netflow-plugin/integrations/*.md` body text, skipping
  `<!--startmeta ... endmeta-->`, returned no matches.
- The original stricter internal-citation scan over hand-authored docs,
  metadata, and generated integration body text also returned no matches.
- `python3 -c 'import yaml; yaml.safe_load(open("src/crates/netflow-plugin/metadata.yaml")); print("metadata yaml ok")'`
  passed.
- `git diff --check` passed for the SOW, Network Flows docs, metadata, and
  generated integration docs.

Quality review decision:

- User decision 2026-05-07: do not change deployment guidance to avoid
  double counting at ingestion. The correct deployment guidance is to export
  all relevant interfaces and all directions. Double counting is a
  visualization and interpretation guideline: users must understand what they
  selected and how bidirectional/interface-overlap views should be read.

Quality review repair completed:

- Aligned IP-intelligence install/default behavior across public docs:
  native packages ship stock DB-IP MMDBs under stock data, source builds do
  not include stock MMDBs, the downloader writes fresher cache copies, and
  Netdata does not install a downloader timer or cron job.
- Reworded retention defaults as suitable for first validation and small
  deployments, with production retention sized from observed flow rate.
- Replaced generated default-behavior boilerplate across all Network Flows
  integration cards so cards no longer claim generic "no limits" or
  "no significant performance impact".
- Fixed UDP-drop quick-reference commands to use `ss -uamn` and
  `/proc/net/snmp`, not `/proc/net/udp` as a drop counter source.
- Replaced broken `#enrichment-geoip` anchors with `#enrichment`.
- Replaced literal `${CMDB_TOKEN}` examples with `<CMDB_TOKEN>` placeholders,
  matching the "headers are passed verbatim" contract.
- Replaced the non-existent `Application` grouping reference with
  `Destination Port` / service wording.
- Preserved all-interfaces/all-directions deployment guidance per the user
  decision; no ingress-only deployment change was made.

Quality review validation:

- Regenerated integration artifacts with
  `python3 integrations/gen_integrations.py` and
  `python3 integrations/gen_docs_integrations.py`.
- Grep validation returned no matches for:
  `${CMDB_TOKEN}`, `enrichment-geoip`, `Application`, `cat /proc/net/udp`,
  stale default-retention wording, `Network Flows tab`, and default-card
  "does not impose any limits" / "significant performance impact" wording in
  Network Flows generated cards.
- Residual internal/current-state wording scans over hand-authored docs,
  metadata, and generated card body text returned no matches.
- `python3 -c 'import yaml; yaml.safe_load(open("src/crates/netflow-plugin/metadata.yaml")); print("metadata yaml ok")'`
  passed.
- `git diff --check` passed for the SOW, Network Flows docs, metadata, and
  generated integration docs.

## Regression Closeout - 2026-05-08

Final status: completed.

Closeout evidence:

- Every regression finding F1-F26 has a repair note and validation evidence in
  this regression log.
- The final quality review findings were repaired in hand-authored docs,
  `metadata.yaml`, and regenerated integration cards.
- `cargo test --manifest-path src/crates/netflow-plugin/Cargo.toml timestamp_source -- --nocapture`
  passed with 5 tests. A pre-existing warning remains in
  `src/crates/netflow-plugin/src/startup_memory_tests.rs` for an unused
  `bytesize::ByteSize` import.
- `python3 integrations/gen_integrations.py` passed.
- `python3 integrations/gen_docs_integrations.py` passed.
- `python3 -c 'import yaml; yaml.safe_load(open("src/crates/netflow-plugin/metadata.yaml")); print("metadata yaml ok")'`
  passed.
- Targeted public-doc quality scans over `docs/network-flows/`,
  `src/crates/netflow-plugin/metadata.yaml`, and generated integration-card
  body text returned no matches for stale anchors, misleading credential
  placeholders, stale UI wording, invalid grouping field names, bad UDP-drop
  commands, internal-current-state wording, source-code citations, or generic
  default-behavior boilerplate.
- `git diff --check` passed for the SOW, Network Flows docs, metadata,
  generated integration docs, and touched netflow-plugin Rust files.
- `.agents/sow/audit.sh` was run after final move. It reported SOW 14
  status/directory as consistent in `done/` and failed only on an
  unrelated pre-existing sensitive-data pattern in
  `.agents/skills/mirror-netdata-repos/SKILL.md` plus existing TODO-file
  warnings. None of those are part of this SOW's staged scope.

Artifact maintenance gate for regression close:

- **AGENTS.md**: no update needed; no workflow or project-wide guardrail changed.
- **Runtime project skills**: no update needed; no new integration-pipeline or
  Learn-site process behavior was discovered beyond existing skill guidance.
- **Specs**: no update needed; this was a documentation/metadata repair and did
  not change the product contract beyond correcting public docs to existing
  behavior.
- **End-user/operator docs**: updated Network Flows docs, `metadata.yaml`, and
  regenerated integration cards.
- **End-user/operator skills**: no update needed; the user rejected AI-skill
  cross-linking as not mandatory for this SOW.
- **SOW lifecycle**: status changed to `completed`; file moved from
  `.agents/sow/current/` to `.agents/sow/done/` together with the repair
  commit.

Follow-up mapping:

- The original follow-up list remains tracked above. No new deferred item was
  introduced by the regression repair. The user explicitly rejected changing
  deployment guidance to ingress-only; double counting remains a visualization
  and interpretation guideline, not an ingestion-side avoidance requirement.

## Regression - 2026-05-08

### Trigger

PR #22449 received post-close reviewer and CI feedback after SOW 14 had been
marked `completed` and moved to `.agents/sow/done/`.

### Purpose

Bring PR #22449 back to merge-ready state for end-user Network Flows
documentation by addressing all valid review, CI, Codacy, and local Learn
preview findings. Add a durable, user-triggered skill/workflow for building
Learn locally with the contents of a documentation PR before merge, so future
documentation PRs can be inspected in a browser before release.

### Evidence

- `bash .agents/skills/pr-reviews/scripts/fetch-all.sh 22449` returned 9 open
  automated review threads, with no human review comments in the fetched
  snapshot.
- `bash .agents/skills/pr-reviews/scripts/ci-status.sh 22449` reported
  failures in `yamllint` and `check-documentation`, `ACTION_REQUIRED` from
  Codacy, 0 Sonar findings, and many still-running build checks.
- `bash .agents/skills/pr-reviews/scripts/fetch-sonar-findings.sh 22449`
  returned 0 Sonar issues and 0 hotspots.
- `bash .agents/skills/codacy-audit/scripts/pr-issues.sh 22449` failed locally
  before reporting findings because the helper passed a very large JSON array
  through `jq --argjson`, hitting `Argument list too long`.
- `python3 ${NETDATA_REPOS_DIR}/learn/ingest/ingest.py --help` failed in the
  system interpreter due to missing `pandas`; the checked-in Learn `venv`
  also lacked required packages such as `GitPython`.
- After commit `0a3eda6614` was pushed and all prior threads were replied to
  and resolved, `bash .agents/skills/pr-reviews/scripts/fetch-all.sh 22449`
  returned 9 new open automated review threads on the new PR head and no open
  human review threads.
- The new PR #22449 comments were verified against source evidence:
  `.github/workflows/check-markdown.yml:54-69` regenerates `COLLECTORS.md`
  before Learn ingest but does not diff-check the committed file;
  `src/crates/netflow-plugin/src/enrichment/init.rs:50-64` keeps the enricher
  enabled for provider-chain-only config; `network_sources/runtime.rs:24-40`
  scans loaded network-source records linearly; `network_sources/service.rs:91-96`
  logs HTTP refresh failures as warnings; and `reqwest-0.13.2` converts URL
  userinfo to HTTP Basic auth during request build.
- A second fetch during this repair returned 2 additional automated review
  threads: one on standalone CLI retention flags and one on ambiguous
  visualization panel count wording.
- The CLI retention finding was valid for standalone mode:
  `src/crates/netflow-plugin/src/plugin_config/runtime.rs:7-11` uses
  `PluginConfig::parse()` outside Netdata, and commit `f00390e2f5` removed the
  legacy top-level CLI flags while keeping `JournalConfig.tiers` skipped for
  Clap.
- After commit `6f5805979d` was pushed and 11 threads were replied to and
  resolved, the PR review watcher detected 2 new open automated review threads
  on `src/crates/netflow-plugin/src/ingest/rebuild.rs`.
- The new rebuild findings were valid: `rebuild_materialized_from_raw()` called
  `scan_journal_files_forward()` directly in the async startup path, while query
  paths already use `tokio::task::spawn_blocking()` for blocking journal scans;
  the rebuild timeout was checked only every 1024 scanned entries.

### Open repair items

- R8.1: Rewrite the SOW close-gate paragraph at the prior
  `SOW-0014...md:1060` in historical tense.
- R8.2: Fix the classifier integration example so the regex matches the
  documented three-letter region suffix.
- R8.3: Remove or correct the decapsulation troubleshooting advice that
  suggests multiple plugin instances despite the documented single-instance
  model.
- R8.4: Correct the Fedora/RHEL `geoipupdate` package name.
- R8.5: Make the static-metadata typo example actually use an unknown key.
- R8.6: Strengthen `main_tests.rs` so timestamp-source persistence cannot pass
  when both journal fields are missing.
- R8.7: Tighten sizing guidance around the ~25k flows/s single-agent planning
  ceiling.
- R8.8: Make storage safety-margin guidance internally consistent.
- R8.9: Correct the UDP troubleshooting note to acknowledge per-socket
  `drops` in `/proc/net/udp` while keeping `RcvbufErrors` as the system-wide
  counter.
- R8.10: Fix or work around the Codacy PR-fetch helper failure, then triage
  Codacy findings for PR #22449.
- R8.11: Investigate and fix the CI `yamllint` and `check-documentation`
  failures.
- R8.12: Create durable local Learn preview guidance that triggers only when a
  user explicitly asks to build/inspect Learn from a PR.
- R8.13: Build or serve Learn locally using PR #22449 contents and record the
  ingest/build/browser validation result.
- R8.14: Fix automated review comments about contradictory retention comments
  and stale shared-budget retention wording.
- R8.15: Fix the broken decapsulation integration icon found during local Learn
  browser inspection.
- R8.16: Verify integration-card source links in hand-authored Network Flows
  docs are compatible with Learn ingest.
- R8.17: Restore bounded startup behavior for receive-time raw rebuild scans.
- R8.18: Reconcile the SOW scope text so the durable record matches the
  accepted documentation scope.
- R8.19: Ensure the generated Learn "Monitor anything" page lists Network
  Flows protocols and enrichment integrations, and update the integration
  lifecycle skill with that mechanism.
- R8.20: Normalize Network Flows catalog descriptions so flow-source rows say
  they collect network flow records and enrichment rows say they enrich or
  annotate network flows, instead of describing provider publication
  mechanisms, variables, defaults, or setup settings.
- R8.21: Correct `integrations-lifecycle` guidance so it says
  `check-markdown.yml` catches broken generated `COLLECTORS.md` content during
  Learn ingest, not stale committed artifact drift.
- R8.22: Update local Learn preview guidance to copy tracked plus untracked
  non-ignored PR files into the isolated source preview.
- R8.23: Add the 32-bit packaging caveat anywhere docs tell users to run
  `topology-ip-intel-downloader`.
- R8.24: Replace AWS `transform: "."` "empty result" wording with the actual
  row-mapping/schema failure around the missing required `prefix` field.
- R8.25: Correct URL-credential docs for remote network sources: URL userinfo
  becomes HTTP Basic auth, while explicit headers remain recommended.
- R8.26: Correct enrichment docs so provider-chain-only config is described as
  enabling the enricher.
- R8.27: Make the Codacy helper's temporary file pattern portable.
- R8.28: Correct NetBox/network-source failure and performance wording:
  refresh failures are warning logs, and runtime cost scales with loaded
  network-source records instead of claiming trie lookup.
- R8.29: Sweep same-class wording across generated cards and hand-authored
  docs so the next review does not rediscover the same problems in adjacent
  pages.
- R8.30: Restore standalone CLI retention tuning without reopening the YAML
  global-retention schema.
- R8.31: Clarify the visualization overview panel count so the text matches the
  listed UI surfaces.
- R8.32: Move receive-time raw rebuild journal scanning and decompression off
  the async startup task so Tokio runtime workers are not blocked.
- R8.33: Enforce raw rebuild timeout checks for every scanned entry rather than
  only every 1024 entries.

### Validation plan

- Re-fetch all PR comments before push and verify no new review items were
  missed.
- Run the narrow docs and Rust validations affected by the fixes.
- Run local Learn ingest/build or dev-server preview with the local PR checkout
  as the `netdata` source.
- Record the local Learn URL, process PID, and cleanup path if a preview server
  is started.
- Move this SOW back to `.agents/sow/done/` after PR review items, relevant
  CI failures, and local Learn preview validation are handled.

### Repair completed

- R8.1: Reworded the prior close-gate paragraph in historical tense.
- R8.2: Fixed the classifier example regex to match the documented
  three-letter region suffix.
- R8.3: Removed the decapsulation advice that implied running multiple plugin
  instances.
- R8.4: Replaced `GeoIP-update` with the Fedora/RHEL package name
  `geoipupdate`.
- R8.5: Changed the static-metadata typo example to `if_index`, leaving
  accepted aliases documented separately.
- R8.6: Strengthened the timestamp-source e2e test so missing raw journal
  fields cannot pass as `None == None`.
- R8.7/R8.8: Tightened the sizing page around the ~25k flows/s planning
  envelope and made the storage safety margin consistently `1.2x to 1.5x`.
- R8.9: Corrected UDP troubleshooting so `/proc/net/udp` is described as
  per-socket `drops`, while `/proc/net/snmp` `RcvbufErrors` remains the
  system-wide signal.
- R8.10: Fixed the Codacy helper to avoid passing a large issue array through
  `jq --argjson`; it now uses a temporary JSON file and `--slurpfile`.
- R8.11: Fixed `yamllint` findings in `metadata.yaml` and `configs/netflow.yaml`;
  replaced hand-authored Learn links to generated integration pages with source
  markdown links so Learn ingest rewrites them correctly.
- R8.12: Added `.agents/skills/learn-pr-preview/SKILL.md`, updated
  `AGENTS.md`, and added
  `.agents/skills/learn-site-structure/how-tos/preview-documentation-pr-locally.md`.
- R8.13: Built an isolated Learn preview from PR #22449 contents, ran ingest,
  built Docusaurus, served the static build locally, and browser-inspected
  representative Network Flows pages.
- R8.14: Reworded the Rust retention comment so optional fields and the
  resolved-tier validation rule agree; corrected visualization docs that still
  described raw-tier retention as a shared budget.
- R8.15: Replaced the missing `tunnel.svg` decapsulation icon with the existing
  hosted `network-wired.svg` icon and regenerated integration artifacts.
- R8.16: Verified by local ingest that source docs must link to the source
  integration markdown files; Learn correlates those links to final routes.
  Direct final `/docs/network-flows/...` links in source markdown fail
  `--fail-links-netdata`, so the source integration links were retained.
- R8.17: Restored the 30-second raw rebuild timeout for the new direct
  receive-time raw scan path, checking the elapsed time during scan progress.
- R8.18: Updated the SOW purpose/request wording to match the final accepted
  scope: document existing Network Flows behavior and exclude non-existent
  topology drilldown behavior.
- R8.19: Updated `integrations/gen_doc_collector_page.py` so the top-level
  `flows` category is grouped as a first-class `Network Flows` section in
  `src/collectors/COLLECTORS.md`; regenerated the file; updated the
  `integrations-lifecycle` skill and added a how-to for this generator rule.
- R8.20: Updated `src/crates/netflow-plugin/metadata.yaml` so generated
  Network Flows catalog rows use action-oriented user copy:
  `Collect network flow records...`, `Enrich network flows...`, or
  `Annotate network flows...`; regenerated per-integration markdown and
  `src/collectors/COLLECTORS.md`; updated the `integrations-lifecycle`
  skill with description-authoring rules.
- R8.21: Updated `.agents/skills/integrations-lifecycle/pipeline.md` so
  `check-markdown.yml` is documented as regenerating `COLLECTORS.md` before
  Learn ingest and catching broken generated content, while artifact drift is
  left to the integration regeneration workflow.
- R8.22: Updated `.agents/skills/learn-pr-preview/SKILL.md` and the matching
  Learn-site how-to to use `git ls-files -co --exclude-standard`, so previews
  include intentional untracked PR docs without copying ignored build output.
- R8.23: Added the packaged 32-bit downloader caveat to IPtoASN and DB-IP
  generated cards plus hand-authored installation, validation, and downloader
  docs.
- R8.24: Updated AWS IP Ranges metadata and generated card so the default
  `transform: "."` failure is described as missing required `prefix` rows, not
  as empty transform output.
- R8.25: Updated generic JSON-over-HTTP IPAM metadata/generated docs and the
  hand-authored enrichment page so URL userinfo is described as HTTP Basic auth
  conversion, with explicit `Authorization` headers recommended for clarity.
- R8.26: Updated the hand-authored enrichment page so the enricher is described
  as running when any enrichment feature is configured, including provider
  chains.
- R8.27: Updated `.agents/skills/codacy-audit/scripts/pr-issues.sh` and its
  how-to to use an explicit portable `mktemp` template.
- R8.28/R8.29: Updated AWS, Azure, NetBox, and generic HTTP network-source
  generated cards so runtime enrichment cost is described as prefix matching
  over loaded records; updated NetBox troubleshooting so HTTP errors are logged
  as refresh-failed warnings.
- R8.30: Restored
  `--netflow-retention-size-of-journal-files` and
  `--netflow-retention-duration-of-journal-files` as CLI-only compatibility
  aliases. They apply uniformly to all tiers in standalone mode while YAML
  remains per-tier-only.
- R8.31: Reworded the visualization overview to list five panel types:
  Sankey, Table, Time-Series, maps, and the 3D globe.
- R8.32: Moved the blocking raw rebuild journal scan into
  `tokio::task::spawn_blocking()` and streamed parsed `FlowFields` back to the
  async ingest task over a bounded channel before observing materialized tiers.
- R8.33: Changed the rebuild scan timeout guard to check elapsed time on every
  entry, before and after payload parsing.

### Code-only review handling

The requested code-only subagent review found timestamp consistency risks. On
verification, the important false premise was that `timestamp_source` should
drive dashboard query windows or rollup tier selection. The public contract in
`docs/network-flows/configuration.md` says the Network Flows view uses journal
entry receive time for query windows and tier selection; `timestamp_source`
controls stored source timestamp metadata.

Repairs:

- Kept live materialized tier observation on receive time, matching the public
  contract and raw journal append-time ordering.
- Changed rebuild to scan recently received raw entries by journal entry time
  and replay them into materialized tiers by receive time, instead of querying
  the raw journal by `_SOURCE_REALTIME_TIMESTAMP`.
- Changed the rebuild upper bound to include the current second.
- Extended the timestamp-source e2e test to prove:
  raw `_SOURCE_REALTIME_TIMESTAMP` equals the decoded flow start timestamp;
  raw journal entry realtime remains receive/write time;
  live materialized tiers use the receive-time bucket; and rebuild replays raw
  entries into the same receive-time bucket.

### Validation evidence

- `cargo test --manifest-path src/crates/netflow-plugin/Cargo.toml timestamp_source -- --nocapture`
  passed: 5 tests passed, 0 failed. The only warning was the pre-existing
  unused `bytesize::ByteSize` import in
  `src/crates/netflow-plugin/src/startup_memory_tests.rs`.
- `cargo fmt --manifest-path src/crates/netflow-plugin/Cargo.toml --check`
  passed after formatting.
- `yamllint src/crates/netflow-plugin/metadata.yaml src/crates/netflow-plugin/configs/netflow.yaml`
  passed.
- `python3 integrations/gen_integrations.py` passed.
- `python3 integrations/gen_docs_integrations.py` passed.
- `git diff --check` passed.
- A hosted-icon check over all Network Flows metadata icons passed:
  `network-wired.svg` returned `200 image/svg+xml`.
- `python3 integrations/gen_doc_collector_page.py` passed and the generated
  `src/collectors/COLLECTORS.md` contains a `### Network Flows` section with
  NetFlow, IPFIX, sFlow, and enrichment integrations.
- The generated `src/collectors/COLLECTORS.md` Network Flows table now uses
  catalog-style descriptions such as `Enrich network flows with...`,
  `Annotate network flows with...`, and `Collect network flow records...`
  rather than setup, option, or provider-publication wording.
- Final pre-commit reviewer verification re-fetched PR #22449 and confirmed
  12 open threads on the old pushed head; every thread was checked against the
  current local tree and the corresponding fix is present in source/generated
  files before committing.
- `git diff --check` passed.
- `cargo fmt --manifest-path src/crates/netflow-plugin/Cargo.toml --check`
  passed.
- `python3 -c 'import yaml; yaml.safe_load(open("src/crates/netflow-plugin/metadata.yaml"))'`
  passed.
- `.agents/sow/audit.sh` exited 2 because of pre-existing unrelated repository
  hygiene findings: a sensitive-pattern warning in
  `.agents/skills/mirror-netdata-repos/SKILL.md`, the unrelated current
  SOW-0012 gate warning, non-project skill classification warnings, and
  existing root TODO files. SOW 14 itself reports status/directory consistency
  as `completed` in `.agents/sow/done/`.
- Local Learn ingest from an isolated source copy passed with
  `--local-repo netdata:<preview-source> --ignore-on-prem-repo --use_plain_https --fail-links-netdata`.
- Local Learn Docusaurus build passed with Node `22.14.0`, Yarn `1.22.22`,
  and `NODE_OPTIONS=--max_old_space_size=4096`.
- Build warnings were not PR-specific Network Flows failures: Docusaurus still
  reports existing site-wide broken anchors and an existing duplicate
  `/docs/collecting-metrics/service-discovery` route.
- Browser inspection returned HTTP 200, expected H1, no 404 page, and no
  MDX/runtime error for:
  `/docs/network-flows/`,
  `/docs/network-flows/retention-and-tiers`,
  `/docs/network-flows/enrichment-methods/static-metadata`,
  `/docs/network-flows/flow-protocols/netflow`, and
  `/docs/network-flows/configuration`.
- The Static Metadata page rendered placeholders such as
  `enrichment.metadata_static.exporters.<key>.if_indexes` as readable text, not
  literal `&lt;key&gt;` and not MDX JSX.
- Browser inspection of
  `/docs/network-flows/enrichment-methods/decapsulation` confirmed the
  integration icon loaded successfully from `network-wired.svg` with non-zero
  rendered dimensions. Only external analytics requests failed in the browser
  session.
- Second review-iteration validation after the 9 new automated comments:
  `python3 integrations/gen_integrations.py`,
  `python3 integrations/gen_docs_integrations.py`, and
  `python3 integrations/gen_doc_collector_page.py` passed.
- `yamllint src/crates/netflow-plugin/metadata.yaml src/crates/netflow-plugin/configs/netflow.yaml`
  passed.
- `python3 -c 'import yaml; yaml.safe_load(open("src/crates/netflow-plugin/metadata.yaml"))'`
  passed.
- `bash -n .agents/skills/codacy-audit/scripts/pr-issues.sh` passed.
- `git diff --check` passed.
- `cargo fmt --manifest-path src/crates/netflow-plugin/Cargo.toml --check`
  passed.
- `cargo test --manifest-path src/crates/netflow-plugin/Cargo.toml journal_cli_retention_aliases_apply_to_all_tiers -- --nocapture`
  passed: 1 test passed, 0 failed. The only warning was the pre-existing unused
  `bytesize::ByteSize` import in `startup_memory_tests.rs`.
- `cargo test --manifest-path src/crates/netflow-plugin/Cargo.toml plugin_config::tests:: -- --nocapture`
  passed: 27 tests passed, 0 failed. The only warning was the same pre-existing
  unused `bytesize::ByteSize` import in `startup_memory_tests.rs`.
- The post-fix wording sweep found no remaining instances of the exact stale
  phrases called out by the second review pass.
- Third review-iteration fetch after commit `6f5805979d` returned 2 new open
  automated threads, both in `src/crates/netflow-plugin/src/ingest/rebuild.rs`.
- `cargo test --manifest-path src/crates/netflow-plugin/Cargo.toml e2e_timestamp_source_first_switched_is_persisted_as_source_timestamp -- --nocapture`
  passed: 1 test passed, 0 failed. This test exercises the raw rebuild path
  after deleting materialized tier directories. The only warning was the same
  pre-existing unused `bytesize::ByteSize` import in `startup_memory_tests.rs`.
- `cargo test --manifest-path src/crates/netflow-plugin/Cargo.toml plugin_config::tests:: -- --nocapture`
  passed after the rebuild repair: 27 tests passed, 0 failed. The only warning
  was the same pre-existing unused `bytesize::ByteSize` import.
- `cargo fmt --manifest-path src/crates/netflow-plugin/Cargo.toml --check`
  passed after the rebuild repair.
- `cargo test --manifest-path src/crates/netflow-plugin/Cargo.toml -- --nocapture`
  passed after the rebuild repair: 428 tests passed, 18 ignored, and the
  vendored gRPC proto test passed. The visible lock-poison panic is expected
  from a recovery test that still passed. The only compiler warning was the
  same pre-existing unused `bytesize::ByteSize` import.

### Artifact maintenance gate

- **AGENTS.md**: updated to list the new `learn-pr-preview` skill and its
  explicit trigger.
- **Runtime project skills**: added `learn-pr-preview`; updated
  `learn-site-structure` with a local PR preview how-to; updated
  `codacy-audit` for the large PR issue-list fetch gotcha; updated
  `integrations-lifecycle` for the generated `COLLECTORS.md` / Monitor
  Anything Network Flows section mechanism and metadata description-authoring
  rules.
- **Specs**: no spec update needed; the code repair preserves the documented
  timestamp contract rather than changing product behavior.
- **End-user/operator docs**: updated Network Flows docs, `metadata.yaml`, and
  regenerated integration cards.
- **End-user/operator skills**: no update needed; the new workflow is a
  repo-work skill for agents validating documentation PRs, not an end-user
  operator skill.
- **SOW lifecycle**: SOW 14 repair is complete; status is `completed`, and
  the file is moved back to `.agents/sow/done/` in the same commit as the
  repair.

## Regression - 2026-05-08 - Flow Integration Section Rendering

### What broke

The merged network-flow integration artifacts expose `metrics` as a JSON object
and `alerts` as a JSON array for `integration_type: flows` entries. Downstream
surfaces expect rendered markdown strings for integration content sections.

Evidence:

- `integrations/schemas/flows.json` delegates to `collector.json`, so flow
  metadata legitimately contains collector-style `metrics` and `alerts`.
- `integrations/gen_integrations.py` `FLOWS_RENDER_KEYS` rendered only
  `overview`, `related_resources`, `setup`, and `troubleshooting`, leaving
  `metrics` and `alerts` in their source YAML shape.
- The website PR generated from the merged artifacts failed production-pinned
  Hugo `0.140.0` because `themes/tailwind/layouts/partials/integration-tabs.html`
  calls `markdownify` on `.integration.metrics`.
- The in-app integrations renderer in cloud-frontend adds tabs for truthy
  `metrics` and `alerts`, while its Markdoc wrapper parses only string input.
  The result would be blank Metrics and Alerts tabs after the next data sync.
- The cloud-frontend integration link checker calls `.match()` on markdown
  fields, so raw objects/arrays in `metrics`/`alerts` can break that check too.

### Why previous validation missed it

The earlier closeout validated the Netdata integrations generator and local
Learn ingest/build, but did not validate the website Hugo renderer or the
cloud-frontend integrations consumer against the newly introduced
`integration_type: flows` artifacts. `gen_integrations.py` itself accepted the
raw structured fields because they are schema-valid before rendering.

### Repair plan

Render flow `metrics` and `alerts` through the same markdown templates used by
collector integrations. This keeps the source metadata schema unchanged and
restores the downstream contract: content sections in `integrations.json` and
`integrations.js` are markdown strings.

### Validation plan

- Run `python3 integrations/gen_integrations.py`.
- Verify all `flows` entries in `integrations/integrations.json` have string
  `metrics` and string `alerts`.
- Run `python3 integrations/gen_docs_integrations.py`.
- Run `python3 integrations/gen_doc_collector_page.py`.
- Rebuild the website PR artifacts with the repaired generated
  `integrations.json` and production-pinned Hugo `0.140.0`.
- Verify cloud-frontend's current renderer and link checker receive strings
  for `flows` `metrics` and `alerts`.

### Artifact updates needed

- **Code**: update `integrations/gen_integrations.py` flow render keys.
- **Generated artifacts**: regenerate `integrations/integrations.json`,
  `integrations/integrations.js`, per-integration markdown, and
  `src/collectors/COLLECTORS.md` if the generator changes them.
- **Runtime project skills**: update `integrations-lifecycle` if the durable
  downstream contract was not already documented clearly enough.
- **Specs**: no product behavior change is expected; this repairs generated
  publishing artifacts.
- **End-user/operator docs**: no content change is expected beyond generated
  artifacts.

### Repair completed

`integrations/gen_integrations.py` now renders the flow `alerts`, `metrics`,
and `functions` sections through the standard templates, matching the
collector-like schema that `flows.json` delegates to.

### Validation evidence

- `python3 integrations/gen_integrations.py` passed.
- `python3 integrations/gen_docs_integrations.py` passed.
- `python3 integrations/gen_doc_collector_page.py` passed.
- Flow artifact type check passed: every `integration_type: flows` entry in
  `integrations/integrations.json` has string `metrics`, string `alerts`, and
  string `functions`.
- Non-deploy markdown-field type check passed: no non-deploy integration emits
  raw object or array values for `overview`, `setup`, `troubleshooting`,
  `alerts`, `metrics`, `functions`, or `related_resources`.
- Website validation passed in a temporary copy of PR #1212 using the repaired
  `integrations.json` and production-pinned Hugo `0.140.0`: `hugo --gc
  --minify` built 3141 pages successfully.
- Cloud-frontend compatibility check passed against the repaired
  `integrations.json`: all flow markdown fields inspected by
  `scripts/checkIntegrations.js` were strings, and `getMarkdownUrls()` extracted
  52 markdown URLs without throwing.
- `git diff --check` passed.
- `.agents/sow/audit.sh` exited 2 because of a pre-existing unrelated
  sensitive-pattern warning in
  `.agents/skills/mirror-netdata-repos/SKILL.md`; SOW status/directory
  consistency passed and SOW 14 reports `completed` in `.agents/sow/done/`.

### Artifact maintenance gate

- **AGENTS.md**: no update needed; this does not change repo-wide workflow.
- **Runtime project skills**: updated `integrations-lifecycle` with the
  downstream markdown-string contract and the new-integration-type validation
  checklist.
- **Specs**: no update needed; product behavior and public data semantics did
  not change.
- **End-user/operator docs**: no hand-authored user docs changed; this repairs
  generated publishing artifacts.
- **End-user/operator skills**: no update needed; no operator workflow changed.
- **SOW lifecycle**: SOW 14 reopened as a regression, status returned to
  `completed`, and the file is moved back to `.agents/sow/done/` in the same
  commit as the repair.

### Follow-up mapping

No deferred follow-up remains for this regression. The website integration PR
must be regenerated after this repair reaches `netdata/master`; that is the
normal downstream propagation path rather than a separate source-code TODO.
