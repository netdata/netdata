<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/snmp-traps/journal-and-querying.md"
sidebar_label: "Journal and Querying"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'journal', 'querying', 'logs', 'journalctl', 'export']
endmeta-->

<!-- markdownlint-disable-file -->

# Journal and Querying

Use this page when you need to search, export, copy, or integrate SNMP trap data from Netdata Cloud Logs or from local journal-compatible files.

Direct-journal jobs write journal-compatible files under the configured Netdata log directory. The per-job root is normally:

```text
/var/log/netdata/traps/<job>/
```

The effective `journalctl --directory` path is the machine-id child directory inside that job root:

```text
/var/log/netdata/traps/<job>/$(tr -d '-' < /etc/machine-id)
```

These files back the job source exposed through the Cloud-required `snmp:traps` Function and appear in Netdata Cloud as **SNMP Trap Logs** when the Agent is connected. Individual journal entries use `ND_LOG_SOURCE=snmp-trap`. They are not the host systemd-journald service's normal journal. When you use `journalctl`, point it at the effective trap journal directory with `--directory`.

OTLP-only jobs do not create local journal files and do not appear as job sources in the `snmp:traps` Function. Query OTLP-only trap events in the OTLP receiver or downstream log system instead.

## Choose the query surface

| Use case | Query surface | Notes |
|---|---|---|
| Interactive triage | Netdata Cloud Logs | Use **SNMP Trap Logs** through the Cloud-required `snmp:traps` Function and the **Trap Jobs** source selector. |
| Local shell search | `journalctl --directory=/var/log/netdata/traps/<job>/$(tr -d '-' < /etc/machine-id)` | Reads one direct-journal job directory without reading the host system journal. |
| Export or integration | `journalctl --output=json` | Produces newline-delimited JSON objects for scripts and downstream tools. |

The `snmp:traps` Function uses these default facets:

- `TRAP_CATEGORY`
- `TRAP_DEVICE_VENDOR`
- `TRAP_NAME`
- `TRAP_SEVERITY`
- `TRAP_SOURCE_IP`
- `_HOSTNAME`
- `TRAP_JOB`

Start with these fields before opening large payload fields such as `TRAP_JSON` or `TRAP_ENRICHMENT`.

## Query in Netdata Cloud Logs

1. Open **SNMP Trap Logs** through Netdata Cloud Logs.
2. Use the **Trap Jobs** selector to choose one direct-journal job, such as `edge-traps`, or leave all direct-journal jobs selected.
3. Narrow the time window first.
4. Filter by report type, source, trap identity, category, or severity.
5. Open `TRAP_JSON` only when the indexed fields do not answer the question.

Useful first filters:

| Question | Fields to use |
|---|---|
| Which listener produced this row? | `TRAP_JOB` |
| Is this a normal trap, decode error, or dedup summary? | `TRAP_REPORT_TYPE` |
| Which device or sender is involved? | `TRAP_SOURCE_IP`, `_HOSTNAME`, `TRAP_SOURCE_UDP_PEER` |
| What trap was decoded? | `TRAP_NAME`, `TRAP_OID` |
| How urgent is it? | `TRAP_SEVERITY`, `PRIORITY` |
| What kind of event is it? | `TRAP_CATEGORY` |
| Which vendor was enriched? | `TRAP_DEVICE_VENDOR` |
| Which event varbind matters? | `TRAP_VAR_*`, `TRAP_VAR_*_RAW` |

For field meanings and population rules, see [Field Reference](/docs/snmp-traps/field-reference.md).

## Query with journalctl

Use `sudo` when the trap journal files are not readable by your user.

Show recent rows from one job:

```bash
sudo journalctl --directory=/var/log/netdata/traps/edge-traps/$(tr -d '-' < /etc/machine-id) \
  --since "2 hours ago" \
  --no-pager
```

Show one source IP in a time window:

```bash
sudo journalctl --directory=/var/log/netdata/traps/edge-traps/$(tr -d '-' < /etc/machine-id) \
  TRAP_SOURCE_IP=192.0.2.10 \
  --since "2 hours ago" \
  --until "now" \
  --no-pager
```

