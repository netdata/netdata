# SOW-0049 - SNMP Traps Decode Error Journal Entries

## Status

Status: completed

Sub-state: user approved replacing the earlier summary-only idea with
per-failure forensic journal rows. Implementation and focused validation are
complete.

## Requirements

### Purpose

Write accepted-source SNMP trap decode failures to the trap output path as
individual forensic events, so operators can inspect malformed or unauthorized
UDP traffic in `snmp:traps` instead of seeing only aggregate metrics.

### User Request

The user clarified that decode failures should be logged one by one with
bounded details about what failed. These are received UDP frames that pass the
early source policy but cannot be decoded as valid accepted traps.

### Acceptance Criteria

- Decode failures write one journal/log entry per failed datagram after source
  allowlist and configured rate-limit policy.
- Decode-error entries use `TRAP_REPORT_TYPE=decode_error`.
- Decode-error entries include bounded/sanitized details: source IP/UDP peer,
  listener endpoint when known, sniffed SNMP version when known, packet size,
  packet fingerprint/hash, failure class, and sanitized decoder error text.
- Decode-error entries MUST NOT write raw packet bytes by default, because SNMP
  v1/v2c community strings and binary payloads can appear in the datagram.
- Existing decode-error metrics remain intact.
- Tests prove journal serialization, hot serializer parity, packet-path write,
  allowlist behavior, and configured rate-limit behavior.

## Pre-Implementation Gate

Status: completed

Problem / root-cause model:

- Decode failures are visible as metrics, but not as forensic journal rows.
- The existing `ReportTypeDecodeErrorSummary` is a reserved summary-only idea
  and is not the desired clean end state for the approved design.
- Current packet flow applies source allowlist before decode, but configured
  rate limiting is mostly after successful decode. Decode-error journaling must
  therefore explicitly honor configured rate limiting before writing a
  decode-error entry.

Evidence reviewed:

- `src/go/plugin/go.d/collector/snmp_traps/collector.go`: decode failures are
  classified and counted, then returned without writing.
- `src/go/plugin/go.d/collector/snmp_traps/decode.go`: `ClassifyDecodeError`
  maps errors to bounded dimensions.
- `src/go/plugin/go.d/collector/snmp_traps/ratelimit.go`: existing per-source
  rate limiter supports `drop` and `sample` modes.
- `src/go/plugin/go.d/collector/snmp_traps/serialize.go`: both reference and
  hot serializers own the journal field schema.
- `src/go/plugin/go.d/collector/snmp_traps/func_logs.go` and
  `src/collectors/systemd-journal.plugin/systemd-journal.c`: trap fields are
  exposed as logs Function/systemd-journal facets.

Affected contracts and surfaces:

- Journal schema, OTLP export payloads, embedded `snmp:traps` Function facets,
  systemd-journal plugin trap facets, metrics, health metadata, generated docs,
  query skill, and tests.

Existing patterns to reuse:

- Existing trap writer path and serializers.
- Existing bounded error dimensions.
- Existing per-source rate limiter.
- Existing collector consistency workflow: update metadata and regenerate the
  integration page when journal fields or operator docs change.

Risk and blast radius:

- Per-failure rows can amplify hostile UDP floods into disk writes. Mitigation:
  honor configured source allowlist first and configured per-source rate limit
  before writing decode-error entries.
- Raw packet bytes may contain SNMP v1/v2c community strings or binary data.
  Mitigation: write packet size and SHA-256 fingerprint, not packet bytes.
- Decoder error text may contain controls or long strings. Mitigation: strip
  controls/newlines and truncate to a bounded length.

Sensitive data handling plan:

- Tests use synthetic malformed packets and documentation-safe private IPs.
- Do not store raw datagram bytes or secrets in durable artifacts.

Implementation plan:

1. Replace the unused summary-only report type with `decode_error`.
2. Extend `TrapEntry` and serializers with decode-error fields.
3. Add a decode-error entry builder that records source, listener, version,
   packet size, SHA-256 fingerprint, bounded failure class, and sanitized error.
4. Write the decode-error entry on decode failure after metrics are incremented
   and after configured rate-limit policy is applied.
5. Update Function/systemd-journal facets, metadata, generated docs, and query
   skill wording.
6. Add unit tests for serialization, packet-path write, allowlist skip, and
   rate-limit behavior.

Validation plan:

- Focused Go tests for `snmp_traps`.
- Serializer parity test between reference and hot serializers.
- Integration generation after metadata changes.
- `git diff --check`.

Artifact impact plan:

- AGENTS.md: no expected change.
- Runtime project skills: no expected change.
- Specs: update SNMP traps journal schema/report type spec if durable field
  list changes.
- End-user/operator docs: update generated integration docs through
  `metadata.yaml`.
- End-user/operator skills: update `query-snmp-traps` if field/query behavior
  changes.
- SOW lifecycle: close after implementation, validation, and handoff cleanup.

Open-source reference evidence:

- `splunk/splunk-connect-for-snmp @ fdd4c74ef3cc8295675039be9f432b00e48b96d8`
  logs ASN.1 decode failures and security model failures for trap input:
  `splunk_connect_for_snmp/traps.py:119`, `splunk_connect_for_snmp/traps.py:145`,
  `splunk_connect_for_snmp/traps.py:297`.
- `librenms/librenms @ 522da5e58bc2c50ed2847a8f974f101f551e547d`
  logs operator-visible trap processing failures and supports generic trap
  event logging: `LibreNMS/Snmptrap/Dispatcher.php:42`,
  `LibreNMS/Snmptrap/Dispatcher.php:48`,
  `doc/Extensions/SNMP-Trap-Handler.md:179`.

Open decisions:

- None blocking. User approved per-failure decode-error journal rows on
  2026-06-10.

## Followup

None yet.

## Regression Log

- Implemented `TRAP_REPORT_TYPE=decode_error` entries for accepted-source decode
  failures.
- Preserved `TRAP_SOURCE_UDP_PEER` as normalized address-only and added
  `TRAP_SOURCE_UDP_PORT` for the UDP source port.
- Added regression coverage for:
  - journal serialization and hot serializer parity;
  - packet-path decode-error write;
  - source allowlist skip;
  - configured rate-limit drop mode;
  - IPv4-mapped source normalization;
  - OTLP decode-error serialization.
- Updated durable trap specs, TrapWriter ADR, `metadata.yaml`, generated
  integration docs, systemd-journal trap field allowlist, and the
  `query-snmp-traps` skill.
- Validation:
  - `go test -count=1 ./plugin/go.d/collector/snmp_traps`
  - `go test -count=1 ./plugin/go.d/collector/snmp_traps ./cmd/godplugin ./plugin/agent/jobmgr/funcctl`
  - `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps`
  - `git diff --check`
