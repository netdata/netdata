<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/snmp-traps/forwarding-to-siem.md"
sidebar_label: "Forwarding to SIEM"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'siem', 'forwarding', 'otlp', 'journal', 'logs']
endmeta-->

<!-- markdownlint-disable-file -->

# Forwarding to SIEM

Use this page when you operate an existing log or SIEM platform and want SNMP trap events from Netdata to reach it with predictable field names, transport behavior, and validation signals.

For listener setup and output options, see [Configuration](/docs/snmp-traps/configuration.md). For the complete field list, see [Field Reference](/docs/snmp-traps/field-reference.md).

## Supported forwarding methods

Netdata supports two forwarding methods for SNMP trap logs:

1. **Direct journal backend.** Netdata writes decoded traps to local journal-compatible files under the Netdata log directory. You can read or export those files with `journalctl --directory=...`, then pipe or ship the output with the journal-aware tooling already used by your log pipeline.
2. **OTLP/gRPC export.** Netdata pushes decoded traps as OTLP LogRecords to an OTLP Logs receiver, such as an OpenTelemetry Collector or another log platform that accepts OTLP/gRPC.

The two methods can be used independently or together. Choose based on where operators need the system of record and whether local journal querying must remain available.

## Choose the backend mode

The `snmp:traps` Function requires a Netdata Cloud connection. Jobs that write local journal files expose trap log sources through this Function.

| Mode | Configuration | Cloud-required Function source | Local journal files | Use when |
|---|---|---:|---:|---|
| Journal only | `journal.enabled: true`, `otlp.enabled: false` | yes | yes | Local `journalctl` querying or an existing journal-based shipper is the forwarding path. |
| Journal and OTLP | `journal.enabled: true`, `otlp.enabled: true` | yes | yes | Operators need local forensic access, Netdata Cloud Logs access, and a downstream OTLP log platform. |
| OTLP only | `journal.enabled: false`, `otlp.enabled: true` | no | no | The external OTLP receiver is the intended system of record and local trap log files are not needed. |

At least one output backend must stay enabled. If you disable direct journal output, enable OTLP export or the job fails validation.

OTLP-only jobs create no local trap journal files and do not appear as job sources in the Cloud-required `snmp:traps` Function that Netdata Cloud uses for SNMP Trap Logs. Use OTLP-only mode only when the downstream OTLP receiver is the intended system of record.

## Direct journal backend

Direct journal output is enabled by default for explicit trap jobs and requires Linux. On non-Linux systems, jobs with `journal.enabled: true` fail validation; use OTLP-only mode instead.

```yaml
journal:
  enabled: true
```

Netdata writes trap entries below the configured Netdata log directory. The per-job root is normally:

```text
/var/log/netdata/traps/<job>/
```

or, when `NETDATA_LOG_DIR` is set:

```text
${NETDATA_LOG_DIR}/traps/<job>/
```

The effective `journalctl --directory` path is the machine-id child inside that job root. The path segment uses the unhyphenated machine ID, so normalize it if needed:

```bash
MACHINE_ID=$(tr -d '-' < /etc/machine-id)
```

These files back the job source exposed through the Cloud-required `snmp:traps` Function when the Agent is connected to Netdata Cloud. Journal filenames use the `snmp-traps` source prefix with chain naming and an at-sign separator, while individual journal entries carry `ND_LOG_SOURCE=snmp-trap`. They are journal-compatible files, not the host systemd-journald service's normal journal.

The host must have `journalctl` installed. Use `sudo` when the trap journal files are not readable by your user. Use `journalctl --directory` to produce JSON output that your existing log shipper or SIEM ingestion path can consume:

```bash
JOB=edge-traps
MACHINE_ID=$(tr -d '-' < /etc/machine-id)
sudo journalctl \
  --directory="/var/log/netdata/traps/${JOB}/${MACHINE_ID}" \
  --since "1 hour ago" \
  --output=json \
  --no-pager
```

