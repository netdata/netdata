//! Benchmark for batch_compute_file_indexes.
//!
//! Points at a directory of journal files, indexes them, and prints bitmap
//! statistics.
//!
//! Usage:
//!     cargo run --release -p journal-engine --example index -- /var/log/journal

// # 1. Create a mount point
//
// dd if=/dev/zero of=/tmp/slow-disk.img bs=1G count=100
// LOOP=$(sudo losetup -f --show /tmp/slow-disk.img)
// sudo mkfs.ext4 $LOOP
// sudo mkdir -p /mnt/slow-disk
// sudo mount $LOOP /mnt/slow-disk
// sudo chown $USER:$USER /mnt/slow-disk
//
// # 2. Copy journal files
// cp -r ~/repos/tmp/otel-aws /mnt/slow-disk/
//
// # 3. Unmount and recreate with delay
// sudo umount /mnt/slow-disk
// SIZE=$(sudo blockdev --getsz $LOOP)
// sudo dmsetup create slow-disk --table "0 $SIZE delay $LOOP 0 50 $LOOP 0 50"
// sudo mount /dev/mapper/slow-disk /mnt/slow-disk
//
// # 4. Now /mnt/slow-disk/otel-aws has your journals on a "slow" disk
//
// # 5. Create slow-io cgroup
// sudo mkdir -p /sys/fs/cgroup/slow-io
// echo "+io" | sudo tee /sys/fs/cgroup/cgroup.controllers
// # Find your device's major:minor (e.g., for nvme0n1)
// cat /sys/block/nvme0n1/dev
// # Let's say it's 259:0, Set a 10MB/s read and write limit
// echo "259:0 rbps=10485760 wbps=10485760" | sudo tee /sys/fs/cgroup/slow-io/io.max

#[global_allocator]
static GLOBAL: tikv_jemallocator::Jemalloc = tikv_jemallocator::Jemalloc;

use clap::Parser;
use journal_engine::{
    FileIndexCacheBuilder, FileIndexKey, IndexingLimits, QueryTimeRange, batch_compute_file_indexes,
};
use journal_index::{FieldName, FileIndex};
use journal_registry::{Monitor, Registry};
use rand::SeedableRng;
use rand::seq::SliceRandom;
use std::{path::PathBuf, time::Duration};
use tokio_util::sync::CancellationToken;

#[allow(unused_imports)]
use tracing::{info, warn};

#[derive(Parser)]
#[command(about = "Benchmark batch_compute_file_indexes with bitmap/FST statistics")]
struct Args {
    /// Path to the journal directory.
    #[arg(default_value = "/mnt/slow-disk/otel-aws")]
    dir: PathBuf,

    /// Maximum number of journal files to index after discovery.
    #[arg(long)]
    max_files: Option<usize>,

    /// Maximum unique values per field (cardinality limit).
    #[arg(long, default_value_t = 100)]
    cardinality: usize,

    /// Select N random facets from ~/Desktop/response.json columns.
    /// When not set, uses the built-in DEFAULT_FACETS list.
    #[arg(long)]
    random_facets: Option<usize>,
}

// ---------------------------------------------------------------------------
// Default facets (extracted from facets.json)
// ---------------------------------------------------------------------------

const DEFAULT_FACETS: &[&str] = &[
    "_HOSTNAME",
    "PRIORITY",
    "SYSLOG_FACILITY",
    "ERRNO",
    "SYSLOG_IDENTIFIER",
    "UNIT",
    "USER_UNIT",
    "MESSAGE_ID",
    "_BOOT_ID",
    "_SYSTEMD_OWNER_UID",
    "_UID",
    "OBJECT_SYSTEMD_OWNER_UID",
    "OBJECT_UID",
    "_GID",
    "OBJECT_GID",
    "_CAP_EFFECTIVE",
    "_AUDIT_LOGINUID",
    "OBJECT_AUDIT_LOGINUID",
    "_RUNTIME_SCOPE",
    "_SELINUX_CONTEXT",
    "_EXE",
    "_NAMESPACE",
    "_SYSTEMD_SLICE",
    "_SYSTEMD_CGROUP",
    "_COMM",
    "_TRANSPORT",
    "_MACHINE_ID",
    "_SYSTEMD_UNIT",
    "CODE_FILE",
    "ND_ALERT_TYPE",
    "ND_ALERT_CLASS",
    "ND_NIDL_NODE",
    "ND_ALERT_STATUS",
    "ND_ALERT_COMPONENT",
    "_STREAM_ID",
    "ND_ALERT_NAME",
    "ND_LOG_SOURCE",
    "CODE_FUNC",
    "ND_NIDL_CONTEXT",
    "_SYSTEMD_SESSION",
    "_KERNEL_SUBSYSTEM",
    "UNIT_RESULT",
    "_SYSTEMD_USER_UNIT",
    "_SYSTEMD_USER_SLICE",
    "_UDEV_DEVNODE",
    "__logs_sources",
];

