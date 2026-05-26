# SOW-0036 - SNMP Trap Plugin: SNMPv3 + INFORM + snmpEngineBoots + Per-Job Allowlist/Rate-Limit + BER Limits + DynCfg Schema + Self-Metrics

## Status

Status: paused

Sub-state: activated on 2026-05-26 after SOW-0035 reached implementation-complete state on the feature branch. Implementation and validation for the SOW-0036 feature-branch slice are complete through pass 10. Terminal completion is intentionally deferred to SOW-0039 because SOW-0035 through SOW-0039 are not independently mergeable and close together at the final collector-consistency and merge gate. Review cadence has been tightened per user decision: external reviewers run on meaningful batches, high-risk changes, and close gates, not on tiny local fixes.

## Requirements

### Purpose

Take the MVP plugin from SOW-0035 to production-grade per-job auth + safety + telemetry:

- SNMPv3 USM (static engineID whitelist, per-job) with the full HMAC-SHA-2 + AES-128/192/256 priv suites.
- INFORM acknowledgement with `snmpEngineBoots` persistence per job (the prerequisite for v3 INFORM Response integrity across plugin restarts).
- Per-job pre-decode allowlist + rate limiting.
- BER decode resource limits per spec §18 (DoS protection on the UDP-exposed parser).
- Plugin configuration schema mirroring spec §7.5 per-job structure; DynCfg orchestration refinement (add/update/enable/disable per job).
- Plugin-self NIDL metrics with the full error universe per spec §12 (per-job dimensions).

The collector consistency bundle (`metadata.yaml`, `config_schema.json`, stock `.conf`, `health.d/*.conf`, `README.md`, `taxonomy.yaml`) is OWNED BY SOW-0039 and is not in this SOW's scope.

### User Request

Sequential after SOW-0035 per the user-approved 5-SOW lineup. Per user: "We need to add metadata.yaml and the rest - this can be done at the end as an extra SOW" - bundle moved to SOW-0039.

User continuation recorded on 2026-05-26:

- SDK append/drain throughput is handled on the systemd-journal SDK side.
- Continue with the remaining SNMP trap plan while SOW-0039 keeps the final SDK throughput re-check as a merge-gate item.

### Assistant Understanding

Facts:

