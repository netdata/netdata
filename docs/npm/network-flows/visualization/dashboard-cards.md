<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/network-flows/visualization/dashboard-cards.md"
sidebar_label: "Plugin Health Charts"
learn_status: "Published"
learn_rel_path: "Network Flows/Visualization"
keywords: ['plugin health', 'metrics', 'operational charts', 'monitoring']
endmeta-->

<!-- markdownlint-disable-file -->

# Plugin Health Charts

The netflow plugin publishes its own operational charts under the `netdata.netflow.*` chart context. These appear on the standard Netdata charts page (alongside system metrics like CPU and memory), **not** inside the Network Flows view. They are how you monitor the plugin itself: is it receiving data, are templates flowing, is memory growing, is disk being written.

This is also where you look first when something seems wrong — long before opening the Network Flows view.

Default production-health charts update every 1 second. Optional memory
diagnostics charts update on `charts.memory_diagnostics.interval` when enabled.

## The charts

| Chart | Type | What it shows |
|---|---|---|
| `netflow.input_packets` | line, packets/s | UDP datagrams received, empty datagrams, and Linux socket-buffer drops |
| `netflow.input_bytes` | line, bytes/s | UDP byte rate |
| `netflow.protocol_packets` | stacked, packets/s | Successfully identified v5, v7, v9, IPFIX, and sFlow packets |
| `netflow.flow_sets` | stacked, sets/s | NetFlow v9 and IPFIX Data, Options Data, Template, missing-template, and ignored Sets |
| `netflow.templates` | stacked, templates/s | Data and Options Template definitions received over v9 and IPFIX |
| `netflow.flow_records` | stacked, records/s | v5, v7, v9, and IPFIX Data Records decoded |
| `netflow.options_records` | stacked, records/s | v9/IPFIX Options Records and sampling configuration sent in Data Records |
| `netflow.sflow_samples` | stacked, samples/s | sFlow flow, counter, discarded-packet, real-time, and unknown samples |
| `netflow.decoder_exceptions` | line, events/s | Receive, parse, template, disabled-protocol, eviction, counter, decapsulation, and unsupported-data exceptions |
| `netflow.flow_rows` | line, rows/s | Decoded rows and their final filtered, journaled, or write-failed outcomes |
| `netflow.nsel_events` | stacked, records/s | Cisco NSEL update, create, teardown, deny, unsupported, and malformed records |
| `netflow.nsel_rows` | stacked, rows/s | Forward and reverse rows emitted from NSEL updates |
| `netflow.nsel_exceptions` | line, events/s | Counterless updates, partial counter directions, and suppressed zero responders |
| `netflow.raw_journal_ops` | line, ops/s | Raw journal: writes, sync calls, errors |
| `netflow.raw_journal_bytes` | line, bytes/s | Raw journal logical bytes written |
| `netflow.materialized_tier_ops` | line, ops/s | Rollup tiers: rows produced per tier, flushes, errors |
| `netflow.materialized_tier_bytes` | stacked, bytes/s | Rollup tier byte rate, broken down by tier |
| `netflow.tier_commit_age` | line, seconds | Time since each tier worker completed a commit |
| `netflow.tier_commit_duration` | line, microseconds | Duration of each tier's most recent commit |
| `netflow.tier_commit_batches` | line, batches/s | Completed commit batches per tier |
| `netflow.tier_commit_stretched` | line, events/s | Tier commits that exceeded their planned window |
| `netflow.open_tiers` | stacked, rows | Rows currently open per tier |
| `netflow.journal_io_ops` | line, ops/s | Decoder-state persistence and facet-index operations/errors |
| `netflow.journal_io_bytes` | line, bytes/s | Decoder-state persist byte rate |
| `netflow.decoder_scopes` | line, scopes | Distinct (exporter, observation domain) scopes the decoder tracks |
| `netflow.facet_values` | line, values | Published facet value cardinality |
| `netflow.facet_fields` | line, fields | Populated and autocomplete-backed facet fields |
| `netflow.tier_index_entries` | line, entries | Tier-index hours and rollup-flow entries |
| `netflow.memory_resident_bytes` | line, bytes | Optional diagnostics: process RSS, peak RSS, breakdown |
| `netflow.memory_resident_mapping_bytes` | stacked, bytes | Optional diagnostics: RSS broken down by mapping type |
| `netflow.memory_allocator_bytes` | line, bytes | Optional diagnostics: allocator-internal stats |
| `netflow.memory_accounted_bytes` | stacked, bytes | Optional diagnostics: RSS attributed to known components, plus `unaccounted` |
| `netflow.memory_tier_index_bytes` | stacked, bytes | Optional diagnostics: tier-index memory drilldown |

