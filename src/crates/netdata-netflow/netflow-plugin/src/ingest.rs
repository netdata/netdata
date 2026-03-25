use crate::decoder::{
    DecapsulationMode as DecoderDecapsulationMode, DecodeStats, DecoderStateNamespaceKey,
    FlowDecoders, TimestampSource as DecoderTimestampSource,
};
use crate::enrichment::{DynamicRoutingRuntime, FlowEnricher, NetworkSourcesRuntime};
use crate::plugin_config::{
    DecapsulationMode as ConfigDecapsulationMode, PluginConfig,
    TimestampSource as ConfigTimestampSource,
};
use crate::tiering::{
    MATERIALIZED_TIERS, OpenTierState, TierAccumulator, TierFlowIndexStore, TierKind,
};
use anyhow::{Context, Result, anyhow};
use journal_common::load_machine_id;
use journal_engine::{
    Facets, FileIndexCacheBuilder, FileIndexKey, IndexingLimits, LogQuery, QueryTimeRange,
    batch_compute_file_indexes,
};
use journal_index::{Anchor, Direction, FieldName, Microseconds, Seconds};
use journal_log_writer::{Config, EntryTimestamps, Log, RetentionPolicy, RotationPolicy};
use journal_registry::{Monitor, Origin, Registry, Source};
use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};
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
    pub(crate) raw_journal_logical_bytes: AtomicU64,
    pub(crate) journal_write_errors: AtomicU64,
    pub(crate) journal_sync_errors: AtomicU64,
    pub(crate) raw_journal_syncs: AtomicU64,
    pub(crate) raw_journal_sync_errors: AtomicU64,
    pub(crate) tier_entries_written: AtomicU64,
    pub(crate) minute_1_entries_written: AtomicU64,
    pub(crate) minute_5_entries_written: AtomicU64,
    pub(crate) hour_1_entries_written: AtomicU64,
    pub(crate) minute_1_logical_bytes: AtomicU64,
    pub(crate) minute_5_logical_bytes: AtomicU64,
    pub(crate) hour_1_logical_bytes: AtomicU64,
    pub(crate) tier_write_errors: AtomicU64,
    pub(crate) tier_flushes: AtomicU64,
    pub(crate) tier_journal_syncs: AtomicU64,
    pub(crate) tier_journal_sync_errors: AtomicU64,
    pub(crate) decoder_state_persist_calls: AtomicU64,
    pub(crate) decoder_state_persist_bytes: AtomicU64,
    pub(crate) decoder_state_write_errors: AtomicU64,
    pub(crate) decoder_state_move_errors: AtomicU64,
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
            "raw_journal_logical_bytes".to_string(),
            self.raw_journal_logical_bytes.load(Ordering::Relaxed),
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
            "raw_journal_syncs".to_string(),
            self.raw_journal_syncs.load(Ordering::Relaxed),
        );
        stats.insert(
            "raw_journal_sync_errors".to_string(),
            self.raw_journal_sync_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "tier_entries_written".to_string(),
            self.tier_entries_written.load(Ordering::Relaxed),
        );
        stats.insert(
            "minute_1_entries_written".to_string(),
            self.minute_1_entries_written.load(Ordering::Relaxed),
        );
        stats.insert(
            "minute_5_entries_written".to_string(),
            self.minute_5_entries_written.load(Ordering::Relaxed),
        );
        stats.insert(
            "hour_1_entries_written".to_string(),
            self.hour_1_entries_written.load(Ordering::Relaxed),
        );
        stats.insert(
            "minute_1_logical_bytes".to_string(),
            self.minute_1_logical_bytes.load(Ordering::Relaxed),
        );
        stats.insert(
            "minute_5_logical_bytes".to_string(),
            self.minute_5_logical_bytes.load(Ordering::Relaxed),
        );
        stats.insert(
            "hour_1_logical_bytes".to_string(),
            self.hour_1_logical_bytes.load(Ordering::Relaxed),
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
            "tier_journal_syncs".to_string(),
            self.tier_journal_syncs.load(Ordering::Relaxed),
        );
        stats.insert(
            "tier_journal_sync_errors".to_string(),
            self.tier_journal_sync_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoder_state_persist_calls".to_string(),
            self.decoder_state_persist_calls.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoder_state_persist_bytes".to_string(),
            self.decoder_state_persist_bytes.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoder_state_write_errors".to_string(),
            self.decoder_state_write_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoder_state_move_errors".to_string(),
            self.decoder_state_move_errors.load(Ordering::Relaxed),
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
    decoder_state_dir: PathBuf,
    last_decoder_state_persist_usec: u64,
    raw_journal: Log,
    tier_writers: MaterializedTierWriters,
    tier_accumulators: HashMap<TierKind, TierAccumulator>,
    open_tiers: Arc<RwLock<OpenTierState>>,
    tier_flow_indexes: Arc<RwLock<TierFlowIndexStore>>,
    routing_runtime: Option<DynamicRoutingRuntime>,
    network_sources_runtime: Option<NetworkSourcesRuntime>,
    encode_buf: JournalEncodeBuffer,
}

