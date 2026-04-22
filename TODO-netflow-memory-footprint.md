## TL;DR

- Purpose: reduce `netflow.plugin` memory footprint drastically under sustained ingest, with a target of at least 30x reduction versus the currently observed catastrophic baseline on office traffic.
- Costa reported: the running plugin uses about 2.4 GB RAM for roughly 20 flows/s. That is operationally unacceptable and likely scales into failure at higher flow rates.
- This task includes: building a reproducible stress environment, generating/replaying flow traffic, measuring memory with evidence, optimizing the code, exposing Netdata charts for plugin memory accounting/breakdown, and re-measuring the improvement.
- Follow-up scope added after install: investigate the remaining 4 failing `cargo test -p netflow-plugin` tests, find their real root cause, and fix them if they are regressions or latent bugs in this worktree.
- Follow-up scope added after live validation on `2026-04-11`: the running plugin now shows about `700 MB` RSS in the local environment, and the current Netdata memory charts leave about `98%` of that RSS in `unaccounted`. This phase is to identify where that live heap goes, add subsystem-level attribution for the dominant owners, and produce a concrete runtime breakdown instead of an opaque `unaccounted` bucket.
- Follow-up scope added after commit `d6e4ff75c0` on `2026-04-11`: rerun the full relevant test suites on the committed memory/rebuild changes, fix any regressions immediately, and then continue with the unresolved live anonymous `mmap_in_use` owner until it is concretely attributed and reduced.
- Follow-up scope added after the retention-policy install on `2026-04-12`: review every PR change that touches shared components outside `netflow-plugin`, verify whether each one is truly necessary for the netflow work, identify any shared-layer risk or overreach, and then define a credible performance-testing plan for NetFlow/sFlow/IPFIX ingestion.

## Analysis

### Verified code paths

- The plugin process instantiates all major in-memory subsystems at startup in [`src/crates/netdata-netflow/netflow-plugin/src/main.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/main.rs):
  - `IngestMetrics`
  - `OpenTierState`
  - `TierFlowIndexStore`
  - `FacetRuntime`
  - `FlowQueryService`
- The plugin now exposes internal memory charts in [`src/crates/netdata-netflow/netflow-plugin/src/charts/metrics.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/charts/metrics.rs), [`src/crates/netdata-netflow/netflow-plugin/src/charts/runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/charts/runtime.rs), and [`src/crates/netdata-netflow/netflow-plugin/src/charts/snapshot.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/charts/snapshot.rs):
  - `netflow.memory_resident_bytes`
  - `netflow.memory_accounted_bytes`
  - `netflow.memory_tier_index_bytes`
- Verified charting gap on `2026-04-11`:
  - the runtime sampler in [`src/crates/netdata-netflow/netflow-plugin/src/charts/runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/charts/runtime.rs) only reads `/proc/self/status` and `/proc/self/smaps_rollup`
  - this means it can capture totals like `rss`, `rss_anon`, `rss_file`, and `anon_huge_pages`, but it cannot classify which mappings own those bytes
  - as a result, the live `memory_accounted_bytes` chart still reports about `97.6%` of RSS as `unaccounted`, even though `/proc/<pid>/maps` and `/proc/<pid>/smaps` clearly show bounded raw/1m journal mmaps, heap, and large anonymous mappings as separate owners
- Verified live state after installing the mmap-backed GeoIP build on `2026-04-11` late evening:
  - the currently running plugin (`PID 1874329`) is now:
    - `VmRSS = 86,204 kB`
    - `RssAnon = 27,672 kB`
    - `RssFile = 58,532 kB`
    - `Threads = 8`
    - `AnonHugePages = 0 kB`
  - `/proc/1874329/maps` now shows the GeoIP databases as shared read-only file mappings instead of copied anonymous memory:
    - `/usr/share/netdata/topology-ip-intel/topology-ip-geo.mmdb`
    - `/var/cache/netdata/topology-ip-intel/topology-ip-asn.mmdb`
  - `/proc/1874329/smaps` shows current resident usage for those mappings at:
    - `topology-ip-geo.mmdb`: `29,116 kB RSS`
    - `topology-ip-asn.mmdb`: `5,960 kB RSS`
  - direct implication:
    - the old large anonymous `~138.5 MB` owner is gone in the live process
    - the GeoIP optimization is now proven live, but as a file-backed/shared mapping reduction, not as an anonymous-memory reduction inside the old binary
  - current top resident classes from live `pmap` / `smaps` are now:
    - main anonymous heap/arena mapping: about `26,024 kB RSS`
    - GeoIP geo MMDB: about `29,116 kB RSS`
    - GeoIP ASN MMDB: about `5,960 kB RSS`
    - the rest split across smaller journal and file-backed mappings
- Verified runtime/chart startup attribution on `2026-04-11`:
  - the startup profiling harness now exercises the real plugin-runtime path on in-memory streams:
    - constructs `PluginRuntime`
    - registers the `flows` handler
    - registers charts
    - starts runtime/chart emission long enough to initialize and emit
  - release measurement on the live dataset snapshot:
    - `plugin_runtime_configured delta_rss = 37,027,840`
    - `plugin_runtime_started delta_rss = 37,359,616`
    - incremental runtime+chart cost over the already settled rebuild state: about `331,776 bytes`
  - implication:
    - the plugin framework path is not the missing large memory owner
    - the remaining steady-state memory is explained by the already loaded runtime state plus file-backed MMDB/journal residency, not by a hidden `PluginRuntime` explosion
- Verified dominant live anonymous-memory owner on `2026-04-11`:
  - the running plugin shows one large anonymous mapping at about `138.5 MB` RSS, and the allocator chart reports `mmap_in_use ≈ 143.2 MB`
  - the stock runtime config auto-detects GeoIP MMDB files even when paths are omitted in YAML:
    - [`src/crates/netdata-netflow/netflow-plugin/src/plugin_config/runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/plugin_config/runtime.rs)
    - [`/usr/lib/netdata/conf.d/netflow.yaml`](/usr/lib/netdata/conf.d/netflow.yaml)
  - the plugin currently loads GeoIP databases with `Reader<Vec<u8>>`, which copies the full files into anonymous RAM:
    - [`src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/files.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/files.rs)
    - [`src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/types.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/types.rs)
  - the detected database sizes on this workstation are:
    - `/var/cache/netdata/topology-ip-intel/topology-ip-asn.mmdb = 10,402,916 bytes`
    - `/usr/share/netdata/topology-ip-intel/topology-ip-geo.mmdb = 131,453,438 bytes`
    - combined = `141,856,354 bytes`
  - this nearly matches the live anonymous `mmap_in_use` / `anon_other` footprint, making GeoIP DB loading the strongest verified remaining owner
  - the local `maxminddb 0.25.0` crate already supports `Reader::open_mmap()` behind its `mmap` feature, so this is optimizable without changing user-facing enrichment behavior
  - superseded by the later live result above:
    - after the mmap-backed build was actually installed and running, the large anonymous owner disappeared and the MMDBs moved to file-backed residency as expected
- Ingestion writes every decoded flow to the raw journal and also updates:
  - facet state on each raw write in [`src/crates/netdata-netflow/netflow-plugin/src/ingest/service/runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest/service/runtime.rs)
  - materialized tier accumulators and tier flow indexes in [`src/crates/netdata-netflow/netflow-plugin/src/ingest/service/tiers.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest/service/tiers.rs)
- `FacetRuntime` stores global facet vocabularies and active-file contributions in memory in [`src/crates/netdata-netflow/netflow-plugin/src/facet_runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/facet_runtime.rs).
- Facet storage includes high-cardinality fields such as `SRC_ADDR`, `DST_ADDR`, `EXPORTER_IP`, ports, ASNs, and text labels, via [`src/crates/netdata-netflow/netflow-plugin/src/facet_catalog.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/facet_catalog.rs).
- The facet store keeps unique values in memory using:
  - `TextValueStore`
  - `DenseBitSet`
  - `RoaringTreemap`
  - `IpValueStore`
  in [`src/crates/netdata-netflow/netflow-plugin/src/facet_runtime/store.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/facet_runtime/store.rs).
- Active facet contributions are stored per active journal file as `BTreeMap<String, BTreeSet<String>>` in [`src/crates/netdata-netflow/netflow-plugin/src/facet_runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/facet_runtime.rs).
- Materialized tier indexes keep one `FlowIndex` per active hour bucket in [`src/crates/netdata-netflow/netflow-plugin/src/tiering/index/store.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/tiering/index/store.rs).
- Open tier rows snapshot all currently open aggregate rows into vectors in [`src/crates/netdata-netflow/netflow-plugin/src/tiering/model.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/tiering/model.rs) and [`src/crates/netdata-netflow/netflow-plugin/src/ingest/service/tiers.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest/service/tiers.rs).

### Verified existing test assets

- There are existing NetFlow/IPFIX/sFlow PCAP fixtures under [`src/crates/netdata-netflow/netflow-plugin/testdata/flows`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/testdata/flows).
- There is already end-to-end UDP replay test scaffolding in [`src/crates/netdata-netflow/netflow-plugin/src/main_tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/main_tests.rs), including:
  - UDP listener reservation
  - PCAP payload extraction
  - fixture replay over UDP

### Measured hotspot evidence

