//! The `pcodec-estimate` subcommand — compares stream entry compression
//! between the current zstd approach and pcodec.
//!
//! Reads an .sfst file, decodes each stream's `Vec<Vec<FileId>>` entries,
//! flattens the FileIds, and compresses with pcodec to compare sizes.

use std::path::PathBuf;

use log_index::fst_builder::{FieldTier, FileId};
use log_index::reader::IndexReader;

pub fn run(path: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    let data = std::fs::read(path)?;
    let file_size = data.len();
    let reader = IndexReader::open(&data)?;
    let fields = reader.field_table()?;
    let sfst = split_fst::Reader::open(&data)?;
    let streams = reader.streams();

    let num_field_chunks = fields.iter().filter(|f| f.tier != FieldTier::Low).count();

    let mut total_zstd = 0usize;
    let mut total_pcodec = 0usize;
    let mut total_logs = 0usize;
    let mut total_ids = 0usize;

    for (si, stream) in streams.iter().enumerate() {
        let chunk_idx = (num_field_chunks + si) as u16;
        let compressed_raw = sfst.chunk_raw(chunk_idx)?;
        let zstd_size = compressed_raw.len();

        // Decode the original entries.
        let decompressed =
            zstd::decode_all(compressed_raw).map_err(|e| format!("zstd: {e}"))?;
        let (entries, _): (Vec<Vec<FileId>>, _) =
            bincode::serde::decode_from_slice(&decompressed, bincode::config::standard())?;

        let num_logs = entries.len();

        // Build lengths array and flat ids array.
        let mut lengths: Vec<u32> = Vec::with_capacity(num_logs);
        let mut ids: Vec<u32> = Vec::new();
        for entry in &entries {
            lengths.push(entry.len() as u32);
            for fid in entry {
                ids.push(fid.0);
            }
        }

        // Sort ids within each entry for better pcodec delta compression.
        {
            let mut offset = 0usize;
            for &len in &lengths {
                let end = offset + len as usize;
                ids[offset..end].sort_unstable();
                offset = end;
            }
        }

        // Compress with pcodec.
        let config = pco::ChunkConfig::default().with_compression_level(8);
        let lengths_pco = pco::standalone::simple_compress(&lengths, &config)
            .map_err(|e| format!("pco lengths: {e}"))?;
        let ids_pco = pco::standalone::simple_compress(&ids, &config)
            .map_err(|e| format!("pco ids: {e}"))?;
        let pco_size = lengths_pco.len() + ids_pco.len();

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

        let saved = zstd_size.saturating_sub(pco_size);
        let pct = if zstd_size > 0 {
            saved as f64 / zstd_size as f64 * 100.0
        } else {
            0.0
        };

        println!(
            "STREAM[{si}] {namespace}/{name}",
        );
        println!(
            "  logs: {num_logs}  ids: {}  ids/log: {:.1}",
            ids.len(),
            ids.len() as f64 / num_logs.max(1) as f64,
        );
        println!(
            "  zstd:   {:>10}  (bincode + zstd level 1)",
            format_size(zstd_size),
        );
        println!(
            "  pcodec: {:>10}  (lengths: {} + ids: {})",
            format_size(pco_size),
            format_size(lengths_pco.len()),
            format_size(ids_pco.len()),
        );
        if pco_size < zstd_size {
            println!("  saved:  {:>10}  ({:.1}%)", format_size(saved), pct);
        } else {
            let overhead = pco_size.saturating_sub(zstd_size);
            println!(
                "  LARGER: {:>10}  (+{:.1}%)",
                format_size(overhead),
                overhead as f64 / zstd_size.max(1) as f64 * 100.0,
            );
        }
        println!();

        total_zstd += zstd_size;
        total_pcodec += pco_size;
        total_logs += num_logs;
        total_ids += ids.len();
    }

    println!("TOTAL");
    println!("  logs: {total_logs}  ids: {total_ids}");
    println!("  zstd:   {:>10}", format_size(total_zstd));
    println!("  pcodec: {:>10}", format_size(total_pcodec));
    let saved = total_zstd.saturating_sub(total_pcodec);
    if total_pcodec < total_zstd {
        let pct = saved as f64 / total_zstd as f64 * 100.0;
        println!("  saved:  {:>10}  ({:.1}%)", format_size(saved), pct);
    } else {
        let overhead = total_pcodec.saturating_sub(total_zstd);
        let pct = overhead as f64 / total_zstd.max(1) as f64 * 100.0;
        println!("  LARGER: {:>10}  (+{:.1}%)", format_size(overhead), pct);
    }
    println!("  file:   {:>10}", format_size(file_size));

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