## Reading the most useful charts

### Input and protocol packets

`netflow.input_packets` describes the UDP socket only:

- **`udp_received`** — every datagram pulled from a listener socket, including empty datagrams.
- **`empty`** — received zero-length datagrams. This is a subset of `udp_received`.
- **`kernel_dropped`** — datagrams Linux dropped because a specific listener socket's receive buffer was full. The plugin reads the kernel counter once per second by exact socket identity. It is zero on platforms that do not expose this counter.

`netflow.protocol_packets` reports successfully identified **v5, v7, v9, IPFIX, and sFlow packets separately**. These counters still increase when that protocol is disabled for row production, so `disabled_protocol_packets` can explain why recognized traffic did not reach storage.

### Sets, templates, records, and samples

Do not compare these rates as though they were the same thing:

- One UDP datagram can contain multiple v9/IPFIX Sets.
- One Set can contain multiple template definitions or Data Records.
- One sFlow datagram can contain multiple samples.
- One Data Record can produce zero, one, or two stored rows.

Use `netflow.flow_sets` to verify v9/IPFIX control traffic. `*_missing_template` means a Data Set could not be decoded because its template was unavailable. `v9_ignored` is a reserved Set ID; `ipfix_ignored` covers template withdrawals and reserved Set IDs.

Use `netflow.templates` to verify that exporters refresh both Data and Options Templates. Use `netflow.flow_records` for decoded Data Records, `netflow.options_records` for sampling/options traffic, and `netflow.sflow_samples` to distinguish flow-bearing sFlow samples from valid counter-only traffic. A Set first received without a template is counted as missing; if the parser later replays it after learning the template, its decoded records are counted then.

### `netflow.flow_rows`

This chart follows records after decoding. Its cumulative counters obey:

`decoded = classifier_filtered + journaled + write_failed`

If records are visible but rows are not:

- `classifier_filtered` means configured enrichment/classifier policy rejected them.
- `write_failed` means the journal could not store them.
- A gap before `decoded` is explained by the protocol-specific charts and `netflow.decoder_exceptions`.

### Cisco NSEL charts

NSEL records and traffic rows are intentionally not one-to-one:

- `netflow.nsel_events` counts every recognized event record by type.
- `netflow.nsel_rows` counts forward and reverse traffic rows emitted from event-5 updates.
- `netflow.nsel_exceptions` explains update records that produced fewer rows because counters were missing, partial, or zero.

Create, teardown, and deny events are visible in `netflow.nsel_events` but are not stored as traffic rows.

### `netflow.decoder_exceptions`

All dimensions are exceptional **events**, but they occur at different stages:

- `udp_receive_errors`, `udp_socket_setup_errors`, and `parse_errors` identify socket/parser failures.
- `missing_template_sets` identifies v9/IPFIX Sets received without an available template.
- `disabled_protocol_packets` identifies valid packets intentionally excluded by configuration.
- `parser_source_evictions` identifies exporter parser state removed at the configured source limit.
- `partial_counter_records`, `decapsulation_failed_records`, `unsupported_data_sets`, and `ipfix_zero_reverse_records` explain record-to-row differences.

Because these dimensions count different objects, use them to identify causes; do not add them together or divide all of them by one common denominator.

### `netflow.decoder_scopes`

Cardinality of decoder state. Reports how many distinct `(exporter, observation domain)` template caches the plugin currently holds. Watch for unbounded growth — an exporter that frequently rotates template IDs (rare but real) will inflate this without bound.

### `netflow.facet_*`, `netflow.tier_index_entries`, and `netflow.open_tiers`

Default lightweight production signals. Use them to see whether memory pressure is likely coming from growing facet vocabulary, rollup-index cardinality, or open rollup rows without enabling expensive byte-level diagnostics.

