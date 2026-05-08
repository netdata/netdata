# SOW-0015 - NetFlow Enrichment Verification

## Status

Status: completed

Sub-state: End-user downloader documentation mismatch on supported providers corrected.

## Requirements

### Purpose

Ensure the Netdata Agent ships network-flow enrichment methods that are fit for DevOps/SRE use: tested in code, validated in practice where possible, documented with working instructions, and proven to produce the fields users expect.

### User Request

The user wants the NetFlow enrichment methods fully tested, including unit tests, integration tests, practical validation on the locally running Netdata Agent, and documentation corrections when instructions are wrong. The user asked the assistant to work autonomously and stop only when progress requires user involvement.

Correction from the user: the scope includes all enrichment paths, plus possible NetFlow and sFlow validation gaps. The user is already testing IPFIX on this workstation, and the default geolocation database is known to work. The priority is to test everything else that is not already covered by unit tests; CI-job integration can come later and is not the immediate gate for this SOW.

The private live flow exporter endpoint is intentionally not recorded here. Runtime evidence may refer to it as `[PRIVATE_FLOW_EXPORTER]`.

Scope:

1. Flow protocols with missing proof: NetFlow and sFlow. IPFIX is treated as a locally working baseline because the user is actively testing it on this workstation.
2. Default geolocation database: treated as a working baseline, but existing unit/runtime proof will be recorded.
3. DB-IP IP Intelligence.
4. IPtoASN.
5. Custom MMDB Database.
6. MaxMind GeoIP / GeoLite2.
7. Static metadata.
8. Sampling overrides.
9. Static networks.
10. Classifiers.
11. ASN and network provider chains.
12. Decapsulation enrichment.
13. AWS IP Ranges.
14. Azure IP Ranges.
15. GCP IP Ranges.
16. Generic JSON-over-HTTP IPAM.
17. NetBox.
18. bio-rd / RIPE RIS.
19. BMP.

### Clarified Beta Validation Target

The target is **not** proving that every external routing stack is production-stable. The target is proving Netdata's side is sane:

- It connects when given valid config.
- It rejects or reports bad config clearly.
- It consumes the expected payload/schema.
- It maps fields correctly into enrichment state.
- It respects configured options.
- It does not panic, hang, leak tasks, or silently ignore data.
- The docs describe commands/configs that actually work.
- The beta can ship without obvious "this was never exercised" failures.

This SOW must not block on the user having a router that supports BMP or a production bio-rd/RIS deployment.

For BMP, GoBGP/FRR plus deterministic BMP frames are sufficient to validate Netdata's listener and route ingestion contract.

For BioRIS, a deterministic bio-rd-compatible gRPC service is sufficient to validate Netdata's client behavior. Running upstream bio-rd is useful as a smoke test, but it is not a hard shipping blocker if upstream itself is unstable in a local lab.

End-user documentation remains product documentation. It must describe supported commands, configuration, payload/schema expectations, limitations, and troubleshooting. It must not discuss internal test gaps, untested status, or why a test could not be performed.

### Assistant Understanding

Facts:

- The current `netdata` instance on this workstation is available for practical validation and already receives flow traffic from `[PRIVATE_FLOW_EXPORTER]`.
- `build-install-netflow-plugin.sh` builds and installs the Rust `netflow-plugin`, then restarts `netdata` unless called with `--no-restart`.
- The backend stores flow records in journals, so local validation can inspect journal-backed output through plugin queries and, where useful, `journalctl`.
- Prior NetFlow documentation work recorded that BMP, BioRIS, and Network Sources had parser/transform unit tests but lacked runtime I/O integration tests.
- User-facing documentation is in scope when validation proves instructions are wrong or incomplete.
- The user reports IPFIX and the default geolocation database already work on this workstation.

Inferences:

- The core gap is not absence of all tests; it is missing proof across the real boundaries for methods not already covered: documented config, MMDB files, HTTP fetchers, gRPC streams, BMP TCP sessions, runtime state publication, flow enrichment, journal-backed query output, and generated integration documentation.
- BMP and bio-rd validation can progress without the user by using local deterministic speakers, public/open-source fixtures, FRR/GoBGP, and a bio-rd-compatible gRPC test service. Production-router proof is not an acceptance criterion for this beta validation pass.

Unknowns:

- Whether all required fixture formats can be generated in Rust using existing dependencies, or whether narrow dev-dependencies are required.
- Whether a practical local bio-rd setup can be made reliable enough for automated validation without pulling in a large external runtime.
- Whether the installed local `netdata` configuration can be safely changed in-place for all live validation cases, or whether some cases should use temporary config and `--no-restart` style isolated test runs.

### Acceptance Criteria

- Each scoped enrichment method has automated unit or integration-test coverage proving the configured source is parsed/fetched/accepted and produces expected enrichment attributes, or the SOW records existing coverage and why no new test was needed.
- NetFlow and sFlow have automated validation for protocol decode/enrichment interaction when existing tests do not already prove it.
- Runtime I/O paths are covered where applicable:
  - HTTP fetch and refresh for cloud/IPAM/NetBox network sources.
  - gRPC `GetRouters`, `DumpRIB`, and `ObserveRIB` behavior for BioRIS or an equivalent local fake.
  - BMP TCP listener behavior with real BMP-framed bytes, route upsert, withdrawal, and session cleanup.
- At least one test path proves enrichment reaches flow records and the fields exposed by journal-backed query output.
- Documentation examples and setup instructions for the scoped methods are executed or mechanically tested where possible; incorrect instructions are fixed from source files and generated artifacts are regenerated.
- Practical local validation is attempted on the installed Netdata Agent using the build/install script and journal-backed output.
- Any remaining blockers are listed with evidence and the exact user involvement required.

## Analysis

Sources checked:

- `AGENTS.md`
- `.agents/sow/SOW.template.md`
- `.agents/sow/done/SOW-0014-20260506-netflow-sflow-ipfix-documentation-guide.md`
- `.agents/sow/specs/sensitive-data-discipline.md`
- `.agents/skills/project-writing-collectors/SKILL.md`
- `.agents/skills/integrations-lifecycle/SKILL.md`
- `src/crates/netflow-plugin/Cargo.toml`
- `src/crates/Cargo.toml`
- `src/crates/netflow-plugin/src/`
- `src/crates/netflow-plugin/metadata.yaml`
- `src/crates/netflow-plugin/configs/netflow.yaml`
- `docs/network-flows/`
- `build-install-netflow-plugin.sh`
- Local mirrored upstream repositories for BMP/BioRIS/network-source fixture patterns.

Current state:

- `src/crates/netflow-plugin/src/main.rs` starts BMP, BioRIS, and network-source tasks when configured.
- `src/crates/netflow-plugin/src/decoder/state/runtime/decode.rs` applies the enricher to decoded flow records.
- Existing tests cover many pure parsing and transform helpers, but not all documented real-source workflows.
- `src/crates/netflow-plugin/Cargo.toml` has no HTTP mock or MMDB writer dev-dependency at SOW start.

Risks:

- Tests that rely on live Internet endpoints can become flaky; automated tests should use frozen, attributed fixtures and local fake services so they are suitable for future CI even though CI-job wiring is not the immediate target.
- Real cloud/IPAM/BGP payloads may contain sensitive network information; durable artifacts must use sanitized fixtures only.
- Installing and restarting the local plugin affects the user's running Netdata instance; use the provided script deliberately and record the command/result.
- Documentation is generated from metadata for integration cards; generated files must not be hand-edited.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- NetFlow enrichment has broad configuration and documentation coverage, but several enrichment methods lack tests at the boundary where real failures happen. A method can have parser tests and still fail because the documented transform is stale, the HTTP fetcher does not publish runtime state, the MMDB schema is not actually decoded, the BMP listener never accepts a real frame, NetFlow/sFlow decode does not interact correctly with enrichment, or the final enriched attributes never reach journal-backed query output.

Evidence reviewed:

- Prior SOW evidence: `.agents/sow/done/SOW-0014-20260506-netflow-sflow-ipfix-documentation-guide.md` records runtime I/O gaps for BMP, BioRIS, and Network Sources.
- Local code evidence:
  - `src/crates/netflow-plugin/src/main.rs` task startup for dynamic enrichment.
  - `src/crates/netflow-plugin/src/network_sources/fetch.rs` HTTP fetch path.
  - `src/crates/netflow-plugin/src/network_sources/service.rs` refresh/publish path.
  - `src/crates/netflow-plugin/src/routing/bmp/listener.rs` BMP TCP listener path.
  - `src/crates/netflow-plugin/src/routing/bioris/runtime/refresh/instance.rs` BioRIS router/RIB refresh path.
  - `src/crates/netflow-plugin/src/enrichment/resolve.rs` provider-chain and merge behavior.
  - `src/crates/netflow-plugin/src/flow/record/journal/network.rs` journal field output.
- Open-source reference evidence:
  - `akvorado/akvorado @ 646eb033a57692f5c2918bb65768e15c921b766e`, `outlet/routing/provider/bmp/root_test.go`, `outlet/routing/provider/bmp/tests.go`, `outlet/routing/provider/bioris/root_test.go`, `cmd/akvorado/testdata/configurations/clickhouse-network-sources/in.yaml`.
  - `pmacct/pmacct @ b06e6829bdc523200cbed2942b7045d64146cb71`, `tests/208-BMP-mem-leak-test/`, `tests/400-IPFIXv10-BMP-CISCO-SRv6-multiple-sources/`.

Affected contracts and surfaces:

- Rust code under `src/crates/netflow-plugin/src/`.
- Test fixtures under `src/crates/netflow-plugin/testdata/`, with attribution.
- `src/crates/netflow-plugin/Cargo.toml` and possibly `src/crates/Cargo.toml` for narrow test dependencies.
- User-facing source docs under `docs/network-flows/`.
- Integration source metadata `src/crates/netflow-plugin/metadata.yaml`.
- Generated integration cards under `src/crates/netflow-plugin/integrations/`.
- Installed local `netdata` runtime during practical validation.
- SOW/spec/skill artifacts if durable workflow knowledge changes.

