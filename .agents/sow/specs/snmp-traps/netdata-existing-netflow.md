# Netdata's Existing NetFlow/IPFIX/sFlow Plugin — Inventory and Analysis

Purpose: provide reference context for the SNMP-trap design discussion. The flow plugin is the closest architectural analog already shipping in this repository: UDP-based, high-volume event-stream ingestion, recently built, written in Rust, running as a separate plugin under the Netdata Agent. This document inventories what it is, how it is wired in, and which patterns are/are not transferable to a trap subsystem.

Repository: `netdata/netdata @ snmptraps` (this working tree).

Scope boundary: this is **not** a comparative system spec (SOW-0032 governs those, with a 20-section template). It is an internal inventory mirroring the shape of `.agents/sow/specs/snmp-traps/netdata-existing-snmp.md`.

---

## 1. Module Inventory

One Rust crate, one binary, one Functions endpoint. No Go counterpart.

| Component | Path | Purpose |
|---|---|---|
| `netflow-plugin` (binary) | `src/crates/netflow-plugin/` | Standalone plugin executable launched by `plugins.d` |
| `netflow-plugin` (crate) | `src/crates/netflow-plugin/Cargo.toml` | Single binary crate, ~240 source files |
| Stock config | `src/crates/netflow-plugin/configs/netflow.yaml` | Operator config, installed to `/usr/lib/netdata/conf.d/netflow.yaml` |
| Metadata | `src/crates/netflow-plugin/metadata.yaml` | 266 KB; drives the integrations pipeline (3 modules: `netflow`, `ipfix`, `sflow` + 18 enrichment sub-integrations) |
| Integration docs | `src/crates/netflow-plugin/integrations/*.md` | 23 generated per-feature docs (netflow.md, ipfix.md, sflow.md, plus enrichment sources) |
| Test fixtures | `src/crates/netflow-plugin/testdata/flows/*.pcap` | 36 pcap captures of real exporter traffic |
| Packaging | `packaging/cmake/pkg-files/deb/plugin-netflow/postinst` | sets `chown root:netdata` + `chmod 0750` on the binary |
| CMake hook | `CMakeLists.txt:3506-3570` | `ENABLE_PLUGIN_NETFLOW` switch, Corrosion-managed Rust build |

The Rust crate depends on framework crates that the rest of the Rust ecosystem in this repo shares:

| Crate (workspace) | Role for `netflow-plugin` |
|---|---|
| `rt` | Plugin runtime: tokio-based `PluginRuntime`, `FunctionHandler` trait, `NetdataChart` derive, env helpers |
| `netdata-plugin-protocol` | `FunctionCall` / `FunctionResult` / `HttpAccess` types and `plugins.d` wire format |
| `netdata-plugin-error` | Plugin error/result types |
| `journal-common` / `journal-core` / `journal-engine` / `journal-index` / `journal-log-writer` / `journal-registry` | The journal stack reused from `systemd-journal-plugin` infrastructure — provides indexed, mmap-backed, time-ordered append-only log files with retention/rotation, plus the query/index machinery the flows UI talks to |

Source-tree breakdown (file counts under `src/crates/netflow-plugin/src/`):

```
api/             5    Functions endpoint glue (flows:netflow handler, model, params)
charts/          5    Internal monitoring charts (input pps/bps, journal ops, memory, decoder scopes)
decoder/        ~70   Per-protocol packet parsers, template state, sampling state, persistence
enrichment/     ~35   GeoIP, static metadata, networks, classifiers, ASN providers, network sources
facet_runtime/   4    Sidecar files that pre-index facet vocabularies per journal file
facet_catalog    1    Static facet field catalog
flow/           ~15   Flow record (~99 canonical fields), schema, encode/decode for journal
flow_index/      2    Tier flow-index storage
ingest/         ~10   The hot path: UDP listener, decode, write, tier accumulation, rebuild
network_sources/ 7    Periodic HTTP fetch of remote IP prefix metadata (Akvorado-style)
plugin_config/  ~25   Listener/protocols/journal/enrichment/routing config, validation
presentation/    8    Column labels, ICMP/TCP/IP label resolution for the UI
query/          ~40   Query engine for the flows:netflow Function (scan/group/rank/timeseries)
routing/        ~20   BGP/BMP (TCP listener) and BioRIS (gRPC) for route enrichment
tiering/        ~25   Rollups (1m/5m/1h): schema, accumulator, encode, materialize, index
main.rs              Tokio bootstrap (4 worker threads max, jemalloc-style arena cap, THP off)
memory_allocator.rs  glibc arena cap, transparent huge pages disable
```

Third-party Rust crates that carry the protocol work (`Cargo.toml:14-66`):

- `netflow_parser` — NetFlow v5/v7/v9/IPFIX parser (Mikrotik-style; vendored via workspace)
- `sflow-parser` — sFlow v5 datagram parser
- `netgauze-bgp-pkt`, `netgauze-bmp-pkt` — BGP/BMP packet parsing (used by routing enrichment)
- `maxminddb` — MMDB GeoIP reader
- `journal-*` (in-repo) — append-only indexed journal storage
- `tokio`, `tokio-util`, `tonic` (gRPC for BioRIS), `reqwest` (network sources)
- `jaq-core` + `jaq-json` + `jaq-std` — embedded jq engine for transforming external IP-range feeds

