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

Direct journal output is enabled by default for listener jobs. It writes local trap log files that can be read with `journalctl --directory` on systems where `journalctl` is available. Use OTLP-only mode when the external OTLP receiver is the system of record and local trap files are not needed.

```yaml
journal:
  enabled: true
```

Netdata writes trap entries as journal-compatible files below the configured Netdata log directory; for the exact per-job path and how the `journalctl --directory` path is built, see [Journal and Querying](/docs/snmp-traps/journal-and-querying.md). Each entry carries `ND_LOG_SOURCE=snmp-trap`, and these files back the job source exposed through the Cloud-required `snmp:traps` Function; they are not the host systemd-journald service's normal journal.

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

OTLP export is disabled by default. It uses gRPC transport only (HTTP/protobuf receivers on port `4318` are not supported), takes a `host:port`/`http://`/`https://` endpoint, and preflights the receiver at job startup so an unreachable collector blocks the job. For the full `otlp` option block, defaults, endpoint/TLS rules, and preflight behavior, see [Configuration](/docs/snmp-traps/configuration.md#otlpgrpc-export).

What matters for forwarding is the delivery behavior under pressure:

- Brief receiver outages recover on their own; sustained ones overflow the send queue, and over-full records are dropped and counted under `otlp_export_failed`.
- In journal and OTLP mode the local journal is unaffected by OTLP drops, so traps remain queryable locally. In OTLP-only mode a dropped record is lost with no local copy.
- The OTLP queue is not durable: records still waiting to be sent are lost on an ungraceful restart, so do not treat OTLP-only as durable storage.

The batching, queue, and retry knobs are defined in [Configuration](/docs/snmp-traps/configuration.md#otlpgrpc-export).

## OTLP mapping highlights

OTLP export sends traps as OTLP LogRecords. Build SIEM rules against the OTLP attribute names that arrive downstream, not the direct-journal field names. The listener resource carries `service.name=netdata-snmptrap` and `service.instance.id=<job>`; the message becomes the log body, the report type is `snmp.trap.report_type`, the selected source is `snmp.source.ip`, and varbinds arrive as the structured `snmp.varbinds` attribute rather than separate `TRAP_VAR_*` fields.

For the complete journal-field-to-OTLP-attribute mapping table, see [Field Reference](/docs/snmp-traps/field-reference.md#otlp-mapping-notes).

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

For receiver metrics, see [Metrics](/docs/snmp-traps/metrics.md); for the default health alerts, see [Alerts](/docs/snmp-traps/alerts.md).

## Related pages

- [Configuration](/docs/snmp-traps/configuration.md) - Configure output backends, retention, OTLP endpoint settings, and secrets.
- [Field Reference](/docs/snmp-traps/field-reference.md) - Map journal fields and OTLP attributes.
- [Journal and querying](/docs/snmp-traps/journal-and-querying.md) - Query and export direct-journal trap rows locally.
- [Metrics](/docs/snmp-traps/metrics.md) - Validate receiver health and forwarding failures.
- [Alerts](/docs/snmp-traps/alerts.md) - Default alerts on OTLP export and write failures.
