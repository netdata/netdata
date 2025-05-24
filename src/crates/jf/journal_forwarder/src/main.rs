use error::Result;
use journal_logger::JournalLogger;
use journal_reader::JournalReader;
use memmap2::Mmap;
use object_file::ObjectFile;
use rand::seq::{IndexedRandom, SliceRandom};
use std::io::Read;
use std::path::{Path, PathBuf};
use std::time::Duration;
use walkdir::WalkDir;

fn process_one_cycle() -> Result<()> {
    // Find all journal files in the specified directory
    let journal_dir = "/var/log/journal";
    let journal_files = find_journal_files(journal_dir)?;
    if journal_files.is_empty() {
        eprintln!("No journal files found in {}", journal_dir);
        return Ok(());
    }

    // Pick a random journal file
    let mut rng = rand::rng();
    let random_file = journal_files.choose(&mut rng).unwrap();

    println!("Selected journal file: {}", random_file.display());

    // Open the selected journal file
    let object_file = ObjectFile::<Mmap>::open(random_file, 4096)?;

    // Choose random entries
    let entries = select_random_entries(&object_file, 50)?;
    println!("Selected {} entries", entries.len());

    // Split entries into batches
    let batches = split_into_batches(entries, 5);

    // Forward each batch with a delay
    for (i, batch) in batches.iter().enumerate() {
        if i > 0 {
            std::thread::sleep(Duration::from_millis(200));
        }

        let mut logger = journal_logger::JournalLogger::new(
            "/home/vk/opt/sd/netdata/usr/sbin/log2journal",
            "/home/vk/opt/sd/netdata/usr/sbin/systemd-cat-native",
        );

        println!("Forwarding batch {} with {} entries", i + 1, batch.len());
        forward_entries(batch, &mut logger)?;
    }

    Ok(())
}

fn find_journal_files(journal_dir: &str) -> Result<Vec<PathBuf>> {
    let mut journal_files = Vec::new();

    for entry in WalkDir::new(journal_dir)
        .follow_links(true)
        .into_iter()
        .filter_map(|e| e.ok())
    {
        let path = entry.path();
        if path.is_file() && is_journal_file(path) {
            journal_files.push(path.to_path_buf());
        }
    }

    Ok(journal_files)
}

fn is_journal_file(path: &Path) -> bool {
    // Check if file begins with 8 bytes "LPKSHHRH"
    if let Ok(mut file) = std::fs::File::open(path) {
        let mut buffer = [0u8; 8];
        if let Ok(bytes_read) = file.read(&mut buffer) {
            if bytes_read == 8 && buffer == *b"LPKSHHRH" {
                return true;
            }
        }
    }
    false
}

fn select_random_entries(
    object_file: &ObjectFile<Mmap>,
    max_entries: usize,
) -> Result<Vec<EntryData>> {
    let mut rng = rand::rng();

    // Get total number of entries in the journal
    let total_entries = object_file.journal_header().n_entries as usize;
    if total_entries == 0 {
        return Ok(Vec::new());
    }

    // Determine how many entries to select (min of max_entries and total_entries)
    let num_to_select = std::cmp::min(max_entries, total_entries);

    // Create a reader to navigate the journal
    let mut reader = JournalReader::default();
    let mut entries = Vec::new();

    // Generate random indices without repetition
    let mut indices: Vec<usize> = (0..total_entries).collect();
    indices.shuffle(&mut rng);
    indices.truncate(num_to_select);

    // Collect entries at the random indices
    for idx in indices {
        // Reset to head
        reader.set_location(journal_reader::Location::Head);

        // Skip forward to the random index
        for _ in 0..idx {
            if !reader.step(object_file, journal_reader::Direction::Forward)? {
                break;
            }
        }

        // Get the entry at the current position
        if let Ok(entry_offset) = reader.get_entry_offset() {
            if let Ok(entry_data) = extract_entry_data(object_file, entry_offset) {
                entries.push(entry_data);
            }
        }
    }

    Ok(entries)
}

// Structure to hold entry data for forwarding
struct EntryData {
    fields: Vec<(String, String)>,
}

fn extract_entry_data(object_file: &ObjectFile<Mmap>, entry_offset: u64) -> Result<EntryData> {
    let mut fields = Vec::new();

    // Iterate through all data objects for this entry
    for data_result in object_file.entry_data_objects(entry_offset)? {
        let data_object = data_result?;
        let payload = data_object.payload_bytes();

        // Find the first '=' character to split field and value
        if let Some(equals_pos) = payload.iter().position(|&b| b == b'=') {
            let field = String::from_utf8_lossy(&payload[0..equals_pos]).to_string();
            let value = String::from_utf8_lossy(&payload[equals_pos + 1..]).to_string();

            // Skip certain internal fields that shouldn't be forwarded
            if field.starts_with("_") || field == "MESSAGE_ID" || field == "PRIORITY" {
                continue;
            }

            fields.push((field, value));
        }
    }

    Ok(EntryData { fields })
}

fn split_into_batches(entries: Vec<EntryData>, num_batches: usize) -> Vec<Vec<EntryData>> {
    let mut batches = Vec::new();
    let entries_per_batch = (entries.len() + num_batches - 1) / num_batches.max(1);

    let mut entries_iter = entries.into_iter();

    for _ in 0..num_batches {
        let mut batch = Vec::new();
        for _ in 0..entries_per_batch {
            if let Some(entry) = entries_iter.next() {
                batch.push(entry);
            } else {
                break;
            }
        }

        if !batch.is_empty() {
            batches.push(batch);
        }
    }

    batches
}

fn forward_entries(entries: &[EntryData], logger: &mut JournalLogger) -> Result<()> {
    // Add a special field to indicate this is a forwarded entry
    for entry in entries {
        // Add all fields from the original entry
        for (key, value) in &entry.fields {
            logger.add_field(key, value);
        }

        // Add our own marker field
        logger.add_field("JOURNAL_FORWARDER", "1");

        // Flush this entry to the journal
        logger.flush().map_err(error::JournalError::Io)?;
    }

    Ok(())
}

fn main() -> Result<()> {
    // Main loop
    loop {
        if let Err(e) = process_one_cycle() {
            eprintln!("Error during cycle: {:?}", e);
        }

        // Wait for the next cycle
        std::thread::sleep(Duration::from_secs(1));
    }
}