---

## 2. Protocol Stack

### 2.1 Versions supported

| Protocol | Status | Parser | Templates? |
|---|---|---|---|
| NetFlow v5 | enabled by default | `netflow_parser::static_versions::v5` | No (fixed schema) |
| NetFlow v7 | enabled by default | `netflow_parser::static_versions::v7` | No (fixed schema) |
| NetFlow v9 (RFC 3954) | enabled by default | `netflow_parser::variable_versions::v9` | Yes (per-exporter, per-observation-domain template cache) |
| IPFIX (RFC 7011/7012) | enabled by default | `netflow_parser::variable_versions::ipfix` | Yes (templates + options templates, enterprise PEN support) |
| sFlow v5 | enabled by default | `sflow-parser::parse_datagram` | No (TLV-based, no templates) |

Toggle per protocol: `protocols.{v5,v7,v9,ipfix,sflow} = true|false` (`plugin_config/types/protocol.rs`). v6 is **not** supported.

### 2.2 Transport

- UDP only, unprivileged default port **2055** (`configs/netflow.yaml:18`, listener default in `plugin_config/types/listener.rs:7`).
- One single UDP socket binding for all three protocol families; the dispatcher branches by content: `is_sflow_payload()` checks the first 4 bytes for `version == 5`, otherwise calls the NetFlow parser, which itself peeks the version word (`decoder/state/runtime/decode.rs:21-44`, `decoder/protocol/entry.rs:3-8`).
- No TLS, no DTLS, no IPFIX-over-TCP/SCTP (RFC 7011 allows TCP/SCTP; not implemented). Defensible — virtually all real flow exporters use UDP.
- BMP enrichment (TCP) and BioRIS enrichment (gRPC/TCP) are separate listeners, separate ports, not part of the main UDP path — see §10.

### 2.3 Concurrency model

Tokio multi-thread runtime, `worker_threads = min(available_parallelism, 4)`, `max_blocking_threads = max(8, worker_threads)` (`main.rs:40-41, 362-371`).

The hot path is **single-threaded**:

```rust
loop {
    tokio::select! {
        _ = shutdown.cancelled() => { break; }
        _ = sync_tick.tick() => { /* periodic sync, decoder state persist, tier maintenance */ }
        recv = socket.recv_from(&mut buffer) => {
            handle_received_packet(source, &buffer[..n], …);
        }
    }
}
```

— `ingest/service/runtime.rs:5-45`. `recv_from` into a single reusable buffer (default 9216 bytes, jumbo-frame compatible), in-line decode, in-line journal write. **No mpsc channel, no worker pool, no per-source affinity.**

Notable consequence: the README's benchmark section reports CPU saturation at ~98-99% of one core for the post-decode ingest path; with decode included the practical ceiling is ~22-25k flows/s at high cardinality on a 12th-gen i9 (`README.md:428-449`). The flow team accepted this single-thread design.

Other tokio tasks: BMP listener, BioRIS listener, network-sources refresher, charts sampler, journal-notify watcher (`main.rs:179-258`). None of them touch the ingest hot path.

### 2.4 Template caching (v9 / IPFIX only)

- Templates are kept in-memory per `(exporter_ip, observation_domain_id)` namespace key (`decoder/state/models.rs:3-7`) — port is intentionally **dropped** (`decoder/normalize_template_scope_source` zeros the port, comment `decoder.rs:103-106`: "Parser/template scope should follow exporter identity, not ephemeral UDP source ports").
- Templates **are persisted to disk** as msgpack with an 8-byte magic, schema version, xxhash64 payload hash, and length (`decoder/state/persisted.rs:13-35`, format `"NDFS" + ver + hash + len + payload`). Persist interval: 30s (`ingest.rs:41` `DECODER_STATE_PERSIST_INTERVAL_USEC = 30_000_000`). Per-namespace file, 8 MiB max payload.
- On restart, the plugin restores templates from disk so it doesn't have to wait for exporters to re-emit templates (`decoder/state/restore/{ipfix,v9}.rs`). This is real engineering effort — flow exporters typically refresh templates every ~30 min, which is operationally painful after an agent restart.

### 2.5 Packet-level dispatch

`decode_udp_payload_at()` in `decoder/state/runtime/decode.rs:14-64` is the entry point per UDP datagram:

1. `observe_decoder_state_from_payload()` — if the packet contains templates, update the per-source template namespace.
2. `is_sflow_payload(payload)` — peek first 4 bytes for sFlow version `5`.
3. Branch to `decode_sflow(...)` or `decode_netflow(...)` (latter accepts per-version enable flags).
4. Apply timestamp fallback (`apply_missing_flow_time_fallback`).
5. Run enricher across the batch (`enricher.enrich_record`); records that fail required-metadata checks are **dropped** here.

---

## 3. Listener Architecture

### 3.1 Socket binding

```rust
let socket = UdpSocket::bind(&listen).await
    .with_context(|| format!("failed to bind {}", listen))?;
```