impl IngestService {
    pub(crate) fn new(
        cfg: PluginConfig,
        metrics: Arc<IngestMetrics>,
        open_tiers: Arc<RwLock<OpenTierState>>,
        tier_flow_indexes: Arc<RwLock<TierFlowIndexStore>>,
    ) -> Result<Self> {
        let machine_id = load_machine_id().context("failed to load machine id")?;
        let build_journal_cfg = |tier: TierKind| {
            let origin = Origin {
                machine_id: Some(machine_id.clone()),
                namespace: None,
                source: Source::System,
            };
            let rotation_policy = RotationPolicy::default()
                .with_size_of_journal_file(cfg.journal.size_of_journal_file.as_u64())
                .with_duration_of_journal_file(cfg.journal.duration_of_journal_file);
            let retention = cfg.journal.retention_for_tier(tier);
            let retention_policy = RetentionPolicy::default()
                .with_number_of_journal_files(retention.number_of_journal_files)
                .with_size_of_journal_files(retention.size_of_journal_files.as_u64())
                .with_duration_of_journal_files(retention.duration_of_journal_files);
            Config::new(origin, rotation_policy, retention_policy).with_machine_id_suffix(false)
        };

        let raw_dir = cfg.journal.raw_tier_dir();
        let raw_journal =
            Log::new(&raw_dir, build_journal_cfg(TierKind::Raw)).with_context(|| {
                format!(
                    "failed to create journal writer in directory {}",
                    raw_dir.display()
                )
            })?;
        let minute_1_dir = cfg.journal.minute_1_tier_dir();
        let minute_5_dir = cfg.journal.minute_5_tier_dir();
        let hour_1_dir = cfg.journal.hour_1_tier_dir();
        let tier_writers =
            MaterializedTierWriters {
                minute_1: Log::new(&minute_1_dir, build_journal_cfg(TierKind::Minute1))
                    .with_context(|| {
                        format!(
                            "failed to create 1m tier writer in directory {}",
                            minute_1_dir.display()
                        )
                    })?,
                minute_5: Log::new(&minute_5_dir, build_journal_cfg(TierKind::Minute5))
                    .with_context(|| {
                        format!(
                            "failed to create 5m tier writer in directory {}",
                            minute_5_dir.display()
                        )
                    })?,
                hour_1: Log::new(&hour_1_dir, build_journal_cfg(TierKind::Hour1)).with_context(
                    || {
                        format!(
                            "failed to create 1h tier writer in directory {}",
                            hour_1_dir.display()
                        )
                    },
                )?,
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
        let decoder_state_dir = cfg.journal.decoder_state_dir();
        if let Err(err) = fs::create_dir_all(&decoder_state_dir) {
            tracing::warn!(
                "failed to prepare netflow decoder state directory {}: {}",
                decoder_state_dir.display(),
                err
            );
        }
        Self::preload_decoder_state_namespaces(&mut decoders, &decoder_state_dir);

        Ok(Self {
            cfg,
            metrics,
            decoders,
            decoder_state_dir,
            last_decoder_state_persist_usec: now_usec(),
            raw_journal,
            tier_writers,
            tier_accumulators,
            open_tiers,
            tier_flow_indexes,
            routing_runtime,
            network_sources_runtime,
            encode_buf: JournalEncodeBuffer::new(),
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
                    self.prune_unused_tier_flow_indexes();
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
                    self.prepare_decoder_state_namespace(source, &buffer[..received]);
                    let batch = self
                        .decoders
                        .decode_udp_payload_at(source, &buffer[..received], receive_time_usec);
                    self.metrics.apply_decode_stats(&batch.stats);

                    for flow in batch.flows {
                        let timestamps = EntryTimestamps::default()
                            .with_source_realtime_usec(receive_time_usec)
                            .with_entry_realtime_usec(receive_time_usec);

                        if let Err(err) = self
                            .encode_buf
                            .encode_record_and_write(&flow.record, &mut self.raw_journal, timestamps)
                        {
                            self.metrics
                                .journal_write_errors
                                .fetch_add(1, Ordering::Relaxed);
                            tracing::warn!("journal write failed: {}", err);
                            continue;
                        }

                        self.metrics
                            .journal_entries_written
                            .fetch_add(1, Ordering::Relaxed);
                        self.metrics
                            .raw_journal_logical_bytes
                            .fetch_add(self.encode_buf.encoded_len(), Ordering::Relaxed);
                        entries_since_sync += 1;

                        self.observe_tiers_record(receive_time_usec, &flow.record);
                    }

                    if let Err(err) = self.flush_closed_tiers(now_usec()) {
                        tracing::warn!("tier flush failed: {}", err);
                    }
                    self.prune_unused_tier_flow_indexes();
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
        self.prune_unused_tier_flow_indexes();
        self.refresh_open_tier_state(now_usec());
        let _ = self.sync_if_needed(entries_since_sync);
        let _ = self.sync_all_tiers();
        self.persist_decoder_state();
        Ok(())
    }

    /// Query the most recent `_SOURCE_REALTIME_TIMESTAMP` from a tier's journal
    /// files. Returns `None` when the tier directory is empty or unreadable.
    async fn find_last_tier_timestamp(
        tier_dir: &std::path::Path,
        cache: &journal_engine::FileIndexCache,
    ) -> Option<u64> {
        let dir_str = tier_dir.to_str()?;
        let (monitor, _rx) = Monitor::new().ok()?;
        let registry = Registry::new(monitor);
        registry.watch_directory(dir_str).ok()?;

        let files = registry
            .find_files_in_range(Seconds(0), Seconds(u32::MAX))
            .ok()?;
        if files.is_empty() {
            return None;
        }

        let source_ts = FieldName::new_unchecked("_SOURCE_REALTIME_TIMESTAMP");
        let facets = Facets::new(&["_SOURCE_REALTIME_TIMESTAMP".to_string()]);
        let keys: Vec<FileIndexKey> = files
            .iter()
            .map(|fi| FileIndexKey::new(&fi.file, &facets, Some(source_ts.clone())))
            .collect();

        let time_range = QueryTimeRange::new(0, u32::MAX).ok()?;
        let cancel = CancellationToken::new();
        let indexed = batch_compute_file_indexes(
            cache,
            &registry,
            keys,
            &time_range,
            cancel,
            IndexingLimits::default(),
            None,
        )
        .await
        .ok()?;

        let indexes: Vec<_> = indexed.into_iter().map(|(_, idx)| idx).collect();
        if indexes.is_empty() {
            return None;
        }

        let entries = LogQuery::new(&indexes, Anchor::Tail, Direction::Backward)
            .with_limit(1)
            .execute()
            .ok()?;

        let entry = entries.into_iter().next()?;
        for pair in &entry.fields {
            if pair.field() == "_SOURCE_REALTIME_TIMESTAMP" {
                return pair.value().parse::<u64>().ok();
            }
        }
        Some(entry.timestamp)
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

        // Find per-tier cutoffs to avoid duplicating already-flushed data.
        let mut tier_cutoffs = HashMap::new();
        for tier in MATERIALIZED_TIERS {
            let tier_dir = self.cfg.journal.tier_dir(tier);
            if let Some(ts) = Self::find_last_tier_timestamp(&tier_dir, &cache).await {
                tracing::info!(
                    "tier {:?}: last flushed timestamp {} — skipping rebuild before it",
                    tier,
                    ts
                );
                tier_cutoffs.insert(tier, ts);
            }
        }

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

        let time_range =
            QueryTimeRange::new(after, before).context("invalid rebuild raw time range")?;

        let indexing_cancellation = CancellationToken::new();
        let indexed_files = tokio::select! {
            result = batch_compute_file_indexes(
                &cache,
                &registry,
                keys,
                &time_range,
                indexing_cancellation.clone(),
                IndexingLimits::default(),
                None,
            ) => result.context("failed to build raw indexes for tier rebuild"),
            _ = tokio::time::sleep(Duration::from_secs(REBUILD_TIMEOUT_SECONDS)) => {
                indexing_cancellation.cancel();
                Err(anyhow!(
                    "timed out building raw indexes for tier rebuild after {}s",
                    REBUILD_TIMEOUT_SECONDS
                ))
            }
        }?;
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
            let mut fields = crate::decoder::FlowFields::new();
            for pair in entry.fields {
                let name = pair.field();
                if let Some(interned) = crate::decoder::intern_field_name(name) {
                    fields.insert(interned, pair.value().to_string());
                }
            }

            self.observe_tiers_with_cutoffs(entry.timestamp, &fields, &tier_cutoffs);
        }

        self.flush_closed_tiers(now)?;
        self.prune_unused_tier_flow_indexes();
        self.refresh_open_tier_state(now);
        Ok(())
    }

    fn observe_tiers_record(&mut self, timestamp_usec: u64, record: &crate::decoder::FlowRecord) {
        use crate::tiering::FlowMetrics;
        let metrics = FlowMetrics::from_record(record);
        let flow_ref = {
            let Ok(mut tier_flow_indexes) = self.tier_flow_indexes.write() else {
                tracing::warn!("failed to lock tier flow index store for write");
                return;
            };
            match tier_flow_indexes.get_or_insert_record_flow(timestamp_usec, record) {
                Ok(flow_ref) => flow_ref,
                Err(err) => {
                    tracing::warn!("failed to intern tier flow dimensions: {}", err);
                    return;
                }
            }
        };
        for tier in MATERIALIZED_TIERS {
            if let Some(acc) = self.tier_accumulators.get_mut(&tier) {
                acc.observe_flow(timestamp_usec, flow_ref, metrics);
            }
        }
    }

    /// Cold path: observe tiers from FlowFields (journal replay at startup).
    /// Skips flows that fall into already-flushed
    /// buckets to prevent duplicate entries on restart.
    fn observe_tiers_with_cutoffs(
        &mut self,
        timestamp_usec: u64,
        fields: &crate::decoder::FlowFields,
        cutoffs: &HashMap<TierKind, u64>,
    ) {
        use crate::tiering::FlowMetrics;
        let record = crate::decoder::FlowRecord::from_fields(fields);
        let metrics = FlowMetrics::from_fields(fields);
        let flow_ref = {
            let Ok(mut tier_flow_indexes) = self.tier_flow_indexes.write() else {
                tracing::warn!("failed to lock tier flow index store for rebuild write");
                return;
            };
            match tier_flow_indexes.get_or_insert_record_flow(timestamp_usec, &record) {
                Ok(flow_ref) => flow_ref,
                Err(err) => {
                    tracing::warn!("failed to intern rebuild tier flow dimensions: {}", err);
                    return;
                }
            }
        };
        for tier in MATERIALIZED_TIERS {
            if let Some(&cutoff) = cutoffs.get(&tier) {
                if timestamp_usec <= cutoff {
                    continue;
                }
            }
            if let Some(acc) = self.tier_accumulators.get_mut(&tier) {
                acc.observe_flow(timestamp_usec, flow_ref, metrics);
            }
        }
    }

    fn flush_closed_tiers(&mut self, now_usec: u64) -> Result<()> {
        let tier_flow_indexes = self
            .tier_flow_indexes
            .read()
            .map_err(|_| anyhow!("failed to lock tier flow index store for read"))?;
        for tier in MATERIALIZED_TIERS {
            let rows = if let Some(acc) = self.tier_accumulators.get_mut(&tier) {
                acc.flush_closed_rows(now_usec)
            } else {
                Vec::new()
            };

            if rows.is_empty() {
                continue;
            }

            for row in rows {
                let Some(mut fields) = tier_flow_indexes.materialize_fields(row.flow_ref) else {
                    tracing::warn!(
                        "failed to materialize tier flow {:?} for {:?}",
                        row.flow_ref,
                        tier
                    );
                    continue;
                };
                row.metrics.write_fields(&mut fields);
                self.encode_buf.encode(&fields);
                let logical_bytes = self.encode_buf.encoded_len();
                let refs = self.encode_buf.field_slices();
                let timestamps = EntryTimestamps::default()
                    .with_source_realtime_usec(row.timestamp_usec)
                    .with_entry_realtime_usec(row.timestamp_usec);
                let write_result = {
                    let writer = self.tier_writers.get_mut(tier);
                    writer.write_entry_with_timestamps(&refs, timestamps)
                };
                if let Err(err) = write_result {
                    self.metrics
                        .tier_write_errors
                        .fetch_add(1, Ordering::Relaxed);
                    tracing::warn!("tier writer {:?} write failed: {}", tier, err);
                    continue;
                }
                self.metrics
                    .tier_entries_written
                    .fetch_add(1, Ordering::Relaxed);
                self.increment_materialized_tier_metrics(tier, logical_bytes);
            }
            self.metrics.tier_flushes.fetch_add(1, Ordering::Relaxed);
        }

        Ok(())
    }

    fn prune_unused_tier_flow_indexes(&self) {
        let mut active_hours = std::collections::BTreeSet::new();
        for tier in MATERIALIZED_TIERS {
            if let Some(acc) = self.tier_accumulators.get(&tier) {
                active_hours.extend(acc.active_hours());
            }
        }

        if let Ok(mut tier_flow_indexes) = self.tier_flow_indexes.write() {
            tier_flow_indexes.prune_unused_hours(&active_hours);
        }
    }

    fn refresh_open_tier_state(&self, now_usec: u64) {
        let mut snapshot = OpenTierState::default();
        if let Ok(tier_flow_indexes) = self.tier_flow_indexes.read() {
            snapshot.generation = tier_flow_indexes.generation();
        }
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

        self.metrics
            .raw_journal_syncs
            .fetch_add(1, Ordering::Relaxed);
        if let Err(err) = self.raw_journal.sync() {
            self.metrics
                .journal_sync_errors
                .fetch_add(1, Ordering::Relaxed);
            self.metrics
                .raw_journal_sync_errors
                .fetch_add(1, Ordering::Relaxed);
            tracing::warn!("journal sync failed: {}", err);
        }

        0
    }

    fn sync_all_tiers(&mut self) -> usize {
        self.metrics
            .tier_journal_syncs
            .fetch_add(1, Ordering::Relaxed);
        if let Err(err) = self.tier_writers.sync_all() {
            self.metrics
                .journal_sync_errors
                .fetch_add(1, Ordering::Relaxed);
            self.metrics
                .tier_journal_sync_errors
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
        for key in self.decoders.dirty_decoder_state_namespaces() {
            let path = self.decoder_state_namespace_path(&key);
            let data = match self.decoders.export_decoder_state_namespace(&key) {
                Ok(Some(data)) => data,
                Ok(None) => {
                    if path.is_file()
                        && let Err(err) = fs::remove_file(&path)
                    {
                        self.metrics
                            .decoder_state_write_errors
                            .fetch_add(1, Ordering::Relaxed);
                        tracing::warn!(
                            "failed to remove stale netflow decoder state {}: {}",
                            path.display(),
                            err
                        );
                        continue;
                    }
                    self.decoders.mark_decoder_state_namespace_persisted(&key);
                    continue;
                }
                Err(err) => {
                    tracing::warn!(
                        "failed to serialize netflow decoder state namespace {}: {}",
                        path.display(),
                        err
                    );
                    continue;
                }
            };

            self.metrics
                .decoder_state_persist_calls
                .fetch_add(1, Ordering::Relaxed);
            self.metrics
                .decoder_state_persist_bytes
                .fetch_add(data.len() as u64, Ordering::Relaxed);

            let tmp_path = path.with_extension("bin.tmp");
            if let Err(err) = fs::write(&tmp_path, &data) {
                self.metrics
                    .decoder_state_write_errors
                    .fetch_add(1, Ordering::Relaxed);
                tracing::warn!(
                    "failed to write temporary netflow decoder state {}: {}",
                    tmp_path.display(),
                    err
                );
                continue;
            }

            if let Err(err) = fs::rename(&tmp_path, &path) {
                self.metrics
                    .decoder_state_move_errors
                    .fetch_add(1, Ordering::Relaxed);
                tracing::warn!(
                    "failed to move netflow decoder state {} to {}: {}",
                    tmp_path.display(),
                    path.display(),
                    err
                );
                let _ = fs::remove_file(&tmp_path);
                continue;
            }

            self.decoders.mark_decoder_state_namespace_persisted(&key);
        }
    }

    fn decoder_state_namespace_path(&self, key: &DecoderStateNamespaceKey) -> PathBuf {
        self.decoder_state_dir
            .join(FlowDecoders::decoder_state_namespace_filename(key))
    }

    fn prepare_decoder_state_namespace(&mut self, source: std::net::SocketAddr, payload: &[u8]) {
        let Some(key) = FlowDecoders::decoder_state_namespace_key(source, payload) else {
            return;
        };

        if !self.decoders.is_decoder_state_namespace_loaded(&key) {
            self.decoders
                .mark_decoder_state_namespace_absent(key, source);
            return;
        }

        if self
            .decoders
            .decoder_state_source_needs_hydration(&key, source)
            && let Err(err) = self
                .decoders
                .hydrate_loaded_decoder_state_namespace(&key, source)
        {
            tracing::warn!(
                "failed to hydrate netflow decoder state namespace {} for {}: {}",
                self.decoder_state_namespace_path(&key).display(),
                source,
                err
            );
        }
    }

    fn preload_decoder_state_namespaces(decoders: &mut FlowDecoders, decoder_state_dir: &Path) {
        let read_dir = match fs::read_dir(decoder_state_dir) {
            Ok(entries) => entries,
            Err(err) => {
                tracing::warn!(
                    "failed to read netflow decoder state directory {}: {}",
                    decoder_state_dir.display(),
                    err
                );
                return;
            }
        };

        let mut paths = read_dir
            .filter_map(|entry| entry.ok().map(|entry| entry.path()))
            .filter(|path| path.is_file())
            .collect::<Vec<_>>();
        paths.sort();

        for path in paths {
            match fs::read(&path) {
                Ok(data) => {
                    if let Err(err) = decoders.preload_decoder_state_namespace(&data) {
                        tracing::warn!(
                            "failed to preload netflow decoder state namespace {}: {}",
                            path.display(),
                            err
                        );
                    }
                }
                Err(err) => {
                    tracing::warn!(
                        "failed to read netflow decoder state namespace {}: {}",
                        path.display(),
                        err
                    );
                }
            }
        }
    }

    fn increment_materialized_tier_metrics(&self, tier: TierKind, logical_bytes: u64) {
        match tier {
            TierKind::Minute1 => {
                self.metrics
                    .minute_1_entries_written
                    .fetch_add(1, Ordering::Relaxed);
                self.metrics
                    .minute_1_logical_bytes
                    .fetch_add(logical_bytes, Ordering::Relaxed);
            }
            TierKind::Minute5 => {
                self.metrics
                    .minute_5_entries_written
                    .fetch_add(1, Ordering::Relaxed);
                self.metrics
                    .minute_5_logical_bytes
                    .fetch_add(logical_bytes, Ordering::Relaxed);
            }
            TierKind::Hour1 => {
                self.metrics
                    .hour_1_entries_written
                    .fetch_add(1, Ordering::Relaxed);
                self.metrics
                    .hour_1_logical_bytes
                    .fetch_add(logical_bytes, Ordering::Relaxed);
            }
            TierKind::Raw => {}
        }
    }
}

/// Reusable buffer for encoding flow fields into journal entries.
/// Avoids ~60 Vec<u8> allocations per flow by writing all fields into
/// a single contiguous buffer and tracking offsets.
struct JournalEncodeBuffer {
    data: Vec<u8>,
    refs: Vec<std::ops::Range<usize>>,
}

impl JournalEncodeBuffer {
    fn new() -> Self {
        Self {
            data: Vec::with_capacity(4096),
            refs: Vec::with_capacity(64),
        }
    }

    /// Encode a FlowRecord and write to journal in one call.
    /// Uses a stack-allocated array for field slices — zero heap allocation.
    /// The borrow of self.data is contained within this method.
    fn encode_record_and_write(
        &mut self,
        record: &crate::decoder::FlowRecord,
        journal: &mut journal_log_writer::Log,
        timestamps: journal_log_writer::EntryTimestamps,
    ) -> journal_log_writer::Result<()> {
        record.encode_to_journal_buf(&mut self.data, &mut self.refs);
        // 87 canonical fields — stack array avoids heap allocation.
        let mut slices = [&[] as &[u8]; 87];
        let n = self.refs.len().min(87);
        for (i, r) in self.refs[..n].iter().enumerate() {
            slices[i] = &self.data[r.clone()];
        }
        journal.write_entry_with_timestamps(&slices[..n], timestamps)
    }

    fn encode(&mut self, fields: &crate::decoder::FlowFields) {
        self.data.clear();
        self.refs.clear();

        for (name, value) in fields {
            let start = self.data.len();
            self.data.extend_from_slice(name.as_bytes());
            self.data.push(b'=');
            self.data.extend_from_slice(value.as_bytes());
            self.refs.push(start..self.data.len());
        }
    }

    fn field_slices(&self) -> Vec<&[u8]> {
        self.refs.iter().map(|r| &self.data[r.clone()]).collect()
    }

    fn encoded_len(&self) -> u64 {
        self.data.len() as u64
    }
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
            first.decoder_state_dir.is_dir(),
            "decoder state directory was not prepared at {}",
            first.decoder_state_dir.display()
        );
        let persisted_files = std::fs::read_dir(&first.decoder_state_dir)
            .unwrap_or_else(|e| {
                panic!(
                    "read decoder state dir {}: {e}",
                    first.decoder_state_dir.display()
                )
            })
            .count();
        assert!(
            persisted_files >= 2,
            "expected decoder namespace files in {}, got {persisted_files}",
            first.decoder_state_dir.display()
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

    #[test]
    fn ingest_service_persist_decoder_state_skips_clean_namespaces() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let mut service = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);

        let _ = decode_fixture_sequence(&mut service, &["template.pcap"]);
        service.persist_decoder_state();

        let calls_after_first = service
            .metrics
            .decoder_state_persist_calls
            .load(Ordering::Relaxed);
        let bytes_after_first = service
            .metrics
            .decoder_state_persist_bytes
            .load(Ordering::Relaxed);
        let persisted_files_after_first = std::fs::read_dir(&service.decoder_state_dir)
            .unwrap_or_else(|e| {
                panic!(
                    "read decoder state dir {}: {e}",
                    service.decoder_state_dir.display()
                )
            })
            .count();

        assert_eq!(
            calls_after_first, 1,
            "expected exactly one dirty namespace write"
        );
        assert!(bytes_after_first > 0, "expected persisted namespace bytes");
        assert_eq!(
            persisted_files_after_first, 1,
            "expected one persisted namespace file"
        );

        service.persist_decoder_state();

        assert_eq!(
            service
                .metrics
                .decoder_state_persist_calls
                .load(Ordering::Relaxed),
            calls_after_first,
            "clean namespaces should not be rewritten"
        );
        assert_eq!(
            service
                .metrics
                .decoder_state_persist_bytes
                .load(Ordering::Relaxed),
            bytes_after_first,
            "clean namespaces should not add persisted bytes"
        );
        let persisted_files_after_second = std::fs::read_dir(&service.decoder_state_dir)
            .unwrap_or_else(|e| {
                panic!(
                    "read decoder state dir {}: {e}",
                    service.decoder_state_dir.display()
                )
            })
            .count();
        assert_eq!(
            persisted_files_after_second, persisted_files_after_first,
            "clean namespaces should not create extra files"
        );
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
            Arc::new(RwLock::new(TierFlowIndexStore::default())),
        )
        .expect("create ingest service")
    }

