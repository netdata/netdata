//! Measure key-set overlap between two consecutive journal files.
//!
//! For each file, collects the full set of `FIELD=value` keys (from data objects),
//! then reports how many keys are shared, only-in-file-A, and only-in-file-B.
//!
//! Usage:
//!   cargo run --release --example fst_key_overlap -- <journal-file-A> <journal-file-B>

use journal_core::file::HashableObject;
use journal_core::file::file::{JournalFile, OpenJournalFile};
use journal_core::file::mmap::Mmap;
use journal_registry::repository::File;
use std::collections::BTreeSet;

/// Open a journal file and collect all `FIELD=value` keys as strings.
fn collect_keys(path: &str) -> BTreeSet<String> {
    let file = File::from_str(path).unwrap_or_else(|| {
        eprintln!("Failed to parse journal file path: {}", path);
        std::process::exit(1);
    });

    let window_size = 32 * 1024 * 1024;
    let journal_file: JournalFile<Mmap> = OpenJournalFile::new(window_size)
        .load_hash_tables()
        .open(&file)
        .unwrap_or_else(|e| {
            eprintln!("Failed to open journal file {}: {:#?}", path, e);
            std::process::exit(1);
        });

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

    let mut keys = BTreeSet::new();
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
            if data_guard.is_compressed() {
                let mut buf = Vec::new();
                if data_guard.decompress(&mut buf).is_ok() {
                    if let Ok(s) = std::str::from_utf8(&buf) {
                        keys.insert(s.to_string());
                    }
                }
            } else if let Ok(s) = std::str::from_utf8(data_guard.raw_payload()) {
                keys.insert(s.to_string());
            }
        }
    }

    keys
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 3 {
        eprintln!("Usage: {} <journal-file-A> <journal-file-B>", args[0]);
        std::process::exit(1);
    }

    let path_a = &args[1];
    let path_b = &args[2];

    println!("=== Collecting keys ===");
    println!("File A: {}", path_a);
    let keys_a = collect_keys(path_a);
    println!("  Keys: {}", keys_a.len());

    println!("File B: {}", path_b);
    let keys_b = collect_keys(path_b);
    println!("  Keys: {}", keys_b.len());
    println!();

    let shared: BTreeSet<_> = keys_a.intersection(&keys_b).collect();
    let only_a: BTreeSet<_> = keys_a.difference(&keys_b).collect();
    let only_b: BTreeSet<_> = keys_b.difference(&keys_a).collect();
    let union_size = keys_a.union(&keys_b).count();

    let overlap_pct = if union_size > 0 {
        shared.len() as f64 / union_size as f64 * 100.0
    } else {
        0.0
    };

    println!("=== Key Overlap ===");
    println!("  Shared:     {:>8}", shared.len());
    println!("  Only in A:  {:>8}", only_a.len());
    println!("  Only in B:  {:>8}", only_b.len());
    println!("  Union:      {:>8}", union_size);
    println!("  Overlap:    {:>7.1}%", overlap_pct);
    println!();

    // Break down by field: for each field, count shared/only-A/only-B keys
    // Extract field name from "FIELD=value" keys
    let mut fields: BTreeSet<String> = BTreeSet::new();
    for key in keys_a.iter().chain(keys_b.iter()) {
        if let Some(eq_pos) = key.find('=') {
            fields.insert(key[..eq_pos].to_string());
        }
    }

    println!("=== Per-Field Breakdown ===");
    println!(
        "  {:40} {:>8} {:>8} {:>8} {:>8} {:>8}",
        "Field", "A", "B", "Shared", "Only-A", "Only-B"
    );
    println!("  {}", "-".repeat(80));

    for field in &fields {
        let prefix = format!("{}=", field);
        let field_a: BTreeSet<_> = keys_a.iter().filter(|k| k.starts_with(&prefix)).collect();
        let field_b: BTreeSet<_> = keys_b.iter().filter(|k| k.starts_with(&prefix)).collect();
        let field_shared = field_a.intersection(&field_b).count();
        let field_only_a = field_a.difference(&field_b).count();
        let field_only_b = field_b.difference(&field_a).count();

        println!(
            "  {:40} {:>8} {:>8} {:>8} {:>8} {:>8}",
            field,
            field_a.len(),
            field_b.len(),
            field_shared,
            field_only_a,
            field_only_b
        );
    }
}