`ingest/service/runtime.rs:9-11`. No `socket2` tuning visible at bind time (despite `socket2` being a workspace dep — it is used elsewhere for SO_RCVBUF on the BMP TCP path, `routing/bmp/session/buffer.rs`). No `SO_REUSEPORT`, no `SO_RCVBUF` sizing, no kernel buffer tuning on the UDP path. Single-socket, single-task.

### 3.2 Privileged-port handling

Default port is **2055**, not 9995, not 6343, not any privileged port. **No `CAP_NET_BIND_SERVICE` plumbing exists.** This is a defensible choice — flow exporters can be configured to send anywhere — but it sidesteps the real issue: trap receivers are expected on UDP/162, which IS privileged.

The postinst (`packaging/cmake/pkg-files/deb/plugin-netflow/postinst`) only handles ownership/permissions; there is no setcap on this binary.

### 3.3 Backpressure / packet loss

Effectively **none**. The runtime loop has only one buffer, one socket, no queue:

- If decode + journal write takes longer than the kernel's UDP socket buffer holds at the offered rate, the kernel **drops packets silently** (visible only via `/proc/net/udp` Drops column or `ss -unum`).
- There is **no internal queue depth metric**; only `udp_packets_received` and `udp_bytes_received` counters on the chart `netdata.netflow.input_packets` (`charts/metrics.rs:11-33`).
- There is `parse_errors`, `template_errors`, and protocol-specific packet counts on the same chart — these reflect what the decoder saw, **not** what the kernel dropped.
- The README explicitly warns: "Above the knee, achieved rate stays at the plateau while offered rate grows" (`README.md:434-435`) — the plateau is observed at the application layer; what the kernel did with overflow is not measured.

This is a real gap for high-rate exporters, and one the trap design must NOT inherit because trap storms are part of the threat model (foundational spec `.agents/sow/specs/snmp-traps/snmp-traps-in-observability.md` §6.6).

### 3.4 Error tolerance

```rust
recv = socket.recv_from(&mut buffer) => {
    let (received, source) = match recv {
        Ok(result) => result,
        Err(err) => {
            tracing::warn!("udp recv error: {}", err);
            continue;
        }
    };
    …
}
```

`ingest/service/runtime.rs:25-32`. Bad receives are logged and ignored. Parse errors are counted (`metrics.apply_decode_stats`) but the loop continues. The decoder never panics on malformed input (well-fuzzed by `proptest`, `dev-dependencies` lists `proptest`).

---

## 4. Configuration Model

### 4.1 Config sources

Two sources, no DynCfg (`plugin_config/runtime.rs:88-109`):

1. `${NETDATA_USER_CONFIG_DIR}/netflow.yaml` (preferred)
2. `${NETDATA_STOCK_CONFIG_DIR}/netflow.yaml` (fallback)

If running standalone (not under Netdata), `clap::Parser::parse()` reads CLI flags (`plugin_config/runtime.rs:10`). Every `clap::Parser` struct also derives `Serialize`/`Deserialize` for YAML; the `#[arg(long = "...")]` and `#[serde(...)]` annotations co-exist (`plugin_config/types/listener.rs:6-22`).

### 4.2 Top-level schema

From `plugin_config/types/plugin.rs` and stock YAML:

```yaml
enabled: true                                # master toggle (writes "DISABLE\n" to plugins.d if false)
listener:
  listen: "0.0.0.0:2055"
  max_packet_size: 9216
  sync_every_entries: 1024
  sync_interval: 1s
protocols:
  v5: true
  v7: true
  v9: true
  ipfix: true
  sflow: true
  decapsulation_mode: none                   # none | srv6 | vxlan
  timestamp_source: input                    # input | netflow_packet | netflow_first_switched
journal:
  journal_dir: flows
  tiers:
    raw:      { size_of_journal_files: 10GB, duration_of_journal_files: 7d }
    minute_1: { size_of_journal_files: 10GB, duration_of_journal_files: 7d }
    minute_5: { size_of_journal_files: 10GB, duration_of_journal_files: 7d }
    hour_1:   { size_of_journal_files: 10GB, duration_of_journal_files: 7d }
  query_max_groups: 50000
enrichment:
  classifier_cache_duration: 5m
  default_sampling_rate: { CIDR: u64 }
  override_sampling_rate: { CIDR: u64 }
  metadata_static: { exporters: { CIDR: ExporterMetadata } }
  geoip: { asn_database: [paths], geo_database: [paths], optional: bool }
  networks: { CIDR: NetworkAttributes }
  network_sources: { name: { url, method, tls, interval, timeout, transform, headers } }
  routing_static: { prefixes: { CIDR: { asn, as_path, communities, large_communities, next_hop } } }
  routing_dynamic:
    bmp:    { enabled, listen, receive_buffer, max_consecutive_decode_errors, rds, collect_*, keep }
    bioris: { enabled, ris_instances, timeout, refresh, refresh_timeout }
  exporter_classifiers: [ … ]                # Akvorado-style rules
  interface_classifiers: [ … ]
  asn_providers: [flow, routing, geoip]
  net_providers: [flow, routing]
```

`serde(deny_unknown_fields)` is used on listener and others (`plugin_config/types/listener.rs:5`) — typos in config are rejected at startup.