Existing patterns to reuse:

- Existing Rust module-local `tests.rs` files.
- Existing flow pcap fixtures and `src/crates/netflow-plugin/testdata/ATTRIBUTION.md`.
- Existing `ingest_test_support` and query tests for journal-backed records.
- Existing integration generator workflow from `integrations-lifecycle`.
- Akvorado-style BMP fixture tests and network-source transform tests, adapted to Netdata's codebase and licensing constraints.

Risk and blast radius:

- Most implementation should be test-only, but validation may expose runtime bugs requiring production code fixes.
- BMP and BioRIS involve async network services; tests must use deterministic localhost ports, explicit shutdown, and timeouts.
- Live validation on the installed Agent can restart `netdata`; this is acceptable per user-provided workflow but must use targeted process discipline and avoid broad kills.
- Docs changes affect generated public integration pages and Learn ingestion; regeneration and link/content checks are required when docs change.

Sensitive data handling plan:

- Do not write literal private IPs, endpoints, node IDs, tokens, real customer data, or personal names to SOWs, specs, docs, skills, code comments, or fixtures.
- Use `[PRIVATE_FLOW_EXPORTER]`, RFC documentation addresses, loopback addresses, or synthetic fixture data.
- Store any raw runtime outputs that may contain private data only under `.local/`, which is gitignored; summarize redacted evidence in this SOW.
- If a live output contains sensitive data needed for debugging, redact before recording and do not commit the raw output.

Implementation plan:

1. Build a coverage matrix from code, docs, config, metadata, and tests for all enrichment methods plus NetFlow/sFlow protocol validation.
2. Add deterministic fixtures and unit/contract tests for enrichment methods that are not already proven.
3. Add integration-style tests that prove runtime state feeds flow enrichment and journal/query-visible fields.
4. Correct source documentation and metadata examples that fail validation; regenerate generated integration artifacts.
5. Run narrow Rust tests, docs/generator validation, and any broader CI-equivalent commands needed by touched surfaces.
6. Build/install the plugin with `build-install-netflow-plugin.sh`, restart `netdata`, and validate practical behavior with journal-backed output.
7. Record exact remaining blockers that require user-provided external systems or credentials.

Validation plan:

- Run targeted `cargo test -p netflow-plugin ...` commands from `src/crates`.
- Run full `cargo test -p netflow-plugin` if feasible after targeted tests pass.
- Run integration generator commands when metadata or generated cards change.
- Validate docs snippets/transforms against frozen or live-official payload shapes where feasible.
- Run the local build/install script and verify the installed plugin produces enriched flow output.
- Inspect `journalctl` and/or plugin query paths for runtime errors and expected enriched fields.
- Search for same-failure patterns across provider docs/tests before closing.

Artifact impact plan:

- AGENTS.md: no update expected unless workflow rules are found wrong.
- Runtime project skills: update only if this work discovers durable collector/integration workflow knowledge not already captured.
- Specs: create or update a NetFlow enrichment behavior spec if tests establish durable contracts not captured elsewhere.
- End-user/operator docs: update docs and metadata when instructions are wrong or incomplete.
- End-user/operator skills: update only if public query skills are affected by changed flow-query instructions.
- SOW lifecycle: this SOW is active in `current/`; close only after validation, artifact gates, and follow-up mapping.

Open-source reference evidence:

- `akvorado/akvorado @ 646eb033a57692f5c2918bb65768e15c921b766e`
  - `outlet/routing/provider/bmp/root_test.go`
  - `outlet/routing/provider/bmp/tests.go`
  - `outlet/routing/provider/bioris/root_test.go`
  - `cmd/akvorado/testdata/configurations/clickhouse-network-sources/in.yaml`
- `pmacct/pmacct @ b06e6829bdc523200cbed2942b7045d64146cb71`
  - `tests/208-BMP-mem-leak-test/`
  - `tests/400-IPFIXv10-BMP-CISCO-SRv6-multiple-sources/`

Open decisions:

- Resolved by user: pursue full testing, practical validation, unit/integration coverage, and documentation correction.
- Resolved by user: assistant may proceed autonomously and stop only when user involvement is strictly required.
- Resolved by user: immediate scope is tests and practical validation, not CI-job integration.
- Resolved by user: IPFIX and default geolocation database are working local baselines; prioritize the remaining unproven methods.
- No implementation-blocking user decision is open at SOW creation.

## Implications And Decisions

1. Decision: confidence level.
   - Selected: full verification using unit tests, CI-safe integration/contract tests, practical local validation, and docs correction.
   - Rationale: user explicitly rejected partial confidence and wants proof that shipped instructions work.

2. Decision: autonomy.
   - Selected: proceed without user involvement until an external dependency, credential, or product/risk decision blocks progress.
   - Rationale: user explicitly granted room to work autonomously.

3. Decision: corrected scope.
   - Selected: all enrichment paths plus NetFlow and sFlow validation gaps; IPFIX and default geolocation database are baselines rather than the primary gap.
   - Rationale: user corrected the scope after SOW creation and clarified that the immediate ask is testing missing unit/integration coverage, not CI-job setup.

4. Decision: downloader/provider coverage.
   - Selected: enhance the topology IP intelligence downloader so every supported free Geo/ASN provider path that can feed Netdata is exercised through generated MMDB output, update user documentation accordingly, test against actual provider data when licensing and public access permit it, then prove the generated output is usable by the netflow plugin.
   - Rationale: the netflow plugin reads MMDBs, while several provider sources start as CSV/TSV/tar/zip files. The missing proof is the conversion boundary from provider data into the MMDB files the plugin consumes.
   - Implication: this SOW now includes `src/go/tools/topology-ip-intel-downloader/`, its tests/docs/config examples, netflow integration metadata/examples that reference generated files, and runtime validation using generated MMDBs.
   - Risk: live provider downloads can be flaky, rate-limited, unavailable, or license-gated; CI-safe automated tests must remain fixture/local-data based, while actual-data validation is recorded as local evidence.

5. Decision: local live-service validation.
   - Selected: after downloader/provider proof, test local NetBox, generic IPAM, bio-rd, and GoBGP/FRR/BMP paths as far as possible without user involvement.
   - Rationale: these are installable or emulatable local systems on this workstation and are better proof than only mocked unit tests.
   - Implication: use temporary/local runtime services, store raw outputs under `.local/`, and only record sanitized evidence in durable artifacts.
   - Risk: full router-equivalent proof may still require a real router or production routing topology; local GoBGP/FRR/BMP proof is protocol-level, not proof against a vendor router.

6. Decision: beta external-routing validation bar.
   - Selected: prove Netdata's BMP and BioRIS contracts with deterministic payloads, local protocol speakers, and bio-rd-compatible gRPC services; do not block on production-router or production-bio-rd stability.
   - Rationale: the user does not have a router supporting these protocols, and the purpose is to catch Netdata-side issues before beta: connection/config failures, wrong fields, wrong payload schemas, ignored options, panics, hangs, leaked tasks, silent drops, and incorrect docs.
   - Implication: FRR/GoBGP plus deterministic BMP frames are sufficient for BMP. A deterministic bio-rd-compatible gRPC service is sufficient for BioRIS client validation. Upstream bio-rd remains useful as a smoke test, but upstream instability does not by itself block the beta.
   - Risk: this does not prove behavior against every vendor router or every production bio-rd deployment. That broader interoperability proof is expected from beta user feedback and later targeted fixes, while this SOW removes the obvious Netdata-side failures before shipping.

## Plan

1. Coverage matrix: map every enrichment method and NetFlow/sFlow from config/docs to runtime code and tests.
2. Test harness: add deterministic local fixtures and fake services.
3. Runtime proof: connect source-specific data to `FlowEnricher` and journal/query-visible fields.
4. Documentation proof: execute or mechanically validate examples; fix source docs/metadata and regenerate.
5. Local installed proof: build/install, restart `netdata`, inspect logs/journals/query output.
6. Closeout: record validation, sensitive-data gate, artifact gates, and blockers requiring user action.

## Execution Log

### 2026-05-08

- Created SOW after user approved autonomous full verification scope.
- Recorded private flow exporter as `[PRIVATE_FLOW_EXPORTER]` instead of writing the literal endpoint.
- Added CI-safe automated coverage for previously weak boundaries:
  - MaxMind/GeoLite2 ASN and City MMDB decoding with attributed public test databases.
  - Custom MMDB-style `netdata.ip_class` metadata from both ASN and GeoIP databases.
  - Static/runtime network-source merge behavior.
  - Documented AWS, Azure, GCP, generic JSON IPAM, and NetBox transform shapes.
  - HTTP network-source fetch, HTTP error handling, refresh loop publication, and cancellation.
  - NetFlow v9 and sFlow decode paths applying enrichment during decode.
  - Journal-backed ingest/query proof that enriched fields are persisted and query-visible.
  - BMP TCP listener accepting real BMP-framed Initiation and Termination messages.
  - BioRIS gRPC listener consuming `GetRouters`, `DumpRIB`, and `ObserveRIB` from a local fake service.
  - Stock `netflow.yaml` parse/validate coverage.