// ---------------------------------------------------------------------------
// Stats collection
// ---------------------------------------------------------------------------

/// Per-file statistics.
#[derive(Debug, Clone, Default)]
#[allow(dead_code)]
struct FileStats {
    /// Path of the journal file.
    path: String,
    /// Total journal entries in this file.
    entries: usize,
    /// Number of field=value bitmaps.
    bitmap_count: usize,
    /// Number of bitmaps using inverted (complement) representation.
    inverted_count: usize,
    /// Size in bytes of the FST index (0 if not built).
    fst_bytes: usize,
    /// Number of keys in the FST index (0 if not built).
    fst_keys: usize,
    /// Allocative size breakdown of each FileIndex field (summed later).
    alloc: AllocBreakdown,
}

/// Allocative heap-size breakdown of a single FileIndex.
#[derive(Debug, Clone, Default)]
struct AllocBreakdown {
    total: u64,
    file: u64,
    histogram: u64,
    entry_offsets: u64,
    file_fields: u64,
    indexed_fields: u64,
    bitmaps: u64,
    fst_bitmaps: u64,
}

/// Aggregated statistics for the full indexing run.
#[derive(Debug, Clone, Default)]
struct RunStats {
    /// Which backend was used.
    backend: String,
    /// Wall-clock time for the batch indexing call.
    index_duration: Duration,
    /// Process memory info after indexing.
    mem: Option<MemInfo>,
    /// Per-file breakdown.
    files: Vec<FileStats>,
}

impl RunStats {
    fn total_files(&self) -> usize {
        self.files.len()
    }

    fn total_entries(&self) -> usize {
        self.files.iter().map(|f| f.entries).sum()
    }

    fn total_bitmaps(&self) -> usize {
        self.files.iter().map(|f| f.bitmap_count).sum()
    }

    fn total_inverted(&self) -> usize {
        self.files.iter().map(|f| f.inverted_count).sum()
    }

    fn total_fst_bytes(&self) -> usize {
        self.files.iter().map(|f| f.fst_bytes).sum()
    }

    fn total_fst_keys(&self) -> usize {
        self.files.iter().map(|f| f.fst_keys).sum()
    }

    fn total_alloc(&self) -> AllocBreakdown {
        let mut t = AllocBreakdown::default();
        for f in &self.files {
            t.total += f.alloc.total;
            t.file += f.alloc.file;
            t.histogram += f.alloc.histogram;
            t.entry_offsets += f.alloc.entry_offsets;
            t.file_fields += f.alloc.file_fields;
            t.indexed_fields += f.alloc.indexed_fields;
            t.bitmaps += f.alloc.bitmaps;
            t.fst_bitmaps += f.alloc.fst_bitmaps;
        }
        t
    }
}

fn backend_name() -> &'static str {
    "treight"
}

