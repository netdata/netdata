//! The `strip-keys-estimate` subcommand — measures the size reduction from
//! stripping the field name prefix from high-cardinality chunk keys.
//!
//! In a high-card chunk for field `foo.bar`, every key is `foo.bar=<value>`.
//! Since the chunk already knows its field, storing just `<value>` (the part
//! after `=`) would eliminate the redundant prefix.

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
    let mut total_stripped = 0usize;

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

                // Strip field name prefix: "field.name=value" → "value"
                let prefix = format!("{}=", field.name);
                let stripped: Vec<(&str, &BitmapValue)> = entries
                    .iter()
                    .map(|(k, bv)| {
                        let val = k.strip_prefix(&prefix).unwrap_or(k);
                        (val, bv)
                    })
                    .collect();

                let stripped_raw = bincode::serde::encode_to_vec(
                    &stripped,
                    bincode::config::standard(),
                )?;
                let stripped_compressed = zstd::encode_all(&stripped_raw[..], 1)?;
                let stripped_size = stripped_compressed.len();

                let saved = original_size.saturating_sub(stripped_size);
                let pct = if original_size > 0 {
                    saved as f64 / original_size as f64 * 100.0
                } else {
                    0.0
                };

                println!(
                    "HC[{chunk_idx}] {:<45} {:>10} → {:>10}  saved {:>10} ({:5.1}%)  entries: {}  prefix: {} bytes",
                    field.name,
                    format_size(original_size),
                    format_size(stripped_size),
                    format_size(saved),
                    pct,
                    entries.len(),
                    prefix.len(),
                );

                total_original += original_size;
                total_stripped += stripped_size;
                chunk_idx += 1;
            }
        }
    }

    println!();
    let saved = total_original.saturating_sub(total_stripped);
    let pct = if total_original > 0 {
        saved as f64 / total_original as f64 * 100.0
    } else {
        0.0
    };
    println!(
        "{:<55} {:>10} → {:>10}  saved {:>10} ({:.1}%)",
        "TOTAL (high-card chunks)",
        format_size(total_original),
        format_size(total_stripped),
        format_size(saved),
        pct,
    );
    let file_pct = if file_size > 0 {
        saved as f64 / file_size as f64 * 100.0
    } else {
        0.0
    };
    println!(
        "{:<55} {:>10} file, saving {:.1}% of file",
        "",
        format_size(file_size),
        file_pct,
    );

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
