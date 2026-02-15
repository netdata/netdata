//! Explore FST size and field cardinality for a journal file's data objects.
//!
//! Builds a unified FST where:
//! - Low-cardinality `FIELD=value` keys → `IndexValue::Bitmap(bitmap)`
//! - High-cardinality `FIELD=value` keys → `IndexValue::Counts(entry_count, field_cardinality)`
//!
//! This means:
//! - Every field=value pair is present in the FST
//! - Low-cardinality fields have bitmaps for fast filtered queries
//! - High-cardinality fields store lightweight metadata (entry count + field cardinality)
//!
//! Usage:
//!   cargo run --release --example fst_cardinality -- <journal-file> [--max-cardinality N]

use journal_core::file::HashableObject;
use journal_core::file::file::{JournalFile, OpenJournalFile};
use journal_core::file::mmap::Mmap;
use journal_registry::repository::File;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, HashMap};
use std::num::NonZeroU64;
use std::time::Instant;

/// Unified value type for the FST index.
///
/// Every `FIELD=value` key is present:
/// - Low-cardinality fields → `Bitmap` (entry indices for fast filtering)
/// - High-cardinality fields → `Counts` (lightweight metadata)
#[derive(Debug, Clone, Serialize, Deserialize)]
enum IndexValue {
    /// Low-cardinality field=value: bitmap of entry indices.
    Bitmap {
        desc: treight::Bitmap,
        data: Vec<u8>,
    },
    /// High-cardinality field=value: (entry_count, field_cardinality).
    Counts(u64, u64),
}

