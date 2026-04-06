//! The `batch-split-estimate` subcommand — measures the overhead of splitting
//! stream entries into 10K-entry batches for the batch-ref scheme.
//!
//! Currently stream entries are one big zstd blob. Splitting into batches
//! means multiple smaller blobs, each compressed independently, which may
//! compress worse. This tool measures that cost.

use std::path::PathBuf;

use log_index::fst_builder::{FieldTier, FileId};
use log_index::reader::IndexReader;

const BATCH_SIZE: usize = 10_000;

pub fn run(path: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    let data = std::fs::read(path)?;
    let file_size = data.len();
    let reader = IndexReader::open(&data)?;
    let fields = reader.field_table()?;
    let sfst = split_fst::Reader::open(&data)?;
    let streams = reader.streams();

    let num_field_chunks = fields.iter().filter(|f| f.tier != FieldTier::Low).count();

    let mut total_original = 0usize;
    let mut total_batched = 0usize;

    for (si, stream) in streams.iter().enumerate() {
        let chunk_idx = (num_field_chunks + si) as u16;
        let compressed_raw = sfst.chunk_raw(chunk_idx)?;
        let original_size = compressed_raw.len();

        // Decode the original entries.
        let decompressed =
            zstd::decode_all(compressed_raw).map_err(|e| format!("zstd: {e}"))?;
        let (entries, _): (Vec<Vec<FileId>>, _) =
            bincode::serde::decode_from_slice(&decompressed, bincode::config::standard())?;

        // Split into batches and compress each independently.
        let mut batched_total = 0usize;
        let num_batches = (entries.len() + BATCH_SIZE - 1) / BATCH_SIZE;
        for batch in entries.chunks(BATCH_SIZE) {
            let raw = bincode::serde::encode_to_vec(batch, bincode::config::standard())?;
            let packed = zstd::encode_all(&raw[..], 1)?;
            batched_total += packed.len();
        }

        let namespace = if stream.namespace.is_empty() {
            "<none>"
        } else {
            &stream.namespace
        };
        let name = if stream.name.is_empty() {
            "<none>"
        } else {
            &stream.name
        };

        let overhead = batched_total.saturating_sub(original_size);
        let pct = if original_size > 0 {
            overhead as f64 / original_size as f64 * 100.0
        } else {
            0.0
        };

        println!(
            "STREAM[{si}] {namespace}/{name}  ({} logs, {} batches)",
            entries.len(),
            num_batches,
        );
        println!(
            "  single blob: {:>10}",
            format_size(original_size),
        );
        println!(
            "  {} batches:  {:>10}  overhead: {:>10} (+{:.1}%)",
            num_batches,
            format_size(batched_total),
            format_size(overhead),
            pct,
        );
        println!();

        total_original += original_size;
        total_batched += batched_total;
    }

    let overhead = total_batched.saturating_sub(total_original);
    let pct = overhead as f64 / total_original.max(1) as f64 * 100.0;
    println!("TOTAL");
    println!("  single blob: {:>10}", format_size(total_original));
    println!(
        "  batched:     {:>10}  overhead: {:>10} (+{:.1}%)",
        format_size(total_batched),
        format_size(overhead),
        pct,
    );
    println!("  file:        {:>10}", format_size(file_size));

    Ok(())
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
