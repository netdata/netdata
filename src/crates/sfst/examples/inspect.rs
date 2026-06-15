//! Inspect an SFST index file three ways: summary, log dump, and section sizes.
//!
//! Run with:
//!
//! ```text
//! cargo run --example inspect -- summary  path/to/file.sfst
//! cargo run --example inspect -- dump     path/to/file.sfst [--limit N]
//! cargo run --example inspect -- sections path/to/file.sfst
//! ```
//!
//! Together the three subcommands exercise most of sfst's public reader API:
//! [`IndexReader::open`], [`IndexReader::field_table`], [`IndexReader::histogram`],
//! [`IndexReader::build_string_table`], [`IndexReader::load_all_stream_entries`],
//! [`IndexReader::load_timestamps`], plus the lower-level raw-chunk accessors
//! on [`sfst::Reader`].

use std::mem;
use std::path::PathBuf;
use std::time::Instant;

use clap::{Parser, Subcommand};
use sfst::{FieldTier, IndexReader};

#[derive(Parser)]
#[command(name = "inspect", about = "Inspect an SFST index file")]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    /// Print field cardinalities and a histogram summary.
    Summary { file: PathBuf },
    /// Reconstruct log entries from the on-disk chunks.
    Dump {
        file: PathBuf,
        /// Max log entries to print (default: all).
        #[arg(short = 'n', long)]
        limit: Option<u32>,
    },
    /// Print per-chunk byte sizes and totals.
    Sections { file: PathBuf },
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let cli = Cli::parse();
    match cli.command {
        Command::Summary { file } => summary(&file),
        Command::Dump { file, limit } => dump(&file, limit),
        Command::Sections { file } => sections(&file),
    }
}

// ── summary ─────────────────────────────────────────────────────────

fn summary(path: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    let t = Instant::now();
    let data = std::fs::read(path)?;
    let reader = IndexReader::open(&data)?;
    println!(
        "{} ({} logs, {:.0}ms)",
        path.display(),
        reader.total_logs(),
        t.elapsed().as_secs_f64() * 1000.0,
    );

    print_histogram(&reader);

    let mut fields = reader.field_table().to_vec();
    fields.sort_by_key(|f| std::cmp::Reverse(f.cardinality));

    let max_name_len = fields.iter().map(|f| f.name.len()).max().unwrap_or(0);
    for f in &fields {
        println!(
            "  {:<max_name_len$}  {:>6}  {}",
            f.name,
            f.cardinality,
            tier_label(f.tier),
        );
    }
    println!("{} fields total", fields.len());

    Ok(())
}

fn print_histogram(reader: &IndexReader) {
    let h = reader.histogram();
    if h.timestamps.is_empty() {
        println!("histogram: empty");
        return;
    }

    let buckets = h.timestamps.len();
    let total = *h.counts.last().unwrap();
    let start_sec = h.timestamps[0];
    let last_sec = *h.timestamps.last().unwrap();
    let span = last_sec.saturating_sub(start_sec) + 1;

    let fmt = |sec: u32| {
        chrono::DateTime::from_timestamp(sec as i64, 0)
            .map(|dt| dt.format("%Y-%m-%d %H:%M:%S").to_string())
            .unwrap_or_else(|| sec.to_string())
    };

    let mem_bytes = buckets * mem::size_of::<u32>() * 2;
    let disk_bytes = bincode::serde::encode_to_vec(h, bincode::config::standard())
        .map(|v| v.len())
        .unwrap_or(0);

    println!(
        "histogram: {} buckets, {} to {} ({}s), {} logs, {:.0} logs/s avg, \
         {} on disk, {} in memory",
        buckets,
        fmt(start_sec),
        fmt(last_sec),
        span,
        total,
        total as f64 / span as f64,
        format_size(disk_bytes),
        format_size(mem_bytes),
    );
}

// ── dump ────────────────────────────────────────────────────────────

