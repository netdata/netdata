//! The indexing pipeline — orchestrates Phase 1 (reading) and Phase 2 (writing).
//!
//! Phase 1 reads WAL frames into a [`WalIndex`], then Phase 2
//! ([`log_index::build_and_write`]) transforms the in-memory data into the
//! on-disk split-FST format.

use std::path::PathBuf;
use std::time::Instant;

use bumpalo::Bump;

use log_index::wal_index::WalIndex;

/// Build a split-FST index from a WAL file.
///
/// This is the entry point for the two-phase indexing pipeline:
/// - **Phase 1**: sequential scan of WAL frames via [`crate::process_frame::process_frame`].
/// - **Phase 2**: [`log_index::build_and_write`] transforms the
///   in-memory data into the on-disk split-FST format.
pub fn run(
    path: &PathBuf,
    limit: Option<u32>,
    cardinality_threshold: u32,
) -> Result<(), Box<dyn std::error::Error>> {
    let max_logs = limit.unwrap_or(u32::MAX) as usize;

    let mut reader = wal::WalReader::open(path)?;
    let arena = Bump::with_capacity(32 * 1024 * 1024);
    let mut wal_index = WalIndex::new(&arena, cardinality_threshold);

    let mut num_frames = 0;
    let t_read = Instant::now();

    while let Some(wal_frame) = reader.next_frame()? {
        if wal_index.num_logs() >= max_logs {
            break;
        }

        num_frames += 1;
        crate::process_frame::process_frame(&mut wal_index, &wal_frame)?;
    }

    let read_elapsed = t_read.elapsed();
    println!(
        "{num_frames} frames with {} logs processed in {:.1}s",
        wal_index.num_logs(),
        read_elapsed.as_secs_f64()
    );

    let out_path = path.with_extension("sfst");

    log_index::build_and_write(&wal_index, &out_path)?;
    print_rss();

    Ok(())
}

pub fn print_rss() {
    let status = std::fs::read_to_string("/proc/self/status").unwrap_or_default();
    let mut rss_kb = 0usize;
    let mut peak_kb = 0usize;
    for line in status.lines() {
        if let Some(val) = line.strip_prefix("VmRSS:") {
            rss_kb = parse_kb(val);
        } else if let Some(val) = line.strip_prefix("VmHWM:") {
            peak_kb = parse_kb(val);
        }
    }
    println!("RSS: {} MB (peak: {} MB)", rss_kb / 1024, peak_kb / 1024);
}

fn parse_kb(val: &str) -> usize {
    val.trim()
        .strip_suffix("kB")
        .unwrap_or("0")
        .trim()
        .parse()
        .unwrap_or(0)
}