    fn fixture_dir() -> PathBuf {
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("testdata/flows")
    }

    fn decode_fixture_sequence(
        service: &mut IngestService,
        fixtures: &[&str],
    ) -> Vec<crate::decoder::FlowFields> {
        let base = fixture_dir();
        let mut out = Vec::new();
        for fixture in fixtures {
            out.extend(decode_pcap_flows(&base.join(fixture), service));
        }
        out
    }

    fn decode_pcap_flows(
        path: &Path,
        service: &mut IngestService,
    ) -> Vec<crate::decoder::FlowFields> {
        let file = File::open(path).unwrap_or_else(|e| panic!("open {}: {e}", path.display()));
        let mut reader =
            PcapReader::new(file).unwrap_or_else(|e| panic!("pcap reader {}: {e}", path.display()));

        let mut flows = Vec::new();
        while let Some(packet) = reader.next_packet() {
            let packet = packet.unwrap_or_else(|e| panic!("read packet {}: {e}", path.display()));
            if let Some((source, payload)) = extract_udp_payload(packet.data.as_ref()) {
                service.prepare_decoder_state_namespace(source, payload);
                let decoded = service.decoders.decode_udp_payload(source, payload);
                flows.extend(
                    decoded
                        .flows
                        .into_iter()
                        .map(|flow| flow.record.to_fields()),
                );
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
        flows: &'a [crate::decoder::FlowFields],
        predicates: &[(&str, &str)],
    ) -> &'a crate::decoder::FlowFields {
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

    /// Pre-extracted UDP payload for benchmark replay.
    struct UdpPayload {
        source: SocketAddr,
        data: Vec<u8>,
    }

    /// Extract raw UDP payloads from pcap files (without decoding).
    fn extract_udp_payloads(path: &Path) -> Vec<UdpPayload> {
        let file = File::open(path).unwrap_or_else(|e| panic!("open {}: {e}", path.display()));
        let mut reader =
            PcapReader::new(file).unwrap_or_else(|e| panic!("pcap reader {}: {e}", path.display()));
        let mut payloads = Vec::new();
        while let Some(packet) = reader.next_packet() {
            let packet = packet.unwrap_or_else(|e| panic!("read packet {}: {e}", path.display()));
            if let Some((source, payload)) = extract_udp_payload(packet.data.as_ref()) {
                payloads.push(UdpPayload {
                    source,
                    data: payload.to_vec(),
                });
            }
        }
        payloads
    }

    /// Benchmark: full hot path throughput.
    ///
    /// Measures decode → journal_encode+write → tier_observe.
    /// Replays pcap data packets in a tight loop.
    /// Run with: cargo test -p netflow-plugin --release -- bench_full_hot_path --nocapture --ignored
    #[test]
    #[ignore] // Only run explicitly
    fn bench_full_hot_path() {
        let (_tmp, mut service) = new_test_ingest_service(ConfigDecapsulationMode::None);
        let base = fixture_dir();

        // Phase 1: Load templates into decoder state.
        let template_files = [
            "options-template.pcap",
            "options-data.pcap",
            "template.pcap",
            "ipfixprobe-templates.pcap",
            "icmp-template.pcap",
            "samplingrate-template.pcap",
            "multiplesamplingrates-options-template.pcap",
            "multiplesamplingrates-template.pcap",
        ];
        for tf in &template_files {
            let payloads = extract_udp_payloads(&base.join(tf));
            for p in &payloads {
                service.decoders.decode_udp_payload(p.source, &p.data);
            }
        }

        // Phase 2: Extract data payloads into memory.
        let data_files = [
            "data.pcap",                       // v9 data
            "ipfixprobe-data.pcap",            // IPFIX biflows
            "nfv5.pcap",                       // NetFlow v5
            "icmp-data.pcap",                  // ICMP
            "samplingrate-data.pcap",          // v9 with sampling
            "multiplesamplingrates-data.pcap", // v9 multiple sampling
        ];
        let mut data_payloads = Vec::new();
        for df in &data_files {
            data_payloads.extend(extract_udp_payloads(&base.join(df)));
        }
        assert!(
            !data_payloads.is_empty(),
            "no data payloads extracted from fixture files"
        );

        // Decode once to count flows per iteration
        let mut flows_per_round = 0_usize;
        for p in &data_payloads {
            let batch = service.decoders.decode_udp_payload(p.source, &p.data);
            flows_per_round += batch.flows.len();
        }
        eprintln!("Payloads per round: {}", data_payloads.len());
        eprintln!("Flows per round:    {}", flows_per_round);

        // Phase 3: Warmup (5 rounds).
        let warmup_rounds = 5;
        for _ in 0..warmup_rounds {
            for p in &data_payloads {
                let receive_time_usec = super::now_usec();
                let batch =
                    service
                        .decoders
                        .decode_udp_payload_at(p.source, &p.data, receive_time_usec);
                for flow in batch.flows {
                    let timestamps = EntryTimestamps::default()
                        .with_source_realtime_usec(receive_time_usec)
                        .with_entry_realtime_usec(receive_time_usec);
                    let _ = service.encode_buf.encode_record_and_write(
                        &flow.record,
                        &mut service.raw_journal,
                        timestamps,
                    );
                    service.observe_tiers_record(receive_time_usec, &flow.record);
                }
            }
        }
        let _ = service.flush_closed_tiers(super::now_usec());

        // Phase 4: Benchmark — full pipeline.
        let bench_rounds = 10_000;
        let total_flows = bench_rounds * flows_per_round;

        let start = std::time::Instant::now();
        for _ in 0..bench_rounds {
            for p in &data_payloads {
                let receive_time_usec = super::now_usec();
                let batch =
                    service
                        .decoders
                        .decode_udp_payload_at(p.source, &p.data, receive_time_usec);
                for flow in batch.flows {
                    let timestamps = EntryTimestamps::default()
                        .with_source_realtime_usec(receive_time_usec)
                        .with_entry_realtime_usec(receive_time_usec);
                    let _ = service.encode_buf.encode_record_and_write(
                        &flow.record,
                        &mut service.raw_journal,
                        timestamps,
                    );
                    service.observe_tiers_record(receive_time_usec, &flow.record);
                }
            }
        }
        let elapsed = start.elapsed();

        let flows_per_sec = total_flows as f64 / elapsed.as_secs_f64();
        let usec_per_flow = elapsed.as_micros() as f64 / total_flows as f64;

        eprintln!();
        eprintln!("=== Full Hot Path Benchmark ===");
        eprintln!("Rounds:         {}", bench_rounds);
        eprintln!("Total flows:    {}", total_flows);
        eprintln!("Elapsed:        {:.3}s", elapsed.as_secs_f64());
        eprintln!("Throughput:     {:.0} flows/s", flows_per_sec);
        eprintln!("Latency:        {:.2} µs/flow", usec_per_flow);
        eprintln!();

        // Phase 5: Benchmark — decode only (no journal write, no tier observe).
        let start_decode = std::time::Instant::now();
        let mut decode_flow_count = 0_usize;
        for _ in 0..bench_rounds {
            for p in &data_payloads {
                let receive_time_usec = super::now_usec();
                let batch =
                    service
                        .decoders
                        .decode_udp_payload_at(p.source, &p.data, receive_time_usec);
                decode_flow_count += batch.flows.len();
            }
        }
        let elapsed_decode = start_decode.elapsed();
        let decode_flows_per_sec = decode_flow_count as f64 / elapsed_decode.as_secs_f64();
        let decode_usec_per_flow = elapsed_decode.as_micros() as f64 / decode_flow_count as f64;

        eprintln!("=== Decode Only Benchmark ===");
        eprintln!("Flows:          {}", decode_flow_count);
        eprintln!("Elapsed:        {:.3}s", elapsed_decode.as_secs_f64());
        eprintln!("Throughput:     {:.0} flows/s", decode_flows_per_sec);
        eprintln!("Latency:        {:.2} µs/flow", decode_usec_per_flow);
        eprintln!();

        // Phase 6: Benchmark — encode+write only (pre-decoded flows).
        // Collect a batch of pre-decoded flows.
        let mut prebuilt_flows = Vec::new();
        for p in &data_payloads {
            let batch = service.decoders.decode_udp_payload(p.source, &p.data);
            prebuilt_flows.extend(batch.flows);
        }
        let start_write = std::time::Instant::now();
        for _ in 0..bench_rounds {
            for flow in &prebuilt_flows {
                let timestamps = EntryTimestamps::default()
                    .with_source_realtime_usec(120_000_000)
                    .with_entry_realtime_usec(120_000_000);
                let _ = service.encode_buf.encode_record_and_write(
                    &flow.record,
                    &mut service.raw_journal,
                    timestamps,
                );
            }
        }
        let elapsed_write = start_write.elapsed();
        let write_total = bench_rounds * prebuilt_flows.len();
        let write_flows_per_sec = write_total as f64 / elapsed_write.as_secs_f64();
        let write_usec_per_flow = elapsed_write.as_micros() as f64 / write_total as f64;

        eprintln!("=== Encode+Write Only Benchmark ===");
        eprintln!("Flows:          {}", write_total);
        eprintln!("Elapsed:        {:.3}s", elapsed_write.as_secs_f64());
        eprintln!("Throughput:     {:.0} flows/s", write_flows_per_sec);
        eprintln!("Latency:        {:.2} µs/flow", write_usec_per_flow);
        eprintln!();

        // Phase 7: Benchmark — tier observe only.
        let start_tier = std::time::Instant::now();
        for _ in 0..bench_rounds {
            for flow in &prebuilt_flows {
                service.observe_tiers_record(120_000_000, &flow.record);
            }
        }
        let elapsed_tier = start_tier.elapsed();
        let tier_total = bench_rounds * prebuilt_flows.len();
        let tier_flows_per_sec = tier_total as f64 / elapsed_tier.as_secs_f64();
        let tier_usec_per_flow = elapsed_tier.as_micros() as f64 / tier_total as f64;

        eprintln!("=== Tier Observe Only Benchmark ===");
        eprintln!("Flows:          {}", tier_total);
        eprintln!("Elapsed:        {:.3}s", elapsed_tier.as_secs_f64());
        eprintln!("Throughput:     {:.0} flows/s", tier_flows_per_sec);
        eprintln!("Latency:        {:.2} µs/flow", tier_usec_per_flow);
        eprintln!();

        eprintln!("=== Summary ===");
        eprintln!(
            "Full pipeline:  {:.0} flows/s ({:.2} µs/flow)",
            flows_per_sec, usec_per_flow
        );
        eprintln!(
            "  Decode:       {:.0} flows/s ({:.2} µs/flow)",
            decode_flows_per_sec, decode_usec_per_flow
        );
        eprintln!(
            "  Encode+Write: {:.0} flows/s ({:.2} µs/flow)",
            write_flows_per_sec, write_usec_per_flow
        );
        eprintln!(
            "  Tier observe: {:.0} flows/s ({:.2} µs/flow)",
            tier_flows_per_sec, tier_usec_per_flow
        );
    }
}
