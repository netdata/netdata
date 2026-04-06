//! The `no-bitmaps-estimate` subcommand — measures the size reduction from
//! dropping bitmaps entirely in high-cardinality chunks, storing only the
//! values (with field name prefix stripped).

use std::path::PathBuf;

use log_index::fst_builder::{BitmapValue, FieldTier};
use log_index::reader::IndexReader;

pub fn run(path: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    let data = std::fs::read(path)?;
    let file_size = data.len();
    let reader = IndexReader::open(&data)?;
    let fields = reader.field_table()?;
    let sfst = split_fst::Reader::open(&data)?;

    let mut total_original = 0usize;
    let mut total_new = 0usize;

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

                // Strip prefix, keep only values — no bitmaps.
                let prefix = format!("{}=", field.name);
                let values_only: Vec<&str> = entries
                    .iter()
                    .map(|(k, _)| k.strip_prefix(&prefix).unwrap_or(k))
                    .collect();

                let new_raw = bincode::serde::encode_to_vec(
                    &values_only,
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

                if new_size <= original_size {
                    println!(
                        "HC[{chunk_idx}] {:<45} {:>10} → {:>10}  saved {:>10} ({:5.1}%)  entries: {}",
                        field.name,
                        format_size(original_size),
                        format_size(new_size),
                        format_size(saved),
                        pct,
                        entries.len(),
                    );
                } else {
                    let overhead = new_size - original_size;
                    println!(
                        "HC[{chunk_idx}] {:<45} {:>10} → {:>10}  LARGER {:>9} (+{:.1}%)  entries: {}",
                        field.name,
                        format_size(original_size),
                        format_size(new_size),
                        format_size(overhead),
                        overhead as f64 / original_size.max(1) as f64 * 100.0,
                        entries.len(),
                    );
                }

                total_original += original_size;
                total_new += new_size;
                chunk_idx += 1;
            }
        }
    }

    println!();
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