- Added attributed MaxMind test MMDB fixtures under `src/crates/netflow-plugin/testdata/mmdb/`.
- Found and fixed a production-code mismatch: `netdata.ip_class` was decoded from ASN MMDB records but not from GeoIP MMDB records, even though the MMDB downloader/docs can stamp the `netdata` metadata section into both outputs.
- Found and fixed a Function request compatibility issue: public flow-query examples use `after: -3600`, `before: 0`, and numeric `top_n`; the plugin only accepted unsigned absolute timestamps and string enum `top_n` values. The request parser now accepts documented relative time bounds and numeric/string `top_n`.
- Found and fixed a full-suite E2E reliability issue: five E2E tests passed individually but timed out in the default parallel full-suite run because the shared ingest wait was only 10 seconds. The shared wait is now 30 seconds.
- Removed an existing unused import warning in the startup memory test module.
- Corrected user-facing integration metadata and regenerated integration pages:
  - NetBox v2 tokens are documented as NetBox 4.5+; NetBox 4.0-4.4 use legacy tokens.
  - NetBox `?limit=0` is documented as dependent on server `MAX_PAGE_SIZE` policy.
  - BMP docs now state v3 is processed, v4 frames are decoded but ignored, and draft v1/v2 frames are decode errors.
- Fixed the public `query-netdata-agents` helper after using it during runtime validation:
  - Bash-only nameref use no longer breaks in the workstation's default `zsh`.
  - Internal `path` variables no longer clear zsh `PATH`.
  - Masked curl logging now redacts node id, machine GUID, claim id, space id, and room id, not only tokens.
  - Added/updated no-token-leak self-tests for Bash and zsh.
- Added `docs/netdata-ai/skills/query-netdata-cloud/how-tos/validate-local-netflow-function.md` because the Cloud/direct-agent validation workflow required multi-step analysis not present in the how-to catalog.
- Ran official/live schema checks for provider payloads:
  - AWS `ip-ranges.json`: verified `ip_prefix`, `ipv6_prefix`, `region`, `service`, `network_border_group`, `syncToken`, and `createDate`.
  - GCP `cloud.json` and `goog.json`: verified `syncToken`, `creationTime`, `ipv4Prefix`/`ipv6Prefix`, and optional `service`/`scope` behavior.
  - Azure Service Tags current public JSON: verified `changeNumber`, `cloud`, `values[].name`, and `properties.addressPrefixes`, `region`, `platform`, `systemService`, `networkFeatures`.
  - NetBox current docs: verified v2 tokens are NetBox 4.5+.
- Built and installed the plugin with `./build-install-netflow-plugin.sh`; `netdata` restarted and remained active.
- Found a local runtime configuration drift before the final install validation:
  - `/etc/netdata/netflow.yaml` used a stale journal schema and failed startup.
  - Backed it up to `/etc/netdata/netflow.yaml.sow-0015-backup-20260508034532`.
  - Installed the current stock config from `src/crates/netflow-plugin/configs/netflow.yaml`.
  - Added stock-config parse/validate coverage so this drift is caught by tests.
- Replayed pcap-derived UDP payloads into the installed collector on `127.0.0.1:2055`:
  - NetFlow v5 fixture.
  - NetFlow v9 template/options/data fixtures.
  - IPFIX template/data fixtures.
  - sFlow expanded-sample fixture.
- Verified installed runtime output through journal-backed evidence:
  - `journalctl --directory=/var/cache/netdata/flows/raw --since [REPLAY_START] -o export` reported rows for `FLOW_VERSION=ipfix`, `FLOW_VERSION=v5`, `FLOW_VERSION=v9`, and `FLOW_VERSION=sflow`.
  - The same row set contained ASN and country fields for source/destination enrichment.
- Verified Cloud-proxied Function output after install:
  - `agents_call_function --via cloud --function flows:netflow --body '{"info":true}'` returned `status: 200`, `type: flows`, and history metadata.
  - A documented `table-sankey` query body with `after: -3600`, `before: 0`, and numeric `top_n: 100` returned `status: 200`, flow rows, stats, and zero journal/template/parse errors.
- Reopened SOW after user decision to:
  - enhance the topology IP intelligence downloader for all supported Geo/ASN providers that can feed generated MMDBs;
  - update related docs;
  - test providers with actual data where public/licensed access permits;
  - validate NetBox, generic IPAM, bio-rd, and GoBGP/FRR/BMP locally.
- Enhanced `src/go/tools/topology-ip-intel-downloader/` provider support:
  - ASN: DB-IP MMDB/CSV, IPtoASN, CAIDA prefix2as, MaxMind GeoLite2 ASN.
  - GEO: DB-IP country/city MMDB/CSV, IPtoASN country, MaxMind GeoLite2 Country CSV, IP2Location Lite country, IPDeny country zones, IPIP country.
  - Added source-specific archive decoding for MaxMind tar.gz/zip, IP2Location zip, IPDeny tar.gz, IPIP zip, CAIDA latest-routeviews resolution, and redacted MaxMind query metadata.
  - Added parser tests for CAIDA, MaxMind Country CSV zip, IP2Location, IPDeny, IPIP, MaxMind env expansion/redaction, and all new provider tokens.
- Added netflow plugin contract tests for generated Netdata topology MMDBs:
  - committed tiny ASN/GEO MMDB fixtures generated by the downloader;
  - `netdata_topology_mmdb_enrichment_populates_asn_and_geo_fields`;
  - opt-in actual-data test `actual_provider_mmdb_outputs_are_readable_when_root_is_set`.
- Ran actual public provider downloads and generated MMDBs under `.local/audits/topology-ip-intel/providers`:
  - `dbip:asn-lite@mmdb`: 469,774 ASN ranges.
  - `dbip:asn-lite@csv`: 469,777 ASN ranges.
  - `iptoasn:combined` as ASN: 618,851 ASN ranges.
  - `caida:prefix2as`: 388,967 ASN ranges from `routeviews-rv2-20260506-1200.pfx2as.gz`.
  - `dbip:country-lite@mmdb`: 702,373 geo ranges.
  - `dbip:country-lite@csv`: 702,409 geo ranges.
  - `dbip:city-lite@mmdb`: 8,061,643 geo ranges.
  - `dbip:city-lite@csv`: 8,061,679 geo ranges.
  - `iptoasn:combined` as GEO: 286,373 geo ranges.
  - `ip2location:country-lite`: 273,642 geo ranges.
  - `ipdeny:country-zones`: 108,309 geo ranges.
  - `ipip:country`: 507,524 geo ranges.
- After the user pointed to a local update-ipsets environment source, live MaxMind downloads were also run without printing the key:
  - `maxmind:geolite2-asn`: 501,939 ASN ranges.
  - `maxmind:geolite2-country`: 599,247 geo ranges.
- Validated local live services:
  - NetBox: cloned `netbox-community/netbox-docker @ 55edb986b22c12b69ec89c79c8ccc7fdea88c5b5`, started NetBox v4.6 under Docker Compose, inserted a prefix, fetched `/api/ipam/prefixes/?limit=1000`, and applied the documented transform to produce `{prefix,name,tenant,role,site}`.
  - Generic JSON-over-HTTP IPAM: served a local JSON endpoint and validated the documented transform shape.
  - GoBGP/BMP: built `osrg/gobgp @ 5f191066a78e2c1e929c54b5b75fe2c683c166e4`; added an opt-in test that starts a Netdata BMP listener, starts `gobgpd`, adds a route with `gobgp`, and verifies the dynamic routing runtime receives ASN and AS-path.
  - FRR/BMP: pulled `frrouting/frr:latest`, verified documented BMP config with `bgpd -M bmp -C`, then ran a container that established a BMP connection and emitted BMP bytes to a local listener.
  - bio-rd: built `bio-routing/bio-rd @ 14a8de966e8b3f488a207aa0d02454a447ed5c99`; its RIS gRPC path is covered by Netdata's in-process test, but a practical GoBGP-to-bio-rd no-neighbor local-RIB setup crashed upstream bio-rd on a BMP Peer Up. This is recorded as an external blocker, not a Netdata client failure.
- Corrected user-facing docs:
  - downloader README and sample config now list all supported provider tokens/formats and MaxMind license-key behavior;
  - MaxMind integration docs now describe the topology downloader path for GeoLite2 ASN/Country;
  - NetBox docs now show the NetBox 4.5+ v2 bearer header as `Bearer nbt_<12-char-key>.<40-char-token>` and explain that sending a v2 token with the legacy `Token` prefix produces an invalid-v1-token error.

## Validation

Acceptance criteria evidence:

- Scoped enrichment coverage:
  - Custom MMDB / MaxMind / GeoLite2: `src/crates/netflow-plugin/src/enrichment/tests.rs` covers public ASN and City MMDB fixtures, ASN org, country/city/state/lat/lon, and `netdata.ip_class`.
  - DB-IP / IPtoASN baseline: existing default geolocation runtime was treated as user-confirmed baseline; new MMDB tests exercise the same resolver path with deterministic fixtures.
  - AWS / Azure / GCP / generic IPAM / NetBox: `src/crates/netflow-plugin/src/network_sources/tests.rs` covers documented transform payloads and the HTTP fetch/publish path.
  - Static metadata, sampling overrides, static networks, classifiers, ASN/network provider chains, and decapsulation: existing tests remain in `src/crates/netflow-plugin/src/enrichment/tests.rs` and decoder fixture tests; new runtime merge and decode/enrichment tests cover the boundary.
  - BioRIS: `src/crates/netflow-plugin/src/routing/bioris/tests.rs` covers a real in-process gRPC service and runtime route lookup after `DumpRIB`.
  - BMP: `src/crates/netflow-plugin/src/routing/bmp/tests.rs` covers a real TCP listener and BMP-framed messages.
  - Downloader provider conversion: `src/go/tools/topology-ip-intel-downloader/*_test.go` covers every supported built-in provider token/format and the provider-specific parser/decode paths.
  - Generated Netdata topology MMDB consumption: `src/crates/netflow-plugin/src/enrichment/tests.rs` reads downloader-generated MMDB fixtures and proves the Rust enricher sees ASN, ASN name, country, city, state, and coordinates.
  - Actual downloaded provider outputs: opt-in test `actual_provider_mmdb_outputs_are_readable_when_root_is_set` was run against `.local/audits/topology-ip-intel/providers` and read every public provider output with the Rust resolver, including live MaxMind GeoLite2 ASN and Country outputs.
  - Real GoBGP BMP speaker: opt-in test `bmp_listener_accepts_gobgp_route_when_binaries_are_set` was run against locally built `gobgpd`/`gobgp` and proved a route added through GoBGP reaches the Netdata dynamic-routing runtime.
  - NetFlow/sFlow: `src/crates/netflow-plugin/src/decoder/tests.rs` proves NetFlow v9 and sFlow records receive enrichment during decode; installed runtime replay produced v5/v9/sFlow journal rows.
  - Journal/query visibility: `src/crates/netflow-plugin/src/main_tests.rs` proves enriched fields are written to the raw journal and are query-visible by `SRC_NET_NAME`.

