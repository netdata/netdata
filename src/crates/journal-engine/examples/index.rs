//! Example: scan a journal directory via the registry, build a foyer index
//! cache, and warm it with [`batch_compute_file_indexes`]
//! for the last 24h. Run from src/crates: cargo run -p journal-engine --example index [dir]

// The default directory (/mnt/slow-disk/otel-aws) is a deliberately slow mount,
// prepared by the steps below, to reproduce batch indexing under slow I/O.
//
// # 1. Create a loopback ext4 mount
// dd if=/dev/zero of=/tmp/slow-disk.img bs=1G count=100
// LOOP=$(sudo losetup -f --show /tmp/slow-disk.img)
// sudo mkfs.ext4 $LOOP
// sudo mkdir -p /mnt/slow-disk && sudo mount $LOOP /mnt/slow-disk
// sudo chown $USER:$USER /mnt/slow-disk
//
// # 2. Copy journal files under it (its name becomes the scan target)
// cp -r /path/to/journals /mnt/slow-disk/
//
// # 3. Recreate the mount through device-mapper with 50ms read+write delays
// sudo umount /mnt/slow-disk
// SIZE=$(sudo blockdev --getsz $LOOP)
// sudo dmsetup create slow-disk --table "0 $SIZE delay $LOOP 0 50 $LOOP 0 50"
// sudo mount /dev/mapper/slow-disk /mnt/slow-disk
//
// # 4. /mnt/slow-disk/otel-aws now sits on the "slow" disk
//
// # 5. Optionally cap throughput with a cgroup v2 io controller
// sudo mkdir -p /sys/fs/cgroup/slow-io
// echo "+io" | sudo tee /sys/fs/cgroup/cgroup.controllers
// cat /sys/block/nvme0n1/dev   # prints major:minor, e.g. 259:0
// echo "259:0 rbps=10485760 wbps=10485760" | sudo tee /sys/fs/cgroup/slow-io/io.max
// echo $$ | sudo tee /sys/fs/cgroup/slow-io/cgroup.procs   # throttle this shell and its children

use journal_engine::{
    Facets, FileIndexCacheBuilder, FileIndexKey, IndexingLimits, QueryTimeRange,
    batch_compute_file_indexes,
};
use journal_index::FieldName;
use journal_registry::{Monitor, Registry};
use std::env;
use std::path::PathBuf;
use tokio_util::sync::CancellationToken;

#[allow(unused_imports)]
use tracing::{info, warn};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Debug-level tracing via tracing_subscriber::fmt().
    tracing_subscriber::fmt()
        .with_max_level(tracing::Level::DEBUG)
        .init();

    // Scan target: argv[1], else the default slow-disk directory prepared above.
    let dir = if let Some(arg) = env::args().nth(1) {
        PathBuf::from(arg)
    } else {
        PathBuf::from("/mnt/slow-disk/otel-aws")
    };

    info!("scanning directory: {}", dir.display());

    // Registry + notify monitor (`journal-registry/src/registry/monitor.rs` `Monitor`): watch_directory scans recursively; events stay unread.
    let (monitor, _event_receiver) = Monitor::new()?;
    let registry = Registry::new(monitor);

    registry.watch_directory(dir.to_str().unwrap())?;

    // Find all scanned files: find_files_in_range over the full 0..u32::MAX range (`journal-registry/src/registry/mod.rs` `Registry::find_files_in_range`).
    let files = registry.find_files_in_range(
        journal_common::Seconds(0),
        journal_common::Seconds(u32::MAX),
    )?;

    info!("found {} journal files", files.len());
    if files.is_empty() {
        return Ok(());
    }
    // Debug toggle: keep only the first file (uncomment).

    // Foyer hybrid cache (`indexing.rs` `FileIndexCacheBuilder`): 1000 in-memory entries + 2 GiB disk cache; the disk path persists across runs.
    let cache = FileIndexCacheBuilder::new()
        // Alternative: keep the disk cache on the slow disk too (swap with the line below).
        .with_cache_path("/tmp/foyer-cache")
        .with_memory_capacity(1000)
        .with_disk_capacity(2048 * 1024 * 1024)
        .with_block_size(4 * 1024 * 1024)
        .build()
        .await?;

    info!("created file index cache");

    // Indexing inputs: which fields become facets (`facets.rs` `Facets::new`) and the source timestamp field.
    let facets = Facets::new(&["log.severity_number".to_string()]);
    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();

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

    // Index only the last 24h; QueryTimeRange derives the aligned bucket duration (`query_time_range.rs` `QueryTimeRange::new`).
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)?
        .as_secs() as u32;
    let time_range = QueryTimeRange::new(now - 86400, now)?;
    let cancellation = CancellationToken::new();

    info!(
        "computing {} file indexes, bucket duration: {}s",
        keys.len(),
        time_range.bucket_duration()
    );

    // Cache check, then parallel compute of misses on a rayon pool; results update the registry and cache.
    let start = std::time::Instant::now();
    let responses = batch_compute_file_indexes(
        &cache,
        &registry,
        keys,
        &time_range,
        cancellation,
        IndexingLimits::default(),
        None,
    )
    .await?;

    let elapsed = start.elapsed();

    info!("responses={}, duration={:?}", responses.len(), elapsed);

    // Flush and shut down the cache's I/O tasks.
    cache.close().await?;

    Ok(())
}
