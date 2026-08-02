# SNMP Trap Pipeline Internals (developer-facing)

Status: accepted (extracted from operator docs cleanup).

This spec records the developer-internal mechanics of the SNMP trap pipeline
that operators do NOT need to run Netdata, and that were therefore removed or
rephrased out of the operator documentation under
`docs/snmp-traps/`. It is an index: where a mechanic already has an
authoritative home in `netdata.md` or `trap-metrics-profiles.md`, this file
points there instead of duplicating it. Only the per-source rate-limit cap
eviction is recorded here in full, because no other spec covered it.

The operator-observable effect of each mechanic stays in the operator docs (and,
where relevant, the metric to watch). Only the mechanism lives here.

## 1. Per-source rate-limit cap and bucket eviction (authoritative)

The per-job `rate_limit:` knob (`netdata.md` §7.5, default off) is a token
bucket per source IP, `per_source_pps` tokens/second.

Internal cap and eviction mechanics (NOT operator-configurable, NOT in operator
docs beyond the observable effect):

- The job tracks up to **10,000 active source buckets**. The cap is fixed.
- An idle bucket expires after **10 minutes** of no traffic.
- Under high source churn, when the cap is reached, the **oldest bucket is
  evicted** before a new source is rejected.
- Each bucket **starts full**, so a source that has been idle (or newly seen)
  can send an initial burst of up to `per_source_pps` traps before limiting
  takes effect.

Operator-observable effect retained in `docs/snmp-traps/configuration.md`
("Rate limiting"): the 10,000-source cap and the initial-burst behavior. The
eviction/idle-expiry/starts-full mechanics are intentionally omitted there.

## 2. BER decode hard limits

Authoritative home: `netdata.md` §18 ("BER decode resource limits").

Per-trap hard limits enforced on the untrusted UDP-delivered ASN.1 BER (max
datagram bytes, max varbinds per PDU, max constructed nesting depth, max OID
encoded length, max OctetString value length). Exceeding any limit drops the
trap and increments the malformed-PDU error metric.

Operator-observable effect retained in the operator docs: the allowlist is
checked "before the packet is parsed"; malformed packets increment the
processing-error metrics. The specific limit values and the BER/parser
terminology stay out of the operator docs.

## 3. Journal-writer queue depth and flush mechanics

Authoritative home: `netdata.md` §19 ("Output writer interface contract", default
queue/flush policy) and §11 ("Journal Storage").

- The journal-direct writer accepts entries into a per-job bounded queue
  (default 10,000 entries); queue-full and permanent writer failure are
  drop-and-continue errors.
- The writer fsyncs every 1 second on a ticker, and on `Flush()` / `Close()`.
  There is no count-based flush (the `defaultFlushEntries = 1000` trigger was
  removed; see `decisions/0001-go-process-and-trapwriter.md`).

Operator-observable effect retained in `docs/snmp-traps/sizing-and-capacity.md`:
the durable write path is the throughput ceiling; sustained overload rejects
traps and increments `journal_write_failed` / `write_failed`; a once-per-second
flush means an abrupt power loss can lose up to the last second, while a clean
restart loses nothing. The "single thread" and "bounded backlog queue" framing
is omitted there.

## 4. Commit ordering and no-rollback semantics

Authoritative home: `netdata.md` §12 ("Commitment and attribution rules") and
§19 (writer ownership / non-blocking `Write`).

- `accepted` and job-level processing error counters are recorded before dedup
  suppression.
- `committed`, category/severity counters, and profile-defined metrics are
  recorded only after successful authoritative output commitment.
- When both journal and OTLP are enabled, the journal-direct path is the
  authoritative commitment path; OTLP failures are export failures and do not
  roll back metrics already updated from the authoritative journal write.
- When OTLP is the only backend, OTLP export failure is a terminal write
  failure.

Operator-observable effect retained in `docs/snmp-traps/configuration.md`,
`trap-profiles.md`, and `metrics.md`: only committed traps update
profile metrics; dedup-suppressed and failed-write traps do not; the journal is
authoritative in dual mode and an OTLP failure can briefly leave journal,
metrics, and OTLP stream out of step. The "accepted into the writer" /
"queued for export" / "enqueue ordering" / "roll back" mechanics are omitted.

## 5. OTLP export queue, retry, and durability

Authoritative home: `netdata.md` §11b ("OTLP Exporter Attribute Universe") and
§19 (OTLP backend batching: default flush window, enqueue-and-return `Write`).

- Records are batched (`batch_size`, `flush_interval`) and a failed batch is
  retried on each later flush interval, with no max retry count and no backoff,
  until the receiver accepts it or the process stops.
- When both backends are enabled, the record is queued for the journal backend
  before the OTLP export, so an OTLP failure does not remove records already
  accepted by the journal backend.
- The in-memory OTLP queue (`queue_capacity`) is not durable; records still
  queued are lost if the process exits before they are exported (ungraceful
  restart or failed shutdown drain).
- In OTLP-only mode a queue-full drop is a terminal write failure and the trap
  is lost.

Operator-observable effect retained in `docs/snmp-traps/forwarding-to-siem.md`:
exports are batched and transient failures recover on their own; queue-full
drops are counted under `otlp_export_failed`; in journal+OTLP mode the local
journal is unaffected by OTLP drops; in OTLP-only mode a dropped record is lost
and OTLP-only must not be treated as durable storage. The queue-ordering,
no-backoff, and shutdown-drain mechanics are omitted.

## 6. Journal filename layout and source tagging

Authoritative home: `netdata.md` §11 ("Journal Storage — per-job journal
directories").

- Journal filenames use the `snmp-traps` source prefix with chain naming and an
  at-sign separator.
- Individual entries carry `ND_LOG_SOURCE=snmp-trap`.
- Generated `TRAP_VAR_*` / `TRAP_TAG_*` field names obey the journald 64-byte
  field-name limit; over-length names keep a readable prefix and append a stable
  hash suffix, with full provenance in `TRAP_JSON`.

Operator-observable effect retained in the operator docs: entries carry
`ND_LOG_SOURCE=snmp-trap` (filter on it); the files are journal-compatible files,
not the host journald journal; long field names are shortened with a hash suffix
and the full value is in `TRAP_JSON`. The chain-naming / at-sign / "to fit
journal field-name limits" framing is omitted.

## 7. Profile loading lifecycle

Authoritative home: `netdata.md` §7 ("Profile loading — leased catalog epochs,
manifest routes, targeted hydration").

- One catalog manager belongs to the plugin registration. Its first listener
  lease creates an epoch shared by all listeners; the final lease drops it.
- Operator profiles and the stock manifest load eagerly in `Collector.Init()`;
  `Collector.Check()` is a no-op. Stock bodies hydrate through exact OID and
  metric-rule routes or through the candidate-file list for a MIB-qualified
  name.
- Every stock manifest entry carries a SHA-256 over the exact decompressed YAML
  bytes. Lazy hydration verifies and parses the same bytes, binding a body to
  the epoch that indexed it; the digest is not an authenticity signature.
- Profiles are immutable while the shared epoch has active job leases.
  Operator and stock changes apply after an Agent restart or after every trap
  job is recreated.

Operator-observable effect retained in `docs/npm/snmp-traps/trap-profiles.md`:
profile changes require an Agent restart or recreation of every trap job.
Invalid operator profiles fail the next job creation; invalid lazy stock
profiles fail their first matching lookup and increment profile-load-failure
metrics.
