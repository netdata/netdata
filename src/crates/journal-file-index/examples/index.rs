//! Build split-FST indexes for a directory of journal files and print stats.
//!
//! Usage:
//!     cargo run --release -p journal-file-index --example index -- ~/opt/sjr-tree8/netdata/var/log/netdata/otel/v1

#[global_allocator]
static GLOBAL: tikv_jemallocator::Jemalloc = tikv_jemallocator::Jemalloc;

use clap::Parser;
use journal_file_index::{FileIndex, FileIndexBuilder, IndexingLimits};
use journal_registry::{Monitor, Registry};
use std::path::PathBuf;
use std::time::Instant;

#[derive(Parser)]
#[command(about = "Build split-FST indexes and print statistics")]
struct Args {
    /// Path to the journal directory.
    #[arg(default_value = "~/opt/sjr-tree8/netdata/var/log/netdata/otel/v1")]
    dir: PathBuf,

    /// Maximum number of journal files to index.
    #[arg(long)]
    max_files: Option<usize>,

    /// Maximum unique values per field (cardinality limit).
    #[arg(long, default_value_t = 1000000)]
    cardinality: usize,
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

fn mem_rss() -> Option<u64> {
    let status = std::fs::read_to_string("/proc/self/status").ok()?;
    for line in status.lines() {
        if let Some(value) = line.strip_prefix("VmRSS:") {
            let kb: u64 = value.trim().trim_end_matches(" kB").trim().parse().ok()?;
            return Some(kb * 1024);
        }
    }
    None
}

fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("warn")),
        )
        .init();

    let args = Args::parse();

    eprintln!("scanning: {}", args.dir.display());
    eprintln!("cardinality limit: {}", args.cardinality);

    // Discover files
    let (monitor, _rx) = Monitor::new().expect("monitor");
    let registry = Registry::new(monitor);
    registry
        .watch_directory(args.dir.to_str().unwrap())
        .expect("watch_directory");

    let mut files = registry
        .find_files_in_range(
            journal_common::Seconds(0),
            journal_common::Seconds(u32::MAX),
        )
        .expect("find_files");

    eprintln!("found {} journal files", files.len());
    if files.is_empty() {
        return;
    }

    if let Some(n) = args.max_files {
        files.truncate(n);
        eprintln!("limited to {} file(s)", files.len());
    }

    let limits = IndexingLimits {
        max_unique_values_per_field: args.cardinality,
        ..Default::default()
    };
    let mut builder = FileIndexBuilder::new(limits);

    let mut total_files = 0usize;
    let mut total_entries = 0usize;
    let mut total_raw_bytes = 0u64;
    let mut total_lc_keys = 0usize;
    let mut total_hc_keys = 0usize;
    let mut total_hc_fields = 0usize;
    let mut total_fields = 0usize;
    let mut errors = 0usize;

    let t0 = Instant::now();

    for file_info in &files {
        let file = &file_info.file;

        match builder.build(file, None) {
            Ok(bytes) => {
                let raw_len = bytes.len() as u64;

                match FileIndex::from_bytes(&bytes) {
                    Ok(index) => {
                        let hc_count = index.high_cardinality_field_names().count();

                        let mut lc_keys = 0;
                        index.for_each_low_cardinality(|_, _| lc_keys += 1);

                        let mut all_keys = 0;
                        index.for_each(|_, _| all_keys += 1);
                        let hc_keys = all_keys - lc_keys;

                        total_files += 1;
                        total_entries += index.total_entries();
                        total_raw_bytes += raw_len;
                        total_lc_keys += lc_keys;
                        total_hc_keys += hc_keys;
                        total_hc_fields += hc_count;
                        total_fields += index.fields().len();
                    }
                    Err(e) => {
                        eprintln!("  read error {}: {e}", file.path());
                        errors += 1;
                    }
                }
            }
            Err(e) => {
                eprintln!("  build error {}: {e}", file.path());
                errors += 1;
            }
        }
    }

    let elapsed = t0.elapsed();

    println!();
    println!("=== Split-FST index stats ===");
    println!("  files indexed:       {total_files}");
    if errors > 0 {
        println!("  errors:              {errors}");
    }
    println!("  total entries:       {total_entries}");
    println!("  total fields:        {total_fields} (across all files)");
    println!("  low-card kv pairs:   {total_lc_keys}");
    println!("  high-card kv pairs:  {total_hc_keys}");
    println!("  high-card fields:    {total_hc_fields} (across all files)");
    println!(
        "  split-fst bytes:     {} (total, compressed)",
        fmt_bytes(total_raw_bytes)
    );
    if total_files > 0 {
        println!(
            "  avg per file:        {}",
            fmt_bytes(total_raw_bytes / total_files as u64)
        );
    }
    println!("  --- timing ---");
    println!("  total time:          {elapsed:.2?}");
    if total_files > 0 {
        let per_file = elapsed / total_files as u32;
        println!("  per file:            {per_file:.2?}");
    }
    if let Some(rss) = mem_rss() {
        println!("  process RSS:         {}", fmt_bytes(rss));
    }
    println!();
}
