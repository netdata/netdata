//! Split FST prototype using gix-chunk.
//!
//! Splits the unified FST into:
//! - A **primary chunk** (always loaded): field metadata + low-cardinality bitmaps
//! - **Per-field chunks** (loaded on demand): high-cardinality field=value → entry count
//!
//! All packaged in a single file using `gix-chunk` for random access via mmap.
//!
//! File layout:
//! ```text
//! [Header: 12 bytes]          magic "SFST" + version u32 + num_chunks u32
//! [TOC]                       written by gix-chunk (12 bytes × (num_chunks + 1))
//! [Primary FST chunk]         chunk ID: b"PRIM"
//! [High-card field 0 chunk]   chunk ID: [b'H', b'C', hi, lo]
//! [High-card field 1 chunk]   ...
//! ```
//!
//! Usage:
//!   cargo run --release -p journal-core --example fst_split -- <journal-file> [--max-cardinality N]

use arrayvec::ArrayString;
use journal_core::file::HashableObject;
use journal_core::file::file::{JournalFile, OpenJournalFile};
use journal_core::file::mmap::Mmap;
use journal_registry::repository::File;
use rayon::prelude::*;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::num::NonZeroU64;
use std::time::Instant;

const ZSTD_LEVEL: i32 = 1;
const MAX_KEY_LEN: usize = 256;

fn rss_mib() -> f64 {
    let status = std::fs::read_to_string("/proc/self/status").unwrap_or_default();
    for line in status.lines() {
        if line.starts_with("VmRSS:") {
            let kb: f64 = line
                .split_whitespace()
                .nth(1)
                .and_then(|s| s.parse().ok())
                .unwrap_or(0.0);
            return kb / 1024.0;
        }
    }
    0.0
}

fn peak_rss_mib() -> f64 {
    let status = std::fs::read_to_string("/proc/self/status").unwrap_or_default();
    for line in status.lines() {
        if line.starts_with("VmHWM:") {
            let kb: f64 = line
                .split_whitespace()
                .nth(1)
                .and_then(|s| s.parse().ok())
                .unwrap_or(0.0);
            return kb / 1024.0;
        }
    }
    0.0
}

/// Stack-allocated key for FST entries. Avoids heap allocation for the ~924K
/// keys collected during field processing.
#[derive(Clone, PartialEq, Eq, PartialOrd, Ord)]
struct Key(ArrayString<MAX_KEY_LEN>);

impl AsRef<[u8]> for Key {
    fn as_ref(&self) -> &[u8] {
        self.0.as_bytes()
    }
}

unsafe extern "C" {
    fn mallopt(param: i32, value: i32) -> i32;
}
const M_MMAP_THRESHOLD: i32 = -3;

/// Value type for the primary FST.
///
/// Two kinds of keys coexist in the primary FST:
/// - Bare `FIELD` keys (no `=`) → `Field` variant with cardinality and optional HC chunk index
/// - `FIELD=value` keys → `Bitmap` variant (low-cardinality fields only)
#[derive(Debug, Clone, Serialize, Deserialize)]
enum PrimaryValue {
    /// Bare FIELD key: cardinality + optional chunk index for high-card lookup.
    Field {
        cardinality: u64,
        chunk_index: Option<u16>,
    },
    /// Low-cardinality FIELD=value: bitmap of entry indices.
    Bitmap {
        desc: treight::Bitmap,
        data: Vec<u8>,
    },
}

