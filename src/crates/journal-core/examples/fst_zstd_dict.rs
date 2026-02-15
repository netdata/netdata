//! Test zstd dictionary compression for serialized unified FSTs.
//!
//! Builds unified FSTs for N consecutive journal files, trains a zstd dictionary
//! on a subset, then compares compression with and without the dictionary.
//!
//! Usage:
//!   cargo run --release --example fst_zstd_dict -- <journal-dir> [--files N] [--train N] [--max-cardinality N] [--dict-size N]

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

/// Build a serialized unified FST for a single journal file.
fn build_unified_fst(path: &str, max_cardinality: usize) -> Option<Vec<u8>> {
    let file = File::from_str(path)?;
    let window_size = 32 * 1024 * 1024;
    let journal_file: JournalFile<Mmap> = OpenJournalFile::new(window_size)
        .load_hash_tables()
        .open(&file)
        .ok()?;

    let header = journal_file.journal_header_ref();
    let tail_object_offset = header.tail_object_offset?;

    let mut entry_offsets: Vec<NonZeroU64> = Vec::new();
    journal_file.entry_offsets(&mut entry_offsets).ok()?;
    entry_offsets.retain(|o| *o <= tail_object_offset);

    let universe_size = entry_offsets.len() as u32;
    let entry_offset_index: HashMap<NonZeroU64, u32> = entry_offsets
        .iter()
        .enumerate()
        .map(|(idx, offset)| (*offset, idx as u32))
        .collect();

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

    let mut field_cardinality: BTreeMap<String, usize> = BTreeMap::new();
    for field_name in &field_names {
        let field_data_iter = match journal_file.field_data_objects(field_name.as_bytes()) {
            Ok(iter) => iter,
            Err(_) => continue,
        };
        let count = field_data_iter.filter(|r| r.is_ok()).count();
        field_cardinality.insert(field_name.clone(), count);
    }

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

    let unified_fst: fst_index::FstIndex<IndexValue> = fst_index::FstIndex::build(entries).ok()?;

    bincode::serde::encode_to_vec(&unified_fst, bincode::config::standard()).ok()
}

