//! The `compact-estimate` subcommand — measures the size reduction from using
//! a compact 4-byte bitmap descriptor instead of the current 6-byte one.
//!
//! Reads an .sfst file, deserializes all BitmapValue entries from every tier,
//! re-serializes with a compact descriptor, re-compresses, and compares sizes.

use std::path::PathBuf;

use log_index::fst_builder::{BitmapValue, FieldTier};
use log_index::reader::IndexReader;
use serde::{Deserialize, Serialize};

/// Compact bitmap descriptor: packs `universe_size` (31 bits) and `inverted`
/// (1 bit) into a single `u32`. `levels` is derived from `universe_size`.
#[derive(Debug, Clone, Serialize, Deserialize)]
struct CompactBitmapValue {
    /// Bit 31 = inverted, bits 0..30 = universe_size.
    desc: u32,
    data: Vec<u8>,
}

impl From<&BitmapValue> for CompactBitmapValue {
    fn from(bv: &BitmapValue) -> Self {
        let universe = bv.desc.universe_size();
        let inverted = bv.desc.is_inverted();
        let desc = universe | if inverted { 1 << 31 } else { 0 };
        CompactBitmapValue {
            desc,
            data: bv.data.clone(),
        }
    }
}

pub fn run(path: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    let data = std::fs::read(path)?;
    let file_size = data.len();
    let reader = IndexReader::open(&data)?;
    let fields = reader.field_table()?;
    let sfst = split_fst::Reader::open(&data)?;

    let mut total_bitmaps = 0usize;
    let mut total_singleton_bitmaps = 0usize;
    let mut total_original = 0usize;
    let mut total_compact = 0usize;

    // Primary FST (low-card).
    let (orig, compact, bitmaps, singletons) = repack_fst_chunk(sfst.primary_raw()?)?;
    print_chunk("PRIM", orig, compact, bitmaps, singletons);
    total_original += orig;
    total_compact += compact;
    total_bitmaps += bitmaps;
    total_singleton_bitmaps += singletons;

    // Secondary chunks: mid and high.
    let mut chunk_idx = 0u16;
    for field in &fields {
        match field.tier {
            FieldTier::Low => continue,
            FieldTier::Mid => {
                let raw = sfst.chunk_raw(chunk_idx)?;
                let (orig, compact, bitmaps, singletons) = repack_fst_chunk(raw)?;
                print_chunk(
                    &format!("HC[{chunk_idx}] mid: {}", field.name),
                    orig,
                    compact,
                    bitmaps,
                    singletons,
                );
                total_original += orig;
                total_compact += compact;
                total_bitmaps += bitmaps;
                total_singleton_bitmaps += singletons;
                chunk_idx += 1;
            }
            FieldTier::High => {
                let raw = sfst.chunk_raw(chunk_idx)?;
                let (orig, compact, bitmaps, singletons) = repack_high_chunk(raw)?;
                print_chunk(
                    &format!("HC[{chunk_idx}] high: {}", field.name),
                    orig,
                    compact,
                    bitmaps,
                    singletons,
                );
                total_original += orig;
                total_compact += compact;
                total_bitmaps += bitmaps;
                total_singleton_bitmaps += singletons;
                chunk_idx += 1;
            }
        }
    }

    println!();
    println!(
        "{:<50} {:>8} bitmaps ({} singletons)",
        "TOTAL",
        total_bitmaps,
        total_singleton_bitmaps,
    );
    println!(
        "{:<50} {:>10} original (compressed)",
        "",
        format_size(total_original),
    );
    println!(
        "{:<50} {:>10} compact  (compressed)",
        "",
        format_size(total_compact),
    );
    let saved = total_original.saturating_sub(total_compact);
    let pct = if total_original > 0 {
        saved as f64 / total_original as f64 * 100.0
    } else {
        0.0
    };
    println!(
        "{:<50} {:>10} saved    ({:.1}%)",
        "",
        format_size(saved),
        pct,
    );
    println!(
        "{:<50} {:>10} file size",
        "",
        format_size(file_size),
    );
    let file_pct = if file_size > 0 {
        saved as f64 / file_size as f64 * 100.0
    } else {
        0.0
    };
    println!(
        "{:<50} {:>10} of file  ({:.1}%)",
        "",
        format_size(saved),
        file_pct,
    );

    Ok(())
}