Tests or equivalent validation:

- `cargo fmt -p netflow-plugin` passed.
- Targeted tests passed:
  - `cargo test -p netflow-plugin apply_geo_record_accepts_netdata_ip_class_metadata -- --nocapture`
  - `cargo test -p netflow-plugin maxmind_geolite2_mmdb_enrichment_populates_asn_and_geo_fields -- --nocapture`
  - `cargo test -p netflow-plugin request_deserialization_accepts -- --nocapture`
  - `cargo test -p netflow-plugin resolve_time_bounds_treats_negative_after_as_relative_to_before -- --nocapture`
- Full Rust plugin suite passed after fixes:
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml`
  - Result: 447 passed, 18 ignored, 0 failed.
  - `tests/grpc_build.rs`: 1 passed, 0 failed.
- Downloader tests passed:
  - `go test ./tools/topology-ip-intel-downloader` from `src/go`.
- Actual provider output tests passed:
  - `NETDATA_TOPOLOGY_IP_INTEL_PROVIDER_ROOT="$(pwd)/.local/audits/topology-ip-intel/providers" cargo test -p netflow-plugin actual_provider_mmdb_outputs_are_readable_when_root_is_set --manifest-path src/crates/Cargo.toml`
  - `cargo test -p netflow-plugin netdata_topology_mmdb_enrichment_populates_asn_and_geo_fields --manifest-path src/crates/Cargo.toml`
- Real GoBGP BMP test passed:
  - `NETDATA_GOBGPD="$(pwd)/.local/audits/netflow-live/bin/gobgpd" NETDATA_GOBGP="$(pwd)/.local/audits/netflow-live/bin/gobgp" cargo test -p netflow-plugin bmp_listener_accepts_gobgp_route_when_binaries_are_set --manifest-path src/crates/Cargo.toml`
- Network-source and routing targeted tests passed:
  - `cargo test -p netflow-plugin network_sources --manifest-path src/crates/Cargo.toml`
  - `cargo test -p netflow-plugin routing::bioris --manifest-path src/crates/Cargo.toml`
  - `cargo test -p netflow-plugin routing::bmp --manifest-path src/crates/Cargo.toml`
- Integration metadata/docs validation passed:
  - `python3 integrations/gen_integrations.py`
  - `python3 integrations/gen_docs_integrations.py`
  - `python3` YAML parse for `src/crates/netflow-plugin/metadata.yaml`
  - `git diff --check`
- SOW audit was run after moving this file to `done/`. Status/directory checks passed; the command exited nonzero only on a pre-existing false positive in an unmodified mirror-skill Git SSH URL that the audit pattern classifies as an email-like string.
- Public helper validation passed:
  - `bash -c 'source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh; agents_selftest_no_token_leak'`
  - `zsh -c 'source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh; agents_selftest_no_token_leak'`
  - `shellcheck --external-sources docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh`

Real-use evidence:

- `./build-install-netflow-plugin.sh` completed:
  - Release build finished.
  - Plugin installed to `/usr/libexec/netdata/plugins.d/netflow-plugin`.
  - `systemctl is-active netdata` returned `active`.
- Installed runtime:
  - `journalctl --namespace=netdata SYSLOG_IDENTIFIER=netflow-plugin --since '2 minutes ago'` showed startup, Function declaration, journal scanning, and no startup failure.
  - `ss -lunp` showed `netflow-plugin` listening on UDP `0.0.0.0:2055`.
- Runtime pcap replay:
  - Eight UDP payloads were sent to `127.0.0.1:2055` from deterministic local pcap fixtures.
  - Raw journal inspection since replay start reported:
    - rows: 513
    - `FLOW_VERSION[ipfix]`: 479
    - `FLOW_VERSION[v5]`: 29
    - `FLOW_VERSION[v9]`: 4
    - `FLOW_VERSION[sflow]`: 1
    - source ASN fields: 369
    - destination ASN fields: 502
    - source country fields: 370
    - destination country fields: 503
- Cloud-proxied Function validation:
  - Local `/api/v3/info` showed one local agent, Cloud status online, and node/machine/claim identifiers present; raw identifiers were stored only under `.local/`.
  - Cloud discovery found the local node reachable in visible rooms by exact local node/machine identifiers.
  - `flows:netflow` info call returned `status: 200` and `type: flows`.
  - A documented flow query returned `status: 200`, 101 flow rows, expected group-by fields, and stats including decoded NetFlow v5, NetFlow v9, IPFIX, and sFlow counters with zero parse/template/journal-write errors.
- Direct-agent bearer validation:
  - A Cloud bearer mint using the local `/api/v3/info` node id, machine GUID, and claim id tuple returned HTTP 200 with a token present.
  - `agents_call_function --via agent --function flows:netflow --body '{"info":true}'` against `127.0.0.1:19999` returned `status: 200`, `type: flows`, and `has_history: true`.
- Actual provider downloader validation:
  - Every live provider listed in the execution log downloaded, parsed, and wrote a non-empty MMDB output.
  - The Rust resolver read all generated outputs successfully in the opt-in actual-data test, including live MaxMind GeoLite2 ASN and Country outputs.
- NetBox live validation:
  - Local NetBox v4.6 reached healthy status under Docker Compose.
  - `/api/ipam/prefixes/?limit=1000` returned the inserted `198.51.100.0/24` prefix.
  - The documented NetBox transform produced `{"prefix":"198.51.100.0/24","tenant":"","role":"","site":"","name":"netdata edge subnet"}`.
  - NetBox v4.6 rejected a v2 token sent as legacy `Token` with "Invalid v1 token"; docs were corrected to show `Bearer nbt_<12-char-key>.<40-char-token>`.
- Generic IPAM live validation:
  - A local HTTP JSON endpoint returned a prefix list and the documented transform produced `{"prefix":"203.0.113.0/24","name":"corp edge","tenant":"prod"}`.
- BioRIS / BMP live validation:
  - GoBGP -> Netdata BMP listener route publication passed with real GoBGP binaries.
  - FRR `bgpd -M bmp -C` accepted the documented BMP config, and a running `frrouting/frr` container established a BMP connection and emitted bytes to a local listener.
  - GoBGP -> upstream bio-rd RIS crashed inside bio-rd on BMP Peer Up in the no-neighbor local-RIB setup; Netdata's BioRIS client path remains covered by the in-process gRPC service test.

Reviewer findings:

- No external AI reviewer was requested by the user.
- Self-review findings handled during execution:
  - GeoIP MMDB did not apply `netdata.ip_class`; fixed and tested.
  - Public flow-query request shape did not accept documented negative `after` and numeric `top_n`; fixed and tested.
  - Public direct-agent helper failed under zsh and leaked identifiers in masked logs; fixed and tested.
  - E2E full-suite ingest wait was too short under parallel load; fixed and full suite passed.

Same-failure scan:

- Searched for provider-transform examples and `top_n` examples in public flow-query docs/skills.
- The public examples using numeric `top_n` now work because the plugin accepts both numeric and string values.
- The public examples using negative relative `after` now work because request parsing and time-bound resolution support it.
- Searched generated integration side effects after docs generation; reverted unrelated SNMP generated-page churn and verified no SNMP diff remained.

Sensitive data gate:

- Raw `/api/v3/info`, Cloud discovery payloads, bearer endpoint responses, and flow Function responses are stored only under `.local/audits/query-netdata-agents/`, which is gitignored.
- This SOW records sanitized summaries only:
  - private flow exporter is `[PRIVATE_FLOW_EXPORTER]`;
  - local hostname is `[LOCAL_HOST]`;
  - no node ids, machine GUIDs, claim ids, tokens, bearer values, raw private endpoints, or raw flow rows are recorded here.
- Public helper logging now masks Cloud tokens, agent bearers, node ids, machine GUIDs, claim ids, space ids, and room ids in stderr.

Artifact maintenance gate:

- AGENTS.md: no update needed; workflow rules were sufficient.
- Runtime project skills: no project-local collector/integration skill update needed; existing `project-writing-collectors` and `integrations-lifecycle` rules were sufficient.
- Specs: no separate spec update needed; this SOW did not change a durable product contract beyond making implementation match already published Function-query examples and MMDB metadata behavior.
- End-user/operator docs: updated `src/crates/netflow-plugin/metadata.yaml` and regenerated affected integration pages.
- End-user/operator skills: updated `docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh`; added Cloud and direct-agent how-tos for local flow Function validation.
- SOW lifecycle: SOW is marked `completed` and moved to `.agents/sow/done/` with the implementation and validation commit. Remaining items require user-supplied credentials or real external systems and are recorded below.

Specs update:

- No spec file was updated. Reason: the durable behavior changes are already captured in code/tests and public operator docs/skills:
  - `after: -N`, `before: 0`, and numeric `top_n` are public Function request behavior now covered by tests.
  - MMDB `netdata.ip_class` handling now matches existing downloader/documentation intent.

Project skills update:

- No runtime project skill update needed.
- Public operator skill helper was updated because its direct-agent transport wrapper failed under zsh and did not mask all identity fields.

End-user/operator docs update:

- Updated:
  - `src/crates/netflow-plugin/metadata.yaml`
  - `src/crates/netflow-plugin/integrations/netbox.md`
  - `src/crates/netflow-plugin/integrations/maxmind_geoip_-_geolite2.md`
  - `src/crates/netflow-plugin/integrations/bmp_bgp_monitoring_protocol.md`
  - `src/go/tools/topology-ip-intel-downloader/README.md`
  - `src/go/tools/topology-ip-intel-downloader/configs/topology-ip-intel.yaml`
  - `docs/netdata-ai/skills/query-netdata-cloud/how-tos/validate-local-netflow-function.md`
  - `docs/netdata-ai/skills/query-netdata-cloud/how-tos/INDEX.md`
  - `docs/netdata-ai/skills/query-netdata-agents/how-tos/validate-direct-local-flow-function.md`
  - `docs/netdata-ai/skills/query-netdata-agents/how-tos/INDEX.md`

End-user/operator skills update:

- Updated `docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh`.
- Added `docs/netdata-ai/skills/query-netdata-agents/how-tos/validate-direct-local-flow-function.md`.
- Validation: Bash self-test, zsh self-test, and shellcheck all passed.

Lessons:

- Testing the public instructions against the installed agent exposed real API-compatibility bugs that pure unit tests had missed.
- Direct-agent bearer validation works when the wrapper uses the exact local `/api/v3/info` node id, machine GUID, and claim id tuple. Mixed node/machine/claim tuples should be treated as identity-resolution bugs before treating direct bearer as blocked.
- Masked command logging must redact identifiers as well as tokens, because node ids, machine GUIDs, claim ids, space ids, and room ids are sensitive enough to keep out of durable artifacts.

Follow-up mapping:

- Requires user involvement if further proof is required:
  - Real BMP from a production router: user does not run BGP/BMP locally. Current proof covers deterministic BMP frames, real GoBGP-to-Netdata BMP route publication, and FRR BMP config/connection. A real router session still requires a router or lab BGP topology.
  - Real bio-rd / RIPE RIS with live routes: Netdata's BioRIS client path is covered by an in-process gRPC service, but the local GoBGP-to-upstream-bio-rd setup crashed inside bio-rd on BMP Peer Up. Further proof needs either a stable bio-rd/RIS endpoint or time to debug/report/fix upstream bio-rd.
  - User-owned NetBox / custom IPAM endpoints: local NetBox and generic IPAM were validated; proof against the user's own systems requires their endpoint and token if desired.
  - Custom operator-provided MMDB: current proof uses public MaxMind test MMDB fixtures and a synthetic `netdata.ip_class` record; no user custom MMDB was supplied.
- Explicitly out of this SOW:
  - CI job wiring. The user corrected that the immediate scope is unit/integration testing and practical validation, not CI-job integration.

## Outcome

Autonomous work completed as far as possible without external systems or user credentials. Automated coverage, downloader provider support, actual public-provider data validation, local NetBox/IPAM validation, GoBGP/FRR BMP validation, docs/source corrections, installed runtime validation, journal proof, Cloud-proxied Function validation, and direct-agent bearer Function validation are complete.

## Lessons Extracted

- Run public examples against the installed plugin, not only through Rust unit tests. This caught `after`/`top_n` request-shape bugs.
- Include shell compatibility in public helper tests when examples are likely to be run from users' default shells.
- Use the exact local `/api/v3/info` node id, machine GUID, and claim id tuple for direct-agent bearer minting; mixed identity tuples are the common failure mode.
- Treat identity fields as sensitive in wrapper logging; masking only bearer/token values is insufficient.

## Followup

User-required items are listed in Follow-up mapping above. No autonomous follow-up remains unattempted in this SOW.

## PR Review Iteration - 2026-05-08

Trigger:

- User asked to solve PR comments/reviews on PR 22453 before continuing.

Finding sources fetched:

- `bash .agents/skills/pr-reviews/scripts/fetch-all.sh 22453`
  - 8 open review threads.
  - All open review threads are from `cubic-dev-ai[bot]`.
  - No human review comments need a user decision before implementation.
- `bash .agents/skills/pr-reviews/scripts/fetch-sonar-findings.sh 22453`
  - 0 SonarCloud issues.
  - 0 SonarCloud hotspots.
- `bash .agents/skills/pr-reviews/scripts/ci-status.sh 22453`
  - 0 failing checks.
  - Most checks are still pending after the latest push.

Open review threads to verify:

- `src/crates/netflow-plugin/src/routing/bioris/tests.rs:242`: local gRPC test reserves and releases an ephemeral port before server bind.
- `src/crates/netflow-plugin/src/routing/bmp/tests.rs:274`: BMP listener test reserves and releases an ephemeral port before listener bind.
- `src/crates/netflow-plugin/src/query/planner/request.rs:20`: timestamp clamp may collapse `after` and `before`.
- `docs/netdata-ai/skills/query-netdata-agents/how-tos/validate-direct-local-flow-function.md:25`: guide persists raw `/api/v3/info` payload containing sensitive identifiers.
- `src/crates/netflow-plugin/src/network_sources/tests.rs:171`: NetBox transform test maps `site` but does not assert it.
- `src/go/tools/topology-ip-intel-downloader/config.go:280`: IPDeny built-in source uses HTTP.
- `src/crates/netflow-plugin/src/routing/bmp/tests.rs:431`: async test uses blocking `std::process::Command::status()`.
- `src/go/tools/topology-ip-intel-downloader/parse.go:775`: IPDeny `.ZONE` extension handling is case-sensitive after case-insensitive filter.

Plan:

- Verify each bot finding against current code.
- Fix valid findings and search touched PR files for the same failure class.
- Run the narrow tests for modified areas plus formatting and diff checks.
- Re-fetch findings before push.
- Push one commit for the review fixes, reply per thread, resolve threads, and retrigger AI reviewers.

Verified fixes:

- BioRIS gRPC test now binds the Tokio listener first and serves tonic from that bound listener via `TcpListenerStream`, removing the reserve/release race.
- BMP listener tests now pass a pre-bound Tokio listener into a shared listener helper, removing the reserve/release race for the Netdata listener side.
- The remaining GoBGP API port is for an external daemon that requires a concrete address; the test records that limitation and keeps the path opt-in.
- Query time-bound resolution now clamps first and enforces `after < before` after clamping.
- Direct-agent and Cloud flow-validation how-tos no longer persist raw `/api/v3/info`; they keep the raw identity payload in shell memory and print only presence checks.
- NetBox transform tests now assert the documented `site` mapping.
- IPDeny built-in URL now uses HTTPS; live HEAD check returned HTTP 200 for the HTTPS URL.
- GoBGP helper commands in the async BMP test now use `tokio::process::Command`.
- IPDeny `.zone` parsing now handles uppercase `.ZONE` suffixes and has order-independent test coverage.

Validation after fixes:

- `go test ./tools/topology-ip-intel-downloader`
- `cargo test -p netflow-plugin documented_cloud_and_ipam_transforms_decode_provider_payloads --manifest-path src/crates/Cargo.toml`
- `cargo test -p netflow-plugin resolve_time_bounds --manifest-path src/crates/Cargo.toml`
- `cargo test -p netflow-plugin routing::bioris --manifest-path src/crates/Cargo.toml`
- `cargo test -p netflow-plugin routing::bmp --manifest-path src/crates/Cargo.toml`
- `NETDATA_GOBGPD="$(pwd)/.local/audits/netflow-live/bin/gobgpd" NETDATA_GOBGP="$(pwd)/.local/audits/netflow-live/bin/gobgp" cargo test -p netflow-plugin bmp_listener_accepts_gobgp_route_when_binaries_are_set --manifest-path src/crates/Cargo.toml`
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml`
  - Result: 448 passed, 18 ignored, 0 failed.
  - `tests/grpc_build.rs`: 1 passed, 0 failed.