fn main() {
    // Lock glibc's mmap threshold at the default 128 KiB, preventing dynamic
    // adjustment that raises it over time.  Allocations >= this size use mmap
    // and are returned to the OS immediately on free.
    unsafe {
        mallopt(M_MMAP_THRESHOLD, 128 * 1024);
    }

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

    // ── Step 1: Build entry offset → index mapping ──────────────────────
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

    // ── Step 2: Collect field names ─────────────────────────────────────
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

    println!("Fields found: {}", field_names.len());

    // ── Steps 3-6: Single-pass field processing ───────────────────────
    // Instead of separate passes for cardinality counting (old Step 3),
    // classification (old Step 4), bitmap building (old Step 5b), and
    // HC entry counting (old Step 6), we process each field in one iteration:
    //   - Iterate data objects once, collecting key + n_entries + inlined_cursor
    //   - Classify by cardinality (= number of collected items)
    //   - Low-card: build bitmaps via collect_offsets
    //   - High-card: use header.n_entries directly (no chain traversal)
    let t_build = Instant::now();

    let mut primary_entries: Vec<(Key, PrimaryValue)> = Vec::new();
    let mut hc_field_data: Vec<(u16, String, Vec<(Key, u64)>)> = Vec::new();
    let mut next_hc_index: u16 = 0;
    let mut scratch_offsets: Vec<NonZeroU64> = Vec::new();
    let mut scratch_indices: Vec<u32> = Vec::new();
    let mut low_card_fields: Vec<String> = Vec::new();
    let mut high_card_count: usize = 0;
    let mut skipped_too_long: usize = 0;

    for field_name in &field_names {
        let field_data_iter = match journal_file.field_data_objects(field_name.as_bytes()) {
            Ok(iter) => iter,
            Err(_) => continue,
        };

        // Single pass: collect key, n_entries, and inlined_cursor from each data object.
        // Keys are stored in stack-allocated ArrayString<256>; payloads longer than
        // 256 bytes are skipped (they are data blobs, not useful FST keys).
        let mut collected = Vec::new();
        for data_result in field_data_iter {
            let data_guard = match data_result {
                Ok(g) => g,
                Err(_) => continue,
            };

            let key = if data_guard.is_compressed() {
                let mut buf = Vec::new();
                match data_guard.decompress(&mut buf) {
                    Ok(_) => std::str::from_utf8(&buf)
                        .ok()
                        .and_then(|s| ArrayString::from(s).ok())
                        .map(Key),
                    Err(_) => None,
                }
            } else {
                std::str::from_utf8(data_guard.raw_payload())
                    .ok()
                    .and_then(|s| ArrayString::from(s).ok())
                    .map(Key)
            };

            let Some(key) = key else {
                skipped_too_long += 1;
                continue;
            };
            let n_entries = data_guard.header.n_entries.map_or(0, |n| n.get());
            let Some(ic) = data_guard.inlined_cursor() else {
                continue;
            };

            collected.push((key, n_entries, ic));
        }

        let cardinality = collected.len();

        let field_key = Key(ArrayString::from(field_name).expect("field name exceeds MAX_KEY_LEN"));

        if cardinality <= max_cardinality {
            // Low-cardinality: bare FIELD key + bitmaps for each value
            low_card_fields.push(field_name.clone());
            primary_entries.push((
                field_key,
                PrimaryValue::Field {
                    cardinality: cardinality as u64,
                    chunk_index: None,
                },
            ));

            for (key, _, inlined_cursor) in collected {
                scratch_offsets.clear();
                if inlined_cursor
                    .collect_offsets(&journal_file, &mut scratch_offsets)
                    .is_err()
                {
                    continue;
                }

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

                primary_entries.push((
                    key,
                    PrimaryValue::Bitmap {
                        desc,
                        data: bm_data,
                    },
                ));
            }
        } else {
            // High-cardinality: bare FIELD key + per-field HC FST using n_entries directly
            high_card_count += 1;
            let chunk_idx = next_hc_index;
            next_hc_index += 1;

            primary_entries.push((
                field_key,
                PrimaryValue::Field {
                    cardinality: cardinality as u64,
                    chunk_index: Some(chunk_idx),
                },
            ));

            // Collect (key, n_entries) pairs — FST build deferred to parallel phase
            let fst_entries: Vec<(Key, u64)> = collected
                .into_iter()
                .map(|(key, n_entries, _)| (key, n_entries))
                .collect();

            hc_field_data.push((chunk_idx, field_name.clone(), fst_entries));
        }
    }

    println!(
        "Low-cardinality fields:  {} (cardinality <= {})",
        low_card_fields.len(),
        max_cardinality
    );
    println!("High-cardinality fields: {}", high_card_count);
    if skipped_too_long > 0 {
        println!(
            "Skipped (key > {} bytes): {}",
            MAX_KEY_LEN, skipped_too_long
        );
    }
    println!();

    let primary_fst: fst_index::FstIndex<PrimaryValue> =
        fst_index::FstIndex::build(primary_entries).expect("failed to build primary FST");
    let primary_packed = split_fst::pack(&primary_fst, ZSTD_LEVEL).expect("pack primary");

    println!("=== Primary FST ===");
    println!("  Keys:     {}", primary_fst.len());
    println!(
        "  FST size: {} bytes ({:.1} KiB)",
        primary_fst.fst_bytes(),
        primary_fst.fst_bytes() as f64 / 1024.0
    );
    println!(
        "  Packed:   {} bytes ({:.1} KiB)",
        primary_packed.len(),
        primary_packed.len() as f64 / 1024.0,
    );
    println!();

    // Build HC FSTs in parallel — each field's FST is independent
    let mut hc_chunks: Vec<(u16, String, Vec<u8>)> = hc_field_data
        .into_par_iter()
        .map(
            |(chunk_idx, field_name, fst_entries): (u16, String, Vec<(Key, u64)>)| {
                let hc_fst: fst_index::FstIndex<u64> =
                    fst_index::FstIndex::build(fst_entries).expect("failed to build HC FST");
                let packed = split_fst::pack(&hc_fst, ZSTD_LEVEL).expect("pack HC FST");
                (chunk_idx, field_name, packed)
            },
        )
        .collect();

    // Sort by chunk index (par_iter may reorder)
    hc_chunks.sort_by_key(|(idx, _, _)| *idx);

    let build_elapsed = t_build.elapsed();

    println!("=== High-Cardinality Chunks ===");
    for (idx, field, packed) in &hc_chunks {
        println!(
            "  HC[{:>3}] {:<40} {} bytes ({:.1} KiB)",
            idx,
            field,
            packed.len(),
            packed.len() as f64 / 1024.0,
        );
    }
    println!();

    // ── Step 7: Write the split-fst file ──────────────────────────────
    let t_write = Instant::now();

    let output_path = format!("{}.split_fst", journal_path);
    let mut out = std::io::BufWriter::new(
        std::fs::File::create(&output_path).expect("failed to create output file"),
    );

    let mut writer = split_fst::Writer::new();
    writer.set_primary(primary_packed.clone());
    for (_, _, packed) in &hc_chunks {
        writer.add_chunk(packed.clone());
    }
    writer.write_to(&mut out).expect("write split-fst");
    drop(out);

    println!("=== RSS after write ===");
    println!("  RSS:      {:.1} MiB", rss_mib());
    println!("  Peak RSS: {:.1} MiB", peak_rss_mib());
    println!();

    let write_elapsed = t_write.elapsed();

    let file_size = std::fs::metadata(&output_path).expect("stat").len();
    let hc_total_packed: usize = hc_chunks.iter().map(|(_, _, p)| p.len()).sum();

    println!("=== Written File ===");
    println!("  Path:         {}", output_path);
    println!(
        "  Total size:   {} bytes ({:.1} KiB)",
        file_size,
        file_size as f64 / 1024.0
    );
    println!(
        "  Primary:      {} bytes ({:.1} KiB)",
        primary_packed.len(),
        primary_packed.len() as f64 / 1024.0
    );
    println!(
        "  HC total:     {} bytes ({:.1} KiB)",
        hc_total_packed,
        hc_total_packed as f64 / 1024.0
    );
    println!(
        "  Build time:   {:.1}ms",
        build_elapsed.as_secs_f64() * 1000.0
    );
    println!(
        "  Write time:   {:.1}ms",
        write_elapsed.as_secs_f64() * 1000.0
    );
    println!();

    // ── Step 8: Read back via mmap ──────────────────────────────────────
    println!("=== Read-back Verification ===");
    let t_read = Instant::now();

    let read_file = std::fs::File::open(&output_path).expect("open split file");
    let mmap = unsafe { memmap2::Mmap::map(&read_file) }.expect("mmap");
    let file_data = &mmap[..];

    let reader = split_fst::Reader::open(file_data).expect("open split-fst");

    println!("  Num chunks: {}", reader.chunk_count());

    let primary_read: fst_index::FstIndex<PrimaryValue> = reader.primary().expect("read primary");

    println!("  Primary FST: {} keys", primary_read.len(),);

    // Demonstrate targeted access: look up a high-card field
    if let Some((idx, field, _)) = hc_chunks.first() {
        // Step A: consult primary FST for field metadata
        if let Some(pv) = primary_read.get(field.as_bytes()) {
            println!("  Primary['{}'] = {:?}", field, pv);
        }

        // Step B: load + decompress just that field's HC chunk
        let hc_fst: fst_index::FstIndex<u64> = reader.chunk(*idx).expect("read HC chunk");
        println!("  HC[{}] '{}': {} keys", idx, field, hc_fst.len(),);

        // Step C: sample lookup in HC FST
        let mut sample_shown = 0;
        hc_fst.for_each(|key, count| {
            if sample_shown < 3 {
                if let Ok(k) = std::str::from_utf8(key) {
                    println!("    '{}' → {} entries", k, count);
                }
                sample_shown += 1;
            }
        });
    }

    // Demonstrate targeted access: look up a low-card field=value
    if let Some(field) = low_card_fields.first() {
        let mut sample_shown = 0;
        primary_read.prefix_for_each(format!("{}=", field).as_bytes(), |key, val| {
            if sample_shown < 3 {
                if let Ok(k) = std::str::from_utf8(key) {
                    match val {
                        PrimaryValue::Bitmap { desc, data } => {
                            println!("  Primary['{}'] = bitmap({} entries)", k, desc.len(data));
                        }
                        other => {
                            println!("  Primary['{}'] = {:?}", k, other);
                        }
                    }
                }
                sample_shown += 1;
            }
        });
    }

    let read_elapsed = t_read.elapsed();
    println!("  Read time:  {:.1}ms", read_elapsed.as_secs_f64() * 1000.0);
    println!();

    // ── Step 9: Size comparison ─────────────────────────────────────────
    println!("=== Size Comparison ===");
    println!(
        "  Primary chunk (always loaded): {} bytes ({:.1} KiB) packed",
        primary_packed.len(),
        primary_packed.len() as f64 / 1024.0
    );
    println!(
        "  HC chunks (loaded on demand):  {} bytes ({:.1} KiB) packed",
        hc_total_packed,
        hc_total_packed as f64 / 1024.0
    );
    println!(
        "  Total split file:              {} bytes ({:.1} KiB)",
        file_size,
        file_size as f64 / 1024.0
    );
    println!();
    println!("  For a targeted query on a single field, only the primary chunk");
    println!(
        "  ({:.1} KiB) + one HC chunk need to be loaded, vs the full {:.1} KiB.",
        primary_packed.len() as f64 / 1024.0,
        file_size as f64 / 1024.0
    );

    println!("=== Final RSS ===");
    println!("  RSS:      {:.1} MiB", rss_mib());
    println!("  Peak RSS: {:.1} MiB", peak_rss_mib());

    // Cleanup
    std::fs::remove_file(&output_path).ok();
}
