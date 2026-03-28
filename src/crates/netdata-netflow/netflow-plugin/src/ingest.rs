use crate::decoder::{
    DecapsulationMode as DecoderDecapsulationMode, DecodeStats, DecoderStateNamespaceKey,
    FlowDecoders, TimestampSource as DecoderTimestampSource,
};
use crate::enrichment::FlowEnricher;
use crate::network_sources::NetworkSourcesRuntime;
use crate::plugin_config::{
    DecapsulationMode as ConfigDecapsulationMode, PluginConfig,
    TimestampSource as ConfigTimestampSource,
};
use crate::routing::DynamicRoutingRuntime;
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

use encode::JournalEncodeBuffer;

const REBUILD_WINDOW_SECONDS: u32 = 60 * 60;
const REBUILD_TIMEOUT_SECONDS: u64 = 30;
const REBUILD_CACHE_MEMORY_CAPACITY: usize = 16;
const REBUILD_CACHE_DISK_CAPACITY: usize = 64 * 1024 * 1024;
const REBUILD_CACHE_BLOCK_SIZE: usize = 1024 * 1024;
const DECODER_STATE_PERSIST_INTERVAL_USEC: u64 = 30 * 1_000_000;

mod encode;
mod metrics;
mod persistence;
mod rebuild;
mod service;

pub(crate) use metrics::IngestMetrics;
pub(crate) use service::IngestService;

fn now_usec() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_micros() as u64)
        .unwrap_or(0)
}

#[cfg(test)]
#[path = "ingest_tests.rs"]
mod tests;
