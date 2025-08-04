use journal_file::{JournalFile, JournalFileOptions, JournalWriter};
use std::collections::HashMap;
use std::path::Path;

fn main() -> error::Result<()> {
    let output_path = "test_journal.journal";

    // Create test data
    let mut test_data = HashMap::new();
    test_data.insert(
        "MESSAGE",
        vec!["Hello, world!", "Another message", "Final message"],
    );
    test_data.insert("PRIORITY", vec!["6", "4", "3"]);
    test_data.insert(
        "_SYSTEMD_UNIT",
        vec!["test.service", "other.service", "test.service"],
    );
    test_data.insert("_PID", vec!["1234", "5678", "9999"]);

    let boot_id = [1; 16];
    let num_entries = test_data.values().next().unwrap().len();

    // Generate random UUIDs for the journal file
    let mut rng = rand::rng();
    use rand::Rng;

    let options = JournalFileOptions::new(rng.random(), rng.random(), rng.random(), rng.random());

    let mut journal_file = JournalFile::create(Path::new(output_path), options)?;
    let mut writer = JournalWriter::new(&mut journal_file)?;

    // Write multiple copies of the test data to have more entries for benchmarking
    let iterations = 1000;
    for iteration in 0..iterations {
        for i in 0..num_entries {
            let mut entry_data = Vec::new();

            // Build the entry data for this index
            for (key, values) in &test_data {
                let kv_pair = format!("{}={}", key, values[i]);
                entry_data.push(kv_pair.into_bytes());
            }

            // Convert to slice references for the writer
            let entry_refs: Vec<&[u8]> = entry_data.iter().map(|v| v.as_slice()).collect();

            // Write the entry with timestamps
            let realtime = 1000000 + (iteration * 1000 + i as u64 * 100); // Mock realtime in microseconds
            let monotonic = 500000 + (iteration * 1000 + i as u64 * 100); // Mock monotonic time

            writer.add_entry(&mut journal_file, &entry_refs, realtime, monotonic, boot_id)?;
        }
    }

    println!(
        "Created test journal file '{}' with {} entries",
        output_path,
        iterations * num_entries as u64
    );
    println!(
        "Now run: cargo run --bin journal_benchmark -- {}",
        output_path
    );

    Ok(())
}
