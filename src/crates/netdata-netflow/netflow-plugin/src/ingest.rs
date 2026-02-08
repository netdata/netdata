use crate::decoder::{
    DecapsulationMode as DecoderDecapsulationMode, DecodeStats, FlowDecoders,
    TimestampSource as DecoderTimestampSource,
};
use crate::enrichment::{DynamicRoutingRuntime, FlowEnricher, NetworkSourcesRuntime};
use crate::plugin_config::{
    DecapsulationMode as ConfigDecapsulationMode, PluginConfig,
    TimestampSource as ConfigTimestampSource,
};
use crate::tiering::{MATERIALIZED_TIERS, OpenTierState, TierAccumulator, TierKind};
use anyhow::{Context, Result};
use foundation::Timeout;
use journal_common::load_machine_id;
use journal_engine::{
    Facets, FileIndexCacheBuilder, FileIndexKey, LogQuery, QueryTimeRange,
    batch_compute_file_indexes,
};
use journal_index::{Anchor, Direction, FieldName, Microseconds, Seconds};
use journal_log_writer::{Config, Log, RetentionPolicy, RotationPolicy};
use journal_registry::{Monitor, Origin, Registry, Source};
use std::collections::{BTreeMap, HashMap};
use std::fs;
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, RwLock};
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use tokio::net::UdpSocket;
use tokio::time::MissedTickBehavior;
use tokio_util::sync::CancellationToken;

const REBUILD_WINDOW_SECONDS: u32 = 60 * 60;
const REBUILD_TIMEOUT_SECONDS: u64 = 30;
const REBUILD_CACHE_MEMORY_CAPACITY: usize = 16;
const REBUILD_CACHE_DISK_CAPACITY: usize = 64 * 1024 * 1024;
const REBUILD_CACHE_BLOCK_SIZE: usize = 1024 * 1024;
const DECODER_STATE_PERSIST_INTERVAL_USEC: u64 = 30 * 1_000_000;

#[derive(Default)]
pub(crate) struct IngestMetrics {
    pub(crate) udp_packets_received: AtomicU64,
    pub(crate) udp_bytes_received: AtomicU64,
    pub(crate) parse_attempts: AtomicU64,
    pub(crate) parsed_packets: AtomicU64,
    pub(crate) parse_errors: AtomicU64,
    pub(crate) template_errors: AtomicU64,
    pub(crate) netflow_v5_packets: AtomicU64,
    pub(crate) netflow_v7_packets: AtomicU64,
    pub(crate) netflow_v9_packets: AtomicU64,
    pub(crate) ipfix_packets: AtomicU64,
    pub(crate) sflow_datagrams: AtomicU64,
    pub(crate) journal_entries_written: AtomicU64,
    pub(crate) journal_write_errors: AtomicU64,
    pub(crate) journal_sync_errors: AtomicU64,
    pub(crate) tier_entries_written: AtomicU64,
    pub(crate) tier_write_errors: AtomicU64,
    pub(crate) tier_flushes: AtomicU64,
    pub(crate) bioris_refresh_success: AtomicU64,
    pub(crate) bioris_refresh_errors: AtomicU64,
    pub(crate) bioris_dump_success: AtomicU64,
    pub(crate) bioris_dump_errors: AtomicU64,
    pub(crate) bioris_observe_stream_starts: AtomicU64,
    pub(crate) bioris_observe_stream_reconnects: AtomicU64,
    pub(crate) bioris_observe_stream_errors: AtomicU64,
    pub(crate) bioris_observe_streams_active: AtomicU64,
}

impl IngestMetrics {
    pub(crate) fn apply_decode_stats(&self, stats: &DecodeStats) {
        self.parse_attempts
            .fetch_add(stats.parse_attempts, Ordering::Relaxed);
        self.parsed_packets
            .fetch_add(stats.parsed_packets, Ordering::Relaxed);
        self.parse_errors
            .fetch_add(stats.parse_errors, Ordering::Relaxed);
        self.template_errors
            .fetch_add(stats.template_errors, Ordering::Relaxed);
        self.netflow_v5_packets
            .fetch_add(stats.netflow_v5_packets, Ordering::Relaxed);
        self.netflow_v7_packets
            .fetch_add(stats.netflow_v7_packets, Ordering::Relaxed);
        self.netflow_v9_packets
            .fetch_add(stats.netflow_v9_packets, Ordering::Relaxed);
        self.ipfix_packets
            .fetch_add(stats.ipfix_packets, Ordering::Relaxed);
        self.sflow_datagrams
            .fetch_add(stats.sflow_datagrams, Ordering::Relaxed);
    }