- A focused synthetic facet stress test now exists in [`src/crates/netdata-netflow/netflow-plugin/src/memory_tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/memory_tests.rs) and was executed with:
  - `cargo test -p netflow-plugin stress_profile_high_cardinality_facet_runtime_memory -- --ignored --nocapture`
  - Baseline result before optimization: RSS grew from `9,715,712` to `30,953,472` bytes.
  - Baseline delta: `21,237,760` bytes for `50,000` synthetic high-cardinality flows.
  - Current result after the facet/runtime storage refactors: RSS grew from `10,420,224` to `18,903,040` bytes.
  - Current delta: `8,482,816` bytes for the same `50,000` synthetic high-cardinality flows.
  - Verified focused improvement for this hotspot: about `2.50x` lower RSS delta (`21.24 MB -> 8.48 MB`), still far from the requested overall `30x` target.
- The same file contains a focused tier index stress test, executed with:
  - `cargo test -p netflow-plugin stress_profile_high_cardinality_tier_index_memory -- --ignored --nocapture`
  - Previous measured hotspot before the sparse row/index work: RSS grew from `6,823,936` to `29,032,448` bytes.
  - Previous delta: `22,208,512` bytes for `50,000` synthetic high-cardinality flows.
  - Current result after sparse/default-aware rollup rows and IPv4-specific IP storage: RSS grew from `6,967,296` to `16,265,216` bytes.
  - Current delta: `9,297,920` bytes for `50,000` synthetic high-cardinality flows.
  - Internal accounted heap for this subsystem is now `7,481,034` bytes with this measured breakdown:
    - row storage: `5,505,308`
    - field stores: `1,685,818`
    - flow lookup: `286,720`
    - schema: `2,668`
    - hour-key index metadata: `8`
    - scratch field ids: `512`
  - Verified focused improvement for this hotspot: about `2.39x` lower RSS delta (`22.21 MB -> 9.30 MB`).
- The end-to-end synthetic ingest stress harness in the same file was executed with:
  - `cargo test -p netflow-plugin stress_profile_high_cardinality_netflow_memory -- --ignored --nocapture`
  - Current result: RSS peaked at `19,611,648` bytes from a `11,640,832` byte baseline.
  - Current peak delta: `7,970,816` bytes on the `5,000` flow synthetic ingest harness.
- A focused decoder source-port churn stress test now exists in [`src/crates/netdata-netflow/netflow-plugin/src/memory_tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/memory_tests.rs) and was executed before and after normalizing decoder parser scope away from raw UDP source ports:
  - `cargo test -p netflow-plugin stress_profile_decoder_source_port_churn_memory -- --ignored --nocapture`
  - Before normalization:
    - RSS grew from `7,118,848` to `875,028,480` bytes.
    - Delta: `867,909,632` bytes.
    - Parser scopes: `v9=20,000`.
  - After normalization:
    - RSS grew from `7,401,472` to `9,420,800` bytes.
    - Delta: `2,019,328` bytes.
    - Parser scopes: `v9=1`.
  - Verified focused improvement for this hotspot: about `430x` lower RSS delta on the same workload, caused by collapsing parser/template scope for one exporter IP across source-port churn.
- The top facet cardinalities observed from the facet stress test were:
  - `DST_ADDR`: `50,000`
  - `SRC_ADDR`: `50,000`
  - `SRC_PORT`: `50,000`
  - `EXPORTER_IP`: `49,805`
  - `DST_PORT`: `13,107`
  - `DST_AS`: `4,096`
  - `DST_AS_NAME`: `4,096`
  - `IN_IF`: `4,096`
  - `OUT_IF`: `4,096`
  - `SRC_AS`: `4,096`
  - `SRC_AS_NAME`: `4,096`
  - `EXPORTER_NAME`: `256`

### Live allocator and runtime evidence on 2026-04-11

- The installed plugin now exports allocator-specific charts in addition to the existing resident/accounted charts:
  - `netflow.memory_allocator_bytes`
  - `netflow.memory_accounted_bytes`
  - `netflow.memory_resident_bytes`
- Before the latest allocator-trim hook, the running plugin reported:
  - `rss = 636,342,300`
  - `unaccounted = 623,447,500`
  - allocator view:
    - `heap_arena = 488,673,300`
    - `heap_free = 421,649,500`
    - `heap_in_use = 67,023,820`
    - `mmap_in_use = 165,027,950`
    - `releasable = 15,486,688`