Show normal trap rows only:

```bash
sudo journalctl --directory=/var/log/netdata/traps/edge-traps/$(tr -d '-' < /etc/machine-id) \
  TRAP_REPORT_TYPE=trap \
  --since "2 hours ago" \
  --no-pager
```

Filter by severity and category:

```bash
sudo journalctl --directory=/var/log/netdata/traps/edge-traps/$(tr -d '-' < /etc/machine-id) \
  TRAP_SEVERITY=warning \
  TRAP_CATEGORY=state_change \
  --since "2 hours ago" \
  --no-pager
```

Filter by source and trap name:

```bash
sudo journalctl --directory=/var/log/netdata/traps/edge-traps/$(tr -d '-' < /etc/machine-id) \
  TRAP_SOURCE_IP=192.0.2.10 \
  TRAP_NAME=IF-MIB::linkDown \
  --since "2 hours ago" \
  --no-pager
```

`journalctl` treats matches on different fields as logical AND. Repeating the same field acts as OR for that field, for example:

```bash
sudo journalctl --directory=/var/log/netdata/traps/edge-traps/$(tr -d '-' < /etc/machine-id) \
  TRAP_SEVERITY=crit \
  TRAP_SEVERITY=err \
  --since "2 hours ago" \
  --no-pager
```

## Export JSON

Use JSON export when you want to copy rows into a ticket, process rows with a script, or feed another tool. Keep exports small and review payload fields before sharing them.

Export selected fields for one source:

```bash
sudo journalctl --directory=/var/log/netdata/traps/edge-traps/$(tr -d '-' < /etc/machine-id) \
  TRAP_SOURCE_IP=192.0.2.10 \
  --since "2 hours ago" \
  --output=json \
  --output-fields=__REALTIME_TIMESTAMP,MESSAGE,TRAP_JOB,TRAP_REPORT_TYPE,TRAP_NAME,TRAP_CATEGORY,TRAP_SEVERITY,TRAP_SOURCE_IP,TRAP_JSON \
  --no-pager > snmp-traps-edge-traps.json
```

Export a compact operator view without the full payload:

```bash
sudo journalctl --directory=/var/log/netdata/traps/edge-traps/$(tr -d '-' < /etc/machine-id) \
  --since "2 hours ago" \
  --output=json \
  --output-fields=__REALTIME_TIMESTAMP,MESSAGE,TRAP_JOB,TRAP_REPORT_TYPE,TRAP_NAME,TRAP_CATEGORY,TRAP_SEVERITY,TRAP_SOURCE_IP \
  --no-pager > snmp-traps-summary.json
```

## Decode errors

Decode-error rows use:

```text
TRAP_REPORT_TYPE=decode_error
```

Use them when packets reached the listener but could not become normal trap rows.

```bash
sudo journalctl --directory=/var/log/netdata/traps/edge-traps/$(tr -d '-' < /etc/machine-id) \
  TRAP_REPORT_TYPE=decode_error \
  TRAP_SOURCE_IP=192.0.2.10 \
  --since "2 hours ago" \
  --output=json-pretty \
  --output-fields=__REALTIME_TIMESTAMP,MESSAGE,TRAP_JOB,TRAP_DECODE_ERROR_KIND,TRAP_DECODE_ERROR,TRAP_SOURCE_IP,TRAP_SOURCE_UDP_PEER,TRAP_SOURCE_UDP_PORT,TRAP_PACKET_SIZE,TRAP_PACKET_SHA256,TRAP_LISTENER,TRAP_ENGINE_ID,TRAP_JSON \
  --no-pager
```

Useful decode-error fields:

| Field | Use |
|---|---|
| `TRAP_DECODE_ERROR_KIND` | Main bounded failure class, such as `malformed_pdu`, `auth_failures`, `usm_failures`, `unknown_engine_id`, or `decode_failed`. |
| `TRAP_DECODE_ERROR` | Sanitized and shortened decoder error text. |
| `TRAP_PACKET_SIZE` | Received datagram size. |
| `TRAP_PACKET_SHA256` | Packet fingerprint for grouping repeated bad packets without storing raw packet bytes. |
| `TRAP_ENGINE_ID` | SNMPv3 engine ID when safely extractable. Treat it as inventory data. |
| `TRAP_JSON` | Structured decode-error details and packet sequence when available. |