    pub(crate) fn snapshot(&self) -> HashMap<String, u64> {
        let mut stats = HashMap::new();
        stats.insert(
            "udp_packets_received".to_string(),
            self.udp_packets_received.load(Ordering::Relaxed),
        );
        stats.insert(
            "udp_bytes_received".to_string(),
            self.udp_bytes_received.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_parse_attempts".to_string(),
            self.parse_attempts.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_parsed_packets".to_string(),
            self.parsed_packets.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_parse_errors".to_string(),
            self.parse_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_template_errors".to_string(),
            self.template_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_netflow_v5".to_string(),
            self.netflow_v5_packets.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_netflow_v7".to_string(),
            self.netflow_v7_packets.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_netflow_v9".to_string(),
            self.netflow_v9_packets.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_ipfix".to_string(),
            self.ipfix_packets.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_sflow".to_string(),
            self.sflow_datagrams.load(Ordering::Relaxed),
        );
        stats.insert(
            "journal_entries_written".to_string(),
            self.journal_entries_written.load(Ordering::Relaxed),
        );
        stats.insert(
            "journal_write_errors".to_string(),
            self.journal_write_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "journal_sync_errors".to_string(),
            self.journal_sync_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "tier_entries_written".to_string(),
            self.tier_entries_written.load(Ordering::Relaxed),
        );
        stats.insert(
            "tier_write_errors".to_string(),
            self.tier_write_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "tier_flushes".to_string(),
            self.tier_flushes.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_refresh_success".to_string(),
            self.bioris_refresh_success.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_refresh_errors".to_string(),
            self.bioris_refresh_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_dump_success".to_string(),
            self.bioris_dump_success.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_dump_errors".to_string(),
            self.bioris_dump_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_observe_stream_starts".to_string(),
            self.bioris_observe_stream_starts.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_observe_stream_reconnects".to_string(),
            self.bioris_observe_stream_reconnects
                .load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_observe_stream_errors".to_string(),
            self.bioris_observe_stream_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_observe_streams_active".to_string(),
            self.bioris_observe_streams_active.load(Ordering::Relaxed),
        );
        stats
    }
}

struct MaterializedTierWriters {
    minute_1: Log,
    minute_5: Log,
    hour_1: Log,
}

impl MaterializedTierWriters {
    fn get_mut(&mut self, tier: TierKind) -> &mut Log {
        match tier {
            TierKind::Minute1 => &mut self.minute_1,
            TierKind::Minute5 => &mut self.minute_5,
            TierKind::Hour1 => &mut self.hour_1,
            TierKind::Raw => panic!("raw tier is not materialized"),
        }
    }

    fn sync_all(&mut self) -> Result<()> {
        self.minute_1.sync()?;
        self.minute_5.sync()?;
        self.hour_1.sync()?;
        Ok(())
    }
}

pub(crate) struct IngestService {
    cfg: PluginConfig,
    metrics: Arc<IngestMetrics>,
    decoders: FlowDecoders,
    decoder_state_path: PathBuf,
    last_decoder_state_persist_usec: u64,
    raw_journal: Log,
    tier_writers: MaterializedTierWriters,
    tier_accumulators: HashMap<TierKind, TierAccumulator>,
    open_tiers: Arc<RwLock<OpenTierState>>,
    routing_runtime: Option<DynamicRoutingRuntime>,
    network_sources_runtime: Option<NetworkSourcesRuntime>,
}