fn collect_file_stats(path: &str, file_index: &FileIndex) -> FileStats {
    let mut stats = FileStats {
        path: path.to_string(),
        entries: file_index.total_entries(),
        ..Default::default()
    };

    let fst_idx = file_index.fst_index();
    stats.fst_bytes = fst_idx.fst_bytes();
    stats.fst_keys = fst_idx.len();
    stats.bitmap_count = fst_idx.len();

    #[cfg(feature = "allocative")]
    {
        use allocative::size_of_unique_allocated_data as alloc_size;
        stats.alloc.total = alloc_size(file_index) as u64;
        stats.alloc.file = alloc_size(file_index.file()) as u64;
        stats.alloc.histogram = alloc_size(file_index.histogram()) as u64;
        stats.alloc.entry_offsets = alloc_size(file_index.entry_offsets()) as u64;
        stats.alloc.file_fields = alloc_size(file_index.fields()) as u64;
        stats.alloc.indexed_fields = alloc_size(file_index.indexed_fields()) as u64;
        stats.alloc.fst_bitmaps = fst_idx.values().iter().map(|b| alloc_size(b) as u64).sum();
    }

    for bitmap in fst_idx.values() {
        if bitmap.is_inverted() {
            stats.inverted_count += 1;
        }
    }

    stats
}

/// Load column names from ~/Desktop/response.json and randomly select `n`.
/// Uses `n` as the RNG seed so the same count always picks the same facets.
fn random_facets_from_response_json(n: usize) -> Result<Vec<String>, Box<dyn std::error::Error>> {
    let home = std::env::var("HOME")?;
    let path = format!("{home}/Desktop/response.json");
    let content = std::fs::read_to_string(&path)?;
    let data: serde_json::Value = serde_json::from_str(&content)?;
    let columns = data
        .get("columns")
        .ok_or("missing 'columns' key in response.json")?;

    let mut names: Vec<String> = match columns {
        serde_json::Value::Object(map) => map.keys().cloned().collect(),
        _ => return Err("'columns' must be an object".into()),
    };

    // Filter out non-field keys (timestamp, rowOptions, etc.)
    names.retain(|name| !matches!(name.as_str(), "timestamp" | "rowOptions" | "message"));

    // Sort first so the input order is deterministic (JSON object key order is not guaranteed).
    names.sort();

    let mut rng = rand::rngs::SmallRng::seed_from_u64(n as u64);
    names.shuffle(&mut rng);

    tracing::warn!("Using {}/{} facets", n, names.len());
    names.truncate(n);

    Ok(names)
}

#[derive(Debug, Clone)]
struct MemInfo {
    rss: u64,
    peak_rss: u64,
}

fn mem_info() -> Option<MemInfo> {
    let status = std::fs::read_to_string("/proc/self/status").ok()?;
    let mut rss = None;
    let mut peak_rss = None;
    for line in status.lines() {
        if let Some(value) = line.strip_prefix("VmRSS:") {
            let kb: u64 = value.trim().trim_end_matches(" kB").trim().parse().ok()?;
            rss = Some(kb * 1024);
        } else if let Some(value) = line.strip_prefix("VmHWM:") {
            let kb: u64 = value.trim().trim_end_matches(" kB").trim().parse().ok()?;
            peak_rss = Some(kb * 1024);
        }
    }
    Some(MemInfo {
        rss: rss?,
        peak_rss: peak_rss?,
    })
}

fn fmt_bytes(bytes: u64) -> String {
    if bytes < 1024 {
        format!("{bytes} B")
    } else if bytes < 1024 * 1024 {
        format!("{:.1} KB", bytes as f64 / 1024.0)
    } else {
        format!("{:.1} MB", bytes as f64 / (1024.0 * 1024.0))
    }
}