fn parse_arg(args: &[String], flag: &str, default: usize) -> usize {
    args.iter()
        .position(|a| a == flag)
        .and_then(|i| args.get(i + 1))
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 2 {
        eprintln!(
            "Usage: {} <journal-dir> [--files N] [--train N] [--max-cardinality N] [--dict-size N]",
            args[0]
        );
        std::process::exit(1);
    }

    let journal_dir = &args[1];
    let num_files = parse_arg(&args, "--files", 10);
    let num_train = parse_arg(&args, "--train", 3);
    let max_cardinality = parse_arg(&args, "--max-cardinality", 50);
    let dict_size = parse_arg(&args, "--dict-size", 112640); // 110 KiB default

    // Discover journal files in the directory, sorted, pick from the middle.
    let mut journal_paths: Vec<String> = Vec::new();
    let mut dir_entries: Vec<_> = std::fs::read_dir(journal_dir)
        .unwrap_or_else(|e| {
            eprintln!("Failed to read directory {}: {}", journal_dir, e);
            std::process::exit(1);
        })
        .filter_map(|e| e.ok())
        .filter(|e| {
            e.path()
                .extension()
                .map(|ext| ext == "journal")
                .unwrap_or(false)
        })
        .collect();

    dir_entries.sort_by_key(|e| e.file_name());

    let total = dir_entries.len();
    if total < num_files {
        eprintln!(
            "Only {} journal files found, need at least {}",
            total, num_files
        );
        std::process::exit(1);
    }

    // Pick from the middle.
    let start = (total - num_files) / 2;
    for entry in &dir_entries[start..start + num_files] {
        journal_paths.push(entry.path().to_string_lossy().into_owned());
    }

    println!("=== Configuration ===");
    println!("  Directory:       {}", journal_dir);
    println!("  Total files:     {}", total);
    println!(
        "  Selected:        {} (indices {}..{})",
        num_files,
        start,
        start + num_files - 1
    );
    println!("  Train on:        first {} files", num_train);
    println!("  Max cardinality: {}", max_cardinality);
    println!(
        "  Dict max size:   {} bytes ({:.1} KiB)",
        dict_size,
        dict_size as f64 / 1024.0
    );
    println!();

    // Step 1: Build serialized unified FSTs for all files.
    println!("=== Building Unified FSTs ===");
    let mut serialized_fsts: Vec<(String, Vec<u8>)> = Vec::new();

    for (i, path) in journal_paths.iter().enumerate() {
        let t = Instant::now();
        let filename = std::path::Path::new(path)
            .file_name()
            .unwrap_or_default()
            .to_string_lossy();

        match build_unified_fst(path, max_cardinality) {
            Some(data) => {
                let elapsed = t.elapsed();
                println!(
                    "  [{:>2}] {} bytes ({:.1} KiB)  {:.1}ms  {}",
                    i,
                    data.len(),
                    data.len() as f64 / 1024.0,
                    elapsed.as_secs_f64() * 1000.0,
                    filename,
                );
                serialized_fsts.push((filename.into_owned(), data));
            }
            None => {
                eprintln!("  [{:>2}] FAILED: {}", i, filename);
            }
        }
    }
    println!();

    if serialized_fsts.len() < num_train {
        eprintln!("Not enough FSTs built to train dictionary");
        std::process::exit(1);
    }

    // Step 2: Train zstd dictionary on the first `num_train` FSTs.
    //
    // Zstd dictionary training expects many small samples, so we chunk each
    // FST into 128 KiB pieces and feed those as individual samples.
    println!("=== Training Dictionary ===");
    let chunk_size = 128 * 1024; // 128 KiB chunks
    let train_chunks: Vec<&[u8]> = serialized_fsts[..num_train]
        .iter()
        .flat_map(|(_, data)| data.chunks(chunk_size))
        .collect();

    let t = Instant::now();
    let dictionary =
        zstd::dict::from_samples(&train_chunks, dict_size).expect("dictionary training failed");
    let train_elapsed = t.elapsed();

    println!(
        "  Dictionary size:  {} bytes ({:.1} KiB)",
        dictionary.len(),
        dictionary.len() as f64 / 1024.0
    );
    println!(
        "  Training time:    {:.1}ms ({} chunks from {} files)",
        train_elapsed.as_secs_f64() * 1000.0,
        train_chunks.len(),
        num_train
    );
    println!();

    // Step 3: Compress each FST with and without dictionary.
    println!("=== Compression Comparison ===");

    for level in [1, 9] {
        println!("--- zstd level {} ---", level);
        println!(
            "  {:>4}  {:>40}  {:>12} {:>12} {:>12} {:>8}",
            "Idx", "File", "Raw", "Plain", "Dict", "Saving"
        );

        let dict_encoder = zstd::dict::EncoderDictionary::copy(&dictionary, level);

        let mut total_raw = 0usize;
        let mut total_plain = 0usize;
        let mut total_dict = 0usize;

        for (i, (filename, data)) in serialized_fsts.iter().enumerate() {
            let plain = zstd::encode_all(data.as_slice(), level).expect("zstd compress failed");

            let dict_compressed = {
                let mut output = Vec::new();
                let mut encoder =
                    zstd::stream::Encoder::with_prepared_dictionary(&mut output, &dict_encoder)
                        .expect("encoder creation failed");
                std::io::copy(&mut data.as_slice(), &mut encoder).expect("compression failed");
                encoder.finish().expect("finish failed");
                output
            };

            let saving_pct = if plain.len() > 0 {
                (1.0 - dict_compressed.len() as f64 / plain.len() as f64) * 100.0
            } else {
                0.0
            };

            let short_name: String = if filename.len() > 40 {
                format!("...{}", &filename[filename.len() - 37..])
            } else {
                filename.clone()
            };

            let marker = if i < num_train { "*" } else { " " };

            println!(
                "  [{:>2}]{} {:>40}  {:>9.1}K {:>9.1}K {:>9.1}K {:>+7.1}%",
                i,
                marker,
                short_name,
                data.len() as f64 / 1024.0,
                plain.len() as f64 / 1024.0,
                dict_compressed.len() as f64 / 1024.0,
                saving_pct,
            );

            total_raw += data.len();
            total_plain += plain.len();
            total_dict += dict_compressed.len();
        }

        let total_saving_pct = if total_plain > 0 {
            (1.0 - total_dict as f64 / total_plain as f64) * 100.0
        } else {
            0.0
        };

        println!("  {}", "-".repeat(100));
        println!(
            "  {:>4}  {:>40}  {:>9.1}K {:>9.1}K {:>9.1}K {:>+7.1}%",
            "",
            "TOTAL",
            total_raw as f64 / 1024.0,
            total_plain as f64 / 1024.0,
            total_dict as f64 / 1024.0,
            total_saving_pct,
        );
        println!(
            "  {:>4}  {:>40}  {:>9.1}K {:>9.1}K {:>9.1}K",
            "",
            "TOTAL + dict overhead",
            total_raw as f64 / 1024.0,
            total_plain as f64 / 1024.0,
            (total_dict + dictionary.len()) as f64 / 1024.0,
        );
        println!(
            "  {:>4}  {:>40}  {:>9.1}K {:>9.1}K {:>9.1}K",
            "",
            "AVG per file",
            total_raw as f64 / serialized_fsts.len() as f64 / 1024.0,
            total_plain as f64 / serialized_fsts.len() as f64 / 1024.0,
            total_dict as f64 / serialized_fsts.len() as f64 / 1024.0,
        );
        println!();
    }

    println!("  * = training sample");
}