impl IngestService {
    pub(crate) fn new(
        cfg: PluginConfig,
        metrics: Arc<IngestMetrics>,
        open_tiers: Arc<RwLock<OpenTierState>>,
    ) -> Result<Self> {
        let machine_id = load_machine_id().context("failed to load machine id")?;
        let origin = Origin {
            machine_id: Some(machine_id),
            namespace: None,
            source: Source::System,
        };

        let rotation_policy = RotationPolicy::default()
            .with_size_of_journal_file(cfg.journal.size_of_journal_file.as_u64())
            .with_duration_of_journal_file(cfg.journal.duration_of_journal_file);
        let retention_policy = RetentionPolicy::default()
            .with_number_of_journal_files(cfg.journal.number_of_journal_files)
            .with_size_of_journal_files(cfg.journal.size_of_journal_files.as_u64())
            .with_duration_of_journal_files(cfg.journal.duration_of_journal_files);

        let journal_cfg =
            Config::new(origin, rotation_policy, retention_policy).with_machine_id_suffix(false);
        let raw_dir = cfg.journal.raw_tier_dir();
        let raw_journal = Log::new(&raw_dir, journal_cfg.clone()).with_context(|| {
            format!(
                "failed to create journal writer in directory {}",
                raw_dir.display()
            )
        })?;
        let minute_1_dir = cfg.journal.minute_1_tier_dir();
        let minute_5_dir = cfg.journal.minute_5_tier_dir();
        let hour_1_dir = cfg.journal.hour_1_tier_dir();
        let tier_writers = MaterializedTierWriters {
            minute_1: Log::new(&minute_1_dir, journal_cfg.clone()).with_context(|| {
                format!(
                    "failed to create 1m tier writer in directory {}",
                    minute_1_dir.display()
                )
            })?,
            minute_5: Log::new(&minute_5_dir, journal_cfg.clone()).with_context(|| {
                format!(
                    "failed to create 5m tier writer in directory {}",
                    minute_5_dir.display()
                )
            })?,
            hour_1: Log::new(&hour_1_dir, journal_cfg).with_context(|| {
                format!(
                    "failed to create 1h tier writer in directory {}",
                    hour_1_dir.display()
                )
            })?,
        };
        let mut tier_accumulators = HashMap::new();
        for tier in MATERIALIZED_TIERS {
            tier_accumulators.insert(tier, TierAccumulator::new(tier));
        }

        let decapsulation_mode = match cfg.protocols.decapsulation_mode {
            ConfigDecapsulationMode::None => DecoderDecapsulationMode::None,
            ConfigDecapsulationMode::Srv6 => DecoderDecapsulationMode::Srv6,
            ConfigDecapsulationMode::Vxlan => DecoderDecapsulationMode::Vxlan,
        };

        let timestamp_source = match cfg.protocols.timestamp_source {
            ConfigTimestampSource::Input => DecoderTimestampSource::Input,
            ConfigTimestampSource::NetflowPacket => DecoderTimestampSource::NetflowPacket,
            ConfigTimestampSource::NetflowFirstSwitched => {
                DecoderTimestampSource::NetflowFirstSwitched
            }
        };

        let mut decoders = FlowDecoders::with_protocols_decap_and_timestamp(
            cfg.protocols.v5,
            cfg.protocols.v7,
            cfg.protocols.v9,
            cfg.protocols.ipfix,
            cfg.protocols.sflow,
            decapsulation_mode,
            timestamp_source,
        );
        let enricher = FlowEnricher::from_config(&cfg.enrichment)
            .context("failed to initialize netflow enrichment pipeline")?;
        let routing_runtime = enricher
            .as_ref()
            .and_then(FlowEnricher::dynamic_routing_runtime);
        let network_sources_runtime = enricher
            .as_ref()
            .and_then(FlowEnricher::network_sources_runtime);
        decoders.set_enricher(enricher);
        let decoder_state_path = cfg.journal.decoder_state_path();
        if decoder_state_path.is_file() {
            match fs::read_to_string(&decoder_state_path) {
                Ok(data) => {
                    if let Err(err) = decoders.import_persistent_state_json(&data) {
                        tracing::warn!(
                            "failed to restore netflow decoder state from {}: {}",
                            decoder_state_path.display(),
                            err
                        );
                    }
                }
                Err(err) => {
                    tracing::warn!(
                        "failed to read netflow decoder state {}: {}",
                        decoder_state_path.display(),
                        err
                    );
                }
            }
        }

        Ok(Self {
            cfg,
            metrics,
            decoders,
            decoder_state_path,
            last_decoder_state_persist_usec: now_usec(),
            raw_journal,
            tier_writers,
            tier_accumulators,
            open_tiers,
            routing_runtime,
            network_sources_runtime,
        })
    }