- How-to jq snippets were smoke-tested with a synthetic `/api/v3/info` payload without writing raw identity JSON.
- `git diff --check`

Final pre-push sync:

- `bash .agents/skills/pr-reviews/scripts/fetch-all.sh 22453`
  - 8 open review threads, unchanged from the initial set.
  - No new comments.
- `bash .agents/skills/pr-reviews/scripts/fetch-sonar-findings.sh 22453`
  - 0 issues.
  - 0 hotspots.
- `bash .agents/skills/pr-reviews/scripts/ci-status.sh 22453`
  - 0 failing checks.
  - 90 checks still running on the previous commit.

Artifact maintenance for review pass:

- AGENTS.md: no update needed; existing PR-review and sensitive-data rules were sufficient.
- Runtime project skills: no update needed.
- Specs: no update needed; fixes align implementation/tests/docs to existing intended behavior.
- End-user/operator docs: updated direct-agent and Cloud validation how-tos to avoid durable raw identity payloads.
- End-user/operator skills: no script behavior changed in this review pass.
- SOW lifecycle: SOW returned to `completed` and is moved back to `.agents/sow/done/` with the review-fix commit.

## Regression Log

### Regression - 2026-05-08 - Missing Provider Integration Modules

What broke:

- The previous completion enhanced and validated the topology IP intelligence downloader for CAIDA, IP2Location, IPDeny, and IPIP, but the user-facing NetFlow enrichment metadata still exposed only DB-IP, MaxMind, IPtoASN, and Custom MMDB as IP intelligence integrations.
- This made the generated integrations catalog incomplete: users could discover downloader tokens in the downloader README, but not from the NetFlow enrichment-method module list.

Evidence:

- `src/crates/netflow-plugin/metadata.yaml` had modules for `dbip`, `maxmind`, `iptoasn`, and `custom-mmdb`.
- Generated pages under `src/crates/netflow-plugin/integrations/` existed only for `db-ip_ip_intelligence.md`, `maxmind_geoip_-_geolite2.md`, `iptoasn.md`, and `custom_mmdb_database.md`.
- Downloader code supports `caida:prefix2as`, `ip2location:country-lite`, `ipdeny:country-zones`, and `ipip:country` in `src/go/tools/topology-ip-intel-downloader/config.go`.