Raw packet bytes are not written to decode-error rows because packets can contain community strings or binary payloads. For troubleshooting workflow, see [Troubleshooting](/docs/snmp-traps/troubleshooting.md).

## Dedup summaries

When deduplication is enabled, the first matching trap is written normally. Later matching traps inside the dedup window are suppressed and counted. Netdata writes a summary row with:

```text
TRAP_REPORT_TYPE=deduplication_summary
```

Query summary rows:

```bash
sudo journalctl --directory=/var/log/netdata/traps/edge-traps/$(tr -d '-' < /etc/machine-id) \
  TRAP_REPORT_TYPE=deduplication_summary \
  --since "2 hours ago" \
  --output=json-pretty \
  --output-fields=__REALTIME_TIMESTAMP,MESSAGE,TRAP_JOB,TRAP_SUPPRESSED_COUNT,TRAP_SUPPRESSED_FINGERPRINTS,TRAP_REPORT_PERIOD_SEC,TRAP_JSON \
  --no-pager
```

Useful dedup summary fields:

| Field | Use |
|---|---|
| `TRAP_SUPPRESSED_COUNT` | Total duplicate trap events suppressed in the summary period. |
| `TRAP_SUPPRESSED_FINGERPRINTS` | Number of distinct dedup fingerprints with suppressed events. |
| `TRAP_REPORT_PERIOD_SEC` | Length of the summary period. |
| `TRAP_JSON` | Structured summary with total suppressed count, period, fingerprint count, and optional per-trap breakdown. |

A dedup summary is a job-level summary row. It is not a normal trap from one device and does not include normal trap varbind fields.

## Payload and sensitive-data notes

`TRAP_VAR_*` fields are query-friendly indexed varbinds. They are emitted only for normal trap rows and only for non-sensitive, non-redundant varbinds. Enum-backed varbinds can also emit `TRAP_VAR_*_RAW` with the raw numeric value.

`TRAP_JSON` stores structured payload detail. For normal traps, it contains non-sensitive varbind entries, including protocol-control varbinds that are not indexed as `TRAP_VAR_*`, and `netdata_packet_sequence` when available. For decode errors and dedup summaries, it contains the matching diagnostic or summary shape. Prefer specific fields for rules and use `TRAP_JSON` for audit, export, and residual searches.

`TRAP_ENRICHMENT` records source-attribution and enrichment decisions. Use it to debug why source, hostname, vendor, interface, neighbor, or reverse-DNS fields were or were not applied. Do not use it as a broad default facet.

Some values can contain newlines, control bytes, or invalid UTF-8. Netdata stores those fields using safe binary journal encoding so the row cannot inject fake fields. In `journalctl --output=json`, binary fields can appear as arrays of unsigned byte values. Treat them as payload data, not display text.

Trap logs can contain sensitive inventory and operational details:

- Do not paste real community strings, SNMPv3 secrets, packet captures, organization names, identifiers, or public IPs that identify your environment into examples or tickets.
- Use placeholders and RFC 5737 example IPs such as `192.0.2.10`.
- Review `TRAP_JSON`, `TRAP_ENRICHMENT`, `TRAP_VAR_*`, `TRAP_ENGINE_ID`, interface names, neighbor names, and hostnames before forwarding or sharing exports.
- Minimize exported fields when integrating with a SIEM. See [Forwarding to SIEM](/docs/snmp-traps/forwarding-to-siem.md).

## Related pages

- [Usage and Output](/docs/snmp-traps/usage-and-output.md) - Understand report types, normal trap rows, decode errors, and dedup summaries.
- [Field Reference](/docs/snmp-traps/field-reference.md) - Check every field, when it appears, and how to query it.
- [Forwarding to SIEM](/docs/snmp-traps/forwarding-to-siem.md) - Forward trap events and map fields in an external log system.
- [Troubleshooting](/docs/snmp-traps/troubleshooting.md) - Investigate listener, decode, journal, and source issues.