    pub(crate) fn routing_runtime(&self) -> Option<DynamicRoutingRuntime> {
        self.routing_runtime.clone()
    }

    pub(crate) fn network_sources_runtime(&self) -> Option<NetworkSourcesRuntime> {
        self.network_sources_runtime.clone()
    }

    pub(crate) async fn run(mut self, shutdown: CancellationToken) -> Result<()> {
        self.rebuild_materialized_from_raw().await?;

        let socket = UdpSocket::bind(&self.cfg.listener.listen)
            .await
            .with_context(|| format!("failed to bind {}", self.cfg.listener.listen))?;

        let mut buffer = vec![0_u8; self.cfg.listener.max_packet_size];
        let mut entries_since_sync = 0_usize;
        let mut sync_tick = tokio::time::interval(self.cfg.listener.sync_interval);
        sync_tick.set_missed_tick_behavior(MissedTickBehavior::Skip);

        loop {
            tokio::select! {
                _ = shutdown.cancelled() => {
                    break;
                }
                _ = sync_tick.tick() => {
                    let now = now_usec();
                    self.decoders.refresh_enrichment_state();
                    if let Err(err) = self.flush_closed_tiers(now) {
                        tracing::warn!("tier flush failed: {}", err);
                    }
                    self.refresh_open_tier_state(now);
                    entries_since_sync = self.sync_if_needed(entries_since_sync);
                    self.persist_decoder_state_if_due(now);
                }
                recv = socket.recv_from(&mut buffer) => {
                    let (received, source) = match recv {
                        Ok(result) => result,
                        Err(err) => {
                            tracing::warn!("udp recv error: {}", err);
                            continue;
                        }
                    };

                    if received == 0 {
                        continue;
                    }

                    self.metrics.udp_packets_received.fetch_add(1, Ordering::Relaxed);
                    self.metrics
                        .udp_bytes_received
                        .fetch_add(received as u64, Ordering::Relaxed);

                    let receive_time_usec = now_usec();
                    let batch = self
                        .decoders
                        .decode_udp_payload_at(source, &buffer[..received], receive_time_usec);
                    self.metrics.apply_decode_stats(&batch.stats);

                    for flow in batch.flows {
                        let entry = entry_from_fields(&flow.fields);
                        let refs: Vec<&[u8]> = entry.iter().map(Vec::as_slice).collect();

                        if let Err(err) = self.raw_journal.write_entry(&refs, flow.source_realtime_usec) {
                            self.metrics
                                .journal_write_errors
                                .fetch_add(1, Ordering::Relaxed);
                            tracing::warn!("journal write failed: {}", err);
                            continue;
                        }

                        self.metrics
                            .journal_entries_written
                            .fetch_add(1, Ordering::Relaxed);
                        entries_since_sync += 1;

                        let ts = flow.source_realtime_usec.unwrap_or_else(now_usec);
                        self.observe_tiers(ts, &flow.fields);
                    }

                    if let Err(err) = self.flush_closed_tiers(now_usec()) {
                        tracing::warn!("tier flush failed: {}", err);
                    }
                    self.refresh_open_tier_state(now_usec());

                    if entries_since_sync >= self.cfg.listener.sync_every_entries {
                        entries_since_sync = self.sync_if_needed(entries_since_sync);
                    }
                }
            }
        }

        if let Err(err) = self.flush_closed_tiers(now_usec()) {
            tracing::warn!("tier flush failed during shutdown: {}", err);
        }
        self.refresh_open_tier_state(now_usec());
        let _ = self.sync_if_needed(entries_since_sync);
        let _ = self.sync_all_tiers();
        self.persist_decoder_state();
        Ok(())
    }

