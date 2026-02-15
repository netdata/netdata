//! Test FSST compression of FST keys.
//!
//! Builds a unified FST with raw keys (baseline), then trains an FSST symbol
//! table on all keys and rebuilds the FST with compressed keys.  Compares
//! FST automaton size, serialized size, and zstd-compressed size.
//!
//! Usage:
//!   cargo run --release --example fst_fsst -- <journal-file> [--max-cardinality N]

use fsst::Compressor as FsstCompressor;
use journal_core::file::HashableObject;
use journal_core::file::file::{JournalFile, OpenJournalFile};
use journal_core::file::mmap::Mmap;
use journal_registry::repository::File;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, HashMap};
use std::num::NonZeroU64;
use std::time::Instant;

#[derive(Debug, Clone, Serialize, Deserialize)]
enum IndexValue {
    Bitmap {
        desc: treight::Bitmap,
        data: Vec<u8>,
    },
    Counts(u64, u64),
}

fn report_zstd(label: &str, data: &[u8]) {
    for level in [1, 9] {
        let t = Instant::now();
        let compressed = zstd::encode_all(data, level).expect("zstd compress failed");
        let elapsed = t.elapsed();
        let ratio = data.len() as f64 / compressed.len() as f64;
        println!(
            "    {label} zstd level {level:>2}: {} bytes ({:.1} KiB)  ratio {:.2}x  {:.1}ms",
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
    println!("Entries:    {}", header.n_entries);
    println!();

    // Build entry offset -> index mapping.
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

    // Collect field names.
    let mut field_names: Vec<String> = Vec::new();
    for field_result in journal_file.fields() {
        let field_guard = match field_result {
            Ok(g) => g,
            Err(_) => continue,
        };
        if let Ok(name) = std::str::from_utf8(field_guard.raw_payload()) {
            field_names.push(name.to_string());
        }
    }

    // Count cardinality per field.
    let mut field_cardinality: BTreeMap<String, usize> = BTreeMap::new();
    for field_name in &field_names {
        let field_data_iter = match journal_file.field_data_objects(field_name.as_bytes()) {
            Ok(iter) => iter,
            Err(_) => continue,
        };
        let count = field_data_iter.filter(|r| r.is_ok()).count();
        field_cardinality.insert(field_name.clone(), count);
    }

    // Collect all (key, value) pairs for the unified FST.
    println!("=== Building Unified Entries ===");
    let t = Instant::now();

    let mut entries: Vec<(String, IndexValue)> = Vec::new();
    let mut scratch_offsets: Vec<NonZeroU64> = Vec::new();
    let mut scratch_indices: Vec<u32> = Vec::new();

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
            } else {
                let entry_count = scratch_offsets
                    .iter()
                    .filter(|o| **o <= tail_object_offset)
                    .count() as u64;

                entries.push((key, IndexValue::Counts(entry_count, card as u64)));
            }
        }
    }

    let build_elapsed = t.elapsed();
    println!(
        "  Entries: {}  ({:.1}ms)",
        entries.len(),
        build_elapsed.as_secs_f64() * 1000.0
    );

    // Measure raw key bytes.
    let total_key_bytes: usize = entries.iter().map(|(k, _)| k.len()).sum();
    let avg_key_len = total_key_bytes as f64 / entries.len() as f64;
    println!(
        "  Total key bytes: {} ({:.1} KiB)  avg {:.1} bytes/key",
        total_key_bytes,
        total_key_bytes as f64 / 1024.0,
        avg_key_len,
    );
    println!();

    // === Baseline: FST with raw keys ===
    println!("=== Baseline: Raw Keys ===");
    let t = Instant::now();
    let raw_fst: fst_index::FstIndex<IndexValue> =
        fst_index::FstIndex::build(entries.clone()).expect("failed to build raw FST");
    let raw_build_ms = t.elapsed().as_secs_f64() * 1000.0;

    println!("  Keys:       {}", raw_fst.len());
    println!(
        "  FST size:   {} bytes ({:.1} KiB)",
        raw_fst.fst_bytes(),
        raw_fst.fst_bytes() as f64 / 1024.0
    );

    let raw_serialized = bincode::serde::encode_to_vec(&raw_fst, bincode::config::standard())
        .expect("serialize failed");
    println!(
        "  Bincode:    {} bytes ({:.1} KiB)",
        raw_serialized.len(),
        raw_serialized.len() as f64 / 1024.0
    );
    println!("  Build time: {:.1}ms", raw_build_ms);
    report_zstd("raw", &raw_serialized);
    println!();

    // === FSST: train on keys, compress keys, build FST ===
    println!("=== FSST Compressed Keys ===");

    let key_bytes: Vec<&[u8]> = entries.iter().map(|(k, _)| k.as_bytes()).collect();

    let t = Instant::now();
    let fsst_compressor = FsstCompressor::train(&key_bytes);
    let train_ms = t.elapsed().as_secs_f64() * 1000.0;
    println!("  FSST train:    {:.1}ms", train_ms);

    // Compress all keys.
    let t = Instant::now();
    let compressed_keys: Vec<Vec<u8>> = key_bytes
        .iter()
        .map(|k| fsst_compressor.compress(k))
        .collect();
    let compress_ms = t.elapsed().as_secs_f64() * 1000.0;

    let total_compressed_key_bytes: usize = compressed_keys.iter().map(|k| k.len()).sum();
    let avg_compressed_key_len = total_compressed_key_bytes as f64 / compressed_keys.len() as f64;
    let key_ratio = total_key_bytes as f64 / total_compressed_key_bytes as f64;

    println!(
        "  Compressed key bytes: {} ({:.1} KiB)  avg {:.1} bytes/key  ratio {:.2}x  {:.1}ms",
        total_compressed_key_bytes,
        total_compressed_key_bytes as f64 / 1024.0,
        avg_compressed_key_len,
        key_ratio,
        compress_ms,
    );

    // Build FST with compressed keys.
    // FstIndex needs sorted keys — compressed keys preserve sort order only if
    // FSST is prefix-preserving, which it is NOT.  So we must sort.
    let mut fsst_entries: Vec<(Vec<u8>, IndexValue)> = compressed_keys
        .into_iter()
        .zip(entries.iter().map(|(_, v)| v.clone()))
        .collect();
    fsst_entries.sort_by(|a, b| a.0.cmp(&b.0));

    let t = Instant::now();
    let fsst_fst: fst_index::FstIndex<IndexValue> =
        fst_index::FstIndex::build(fsst_entries).expect("failed to build FSST FST");
    let fsst_build_ms = t.elapsed().as_secs_f64() * 1000.0;

    println!("  Keys:          {}", fsst_fst.len());
    println!(
        "  FST size:      {} bytes ({:.1} KiB)",
        fsst_fst.fst_bytes(),
        fsst_fst.fst_bytes() as f64 / 1024.0
    );

    let fsst_serialized = bincode::serde::encode_to_vec(&fsst_fst, bincode::config::standard())
        .expect("serialize failed");
    println!(
        "  Bincode:       {} bytes ({:.1} KiB)",
        fsst_serialized.len(),
        fsst_serialized.len() as f64 / 1024.0
    );
    println!("  Build time:    {:.1}ms", fsst_build_ms);
    report_zstd("fsst", &fsst_serialized);

    // FSST symbol table overhead.
    let symbols = fsst_compressor.symbol_table();
    let lengths = fsst_compressor.symbol_lengths();
    let symbol_table_bytes = symbols.len() * 8 + lengths.len();
    println!(
        "  Symbol table:  {} symbols, {} bytes ({:.1} KiB)",
        symbols.len(),
        symbol_table_bytes,
        symbol_table_bytes as f64 / 1024.0,
    );
    println!();

    // === Summary ===
    println!("=== Summary ===");
    println!("  {:30} {:>12} {:>12}", "", "Raw", "FSST");
    println!(
        "  {:30} {:>12} {:>12}",
        "Key bytes",
        format!("{:.1} KiB", total_key_bytes as f64 / 1024.0),
        format!("{:.1} KiB", total_compressed_key_bytes as f64 / 1024.0),
    );
    println!(
        "  {:30} {:>12} {:>12}",
        "FST automaton",
        format!("{:.1} KiB", raw_fst.fst_bytes() as f64 / 1024.0),
        format!("{:.1} KiB", fsst_fst.fst_bytes() as f64 / 1024.0),
    );
    println!(
        "  {:30} {:>12} {:>12}",
        "Bincode total",
        format!("{:.1} KiB", raw_serialized.len() as f64 / 1024.0),
        format!("{:.1} KiB", fsst_serialized.len() as f64 / 1024.0),
    );

    let raw_z1 = zstd::encode_all(raw_serialized.as_slice(), 1).unwrap();
    let fsst_z1 = zstd::encode_all(fsst_serialized.as_slice(), 1).unwrap();
    let raw_z9 = zstd::encode_all(raw_serialized.as_slice(), 9).unwrap();
    let fsst_z9 = zstd::encode_all(fsst_serialized.as_slice(), 9).unwrap();

    println!(
        "  {:30} {:>12} {:>12}",
        "zstd level 1",
        format!("{:.1} KiB", raw_z1.len() as f64 / 1024.0),
        format!("{:.1} KiB", fsst_z1.len() as f64 / 1024.0),
    );
    println!(
        "  {:30} {:>12} {:>12}",
        "zstd level 9",
        format!("{:.1} KiB", raw_z9.len() as f64 / 1024.0),
        format!("{:.1} KiB", fsst_z9.len() as f64 / 1024.0),
    );
}
