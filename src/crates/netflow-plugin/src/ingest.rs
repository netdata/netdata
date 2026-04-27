use crate::decoder::{
    DecapsulationMode as DecoderDecapsulationMode, DecodeStats, DecoderScopeSnapshot,
    DecoderStateNamespaceKey, FlowDecoders, TimestampSource as DecoderTimestampSource,
    normalize_template_scope_source,
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

pub(crate) use encode::JournalEncodeBuffer;

const REBUILD_WINDOW_SECONDS: u32 = 60 * 60;
const REBUILD_TIMEOUT_SECONDS: u64 = 30;
const REBUILD_CACHE_MEMORY_CAPACITY: usize = 16;
const DECODER_STATE_PERSIST_INTERVAL_USEC: u64 = 30 * 1_000_000;
const QUERY_TIME_RANGE_MAX_BUCKET_SECONDS: u32 = 30 * 24 * 60 * 60;

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

fn tier_timestamp_lookup_query_end(now_usec: u64) -> u32 {
    let safe_end = u32::MAX.saturating_sub(QUERY_TIME_RANGE_MAX_BUCKET_SECONDS);
    let now_seconds = now_usec.saturating_div(1_000_000);
    now_seconds
        .saturating_add(1)
        .min(u64::from(safe_end))
        .max(1) as u32
}

#[cfg(test)]
#[path = "ingest_bench_support.rs"]
mod bench_support;
#[cfg(test)]
#[path = "ingest_bench_tests.rs"]
mod bench_tests;
#[cfg(test)]
#[path = "ingest_resource_bench_support.rs"]
mod resource_bench_support;
#[cfg(test)]
#[path = "ingest_resource_bench_tests.rs"]
mod resource_bench_tests;
#[cfg(test)]
#[path = "ingest_test_support.rs"]
mod test_support;
#[cfg(test)]
#[path = "ingest_tests.rs"]
mod tests;
