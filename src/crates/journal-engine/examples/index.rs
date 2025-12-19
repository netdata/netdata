//! Simple test binary for batch_compute_file_indexes.
//!
//! Points at a directory of journal files and indexes them.

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

use journal_engine::{
    Facets, FileIndexCacheBuilder, FileIndexKey, Timeout, batch_compute_file_indexes,
};
use journal_index::FieldName;
use journal_registry::{Monitor, Registry};
use std::env;
use std::path::PathBuf;
use std::time::Duration;

#[allow(unused_imports)]
use tracing::{info, warn};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_max_level(tracing::Level::DEBUG)
        .init();

    // Get directory from args or use default
    let dir = if let Some(arg) = env::args().nth(1) {
        PathBuf::from(arg)
    } else {
        // PathBuf::from("/mnt/slow-disk/otel-aws")
        PathBuf::from("/home/vk/repos/tmp/otel-aws")
    };

    info!("scanning directory: {}", dir.display());

    // Create registry and scan directory
    let (monitor, _event_receiver) = Monitor::new()?;
    let registry = Registry::new(monitor);

    registry.watch_directory(dir.to_str().unwrap())?;

    // Find all files
    let files = registry.find_files_in_range(
        journal_common::Seconds(0),
        journal_common::Seconds(u32::MAX),
    )?;

    info!("found {} journal files", files.len());
    if files.is_empty() {
        return Ok(());
    }
    // files.truncate(1);

    // Create file index cache
    let cache = FileIndexCacheBuilder::new()
        // .with_cache_path("/mnt/slow-disk/foyer-cache")
        .with_cache_path("/tmp/foyer-cache")
        .with_memory_capacity(1000)
        .with_disk_capacity(2048 * 1024 * 1024)
        .with_block_size(4 * 1024 * 1024)
        .build()
        .await?;

    info!("created file index cache");

    // Configure indexing parameters (modify these as needed)
    let facets = Facets::new(&["log.severity_number".to_string()]);

    let keys: Vec<FileIndexKey> = files
        .iter()
        .map(|file_info| FileIndexKey::new(&file_info.file, &facets))
        .collect();

    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let bucket_duration = journal_common::Seconds(60);
    let timeout = Timeout::new(Duration::from_secs(60));

    info!(
        "computing {} file indexes with timeout {:?}",
        keys.len(),
        timeout.remaining()
    );

    // Run batch indexing
    let start = std::time::Instant::now();
    let responses = batch_compute_file_indexes(
        &cache,
        &registry,
        keys,
        source_timestamp_field,
        bucket_duration,
        timeout,
    )
    .await?;

    let elapsed = start.elapsed();

    info!("responses={}, duration={:?}", responses.len(), elapsed);

    // Close the cache to flush and shut down I/O tasks gracefully
    cache.close().await?;

    Ok(())
}