/// Compress with zstd and print size + time for levels 1 and 9.
fn report_zstd(label: &str, data: &[u8]) {
    for level in [1, 9] {
        let t = Instant::now();
        let compressed = zstd::encode_all(data, level).expect("zstd compress failed");
        let elapsed = t.elapsed();
        let ratio = data.len() as f64 / compressed.len() as f64;
        println!(
            "  {label} zstd level {level:>2}: {} bytes ({:.1} KiB)  ratio {:.2}x  {:.1}ms",
            compressed.len(),
            compressed.len() as f64 / 1024.0,
            ratio,
            elapsed.as_secs_f64() * 1000.0,
        );
    }
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 2 {
        eprintln!("Usage: {} <journal-file> [--max-cardinality N]", args[0]);
        std::process::exit(1);
    }

    let journal_path = &args[1];
    let max_cardinality: usize = args
        .iter()
        .position(|a| a == "--max-cardinality")
        .and_then(|i| args.get(i + 1))
        .and_then(|v| v.parse().ok())
        .unwrap_or(50);

    let file = File::from_str(journal_path).unwrap_or_else(|| {
        eprintln!("Failed to parse journal file path: {}", journal_path);
        std::process::exit(1);
    });

    let window_size = 32 * 1024 * 1024;
    let journal_file: JournalFile<Mmap> = OpenJournalFile::new(window_size)
        .load_hash_tables()
        .open(&file)
        .unwrap_or_else(|e| {
            eprintln!("Failed to open journal file: {:#?}", e);
            std::process::exit(1);
        });

    let header = journal_file.journal_header_ref();
    let tail_object_offset = header
        .tail_object_offset
        .expect("missing tail_object_offset");

    println!("=== Journal File Info ===");
    println!("Path:       {}", journal_path);
    println!("Objects:    {}", header.n_objects);
    println!("Entries:    {}", header.n_entries);
    println!(
        "Arena size: {} bytes ({:.1} MiB)",
        header.arena_size,
        header.arena_size as f64 / (1024.0 * 1024.0)
    );
    println!();

    // Step 1: Build entry offset -> index mapping
    let mut entry_offsets: Vec<NonZeroU64> = Vec::new();
    journal_file
        .entry_offsets(&mut entry_offsets)
        .expect("failed to load entry offsets");
    entry_offsets.retain(|o| *o <= tail_object_offset);

    let universe_size = entry_offsets.len() as u32;
    let entry_offset_index: HashMap<NonZeroU64, u32> = entry_offsets
        .iter()
        .enumerate()
        .map(|(idx, offset)| (*offset, idx as u32))
        .collect();

    println!(
        "Entry offsets loaded: {} (universe_size for bitmaps)",
        universe_size
    );
    println!();

    // Step 2: Collect all field names
    let mut field_names: Vec<String> = Vec::new();
    for field_result in journal_file.fields() {
        let field_guard = match field_result {
            Ok(g) => g,
            Err(e) => {
                eprintln!("Error reading field object: {:#?}", e);
                continue;
            }
        };
        if let Ok(name) = std::str::from_utf8(field_guard.raw_payload()) {
            field_names.push(name.to_string());
        }
    }

    println!("=== Fields Found: {} ===", field_names.len());
    println!();

    // Step 3: Count cardinality per field
    let mut field_cardinality: BTreeMap<String, usize> = BTreeMap::new();
    for field_name in &field_names {
        let field_data_iter = match journal_file.field_data_objects(field_name.as_bytes()) {
            Ok(iter) => iter,
            Err(_) => continue,
        };
        let count = field_data_iter.filter(|r| r.is_ok()).count();
        field_cardinality.insert(field_name.clone(), count);
    }

    // Step 4: Classify fields
    let mut low_card_field_names: Vec<String> = Vec::new();
    let mut high_card_count = 0usize;
    for (field, &card) in &field_cardinality {
        if card <= max_cardinality {
            low_card_field_names.push(field.clone());
        } else {
            high_card_count += 1;
        }
    }

    // Step 5: Reconstruct log entries as text lines
    println!("=== Log Text Baseline ===");
    {
        let mut text_buf: Vec<u8> = Vec::new();
        let mut decompress_buf: Vec<u8> = Vec::new();
        let mut entries_written = 0u64;
        let mut errors = 0u64;

        for &entry_offset in &entry_offsets {
            let data_iter = match journal_file.entry_data_objects(entry_offset) {
                Ok(iter) => iter,
                Err(_) => {
                    errors += 1;
                    continue;
                }
            };

            let line_start = text_buf.len();
            let mut first = true;

            for data_result in data_iter {
                let data_guard = match data_result {
                    Ok(g) => g,
                    Err(_) => {
                        errors += 1;
                        continue;
                    }
                };

                let payload: &[u8] = if data_guard.is_compressed() {
                    decompress_buf.clear();
                    match data_guard.decompress(&mut decompress_buf) {
                        Ok(_) => &decompress_buf,
                        Err(_) => continue,
                    }
                } else {
                    data_guard.raw_payload()
                };

                if !first {
                    text_buf.push(b' ');
                }
                first = false;
                text_buf.extend_from_slice(payload);
            }

            if text_buf.len() > line_start {
                text_buf.push(b'\n');
                entries_written += 1;
            }
        }

        let text_size = text_buf.len();
        println!("  Entries:    {} ({} errors)", entries_written, errors);
        println!(
            "  Text size:  {} bytes ({:.1} MiB)",
            text_size,
            text_size as f64 / (1024.0 * 1024.0)
        );
        report_zstd("text", &text_buf);
    }
    println!();

    // Step 6: Build full FST mapping key=value → data object offset
    println!("=== Full FST (key=value → data object offset) ===");

    let mut all_entries: Vec<(String, u64)> = Vec::new();
    for field_name in &field_names {
        let field_data_iter = match journal_file.field_data_objects(field_name.as_bytes()) {
            Ok(iter) => iter,
            Err(_) => continue,
        };
        for data_result in field_data_iter {
            let data_guard = match data_result {
                Ok(g) => g,
                Err(_) => continue,
            };
            let offset = data_guard.offset().get();
            if data_guard.is_compressed() {
                let mut buf = Vec::new();
                if data_guard.decompress(&mut buf).is_ok() {
                    if let Ok(s) = std::str::from_utf8(&buf) {
                        all_entries.push((s.to_string(), offset));
                    }
                }
            } else if let Ok(s) = std::str::from_utf8(data_guard.raw_payload()) {
                all_entries.push((s.to_string(), offset));
            }
        }
    }

    let full_fst: fst_index::FstIndex<u64> =
        fst_index::FstIndex::build(all_entries).expect("failed to build full FST");

    println!("  Keys:     {}", full_fst.len());
    println!(
        "  FST size: {} bytes ({:.1} KiB)",
        full_fst.fst_bytes(),
        full_fst.fst_bytes() as f64 / 1024.0
    );

    let full_serialized = bincode::serde::encode_to_vec(&full_fst, bincode::config::standard())
        .expect("failed to serialize full FST");
    println!(
        "  Bincode:  {} bytes ({:.1} KiB)",
        full_serialized.len(),
        full_serialized.len() as f64 / 1024.0
    );
    report_zstd("full", &full_serialized);
    println!();

    // Step 7: Build unified FST
    //
    // Key scheme:
    //   "FIELD=value" → IndexValue::Bitmap(bitmap)            (low-cardinality fields)
    //   "FIELD=value" → IndexValue::Counts(entries, card)     (high-cardinality fields)
    println!("=== Unified FST ===");

    let mut entries: Vec<(String, IndexValue)> = Vec::new();
    let mut scratch_offsets: Vec<NonZeroU64> = Vec::new();
    let mut scratch_indices: Vec<u32> = Vec::new();
    let mut total_bitmap_data_bytes: usize = 0;
    let mut bitmap_key_count: usize = 0;
    let mut counts_key_count: usize = 0;

    for (field, &card) in &field_cardinality {
        let field_data_iter = match journal_file.field_data_objects(field.as_bytes()) {
            Ok(iter) => iter,
            Err(_) => continue,
        };

        let is_low_card = card <= max_cardinality;

        for data_result in field_data_iter {
            let (key, inlined_cursor) = {
                let data_guard = match data_result {
                    Ok(g) => g,
                    Err(_) => continue,
                };

                let key = if data_guard.is_compressed() {
                    let mut buf = Vec::new();
                    match data_guard.decompress(&mut buf) {
                        Ok(_) => std::str::from_utf8(&buf).ok().map(|s| s.to_string()),
                        Err(_) => None,
                    }
                } else {
                    std::str::from_utf8(data_guard.raw_payload())
                        .ok()
                        .map(|s| s.to_string())
                };

                let Some(key) = key else { continue };
                let Some(ic) = data_guard.inlined_cursor() else {
                    continue;
                };

                (key, ic)
            };

            scratch_offsets.clear();
            if inlined_cursor
                .collect_offsets(&journal_file, &mut scratch_offsets)
                .is_err()
            {
                continue;
            }

            if is_low_card {
                scratch_indices.clear();
                for offset in scratch_offsets
                    .iter()
                    .copied()
                    .filter(|o| *o <= tail_object_offset)
                {
                    if let Some(&idx) = entry_offset_index.get(&offset) {
                        scratch_indices.push(idx);
                    }
                }
                scratch_indices.sort_unstable();

                total_bitmap_data_bytes +=
                    treight::estimate_data_size(universe_size, scratch_indices.iter().copied());

                let mut bm_data = Vec::new();
                let desc = treight::Bitmap::from_sorted_iter(
                    scratch_indices.iter().copied(),
                    universe_size,
                    &mut bm_data,
                );

                entries.push((
                    key,
                    IndexValue::Bitmap {
                        desc,
                        data: bm_data,
                    },
                ));
                bitmap_key_count += 1;
            } else {
                let entry_count = scratch_offsets
                    .iter()
                    .filter(|o| **o <= tail_object_offset)
                    .count() as u64;

                entries.push((key, IndexValue::Counts(entry_count, card as u64)));
                counts_key_count += 1;
            }
        }
    }

    let unified_fst: fst_index::FstIndex<IndexValue> =
        fst_index::FstIndex::build(entries).expect("failed to build unified FST");

    println!(
        "  Total keys:       {} ({} bitmap + {} counts)",
        unified_fst.len(),
        bitmap_key_count,
        counts_key_count
    );
    println!(
        "  Low-card fields:  {} (cardinality <= {})",
        low_card_field_names.len(),
        max_cardinality
    );
    println!("  High-card fields: {}", high_card_count);
    println!(
        "  FST size:         {} bytes ({:.1} KiB)",
        unified_fst.fst_bytes(),
        unified_fst.fst_bytes() as f64 / 1024.0
    );
    println!(
        "  Bitmap data:      {} bytes ({:.1} KiB)",
        total_bitmap_data_bytes,
        total_bitmap_data_bytes as f64 / 1024.0
    );
    println!();

    // Step 8: Serialize and compress unified FST
    println!("=== Unified FST: Serialized + Compressed ===");

    let serialized = bincode::serde::encode_to_vec(&unified_fst, bincode::config::standard())
        .expect("failed to serialize");
    println!(
        "  Bincode:  {} bytes ({:.1} KiB)",
        serialized.len(),
        serialized.len() as f64 / 1024.0
    );
    report_zstd("unified", &serialized);
    println!();
}