### 4.3 Validation

Validation runs at startup (`plugin_config.rs::PluginConfig::new() -> validate()`), with per-section validators in `plugin_config/validation/{enrichment,journal,listener}.rs`. Examples: `size_of_journal_files >= 100MB`, at least one positive retention per tier, listen address parseable, etc.

### 4.4 No DynCfg

The flow plugin does **not** integrate with Netdata's DynCfg (dynamic config) infrastructure. Edits require a YAML edit + plugin restart. This is a regression vs the go.d SNMP collector and other modern collectors (where DynCfg is integrated for runtime add/remove of jobs).

---

## 5. Storage & Persistence

### 5.1 What gets stored

Decoded flow records — **not** raw UDP packets — go into a tiered, mmap-backed journal store (`ingest/service/runtime.rs:113-166`).

Flow record schema: ~99 canonical fields (`flow/schema.rs:7-99`), all-string-keyed, journal-friendly. The encoder skips default values (zero/empty), reducing per-entry items from 87 to ~20-25 for typical flows (`flow/record/journal.rs:18-37`).

### 5.2 Tier structure

Four tiers, all on disk under `${NETDATA_CACHE_DIR}/flows/`:

| Tier | Directory | Stored | Default retention |
|---|---|---|---|
| `raw` | `raw/` | Individual flow records, one entry per decoded flow | 10 GB / 7 days |
| `1m` | `1m/` | Pre-aggregated rollups, 1-minute buckets | 10 GB / 7 days |
| `5m` | `5m/` | 5-minute buckets | 10 GB / 7 days |
| `1h` | `1h/` | 1-hour buckets | 10 GB / 7 days |

Each tier is a separate `journal_log_writer::Log` (`ingest/service.rs:8-12`) with its own rotation and retention. Rotation derives from size cap: `clamp(size/20, 5MB, 200MB)`, with a 1-hour time-based cadence as a floor (`README.md:249-256`). On rotation, a lifecycle observer notifies the facet runtime so it can index the archived file.

### 5.3 The journal engine

The flow plugin reuses the **systemd-journal-derived storage** (`src/crates/journal-*`). Each entry has:

- `_SOURCE_REALTIME_USEC` (the original packet/flow timestamp)
- `_ENTRY_REALTIME_USEC` (reception time at ingest)
- A bag of key=value field assignments (the FlowRecord fields)

This is the same indexed log format the Netdata agent already uses for systemd-journald ingestion. It is therefore:

- **mmap-readable** by query workers (zero-copy scan, `query/scan/raw.rs` etc.)
- **field-indexed** via `journal-index` (fast filter by field=value)
- **dedup-key-free** — there is no dedup of identical flows (intentional; each flow record is one event)

### 5.4 Facet pre-indexing

`facet_runtime/` maintains per-journal-file **sidecar facet indexes** (vocabulary + value bitmap) so the UI can render facet filters (top SRC_ADDR, top DST_PORT, etc.) without scanning. The facet runtime watches journal lifecycle events (rotation, retention delete) and updates sidecars accordingly (`ingest/service.rs:14-37`).

### 5.5 What gets persisted (besides flows)

| Artifact | Where | Purpose |
|---|---|---|
| Decoder template state | `${cache}/flows/decoder-state/<namespace>.bin` | Survive plugin restart without waiting for exporter template refresh |
| Facet sidecars | next to each journal file | Pre-built facet vocab/bitmap |
| Journal files | `flows/{raw,1m,5m,1h}/` | The events themselves |
| GeoIP MMDBs | `${cache}/topology-ip-intel/*.mmdb` (or `${stock_data}/topology-ip-intel/*.mmdb`) | Auto-detected; downloaded by `topology-ip-intel-downloader` |

### 5.6 Memory

Plugin disables transparent huge pages (`main.rs:65-72`) and caps glibc malloc arenas (`main.rs:49-63`) — both linux-only optimisations to keep RSS predictable. Plugin exposes detailed self-RSS / mmap / journal / GeoIP memory charts (`README.md:259-294`).

---

## 6. Integration With Other Signals

### 6.1 Metrics

The flow plugin emits **operational metrics** about itself — not summary metrics derived from flows. Charts under family `netflow`, context prefix `netdata.netflow.*` (`charts/metrics.rs`):

- `netdata.netflow.input_packets` (per-protocol)
- `netdata.netflow.input_bytes`
- `netdata.netflow.raw_journal_ops`, `netdata.netflow.raw_journal_bytes`
- `netdata.netflow.materialized_tier_ops`, `netdata.netflow.materialized_tier_bytes`
- `netflow.memory_resident_bytes` and related memory breakdown
- `netflow.decoder_scopes` (template namespaces, hydrated sources)

There is **no flow-content metric** (e.g., "bytes per src_country per minute"). All flow analytics are query-on-demand via the Function endpoint.

### 6.2 Topology

**No topology integration.** Flow data does not feed `network-connections`, `streaming`, or any other topology layer. The frontend has a dead hook (`useFlowsDrilldownData`) that was never wired up; SOW-0014 explicitly notes "Topology drilldown: Frontend hook is dead code (never imported), no Flows tab in topology actor modal. Not an untested feature — it simply does not exist" (SOW-0014:54-57).