Why previous validation missed it:

- Validation proved downloader parsing, live provider downloads, generated MMDB output, and Rust resolver consumption, but did not compare the complete downloader provider matrix against generated integration module coverage.

Repair plan:

- Add first-class `metadata.yaml` modules for CAIDA Routeviews Prefix-to-AS, IP2Location LITE IP-Country, IPDeny Country Zones, and IPIP Country.
- Cross-link these modules with the existing DB-IP, MaxMind, IPtoASN, and Custom MMDB modules.
- Regenerate generated integration pages from `metadata.yaml`.
- Validate metadata generation and downloader tests.

Sensitive data handling:

- No secrets, private endpoints, node identifiers, or raw provider payloads are needed in durable artifacts. Provider descriptions use public source names, public URLs, and downloader tokens only.

Implementation:

- Added first-class NetFlow enrichment metadata modules for:
  - `caida-prefix2as` - CAIDA Routeviews Prefix-to-AS.
  - `ip2location` - IP2Location LITE IP-Country.
  - `ipdeny` - IPDeny Country Zones.
  - `ipip` - IPIP Country Database.
- Updated related-resource links from DB-IP, MaxMind, IPtoASN, and Custom MMDB so the generated integration catalog surfaces the full supported IP intelligence provider set.
- Regenerated the new NetFlow integration pages from `src/crates/netflow-plugin/metadata.yaml`.

Validation:

- `go test ./tools/topology-ip-intel-downloader` passed from `src/go`.
- `python3 integrations/gen_integrations.py` passed.
- `python3 integrations/gen_docs_integrations.py` generated the new NetFlow provider pages. It also reproduced a pre-existing unrelated SNMP generated-page diff; that unrelated SNMP churn was removed from this changeset.
- A metadata coverage check verified all expected IP intelligence modules are present: `dbip`, `maxmind`, `iptoasn`, `custom-mmdb`, `caida-prefix2as`, `ip2location`, `ipdeny`, and `ipip`.
- Generated page existence check verified:
  - `src/crates/netflow-plugin/integrations/caida_routeviews_prefix-to-as.md`
  - `src/crates/netflow-plugin/integrations/ip2location_lite_ip-country.md`
  - `src/crates/netflow-plugin/integrations/ipdeny_country_zones.md`
  - `src/crates/netflow-plugin/integrations/ipip_country_database.md`
- `git diff --check` passed after removing unrelated generated SNMP churn.

Artifact maintenance:

- AGENTS.md: no update needed; existing integration-generation and SOW regression rules were sufficient.
- Runtime project skills: no update needed; `integrations-lifecycle` already documents the source/generator contract.
- Specs: no update needed; this is a documentation/catalog completeness repair for already-implemented provider behavior.
- End-user/operator docs: updated `src/crates/netflow-plugin/metadata.yaml` and generated four new provider integration pages.
- End-user/operator skills: no update needed; no public skill workflow changed.
- SOW lifecycle: regression recorded here, then SOW returned to `completed` and moved back to `.agents/sow/done/` with the metadata/doc commit.

### PR Review Follow-up - 2026-05-08 - Copilot Findings

Trigger:

- Copilot opened PR review threads on:
  - `docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh:171`
  - `src/go/tools/topology-ip-intel-downloader/fetch.go:295`

Findings:

- `_agents_set_outvar` used `eval "${name}=\${value}"` to write external command output into a caller-provided variable.
- The variable name was validated, and the previous assignment form avoided common word-splitting behavior, but the pattern was still fragile and hard to audit because token and claim values originate from `curl` / `jq` output.
- `decodeMaxMindASNPayload` accepted every `extractMMDBFromTar` error as "not a tarred MMDB" and returned the ungzipped content. That preserved plain gzipped MMDB support, but it also hid corrupt tar-like payloads behind a later, less-specific MMDB parse error.

Implementation:

- Replaced the eval assignment with `printf -v "${_agents_out_name}" '%s' "${_agents_out_value}"`.
- Kept variable-name validation before assignment.
- Added a self-test payload containing whitespace, glob characters, shell metacharacters, command-substitution text, backticks, quotes, and a newline, verifying the helper preserves it as data and does not interpret it.
- Updated the public `query-netdata-agents` skill text from "bash nameref" to "validated caller-local output variable", matching the zsh-compatible implementation.
- Updated MaxMind ASN payload decoding so plain gzipped MMDB payloads still pass through, while tar-like payloads with invalid tar structure now return a clear extraction error.
- Added downloader test coverage for corrupt tar-like MaxMind ASN payloads.

Validation:

- `bash -c 'source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh; agents_selftest_no_token_leak'` passed.
- `zsh -c 'source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh; agents_selftest_no_token_leak'` passed.
- `shellcheck --external-sources docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh` passed.
- `git diff --check` passed.
- `bash .agents/sow/audit.sh` verified SOW status/directory consistency. It exited nonzero on the existing unmodified `.agents/skills/mirror-netdata-repos/SKILL.md:112` Git SSH URL pattern that the audit classifies as email-like sensitive data.
- `git diff --check` passed.
- `bash .agents/sow/audit.sh` verified SOW status/directory consistency. It exited nonzero on the existing unmodified `.agents/skills/mirror-netdata-repos/SKILL.md:112` Git SSH URL pattern that the audit classifies as email-like sensitive data.
- `go test ./tools/topology-ip-intel-downloader` passed from `src/go`.
- Same-pattern search found no remaining eval-based `_agents_set_outvar` assignment in the public skill path.

Artifact maintenance:

- AGENTS.md: no update needed; existing sensitive-data and public-skill rules were sufficient.
- Runtime project skills: no update needed.
- Specs: no update needed; this repaired implementation safety/error handling for existing helper/provider contracts.
- End-user/operator docs: updated `docs/netdata-ai/skills/query-netdata-agents/SKILL.md` to describe the actual output-variable contract.
- End-user/operator skills: updated `docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh`.
- SOW lifecycle: reopened from `done`, recorded this PR review follow-up, then returned to `completed` and moved back to `.agents/sow/done/` with the fix commit.

### External App Validation Follow-up - 2026-05-08

Trigger:

- User asked to proceed with the remaining external application validation after PR review cleanup.

Scope exercised:

- Generic JSON-over-HTTP IPAM.
- NetBox.
- FRR BMP.
- GoBGP BMP.
- bio-rd RIS / BioRIS.

Evidence and results:

- Generic JSON-over-HTTP IPAM:
  - Started a temporary localhost HTTP server serving sanitized RFC documentation prefixes.
  - Fetched `networks.json` over HTTP and applied the documented transform `.[] | {prefix: .cidr, name: .label, tenant: .tenant}`.
  - Result: `{"prefix":"203.0.113.0/24","name":"corp edge","tenant":"prod"}`.
  - Temporary HTTP process was verified stopped.
- NetBox:
  - Started local `netbox-docker` under a dedicated Compose project.
  - Initial health check exceeded the Docker health timeout while migrations were still finishing, but the NetBox login endpoint returned HTTP 200 and the API was usable after startup.
  - Bootstrapped a local prefix and queried `/api/ipam/prefixes/?limit=1000` with a generated local token.
  - Applied the documented transform and verified the expected result:
    `{"prefix":"198.51.100.0/24","tenant":"","role":"","site":"","name":"netdata edge subnet"}`.
  - Compose project was shut down with volumes removed.
- FRR BMP:
  - Used `quay.io/frrouting/frr:10.6.0`.
  - `bgpd -M bmp -C -f /etc/frr/bgpd.conf` accepted the BMP config. FRR logged warnings about missing synthetic OPEN messages for the static announcement, but did not reject the config.
  - A running FRR container connected to a local BMP listener and emitted 297 bytes. The captured stream began with BMP v3 and an FRRouting initiation payload.
  - Temporary listener process and container were verified stopped.
- GoBGP BMP:
  - `NETDATA_GOBGPD=.local/audits/netflow-live/bin/gobgpd NETDATA_GOBGP=.local/audits/netflow-live/bin/gobgp cargo test -p netflow-plugin routing::bmp --manifest-path src/crates/Cargo.toml`
  - Result: 13 passed, 0 failed, including `bmp_listener_accepts_gobgp_route_when_binaries_are_set`.
- BioRIS / bio-rd:
  - Upstream source evidence shows `bio-rd` `cmd/ris` is BMP-backed:
    - `bio-routing/bio-rd @ 14a8de966e8b3f488a207aa0d02454a447ed5c99`, `cmd/ris/main.go:52-83` creates a BMP receiver, listens for BMP, adds configured BMP servers, then exposes the RIS gRPC server.
    - `bio-routing/bio-rd @ 14a8de966e8b3f488a207aa0d02454a447ed5c99`, `cmd/ris/config/config.go:10-20` defines `bmp_servers`.
  - Official RIPE RIS documentation checked:
    - `https://ris-live.ripe.net/` describes RIS Live as WebSocket JSON.
    - `https://ris.ripe.net/docs/` lists route collectors, raw MRT files, RIS Live, RISwhois, and routing beacons.
  - No official public RIPE RIS endpoint implementing `bio.ris.RoutingInformationService` gRPC was found.
  - GoBGP-to-upstream-bio-rd reached BMP Peer Up, then upstream bio-rd crashed in its BGP/BMP handling. This is an upstream/runtime issue, not a Netdata BioRIS client failure.
  - FRR-to-upstream-bio-rd did not crash; `riscli routers` saw a router from `[LOCAL_HOST]`, but the local loopback lab did not produce a stable non-empty RIB. The logs showed peer up/down behavior and duplicate-neighbor handling.
  - Netdata's BioRIS client path remains covered by the in-process gRPC test that exercises `GetRouters`, `DumpRIB`, and `ObserveRIB`.

