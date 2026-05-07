# SOW-0014 - Network Flows Documentation & Integration Infrastructure

## Status

Status: completed

Closed 2026-05-07. All acceptance criteria delivered or explicitly rejected. Phase 1C (AI skills cross-links) was rejected as not mandatory by user decision -- recorded in Validation. Architectural pivot to a top-level `flows` integration_type with 14 cards is recorded as Decision 10. Companion changes shipped in netdata PR #22439, learn PR #2854 (merged), and a deferred cloud-frontend `Update integrations.js` refresh.

## Requirements

### Purpose

Provide DevOps/SREs a complete, authoritative documentation and integration infrastructure for Netdata's network flow analysis. Documentation covers only tested and verified features. Untested features (BMP, BioRIS, Network Sources, Topology drilldown) are deferred to a follow-up SOW after testing/validation. The documentation must include sizing, capacity planning, and optimization guidance so businesses can make informed deployment decisions.

### User Request

Phase 1: Document tested features only. Include sizing/benchmarking/capacity planning.
Phase 2 (follow-up SOW): Test BMP, BioRIS, Network Sources, Topology drilldown, then document them.

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

None yet.