### 6.3 Logs

**No flow-as-log path.** The journal storage is reused from the log pipeline but flow events do not appear in the `systemd-journal:netdata` Function or any unified-logs feed. The flows Function is a separate endpoint, with a separate schema, on a separate storage tree.

### 6.4 Alerts

**No stock health alerts.** `find src/health/health.d -iname '*netflow*' -o -iname '*flow*'` returns nothing. The chart contexts exist (`netdata.netflow.input_packets` etc.) so operators *could* write their own, but the plugin ships none — not for "exporter went silent", not for "drop rate too high", not for "template flapping".

### 6.5 Functions endpoint

Exactly one global function: **`flows:netflow`** (`api/flows/handler.rs:266-276`):

```rust
let mut func_decl = FunctionDeclaration::new("flows:netflow", "NetFlow/IPFIX/sFlow flow analysis data");
func_decl.global = true;
func_decl.tags = Some("flows".to_string());
func_decl.access = Some(HttpAccess::SIGNED_ID | HttpAccess::SAME_SPACE | HttpAccess::SENSITIVE_DATA);
func_decl.timeout = 30;
func_decl.version = Some(FLOWS_FUNCTION_VERSION);
```

Three modes (`api/flows/handler.rs:43-181`):

1. **Table mode** (default): `view`, `after`, `before`, `group_by`, `top_n`, `sort_by`, optional `selections`, returns flat flow rows + facet counts.
2. **Timeseries mode** (`is_timeseries_view()`): Top-N time-series for a group-by combination.
3. **Autocomplete mode** (`is_autocomplete_mode()`): substring autocomplete on text fields (uses an FST sidecar; offloaded to `task::spawn_blocking` to keep tokio workers free).

The handler dispatches to `FlowQueryService`, which scans the appropriate tier(s), applies selections, builds groupings, and renders columns and chart payloads. Output schema:

- `source: "netflow"`
- `layer: "3"`
- `view`, `group_by`, `columns`, `flows | metrics | chart`, `stats`, `warnings`, `facets`

This Function is consumed by the UI's "Network Flows" tab and supports the standard Netdata Functions transport (HTTP/MCP/etc.).

### 6.6 Northbound

No outbound forwarding. Netdata does not relay or re-export flow records.

---

## 7. Tests and Fixtures

### 7.1 Unit tests

Inline `#[cfg(test)]` modules across the source tree, plus dedicated files:

- `src/main_tests.rs`, `src/decoder/tests.rs`, `src/ingest_tests.rs`, `src/query/tests.rs`, `src/routing/bmp/tests.rs`, `src/routing/bioris/tests.rs`, `src/enrichment/tests.rs`, `src/network_sources/tests.rs`, `src/presentation/tests.rs`, `src/charts/tests.rs`
- `tests/grpc_build.rs` (integration test for the gRPC proto build)

The test infrastructure is rich enough to replay packets through the entire pipeline in process (`ingest/test_support.rs`).

### 7.2 Benchmarks

Two complementary benchmark suites (`README.md:313-336`):

1. **Unpaced full UDP→journal matrix** (`ingest::bench_tests::bench_ingestion_protocol_matrix`) — per-protocol single-thread throughput ceiling.
2. **Paced post-decode resource envelope** (`ingest::resource_bench_tests::bench_resource_envelope_child`) — measures achieved rate, CPU%, RSS peak, disk I/O at a configurable rate.

Both are gated by `--ignored` so they don't run in normal CI; published numbers in the README pin to a specific host.

### 7.3 Fixture pcaps

`testdata/flows/` ships **36 pcap captures** (`testdata/ATTRIBUTION.md` records origins):

```
template.pcap, options-template.pcap, data.pcap, data-1140.pcap,
datalink-template.pcap, datalink-data.pcap, data-encap-vxlan.pcap,
data-icmpv4.pcap, data-icmpv6.pcap, icmp-template.pcap,
data-local-interface.pcap, data-discard-interface.pcap, data-multiple-interfaces.pcap,
ipfix-srv6-template.pcap, ipfix-srv6-data.pcap, ipfixprobe-templates.pcap,
juniper-cpid-template.pcap, juniper-cpid-data.pcap,
multiplesamplingrates-{template,data,options-template,options-data}.pcap,
samplingrate-data.pcap, nat.pcap, mpls.pcap, physicalinterfaces.pcap,
data-sflow-raw-ipv4.pcap, data-sflow-ipv4-data.pcap, …
```

Uses `pcap-file` and `etherparse` dev-deps to replay these against the parser in-process. **This is the most directly transferable pattern for trap testing**: capture real exporter PDUs once, never again need a live device for unit-level coverage.

### 7.4 CI workflows

`find .github/workflows -name '*netflow*'` returns nothing — there is no dedicated netflow CI workflow. Tests run inside the general Rust build, gated by `ENABLE_PLUGIN_NETFLOW`.

---

## 8. Documentation