- After adding a post-facet-reconcile `malloc_trim(0)` hook in [`src/crates/netdata-netflow/netflow-plugin/src/query/service.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/query/service.rs), the running plugin dropped to about half the live RSS:
  - `rss = 309,284,900`
  - `unaccounted = 296,349,500`
  - allocator view:
    - `heap_arena = 478,384,100`
    - `heap_free = 411,117,500`
    - `heap_in_use = 67,266,620`
    - `mmap_in_use = 165,027,950`
    - `releasable = 26,359,620`
- Kernel/process evidence for the running post-trim process (`PID 1267128`) shows the remaining memory is still dominated by anonymous mappings:
  - `/proc/1267128/smaps_rollup`:
    - `Rss: 303,888 kB`
    - `Private_Dirty: 288,944 kB`
    - `AnonHugePages: 188,416 kB`
  - top resident mappings:
    - unlabeled anonymous mapping: `128,376 kB RSS`, `124,928 kB AnonHugePages`
    - `[heap]`: `85,460 kB RSS`, `26,624 kB AnonHugePages`
    - anonymous mapping at `7f0120000000-7f01221e5000`: `33,972 kB RSS`
    - anonymous mapping at `7f00f8000000-7f00fc000000`: `20,604 kB RSS`
    - anonymous mapping at `7f011c000000-7f0120000000`: `14,096 kB RSS`
- These large anonymous mappings are aligned and sized like glibc secondary arena mappings, not like flow journals.
- The running process has far more Tokio threads than the plugin workload appears to justify:
  - `/proc/1267128/status`: `Threads: 69`
  - CPU allowance for the process: `Cpus_allowed_list: 0-23`
  - `comm` entries show almost all threads are `tokio-runtime-w`
  - this is strong evidence that allocator arenas and thread-pool sizing are a major remaining memory driver
- A release-profile startup harness was run against the live journal directory with:
  - `env MALLOC_ARENA_MAX=1 cargo test --release -p netflow-plugin stress_profile_live_startup_memory -- --ignored --nocapture`
  - `env MALLOC_ARENA_MAX=4 cargo test --release -p netflow-plugin stress_profile_live_startup_memory -- --ignored --nocapture`
- Both release harness runs were effectively identical because the harness uses `#[tokio::test(flavor = "current_thread")]` and does not reproduce the live multithreaded runtime:
  - `MALLOC_ARENA_MAX=1`: `49,917,952` byte final RSS delta
  - `MALLOC_ARENA_MAX=4`: `49,885,184` byte final RSS delta
- Conclusion from this evidence:
  - the on-disk journal/facet restore path explains about `50 MB`
  - the remaining `~250 MB` live gap is now most likely runtime-thread/allocator behavior, not just retained flow/journal structures

### Rebuild-path root cause verified on 2026-04-11

- The earlier startup harness was incomplete because it stopped at `IngestService::new_with_facet_runtime()` and never ran the production startup rebuild path:
  - [`src/crates/netdata-netflow/netflow-plugin/src/ingest/service/runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest/service/runtime.rs) calls `self.rebuild_materialized_from_raw().await?` before the UDP receive loop starts.
- A new multithreaded release harness was added in [`src/crates/netdata-netflow/netflow-plugin/src/startup_memory_tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/startup_memory_tests.rs) to reproduce the real startup path, thread pools, allocator counters, and rebuild step.
- Verified pre-fix rebuild behavior with the multithread harness:
  - `env NETFLOW_PROFILE_WORKER_THREADS=24 NETFLOW_PROFILE_MAX_BLOCKING_THREADS=64 cargo test --release -p netflow-plugin stress_profile_live_startup_memory_multithreaded -- --ignored --nocapture`
  - rebuild phase before the latest fixes:
    - RSS: about `637 MB`
    - threads: `116`
    - `heap_in_use`: about `56 MB`
    - `heap_free`: about `603 MB`
  - This proved the catastrophic resident growth was primarily allocator retention created during rebuild, not live in-memory NetFlow state.
- Verified that trimming only after facet reconcile was insufficient:
  - rebuild still settled around `272-286 MB` RSS even after the first rebuild trim
  - a second trim after a `12s` settle window recovered only about `0-5 MB`
- The remaining long-lived owner was identified as the global Rayon pool used by journal indexing:
  - [`src/crates/journal-engine/src/indexing.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-engine/src/indexing.rs) used `into_par_iter()` on Rayon’s global pool inside `batch_compute_file_indexes()`
  - multithread harness after rebuild settled at `31` threads with `workers=4`, which matches `7` Tokio threads plus a persistent `24`-thread Rayon global pool
- Fix implemented:
  - [`src/crates/netdata-netflow/netflow-plugin/src/ingest/rebuild.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest/rebuild.rs)
    - trim glibc heap after raw rebuild completes
    - keep rebuild-only heavy objects scoped so they are eligible for release before the post-rebuild trim
  - [`src/crates/journal-engine/src/indexing.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-engine/src/indexing.rs)
    - replace Rayon global-pool indexing with a bounded local Rayon pool capped at `4` threads
- Verified post-fix rebuild behavior:
  - `workers=4`, `max_blocking=8`
    - rebuild RSS: about `137 MB`
    - rebuild settle RSS: about `137 MB`
    - threads after settle: `7`
  - `workers=24`, `max_blocking=64`
    - rebuild RSS: about `152 MB`
    - rebuild settle RSS: about `151 MB`
    - threads after settle: `27`
- Net effect of the verified rebuild-path fixes on the same local journal directory:
  - from about `637 MB` rebuild RSS down to about `152 MB` with the runtime shape closest to the real service on this host
  - about `4.2x` lower resident memory for the verified catastrophic startup path
  - from `116` threads down to `27` after rebuild settles

### Verified structural causes

- `FacetRuntime` used to keep the same logical vocabulary in multiple resident forms in [`src/crates/netdata-netflow/netflow-plugin/src/facet_runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/facet_runtime.rs):
  - before the refactor it kept `archived_fields`, `fields`, and `active_contributions`
  - after the refactor it keeps `archived_fields`, `active_fields`, `active_contributions`, and a lightweight published snapshot
- `active_contributions` no longer stores per-file values as `BTreeSet<String>`. It now uses typed `FacetStore` containers, so IPs, ports, and ASNs stay in compact representations instead of always becoming heap strings.
- The full duplicate combined facet store (`fields`) has been removed from runtime memory. The plugin now keeps archived stores, active stores, per-file active contributions, and lightweight published metadata instead.
- The tier index hot path in [`src/crates/netdata-netflow/netflow-plugin/src/tiering/index/store.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/tiering/index/store.rs) uses the custom `FlowIndex` in [`src/crates/netdata-netflow/flow-index/src/lib.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/flow-index/src/lib.rs), which interns field values but still stores a full `u32` field-id tuple per unique rollup flow.
- The tier index now uses sparse/default-aware rollup rows instead of a fully dense `u32` tuple per stored flow, and it uses a smaller IPv4-specific representation for IP field dictionaries in the `FlowIndex` crate.
- The facet IP store now uses bitmap-backed IPv4 sets and explicit IPv6 storage instead of a generic 17-byte packed representation for all IP addresses.
- The local frontend gap notes already document that high-cardinality facets should not be fully enumerated inline and instead need text-input or autocomplete behavior in [`/home/costa/src/dashboard/cloud-frontend/TODO-flows-gaps.md`](/home/costa/src/dashboard/cloud-frontend/TODO-flows-gaps.md).
- The upstream `netflow_parser::scoped_parser::AutoScopedParser` keys parser/template caches by full `SocketAddr`, not exporter identity:
  - `ipfix_parsers: HashMap<IpfixSourceKey, NetflowParser>`
  - `v9_parsers: HashMap<V9SourceKey, NetflowParser>`
  - `legacy_parsers: HashMap<SocketAddr, NetflowParser>`
  in [`/home/costa/.cargo/registry/src/index.crates.io-1949cf8c6b5b557f/netflow_parser-0.9.0/src/scoped_parser.rs`](/home/costa/.cargo/registry/src/index.crates.io-1949cf8c6b5b557f/netflow_parser-0.9.0/src/scoped_parser.rs).
- The plugin also tracks hydrated decoder namespaces per full `SocketAddr` in [`src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/init.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/decoder/state/runtime/init.rs) and [`src/crates/netdata-netflow/netflow-plugin/src/ingest/persistence.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest/persistence.rs).

### Shared-component review on 2026-04-12

- Reviewed shared crates changed by the PR against `origin/master`:
  - `journal-core`
  - `journal-engine`
  - `journal-log-writer`
  - `netdata-plugin/rt`
  - `journal-common`
  - `jf/journal_file`
- Shared changes that are justified by the netflow work:
  - [`src/crates/journal-core/src/file/mmap.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-core/src/file/mmap.rs)
    - required to stop append-heavy writer windows from growing from file start toward file tail, which matched the live journal RSS explosion
  - [`src/crates/journal-engine/src/indexing.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-engine/src/indexing.rs)
    - `without_disk_cache()` is required by [`src/crates/netdata-netflow/netflow-plugin/src/ingest/rebuild.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest/rebuild.rs) to avoid rebuild-time cache files and resident disk-cache overhead
    - the bounded local Rayon pool is required to eliminate the persistent global indexing pool that previously inflated rebuild threads and allocator arenas
  - [`src/crates/journal-engine/src/logs/query.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-engine/src/logs/query.rs)
    - the `merge_log_entries()` allocation fix is required because netflow rebuild executes an unlimited `LogQuery::execute()` path in [`src/crates/netdata-netflow/netflow-plugin/src/ingest/rebuild.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest/rebuild.rs)
  - [`src/crates/journal-core/src/file/reader.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-core/src/file/reader.rs)
    - `build_filter()` is directly consumed by netflow direct journal scans in [`src/crates/netdata-netflow/netflow-plugin/src/query/scan/direct.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/query/scan/direct.rs)
  - [`src/crates/journal-log-writer/src/log/mod.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-log-writer/src/log/mod.rs), [`src/crates/journal-log-writer/src/log/chain.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-log-writer/src/log/chain.rs), and [`src/crates/journal-common/src/time.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-common/src/time.rs)
    - required for event-time journal writes (`EntryTimestamps` with realtime override), active-file visibility, and lifecycle notifications used by the facet runtime
  - [`src/crates/netdata-plugin/rt/src/netdata_env.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-plugin/rt/src/netdata_env.rs)
    - `stock_data_dir` is required by netflow GeoIP auto-detection in [`src/crates/netdata-netflow/netflow-plugin/src/plugin_config/runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/plugin_config/runtime.rs)
- Shared changes that look like scope expansion rather than netflow necessity:
  - [`src/crates/netdata-plugin/rt/src/lib.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-plugin/rt/src/lib.rs)
    - the `flows:*` GET-args-to-JSON shim is netflow-specific behavior inside the shared runtime
    - `ProgressState::snapshot()`, cancellation `499` behavior, and their new tests are not required by the production netflow path; current references are only test-side
  - [`src/crates/journal-engine/src/logs/query.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-engine/src/logs/query.rs)
    - `with_output_fields()` is not used by netflow production code; repository search found only self-tests as callers
  - [`src/crates/journal-core/src/file/file.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-core/src/file/file.rs) and [`src/crates/jf/journal_file/src/file.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/jf/journal_file/src/file.rs)
    - the zero-offset iterator hardening is a valid defensive fix, but it is not tied to a reproduced netflow requirement
    - the `jf/journal_file` copy is especially outside the netflow scope
- Netflow-local extraction that is acceptable and should stay:
  - [`src/crates/netdata-netflow/flow-index/src/lib.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/flow-index/src/lib.rs)
    - this is a netflow-owned crate, not a general shared runtime/library change
    - it cleanly isolates the compact rollup/grouping index logic and memory accounting used by the plugin
- This creates a verified memory-risk pattern: if the same exporter IP changes UDP source port over time, parser/template caches can grow by source-port churn even when exporter identity is operationally unchanged.
- The plugin entrypoint in [`src/crates/netdata-netflow/netflow-plugin/src/main.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/main.rs) uses `#[tokio::main]`, which means:
  - Tokio chooses the multithread runtime defaults automatically
  - worker thread count is not explicitly bounded for this plugin
  - blocking-thread pool limits are not explicitly bounded for this plugin
- This matters because the live process currently shows `69` threads, mostly `tokio-runtime-w`, on a machine where `nproc` and `getconf _NPROCESSORS_ONLN` both report `24`.
- The current startup harness therefore underestimates live memory by construction, because it uses `current_thread` runtime flavor and never reproduces the production thread-pool shape.

### Current accounted facet breakdown

- The focused facet profile now exposes an internal accounted breakdown from `FacetRuntime::estimated_memory_breakdown()`:
  - archived facet stores: `972,497` bytes
  - active facet stores: `572,521` bytes
  - active per-file contributions: `505,107` bytes
  - published snapshot metadata: `3,005` bytes
  - archived path tracking: `152` bytes
- This shows the duplicate combined store is gone and high-cardinality IP storage is materially smaller, but the total measured reduction is still nowhere near the requested `30x`.
- Re-validated on `2026-04-11` after the latest live-memory fixes:
  - `cargo test -p netflow-plugin stress_profile_high_cardinality_facet_runtime_memory -- --ignored --nocapture`
  - Measured RSS growth: `8,966,144` bytes
  - Current facet-accounted total: `2,053,282` bytes
  - Verified undercount factor on this focused harness: about `4.37x`
  - This matters because the live process currently reports only about `12 MB` of facet memory in charts, while the startup harness and this focused profile strongly suggest the real facet footprint is materially larger.
- Re-validated on `2026-04-11` with the live-startup multithread harness after disabling rebuild disk cache:
  - `cargo test -p netflow-plugin --release stress_profile_live_startup_memory_multithreaded -- --ignored --nocapture`
  - `facet_runtime_new` raised RSS by `48,795,648` bytes from baseline
  - `query_service_new` added only `856,064` bytes more
  - `ingest_service_new` added only `684,032` bytes more
  - `rebuild_materialized_from_raw` still settles around `151.8 MB` RSS
  - This confirms the remaining live `~315 MB` process is not just startup rebuild; a large steady-state owner remains, and facet memory accounting is one verified blind spot.
- Re-validated on `2026-04-11` after removing the duplicate active store and making `DenseBitSet` lazy:
  - `cargo test -p netflow-plugin stress_profile_high_cardinality_facet_runtime_memory -- --ignored --nocapture`
  - focused facet RSS delta improved from `8,966,144` bytes to `8,302,592` bytes
  - a later compact persisted-text experiment improved this focused harness only marginally again (`8,302,592 -> 8,228,864` bytes), which is too small to justify a schema change by itself
  - `NETFLOW_PROFILE_SETTLE_SECS=20 cargo test -p netflow-plugin --release stress_profile_live_startup_memory_multithreaded -- --ignored --nocapture`
  - startup `rebuild_settle_trim` improved materially with the validated active-store removal and lazy bitset work: about `145.36 MB -> 124.69 MB`
- Re-validated on `2026-04-11` after removing internal timestamp facets from the user-facing catalog and request allowlists:
  - `env MALLOC_ARENA_MAX=1 NETFLOW_PROFILE_SETTLE_SECS=5 NETFLOW_PROFILE_DISABLE_THP=1 cargo test -p netflow-plugin --release stress_profile_live_startup_memory_multithreaded -- --ignored --nocapture`
  - measured startup phases on the retained live dataset:
    - `facet_runtime_new delta_rss = 10,301,440`
    - `query_service_new delta_rss = 11,055,104`
    - `ingest_service_new delta_rss = 11,673,600`
    - `rebuild_materialized_from_raw delta_rss = 15,089,664`
    - `rebuild_settle_trim delta_rss = 15,052,800`
  - measured rebuild-time facet breakdown:
    - `archived = 3,598,723`
    - `published = 13,812`
    - `archived_paths = 55,050`
  - implication:
    - the archived timestamp-field removal is visible not only in deep allocative accounting but also in the startup harness; archived facet state is now a much smaller contributor during rebuild/load
- Verified profiling confounder on `2026-04-11`:
  - the installed live plugin at [`/usr/libexec/netdata/plugins.d/netflow-plugin`](/usr/libexec/netdata/plugins.d/netflow-plugin) is still older code and continues writing `/var/cache/netdata/flows/facet-state.bin`
  - direct decode of the live `facet-state.bin` under the experimental compact persisted-text schema failed with `UnexpectedEof`
  - raw bytes inspection showed the file header still begins with `04 00 00 00`, consistent with the older on-disk schema, not the experimental bumped version
  - conclusion: startup profiling against `/var/cache/netdata/flows` is contaminated for persisted-state comparisons until the running service is updated or profiling is moved to an isolated snapshot
  - action taken: reverted the unproven compact persisted-text schema change and kept only the validated in-memory facet optimizations

## Feasibility Assessment

### Assumptions checked

1. Assumption: the plugin can be run in a local isolated environment without touching production services.
   - Verified: `PluginConfig::new()` supports Netdata-style config discovery from environment variables in [`src/crates/netdata-netflow/netflow-plugin/src/plugin_config/runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/plugin_config/runtime.rs) and [`src/crates/netdata-plugin/rt/src/netdata_env.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-plugin/rt/src/netdata_env.rs).
2. Assumption: we can generate realistic ingest load without office routers.
   - Verified: existing PCAP fixtures and replay helpers already provide a base path for UDP ingestion tests in [`src/crates/netdata-netflow/netflow-plugin/src/main_tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/main_tests.rs).
3. Assumption: we can profile memory locally.
   - Verified: `/usr/bin/time`, `perf`, and `valgrind` are available on this machine.
4. Assumption: the current worktree is clean enough to make isolated changes.
   - Verified false: `git status --short` now shows tracked changes in the netflow plugin crate plus untracked local helper files (`TODO-netflow-memory-footprint.md`, `build-install-netflow-plugin.sh`).
   - This is not a blocker because the modified files are scoped to the current task and the untracked files are intentionally left out of commits.

### Feasibility verdict

FEASIBLE AS SPECIFIED

The codebase already contains the pieces needed to build a reproducible stress/profiling harness and to validate memory reductions locally before reporting results.

## Decisions

- Decision made by Costa on `2026-04-12`: clean up unnecessary shared-component changes before proceeding with flow-ingestion performance work.
  - Scope to clean first:
    - netflow-specific `flows:*` request parsing currently living in shared `rt`
    - shared-library additions that are not required by the netflow production path
    - duplicated/shared defensive fixes that do not benefit the netflow path and only expand review surface
  - Purpose:
    - keep the PR reviewable
    - keep shared crates generic
    - ensure the later performance-testing work is built on the code we actually intend to upstream
  - Implemented on `2026-04-12`:
    - moved the `flows:*` GET-argument compatibility shim out of shared [`src/crates/netdata-plugin/rt/src/lib.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-plugin/rt/src/lib.rs) and into netflow-local request parsing in [`src/crates/netdata-netflow/netflow-plugin/src/api/flows/handler.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/api/flows/handler.rs)
    - removed unused shared `journal-engine` API surface `LogQuery::with_output_fields()` from [`src/crates/journal-engine/src/logs/query.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-engine/src/logs/query.rs)
    - removed the duplicated zero-offset iterator hardening and test from the out-of-path [`src/crates/jf/journal_file/src/file.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/jf/journal_file/src/file.rs)
  - Verification on `2026-04-12`:
    - `cargo test -p rt --manifest-path src/crates/Cargo.toml` passed
    - `cargo test -p journal-engine --manifest-path src/crates/Cargo.toml` passed
    - `cargo test -p journal-core --manifest-path src/crates/Cargo.toml` passed
    - `cargo test -p journal-log-writer --manifest-path src/crates/Cargo.toml` passed
    - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml` passed with `388 passed, 0 failed, 14 ignored`
    - standalone duplicate crate check:
      - `cargo test --manifest-path src/crates/jf/journal_file/Cargo.toml writer::tests::test_write_and_read_journal_entries -- --nocapture`
      - failed both on the cleanup worktree and on baseline checkpoint commit `973f200dd5`
      - panic is in [`src/crates/jf/journal_file/src/writer.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/jf/journal_file/src/writer.rs) at line `637`, not in the reverted iterator code
      - implication: this `jf` failure is pre-existing noise for the duplicated crate, not a regression introduced by the cleanup
- Decision made autonomously on `2026-04-12` for the first ingestion-performance pass:
  - implement the benchmark as ignored `cargo test` harnesses inside `netflow-plugin`, not as Criterion benches yet
  - Evidence:
    - the repo already uses ignored profiling tests for startup, memory, and query profiling
    - the existing ingest throughput harness was already in [`src/crates/netdata-netflow/netflow-plugin/src/ingest_tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest_tests.rs)
    - nearby flow-parser projects such as the mirrored NetGauze parser benchmark by protocol and packet shape, which this implementation now mirrors operationally without introducing new bench dependencies
  - Implication:
    - the first pass is easy to run in CI-like environments with `cargo test --release ... --ignored --nocapture`
    - if we later need statistical benchmarking or regression thresholds, we can still add Criterion on top of the same scenarios
- Decision made by Costa on `2026-04-11`: proceed autonomously with diagnosis and optimization of the remaining live memory footprint until the dominant owners are concretely attributed and reduced.
- Decision pending on `2026-04-11`: whether to change the shared journal window-manager logic in [`src/crates/journal-core/src/file/mmap.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-core/src/file/mmap.rs) as part of the netflow memory work.
  - Context:
    - this logic is shared infrastructure, not netflow-only code
    - live evidence currently shows one large `rw-s` mapping per active tier journal file, which matches the current remap behavior in the shared window manager
    - changing shared journal infrastructure could reduce resident memory for netflow and any other current/future users of the same writer path, but it must be treated as an infrastructure fix, not a plugin-local tweak
- If implementation reveals that the only viable 30x reduction requires changing query/facet behavior visible to users, I must stop and present options with concrete impact before proceeding.
- Decision made by Costa on `2026-04-11`: proceed autonomously with the recommended attribution path for the live `~700 MB` RSS case:
  - inspect the live allocator/process footprint first
  - expand the plugin's internal accounting for major in-memory subsystems
  - use stress-harness profiling to map large allocations back to code paths if the live allocator/process view is not sufficient
- Decision made by Costa on `2026-04-11`: choose the implementation and tooling path autonomously using best practices. Do not escalate Rust/allocator implementation details back to Costa unless there is a real product-level tradeoff. Allocator replacement is allowed if evidence shows it materially improves accounting or production memory behavior, but it must not be used blindly to hide ownership bugs.
- Operational decision made by Costa on `2026-04-11`: when Netdata service restarts are needed on this workstation, use `sudo` so the authentication flow is explicit and does not rely on a background privilege escalation path.
- Decision made autonomously on `2026-04-11` after direct benchmark evidence: keep the plugin on glibc and do not switch to mimalloc, jemalloc, or tcmalloc for this workload.
  - Evidence from the release startup harness on the retained live dataset:
    - glibc with `MALLOC_ARENA_MAX=1`: `rebuild_settle_trim delta_rss = 45,494,272`
    - mimalloc (`LD_PRELOAD=/usr/lib/libmimalloc.so`): `99,500,032`
    - jemalloc (`LD_PRELOAD=/usr/lib/libjemalloc.so`): `303,493,120`
    - tcmalloc (`LD_PRELOAD=/usr/lib/libtcmalloc_minimal.so`): `278,237,184`
  - Implication:
    - allocator replacement is not the right optimization path here; the right path is reducing transient allocations and capping glibc arena growth inside the process.
- Decision made by Costa on `2026-04-11`: remove exact timestamp-microsecond fields such as `FLOW_START_USEC`, `FLOW_END_USEC`, and `OBSERVATION_TIME_MILLIS` from the facet/query catalog entirely.
  - Context:
    - the field is currently configured as `SparseU64`, `supports_autocomplete = true`, and `uses_sidecar = true` in [`src/crates/netdata-netflow/netflow-plugin/src/facet_catalog.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/facet_catalog.rs)
    - archived facet autocomplete/search uses sidecars when a field is marked this way in [`src/crates/netdata-netflow/netflow-plugin/src/facet_runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/facet_runtime.rs)
    - a targeted allocative diagnostic on the live dataset showed archived facet memory of `40,524,572` bytes total, of which `FLOW_END_USEC` alone consumed `32,263,968` bytes
  - Implication:
    - these fields will no longer be available as facet fields, autocomplete targets, or selection/filter fields for user requests
    - internal chart logic that reads `FLOW_END_USEC` directly from records remains unaffected
  - Implemented on `2026-04-11`:
    - removed from facet catalog and from query request allowlists in [`src/crates/netdata-netflow/netflow-plugin/src/facet_catalog.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/facet_catalog.rs) and [`src/crates/netdata-netflow/netflow-plugin/src/query/request/constants.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/query/request/constants.rs)
    - added regressions for unsupported `group_by`, `selection`, and autocomplete requests in [`src/crates/netdata-netflow/netflow-plugin/src/query/tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/query/tests.rs)
    - verified with `cargo test -p netflow-plugin`: `378 passed, 0 failed, 14 ignored`
- Decision made by Costa on `2026-04-12`: base per-tier journal rotation size on the per-tier total byte-retention budget, not on a fixed global file size.
  - Rule:
    - for per-tier `size_of_journal_files >= 100MB`, derive `size_of_journal_file = clamp(size_of_journal_files / 20, 5MB, 200MB)`
    - per-tier total byte-retention budgets below `100MB` are considered invalid for `netflow-plugin`
  - Intended operational effect:
    - once a tier is at steady-state retention, one full-file deletion should represent at most about `1/20` of the tier's byte history until the `200MB` cap is reached
    - above `4GB` per tier, the file-size cap keeps the rotation unit at `200MB`, so the fraction of history replaced on each steady-state rotation becomes even smaller than `1/20`
  - Critical implementation constraint verified in code:
    - the current plugin still enforces `number_of_journal_files` as an active retention limit in [`src/crates/netdata-netflow/netflow-plugin/src/ingest/service/init.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest/service/init.rs) and [`src/crates/journal-log-writer/src/log/chain.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-log-writer/src/log/chain.rs)
    - the current default of `64` files in [`src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/journal.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/journal.rs) would conflict with this rule for larger byte budgets unless that file-count limit is also derived, raised, or disabled by default
- Decision made by Costa on `2026-04-12`: users must not configure `number_of_journal_files` directly.
  - Desired user-facing contract:
    - users configure per-tier retention by `size_of_journal_files` and/or `duration_of_journal_files`
    - the plugin derives any internal file-count limit needed to satisfy that contract
    - if the journal layer already supports size/time retention directly, the plugin should not keep an independent user-visible file-count knob
  - Verified library capability:
    - `journal-log-writer` retention already supports optional file-count, total-size, and age limits independently in [`src/crates/journal-log-writer/src/log/config.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-log-writer/src/log/config.rs)
    - because those retention limits are optional, the plugin can satisfy size-only, time-only, or size+time retention without exposing `number_of_journal_files` to users
  - Implication:
    - the current plugin config model and validation in [`src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/journal.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/journal.rs) and [`src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation/journal.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/plugin_config/validation/journal.rs) need redesign before implementation, because they currently require `number_of_journal_files`, `size_of_journal_files`, and `duration_of_journal_files` all to be present and non-zero
- Decision made by Costa on `2026-04-12`: for tiers that use time-only retention and do not define `size_of_journal_files`, start with a fixed internal rotation size of `100MB`.
  - Context:
    - rotation uses `size_of_journal_file` and/or `duration_of_journal_file`, and the active file rotates when any configured rotation limit is exceeded
    - retention deletion itself can already be time-only in [`src/crates/journal-log-writer/src/log/config.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-log-writer/src/log/config.rs)
    - the current plugin default still hardcodes `size_of_journal_file = 256MB` and `duration_of_journal_file = 1h` in [`src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/journal.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/journal.rs)
    - because active journal residency is sensitive to file size, time-only retention still needs a bounded default rotation size even without a byte-retention budget
  - Additional feasibility constraint verified on `2026-04-12`:
    - true adaptive per-file sizing would require a small shared-writer change, because `journal-log-writer::Log` snapshots `size_of_journal_file` into `RotationState` at construction time in [`src/crates/journal-log-writer/src/log/mod.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-log-writer/src/log/mod.rs)
    - there is currently no public API to update `rotation_policy.size_of_journal_file` or rebuild `RotationState` on an existing `Log`
  - Decision details:
    - use a fixed internal `100MB` rotation size for time-only tiers now
    - do not change `journal-log-writer` for adaptive rotation sizing in this pass
    - revisit adaptive sizing later only if real users ask for it
  - Implementation constraint found and fixed during validation on `2026-04-12`:
    - per-tier overrides need a tri-state model to distinguish:
      - omitted field => inherit global limit
      - explicit `null` => disable inherited limit
      - concrete value => override global limit
    - the first draft used plain `Option`, which could not represent explicit `null` for tier overrides correctly
    - the config model was updated to preserve this contract before claiming the refactor complete

## Plan

- Review the PR diff against `origin/master` and isolate every changed file outside `src/crates/netdata-netflow/netflow-plugin`.
- For each shared-component change:
  - verify what behavior it changes in code
  - verify why it was introduced
  - decide whether it is necessary, optional, or should be reverted from the PR
  - record risks, especially shared-runtime or journal-layer blast radius
- Present Costa with a review-style summary focused on findings first:
  - unnecessary shared changes
  - risky shared changes
  - shared changes that are justified and should remain
- Cleanup implementation for the clearly unnecessary shared changes before performance work:
  - keep shared runtime generic by moving netflow request translation into the netflow handler
  - remove unused shared `journal-engine` builder surface that has no netflow production caller
  - revert duplicated `jf`-crate changes that are outside the netflow path
  - rerun the affected shared and plugin test suites before any performance work begins
- After the shared-component review, design a performance test strategy for flow ingestion:
  - synthetic benchmark coverage for NetFlow, sFlow, and IPFIX
  - real fixture replay coverage where available
  - throughput and memory measurement dimensions
  - cardinality and field-variability scenarios
- Implemented on `2026-04-12`:
  - split ingest test support out of oversized [`src/crates/netdata-netflow/netflow-plugin/src/ingest_tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest_tests.rs) into dedicated support and benchmark modules
  - added protocol-specific release benchmarks in [`src/crates/netdata-netflow/netflow-plugin/src/ingest_bench_tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest_bench_tests.rs):
    - NetFlow v5
    - NetFlow v9
    - IPFIX
    - sFlow
  - added a second benchmark matrix for post-decode ingest sensitivity to field variability/cardinality:
    - low-cardinality
    - medium-cardinality (`256` buckets)
    - high-cardinality (`50,000` unique buckets)
  - extracted a single-record ingest helper from [`src/crates/netdata-netflow/netflow-plugin/src/ingest/service/runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest/service/runtime.rs) so the benchmark exercises the same raw-write/facet/tier path as the production packet handler
- Verified on `2026-04-12`:
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml` passed with `388 passed, 0 failed, 15 ignored`
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml --release bench_ingestion_protocol_matrix -- --ignored --nocapture` passed
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml --release bench_ingestion_cardinality_matrix -- --ignored --nocapture` passed
- First measured throughput on this workstation with the current release harness:
  - protocol matrix:
    - NetFlow v5 full ingest: about `49.6k flows/s`
    - NetFlow v9 full ingest: about `47.6k flows/s`
    - IPFIX full ingest: about `62.7k flows/s`
    - sFlow full ingest: about `45.7k flows/s`
    - decode-only throughput is much higher (`0.75M - 5.2M flows/s` depending on protocol), so the dominant steady-state cost is post-decode ingest, not wire parsing
  - post-decode cardinality matrix:
    - low-cardinality: about `53.4k flows/s`
    - medium-cardinality (`256` buckets): about `47.6k flows/s`
    - high-cardinality (`50,000` unique buckets): about `29.3k flows/s`
  - operational implication:
    - with the current single-threaded ingest path on this host, a reasonable expectation is roughly `45k - 63k flows/s` on warmed real fixtures at low-to-medium cardinality
    - high-cardinality post-decode ingest drops to about `29k flows/s`, which is the more realistic planning number when users have highly variable endpoint/exporter/interface dimensions
- Re-run the full affected test suites after each memory change:
  - `cargo test -p journal-core -p journal-log-writer -p journal-engine -p netflow-plugin`
- Improve facet runtime accounting first, because it is now a proven blind spot:
  - replace rough `HashTable` byte heuristics with real allocation reporting where the container exposes it
  - use a better size estimate for `RoaringTreemap`
  - include missing container-structure overhead for the facet `BTreeMap`/published snapshot bookkeeping
- Re-run the focused facet harness after the estimator changes and compare:
  - actual RSS growth
  - facet-accounted bytes
  - remaining gap
- Use the improved live charts to decide the next optimization target instead of guessing.
- New verified optimization target from Massif on `2026-04-11`:
  - `TextValueStore::insert()` appears twice from `apply_active_contribution()`, once for the per-file contribution store and once for the global `active_fields` union.
  - This means new active text values are stored twice in long-lived runtime memory.
  - `DenseBitSet::new()` also shows measurable startup allocation through `empty_field_stores()`, even before those dense stores hold any values.
- Next implementation step:
  - remove the persistent `active_fields` duplicate store and derive active unions from `active_contributions` when needed
  - make dense facet bitsets allocate lazily on first insert instead of at empty-store construction
  - then rerun the full `netflow-plugin` suite and the focused facet-memory harness
- Additional verified next target:
  - archived facet startup load still costs about `48.3 MB` RSS even after `active=0` and `active_contrib=0`
  - the current persisted text facet format is `Vec<String>`, which means startup deserializes many heap strings and then rebuilds compact text stores from them
  - the next optimization pass should persist/load text stores in compact arena form so startup can rebuild the lookup index without first materializing `Vec<String>`
- New verified next target on `2026-04-11`:
  - the remaining live resident gap is dominated by allocator-retained free heap, not by GeoIP/MMDB data or by a better general-purpose allocator existing on this host
  - the next optimization pass should therefore focus on reducing transient startup allocations further and on improving charts so allocator-retained heap is shown explicitly instead of being hidden inside `unaccounted`
- New verified next target on `2026-04-11` after allocative archived-facet profiling:
  - the current archived facet estimate is materially wrong for `SparseU64` timestamp-like fields
  - `FLOW_END_USEC` is the dominant verified archived-facet memory owner and should be treated as the next major optimization decision
- New verified next target on `2026-04-11` after the latest live chart review:
  - add a true process-side resident mapping breakdown chart from `/proc/self/smaps`
  - classify resident bytes into disjoint buckets such as:
    - heap
    - anonymous non-heap mappings
    - raw journal mmaps
    - `1m` journal mmaps
    - `5m` journal mmaps
    - `1h` journal mmaps
    - other file-backed mappings
    - shmem
  - this will not replace internal heap ownership accounting, but it will stop forcing most of RSS into an opaque `unaccounted` bucket when the kernel already exposes the owning mapping classes
- New verified next target on `2026-04-11` after correlating live mappings with enrichment assets:
  - switch GeoIP database loading from eager copied `Reader<Vec<u8>>` to mmap-backed readers
  - extend the resident mapping chart to break GeoIP MMDB residency out separately from generic `other_file`
  - this should remove about `142 MB` of anonymous copied database memory while preserving enrichment behavior
  - status update after install:
    - implemented and verified live
    - current remaining blind spot is no longer a huge anonymous GeoIP owner; it is the smaller steady-state heap / runtime path that the existing startup harness still does not exercise

1. Build a local stress environment around the real `netflow-plugin` binary/crate using isolated cache/config dirs and existing UDP replay helpers as the starting point.
2. Add a reproducible load generator that can sustain much higher cardinality and volume than the office feed, so memory growth can be observed quickly and deterministically.
3. Capture a baseline:
   - RSS / max RSS
   - ingest counters
   - journal growth
   - memory snapshots over time
   - allocator evidence where possible
4. Attribute memory to concrete subsystems by selectively measuring with feature/code-path instrumentation and targeted profiling.
5. Add internal memory accounting for the major in-memory subsystems and expose it via Netdata charts.
6. Implement the highest-impact reductions first:
  - first, replace per-file facet string sets with compact typed stores and stop reparsing/recloning compact values into the resident path
  - then remove the duplicate combined facet store from runtime memory while preserving published facet behavior on restart
  - then add byte accounting for resident facet/tier/open-row structures and expose them as charts
  - then switch tier rollup rows to sparse/default-aware storage and compress IPv4-heavy IP dictionaries
  - then normalize decoder template scope away from raw UDP source-port churn if the parser path proves to be retaining duplicate template state per exporter IP
  - then re-measure and decide whether more aggressive high-cardinality facet residency reduction is still required
7. Add tests/benchmarks so the regression is pinned in CI or at least in local automated verification.
8. Re-run the same stress scenario and compare against baseline with exact numbers.
9. Attribute the current live `~700 MB` RSS case by combining:
  - kernel/process evidence from `/proc/<pid>/status`, `/proc/<pid>/smaps`, and `/proc/<pid>/smaps_rollup`
  - allocator/process evidence from the live glibc-backed binary
  - plugin-side accounting for the largest resident runtime structures
10. Replace the coarse `unaccounted` bucket with named subsystem buckets where the codebase permits reliable accounting, so field diagnosis in customer environments points to concrete owners instead of just total RSS.
11. Prove the live writable-journal mmap growth in the shared `journal-core` window manager with a focused regression test, then fix that shared behavior if the test confirms full-file growth from sequential append access.
12. Verify or reject the current GeoIP/MMDB hypothesis with direct evidence from:
  - the auto-detection code path in the plugin
  - the actual MMDB files present on this machine
  - the live allocator/process footprint
13. If GeoIP/MMDB is a dominant owner:
  - add explicit accounting for it in the memory charts
  - decide whether eager `Vec<u8>` loading is acceptable or should be replaced with a lower-RSS access pattern
14. Extend the startup profiling harness so it also exercises the real plugin-runtime startup path:
  - construct `PluginRuntime`
  - register the `flows` handler
  - register charts
  - start the runtime on in-memory streams long enough for chart/runtime initialization to happen
  - measure whether that path adds any meaningful steady-state heap beyond the already measured ingest/query/rebuild path
15. Re-run the full relevant test suites after the runtime-harness change:
  - `cargo test -p journal-core -p journal-log-writer -p journal-engine -p netflow-plugin`
16. Re-check the live installed process after the harness and chart work:
  - validate RSS / anonymous / file-backed split
  - validate `memory_resident_mapping_bytes`
  - validate whether `memory_accounted_bytes.unaccounted` is now small enough to be operationally useful
  - current reality:
    - the live mapping chart is now operationally useful and explains the majority of RSS in concrete buckets (`heap`, `geoip_geo`, `geoip_asn`, journal tiers, `other_file`)
    - the logical `memory_accounted_bytes` chart still keeps a smaller `unaccounted` bucket because it intentionally tracks named plugin subsystems, not every resident mapping class

## Implied decisions

- All stress work will stay inside this worktree and temp directories created under it or safe local temp space.
- No production routers, remote servers, or non-project directories will be touched beyond local profiling tools and temporary test data.
- The stress harness should be reusable for future memory regressions, not a one-off manual sequence.

## Testing requirements

- Baseline and post-fix stress runs on the same workload and same binary mode.
- Automated test coverage for the specific memory-driving structure(s) changed.
- Regression-oriented assertions where practical:
  - bounded facet growth
  - bounded per-hour rollup index growth
  - no loss of ingest/query correctness for representative flows
- Verification that new memory accounting/charts track the intended subsystems and update during load.
- Re-run relevant `netflow-plugin` tests after code changes.

### Verification completed so far

- `cargo test -p netflow-plugin charts::tests`
- `cargo test -p netflow-plugin facet_runtime::tests`
- `cargo test -p netflow-plugin tiering::tests`
- `cargo test -p netflow-plugin query::tests`
- `cargo test -p netdata-flow-index`
- `cargo test -p netflow-plugin stress_profile_high_cardinality_facet_runtime_memory -- --ignored --nocapture`
- `cargo test -p netflow-plugin stress_profile_high_cardinality_tier_index_memory -- --ignored --nocapture`
- `cargo test -p netflow-plugin stress_profile_high_cardinality_netflow_memory -- --ignored --nocapture`
- `cargo test -p netflow-plugin stress_profile_decoder_source_port_churn_memory -- --ignored --nocapture`
- `cargo test -p netflow-plugin decoder::tests::v9_parser_scope_reuses_templates_across_source_port_churn -- --nocapture`
- `cargo test -p netflow-plugin decoder::tests::persisted_decoder_state_reuses_loaded_namespace_for_new_source_port -- --nocapture`
- `cargo test -p journal-engine`
- `cargo test -p netflow-plugin`

### Broader suite status

- The earlier 4 broader-suite failures were root-caused and fixed in this worktree:
  - `decoder::tests::akvorado_sflow_vxlan_fixture_matches_expected_inner_projection`
  - `decoder::tests::akvorado_sflow_1140_fixture_matches_expected_sample_projection`
  - `enrichment::tests::enricher_is_disabled_when_configuration_is_empty`
  - `ingest::tests::ingest_service_with_decap_vxlan_extracts_inner_header_view`
- Verified before the latest memory/rebuild commit:
  - `cargo test -p netflow-plugin`
  - result: `375 passed, 0 failed, 13 ignored`
- New requirement on `2026-04-11`: re-run the relevant suites after the latest pushed commit and treat any new failure as a blocker before continuing the unresolved live memory investigation.

### Verified root causes for the remaining test failures

- Empty enrichment config is not actually empty:
  - [`src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/root.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/plugin_config/types/enrichment/root.rs) makes `EnrichmentConfig::default()` include non-empty `asn_providers` and `net_providers`.
  - [`src/crates/netdata-netflow/netflow-plugin/src/enrichment/init.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/enrichment/init.rs) treats any non-empty provider list as sufficient to enable enrichment.
  - Result: [`src/crates/netdata-netflow/netflow-plugin/src/enrichment/tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/enrichment/tests.rs) `enricher_is_disabled_when_configuration_is_empty()` fails because `EnrichmentConfig::default()` can no longer represent a disabled configuration.
- IPv4/IPv6 packet-section length accounting is clamping to captured bytes instead of using the on-wire L3 length encoded in the packet headers:
  - [`src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet/ip.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/packet/ip.rs) returns `captured_length` / `payload_end`.
  - [`src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse/ip.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/decoder/record/packet/parse/ip.rs) mirrors the same clamp in the `FlowRecord` path.
  - [`src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/sflow/record.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/decoder/protocol/sflow/record.rs) then uses that returned length as `rec.bytes`.
  - Reproduced failures:
    - `data-encap-vxlan.pcap`: expected `BYTES=104`, actual `BYTES=64`
    - `data-1140.pcap`: expected visible `BYTES=1536000`, actual `BYTES=116736`, which means raw `114 * 1024` instead of `1500 * 1024`
  - Upstream Akvorado evidence:
    - [`/opt/baddisk/monitoring/akvorado/akvorado__akvorado/outlet/flow/decoder/sflow/root_test.go`](/opt/baddisk/monitoring/akvorado/akvorado__akvorado/outlet/flow/decoder/sflow/root_test.go) expects `1500` for `data-1140.pcap` and `104` for `data-encap-vxlan.pcap`.
    - [`/opt/baddisk/monitoring/akvorado/akvorado__akvorado/outlet/flow/decoder/helpers.go`](/opt/baddisk/monitoring/akvorado/akvorado__akvorado/outlet/flow/decoder/helpers.go) returns the IPv4/IPv6 header-declared L3 length, not the truncated capture size.

### Open reality check

- The work is not yet at the requested end state.
- The facet hotspot is materially better and now observable via charts/accounting.
- The tier index hotspot is materially better and now exposes an internal breakdown via charts/accounting.
- The decoder template/parser churn case is now both observable and fixed for same-exporter source-port churn, with a measured `~430x` reduction on the synthetic churn harness.
- The measured focused reductions are real:
  - facet hotspot RSS delta: `21.24 MB -> 8.77 MB`
  - tier hotspot RSS delta: `22.21 MB -> 9.90 MB`
  - decoder source-port churn hotspot RSS delta: `867.91 MB -> 2.02 MB`
- These measured results support a `>30x` reduction only for the verified decoder source-port churn failure mode. They still do not justify claiming an overall end-to-end `30x` reduction for all workloads.
- Live measurement on `2026-04-11` shows the currently installed plugin at about `674 MiB` RSS (`706,560,000` bytes from the chart; `~690-708 MB` from `/proc` depending on sampling time).
- The live process is dominated by private anonymous memory:
  - `RssAnon`: about `660 MB`
  - `RssFile`: about `30-48 MB`
  - largest mappings: `[heap]` about `361 MB`, `[anon]` about `299 MB`
- The current charted accounting does not explain the live footprint yet:
  - `netdata.netflow.memory_accounted_bytes.unaccounted`: `692,828,350` bytes
  - all explicitly named buckets combined: about `13.1 MB`
  - conclusion: the chart framework works, but it still lacks attribution for the dominant live heap owners in this case
- Live allocator introspection through `gdb` has a verified safety boundary:
  - attaching and detaching from the running process is safe on this machine
  - resolving glibc allocator symbols (`malloc_info`, `mallinfo2`) is also possible
  - calling `malloc_info()` from inside the live process is not safe in this environment: it faulted inside glibc `fputs()` while writing the allocator report and the plugin PID disappeared afterwards
  - this means "live allocator dump via injected `gdb` call" is no longer an acceptable primary method for this process

## New obstacle

- `gdb`-injected allocator reporting is not safe enough for the running plugin.
- Verified evidence:
  - `gdb` attach/detach works and shared-library symbol resolution can find `malloc_info` / `mallinfo2`.
  - forcing `malloc_info()` inside the live process caused a fault in glibc while executing `fputs()`.
  - the inspected plugin PID (`1164472`) disappeared after that attempt.
- Implication:
  - allocator attribution must move to one of these safer paths:
    - an in-process diagnostic endpoint/chart implemented in the plugin itself
    - an isolated stress-harness build with allocator tracing/profiling enabled
    - offline process/core analysis instead of injected live function calls
- The broader `netflow-plugin` suite was green before commit `d6e4ff75c0`, but it must be re-validated after that commit before more optimization work continues.
- Re-validated after commit `d6e4ff75c0`:
  - `cargo test -p journal-engine`: passed
  - `cargo test -p netflow-plugin`: passed
  - `netflow-plugin` result: `375 passed, 0 failed, 13 ignored`

### New live-memory findings after test revalidation on 2026-04-11

- The current running plugin (`/usr/libexec/netdata/plugins.d/netflow-plugin`, PID observed as `1356956`) is now dominated by both anonymous and file-backed resident memory:
  - `VmRSS`: about `623,908 kB`
  - `RssAnon`: about `355,928 kB`
  - `RssFile`: about `267,980 kB`
  - `Threads`: `14`
- The live process currently holds four very large writable shared journal mappings:
  - raw journal mapping: about `111,668 kB RSS`
  - `1m` journal mapping: about `77,344 kB RSS`
  - `5m` journal mapping: about `53,480 kB RSS`
  - `1h` journal mapping: about `19,552 kB RSS`
  - Evidence: `/proc/<pid>/maps` and `pmap -x` show large `rw-s` mappings for the active journal files in `raw`, `1m`, `5m`, and `1h`.
- The same live process also still holds dozens of open file descriptors to the rebuild index cache directory:
  - `/var/cache/netdata/flows/.rebuild-index-cache/foyer-storage-direct-fs-*`
  - Evidence: `/proc/<pid>/fd`
  - This means the rebuild-scoped Foyer cache is not being closed cleanly after startup.
- The release startup harness with the current code and the same local journal base directory does not reproduce the full live file-backed shape:
  - `mmap_in_use` peaks at about `33.6 MB`
  - rebuild settle RSS is about `156.9 MB`
  - conclusion: the remaining live RSS is not just startup rebuild anymore
- The journal window manager implementation in [`src/crates/journal-core/src/file/mmap.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-core/src/file/mmap.rs) has a likely structural bug for sequential appends:
  - when a request starts inside an existing window but extends beyond its end, `get_window()` remaps from the existing window's original start instead of aligning a fresh bounded tail window to the current position
  - this can grow one mapping from the beginning of the file toward the current write position over time
  - this behavior matches the observed one-large-map-per-active-tier pattern in the live process

### Verified diagnosis of the large live journal mmaps on 2026-04-11

- The suspicious shared remap logic in [`src/crates/journal-core/src/file/mmap.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-core/src/file/mmap.rs) was **not** introduced by this PR:
  - `git blame` on the remap branch (`window_start = window.offset`) points to commit `adc4d66ea03` from `2025-05-28`
  - the only newer changes around that code are the later consistency guards for mmap failure handling
- The `journal-log-writer` active writer window size of `8 MiB` was also not changed by this PR:
  - `git blame` on [`src/crates/journal-log-writer/src/log/mod.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-log-writer/src/log/mod.rs) shows `.with_window_size(8 * 1024 * 1024)` coming from commit `d0905d9b99`
- The shared journal code explicitly documents the intended design:
  - [`src/crates/journal-core/src/file/file.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-core/src/file/file.rs) says the implementation should maintain a **small set of memory-mapped windows** instead of mapping the entire file
- Live process evidence contradicts that intended design for the active writable journals:
  - current active file sizes:
    - raw: `152 MiB`
    - `1m`: `128 MiB`
    - `5m`: `72 MiB`
    - `1h`: `24 MiB`
  - current main `rw-s` mapping sizes for those same files:
    - raw: `152 MiB`
    - `1m`: `128 MiB`
    - `5m`: `72 MiB`
    - `1h`: `24 MiB`
  - conclusion: the main writable mapping size matches the full current file size, not the configured `8 MiB` writer window
- The always-mapped journal hash tables are **not** the explanation:
  - for a `256 MiB` max file size, the persistent data-hash table is about `1 MiB`
  - the field-hash table is negligible (`~2 KiB`)
  - therefore the large `24-152 MiB` mappings are not expected fixed metadata maps
- The most likely overall explanation is now:
  - the shared journal window-manager behavior is pre-existing
  - this PR exposed it operationally by creating a workload with four continuously written active journals (`raw`, `1m`, `5m`, `1h`) and large active file sizes
  - the recent PR changes in `journal-log-writer` themselves are timestamp/lifecycle additions, not mmap-window behavior changes

### Live revalidation after fixing the shared journal window manager on 2026-04-11

- After installing the bounded-window fix from [`src/crates/journal-core/src/file/mmap.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-core/src/file/mmap.rs), the live process no longer maps whole active journal files:
  - live `raw` writer map: `8 MiB`
  - live `1m` writer map: `8 MiB`
  - the extra `~1 MiB` maps per file remain, which matches expected fixed per-file metadata mappings
- Live resident memory after restart materially improved:
  - `/proc/<pid>/status` showed `RssFile` collapse from about `268 MiB` before the fix to about `19-28 MiB` after the fix
  - the same live process stabilized around `VmRSS ≈ 350-408 MiB`, `RssAnon ≈ 331-384 MiB`, `Threads = 14`
- Netdata's own live charts now agree with `/proc`:
  - `netdata.netflow.memory_resident_bytes` current samples show `rss ≈ 406-408 MiB`, `rss_anon ≈ 383.6 MiB`, `rss_file ≈ 22.7-25.0 MiB`
  - `netdata.netflow.memory_accounted_bytes` current samples still show `unaccounted ≈ 393-395 MiB`
  - `netdata.netflow.memory_allocator_bytes` current samples show:
    - `heap_in_use ≈ 66.97 MiB`

### Verified allocator-retention diagnosis and post-fix live state on 2026-04-11

- GeoIP/MMDB data is not a dominant owner in this environment:
  - auto-detection uses [`src/crates/netdata-netflow/netflow-plugin/src/plugin_config/runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/plugin_config/runtime.rs) and eager loading uses [`src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/files.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/enrichment/data/geoip/files.rs)
  - actual detected files on this machine:
    - `/var/cache/netdata/topology-ip-intel/topology-ip-asn.mmdb`: `10,402,916` bytes
    - `/var/cache/netdata/topology-ip-intel/topology-ip-country.mmdb`: `5,875,413` bytes
  - combined resident upper bound is only about `16.3 MB`, so MMDB files cannot explain the remaining `~200+ MB` anonymous RSS
- The allocator chart's `mmap_in_use` value was re-checked and must not be treated as resident RAM by itself:
  - [`src/crates/netdata-netflow/netflow-plugin/src/memory_allocator.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/memory_allocator.rs) reports `mallinfo2().hblkhd`
  - `/proc/<pid>/smaps` still shows the dominant live resident memory in anonymous heap/arena mappings, not in a separate resident mapping equal to `mmap_in_use`
  - implication: `mmap_in_use` is allocator state, not a direct resident-memory answer
- Correct live process before the latest allocator-focused fixes:
  - observed plugin PID: `1699732`
  - `/proc/<pid>/status`:
    - `VmRSS: 362,188 kB`
    - `RssAnon: 320,984 kB`
    - `RssFile: 41,204 kB`
    - `Threads: 9`
  - `/proc/<pid>/smaps_rollup`:
    - `Private_Dirty: 320,984 kB`
    - `AnonHugePages: 0 kB`
  - top resident mappings:
    - anonymous: `146,000 kB`
    - `[heap]`: `82,912 kB`
    - anonymous: `39,032 kB`
    - anonymous: `33,164 kB`
    - anonymous: `18,784 kB`
- The dominant remaining mechanism was verified as allocator-retained free heap:
  - live allocator chart before the latest code change showed approximately:
    - `heap_arena ≈ 500,064,300`
    - `heap_free ≈ 433,473,010`
    - `heap_in_use ≈ 66,591,236`
  - conclusion:
    - most anonymous heap footprint was not live payload; it was free heap retained by glibc after earlier startup/runtime allocation spikes
- The service environment already had `MALLOC_ARENA_MAX=4`, but reducing it further to `1` still helped on the real startup workload:
  - release multithread harness on the retained live dataset:
    - `MALLOC_ARENA_MAX=4`: `rebuild_settle_trim delta_rss = 80,465,920`
    - `MALLOC_ARENA_MAX=1`: `rebuild_settle_trim delta_rss = 47,730,688`
  - verified reduction: about `32.7 MB`
- Fix implemented in [`src/crates/netdata-netflow/netflow-plugin/src/memory_allocator.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/memory_allocator.rs) and wired from [`src/crates/netdata-netflow/netflow-plugin/src/main.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/main.rs):
  - call `mallopt(M_ARENA_MAX, 1)` inside the plugin process before Tokio runtime creation
  - keep THP disabled for the process
- Additional startup-allocation reduction implemented in [`src/crates/netdata-netflow/netflow-plugin/src/facet_runtime.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/facet_runtime.rs):
  - load persisted facet state via `BufReader + bincode::deserialize_from()` instead of `fs::read()`
  - consume persisted archived stores directly instead of cloning every saved store during hydration
- Verified live state after installing the arena-cap build:
  - observed plugin PID: `1728342`
  - `/proc/<pid>/status`:
    - `VmRSS: 227,880 kB`
    - `RssAnon: 213,344 kB`
    - `RssFile: 14,536 kB`
    - `Threads: 9`
  - allocator chart:
    - `heap_arena ≈ 326,135,800`
    - `heap_free ≈ 261,295,660`
    - `heap_in_use ≈ 64,840,148`
    - `mmap_in_use ≈ 131,457,140`
    - `unaccounted ≈ 225,923,660`
  - verified improvement versus the earlier `362,188 kB` live RSS: about `134 MB` lower
- Verified live state after also installing the persisted-load cleanup:
  - observed plugin PID: `1743390`
  - `/proc/<pid>/status`:
    - `VmRSS: 227,840 kB`
    - `RssAnon: 211,636 kB`
    - `RssFile: 16,204 kB`
    - `Threads: 9`
  - allocator chart:
    - `heap_arena ≈ 313,335,800`
    - `heap_free ≈ 260,037,540`
    - `heap_in_use ≈ 53,298,250`
    - `mmap_in_use ≈ 143,175,840`
    - `unaccounted ≈ 222,713,120`
  - conclusion:
    - the persisted-load cleanup helped slightly, but the major win came from capping glibc arena growth inside the process

### Verified archived facet deep-size diagnosis on 2026-04-11

- A targeted allocative diagnostic was added and executed against the live dataset:
  - `cargo test -p netflow-plugin stress_profile_live_archived_facet_allocative_breakdown -- --ignored --nocapture`
- Results on the current `facet-state.bin`:
  - archived facet memory by deep allocative traversal: `40,524,572` bytes
  - archived path tracking: `38,026` bytes
  - published snapshot: `5,844` bytes
- Top archived facet fields by measured deep allocation:
  - `FLOW_END_USEC`: `32,263,968` bytes
  - `DST_ADDR_NAT`: `1,564,840`
  - `DST_ADDR`: `1,564,768`
  - `SRC_ADDR`: `1,560,312`
  - `SRC_ADDR_NAT`: `1,560,304`
  - `DST_AS_NAME`: `589,824`
  - `SRC_AS_NAME`: `589,824`
- This diagnostic also proved the current estimator is materially wrong for that dominant field:
  - `FLOW_END_USEC` current estimated bytes: `5,456,148`
  - `FLOW_END_USEC` measured deep bytes: `32,263,968`
  - undercount factor: about `5.9x`
- Interpretation:
  - the current facet chart under-accounting is not generic hand-waving anymore; it now has a named dominant owner
  - the dominant owner is a timestamp-microsecond facet that is currently being treated as an archived/autocomplete/searchable dimension like a normal categorical facet
- Follow-up verification after Costa approved removal of those fields:
  - `cargo test -p netflow-plugin stress_profile_live_archived_facet_allocative_breakdown -- --ignored --nocapture`
  - current archived facet memory by deep allocative traversal: `8,260,553` bytes
  - current top archived fields are now IP and ASN/name dimensions:
    - `DST_ADDR_NAT`: `1,564,840`
    - `DST_ADDR`: `1,564,768`
    - `SRC_ADDR`: `1,560,312`
    - `SRC_ADDR_NAT`: `1,560,304`
    - `DST_AS_NAME`: `589,824`
    - `SRC_AS_NAME`: `589,824`
  - conclusion:
    - removing the internal timestamp fields produced a measured `~4.9x` reduction in archived facet deep size on the same dataset (`40.5 MB -> 8.3 MB`)
    - the next archived facet owners are IP dictionaries, not hidden timestamp sidecars
    - `heap_free ≈ 604.41 MiB`
    - `heap_arena ≈ 671.38 MiB`
    - `mmap_in_use ≈ 165.03 MiB`
    - `releasable ≈ 4 KiB`
- This is a strong allocator-fragmentation/retention signal:
  - the live process currently has far more free heap retained inside glibc arenas than live heap objects
  - `malloc_trim()` by itself is unlikely to solve the remaining problem because the allocator reports only `~4 KiB` releasable despite `~604 MiB` free inside arenas
- A release startup harness run against the same live journal corpus no longer reproduces the current live RSS:
  - `cargo test -p netflow-plugin --release stress_profile_live_startup_memory_multithreaded -- --ignored --nocapture`
  - result after settle+trim: `rss ≈ 97.8 MiB`, `heap_free ≈ 324.9 MiB`, `heap_arena ≈ 379.5 MiB`, `threads = 7`
  - implication: pure startup rebuild is no longer sufficient to explain the live `~400 MiB` steady-state footprint; the remaining growth is tied to runtime behavior while the service is active
- A new verified runtime-lifetime bug remains:
  - the live process still keeps `64` open file descriptors under `/var/cache/netdata/flows/.rebuild-index-cache/foyer-storage-direct-fs-*` long after rebuild should have finished
  - this indicates the Foyer rebuild index cache must be closed explicitly instead of relying on drop timing
  - source review of `foyer-storage` shows why `close()` alone is insufficient: the block-engine `close()` path stops writers and waits for reclaim, but the direct-fs partitions keep their `File` handles in `FsPartition.file` until the storage device itself is dropped
  - because the rebuild cache is only scratch state for one rebuild run, the safer memory-first fix is to stop using disk-backed Foyer storage for this path and keep the cache in memory only
- Allocator-substitution diagnostics were run against the same startup profile and rejected:
  - glibc baseline settle+trim: `~97.8 MiB RSS`
  - `mimalloc` preload settle: `~252.3 MiB RSS`
  - `jemalloc` preload settle: `~446.4 MiB RSS`
  - glibc with transparent huge pages disabled settle+trim: `~138.2 MiB RSS`
  - conclusion: allocator replacement and THP disablement are not the right first-line fix for this workload

### Verified shared journal mmap growth diagnosis on 2026-04-11

- The shared remap branch in `origin/master` was inspected with:
  - `git show origin/master:src/crates/journal-core/src/file/mmap.rs | sed -n '200,260p'`
  - verified old logic:
    - remove current window
    - keep `window_start = window.offset`
    - extend only `window_end` to cover the new request
- That means append-heavy accesses crossing the current window boundary can grow a single writable mapping from the beginning of the file toward the tail instead of sliding a bounded window.
- The current worktree contains a focused regression in [`src/crates/journal-core/src/file/mmap.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/journal-core/src/file/mmap.rs#L476):
  - `sequential_boundary_crossing_slides_window_instead_of_growing_from_start`
  - it verifies a `max_windows=1` manager with `4 KiB` chunks:
    - first cross-boundary remap produces a `2`-chunk window at offset `0`
    - the next cross-boundary remap slides to offset `4096` instead of growing to `12288` bytes from offset `0`
    - the next remap slides again to offset `8192`
- Verified test evidence:
  - `cargo test -p journal-core sequential_boundary_crossing_slides_window_instead_of_growing_from_start -- --nocapture`
  - `cargo test -p journal-core`
  - `cargo test -p journal-log-writer`
  - `cargo test -p journal-engine`
  - `cargo test -p netflow-plugin`
  - all passed on the current worktree
- Conclusion:
  - the large writable journal mmaps observed live are consistent with a real pre-existing shared `journal-core` remap-growth bug/design failure, not with new mmap logic introduced by this PR
  - `netflow-plugin` exposed it because it keeps several active tier writers growing concurrently
  - this shared fix is still uncommitted at the time of this TODO update
- Live post-install snapshot after installing the current worktree and restarting Netdata:
  - observed plugin PID: `1808192`
  - `/proc/<pid>/status`:
    - `VmRSS: 181,204 kB` initially, `185,940 kB` after another minute
    - `RssAnon: 165,896 kB` initially, `166,268 kB` after another minute
    - `RssFile: 15,308 kB` initially, `19,672 kB` after another minute
    - `Threads: 9`
  - `/proc/<pid>/smaps_rollup` after the extra minute:
    - `Rss: 185,940 kB`
    - `Private_Dirty: 168,508 kB`
    - `Private_Clean: 14,944 kB`
    - `AnonHugePages: 0 kB`
  - active journal mappings in `/proc/<pid>/maps`:
    - raw active journal mapped bytes: `9,449,472`
    - `1m` active journal mapped bytes: `9,449,472`
    - current active file sizes: `8,388,608` each
  - caveat:
    - this is a fresh-restart snapshot, so it is not a perfect long-runtime comparison to the earlier `24-152 MB` active full-file mappings
    - however it is consistent with the bounded-window fix and materially lower than the earlier live `227-362 MB` snapshots on this workstation

### Performance-testing feasibility on 2026-04-12

- `FEASIBLE AS SPECIFIED`
- Existing benchmark and profiling surface already covers most of the path we need:
  - end-to-end ingest hot path replay already exists in [`src/crates/netdata-netflow/netflow-plugin/src/ingest_tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/ingest_tests.rs) via `bench_full_hot_path()`
  - fixture replay helpers already exist in [`src/crates/netdata-netflow/netflow-plugin/src/main_tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/main_tests.rs) via `ingest_fixture()` and `replay_fixture_udp()`
  - raw query stage profiling already exists in [`src/crates/netdata-netflow/netflow-plugin/src/query/scan/bench.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/query/scan/bench.rs)
  - memory stress harnesses already exist in [`src/crates/netdata-netflow/netflow-plugin/src/memory_tests.rs`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/src/memory_tests.rs)
- Current gaps:
  - the existing ingest hot-path benchmark mixes protocols together instead of reporting per-protocol throughput
  - it currently covers NetFlow v5, NetFlow v9, and IPFIX fixture replay, but not sFlow in the same throughput harness
  - it does not yet vary field variability or cardinality in a controlled way
  - it does not emit a stable summary format suitable for repeated comparison
- First implementation path:
  - keep the current ignored-test harness model
  - extend `bench_full_hot_path()` or split it into protocol-specific ignored tests
  - add protocol-isolated replay groups for NetFlow v5, NetFlow v9, IPFIX, and sFlow
  - add synthetic high-cardinality flow generation using the existing stress-helper patterns so we can measure the effect of field variability/cardinality separately from decoder cost
  - report at minimum:
    - flows per second
    - packets per second
    - bytes per second
    - microseconds per flow
    - peak RSS / resident mapping snapshot when practical

## Documentation updates required

- Completed:
  - updated [`src/crates/netdata-netflow/netflow-plugin/README.md`](/home/costa/src/PRs/topology-netflow-22111/src/crates/netdata-netflow/netflow-plugin/README.md) with the new memory chart names and their operational meaning, including decoder scope diagnostics for source-port churn.