    async fn rebuild_materialized_from_raw(&mut self) -> Result<()> {
        let now = now_usec();
        let before = (now / 1_000_000).max(1) as u32;
        let after = before.saturating_sub(REBUILD_WINDOW_SECONDS);

        let raw_dir = self.cfg.journal.raw_tier_dir();
        let raw_dir_str = raw_dir
            .to_str()
            .context("raw tier directory contains invalid UTF-8")?;

        let (monitor, _notify_rx) = Monitor::new().context("failed to initialize raw monitor")?;
        let registry = Registry::new(monitor);
        registry
            .watch_directory(raw_dir_str)
            .with_context(|| format!("failed to watch raw tier directory {}", raw_dir.display()))?;

        let files = registry
            .find_files_in_range(Seconds(after), Seconds(before))
            .context("failed to find raw files for tier rebuild")?;
        if files.is_empty() {
            self.refresh_open_tier_state(now);
            return Ok(());
        }

        let cache_dir = self.cfg.journal.base_dir().join(".rebuild-index-cache");
        let cache = FileIndexCacheBuilder::new()
            .with_cache_path(cache_dir)
            .with_memory_capacity(REBUILD_CACHE_MEMORY_CAPACITY)
            .with_disk_capacity(REBUILD_CACHE_DISK_CAPACITY)
            .with_block_size(REBUILD_CACHE_BLOCK_SIZE)
            .build()
            .await
            .context("failed to initialize rebuild index cache")?;

        let source_timestamp_field = FieldName::new_unchecked("_SOURCE_REALTIME_TIMESTAMP");
        let facets = Facets::new(&["_SOURCE_REALTIME_TIMESTAMP".to_string()]);
        let keys: Vec<FileIndexKey> = files
            .iter()
            .map(|file_info| {
                FileIndexKey::new(
                    &file_info.file,
                    &facets,
                    Some(source_timestamp_field.clone()),
                )
            })
            .collect();

        let timeout = Timeout::new(Duration::from_secs(REBUILD_TIMEOUT_SECONDS));
        let time_range =
            QueryTimeRange::new(after, before).context("invalid rebuild raw time range")?;

        let indexed_files =
            batch_compute_file_indexes(&cache, &registry, keys, &time_range, timeout)
                .await
                .context("failed to build raw indexes for tier rebuild")?;
        let file_indexes: Vec<_> = indexed_files.into_iter().map(|(_, idx)| idx).collect();

        if file_indexes.is_empty() {
            self.refresh_open_tier_state(now);
            return Ok(());
        }

        let after_usec = (after as u64).saturating_mul(1_000_000);
        let before_usec = (before as u64).saturating_mul(1_000_000);
        let anchor_usec = before_usec.saturating_sub(1);

        let entries = LogQuery::new(
            &file_indexes,
            Anchor::Timestamp(Microseconds(anchor_usec)),
            Direction::Backward,
        )
        .with_after_usec(after_usec)
        .with_before_usec(before_usec)
        .execute()
        .context("failed to query raw flows for tier rebuild")?;

        for entry in entries {
            let mut fields = BTreeMap::new();
            for pair in entry.fields {
                fields.insert(pair.field().to_string(), pair.value().to_string());
            }

            let timestamp_usec = fields
                .get("_SOURCE_REALTIME_TIMESTAMP")
                .and_then(|v| v.parse::<u64>().ok())
                .unwrap_or(entry.timestamp);
            self.observe_tiers(timestamp_usec, &fields);
        }

        self.flush_closed_tiers(now)?;
        self.refresh_open_tier_state(now);
        Ok(())
    }

    fn observe_tiers(
        &mut self,
        timestamp_usec: u64,
        fields: &std::collections::BTreeMap<String, String>,
    ) {
        for tier in MATERIALIZED_TIERS {
            if let Some(acc) = self.tier_accumulators.get_mut(&tier) {
                acc.observe_flow(timestamp_usec, fields);
            }
        }
    }

    fn flush_closed_tiers(&mut self, now_usec: u64) -> Result<()> {
        for tier in MATERIALIZED_TIERS {
            let rows = if let Some(acc) = self.tier_accumulators.get_mut(&tier) {
                acc.flush_closed_rows(now_usec)
            } else {
                Vec::new()
            };

            if rows.is_empty() {
                continue;
            }

            let writer = self.tier_writers.get_mut(tier);
            for row in rows {
                let entry = entry_from_fields(&row.fields);
                let refs: Vec<&[u8]> = entry.iter().map(Vec::as_slice).collect();
                if let Err(err) = writer.write_entry(&refs, Some(row.timestamp_usec)) {
                    self.metrics
                        .tier_write_errors
                        .fetch_add(1, Ordering::Relaxed);
                    tracing::warn!("tier writer {:?} write failed: {}", tier, err);
                    continue;
                }
                self.metrics
                    .tier_entries_written
                    .fetch_add(1, Ordering::Relaxed);
            }
            self.metrics.tier_flushes.fetch_add(1, Ordering::Relaxed);
        }

        Ok(())
    }