/// Repack an FstIndex<BitmapValue> chunk (primary or mid-card).
/// Returns (original_compressed_size, compact_compressed_size, bitmap_count, singleton_count).
fn repack_fst_chunk(
    compressed: &[u8],
) -> Result<(usize, usize, usize, usize), Box<dyn std::error::Error>> {
    let decompressed =
        zstd::decode_all(compressed).map_err(|e| format!("zstd decompress: {e}"))?;
    let (fst, _): (fst_index::FstIndex<BitmapValue>, _) =
        bincode::serde::decode_from_slice(&decompressed, bincode::config::standard())?;

    let mut compact_values: Vec<CompactBitmapValue> = Vec::new();
    let mut bitmap_count = 0usize;
    let mut singleton_count = 0usize;
    fst.for_each(|_, bv| {
        bitmap_count += 1;
        if bv.desc.len(&bv.data) == 1 {
            singleton_count += 1;
        }
        compact_values.push(CompactBitmapValue::from(bv));
    });

    // We can't easily rebuild the FST with compact values, so measure the
    // values portion only. Build a standalone Vec for comparison.
    let orig_vals_raw =
        bincode::serde::encode_to_vec(&extract_bitmaps(&fst), bincode::config::standard())?;
    let compact_vals_raw =
        bincode::serde::encode_to_vec(&compact_values, bincode::config::standard())?;

    let orig_vals_compressed = zstd::encode_all(&orig_vals_raw[..], 3)?;
    let compact_vals_compressed = zstd::encode_all(&compact_vals_raw[..], 3)?;

    Ok((
        orig_vals_compressed.len(),
        compact_vals_compressed.len(),
        bitmap_count,
        singleton_count,
    ))
}

/// Repack a high-card chunk (bincode blob of Vec<(String, BitmapValue)>).
fn repack_high_chunk(
    compressed: &[u8],
) -> Result<(usize, usize, usize, usize), Box<dyn std::error::Error>> {
    let decompressed =
        zstd::decode_all(compressed).map_err(|e| format!("zstd decompress: {e}"))?;
    let (entries, _): (Vec<(String, BitmapValue)>, _) =
        bincode::serde::decode_from_slice(&decompressed, bincode::config::standard())?;

    let mut singleton_count = 0usize;
    let compact_entries: Vec<(String, CompactBitmapValue)> = entries
        .iter()
        .map(|(k, bv)| {
            if bv.desc.len(&bv.data) == 1 {
                singleton_count += 1;
            }
            (k.clone(), CompactBitmapValue::from(bv))
        })
        .collect();

    let orig_raw =
        bincode::serde::encode_to_vec(&entries, bincode::config::standard())?;
    let compact_raw =
        bincode::serde::encode_to_vec(&compact_entries, bincode::config::standard())?;

    let orig_compressed = zstd::encode_all(&orig_raw[..], 1)?;
    let compact_compressed = zstd::encode_all(&compact_raw[..], 1)?;

    Ok((
        orig_compressed.len(),
        compact_compressed.len(),
        entries.len(),
        singleton_count,
    ))
}

fn extract_bitmaps(fst: &fst_index::FstIndex<BitmapValue>) -> Vec<BitmapValue> {
    let mut out = Vec::new();
    fst.for_each(|_, bv| out.push(bv.clone()));
    out
}

fn print_chunk(name: &str, original: usize, compact: usize, bitmaps: usize, singletons: usize) {
    let saved = original.saturating_sub(compact);
    let pct = if original > 0 {
        saved as f64 / original as f64 * 100.0
    } else {
        0.0
    };
    println!(
        "{:<50} {:>10} → {:>10}  saved {:>10} ({:5.1}%)  bitmaps: {} (singletons: {})",
        name,
        format_size(original),
        format_size(compact),
        format_size(saved),
        pct,
        bitmaps,
        singletons,
    );
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
