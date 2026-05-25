# SOW-0036 - SNMP Trap Plugin: SNMPv3 + INFORM + snmpEngineBoots + Per-Job Allowlist/Rate-Limit + BER Limits + DynCfg Schema + Self-Metrics

## Status

Status: open

Sub-state: queued in `.agents/sow/pending/`. Depends on SOW-0035 completion. NOT independently mergeable — merge gate is SOW-0039.

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

Sequential after SOW-0035 per the user-approved 5-SOW lineup. Per user: "We need to add metadata.yaml and the rest — this can be done at the end as an extra SOW" — bundle moved to SOW-0039.

### Assistant Understanding

Facts:

- Per spec §5, each job has its own `snmpEngineBoots` counter persisted at `/var/lib/netdata/snmp-trap/{job_name}/engine-boots`. Without persistence, devices that cached the old boot counter reject INFORM Response PDUs as replay attacks after every Netdata restart (RFC 3414 §2.2.2).
- Per spec §6, INFORM Response is sent synchronously from the receive socket immediately after BER decode + auth; sender retransmits per RFC 3414 §3 handle UDP loss.
- Per spec §18, BER decode limits apply **per-job** with identical default values across all jobs (each job's decoder enforces independently against its own UDP buffer — "global" means uniform defaults, NOT a single shared decoder state). Per spec §12, the `snmp.trap.errors` dimensions are: `unknown_oid, decode_failed, template_unresolved, malformed_pdu, dropped_allowlist, rate_limited, auth_failures, usm_failures, unknown_engine_id, inform_response_failed, sanitized, profile_load_failed, journal_write_failed`. All metrics carry `job_name` as a label.
- SNMP communities and USM keys must be sourced from the existing Netdata Secrets infrastructure (no raw keys in any committed artifact).
- Per spec §5, SNMPv3 dynamic engineID discovery is opt-in and deferred to SOW-0038; this SOW ships only the static whitelist mode.

Inferences:

- The plugin-self metric universe drives `metadata.yaml` content (SOW-0039); the metric set must be settled in M4 before SOW-0039 can author `metadata.yaml`.

Unknowns:

- Default rate-limit values (settled at M2 validation — likely `1000 traps/sec/source` initial).

### Acceptance Criteria

- M1: SNMPv3 USM works for SHA-224/SHA-256/SHA-384/SHA-512 auth + AES-128/192/256 priv against a static engineID whitelist per job; INFORM acknowledgement returns proper Response PDU synchronously on the receive socket; `snmpEngineBoots` persists to `/var/lib/netdata/snmp-trap/{job_name}/engine-boots` across plugin restarts.
- M2: pre-decode source-IP CIDR allowlist drops unwanted traffic before any BER parse; community-string allowlist (v1/v2c); per-source token-bucket rate-limit (drop or sample mode); BER decode limits per spec §18 enforced with counter increments on violation.
- M3: per-job plugin configuration loadable from `/etc/netdata/go.d/snmp.trap.conf`; DynCfg integration handles Add/Update/Enable/Disable/Remove per job; bind-or-fail surfaces in DynCfg as HTTP-422.
- M4: plugin-self NIDL metrics emit at 1Hz cold-path with hot-path being counter-increment only (zero allocation). Per spec §12: `snmp.trap.events` has **instance = per device** (the source device vnode), **dimensions = 8 category slugs** (`state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, `unknown`), **labels** = `job_name`, `severity`, `device`, `vendor`, `hub`, plus operator-defined labels from profile/plugin config. `snmp.trap.errors` has the full dimension universe from spec §12 (13 dimensions) with labels `job_name`, `hub`. `snmp.trap.dedup_suppressed` is owned by SOW-0037.

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

Reviewers: all 7 — auth/crypto critical.

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

Reviewers: 3 rotating (group A: kimi/qwen/minimax).

### M3 — Plugin configuration schema + DynCfg per-job orchestration

- `/etc/netdata/go.d/snmp.trap.conf` schema per spec §7.5 (per-job structure with `listen`, `versions`, `communities`, `usm_users`, `engine_id_whitelist`, `allowlist`, `rate_limit`, `dedup`, `retention`, `overrides`, `metrics`).
- DynCfg integration mirroring `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go`:
  - Add/Enable creates job, attempts bind. Failure → HTTP-422 coded error.
  - Update stops old job, recreates with new config.
  - Disable/Remove stops job, closes socket, retains journal directory.
- Validation at load: reject unknown keys, malformed CIDRs, invalid USM proto names, invalid label-key characters or reserved prefixes per spec §7.5.

Cohort reference: existing go.d collector config patterns; `src/go/plugin/agent/jobmgr/dyncfg_collector.go` (DynCfg routing).

Reviewers: 3 rotating (group B: glm/mimo/deepseek).

### M4 — Plugin-self NIDL metrics (per-job dimensions)

- Hot path: atomic counter increments only (zero heap allocation in trap-handling code path).
- Cold path: 1Hz sweep of per-job counter maps → PLUGINSD `BEGIN`/`SET`/`END` on stdout.
- Metric universe per spec §12:
  - `snmp.trap.events` (instance: per source device; dimensions: 8 category slugs `state_change`/`config_change`/`security`/`auth`/`license`/`mobility`/`diagnostic`/`unknown`; labels: `job_name`, `severity`, `device`, `vendor`, `hub`, plus operator-defined labels from profile/plugin config).
  - `snmp.trap.errors` (instance: per job; dimensions: `unknown_oid`, `decode_failed`, `template_unresolved`, `malformed_pdu`, `dropped_allowlist`, `rate_limited`, `auth_failures`, `usm_failures`, `unknown_engine_id`, `inform_response_failed`, `sanitized`, `profile_load_failed`, `journal_write_failed`; labels: `job_name`, `hub`, `source_device` when source is identifiable).
- `snmp.trap.dedup_suppressed` is owned by SOW-0037 (only emitted when opt-in dedup is enabled).

Cohort reference: spec §12; existing collector NIDL patterns (`project-writing-collectors` skill).

Reviewers: 3 rotating (group A: kimi/qwen/minimax).

## Reviewer Protocol

- M1: all 7 reviewers (auth/crypto critical).
- M2 + M3 + M4: 3 rotating reviewers per round.
- Fix-cycle: same reviewers as the round being fixed.

## Pre-Implementation Gate

Status: blocked

Reason: depends on SOW-0035 completion. Full gate filled at activation — the gate's evidence depends on SOW-0035's resolved language/process-model choice and the journal writer's actual field implementation.

## Plan

Sequential M1 → M4. The collector consistency bundle that AGENTS.md mandates is NOT in this SOW; it is owned by SOW-0039. SOW-0036 is one of four SOWs (0035-0038) landing on a feature branch until SOW-0039 produces the bundle and a single PR sequence merges.

## Execution Log

### 2026-05-25

SOW rewritten under the 5-SOW lineup: M5 consistency bundle removed (moved to SOW-0039); M1 expanded to include `snmpEngineBoots` persistence; M2 adds BER decode limits per spec §18; M4 expanded to full metric universe per spec §12. Not yet activated.

## Validation

Acceptance criteria evidence: pending.
Tests or equivalent validation: pending.
Real-use evidence: pending — v3 INFORM persistence must be tested against a real device (or test fixture) across plugin restart.
Reviewer findings: pending.
Same-failure scan: pending.
Sensitive data gate: pending — USM keys + community strings must NOT land in any committed artifact (Secrets refs only).
Artifact maintenance gate: pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