- Generated integration pages: `src/crates/netflow-plugin/integrations/{netflow,ipfix,sflow}.md` (per-protocol). All three plus 20 enrichment-feature pages are generated from `metadata.yaml` via the standard integrations pipeline.
- README inside the crate: `src/crates/netflow-plugin/README.md` (455 lines; config, retention, benchmarks, performance discussion).
- Learn pages: `docs/.map/map.yaml` carries a "Network Flows" section (SOW-0014 phase 1 work).
- `metadata.yaml` is huge (266 KB) because it inlines copious vendor configuration examples (Cisco IOS, IOS-XR, NX-OS, Juniper Junos, Mikrotik, etc.). The integrations pipeline expands this into the published integration pages plus `COLLECTORS.md` entries.

---

## 9. Plugin Framework Integration (How a Rust Plugin Lives Among Go Collectors)

This is critical for the trap design — the trap subsystem will face the same choice.

### 9.1 Binary, not in-process

`netflow-plugin` is a **separate binary**, installed to `/usr/libexec/netdata/plugins.d/netflow-plugin`, launched by the `plugins.d` orchestrator inside the Netdata Agent. It is **not** a go.d collector module, **not** a C-internal plugin, **not** a shared library.

### 9.2 plugins.d protocol

The binary speaks the `plugins.d` line-protocol over stdout (charts/dimensions/values), driven by the `rt::PluginRuntime`. Key behaviours:

- Prints `TRUST_DURATIONS 1` at startup (`main.rs:52`) — tells the parent it can pass durations as-is.
- If disabled, writes `DISABLE\n` to stdout and exits (`main.rs:110-118`).
- When stdout is **not** a TTY (normal plugins.d runtime), emits `PLUGIN_KEEPALIVE\n` every 60s to avoid parser inactivity timeouts (`main.rs:267-278`, `README.md:452-455`).
- Registers Functions via `rt::PluginRuntime::register_handler()`, which serialize/deserialize through `netdata-plugin-protocol::FunctionCall`/`FunctionResult`.

### 9.3 Function calls

The agent forwards Function calls (HTTP, MCP, Cloud) to the plugin's stdin. The plugin's tokio runtime drives `FunctionHandler::on_call` (`api/flows/handler.rs:243-257`), which spawns blocking-pool jobs for heavy work (query scan, autocomplete FST stream).

### 9.4 Lifecycle

The runtime exits if either:
- The ingest task panics or returns an error (`main.rs:292-309`), or
- The parent closes stdin / `runtime.run()` returns.

On shutdown, the listener loop finalizes tier flushes, persists decoder state, and exits cleanly (`finish_shutdown`, `ingest/service/runtime.rs:168-173`).

### 9.5 Implication for traps

A trap receiver could follow the same pattern: separate Rust (or Go) binary, plugins.d protocol, single Function endpoint (e.g., `traps:snmp`). The reusable scaffolding is the `rt` crate (Rust) or the existing `plugin/go.d` framework (Go). The flow plugin's main loop is small enough (~360 lines) to clone-and-adapt for trap UDP/162.

---

## 10. Enrichment Stack (Reference for "Source IP → Identity")

This is the closest analog to "trap source IP → device profile" mapping in the existing SNMP code.

Enrichment runs **inline on every flow** (single-threaded, all in-memory, RwLock reads only — no per-flow RPC, no per-flow disk read). Five layers, ordered:

1. **Static metadata by exporter prefix + ifIndex** — operator-defined router names, regions, roles (`enrichment/data/static_data.rs`, config `metadata_static.exporters`).
2. **Sampling rate by exporter prefix** — defaults and overrides (`SamplingRateSetting`).
3. **Akvorado-compatible classifiers** — exporter and interface match rules with a 5-minute TTL cache (`enrichment/classifiers/`, `classifier_cache_duration` config).
4. **Network attributes by IP prefix** — name/role/site/region/country/state/city/tenant/asn for source AND destination IPs (`enrichment/data/network/`). Trie-backed (`ipnet-trie` workspace dep).
5. **GeoIP** (MaxMind/DB-IP/IPDeny MMDB) — country/city/ASN per IP (`enrichment/data/geoip/`).
6. **Network sources** — periodically refreshed remote feeds (AWS, Azure, GCP, NetBox, generic JSON) transformed by an embedded jq engine into the network-attributes layer (`network_sources/`, `enrichment.network_sources` config).
7. **Routing** — static prefix-to-ASN/path map AND dynamic BMP listener + BioRIS gRPC client maintain an in-memory RIB that's looked up per flow (`routing/`).

Records that **fail** required metadata (missing exporter metadata or missing sampling rate when required) are **dropped** at the enricher boundary (`decoder/state/runtime/decode.rs:50-54`: `batch.flows.retain_mut(|flow| enricher.enrich_record(&mut flow.record))`). This is a real semantic: misconfigured exporters' flows go to `/dev/null`, not into the journal. For traps, dropping unknown sources would be a UX disaster — operators expect "I see a trap from an unconfigured device" to surface, not vanish.

---

## 11. What is Reusable for SNMP Trap Design

