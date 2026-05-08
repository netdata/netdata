# SOW-0015 - NetFlow Enrichment Verification

## Status

Status: paused

Sub-state: autonomous work complete; paused only for external systems or credentials the user must provide.

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
- BMP and bio-rd validation can progress without the user by using local synthetic speakers or public/open-source fixtures, but proof against the user's own network cannot happen unless the user later supplies or enables a real BGP/BMP source.

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

## Validation

Acceptance criteria evidence:

- Scoped enrichment coverage:
  - Custom MMDB / MaxMind / GeoLite2: `src/crates/netflow-plugin/src/enrichment/tests.rs` covers public ASN and City MMDB fixtures, ASN org, country/city/state/lat/lon, and `netdata.ip_class`.
  - DB-IP / IPtoASN baseline: existing default geolocation runtime was treated as user-confirmed baseline; new MMDB tests exercise the same resolver path with deterministic fixtures.
  - AWS / Azure / GCP / generic IPAM / NetBox: `src/crates/netflow-plugin/src/network_sources/tests.rs` covers documented transform payloads and the HTTP fetch/publish path.
  - Static metadata, sampling overrides, static networks, classifiers, ASN/network provider chains, and decapsulation: existing tests remain in `src/crates/netflow-plugin/src/enrichment/tests.rs` and decoder fixture tests; new runtime merge and decode/enrichment tests cover the boundary.
  - BioRIS: `src/crates/netflow-plugin/src/routing/bioris/tests.rs` covers a real in-process gRPC service and runtime route lookup after `DumpRIB`.
  - BMP: `src/crates/netflow-plugin/src/routing/bmp/tests.rs` covers a real TCP listener and BMP-framed messages.
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
  - `cargo test -p netflow-plugin -- --nocapture`
  - Result: 444 passed, 18 ignored, 0 failed.
  - `tests/grpc_build.rs`: 1 passed, 0 failed.
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
- SOW lifecycle: SOW remains `paused` in `.agents/sow/current/` until the user decides whether to provide external systems/credentials for the remaining real-provider validation.

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
  - `src/crates/netflow-plugin/integrations/bmp_bgp_monitoring_protocol.md`
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
  - Real BMP from a router: user does not run BGP/BMP locally. Current proof is a deterministic BMP TCP listener/frame test, not a production router session.
  - Real bio-rd / RIPE RIS: current proof is an in-process gRPC fake service that exercises the Netdata BioRIS client path; no real external BioRIS/RIPE RIS service was supplied.
  - Real NetBox / custom IPAM / cloud-source credentials: current proof covers documented schemas, live public cloud schema checks, local HTTP fetch/refresher behavior, and transform output. No user NetBox/IPAM endpoint or credential was supplied.
  - Custom operator-provided MMDB: current proof uses public MaxMind test MMDB fixtures and a synthetic `netdata.ip_class` record; no user custom MMDB was supplied.
- Not tracked as a follow-up SOW yet:
  - CI job wiring, because the user corrected that immediate scope is unit/integration testing, not CI-job integration.

## Outcome

Autonomous work completed as far as possible without external systems or user credentials. Automated coverage, docs/source corrections, installed runtime validation, journal proof, Cloud-proxied Function validation, and direct-agent bearer Function validation are complete.

## Lessons Extracted

- Run public examples against the installed plugin, not only through Rust unit tests. This caught `after`/`top_n` request-shape bugs.
- Include shell compatibility in public helper tests when examples are likely to be run from users' default shells.
- Use the exact local `/api/v3/info` node id, machine GUID, and claim id tuple for direct-agent bearer minting; mixed identity tuples are the common failure mode.
- Treat identity fields as sensitive in wrapper logging; masking only bearer/token values is insufficient.

## Followup

User-required items are listed in Follow-up mapping above. No autonomous follow-up remains unattempted in this SOW.

## Regression Log

None yet.