Documentation correction:

- Corrected `src/crates/netflow-plugin/metadata.yaml` so the BioRIS module says Netdata consumes only a bio-rd-compatible `RoutingInformationService` gRPC endpoint.
- Corrected the generated `src/crates/netflow-plugin/integrations/bio-rd_-_ripe_ris.md` from metadata.
- Corrected `docs/network-flows/enrichment.md` and `docs/network-flows/configuration.md` link text / routing-overlay language.
- Removed misleading instructions that implied users can point `grpc_addr` at RIPE RIS Live, RIPEstat, MRT dumps, route collector sessions, or looking-glass sources directly.

Validation after documentation correction:

- `python3 integrations/gen_integrations.py` passed.
- `python3 integrations/gen_docs_integrations.py` passed.
- `python3 -c` YAML parse of `src/crates/netflow-plugin/metadata.yaml` passed.
- `cargo test -p netflow-plugin network_sources --manifest-path src/crates/Cargo.toml`
  - Result: 17 passed, 0 failed.
- `cargo test -p netflow-plugin routing::bioris --manifest-path src/crates/Cargo.toml`
  - Result: 7 passed, 0 failed.
- `NETDATA_GOBGPD="$(pwd)/.local/audits/netflow-live/bin/gobgpd" NETDATA_GOBGP="$(pwd)/.local/audits/netflow-live/bin/gobgp" cargo test -p netflow-plugin routing::bmp --manifest-path src/crates/Cargo.toml`
  - Result: 13 passed, 0 failed.
- `git diff --check` passed.
- Generated-doc noise from unrelated SNMP and trailing blank-line changes was removed from the changeset.
- A process/container cleanup check found no remaining temporary live-validation processes or containers.

Artifact maintenance:

- AGENTS.md: no update needed; existing SOW, sensitive-data, and generated-doc rules were sufficient.
- Runtime project skills: no update needed; `project-writing-collectors`, `integrations-lifecycle`, and `mirrored-repos` already cover the workflow.
- Specs: no update needed; this corrected public/operator documentation to match existing implementation and upstream protocol reality.
- End-user/operator docs: updated BioRIS metadata, generated integration docs, and hand-authored network-flow docs.
- End-user/operator skills: no update needed; no public skill workflow changed.
- SOW lifecycle: reopened from `done`, recorded external validation, then returned to `completed` and moved back to `.agents/sow/done/` with the validation/doc commit.

Additional proof outside the clarified beta validation bar:

- A real stable bio-rd-compatible `RoutingInformationService` endpoint with a non-empty RIB would expand proof beyond Netdata's client contract into production-style BioRIS interoperability.
- A real router/BMP source would expand proof beyond deterministic BMP frames, GoBGP route publication, and FRR config/connection validation into vendor-router interoperability.
- Debugging, reporting, or fixing upstream bio-rd behavior would be useful only if this beta work is expanded into upstream bio-rd stabilization.
- User-owned NetBox or custom IPAM endpoints would expand proof beyond local NetBox and local generic IPAM validation into user-specific environment validation.

These are not blockers under the clarified beta validation target. The final autonomous work in this pass strengthened Netdata-side automated coverage for configuration, payload/schema consumption, field mapping, option handling, and stability.

### Netdata-side Contract Hardening - 2026-05-08

Trigger:

- The user clarified that this beta validation does not need to prove every external routing stack is production-stable.
- The required bar is to prove Netdata's side is sane: valid configs connect, bad configs are rejected or reported clearly, expected payloads are consumed, fields map correctly into enrichment state, options are respected, no obvious panic/hang/task leak/silent ignore exists, and user docs describe working commands/configs.

Implementation:

- BioRIS:
  - Added endpoint URI contract coverage for explicit schemes and `grpc_secure`.
  - Added invalid-endpoint coverage proving malformed gRPC URIs produce a clear `invalid BioRIS endpoint URI` error before network dialing.
  - Added router IP parsing coverage for plain IPs and socket-address forms.
  - Extended the in-process `RoutingInformationService` fixture so tests record `DumpRIB` and `ObserveRIB` requests.
  - Added coverage proving configured `vrf_id` and `vrf` are sent to `DumpRIB` and `ObserveRIB`.
  - Added coverage proving `DumpRIB` and `ObserveRIB` advertisements publish AS path, communities, and large communities into the runtime trie.
  - Added coverage proving `ObserveRIB` withdrawals remove the more-specific route and fall back to the broader route.
- BMP:
  - Added deterministic `apply_update` tests proving `collect_asns`, `collect_as_paths`, and `collect_communities` are respected.
  - Added deterministic update/withdrawal tests proving AS number, AS path, communities, large communities, next hop, and route deletion map correctly into the runtime trie.
- Config validation:
  - Added coverage rejecting enabled BioRIS instances with `grpc_addr` that has neither `host:port` nor an explicit URI scheme.
  - Added coverage rejecting zero `timeout`, `refresh`, and `refresh_timeout` values for enabled BioRIS.

Validation:

- `cargo test -p netflow-plugin routing::bioris --manifest-path src/crates/Cargo.toml`
  - Result: 11 passed, 0 failed.
- `cargo test -p netflow-plugin routing::bmp --manifest-path src/crates/Cargo.toml`
  - Result: 15 passed, 0 failed, including the opt-in GoBGP route publication test because local GoBGP binaries are present.
- `cargo test -p netflow-plugin plugin_config --manifest-path src/crates/Cargo.toml`
  - Result: 30 passed, 0 failed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml`
  - Result: 456 passed, 18 ignored, 0 failed.
  - `tests/grpc_build.rs`: 1 passed, 0 failed.
- `git diff --check` passed.
- `bash .agents/sow/audit.sh` verified SOW status/directory consistency. It exited nonzero on the existing unmodified `.agents/skills/mirror-netdata-repos/SKILL.md:112` Git SSH URL pattern that the audit classifies as email-like sensitive data.

End-user documentation check:

- No end-user docs were changed in this hardening pass.
- Previous BioRIS product docs already describe BioRIS as a bio-rd-compatible `RoutingInformationService` gRPC endpoint and do not describe internal test gaps.
- A docs search was run for internal test-gap phrases across `docs/network-flows`, `src/crates/netflow-plugin/metadata.yaml`, and generated NetFlow integration pages. No product-doc statement was found saying BioRIS or BMP are untested or explaining why internal testing could not be performed.

Artifact maintenance:

- AGENTS.md: no update needed; existing SOW, sensitive-data, and validation rules were sufficient.
- Runtime project skills: no update needed; `project-writing-collectors` and `integrations-lifecycle` already cover this workflow.
- Specs: no update needed; this hardening adds tests for existing intended contracts, not a new public contract.
- End-user/operator docs: no update needed in this pass; previous product-doc correction remains valid.
- End-user/operator skills: no update needed; no public skill workflow changed.
- SOW lifecycle: reopened from `done`, clarified the beta validation bar, recorded the contract-hardening work, then returned to `completed` and moved back to `.agents/sow/done/` with the hardening commit.

Outcome:

- Under the clarified beta validation target, no autonomous Netdata-side blocker remains for BMP or BioRIS contract proof.
- Broader production-router or production-bio-rd interoperability is explicitly outside this beta blocker and can be handled later as expanded validation or beta feedback.

### PR Review Follow-up - 2026-05-08 - Direct-agent Output Variable Failure Propagation

Trigger:

- Copilot opened one unresolved PR review thread on `docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh:192`.

Finding:

- `_agents_get_claim_id` called `_agents_set_outvar` without checking its exit status.
- Same-pattern search found additional `_agents_set_outvar` and helper-boundary call sites in `_agents_resolve_bearer` and `agents_query_agent`.
- If a caller supplied an invalid output variable name, the helper could continue after an assignment failure and make a public wrapper proceed with an unset or stale claim/bearer value.

Implementation:

- Propagated `_agents_set_outvar` failures with `|| return 1` at every call site.
- Propagated `_agents_get_claim_id` failure from `_agents_resolve_bearer`.
- Propagated `_agents_resolve_bearer` failure from `agents_query_agent`.
- Extended `agents_selftest_no_token_leak` with an invalid-output-variable path proving `_agents_get_claim_id` returns failure when `_agents_set_outvar` rejects the caller-provided variable name.

Validation:

- `bash -c 'source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh; agents_selftest_no_token_leak'` passed.
- `zsh -c 'source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh; agents_selftest_no_token_leak'` passed.
- `shellcheck --external-sources docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh` passed.
- Same-pattern search confirmed all `_agents_set_outvar` call sites now propagate failure.
- `git diff --check` passed.
- `bash .agents/sow/audit.sh` verified SOW status/directory consistency. It exited nonzero on the existing unmodified `.agents/skills/mirror-netdata-repos/SKILL.md:112` Git SSH URL pattern that the audit classifies as email-like sensitive data.

Artifact maintenance:

- AGENTS.md: no update needed; existing public-skill and sensitive-data rules were sufficient.
- Runtime project skills: no update needed.
- Specs: no update needed; this repaired helper failure propagation for an existing public-skill contract.
- End-user/operator docs: no separate docs update needed; the script behavior now matches the existing token-safe helper contract.
- End-user/operator skills: updated `docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh`.
- SOW lifecycle: reopened from `done`, recorded this PR review follow-up, then returned to `completed` and moved back to `.agents/sow/done/` with the fix commit.

### PR Review Follow-up - 2026-05-08 - Deterministic CAIDA Prefix2AS Resolution

Trigger:

- Copilot's refreshed review generated no new inline comments, but its review body listed a low-confidence note about `resolveCAIDAPrefix2ASURL` choosing the last `.pfx2as.gz` log entry.

Findings:

- The `parse.go` low-confidence note about missing `netipx` import was false; `src/go/tools/topology-ip-intel-downloader/parse.go` already imports `go4.org/netipx`.
- The CAIDA note was valid. The resolver walked the creation log and retained the last matching candidate, so an out-of-order log could select an older dataset.
- The live CAIDA creation log uses tab-separated rows with a numeric timestamp before the `.pfx2as.gz` path, so the resolver can select deterministically by timestamp and tie-break by path.

Implementation:

- Changed `resolveCAIDAPrefix2ASURL` to collect all `.pfx2as.gz` candidates.
- Sorted candidates by parsed numeric timestamp when available, with path as a deterministic fallback/tie-breaker.
- Updated `TestResolveCAIDAPrefix2ASURL` so an older candidate appears after the newer candidate, proving the resolver no longer depends on log order.

Validation:

- `curl -fsSL https://data.caida.org/datasets/routing/routeviews-prefix2as/pfx2as-creation.log | tail -n 20` confirmed the current CAIDA log row shape includes a numeric timestamp and path ending in `.pfx2as.gz`.
- `go test ./tools/topology-ip-intel-downloader` from `src/go` passed.
- `git diff --check` passed.
- `bash .agents/sow/audit.sh` verified SOW status/directory consistency. It exited nonzero on the existing unmodified `.agents/skills/mirror-netdata-repos/SKILL.md:112` Git SSH URL pattern that the audit classifies as email-like sensitive data.