### `netflow.materialized_tier_*`

Show the rollup pipeline working. `*_rows` should track ingest. `flushes` should tick steadily; if it stops, tiering is stalled.

### `netflow.memory_resident_bytes` and `netflow.memory_accounted_bytes`

These charts exist only when `charts.memory_diagnostics.enabled: true` is set in `netflow.yaml`.

If RSS climbs over time:

- Check `netflow.memory_accounted_bytes` to see where it's going.
- The `unaccounted` dimension is `RSS - sum(known components)`. **A growing `unaccounted` is your leak signal.**
- `tier_indexes` and `open_tiers` are normal sources of growth — they should track ingest rate.
- `geoip_asn` and `geoip_geo` are mmap'd MMDB files. Their size grows as the kernel pages the file in under read pressure.

### `netflow.memory_resident_mapping_bytes`

This one breaks RSS down by what's mapped. Useful when you want to attribute "this process is using 800 MB" — heap, journals (per tier), MMDB files, anonymous mappings, etc.

## What's NOT in these charts

These charts do not include:

- **Per-exporter ingest counter.** No per-source rate dimension. Decoder-scope cardinality tells you how many sources, not how busy each one is.
- **NIC and network-wide packet loss.** `kernel_dropped` covers only the plugin's Linux UDP listener sockets, not upstream network loss or NIC drops.
- **Template cache hit ratio.** Missing-template Sets are counted, but there is no corresponding hit counter.
- **GeoIP staleness signal.** No "MMDB last loaded" timestamp or version. The mapping memory dimensions tell you if a database is loaded, not how old it is.
- **Per-tier query latency.** These charts cover ingest and storage; query-side performance isn't observable.
- **BioRIS counters.** BioRIS routing-state details are not published as chart dimensions.

## How to use these charts for diagnosis

| Symptom | Look at | What it means |
|---|---|---|
| Network Flows view is empty | `netflow.input_packets`, then `netflow.protocol_packets` | No UDP = network/listener problem. UDP without protocol packets = malformed or unsupported traffic; check decoder exceptions. |
| Sudden drop in flows | `netflow.protocol_packets`, `netflow.flow_records`, `netflow.flow_rows` | Identifies whether traffic stopped at the exporter, decoder, classifier, or journal. |
| Templates failing | `*_missing_template` in `netflow.flow_sets` | Exporter not sending templates often enough, collector has not learned them yet, or exporter identity changed. |
| sFlow packets but no rows | `netflow.sflow_samples` | Counter-only samples are valid but do not contain endpoint traffic rows. |
| NSEL records but fewer rows | `netflow.nsel_events`, `netflow.nsel_rows`, `netflow.nsel_exceptions` | Non-update events are intentionally not traffic; directional counter outcomes explain event-5 rows. |
| UDP loss under load | `kernel_dropped` in `netflow.input_packets` | Linux listener receive buffer overflow. This does not cover upstream or NIC loss. |
| Cache growing without bound | `decoder_scopes` rising over hours | Exporter churn or unstable template IDs. Investigate per-router behaviour. |
| Memory pressure | `netflow.facet_values`, `netflow.tier_index_entries`, `netflow.open_tiers`; optional `netflow.memory_*` diagnostics | Default count charts show where state cardinality grows. If byte diagnostics are enabled and `unaccounted` grows → unattributed allocation, possibly a leak. |
| Disk write stalls | `netflow.raw_journal_ops` `write_errors`, `sync_errors` | Disk full, permission denied, fs error. |
| Decoder/facet state not persisting | `netflow.journal_io_ops` | Persist calls should tick periodically; all error dimensions should remain zero. |

## Where these are NOT shown

These charts are **not** in the Network Flows view. Look for them on the standard Netdata charts page, in the family `netflow`. The Network Flows view itself shows traffic data, not plugin health.

## What's next

- [Troubleshooting](/docs/npm/network-flows/troubleshooting.md) — Concrete diagnostic workflows.
- [Validation and Data Quality](/docs/npm/network-flows/validation.md) — Cross-checking plugin counters against SNMP.
- [Configuration](/docs/npm/network-flows/configuration.md) — Tuning that affects what these charts show.