    fn refresh_open_tier_state(&self, now_usec: u64) {
        let mut snapshot = OpenTierState::default();
        if let Some(acc) = self.tier_accumulators.get(&TierKind::Minute1) {
            snapshot.minute_1 = acc.snapshot_open_rows(now_usec);
        }
        if let Some(acc) = self.tier_accumulators.get(&TierKind::Minute5) {
            snapshot.minute_5 = acc.snapshot_open_rows(now_usec);
        }
        if let Some(acc) = self.tier_accumulators.get(&TierKind::Hour1) {
            snapshot.hour_1 = acc.snapshot_open_rows(now_usec);
        }

        if let Ok(mut guard) = self.open_tiers.write() {
            *guard = snapshot;
        }
    }

    fn sync_if_needed(&mut self, entries_since_sync: usize) -> usize {
        if entries_since_sync == 0 {
            return 0;
        }

        if let Err(err) = self.raw_journal.sync() {
            self.metrics
                .journal_sync_errors
                .fetch_add(1, Ordering::Relaxed);
            tracing::warn!("journal sync failed: {}", err);
        }

        0
    }

    fn sync_all_tiers(&mut self) -> usize {
        if let Err(err) = self.tier_writers.sync_all() {
            self.metrics
                .journal_sync_errors
                .fetch_add(1, Ordering::Relaxed);
            tracing::warn!("tier journal sync failed: {}", err);
            return 1;
        }
        0
    }

    fn persist_decoder_state_if_due(&mut self, now_usec: u64) {
        if now_usec.saturating_sub(self.last_decoder_state_persist_usec)
            < DECODER_STATE_PERSIST_INTERVAL_USEC
        {
            return;
        }
        self.persist_decoder_state();
        self.last_decoder_state_persist_usec = now_usec;
    }

    fn persist_decoder_state(&mut self) {
        let data = match self.decoders.export_persistent_state_json() {
            Ok(data) => data,
            Err(err) => {
                tracing::warn!("failed to serialize netflow decoder state: {}", err);
                return;
            }
        };

        let tmp_path = self.decoder_state_path.with_extension("json.tmp");
        if let Err(err) = fs::write(&tmp_path, data.as_bytes()) {
            tracing::warn!(
                "failed to write temporary netflow decoder state {}: {}",
                tmp_path.display(),
                err
            );
            return;
        }

        if let Err(err) = fs::rename(&tmp_path, &self.decoder_state_path) {
            tracing::warn!(
                "failed to move netflow decoder state {} to {}: {}",
                tmp_path.display(),
                self.decoder_state_path.display(),
                err
            );
            let _ = fs::remove_file(&tmp_path);
        }
    }
}

fn entry_from_fields(fields: &std::collections::BTreeMap<String, String>) -> Vec<Vec<u8>> {
    fields
        .iter()
        .map(|(name, value)| format!("{}={}", name, value).into_bytes())
        .collect()
}