Artifact maintenance:

- AGENTS.md: no update needed; existing PR review and SOW lifecycle rules were sufficient.
- Runtime project skills: no update needed; this did not change how future agents should work.
- Specs: no update needed; this hardens an existing downloader behavior without changing the public contract.
- End-user/operator docs: no update needed; no user-facing command, option, or provider contract changed.
- End-user/operator skills: no update needed.
- SOW lifecycle: reopened from `done`, recorded this PR review follow-up, then returned to `completed` for the deterministic CAIDA resolver commit.

### PR Review Follow-up - 2026-05-08 - Helper Comment And Parser Preallocation

Trigger:

- Copilot opened a new unresolved PR review thread on `docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh:172`.
- The same review body listed low-confidence notes about fixed `1<<20` and `1<<18` parser preallocations in `src/go/tools/topology-ip-intel-downloader/parse.go`.

Findings:

- The shell helper logic works under both bash and zsh in local smoke tests, but the source comment over-explained caller-local dynamic scoping and made the zsh support claim look broader than necessary.
- Same-pattern search found no other `Bash and zsh`, `dynamic scope`, or `dynamic scoping` wording in the direct-agent skill helper.
- Same-pattern search found all remaining fixed large range-slice preallocations in `parse.go`.
- The parser preallocation concern was valid: small payloads and tests paid immediate allocation cost for capacities intended for full datasets.

Implementation:

- Tightened the `_agents_set_outvar` comment to document only the `eval` avoidance reason.
- Added `estimatedRangeCapacity` for bounded initial capacity based on payload or zip-entry size.
- Replaced fixed `1<<20` and `1<<18` parser preallocations with size-based estimates where useful, or natural growth where no reliable row-count proxy exists.
- Added unit coverage for the capacity-estimation helper.

Validation:

- Same-pattern search found no remaining fixed `make([]asnRange|[]geoRange, 0, 1<<...)` allocations in `parse.go`.
- Same-pattern search found no remaining direct-agent helper comments claiming bash/zsh dynamic scoping.
- `go test ./tools/topology-ip-intel-downloader` from `src/go` passed.
- `bash -c 'source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh; agents_selftest_no_token_leak'` passed.
- `zsh -c 'source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh; agents_selftest_no_token_leak'` passed.
- `shellcheck --external-sources docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh` passed.

Artifact maintenance:

- AGENTS.md: no update needed; existing PR review and SOW lifecycle rules were sufficient.
- Runtime project skills: no update needed.
- Specs: no update needed; this is implementation hardening and comment clarification for existing behavior.
- End-user/operator docs: no update needed; no user-facing command, option, or provider contract changed.
- End-user/operator skills: updated `docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh`.
- SOW lifecycle: reopened from `done`, recorded this PR review follow-up, then returned to `completed` for the helper-comment and parser-preallocation commit.

### PR Review Follow-up - 2026-05-08 - Downloader URL, Archive, ZIP, And GoBGP Live-Test Hardening

Trigger:

- Copilot's refreshed review generated no new inline comments, but its review body listed low-confidence notes about:
  - MaxMind ASN tar detection relying on the fixed `ustar` marker.
  - CAIDA prefix2as URL construction using fixed suffix trimming and string concatenation.
  - ZIP entry lookup using OS-specific `filepath.Base`.
  - The optional GoBGP BMP live test reserving and then releasing an API port before starting `gobgpd`.

Findings:

- The downloader notes were valid hardening opportunities:
  - `extractMMDBFromTar` did reject tar payloads before attempting tar iteration unless the `ustar` marker was present.
  - `resolveCAIDAPrefix2ASURL` did build the resolved URL by trimming `pfx2as-creation.log` from the full URL string.
  - ZIP members use slash-separated paths, so `path.Base` is the correct package for archive member names.
- The GoBGP note was a real race in theory. The test is opt-in and local-only, but a retry loop keeps the validation from becoming flaky on shared machines.

Implementation:

- Changed CAIDA prefix2as URL resolution to parse `logURL`, remove query/fragment, normalize the directory path, and resolve the selected candidate with `url.ResolveReference`.
- Changed MaxMind ASN tar extraction to attempt tar iteration first, while still treating corrupt tar-like payloads with a `ustar` marker as extraction errors instead of silently accepting them as raw MMDB bytes.
- Added test coverage for CAIDA log URLs with query strings and nested paths.
- Added test coverage for gzipped legacy tar payloads without the `ustar` marker.
- Switched archive member base-name handling from `filepath.Base` to `path.Base`.
- Added retry behavior around the optional GoBGP API port reservation in the BMP live test.

Validation:

- `go test ./tools/topology-ip-intel-downloader` from `src/go` passed.
- `NETDATA_GOBGPD="$(pwd)/.local/audits/netflow-live/bin/gobgpd" NETDATA_GOBGP="$(pwd)/.local/audits/netflow-live/bin/gobgp" cargo test -p netflow-plugin routing::bmp --manifest-path src/crates/Cargo.toml` passed with 15 tests.
- `bash -c 'source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh; agents_selftest_no_token_leak'` passed.
- `zsh -c 'source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh; agents_selftest_no_token_leak'` passed.
- `shellcheck --external-sources docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh` passed.

Artifact maintenance:

- AGENTS.md: no update needed; existing PR review and SOW lifecycle rules were sufficient.
- Runtime project skills: no update needed.
- Specs: no update needed; this is implementation hardening for existing downloader and test behavior.
- End-user/operator docs: no update needed; no user-facing command, option, or provider contract changed.
- End-user/operator skills: no additional update needed beyond the direct-agent helper comment from the previous follow-up.
- SOW lifecycle: reopened from `done`, recorded this PR review follow-up, then returned to `completed` for the downloader and BMP live-test hardening commit.

### PR Review Follow-up - 2026-05-08 - Intel Downloader Provider Documentation

Trigger:

- Copilot's refreshed review body listed a low-confidence note about `netipx` import status and a note about `maxmind:geolite2-country@csv` being a ZIP bundle rather than a single raw CSV file.
- The user asked whether the PR was ready to merge and approved fixing the remaining documentation mismatch first.

Findings:

- The `netipx` import note was false. `src/go/tools/topology-ip-intel-downloader/parse.go` imports `go4.org/netipx`, and the downloader package test passes.
- The MaxMind `format: csv` note exposed a real user-documentation issue. `docs/network-flows/intel-downloader.md` still listed only DB-IP and IPtoASN, and still said MaxMind was not supported by the downloader.
- The current code supports DB-IP, IPtoASN, CAIDA prefix2as, MaxMind GeoLite2 ASN, MaxMind GeoLite2 Country, IP2Location, IPDeny, and IPIP through the downloader.
- MaxMind GeoLite2 Country and IP2Location country-lite use provider CSV ZIP bundles. For MaxMind Country, `format: csv` means the official GeoLite2 Country CSV ZIP bundle with locations plus IPv4/IPv6 block CSVs; it does not mean a single raw CSV file.

Implementation:

- Updated `docs/network-flows/intel-downloader.md` to list all supported provider/artifact combinations.
- Removed the stale statement saying MaxMind is unsupported by the downloader.
- Added explicit MaxMind `MAXMIND_LICENSE_KEY` and CSV ZIP bundle wording.
- Added notes for CAIDA, IP2Location, IPDeny, and IPIP payload shapes.
- Updated the refresh-cadence wording and per-provider links.

Validation:

- `rg` confirmed the stale "MaxMind unsupported" wording was removed and the new provider entries / MaxMind CSV ZIP clarification are present.
- `go test ./tools/topology-ip-intel-downloader` from `src/go` passed.
- `git diff --check` passed.
- `bash .agents/sow/audit.sh` verified SOW status/directory consistency. It exited nonzero on existing unrelated audit findings, including `.agents/skills/mirror-netdata-repos/SKILL.md:112` Git SSH URL pattern classified as email-like sensitive data.

Artifact maintenance:

- AGENTS.md: no update needed; this work followed the existing PR review and SOW lifecycle rules.
- Runtime project skills: no update needed.
- Specs: no update needed; source code and provider metadata already define the current downloader contract.
- End-user/operator docs: updated `docs/network-flows/intel-downloader.md`.
- End-user/operator skills: no update needed; no public skill workflow changed.
- SOW lifecycle: reopened from `done`, recorded this end-user documentation correction, then returned to `completed` for the docs commit.