1. **Plugin packaging pattern** — Corrosion + CMake + `plugins.d` install (`CMakeLists.txt:3506-3517`). Adding a `snmp-traps-plugin` Rust binary alongside `netflow-plugin` is a copy-modify exercise.
2. **`rt::PluginRuntime` + `FunctionHandler`** — Direct reuse for a `traps:snmp` Function endpoint mirroring `flows:netflow`'s shape (table/timeseries/autocomplete modes).
3. **Configuration loader pattern** — `clap::Parser` derives + `serde(deny_unknown_fields)` + YAML loader + per-section validators (`plugin_config/runtime.rs`, `plugin_config/types/*`). Clean template.
4. **Journal storage stack** — `journal-log-writer::Log` with `RetentionPolicy`/`RotationPolicy`, tier accumulators, facet sidecars, lifecycle observers. Traps fit this model directly — one log entry per trap, with fields like `TRAP_OID`, `SRC_IP`, `VAR_BINDINGS`, severity. Index-first storage; query Function consumes it.
5. **Fixture pcap pattern** — capture once, replay forever (`testdata/flows/*.pcap` + `pcap-file` + `etherparse` dev deps). For traps: capture real `snmptrapd` PDUs (v1/v2c/v3) once, ship as fixtures.
6. **Template/state persistence with magic+version+xxhash** — `decoder/state/persisted.rs` is a clean template for any per-source state we need to persist (e.g., trap-source-credentials cache, last-seen state).
7. **Source-IP normalization at scope boundary** — `normalize_template_scope_source()` drops ephemeral port; same logic needed for "this trap source is this device".
8. **Memory discipline** — glibc arena cap, THP off, jemalloc-style allocator tuning, self-RSS chart family. Worth replicating for any high-volume Rust plugin.
9. **`spawn_blocking` for heavy query work** — keeps tokio reactor responsive. Trap queries on a large journal will need the same discipline.
10. **Decode→enrich→write→tier-accumulate pipeline shape** — Once decode is in hand, the rest is mostly schema-mapping and config.
11. **Akvorado-style classifier engine** — `enrichment/classifiers/` is a flexible rule engine (parse tree + cache). Traps may want similar "match severity from OID prefix" or "tag this trap as customer-impacting" rules.
12. **`flows:netflow` Function as UI surface** — exact pattern for a `traps:snmp` tab in the UI.

---

## 12. What We Would Specifically NOT Want to Copy

1. **Single-task UDP receive loop with no queue.** OK for flows (a single core saturates at ~22-25k flows/s); **not OK for traps** where storm-handling is part of the threat model. A trap subsystem needs: a small receive task that does almost nothing (recv+timestamp+enqueue), bounded mpsc to a decoder pool, **explicit drop counters with reasons** (queue full, parse error, dedup window), and SO_RCVBUF tuning.

2. **Drop-on-missing-metadata semantics.** The flow enricher silently discards flows from misconfigured exporters (`decoder/state/runtime/decode.rs:50-54`). For traps, an unknown source IP is itself a signal — a trap from a device the operator didn't configure should still appear with a "unknown_source" label, not vanish.

3. **No DynCfg.** Editing `netflow.yaml` + restart is acceptable for "I'm adding a new exporter prefix"; it is **not** acceptable for "I'm adding the credentials for a new SNMPv3 trap source" mid-flight. Trap subsystem should integrate DynCfg from day one, matching the go.d SNMP collector.

4. **No stock health alerts.** "Exporter went silent" is one of the most useful flow alerts and the plugin ships none. Traps must ship at least: "trap source silent > N minutes", "trap drop rate exceeds threshold", "trap storm detected".

5. **No source-IP-to-vnode mapping.** The flow plugin has its own `EXPORTER_*` fields (name, group, role, site, region, tenant) populated from the config's `metadata_static.exporters`. It does **not** consult the existing SNMP vnode mapping (`collector/snmp/init.go` + `vnodes` package). For traps, we should reuse the SNMP vnode mapping so a trap from `192.0.2.5` shows up under the same vnode as the polled metrics from `192.0.2.5`. Inventing a parallel identity space (as flows did) is a maintenance trap.

6. **All-fields-in-one-record schema (FlowRecord with 99 fields).** Flows have a small, fixed-ish set of fields (the IPFIX information element registry plus enrichment). Traps have **vendor-specific variable bindings** with arbitrary structure. A 99-field flat record won't model that — traps need either nested fields or a sub-store for varbinds.

7. **GeoIP/AS-path/jq-transforms as a hard dependency.** Useful for flows; almost certainly irrelevant for traps. Trap subsystem should not pull in `maxminddb`, `jaq-*`, BMP, BioRIS.

8. **Tiered rollups (1m/5m/1h).** Flows roll up cleanly because they have natural numeric dimensions (bytes, packets) to sum. Traps are discrete events — rolling them up "by minute" loses the entire forensic value of "exactly which trap fired at HH:MM:SS.fff". Keep traps as `raw` only, or only roll up counts (not contents).

9. **No northbound emission.** Traps require thought about forwarding (alert transport, audit-trail relay). Flows had no reason to forward; traps will.

10. **Documentation through one 266 KB metadata.yaml.** The flow metadata.yaml is unwieldy because of inlined vendor examples. Trap setup is simpler (point `snmptrapd` here); we don't need to clone the same structure.

---

## 13. Open Questions Surfaced by This Inventory

(These are NOT decided. They are questions the trap design must answer; the answers are not in the foundational spec yet.)