fn dump(path: &PathBuf, limit: Option<u32>) -> Result<(), Box<dyn std::error::Error>> {
    let data = std::fs::read(path)?;
    let reader = IndexReader::open(&data)?;

    let fields = reader.field_table();
    let string_table = reader.build_string_table(fields)?;
    eprintln!("string table: {} entries", string_table.len());

    let stream = reader.stream();
    let entries = reader.load_all_stream_entries()?;
    let timestamps = reader.load_timestamps()?;
    eprintln!(
        "stream {}/{}: {} entries across {} batch(es)",
        stream.namespace,
        stream.name,
        entries.len(),
        reader.num_stream_batches(),
    );

    if timestamps.len() != entries.len() {
        eprintln!(
            "warning: timestamps ({}) and stream-entries ({}) lengths differ",
            timestamps.len(),
            entries.len(),
        );
    }

    let mut total_printed = 0u32;
    for (pos, kv_ids) in entries.iter().enumerate() {
        if let Some(max) = limit {
            if total_printed >= max {
                break;
            }
        }

        let ts = timestamps.at(pos as u32).unwrap_or(0);
        println!("--- log {total_printed} (pos {pos}, t={ts}ns)");
        for id in kv_ids {
            let idx = id.0 as usize;
            if idx < string_table.len() {
                println!("  {}", string_table[idx]);
            } else {
                println!("  <unknown KvId({})>", id.0);
            }
        }
        total_printed += 1;
    }
    eprintln!("{total_printed} log entries");

    Ok(())
}

// ── sections ────────────────────────────────────────────────────────

fn sections(path: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    let data = std::fs::read(path)?;
    let file_size = data.len();
    let reader = IndexReader::open(&data)?;
    let fields = reader.field_table();
    let sfst = sfst::Reader::open(&data)?;

    let mut total_sections = 0usize;

    if let Ok(raw) = sfst.metadata_raw() {
        print_section("META", raw.len(), file_size);
        total_sections += raw.len();
        let mut sorted = fields.to_vec();
        sorted.sort_by_key(|f| f.cardinality);
        let max_name = sorted.iter().map(|f| f.name.len()).max().unwrap_or(0);
        for f in &sorted {
            println!(
                "  {:<max_name$}  {:>6}  {}",
                f.name,
                f.cardinality,
                tier_label(f.tier),
            );
        }
    }

    let mut prim_size = 0usize;
    if let Ok(raw) = sfst.primary_raw() {
        prim_size = raw.len();
        print_section("PRIM", prim_size, file_size);
        total_sections += prim_size;
    }

    let mut mid_total = 0usize;
    let mut high_total = 0usize;
    let mut mid_idx = 0u16;
    let mut high_idx = 0u16;
    for field in fields.iter() {
        match field.tier {
            FieldTier::Low => continue,
            FieldTier::Mid => {
                if let Ok(raw) = sfst.mid_field_raw(mid_idx) {
                    print_section(
                        &format!("MF[{mid_idx}] mid: {}", field.name),
                        raw.len(),
                        file_size,
                    );
                    mid_total += raw.len();
                }
                mid_idx += 1;
            }
            FieldTier::High => {
                if let Ok(raw) = sfst.high_field_raw(high_idx) {
                    print_section(
                        &format!("HF[{high_idx}] high: {}", field.name),
                        raw.len(),
                        file_size,
                    );
                    high_total += raw.len();
                }
                high_idx += 1;
            }
        }
    }

    let stream = reader.stream();
    let mut stream_total = 0usize;
    for b in 0..reader.num_stream_batches() {
        if let Ok(raw) = sfst.stream_batch_raw(b) {
            print_section(
                &format!("SB{b:02} {}/{}", stream.namespace, stream.name),
                raw.len(),
                file_size,
            );
            stream_total += raw.len();
        }
    }

    total_sections += mid_total + high_total + stream_total;

    println!();
    print_section("low tier (PRIM)", prim_size, file_size);
    print_section("mid tier chunks", mid_total, file_size);
    print_section("high tier chunks", high_total, file_size);
    println!();
    print_section("field chunks total", mid_total + high_total, file_size);
    print_section("stream chunks total", stream_total, file_size);
    print_section("sections total", total_sections, file_size);
    println!("{:<40} {:>10}", "file size", format_size(file_size));
    let overhead = file_size.saturating_sub(total_sections);
    print_section("overhead (header + TOC)", overhead, file_size);

    Ok(())
}

// ── helpers ─────────────────────────────────────────────────────────

fn tier_label(tier: FieldTier) -> &'static str {
    match tier {
        FieldTier::Low => "low",
        FieldTier::Mid => "mid",
        FieldTier::High => "high",
    }
}

fn print_section(name: &str, size: usize, total: usize) {
    let pct = if total > 0 {
        size as f64 / total as f64 * 100.0
    } else {
        0.0
    };
    println!("{:<40} {:>10}  ({:5.1}%)", name, format_size(size), pct);
}

fn format_size(bytes: usize) -> String {
    if bytes >= 1024 * 1024 {
        format!("{:.1} MiB", bytes as f64 / (1024.0 * 1024.0))
    } else if bytes >= 1024 {
        format!("{:.1} KiB", bytes as f64 / 1024.0)
    } else {
        format!("{bytes} B")
    }
}
