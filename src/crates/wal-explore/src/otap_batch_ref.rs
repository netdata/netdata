//! The `batch-ref-estimate` subcommand — measures the size reduction from
//! replacing high-card bitmaps with a batch-reference bitmask.
//!
//! Instead of a full treight bitmap per high-card value, store a small
//! bitmask indicating which 10K-entry batch(es) the value appears in.
//! On query, scan only the matching batch(es) in the stream entries section.

use std::path::PathBuf;

use log_index::fst_builder::{BitmapValue, FieldTier};
use log_index::reader::IndexReader;
use serde::{Deserialize, Serialize};

const BATCH_SIZE: u32 = 12_500;

/// Replacement for BitmapValue: the batch index (0..N) this value appears in,
/// or 0xFF if it spans multiple batches (fall back to scanning all).
#[derive(Debug, Clone, Serialize, Deserialize)]
struct BatchRef(u8);

pub fn run(path: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    let data = std::fs::read(path)?;
    let file_size = data.len();
    let reader = IndexReader::open(&data)?;
    let fields = reader.field_table()?;
    let sfst = split_fst::Reader::open(&data)?;
    let universe_size = reader.total_logs();
    let num_batches = (universe_size + BATCH_SIZE - 1) / BATCH_SIZE;

    println!(
        "universe: {} logs, batch_size: {}, batches: {}",
        universe_size, BATCH_SIZE, num_batches,
    );
    if num_batches > 8 {
        println!(
            "WARNING: {} batches exceeds u8 bitmask capacity (8), results may be inaccurate",
            num_batches,
        );
    }
    println!();

    let mut total_original = 0usize;
    let mut total_new = 0usize;
    let mut total_entries = 0usize;
    let mut total_single_batch = 0usize;

    let mut chunk_idx = 0u16;
    for field in &fields {
        match field.tier {
            FieldTier::Low | FieldTier::Mid => {
                if field.tier != FieldTier::Low {
                    chunk_idx += 1;
                }
                continue;
            }
            FieldTier::High => {
                let raw = sfst.chunk_raw(chunk_idx)?;
                let original_size = raw.len();

                let decompressed =
                    zstd::decode_all(raw).map_err(|e| format!("zstd: {e}"))?;
                let (entries, _): (Vec<(String, BitmapValue)>, _) =
                    bincode::serde::decode_from_slice(
                        &decompressed,
                        bincode::config::standard(),
                    )?;

                let prefix = format!("{}=", field.name);
                let mut single_batch_count = 0usize;

                let converted: Vec<(&str, BatchRef)> = entries
                    .iter()
                    .map(|(k, bv)| {
                        let val = k.strip_prefix(&prefix).unwrap_or(k);
                        let mut first_batch = None;
                        let mut multi = false;
                        for pos in bv.desc.iter(&bv.data) {
                            let batch = (pos / BATCH_SIZE) as u8;
                            match first_batch {
                                None => first_batch = Some(batch),
                                Some(b) if b != batch => { multi = true; break; }
                                _ => {}
                            }
                        }
                        let batch_id = if multi {
                            0xFF
                        } else {
                            single_batch_count += 1;
                            first_batch.unwrap_or(0)
                        };
                        (val, BatchRef(batch_id))
                    })
                    .collect();

                let new_raw = bincode::serde::encode_to_vec(
                    &converted,
                    bincode::config::standard(),
                )?;
                let new_compressed = zstd::encode_all(&new_raw[..], 1)?;
                let new_size = new_compressed.len();

                let saved = original_size.saturating_sub(new_size);
                let pct = if original_size > 0 {
                    saved as f64 / original_size as f64 * 100.0
                } else {
                    0.0
                };

                let single_pct = if !entries.is_empty() {
                    single_batch_count as f64 / entries.len() as f64 * 100.0
                } else {
                    0.0
                };

                if new_size <= original_size {
                    println!(
                        "HC[{chunk_idx}] {:<45} {:>10} → {:>10}  saved {:>10} ({:5.1}%)  entries: {}  single-batch: {:.0}%",
                        field.name,
                        format_size(original_size),
                        format_size(new_size),
                        format_size(saved),
                        pct,
                        entries.len(),
                        single_pct,
                    );
                } else {
                    let overhead = new_size - original_size;
                    println!(
                        "HC[{chunk_idx}] {:<45} {:>10} → {:>10}  LARGER {:>9} (+{:.1}%)  entries: {}  single-batch: {:.0}%",
                        field.name,
                        format_size(original_size),
                        format_size(new_size),
                        format_size(overhead),
                        overhead as f64 / original_size.max(1) as f64 * 100.0,
                        entries.len(),
                        single_pct,
                    );
                }

                total_original += original_size;
                total_new += new_size;
                total_entries += entries.len();
                total_single_batch += single_batch_count;
                chunk_idx += 1;
            }
        }
    }

    println!();
    let single_pct = total_single_batch as f64 / total_entries.max(1) as f64 * 100.0;
    println!(
        "single-batch values: {} / {} ({:.1}%)",
        total_single_batch, total_entries, single_pct,
    );

    if total_new <= total_original {
        let saved = total_original - total_new;
        let pct = saved as f64 / total_original.max(1) as f64 * 100.0;
        println!(
            "{:<55} {:>10} → {:>10}  saved {:>10} ({:.1}%)",
            "TOTAL (high-card chunks)",
            format_size(total_original),
            format_size(total_new),
            format_size(saved),
            pct,
        );
        let file_pct = saved as f64 / file_size.max(1) as f64 * 100.0;
        println!(
            "{:<55} {:>10} file, saving {:.1}% of file",
            "",
            format_size(file_size),
            file_pct,
        );
    } else {
        let overhead = total_new - total_original;
        println!(
            "{:<55} {:>10} → {:>10}  LARGER {:>9} (+{:.1}%)",
            "TOTAL (high-card chunks)",
            format_size(total_original),
            format_size(total_new),
            format_size(overhead),
            overhead as f64 / total_original.max(1) as f64 * 100.0,
        );
    }

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
