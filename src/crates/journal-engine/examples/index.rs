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

use journal_engine::{Facets, FileIndexKey, IndexingEngineBuilder, batch_compute_file_indexes};
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
        PathBuf::from("/mnt/slow-disk/otel-aws")
    };

    info!("scanning directory: {}", dir.display());

    // Create registry and scan directory
    let (monitor, _event_receiver) = Monitor::new()?;
    let registry = Registry::new(monitor);

    registry.watch_directory(dir.to_str().unwrap())?;

    // Find all files
    let mut files = registry.find_files_in_range(
        journal_common::Seconds(0),
        journal_common::Seconds(u32::MAX),
    )?;

    info!("found {} journal files", files.len());
    if files.is_empty() {
        return Ok(());
    }
    files.truncate(1);

    // Create indexing engine with cache
    let indexing_engine = IndexingEngineBuilder::new()
        .with_cache_path("/mnt/slow-disk/foyer-cache")
        .with_memory_capacity(128)
        .with_disk_capacity(16 * 1024 * 1024)
        .build()
        .await?;

    info!("created indexing engine");

    // Configure indexing parameters (modify these as needed)
    let facets = Facets::new(&["PRIORITY".to_string(), "_HOSTNAME".to_string()]);

    let keys: Vec<FileIndexKey> = files
        .iter()
        .map(|file_info| FileIndexKey::new(&file_info.file, &facets))
        .collect();

    let source_timestamp_field = FieldName::new("_SOURCE_REALTIME_TIMESTAMP").unwrap();
    let bucket_duration = journal_common::Seconds(60);
    let time_budget = Duration::from_secs(2);

    info!(
        "computing {} file indexes with time budget {} seconds",
        keys.len(),
        time_budget.as_secs()
    );

    // Run batch indexing
    let start = std::time::Instant::now();
    let responses = batch_compute_file_indexes(
        &indexing_engine,
        &registry,
        keys,
        source_timestamp_field,
        bucket_duration,
        time_budget,
    )
    .await?;

    let elapsed = start.elapsed();

    // Print results
    let mut success_count = 0;
    let mut error_count = 0;

    for (i, response) in responses.iter().enumerate() {
        let _file_info = &files[i];
        match &response.result {
            Ok(_) => {
                success_count += 1;
            }
            Err(_e) => {
                error_count += 1;
            }
        }
    }

    info!(
        "responses={}, computed={}, errors={}, duration={:?}",
        responses.len(),
        success_count,
        error_count,
        elapsed
    );

    // Close the indexing engine to flush cache and shut down I/O tasks gracefully
    indexing_engine.close().await?;

    Ok(())
}