For local query examples, see [Journal and querying](/docs/snmp-traps/journal-and-querying.md).
For journal retention and rotation controls, see [Configuration](/docs/snmp-traps/configuration.md#direct-journal-retention).

## OTLP/gRPC export

OTLP export is disabled by default. Enable it under the job's `otlp` section. This example shows a remote collector; the default endpoint is the loopback address shown in the defaults table.

```yaml
otlp:
  enabled: true
  endpoint: "https://otel-collector.example.net:4317"
  headers:
    authorization: "${file:/run/secrets/snmp-trap-otlp-authorization}"
  request_timeout: 5s
  flush_interval: 200ms
  batch_size: 512
  queue_capacity: 10000
```

Defaults:

| Setting | Default |
|---|---|
| `endpoint` | `http://127.0.0.1:4317` |
| `headers` | `null` |
| `request_timeout` | `5s` |
| `flush_interval` | `200ms` |
| `batch_size` | `512` |
| `queue_capacity` | `10000` |

When `headers` is set, it is a gRPC metadata map. Header values can use secret references, and keys with the reserved `grpc-` prefix are rejected at job startup.

OTLP export uses gRPC transport only. HTTP/protobuf OTLP receivers, commonly exposed on port `4318`, are not supported by this output.

At startup, Netdata opens the OTLP connection and sends a preflight export. If the receiver is unreachable or rejects the preflight, a job defined in the configuration file does not start. A Dynamic Configuration apply is rejected with the same error. This is a startup error, not an `otlp_export_failed` metric.

Records are buffered until `batch_size` records are ready or `flush_interval` expires. If an export fails, Netdata retries the pending batch on later flushes. There is no maximum retry count and no backoff; the batch is retried on each later flush interval until the receiver accepts it or the process stops. If the in-memory queue reaches `queue_capacity`, new records are dropped from the OTLP path and counted under `otlp_export_failed`. In journal and OTLP mode, Netdata queues the record for the journal backend before it queues the OTLP export, so OTLP failures do not remove records already accepted by the journal backend. In OTLP-only mode, a queue-full drop is a terminal write failure and the trap is lost. The OTLP queue is not durable; records still queued are lost if the process exits before they are exported, such as an ungraceful restart or a failed shutdown drain.

Endpoint rules:

| Endpoint form | Transport |
|---|---|
| `host:port` | Plaintext gRPC |
| `http://host:port` | Plaintext gRPC |
| `https://host:port` | TLS gRPC with system trust roots, TLS 1.2 or later |

Paths, query strings, and fragments are not supported. Use `https://` for remote collectors when trap contents should be protected in transit. Plaintext loopback, such as the default `http://127.0.0.1:4317`, is intended for local collectors; non-loopback plaintext is allowed but logs a warning. `https://` uses the operating system trust store, so private CA certificates must be installed there. Invalid endpoints and header keys with the reserved `grpc-` prefix are rejected at job startup.

## OTLP mapping highlights

OTLP export sends traps as OTLP LogRecords. Build SIEM rules against the OTLP attribute names that arrive downstream, not the direct-journal field names.

| Netdata concept | OTLP location |
|---|---|
| Listener identity | Resource `service.name` is `netdata-snmptrap`; resource `service.instance.id` is the job name. |
| Message | Log body. This is equivalent to `MESSAGE`. |
| Report type | Attribute `snmp.trap.report_type`. Values are `trap`, `deduplication_summary`, and `decode_error`. |
| Event name | LogRecord event name. Normal traps use `snmp.trap.<category>`, dedup summaries use `snmp.trap.deduplication_summary`, and decode errors use `snmp.trap.decode_error`. |
| Severity | OTLP severity number/text plus attribute `snmp.trap.severity`. Emitted on normal trap and decode-error rows; dedup summary rows do not include this attribute. |
| Source identity | Attribute `snmp.source.ip`. This is the selected trap source IP: the UDP peer by default, or the relay-attributed device source when trusted relays are configured. If the selected source is unset, Netdata falls back to the UDP peer. |
| UDP peer | Attribute `network.peer.address`. This is the packet peer. If it is empty, Netdata falls back to the selected source IP. |
| UDP source port | Attribute `network.peer.port`. Decode-error rows only. |
| Trap identifiers | Attributes `snmp.trap.oid`, `snmp.trap.name`, `snmp.trap.category`, `snmp.trap.pdu_type`, and `snmp.version`. |
| Enrichment | Attributes `snmp.source.reverse_dns`, `snmp.device.hostname`, `snmp.device.vendor`, `netdata.nidl.node`, `netdata.topology.interface`, and `netdata.topology.neighbors` when available. |
| Dedup summaries | Attributes `snmp.trap.suppressed_count`, `snmp.trap.suppressed_fingerprints`, and `snmp.trap.report_period_sec`. |
| Decode errors | Attributes `snmp.trap.decode_error.kind`, `snmp.trap.decode_error.message`, `snmp.trap.packet_size`, `snmp.trap.packet_sha256`, `netdata.trap.listener`, and `snmp.engine_id`. |
| Varbind payload | Attribute `snmp.varbinds`. OTLP does not export separate `TRAP_VAR_*` fields. The value is a structured list, not a flat string. |
| Profile tags | Attributes named `trap.<lowercase key>`. |

For all journal fields and OTLP attributes, see [Field Reference](/docs/snmp-traps/field-reference.md).

## Security guidance

Trap logs can contain sensitive network, inventory, and security context. Treat both local journal files and downstream SIEM copies as operational data.

- Use [Secrets Management](/src/collectors/SECRETS.md) for SNMP communities, SNMPv3 auth and privacy keys, and OTLP header values.
- Do not place real bearer tokens, API keys, community strings, or SNMPv3 passphrases directly in `go.d/snmp_traps.conf`, docs, tickets, support artifacts, or SIEM examples.
- OTLP metadata headers, including `authorization`, are sent only on the OTLP/gRPC connection. They are not written to `snmp.varbinds` or any other trap attribute, but they are still sensitive and should come from a secret reference.
- Netdata automatically omits the `snmpTrapCommunity` varbind from indexed `TRAP_VAR_*` fields, `TRAP_JSON`, and `snmp.varbinds`. Other varbind values can still contain sensitive operational data.
- Protect `TRAP_JSON` in journal output and `snmp.varbinds` in OTLP output. They can include device-provided varbind values such as hostnames, interface descriptions, public IP addresses, locations, usernames, or vendor text.
- Prefer indexed fields such as `TRAP_SOURCE_IP`, `TRAP_NAME`, `TRAP_SEVERITY`, `snmp.source.ip`, `snmp.trap.name`, and `snmp.trap.severity` for SIEM rules.
- Minimize forwarding or indexing full payload fields where possible. Keep full payloads only where audit, investigation, or compliance requirements need them.

## Validation signals

Validate both the Netdata output path and the downstream receiver path.

| Signal | What to check |
|---|---|
| `otlp_export_failed` metric dimension (`snmp_trap_errors_otlp_export_failed` selector) | OTLP write-path failures occurred, including queue-overflow drops and gRPC export errors. For jobs with OTLP enabled, check the `snmp_trap_otlp_export_failures` alert, endpoint, credentials, TLS trust, queue capacity, and network path. |
| `journal_write_failed` metric dimension (`snmp_trap_errors_journal_write_failed` selector) | Direct journal writes failed. For jobs with journal enabled, check the `snmp_trap_journal_write_failures` alert, disk space, permissions, and the per-job journal directory. |
| Local journal availability | For direct-journal jobs, confirm `/var/log/netdata/traps/<job>/` exists, contains the machine-id child directory, and `journalctl --directory=/var/log/netdata/traps/<job>/$(tr -d '-' < /etc/machine-id)` can read recent trap rows. |
| Cloud-required `snmp:traps` Function source | For direct-journal jobs, confirm the job appears as a trap log source in Netdata Cloud. OTLP-only jobs are not expected to appear there. |
| Downstream receiver checks | In the SIEM or OTLP receiver, confirm records arrive with `service.name=netdata-snmptrap`, the expected `service.instance.id`, and the expected event names and attributes. |

For receiver metrics and alerts, see [Metrics and Alerts](/docs/snmp-traps/metrics-and-alerts.md).

## Related pages

- [Configuration](/docs/snmp-traps/configuration.md) - Configure output backends, retention, OTLP endpoint settings, and secrets.
- [Field Reference](/docs/snmp-traps/field-reference.md) - Map journal fields and OTLP attributes.
- [Journal and querying](/docs/snmp-traps/journal-and-querying.md) - Query and export direct-journal trap rows locally.
- [Metrics and Alerts](/docs/snmp-traps/metrics-and-alerts.md) - Validate receiver health and forwarding failures.
