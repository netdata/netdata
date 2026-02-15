//! The `read` subcommand — opens an .sfst index and prints field cardinalities.

use std::mem;
use std::path::PathBuf;
use std::time::Instant;

use log_index::fst_builder::FieldTier;
use log_index::reader::IndexReader;

pub fn run(path: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
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

    let t = Instant::now();
    let mut fields = reader.field_table()?;
    println!(
        "field table: {} fields ({:.0}ms)",
        fields.len(),
        t.elapsed().as_secs_f64() * 1000.0,
    );

    fields.sort_by(|a, b| b.cardinality.cmp(&a.cardinality));

    let max_name_len = fields.iter().map(|f| f.name.len()).max().unwrap_or(0);

    for f in &fields {
        let tier = match f.tier {
            FieldTier::Low => "low",
            FieldTier::Mid => "mid",
            FieldTier::High => "high",
        };
        println!("  {:<max_name_len$}  {:>6}  {tier}", f.name, f.cardinality);
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

    // In-memory size: two Vec<u32>.
    let mem_bytes = buckets * mem::size_of::<u32>() * 2;

    // On-disk size: re-encode with bincode to measure serialized size.
    let disk_bytes = bincode::serde::encode_to_vec(h, bincode::config::standard())
        .map(|v| v.len())
        .unwrap_or(0);

    println!(
        "histogram: {} buckets, {} to {} ({}s), {} logs, {:.0} logs/s avg, \
         {} bytes on disk, {} bytes in memory",
        buckets,
        fmt(start_sec),
        fmt(last_sec),
        span,
        total,
        total as f64 / span as f64,
        disk_bytes,
        mem_bytes,
    );
}