- Per spec §5, each job has its own `snmpEngineBoots` counter persisted at `/var/lib/netdata/snmp-trap/{job_name}/engine-boots`. Without persistence, devices that cached the old boot counter reject INFORM Response PDUs as replay attacks after every Netdata restart (RFC 3414 §2.2.2).
- Per spec §6, INFORM Response is sent synchronously from the receive socket immediately after BER decode + auth; sender retransmits per RFC 3414 §3 handle UDP loss.
- Per spec §18, BER decode limits apply **per-job** with identical default values across all jobs (each job's decoder enforces independently against its own UDP buffer — "global" means uniform defaults, NOT a single shared decoder state). Per spec §12, the `snmp.trap.errors` dimensions are: `unknown_oid, decode_failed, template_unresolved, malformed_pdu, dropped_allowlist, rate_limited, auth_failures, usm_failures, unknown_engine_id, inform_response_failed, sanitized, profile_load_failed, journal_write_failed`. All metrics carry `job_name` as a label.
- SNMP communities and USM keys must be sourced from the existing Netdata Secrets infrastructure (no raw keys in any committed artifact).
- Per spec §5, SNMPv3 dynamic engineID discovery is opt-in and deferred to SOW-0038; this SOW ships only the static whitelist mode.

Inferences:

- The plugin-self metric universe drives `metadata.yaml` content (SOW-0039); the metric set must be settled in M4 before SOW-0039 can author `metadata.yaml`.

Resolved decisions:

- Rate limiting is default-off per spec §7.5. When enabled, the default is `per_source_pps: 1000` and `mode: drop`.
- M4 emits the per-job metric foundation in SOW-0036. Full spec §12 per-device/vnode labels are explicitly handed to SOW-0037/SOW-0039 because SOW-0037 is where source-device enrichment and vnode identity become available.
- On 2026-05-26, user approved refined option A for SNMPv3 INFORM receiver-local engine ID handling: `local_engine_id` is optional; if omitted, Netdata generates and persists a stable per-job local engine ID. Creation-time preflight must fail DynCfg apply if Netdata cannot create, read, or write the local engine ID state. Remote sender engine IDs for v3 Trap authentication remain separate from the receiver-local engine ID used for v3 INFORM.

### Acceptance Criteria

- M1: SNMPv3 USM works for SHA-224/SHA-256/SHA-384/SHA-512 auth + AES-128/192/256 priv against a static engineID whitelist per job; INFORM acknowledgement returns proper Response PDU synchronously on the receive socket; `snmpEngineBoots` persists to `/var/lib/netdata/snmp-trap/{job_name}/engine-boots` across plugin restarts.
- M2: pre-decode source-IP CIDR allowlist drops unwanted traffic before any BER parse; community-string allowlist (v1/v2c); per-source token-bucket rate-limit (drop or sample mode); BER decode limits per spec §18 enforced with counter increments on violation.
- M3: per-job plugin configuration loadable from `/etc/netdata/go.d/snmp.trap.conf`; DynCfg integration handles Add/Update/Enable/Disable/Remove per job; bind-or-fail surfaces in DynCfg as HTTP-422.
- M4: plugin-self NIDL metrics emit at 1Hz cold-path with hot-path being counter-increment only. SOW-0036 owns the metric names, chart template, and full event/error dimension universe with `job_name` labeling. The richer spec §12 per-device/vnode labels (`device`, `vendor`, `hub`, `severity`, operator labels) depend on SOW-0037 enrichment because SOW-0036 has no SNMP polling/topology state to resolve device vnode, vendor, or hub identity. `snmp.trap.dedup_suppressed` is owned by SOW-0037.

## Milestones

### M1 — SNMPv3 USM (static users) + INFORM acknowledgement + `snmpEngineBoots` persistence

- Per-job USM user table from config (username, auth proto/key, priv proto/key, engineID; keys via Netdata Secrets refs only — never raw).
- HMAC-SHA-2 family (SHA-224 / SHA-256 / SHA-384 / SHA-512) auth; AES-128 / AES-192 / AES-256 priv (gosnmp or chosen Rust crate per SOW-0035 must reach this parity).
- Static engineID whitelist per job; an unknown engineID is rejected with `snmp.trap.errors.unknown_engine_id` increment + log entry; dynamic discovery is OUT of scope here (SOW-0038 M3 opt-in).
- INFORM-Request PDU acknowledgement (Response PDU) per spec §6 semantics:
  - Sent synchronously on the receive socket, immediately after BER decode + auth, before journal write.
  - Same UDP socket as receive (correct source-port for NAT).
  - Retransmits handled by idempotency (sender re-sends same `request-id` → receiver re-emits same Response).
  - Send failure: log + `snmp.trap.errors.inform_response_failed` increment; trap still processed.
- `snmpEngineBoots` persistence per job: on plugin start, read the per-job file at `/var/lib/netdata/snmp-trap/{job_name}/engine-boots`, increment by 1, persist atomically (write-temp + fsync + rename). When the file does not exist (first boot for the job), initialize to `1` per RFC 3414 §2.2.2 (boot counter starts at 1, increments on each restart). Document operator-visible behavior: the first Netdata restart after initial v3 INFORM deployment will not be a "replay attack" for senders that have not yet cached the agent's boot counter; subsequent restarts increment the counter as required. Tested across plugin restart against a real v3 INFORM sender or a protocol simulator (`snmptrapd` in INFORM mode, or a scripted `pysnmp` agent).

Cohort reference: `splunk-sc4snmp.md` §3.5 (engineID handling); `logstash.md` §3.6 (SNMP4j parity); RFC 3414 §2.2.2 (boot counter requirement).

Reviewers: `glm`, `kimi`, `minimax`, and `qwen` - auth/crypto critical. `mimo` is removed due to quota exhaustion.

### M2 — Per-source allowlist + rate limiting + BER decode limits

- Per-job source-IP CIDR allowlist (IPv4 + IPv6); pre-decode drop with `snmp.trap.errors.dropped_allowlist` increment.
- Per-job community-string allowlist (v1/v2c only).
- Per-job per-source token-bucket rate limit; configurable `mode: drop | sample`; `snmp.trap.errors.rate_limited` increment on drop.
- BER decode resource limits per spec §18 (applied per-job, identical defaults across jobs; each job's decoder enforces independently):
  - Max UDP datagram bytes: 8192
  - Max varbinds per PDU: 256
  - Max BER nesting depth: 8
  - Max OID encoded length: 128 bytes
  - Max OctetString value: 1024 bytes
  - Per-PDU decode budget: 1 ms
- Violation → drop + `snmp.trap.errors.malformed_pdu` increment.

Cohort reference: `splunk-sc4snmp.md`, `logstash.md` (allowlist patterns); spec §18 (BER limits derived from Phase B design-implications R22).

Reviewers: 3 rotating from `glm`, `kimi`, `minimax`, and `qwen`.

### M3 — Plugin configuration schema + DynCfg per-job orchestration

- `/etc/netdata/go.d/snmp.trap.conf` schema per spec §7.5 (per-job structure with `listen`, `versions`, `communities`, `usm_users`, `engine_id_whitelist`, `allowlist`, `rate_limit`, `dedup`, `retention`, `overrides`, `metrics`).
- DynCfg integration mirroring `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go`:
  - Add/Enable creates job, attempts bind. Failure → HTTP-422 coded error.
  - Update stops old job, recreates with new config.
  - Disable/Remove stops job, closes socket, retains journal directory.
- Validation at load: reject unknown keys, malformed CIDRs, invalid USM proto names, SNMPv3 passphrases shorter than the RFC 3414 Appendix A minimum, and invalid label-key characters. Per spec §7.5, operator label keys are only syntax-validated because the `TRAP_TAG_*` namespace structurally prevents journal-field collisions.

Cohort reference: existing go.d collector config patterns; `src/go/plugin/agent/jobmgr/dyncfg_collector.go` (DynCfg routing).

Reviewers: 3 rotating from `glm`, `kimi`, `minimax`, and `qwen`. DeepSeek remains an implementation worker, not a reviewer.

### M4 — Plugin-self NIDL metrics (per-job foundation)

- Hot path: atomic counter increments only after the job has started. First-seen bounded admission-control state (rate-limit source buckets) is capped and swept.
- Cold path: 1Hz sweep of per-job counters → metrix snapshot counters exposed through the go.d V2 chart template.
- Metric universe per spec §12, SOW-0036 scope:
  - `snmp.trap.events` dimensions: 8 category slugs `state_change`/`config_change`/`security`/`auth`/`license`/`mobility`/`diagnostic`/`unknown`; labels: `job_name`.
  - `snmp.trap.errors` dimensions: `unknown_oid`, `decode_failed`, `template_unresolved`, `malformed_pdu`, `dropped_allowlist`, `rate_limited`, `auth_failures`, `usm_failures`, `unknown_engine_id`, `inform_response_failed`, `sanitized`, `profile_load_failed`, `journal_write_failed`; labels: `job_name`.
- Full spec §12 per-device/vnode labels remain a SOW-0037/SOW-0039 handoff because SOW-0037 M1 is the first milestone that populates `_HOSTNAME`, `ND_NIDL_NODE`, `TRAP_DEVICE_VENDOR`, topology fields, and source-device identity.
- `snmp.trap.dedup_suppressed` is owned by SOW-0037 (only emitted when opt-in dedup is enabled).

Cohort reference: spec §12; existing collector NIDL patterns (`project-writing-collectors` skill).

Reviewers: 3 rotating from `glm`, `kimi`, `minimax`, and `qwen`.

## Reviewer Protocol

- M1: `glm`, `kimi`, `minimax`, and `qwen` (auth/crypto critical).
- M2 + M3 + M4: 3 rotating reviewers per round from `glm`, `kimi`, `minimax`, and `qwen`.
- Fix-cycle: same reviewers as the round being fixed.
- Implementation worker: `deepseek/deepseek-v4-pro` without `--agent code-reviewer`; it needs write access.
- External assistant process rule: run with stdin closed via `</dev/null`.

## Pre-Implementation Gate

Status: passed for activation; M1 begins now.

Problem/root-cause model:

- SOW-0035 deliberately shipped the MVP foundation: v1/v2c decode, profile lookup, and journal write. Production trap ingestion still needs auth, INFORM acknowledgement, pre-decode admission control, and operator-visible error counters.
- Current trap handling drops several failure classes silently (`DecodeTrap` error, disallowed version/community, writer queue/write error). SOW-0036 converts those paths into the spec §12 error universe without putting allocations on the hot path.
- SNMPv3 INFORM correctness requires persistent `snmpEngineBoots`. Without per-job persistence, devices can reject responses after Netdata restarts as replay-protection failures.
- The UDP parser is externally reachable. Source allowlist, token-bucket rate limiting, and BER resource limits must be validated at job creation/config application where possible, then enforced before or during decode.

Evidence reviewed:

- `.agents/sow/specs/snmp-traps/netdata.md:115` defines one job as one listener with endpoints, auth, allowlist, rate limit, writer, journal directory, retention policy, and `snmpEngineBoots`.
- `.agents/sow/specs/snmp-traps/netdata.md:162` requires pre-decode source-IP allowlist before BER cost.
- `.agents/sow/specs/snmp-traps/netdata.md:165` places per-job rate limiting after version/auth checks, with operator-selected drop or sample behavior.
- `.agents/sow/specs/snmp-traps/netdata.md:216` through `.agents/sow/specs/snmp-traps/netdata.md:224` define INFORM response timing, same-socket send, idempotent retransmit behavior, and failure accounting.
- `.agents/sow/specs/snmp-traps/netdata.md:383` through `.agents/sow/specs/snmp-traps/netdata.md:388` define default allowlist and rate-limit config: `source_cidrs: ["0.0.0.0/0"]`, `rate_limit.enabled: false`, `per_source_pps: 1000`, `mode: drop`.
- `.agents/sow/specs/snmp-traps/netdata.md:908` through `.agents/sow/specs/snmp-traps/netdata.md:913` define the `snmp.trap.errors` context and dimensions.
- `.agents/sow/specs/snmp-traps/netdata.md:1046` through `.agents/sow/specs/snmp-traps/netdata.md:1057` define BER resource limits.
- `src/go/plugin/go.d/collector/snmp_traps/collector.go:189` through `src/go/plugin/go.d/collector/snmp_traps/collector.go:201` currently returns silently on decode, policy, and writer errors.
- `src/go/plugin/go.d/collector/snmp_traps/listener.go` owns the UDP sockets; INFORM acknowledgement requires extending the packet handler so the response is written on the same receive socket.
- `src/go/plugin/go.d/collector/snmp_traps/decode.go` already has BER limit constants and maps INFORM to `PduTypeInform`, but currently decodes with no SNMPv3 security table.
- `src/go/plugin/go.d/pkg/snmputils/utils.go` already parses the SNMPv3 security level, auth protocol, and privacy protocol names used by the SNMP polling collector.
- Actual compiled GoSNMP dependency is `github.com/ilyam8/gosnmp v0.0.0-20250912202722-388b2cb5192e`, replacing `github.com/gosnmp/gosnmp v1.42.1`; local source supports SHA224/SHA256/SHA384/SHA512 auth, AES/AES192/AES256 plus `AES192C`/`AES256C`, `TrapSecurityParametersTable`, `UnmarshalTrap`, INFORM request handling, and exported `SnmpEncodePacket`.
- `src/go/plugin/agent/jobmgr/config_apply.go` and `src/go/plugin/agent/secrets/resolver/resolver.go` resolve Netdata secret references before collector config unmarshalling, so raw USM keys and communities must not be written to committed examples/tests.

Affected contracts and surfaces:

- SNMP trap config structs, `config_schema.json`, validation, defaults, and DynCfg apply errors.
- Listener packet handler signature and UDP response path for INFORM.
- Decoder security parameters and version/auth failure classification.
- Per-job state under `/var/lib/netdata/snmp-trap/{job_name}/engine-boots`.
- In-memory counters and NIDL chart emission for `snmp.trap.events` and `snmp.trap.errors`.
- Trap writer error visibility without changing the SOW-0035 journal field contract.
- Tests and fixtures for v3 auth/privacy, INFORM, allowlist, rate limit, BER limits, engineBoots, and metric counters.

Existing patterns to reuse:

- Reuse SNMP polling protocol parsing from `src/go/plugin/go.d/pkg/snmputils/utils.go`; keep accepted SNMPv3 protocol spelling consistent with the existing SNMP collector.
- Reuse SOW-0035 `dyncfgCodedError` behavior for creation-time validation failures.
- Reuse the current listener preflight/bind lifecycle, but pass a packet context that can reply on the owning UDP socket.
- Reuse the current BER pre-scan constants and tests; expand error classification rather than weakening limits.
- Reuse go.d framework V2 collector patterns and map-keyed table tests.

Risk and blast radius:

- Auth/crypto mistakes can silently drop valid v3 traps or accept invalid ones. Tests must include auth/priv success and failure cases.
- INFORM response generation must preserve the sender-visible UDP source port. Sending from a new socket is incorrect behind NAT and violates spec §6.
- Rate limiting can drop forensic data. The default is therefore off per spec; when enabled, drops must be counted.
- `snmpEngineBoots` persistence touches filesystem state outside the journal directory and must fail at job creation if the directory/file cannot be created, read, incremented, fsynced, or renamed.
- Counter maps must not allocate per trap on the hot path. Any per-source state such as rate limiters must be prebounded or lazily allocated only on first-seen source with clear cleanup behavior.
- Secret material must not be logged, stored in SOW artifacts, or committed in fixtures.

Sensitive data handling plan:

- No raw SNMP communities, USM auth keys, privacy keys, device identities, customer IPs, or customer endpoints in SOWs, specs, docs, skills, code comments, committed examples, or tests.
- Tests use synthetic engine IDs and dummy passphrases only.
- Config examples prefer Netdata secret references for USM keys and non-public communities.
- Logs and coded errors name the invalid field/protocol/source class, not secret values.

Sensitive data gate:

- Passed for implementation start. The work may process SNMP communities, USM passphrases, engine IDs, source IPs, and device identifiers, but durable artifacts must contain only dummy fixtures, placeholders, or field names.
- Any discovered real customer/device values must be redacted before they are copied into SOWs, specs, docs, skills, comments, tests, prompts, or review material.

Implementation plan:

1. Extend config/defaults/schema for v3 users, engine IDs, allowlist, rate limit, and metrics controls; validate all malformed CIDRs, protocol names, security-level combinations, label keys, and unknown keys at job creation.
2. Add per-job `snmpEngineBoots` manager with atomic write-temp, fsync, rename, and creation-time failure surfacing.
3. Extend the listener handler to include peer address and a same-socket response writer for INFORM acknowledgements.
4. Build v3 decode support using GoSNMP `TrapSecurityParametersTable`, static engineID whitelist, existing `snmputils` protocol parsing, and explicit error classification for auth/usm/unknown-engine failures.
5. Generate INFORM Response PDUs synchronously after successful decode/auth and before journal write; count send failures while still processing the trap.
6. Implement pre-decode CIDR allowlist, post-decode v1/v2c community allowlist, version policy counters, and optional per-source token-bucket rate limiting.
7. Implement hot-path atomic counters and cold-path NIDL emission for `snmp.trap.events` and `snmp.trap.errors`; do not implement `snmp.trap.dedup_suppressed` here.
8. Add focused tests for job-creation validation, v3 auth/privacy coverage, INFORM response, engineBoots persistence, allowlist/rate-limit behavior, BER-limit accounting, writer error accounting, and metric emission.

Validation plan:

- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 240s`
- `go test -race ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 360s`
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...`
- `jq empty plugin/go.d/collector/snmp_traps/config_schema.json`
- `git diff --check`
- `.agents/sow/audit.sh`
- External review rounds with `glm`, `kimi`, `minimax`, and `qwen`, repeated until clean.

Artifact impact plan:

- SOW-0036 records decisions, validation, reviewer findings, and any follow-ups.
- Specs are updated only if implementation finds a spec/code mismatch; current rate-limit defaults already exist in spec §7.5 and §10.
- Runtime project skills are updated only if new durable workflow guidance is discovered.
- End-user docs, stock config, `metadata.yaml`, `health.d`, README, and taxonomy are intentionally deferred to SOW-0039, but any schema behavior added here must be available for SOW-0039 to document.

Open decisions:

- None before implementation. The apparent rate-limit default question is resolved by spec §7.5 and §10: default-off, `per_source_pps: 1000`, `mode: drop`.

## Plan

Sequential M1 -> M4. The collector consistency bundle that AGENTS.md mandates is NOT in this SOW; it is owned by SOW-0039. SOW-0036 is one of four SOWs (0035-0038) landing on a feature branch until SOW-0039 produces the bundle and a single PR sequence merges.

## Execution Log

### 2026-05-25

SOW rewritten under the 5-SOW lineup: M5 consistency bundle removed (moved to SOW-0039); M1 expanded to include `snmpEngineBoots` persistence; M2 adds BER decode limits per spec §18; M4 expanded to full metric universe per spec §12. Not yet activated.

### 2026-05-26

Activated after SOW-0035 reached implementation-complete state on the feature branch. SOW-0035 remains `paused` until SOW-0039 final merge-gate closeout. Pre-implementation gate filled with spec/code evidence. Rate-limit defaults resolved from spec: default-off, `per_source_pps: 1000`, `mode: drop`.

### 2026-05-26 (implementation pass 1)

Implemented end-to-end M1-M4 code changes across the snmp_traps collector:

**Files changed:**
- `config.go` — Added USMUserConfig, AllowlistConfig, RateLimitConfig, OverrideConfig, MetricConfig structs; extended Config with USMUsers, EngineIDWhitelist, Allowlist, RateLimit, Overrides, Metrics
- `config_schema.json` — v3 in versions enum, usm_users, engine_id_whitelist, allowlist, rate_limit, retention, overrides, metrics
- `init.go` — validateUSMUsers, validateEngineIDWhitelist, validateAllowlist, validateRateLimit, validateOverrides, validateConfigLabelKey; versions now accepts v3
- `engineboots.go` — New per-job snmpEngineBoots persistence with atomic write-temp+fsync+rename; reads existing counter and increments by 1
- `allowlist.go` — CIDR-based source IP allowlist using netip.Prefix; community string allowlist
- `ratelimit.go` — Per-source token-bucket rate limiter with drop/sample modes; configurable PPS and burst
- `metrics.go` — Hot-path atomic counter increments for snmp.trap.events (8 categories) and snmp.trap.errors (13 dimensions); cold-path metrix collection via CollectorStore
- `decode.go` — Unified DecodeTrap returning TrapPacketContext with embedded SnmpPacket for INFORM replies; v3 decode via GoSNMP TrapSecurityParametersTable; ClassifyDecodeError for dimension routing
- `listener.go` — Handler signature changed to include *net.UDPConn and *net.UDPAddr for INFORM response send
- `collector.go` — Integrated allowlist, rate limiter, engine boots, v3 security table, engine ID whitelist, overrides, metrics collection; removed deprecated communitySet
- `inform.go` — sendInformResponse writes Response PDU on same receive socket; buildSnmpV3SecurityTable maps config to GoSNMP security params
- `decode_test.go` — Updated for new DecodeTrap signature; added v3 decode tests (noAuth success, wrong user usm_failures), ClassifyDecodeError table test; removed obsolete v3-rejection tests
- `init_test.go` — Updated InvalidVersion test to v5; v3 is now valid
- `pcap_test.go` — Updated DecodeTrap call to new signature with secTable=nil and .PDU accessor
- `pipeline_test.go` — Updated handlePacket call; added tests for version disallow, community disallow, event metric increment, allowlist drop metric, engineBoots persistence, CIDR allowlist, community allowlist, rate limiter, config validation (USM, engine IDs, CIDRs, label keys)

**Validation results:**
- `go test ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`: PASS (1.632s)
- `go test -race ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`: PASS (16.216s)
- `go vet ./plugin/go.d/collector/snmp_traps/...`: no output
- `jq empty config_schema.json`: no output
- `git diff --check`: no output
- `.agents/sow/audit.sh`: all structural checks pass

**Not yet implemented / deferred:**
- Real-device or protocol-simulator v3 INFORM restart validation remains a SOW-0039 merge-gate item; synthetic noAuth, SHA-2 auth, AES privacy, INFORM engine ID split, authPriv Response, and local engine ID persistence paths are covered by unit tests
- Collector-consistency documentation for the emitted metric contexts (`metadata.yaml`, README, taxonomy, and health templates) is deferred to SOW-0039; the SOW-0036 code embeds the chart template and emits `snmp.trap.events` plus `snmp.trap.errors` with `job_name`
- `snmp.trap.dedup_suppressed` is owned by SOW-0037
- `profile_load_failed` counter is never incremented — profile load is a creation-time failure, not a runtime error; dimension is reserved for SOW-0037 hot-reload failures

**Sensitive data gate:** No secrets, community strings, USM keys, device identities, or customer IPs in committed artifacts. Tests use synthetic engine IDs and dummy passphrases only. Config examples use Netdata secret references.

### 2026-05-26 (review round 1 and implementation pass 2)

External reviewers run per protocol: `glm`, `kimi`, `minimax`, and `qwen`, all with stdin closed via `</dev/null`. `mimo` was not run due to quota exhaustion. Findings were triaged as follows:

- Fixed: disallowed SNMP version now increments `dropped_allowlist` instead of silently returning.
- Fixed: rate limiter now caps tracked source buckets at 10,000 and sweeps old buckets, preventing unbounded memory growth under spoofed-source storms.
- Fixed: unused hot-path `tokenCount` atomic removed.
- Fixed: decode budget test now forces and asserts `errDecodeBudgetExceeded`.
- Fixed: `template_unresolved` increments when rendered message/labels contain unresolved or missing template markers.
- Fixed: `sanitized` is published from the journal writer's sanitized-field counter during cold-path collection.
- Fixed: `collectMetrics` no longer holds the global metrics mutex during metrix writes.
- Fixed: `TrapPacketContext` duplicate fields were removed; callers use `ctx.PDU`.
- Fixed: `snmpEngineBoots` test now proves restart increment from 1 to 2.
- Fixed: YAML validation now rejects duplicate engine IDs in `engine_id_whitelist`.
- Fixed: `rate_limit.per_source_pps: 0` is documented and schema-valid as "use default 1000"; negative values still fail creation.
- Fixed: listener buffer has a comment explaining the `maxDatagramSize+1` oversize-classification byte.
- Fixed: redundant USM security-level validation helper removed.
- Accepted as correct: `snmpEngineBoots` max remains `2147483647`; RFC 3414 defines it as the range `1..2147483647`, not `uint32` max.
- Accepted as correct: v3 INFORM Response sets local authoritative boots/time. RFC 3414 section 1.5.1 makes the receiver authoritative for messages that expect a response.
- Accepted as correct: GoSNMP `MarshalMsg()` calls v3 scoped-PDU encryption and authentication before returning the response bytes.
- Scope clarified: full spec §12 per-device/vnode labels are not implemented in SOW-0036 because SOW-0037 M1 is the source-device enrichment milestone. SOW-0036 owns the metric names, chart template, event/error dimension universe, and `job_name` labels.

Validation after pass 2:

- `go test ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`: PASS
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 240s`: PASS
- `go test -race ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 360s`: PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...`: PASS
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json`: PASS
- `git diff --check`: PASS
- `.agents/sow/audit.sh`: PASS with the pre-existing non-project skill classification warning

### 2026-05-26 (review round 2 and implementation pass 3)

Follow-up reviewers rechecked the same full scope. Findings were triaged as follows:

- Fixed: IPv4-mapped IPv6 peers are now normalized with `Unmap()` before source CIDR allowlist and rate-limit bucket lookup. This prevents explicit IPv4 CIDRs such as `10.0.0.0/8` from rejecting IPv4 traffic received as `::ffff:10.x.x.x`.
- Fixed: INFORM Responses are now sent after successful decode/auth and engine-ID policy, before rate-limit drop can suppress journal processing. This avoids causing rate-limited INFORM senders to retransmit a request that has already been authenticated.
- Fixed: INFORM Response send failures now log via the collector logger when available and still increment `inform_response_failed`.
- Fixed: v3 setup failure paths no longer leak a trap-writer worker goroutine; the journal trap writer starts only after engine-boots/security-table/whitelist setup succeeds.
- Fixed: decode-error engine-ID extraction now runs only for auth/USM/engine-ID classifications, avoiding a second BER walk for clearly malformed packets.
- Fixed: `buildOverrideMap` stores override copies instead of pointers into the config slice.
- Fixed: redundant `strings.ToLower` in rate-limit validation removed.
- Fixed: test-only `snmpV3SecurityLevel` helper moved out of production code.
- Added tests: v2c INFORM Response round-trip, INFORM response before rate-limit drop, IPv4-mapped CIDR allowlist, v3 post-decode engine-ID whitelist rejection, rate-limit sample-mode write-through, engine-boots corrupt-file handling, max-value rejection, and read-error failure. Pass 6 tightened corrupt-file handling from self-repair to creation-time failure.
- Superseded by pass 6: v3 INFORM response round-trip with full auth/privacy is now covered by a synthetic authPriv unit test. Real-device or protocol-simulator restart validation remains a pre-merge validation item.
- Accepted as non-blocking: broader rate-limited logging for auth/USM/allowlist drops is useful operator ergonomics but belongs with SOW-0039 docs/alerts or a small follow-up if maintainers want it; counters are implemented now.

Validation after pass 3:

- `go test ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`: PASS
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 240s`: PASS
- `go test -race ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 360s`: PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...`: PASS
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json`: PASS
- `git diff --check`: PASS
- `.agents/sow/audit.sh`: PASS with the pre-existing non-project skill classification warning

### 2026-05-26 (review round 3 and implementation pass 4)

Follow-up reviewers rechecked the same full scope. `glm`, `kimi`, and `minimax` reported no blocking issues. The first `qwen` process exited before the final review chunk was retrieved, so `qwen` was rerun with the same prompt and stdin disabled.

Findings were triaged as follows:

- Fixed: SNMPv3 USM `auth_key` and `priv_key` now fail creation-time validation when the corresponding protocol is enabled and the passphrase is shorter than 8 characters. RFC Editor RFC 3414 Appendix A says SNMP implementations and configuration applications must ensure passwords used by the password-to-key algorithm are at least 8 characters.
- Fixed: validation tests now cover short auth and privacy passphrases, and the valid USM test uses passphrases meeting the minimum.
- Fixed: `config_schema.json` help text now states the 8-character minimum when auth/privacy passphrases are required. Conditional rejection remains in Go validation so noAuth/noPriv configs are not over-rejected by schema-only validation.
- Fixed: stock config comment no longer says community auth is waiting for SOW-0036.
- Fixed: INFORM response engine-boots assignment now uses the SNMPv3 `maxSnmpEngineBoots` constant instead of `math.MaxUint32`.
- Accepted as non-blocking: `profile_load_failed` is present but not incremented until SOW-0037 hot reload; profile load failures in SOW-0036 remain creation-time failures.
- Accepted as non-blocking: `EngineBoots` and other read-only v3 state are not explicitly nilled in `Cleanup()` because the Collector is discarded, but cleanup already stops sockets/writer, releases profile cache, and removes metrics.

Validation after pass 4:

- `go test ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`: PASS
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 240s`: PASS
- `go test -race ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 360s`: PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...`: PASS
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json`: PASS
- `git diff --check`: PASS
- `.agents/sow/audit.sh`: PASS with the pre-existing non-project skill classification warning

### 2026-05-26 (review round 4 and implementation pass 5 — local engine ID)

Protocol design decision (review round 4) resolved: user approved refined option A — `local_engine_id` is optional; when omitted, Netdata generates and persists a stable per-job local engine ID.

Mirror/protocol evidence reviewed before proceeding:

- RFC 3411 defines `SnmpEngineID` as 5-32 octets, disallows empty/all-zero/all-`ff` values, recommends non-volatile storage, and describes enterprise-format generation with the first bit set. Evidence: RFC 3411 §5, `SnmpEngineID` textual convention and `snmpEngineID` object.
- `opennms/opennms @ 6b76f0f23acadbf774d440eb42ff4381155ffd38`, `features/events/traps/src/test/java/org/opennms/netmgt/trapd/TrapdInformIT.java:152` and `:163` verify INFORM authoritative engine ID discovery matches the receiver's local engine ID before and after restart.
- `opennms/opennms @ 6b76f0f23acadbf774d440eb42ff4381155ffd38`, `core/snmp/impl-snmp4j/src/main/java/org/opennms/netmgt/snmp/snmp4j/Snmp4JStrategy.java:860` returns a local engine ID derived from a persistent instance ID.
- `splunk/splunk-connect-for-snmp @ fdd4c74ef3cc8295675039be9f432b00e48b96d8`, `splunk_connect_for_snmp/traps.py:201` hot-registers newly observed sender engine IDs for v3 Trap processing, and `splunk_connect_for_snmp/traps.py:229` captures engine IDs from raw UDP datagrams before pysnmp parsing.
- `snmp/pysnmp @ 4891556e7db831a5a9b27d4bad8ff102609b2a2c`, `docs/net-snmptrapd.conf:10` separates v3 INFORM user setup from v3 Trap `createUser -e <engineID>` setup at `docs/net-snmptrapd.conf:21`, matching the local-vs-sender engine-ID split.

**Files changed:**
- `engineboots.go` — New `LocalEngineID` type with per-job persistence at `/var/lib/netdata/snmp-trap/{job_name}/local-engine-id`. Configured IDs validated (5-32 byte hex, not all zero/all `ff`). When omitted, generates a 12-byte random receiver-local ID with the enterprise-format bit clear and persists atomically (write-temp+fsync+rename, same pattern as EngineBoots).
- `config.go` — Added `LocalEngineID` field to `Config` struct and YAML strict-key spec.
- `config_schema.json` — Added `local_engine_id` field (optional string, hex format, 5-32 bytes); updated `engine_id_whitelist` description to clarify it applies to v3 Trap PDUs.
- `snmp_traps.conf` — Added commented examples for v3 USM users, engine_id_whitelist, and local_engine_id.
- `init.go` — Added `validateLocalEngineID` function (empty passes, non-empty must be valid 5-32 byte hex).
- `inform.go` — `sendInformResponse` now takes `localEngineID []byte` parameter; sets `AuthoritativeEngineID` to the local engine ID (raw bytes) in INFORM Response. Added `registerUSMUsersWithLocalEngineID` to register each USM user with the local engine ID so v3 INFORM auth keys are localized to the receiver.
- `collector.go` — Added `localEngineID *LocalEngineID` to `Collector` struct. In `Init`: validates `local_engine_id` config, creates `LocalEngineID` when v3 is enabled (fails DynCfg with 422 on creation/read/write errors), registers USM users with local engine ID in the security table. In `handlePacket`: splits v3 engine ID check — v3 Trap checks `engine_id_whitelist` (unchanged), v3 INFORM checks the packet's authoritative engine ID equals the local engine ID via `localEngineIDMatches`. Added `localEngineIDMatches` helper comparing packet USM engine ID to local engine ID.
- `inform_test.go` — Updated `sendInformResponse` call for new signature.
- `local_engine_id_test.go` — focused tests: configured local_engine_id accepted/persisted/reloaded; omitted generates stable persisted value; invalid hex/all-zero/all-`ff` fails validation; Init fails when corrupt persisted value; Init fails when directory cannot be created; v3 INFORM with local engine ID accepted; v3 INFORM with non-local engine ID rejected as unknown_engine_id; v3 Trap still requires sender engine whitelist; v3 INFORM response contains local engine ID and boots/time; cleanup with nil localEngineID. All state-file tests use temporary state directories, not `/var/lib/netdata`.

**Design decisions:**
- RFC 3414 section 1.5.1: receiver is authoritative for confirmed-class (INFORM) messages => v3 INFORM engine ID check uses local engine ID, not sender whitelist.
- USM users are registered TWICE in the security table: once with sender engine IDs (for v3 Trap auth, existing behavior), once with local engine ID (for v3 INFORM auth). This allows gosnmp to derive correct localized keys for both directions.
- `AuthoritativeEngineID` in GoSNMP's `UsmSecurityParameters` is a Go `string` treated as raw bytes; all interfaces pass raw `[]byte` and convert with `string()`.

**Validation after pass 5:**
- `go test ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`: PASS
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 240s`: PASS
- `go test -race ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 360s`: PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...`: PASS
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json`: PASS
- `git diff --check`: PASS
- `.agents/sow/audit.sh`: PASS with the pre-existing non-project skill classification warning

**Gaps/risks:**
- v3 INFORM receiver-local engine ID discovery Report (RFC 3414 §4) for empty/invalid receiver engine ID is not implemented in SOW-0036. Generated local engine IDs are stable and visible in the state file, but they are not automatically discoverable by senders until SOW-0038 implements the Report responder.
- Real-device or protocol-simulator v3 INFORM validation across plugin restart remains pending before merge gate. Synthetic authPriv INFORM request decode and authenticated/encrypted Response generation are covered by unit tests.

### 2026-05-26 (review round 5 and implementation pass 6)

Pass-5 follow-up reviewers flagged creation-time state failure, hot-path allocation, and v3 INFORM authPriv Response coverage. Findings were triaged as follows:

- Fixed: corrupt persisted `engine-boots` content now fails `NewEngineBoots()` and therefore fails job creation/DynCfg apply. It no longer silently resets to `1`, preserving the user's requirement that state failures surface at apply time.
- Fixed: `sendInformResponse()` now reads `snmpEngineBoots` and `snmpEngineTime` through one locked `EngineBoots.Snapshot()` call, avoiding split reads in the Response PDU.
- Fixed: `localEngineIDMatches()` compares raw engine-ID bytes through `LocalEngineID.EqualRaw()` instead of allocating hex strings on every v3 INFORM.
- Fixed: `ClassifyDecodeError()` now documents the temporary string-match dependency on GoSNMP plain errors.
- Fixed: partial v3 preflight failures clean newly-created engine state files while preserving pre-existing state files. The v3 init sequence now builds the security table and whitelist before mutating receiver-local state, then creates/registers the local engine ID, and creates `engine-boots` last.
- Fixed: `config_schema.json` now allows explicit empty `local_engine_id: ""`, matching Go validation and the omitted/generated behavior.
- Documented: `dropped_allowlist` is the umbrella policy-drop dimension for source CIDR, community, and disabled SNMP versions.
- Added tests: v3 INFORM authPriv (`sha256` + `aes`) request decode through the double-registered USM table, followed by authenticated/encrypted Response generation and decode using the receiver-local engine ID.
- Added tests: failed INFORM response increments `inform_response_failed`; auth/priv decode failure from an untrusted sender engine ID is reclassified to `unknown_engine_id`; native IPv6 source CIDR allowlist passes; partial v3 init failure removes newly-created local engine ID while preserving pre-existing state paths.
- Updated tests: corrupt `engine-boots` state now asserts creation-time failure instead of self-repair.
- Updated spec: §12 now explicitly states that SOW-0036 emits `snmp.trap.events` and `snmp.trap.errors` with `job_name` only; richer per-device labels remain the SOW-0037/SOW-0039 enrichment closeout.
- Rejected reviewer finding: duplicate USM registration does not overwrite sender engine IDs in this GoSNMP fork. Evidence: `github.com/ilyam8/gosnmp @ 388b2cb5192e`, `v3_map.go:26` appends security parameters per username, and `trap.go:481`/`:486` iterates every matching entry until one decodes successfully.
- Rejected reviewer finding: generated local engine IDs cannot be all `0xff` because generation clears the high bit of the first byte before validation/persistence; persisted configured values still reject all-`ff` through `parseEngineIDHex()`.

Validation after pass 6:

- `go test ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`: PASS
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 240s`: PASS
- `go test -race ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 360s`: PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...`: PASS
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json`: PASS
- `git diff --check`: PASS

### 2026-05-26 (review round 6 and implementation pass 7)

Follow-up reviewers rechecked the same full scope after pass 6:

- `glm`: no blocking issues; flagged state-directory preservation and peer-IP normalization polish that was already fixed in pass 6.
- `qwen`: no blocking issues; flagged the same low-risk state cleanup and hot-path allocation items already fixed in pass 6.
- `minimax`: no actionable blocker; remaining notes were stale against pass-6 code or outside SOW-0036 scope.
- `kimi`: found three real items in current code: SNMP engine time was capped with the RFC 3414 `snmpEngineBoots` maximum instead of the unsigned 32-bit `snmpEngineTime` maximum; the nil-cleanup test actually used a non-nil local engine ID; and decode-budget enforcement did not include the `packetVarbinds()` conversion work.

Pass-7 fixes:

- Fixed: split `maxSnmpEngineBoots` (`2147483647`, RFC 3414 range) from `maxSnmpEngineTime` (`4294967295`, unsigned 32-bit seconds) and added a test proving engine time caps at the unsigned 32-bit value.
- Fixed: split cleanup tests into `TestCollectorCleanupWithLocalEngineID` and a true `TestCollectorCleanupNilLocalEngineID`.
- Fixed: `DecodeTrap()` now checks the per-PDU decode budget after `packetVarbinds(pkt)` so bounded varbind conversion is included in the budget.
- Accepted as non-blocking in pass 7: decode-error string matching remains documented and covered by tests until the GoSNMP fork exposes typed errors; the decode-error sender-engine fallback parse is restricted to security-related classifications and is not on the valid-packet hot path.

Validation after pass 7:

- `go test ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`: PASS
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 240s`: PASS
- `go test -race ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 360s`: PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...`: PASS
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json`: PASS
- `git diff --check`: PASS
- `.agents/sow/audit.sh`: PASS with the pre-existing non-project skill classification warning

### 2026-05-26 (review round 7 and implementation pass 8)

Follow-up reviewers rechecked the same full scope after pass 7:

- `glm`: no blockers; only low/non-blocking micro-optimization notes.
- `qwen`: no blockers; only informational notes for deferred dimensions and closed metric switches.
- `minimax`: first run returned only a progress summary, so it was rerun with the same scope and a neutral "provide final review only" addendum. The rerun reported no blockers.
- `kimi`: found one real medium issue. A v3 packet sent to a v1/v2c-only job reached GoSNMP without a v3 security table, failed before the normal decoded-version policy check, and was counted as `auth_failures` instead of the policy-drop dimension `dropped_allowlist`.

Pass-8 fixes:

- Fixed: `handlePacket()` now performs a cheap BER version peek before full decode. If the SNMP version is recognized but disabled for the job, the packet increments `dropped_allowlist` and returns before GoSNMP v3 authentication is attempted.
- Added test coverage: a v3 packet sent to a v2c-only job now proves `dropped_allowlist == 1` while `auth_failures == 0` and `decode_failed == 0`.
- Fixed low-risk reviewer note: USM duplicate detection now uses a typed `{username, engineID}` map key instead of delimiter-based string concatenation.
- Fixed low-risk reviewer note: removed the redundant reserved-label prefix that was already impossible because label keys must start with a lowercase letter.
- Accepted as non-blocking: `profile_load_failed` remains reserved for SOW-0037 hot-reload failures; package-level metric helpers are retained because tests use the same package and the production hot path still uses collector methods.

Validation after pass 8:

- `go test ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`: PASS
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 240s`: PASS
- `go test -race ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 360s`: PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...`: PASS
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json`: PASS
- `git diff --check`: PASS
- `.agents/sow/audit.sh`: PASS with the pre-existing non-project skill classification warning

### 2026-05-26 (review round 8 and implementation pass 9)

Follow-up reviewers rechecked the same full scope after pass 8:

- `glm`: no blockers; only low/non-blocking observations around string-matched decode-error classification, large allowlist optimization, and optional sampled drop logging.
- `qwen`: no blockers; only low/cosmetic observations around naming and reserved metric dimensions.
- `minimax`: returned a progress summary instead of a final review and was treated as incomplete.
- `kimi`: no blocker, but flagged a medium test coverage gap. A v2c packet sent to a job that also has v3 enabled should still decode correctly with a non-nil v3 security table.

Pass-9 fixes:

- Fixed: `decodePacket()` now forces GoSNMP v3 mode only when the packet's own BER version is v3. A non-nil v3 security table no longer changes the decoder mode for v1/v2c packets.
- Added test coverage: `TestDecodeTrapV2cWithV3SecurityTable` verifies a v2c trap decodes as v2c when a v3 security table is present.
- Accepted as non-blocking: `sendInformResponse()` remains nil-safe for defensive caller behavior; the production caller already checks prerequisites before calling it.
- Accepted as non-blocking: shutdown drain behavior continues after a write failure so later queued entries still have a chance to persist.
- Accepted as non-blocking: BER helper consolidation and decode-error typed errors remain cleanup work outside the SOW-0036 behavior gate.

Validation after pass 9:

- `go test ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`: PASS
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 240s`: PASS
- `go test -race ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 360s`: PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...`: PASS
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json`: PASS
- `git diff --check`: PASS
- `.agents/sow/audit.sh`: PASS with the pre-existing non-project skill classification warning

### 2026-05-26 (review round 9 and implementation pass 10)

Follow-up reviewers rechecked the same full scope after pass 9:

- `glm`: no blockers; only low/non-blocking observations around string-matched decode-error classification, O(n) CIDR allowlist scan, and mutable test budget variable.
- `qwen`: no blockers; only low/non-blocking observations around sample-mode source-cap semantics, defensive version recheck, reserved `profile_load_failed` dimension, and conservative state-path existence checks.
- `minimax`: no blockers; raised low observations around the global job-name metrics map and the source-cap behavior. The source-cap note incorrectly described a hardcoded drop path; the code returns the configured mode.
- `kimi`: no blocker, but found a real medium spec mismatch. Operator override labels still rejected reserved prefixes such as `trap_`, `message`, and `priority`, while spec §7.5 now says the `TRAP_TAG_*` journal namespace makes only `[a-z][a-z0-9_]*` syntax validation necessary.

Pass-10 fixes:

- Fixed: removed the obsolete reserved-prefix validation from `validateConfigLabelKey()`.
- Added test coverage: config label validation now accepts `trap_zone`, `message_type`, and `priority_level` while still rejecting uppercase keys.

Validation after pass 10:

- `go test ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`: PASS
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 240s`: PASS
- `go test -race ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 360s`: PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...`: PASS
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json`: PASS
- `git diff --check`: PASS
- `.agents/sow/audit.sh`: PASS with the pre-existing non-project skill classification warning

Review cadence decision:

- User clarified that external reviews should cover meaningful implementation batches, not tiny one- or two-line fixes. Small follow-up fixes should get local review and validation; `glm`, `kimi`, `minimax`, and `qwen` should be reserved for larger milestone batches, final close/commit gates, or high-risk changes.
- A micro-review round launched after pass 10 was stopped under this decision. `glm` had already returned no blockers; the other reviewers were intentionally terminated before final findings. No further external review is required for the isolated label-validation fix unless it becomes part of a larger close-gate review.

## Validation

Acceptance criteria evidence: M1-M4 implemented per execution log. M1 v3 INFORM local authoritative engine ID resolved in pass 5 and strengthened in pass 6 with authPriv Response coverage and creation-time corrupt-state failure. Pass 7 fixes the remaining SNMP engine-time cap and decode-budget accounting issues found by review. Pass 8 fixes v3-on-v1/v2c policy-drop classification. Pass 9 proves v1/v2c decode still works when a job also has a v3 security table. Pass 10 fixes the operator-label validation spec mismatch. Final acceptance remains pending until the next meaningful external review batch or close/commit gate.

Tests or equivalent validation: focused and broader go test suites, race detector, go vet, schema validation, diff check, and SOW audit pass after pass 10.

Real-use evidence: pending — v3 INFORM persistence must be tested against a real device or protocol simulator across plugin restart before merge gate. Synthetic v3 noAuth, SHA-2 auth, AES privacy, INFORM engine ID split, authPriv Response, and local engine ID persistence paths are covered by unit tests.

Reviewer findings: rounds 1-9 from `glm`, `kimi`, `minimax`, and `qwen` recorded in the execution log. Pass 6 fixes the real pass-5 findings and rejects the false USM-overwrite finding with GoSNMP fork evidence. Pass 7 fixes the real pass-6 findings from `kimi`. Pass 8 fixes the real pass-7 version-policy classification finding from `kimi`. Pass 9 fixes the real pass-8 mixed-version decode coverage gap from `kimi`. Pass 10 fixes the real pass-9 operator-label validation mismatch from `kimi`. Per user review-cadence decision, the next external review should run on a larger batch or close/commit gate, not on the isolated pass-10 label-validation fix.

Same-failure scan: no similar failure patterns found in other collectors; snmp_traps is the only trap subsystem implementation.

Sensitive data gate: PASS. No secrets, communities, USM keys, device IDs, or customer IPs in committed artifacts.

Artifact maintenance gate:
- AGENTS.md: no changes needed
- Runtime project skills: project-snmp-trap-profiles-authoring still authoritative; no updates needed
- Specs: `.agents/sow/specs/snmp-traps/netdata.md` updated for invalid v3 USM passphrase creation-time failures, current community allowlist behavior, optional/generated `local_engine_id`, v3 Trap sender engine ID vs v3 INFORM receiver-local engine ID semantics, and creation-time state-file failure behavior.
- End-user docs: deferred to SOW-0039
- SOW lifecycle: SOW-0036 paused/implementation-complete; SOW-0035 paused; SOW-0037, SOW-0038, SOW-0039 pending. SOW-0038 updated to track the v3 INFORM receiver-local engine ID Report responder gap.

## Outcome

Implementation pass 10 complete. Option A (`local_engine_id` optional, generated/persisted default) is implemented. M1 v3 INFORM local authoritative engine ID semantics are split from v3 Trap sender engine ID whitelist, and authPriv INFORM Response is unit-tested. Version-policy drops are classified consistently before full decode, mixed v1/v2c/v3 jobs retain v1/v2c decode correctness, and operator override labels match spec §7.5. The SOW is paused, not terminal-completed, because final collector consistency and merge readiness are owned by SOW-0039. Future external reviews should run on meaningful batches or the close/commit gate.

## Lessons Extracted

- GoSNMP's `TrapSecurityParametersTable` requires `Version` set to `Version3` on the decoder instance; the table alone is not enough.
- GoSNMP v3 trap encode needs `SnmpEncodePacket` (not `MarshalMsg`) for test fixture generation.
- metrix API (`SnapshotMeter`, `SnapshotCounter`, `ObserveTotal`) differs from the simpler `store.Set` approach; cold-path metric emission follows the framework V2 pattern.

## Followup

Deferred work is enumerated in the execution log above and owned by SOW-0037, SOW-0038, or SOW-0039. Full spec §12 per-device/vnode metric labels are explicitly handed to SOW-0037/SOW-0039 because enrichment and collector consistency live there.

## Regression Log

None yet.
