use journal_file::{
    Direction, JournalFile, JournalFileOptions, JournalReader, JournalWriter, Location,
};
use memmap2::Mmap;
use std::env;
use std::path::Path;
use std::time::{Duration, Instant};
use tempfile::NamedTempFile;

fn main() -> error::Result<()> {
    let args: Vec<String> = env::args().collect();

    if args.len() != 2 {
        eprintln!("Usage: {} <journal_file_path>", args[0]);
        std::process::exit(1);
    }

    let input_path = &args[1];

    println!("Reading journal file: {}", input_path);

    // Step 1: Read all entries from the input journal file
    println!("Step 1: Reading all entries from input file...");
    let (entries, total_read_time) = read_all_entries(input_path)?;
    println!(
        "Read {} entries in {:.2} seconds",
        entries.len(),
        total_read_time.as_secs_f64()
    );

    // Step 2: Create a new journal file and write all entries
    println!("Step 2: Writing all entries to new journal file...");
    let (entries_per_sec, total_write_time) = write_benchmark(&entries)?;

    println!("\n=== BENCHMARK RESULTS ===");
    println!("Total entries: {}", entries.len());
    println!("Read time: {:.3} seconds", total_read_time.as_secs_f64());
    println!("Write time: {:.3} seconds", total_write_time.as_secs_f64());
    println!("Write performance: {:.0} entries/second", entries_per_sec);

    Ok(())
}

#[derive(Debug, Clone)]
struct JournalEntry {
    fields: Vec<Vec<u8>>,
    realtime: u64,
    monotonic: u64,
    boot_id: [u8; 16],
}

fn read_all_entries(journal_path: &str) -> error::Result<(Vec<JournalEntry>, Duration)> {
    let start_time = Instant::now();

    let journal_file = JournalFile::<Mmap>::open(Path::new(journal_path), 64 * 1024 * 1024)?;
    let mut reader = JournalReader::default();
    let mut entries = Vec::new();

    reader.set_location(Location::Head);

    while reader.step(&journal_file, Direction::Forward)? {
        // Get entry metadata
        let realtime = reader.get_realtime_usec(&journal_file)?;
        let (_, boot_id) = reader.get_seqnum(&journal_file)?;
        let monotonic = 0; // We'll use 0 for monotonic as we don't have easy access to it

        // Read all data fields for this entry
        let mut fields = Vec::new();
        reader.entry_data_restart();

        while let Some(data_guard) = reader.entry_data_enumerate(&journal_file)? {
            let payload = data_guard.payload_bytes().to_vec();
            fields.push(payload);
        }

        let entry = JournalEntry {
            fields,
            realtime,
            monotonic,
            boot_id,
        };

        entries.push(entry);
    }

    let read_time = start_time.elapsed();
    Ok((entries, read_time))
}

fn write_benchmark(entries: &[JournalEntry]) -> error::Result<(f64, Duration)> {
    // Create a temporary file for the new journal
    let temp_file = NamedTempFile::new().map_err(error::JournalError::Io)?;
    let journal_path = temp_file.path();

    // Generate random UUIDs for the journal file
    let mut rng = rand::rng();
    use rand::Rng;

    let options = JournalFileOptions::new(rng.random(), rng.random(), rng.random(), rng.random())
        .with_window_size(64 * 1024 * 1024)
        .with_data_hash_table_buckets(1024 * 1024)
        .with_field_hash_table_buckets(16 * 1024)
        .with_keyed_hash(true);

    let mut journal_file = JournalFile::create(journal_path, options)?;
    let mut writer = JournalWriter::new(&mut journal_file)?;

    let start_time = Instant::now();

    for entry in entries {
        // Convert fields to slice references
        let field_refs: Vec<&[u8]> = entry.fields.iter().map(|f| f.as_slice()).collect();

        writer.add_entry(
            &mut journal_file,
            &field_refs,
            entry.realtime,
            entry.monotonic,
            entry.boot_id,
        )?;
    }

    let write_time = start_time.elapsed();
    let entries_per_sec = entries.len() as f64 / write_time.as_secs_f64();

    println!(
        "Wrote {} entries to temporary file: {}",
        entries.len(),
        journal_path.display()
    );

    Ok((entries_per_sec, write_time))
}