fn print_stats(stats: &RunStats) {
    let alloc = stats.total_alloc();
    let fst_map = stats.total_fst_bytes() as u64;
    let fst_keys = stats.total_fst_keys();
    let bitmap_storage = fst_map + alloc.fst_bitmaps;
    let true_total = alloc.total + fst_map + alloc.fst_bitmaps;

    let inverted = stats.total_inverted();
    let total_bitmaps = stats.total_bitmaps();

    println!();
    println!("=== Index stats ({} + FST) ===", stats.backend);
    println!("  files:            {}", stats.total_files());
    println!("  entries:          {}", stats.total_entries());
    println!("  key-value pairs:  {total_bitmaps}");
    if inverted > 0 {
        println!(
            "  inverted:         {} ({:.1}%)",
            inverted,
            inverted as f64 / total_bitmaps as f64 * 100.0
        );
    }

    println!("  --- memory (sum across all files) ---");
    println!("  total:            {}", fmt_bytes(true_total));
    println!("    entry_offsets:  {}", fmt_bytes(alloc.entry_offsets));
    println!("    file_fields:    {}", fmt_bytes(alloc.file_fields));
    println!("    indexed_fields: {}", fmt_bytes(alloc.indexed_fields));
    println!("    histogram:      {}", fmt_bytes(alloc.histogram));
    println!("    file:           {}", fmt_bytes(alloc.file));
    println!(
        "    bitmaps:        {} (FST + Vec<Bitmap>)",
        fmt_bytes(bitmap_storage)
    );
    println!(
        "      fst map:      {} ({} keys, {} B/key)",
        fmt_bytes(fst_map),
        fst_keys,
        if fst_keys > 0 {
            fst_map / fst_keys as u64
        } else {
            0
        },
    );
    println!("      bitmap data:  {}", fmt_bytes(alloc.fst_bitmaps));

    println!("  --- timing ---");
    println!("  index time:       {:.2?}", stats.index_duration);
    if let Some(ref mem) = stats.mem {
        println!("  process RSS:      {}", fmt_bytes(mem.rss));
        println!("  peak RSS:         {}", fmt_bytes(mem.peak_rss));
    }
    println!();
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing. Use RUST_LOG to control verbosity, e.g.:
    //   RUST_LOG=debug ./run.sh ...
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("warn")),
        )
        .init();

    let args = Args::parse();

    info!("scanning directory: {}", args.dir.display());
    info!("bitmap backend: {}", backend_name());
    info!("cardinality limit: {}", args.cardinality);

    // Create registry and scan directory
    let (monitor, _event_receiver) = Monitor::new()?;
    let registry = Registry::new(monitor);

    registry.watch_directory(args.dir.to_str().unwrap())?;

    // Find all files
    let mut files = registry.find_files_in_range(
        journal_common::Seconds(0),
        journal_common::Seconds(u32::MAX),
    )?;

    info!("found {} journal files", files.len());
    if files.is_empty() {
        return Ok(());
    }

    if let Some(n) = args.max_files {
        files.truncate(n);
        info!("limited to {} file(s)", files.len());
    }

    // Create file index cache (in-memory LRU)
    let cache = FileIndexCacheBuilder::new()
        .with_memory_capacity(128)
        .build();

    info!("created file index cache");

    // Load facets: random selection from response.json or built-in defaults
    let facet_names = if let Some(n) = args.random_facets {
        let names = random_facets_from_response_json(n)?;
        info!(
            "randomly selected {} facets from ~/Desktop/response.json",
            names.len()
        );
        names
    } else {
        DEFAULT_FACETS.iter().map(|s| s.to_string()).collect()
    };
    info!("indexing {} facets", facet_names.len());
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();

    let keys: Vec<FileIndexKey> = files
        .iter()
        .map(|file_info| FileIndexKey::new(&file_info.file, Some(source_timestamp_field.clone())))
        .collect();

    // Create a time range for indexing (1 year)
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)?
        .as_secs() as u32;
    let time_range = QueryTimeRange::new(now - 3600 * 24 * 365, now)?;
    let cancellation = CancellationToken::new();

    info!(
        "computing {} file indexes, bucket duration: {}s",
        keys.len(),
        time_range.bucket_duration()
    );

    let indexing_limits = IndexingLimits {
        max_unique_values_per_field: args.cardinality,
        ..Default::default()
    };

    // Run batch indexing
    let start = std::time::Instant::now();
    let responses =
        batch_compute_file_indexes(&cache, &registry, keys, cancellation, indexing_limits, None)
            .await?;

    let index_duration = start.elapsed();

    // Collect stats.
    let mut run_stats = RunStats {
        backend: backend_name().to_string(),
        index_duration,
        mem: mem_info(),
        ..Default::default()
    };

    for (key, file_index) in &responses {
        run_stats
            .files
            .push(collect_file_stats(key.file.path(), file_index));
    }

    print_stats(&run_stats);

    drop(responses);
    tracing::warn!("After dropping responses");
    {
        // Collect stats.
        let run_stats = RunStats {
            backend: backend_name().to_string(),
            index_duration,
            mem: mem_info(),
            ..Default::default()
        };

        print_stats(&run_stats);
    }

    Ok(())
}