fn now_usec() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_micros() as u64)
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::plugin_config::DecapsulationMode as ConfigDecapsulationMode;
    use etherparse::{NetSlice, SlicedPacket, TransportSlice};
    use pcap_file::pcap::PcapReader;
    use std::fs::File;
    use std::net::{IpAddr, SocketAddr};
    use std::path::{Path, PathBuf};
    use tempfile::TempDir;

    #[test]
    fn ingest_service_with_decap_none_keeps_outer_header_view() {
        let (_tmp, mut service) = new_test_ingest_service(ConfigDecapsulationMode::None);
        let flows = decode_fixture_sequence(
            &mut service,
            &["ipfix-srv6-template.pcap", "ipfix-srv6-data.pcap"],
        );
        assert!(
            !flows.is_empty(),
            "no flows decoded from ipfix-srv6 fixture sequence"
        );

        let outer = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "fc30:2200:1b::f"),
                ("DST_ADDR", "fc30:2200:23:e009::"),
                ("PROTOCOL", "4"),
            ],
        );
        assert_eq!(outer.get("BYTES").map(String::as_str), Some("104"));
        assert_eq!(outer.get("ETYPE").map(String::as_str), Some("34525"));
        assert_eq!(outer.get("DIRECTION").map(String::as_str), Some("ingress"));
    }

    #[test]
    fn ingest_service_with_decap_srv6_extracts_inner_header_view() {
        let (_tmp, mut service) = new_test_ingest_service(ConfigDecapsulationMode::Srv6);
        let flows = decode_fixture_sequence(
            &mut service,
            &["ipfix-srv6-template.pcap", "ipfix-srv6-data.pcap"],
        );
        assert!(
            !flows.is_empty(),
            "no flows decoded from ipfix-srv6 fixture sequence"
        );

        let inner = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "8.8.8.8"),
                ("DST_ADDR", "213.36.140.100"),
                ("PROTOCOL", "1"),
            ],
        );
        assert_eq!(inner.get("BYTES").map(String::as_str), Some("64"));
        assert_eq!(inner.get("PACKETS").map(String::as_str), Some("1"));
        assert_eq!(inner.get("IPTTL").map(String::as_str), Some("63"));
        assert_eq!(
            inner.get("IP_FRAGMENT_ID").map(String::as_str),
            Some("51563")
        );
        assert_eq!(inner.get("DIRECTION").map(String::as_str), Some("ingress"));
    }

    #[test]
    fn ingest_service_with_decap_vxlan_extracts_inner_header_view() {
        let (_tmp, mut service) = new_test_ingest_service(ConfigDecapsulationMode::Vxlan);
        let flows = decode_fixture_sequence(&mut service, &["data-encap-vxlan.pcap"]);
        assert!(
            !flows.is_empty(),
            "no flows decoded from data-encap-vxlan fixture"
        );

        let inner = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "2001:db8:4::1"),
                ("DST_ADDR", "2001:db8:4::3"),
                ("PROTOCOL", "58"),
            ],
        );
        assert_eq!(inner.get("BYTES").map(String::as_str), Some("104"));
        assert_eq!(inner.get("PACKETS").map(String::as_str), Some("1"));
        assert_eq!(inner.get("ETYPE").map(String::as_str), Some("34525"));
        assert_eq!(inner.get("IPTTL").map(String::as_str), Some("64"));
        assert_eq!(inner.get("ICMPV6_TYPE").map(String::as_str), Some("128"));
        assert_eq!(
            inner.get("SRC_MAC").map(String::as_str),
            Some("ca:6e:98:f8:49:8f")
        );
        assert_eq!(
            inner.get("DST_MAC").map(String::as_str),
            Some("01:02:03:04:05:06")
        );
    }

    #[test]
    fn ingest_service_restores_decoder_state_from_disk_after_restart() {
        let tmp = tempfile::tempdir().expect("create temp dir");

        let mut first = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
        let _ = decode_fixture_sequence(
            &mut first,
            &[
                "options-template.pcap",
                "options-data.pcap",
                "template.pcap",
                "ipfixprobe-templates.pcap",
            ],
        );
        first.persist_decoder_state();
        assert!(
            first.decoder_state_path.is_file(),
            "decoder state file was not persisted at {}",
            first.decoder_state_path.display()
        );

        let mut second = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);

        let v9_flows = decode_fixture_sequence(&mut second, &["data.pcap"]);
        assert_eq!(
            v9_flows.len(),
            4,
            "expected exactly four decoded v9 flows from data.pcap after restart restore, got {}",
            v9_flows.len()
        );
        let v9_flow = find_flow(
            &v9_flows,
            &[
                ("SRC_ADDR", "198.38.121.178"),
                ("DST_ADDR", "91.170.143.87"),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "443"),
                ("DST_PORT", "19624"),
            ],
        );
        assert_eq!(
            v9_flow.get("SAMPLING_RATE").map(String::as_str),
            Some("30000")
        );
        assert_eq!(v9_flow.get("BYTES").map(String::as_str), Some("1500"));
        assert_eq!(v9_flow.get("PACKETS").map(String::as_str), Some("1"));

        let ipfix_flows = decode_fixture_sequence(&mut second, &["ipfixprobe-data.pcap"]);
        assert_eq!(
            ipfix_flows.len(),
            6,
            "expected exactly six decoded IPFIX biflows after restart restore, got {}",
            ipfix_flows.len()
        );
        let ipfix_flow = find_flow(
            &ipfix_flows,
            &[
                ("SRC_ADDR", "10.10.1.4"),
                ("DST_ADDR", "10.10.1.1"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "56166"),
                ("DST_PORT", "53"),
            ],
        );
        assert_eq!(ipfix_flow.get("BYTES").map(String::as_str), Some("62"));
        assert_eq!(ipfix_flow.get("PACKETS").map(String::as_str), Some("1"));
    }

    fn new_test_ingest_service(
        decapsulation_mode: ConfigDecapsulationMode,
    ) -> (TempDir, IngestService) {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let service = new_test_ingest_service_in_dir(tmp.path(), decapsulation_mode);
        (tmp, service)
    }

    fn new_test_ingest_service_in_dir(
        base_dir: &Path,
        decapsulation_mode: ConfigDecapsulationMode,
    ) -> IngestService {
        let mut cfg = PluginConfig::default();
        cfg.journal.journal_dir = base_dir.join("flows").to_string_lossy().to_string();
        cfg.protocols.decapsulation_mode = decapsulation_mode;

        for dir in cfg.journal.all_tier_dirs() {
            std::fs::create_dir_all(&dir)
                .unwrap_or_else(|e| panic!("create tier directory {}: {e}", dir.display()));
        }

        IngestService::new(
            cfg,
            Arc::new(IngestMetrics::default()),
            Arc::new(RwLock::new(OpenTierState::default())),
        )
        .expect("create ingest service")
    }

    fn fixture_dir() -> PathBuf {
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../go/plugin/go.d/collector/netflow/testdata/flows")
    }

    fn decode_fixture_sequence(
        service: &mut IngestService,
        fixtures: &[&str],
    ) -> Vec<BTreeMap<String, String>> {
        let base = fixture_dir();
        let mut out = Vec::new();
        for fixture in fixtures {
            out.extend(decode_pcap_flows(
                &base.join(fixture),
                &mut service.decoders,
            ));
        }
        out
    }

    fn decode_pcap_flows(
        path: &Path,
        decoders: &mut FlowDecoders,
    ) -> Vec<BTreeMap<String, String>> {
        let file = File::open(path).unwrap_or_else(|e| panic!("open {}: {e}", path.display()));
        let mut reader =
            PcapReader::new(file).unwrap_or_else(|e| panic!("pcap reader {}: {e}", path.display()));

        let mut flows = Vec::new();
        while let Some(packet) = reader.next_packet() {
            let packet = packet.unwrap_or_else(|e| panic!("read packet {}: {e}", path.display()));
            if let Some((source, payload)) = extract_udp_payload(packet.data.as_ref()) {
                let decoded = decoders.decode_udp_payload(source, payload);
                flows.extend(decoded.flows.into_iter().map(|flow| flow.fields));
            }
        }
        flows
    }

    fn extract_udp_payload(packet: &[u8]) -> Option<(SocketAddr, &[u8])> {
        let sliced = SlicedPacket::from_ethernet(packet).ok()?;
        let src_ip = match sliced.net {
            Some(NetSlice::Ipv4(v4)) => IpAddr::V4(v4.header().source_addr()),
            Some(NetSlice::Ipv6(v6)) => IpAddr::V6(v6.header().source_addr()),
            _ => return None,
        };

        let (src_port, payload) = match sliced.transport {
            Some(TransportSlice::Udp(udp)) => (udp.source_port(), udp.payload()),
            _ => return None,
        };
        Some((SocketAddr::new(src_ip, src_port), payload))
    }

    fn find_flow<'a>(
        flows: &'a [BTreeMap<String, String>],
        predicates: &[(&str, &str)],
    ) -> &'a BTreeMap<String, String> {
        flows
            .iter()
            .find(|fields| {
                predicates
                    .iter()
                    .all(|(k, v)| fields.get(*k).map(String::as_str) == Some(*v))
            })
            .unwrap_or_else(|| {
                panic!(
                    "flow not found for predicates {:?}; decoded flow count={}",
                    predicates,
                    flows.len()
                )
            })
    }
}