1. **Where does the trap UDP listener live?** Same crate as flows? Sister crate? Plugin-level singleton vs per-source collector? Flows chose "separate binary, single listener, separate journal tree" — defensible default for traps but worth confirming.

2. **Storage shape: journal-engine (like flows) or TSDB (like SNMP metrics)?** The flow plugin proved journal-engine works for high-volume events. Pro: indexed, queryable, retention controls, facet sidecars. Con: not the same as SNMP polled metrics, so a trap-as-metric path (counter increments) would still need to exist alongside the trap-as-event path.

3. **Identity model: separate config like flow's `metadata_static.exporters`, or reuse SNMP vnodes?** Flows invented their own identity; SNMP polling has vnodes. The trap subsystem should pick one, not invent a third.

4. **DynCfg or YAML-restart?** The flow plugin's YAML-restart model is a known regression. Trap subsystem should aim for DynCfg from day one.

5. **Backpressure model: bounded mpsc queue between socket task and decoder?** Flows have no queue, no backpressure metric, no kernel-drop counter. Traps must have all three explicitly.

6. **Storm/dedup window: in the listener, in a separate stage, or in a profile-driven rule engine?** Akvorado-style classifiers (`enrichment/classifiers/`) are a candidate engine pattern but the flow code applies them in enrichment, not at the receive boundary. Traps need to dedup BEFORE enrichment for storm protection.

7. **Function endpoint name and shape: `traps:snmp` mirroring `flows:netflow`?** Modes (table/timeseries/autocomplete) and field schema both need to be designed. The flows Function is a strong reference; the trap Function will have a different field set (TRAP_OID, VARBINDS, SEVERITY, SOURCE_VNODE, ENTERPRISE_OID).

8. **Stock alerts to ship: silent-source, drop-rate, storm-detected — what thresholds?** Flows ship none; traps must ship at least these three.

9. **Privileged port: setcap on the binary in postinst, or socket-activation via systemd, or run-as-root + drop?** Flows sidestep this entirely (port 2055); traps cannot.

10. **InformRequest handling: receive only, or also acknowledge?** RFC 2576/3414 mandate ACKs for informs. gosnmp supports both; whichever Rust crate we choose must too. Defer until decided whether the Netdata agent should ever emit network traffic in response to a received trap.

---

## 14. Evidence Trail

- Plugin location and structure: `find . -type d -iname '*netflow*' -not -path './.git/*'`, `find ./src/crates/netflow-plugin -type f -name '*.rs' | sort`
- Cargo manifest: `src/crates/netflow-plugin/Cargo.toml:1-81`
- README (config, retention, benchmarks): `src/crates/netflow-plugin/README.md:1-455`
- Main entry / runtime setup: `src/crates/netflow-plugin/src/main.rs:43-372`
- Listener config defaults: `src/crates/netflow-plugin/src/plugin_config/types/listener.rs:1-33`
- UDP receive loop: `src/crates/netflow-plugin/src/ingest/service/runtime.rs:5-45`
- Protocol dispatch (sFlow vs NetFlow vs IPFIX): `src/crates/netflow-plugin/src/decoder/state/runtime/decode.rs:14-64`, `src/crates/netflow-plugin/src/decoder/protocol/entry.rs:3-169`
- Template namespace (exporter-IP + obs-domain): `src/crates/netflow-plugin/src/decoder/state/models.rs:3-7`
- Template persistence format: `src/crates/netflow-plugin/src/decoder/state/persisted.rs:13-35`, `src/crates/netflow-plugin/src/decoder.rs:87-89`
- Function declaration: `src/crates/netflow-plugin/src/api/flows/handler.rs:266-276`
- Flow record schema (99 fields): `src/crates/netflow-plugin/src/flow/schema.rs:7-99`
- Operational charts: `src/crates/netflow-plugin/src/charts/metrics.rs:3-120`
- Journal tier setup: `src/crates/netflow-plugin/src/ingest/service.rs:1-72`, `src/crates/netflow-plugin/configs/netflow.yaml:34-65`
- Enrichment pipeline: `src/crates/netflow-plugin/src/enrichment.rs:1-86`, `src/crates/netflow-plugin/README.md:64-200`
- BMP TCP listener (separate from UDP path): `src/crates/netflow-plugin/src/routing/bmp/listener.rs:7-80`
- Packaging postinst: `packaging/cmake/pkg-files/deb/plugin-netflow/postinst`
- CMake hook: `CMakeLists.txt:189`, `CMakeLists.txt:3506-3570`
- Fixture pcaps: `find src/crates/netflow-plugin/testdata/flows -name '*.pcap' | wc -l` → 36
- No CI workflow dedicated to netflow: `find .github/workflows -name '*netflow*'` → empty
- No flow health alerts: `find src/health/health.d -iname '*netflow*' -o -iname '*flow*'` → empty
- Recent SOWs read for context: `.agents/sow/done/SOW-0014-20260506-netflow-sflow-ipfix-documentation-guide.md`, `.agents/sow/done/SOW-0015-20260508-netflow-enrichment-verification.md`
- Reference style: `.agents/sow/specs/snmp-traps/netdata-existing-snmp.md`

End of inventory.
