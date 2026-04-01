//! The `dump-index` subcommand — reconstructs log entries from an .sfst file.

use std::path::PathBuf;
use std::time::Instant;

use log_index::reader::IndexReader;

pub fn run(path: &PathBuf, limit: Option<u32>) -> Result<(), Box<dyn std::error::Error>> {
    let data = std::fs::read(path)?;
    let reader = IndexReader::open(&data)?;

    let t = Instant::now();
    let fields = reader.field_table()?;
    let string_table = reader.build_string_table(&fields)?;
    eprintln!(
        "string table: {} entries ({:.0}ms)",
        string_table.len(),
        t.elapsed().as_secs_f64() * 1000.0,
    );

    let num_field_chunks = fields
        .iter()
        .filter(|f| f.tier != log_index::fst_builder::FieldTier::Low)
        .count();

    let mut total_printed = 0u32;

    for (si, stream) in reader.streams().iter().enumerate() {
        let t = Instant::now();
        let entries = reader.load_stream_entries(si, num_field_chunks)?;
        eprintln!(
            "stream {si} ({}/{}): {} entries ({:.0}ms)",
            stream.namespace,
            stream.name,
            entries.len(),
            t.elapsed().as_secs_f64() * 1000.0,
        );

        for (pos, file_ids) in entries.iter().enumerate() {
            if let Some(max) = limit {
                if total_printed >= max {
                    return Ok(());
                }
            }

            println!("--- log {total_printed} (stream {si}, pos {pos})");
            for id in file_ids {
                let idx = id.0 as usize;
                if idx < string_table.len() {
                    println!("  {}", string_table[idx]);
                } else {
                    println!("  <unknown FileId({})>", id.0);
                }
            }

            total_printed += 1;
        }
    }

    eprintln!("{total_printed} log entries");
    Ok(())
}
