use chrono::{DateTime, TimeZone, Utc};
use error::{JournalError, Result};
use journal_reader::{Direction, JournalReader, Location};
use object_file::*;
use window_manager::MemoryMap;

pub struct EntryData {
    pub offset: u64,
    pub realtime: u64,
    pub monotonic: u64,
    pub boot_id: String,
    pub seqnum: u64,
    pub fields: Vec<(String, String)>,
}

impl std::fmt::Debug for EntryData {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        // Start with a custom struct name
        write!(f, "@{:#x?} {{ fields: [", self.offset)?;

        // Iterate through fields and format each one
        for (i, (key, value)) in self.fields.iter().enumerate() {
            if i > 0 {
                write!(f, ", ")?;
            }
            write!(f, "({:?}, {:?})", key, value)?;
        }

        // Close the formatting
        write!(f, "] }}")
    }
}

impl EntryData {
    /// Extract all data from an entry into an owned structure
    pub fn from_offset<M: MemoryMap>(
        object_file: &ObjectFile<M>,
        entry_offset: u64,
    ) -> Result<EntryData> {
        // Get the entry object
        let entry_object = object_file.entry_object(entry_offset)?;

        // Extract basic information from the entry header
        let realtime = entry_object.header.realtime;
        let monotonic = entry_object.header.monotonic;
        let boot_id = format_uuid_bytes(&entry_object.header.boot_id);
        let seqnum = entry_object.header.seqnum;

        drop(entry_object);

        // Create a vector to hold all fields
        let mut fields = Vec::new();

        // Iterate through all data objects for this entry
        for data_result in object_file.entry_data_objects(entry_offset)? {
            let data_object = data_result?;
            let payload = data_object.payload_bytes();

            // Find the first '=' character to split field and value
            if let Some(equals_pos) = payload.iter().position(|&b| b == b'=') {
                let field = String::from_utf8_lossy(&payload[0..equals_pos]).to_string();
                let value = String::from_utf8_lossy(&payload[equals_pos + 1..]).to_string();

                // if field.starts_with("_") {
                //     continue;
                // }

                fields.push((field, value));
            }
        }

        // Create and return the EntryData struct
        Ok(EntryData {
            offset: entry_offset,
            realtime,
            monotonic,
            boot_id,
            seqnum,
            fields,
        })
    }

    pub fn get_field(&self, name: &str) -> Option<&str> {
        self.fields
            .iter()
            .find(|(k, _)| k == name)
            .map(|(_, v)| v.as_str())
    }

    pub fn datetime(&self) -> DateTime<Utc> {
        let seconds = (self.realtime / 1_000_000) as i64;
        let nanoseconds = ((self.realtime % 1_000_000) * 1000) as u32;
        Utc.timestamp_opt(seconds, nanoseconds).unwrap()
    }
}

fn format_uuid_bytes(bytes: &[u8; 16]) -> String {
    format!(
        "{:02x}{:02x}{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}{:02x}{:02x}{:02x}{:02x}",
        bytes[0], bytes[1], bytes[2], bytes[3],
        bytes[4], bytes[5],
        bytes[6], bytes[7],
        bytes[8], bytes[9],
        bytes[10], bytes[11], bytes[12], bytes[13], bytes[14], bytes[15]
    )
}

fn create_logs() {
    let mut logger = journal_logger::JournalLogger::new(
        "/home/vk/opt/sd/netdata/usr/sbin/log2journal",
        "/home/vk/opt/sd/netdata/usr/sbin/systemd-cat-native",
    );

    for i in 0..5 {
        logger.add_field("SVD_1", "svd-1");
        logger.add_field("MESSAGE", &format!("svd-1-iteration-{}", i));
        logger.flush().unwrap();

        logger.add_field("SVD_1", "svd-a");
        logger.add_field("MESSAGE", &format!("svd-1-iteration-{}", i));
        logger.flush().unwrap();

        logger.add_field("SVD_2", "svd-2");
        logger.add_field("MESSAGE", &format!("svd-2-iteration-{}", i));
        logger.flush().unwrap();

        logger.add_field("SVD_2", "svd-b");
        logger.add_field("MESSAGE", &format!("svd-2-iteration-{}", i));
        logger.flush().unwrap();

        logger.add_field("SVD_3", "svd-3");
        logger.add_field("MESSAGE", &format!("svd-3-iteration-{}", i));
        logger.flush().unwrap();

        logger.add_field("SVD_3", "svd-c");
        logger.add_field("MESSAGE", &format!("svd-3-iteration-{}", i));
        logger.flush().unwrap();

        logger.add_field("SVD_1", "svd-1");
        logger.add_field("SVD_2", "svd-2");
        logger.add_field("MESSAGE", &format!("svd-12-iteration-{}", i));
        logger.flush().unwrap();

        logger.add_field("SVD_1", "svd-1");
        logger.add_field("SVD_3", "svd-3");
        logger.add_field("MESSAGE", &format!("svd-13-iteration-{}", i));
        logger.flush().unwrap();

        logger.add_field("SVD_2", "svd-2");
        logger.add_field("SVD_3", "svd-3");
        logger.add_field("MESSAGE", &format!("svd-23-iteration-{}", i));
        logger.flush().unwrap();

        logger.add_field("SVD_1", "svd-1");
        logger.add_field("SVD_2", "svd-2");
        logger.add_field("SVD_3", "svd-3");
        logger.add_field("MESSAGE", &format!("svd-123-iteration-{}", i));
        logger.flush().unwrap();

        std::thread::sleep(std::time::Duration::from_secs(1));
    }
}

fn test_head<M: MemoryMap>(object_file: &ObjectFile<M>) -> Result<()> {
    let mut jr = JournalReader::default();
    assert!(!jr.step(object_file, Direction::Backward)?);

    match jr.get_realtime_usec(object_file).expect_err("unset cursor") {
        JournalError::UnsetCursor => {}
        _ => {
            panic!("unexpected journal error");
        }
    };

    assert!(jr.step(object_file, Direction::Forward)?);
    let rt1a = jr.get_realtime_usec(object_file).expect("head's realtime");
    assert_eq!(rt1a, object_file.journal_header().head_entry_realtime);

    assert!(jr.step(object_file, Direction::Forward)?);
    let rt2 = jr.get_realtime_usec(object_file).expect("2nd realtime");
    assert_ne!(rt2, rt1a);

    assert!(jr.step(object_file, Direction::Backward)?);
    let rt1b = jr.get_realtime_usec(object_file).expect("head's realtime");
    assert_eq!(rt1a, rt1b);

    assert!(!jr.step(object_file, Direction::Backward).unwrap());
    let rt1c = jr.get_realtime_usec(object_file).expect("head's realtime");
    assert_eq!(rt1a, rt1c);

    while jr.step(object_file, Direction::Forward).unwrap() {}
    while jr.step(object_file, Direction::Backward).unwrap() {}

    let rt1d = jr.get_realtime_usec(object_file).expect("head's realtime");
    assert_eq!(rt1a, rt1d);

    Ok(())
}

fn test_tail<M: MemoryMap>(object_file: &ObjectFile<M>) -> Result<()> {
    let mut jr = JournalReader::default();

    jr.set_location(Location::Tail);
    assert!(!jr.step(object_file, Direction::Forward)?);

    match jr.get_realtime_usec(object_file).expect_err("unset cursor") {
        JournalError::UnsetCursor => {}
        _ => {
            panic!("unexpected journal error");
        }
    };

    assert!(jr.step(object_file, Direction::Backward)?);
    let rt1a = jr.get_realtime_usec(object_file).expect("tails's realtime");
    assert_eq!(rt1a, object_file.journal_header().tail_entry_realtime);

    assert!(jr.step(object_file, Direction::Backward)?);
    let rt2 = jr
        .get_realtime_usec(object_file)
        .expect("realtime before tail");
    assert_ne!(rt2, rt1a);

    assert!(jr.step(object_file, Direction::Forward)?);
    let rt1b = jr.get_realtime_usec(object_file).expect("tails's realtime");
    assert_eq!(rt1a, rt1b);

    assert!(!jr.step(object_file, Direction::Forward).unwrap());
    let rt1c = jr.get_realtime_usec(object_file).expect("tails's realtime");
    assert_eq!(rt1a, rt1c);

    while jr.step(object_file, Direction::Backward).unwrap() {}
    while jr.step(object_file, Direction::Forward).unwrap() {}

    let rt1d = jr.get_realtime_usec(object_file).expect("tail's realtime");
    assert_eq!(rt1a, rt1d);

    Ok(())
}

fn test_midpoint_entry<M: MemoryMap>(object_file: &ObjectFile<M>) -> Result<()> {
    let header = object_file.journal_header();
    let total_entries = header.n_entries;

    let midpoint_idx = total_entries / 2;

    let mut jr = JournalReader::default();

    for _ in 0..midpoint_idx {
        assert!(jr
            .step(object_file, Direction::Forward)
            .expect("step to succeed"));
    }

    let midpoint_entry_offset = jr.get_entry_offset().expect("A valid entry offset");
    let midpoint_entry_realtime = jr.get_realtime_usec(object_file)?;

    jr.set_location(Location::Head);
    assert!(jr.step(object_file, Direction::Forward).expect("no error"));
    assert_eq!(
        jr.get_realtime_usec(object_file)
            .expect("realtime of head entry"),
        object_file.journal_header().head_entry_realtime,
    );

    // By offset
    {
        jr.set_location(Location::Entry(midpoint_entry_offset));
        assert!(jr.step(object_file, Direction::Forward).expect("no error"));

        let entry_offset = jr.get_entry_offset().expect("A valid entry offset");
        assert_eq!(entry_offset, midpoint_entry_offset);
        let entry_realtime = jr.get_realtime_usec(object_file)?;
        assert_eq!(entry_realtime, midpoint_entry_realtime);

        jr.set_location(Location::Entry(midpoint_entry_offset));
        assert!(jr.step(object_file, Direction::Backward).expect("no error"));

        let entry_offset = jr.get_entry_offset().expect("A valid entry offset");
        assert!(entry_offset < midpoint_entry_offset);
        let entry_realtime = jr.get_realtime_usec(object_file)?;
        assert!(entry_realtime < midpoint_entry_realtime);

        assert!(jr.step(object_file, Direction::Forward).expect("no error"));

        let entry_offset = jr.get_entry_offset().expect("A valid entry offset");
        assert_eq!(entry_offset, midpoint_entry_offset);
        let entry_realtime = jr.get_realtime_usec(object_file)?;
        assert_eq!(entry_realtime, midpoint_entry_realtime);
    }

    // By realtime
    {
        jr.set_location(Location::Realtime(midpoint_entry_realtime));
        assert!(jr.step(object_file, Direction::Forward).expect("no error"));

        let entry_offset = jr.get_entry_offset().expect("A valid entry offset");
        assert_eq!(entry_offset, midpoint_entry_offset);
        let entry_realtime = jr.get_realtime_usec(object_file)?;
        assert_eq!(entry_realtime, midpoint_entry_realtime);

        jr.set_location(Location::Realtime(midpoint_entry_realtime));
        assert!(jr.step(object_file, Direction::Backward).expect("no error"));

        let entry_offset = jr.get_entry_offset().expect("A valid entry offset");
        assert!(entry_offset < midpoint_entry_offset);
        let entry_realtime = jr.get_realtime_usec(object_file)?;
        assert!(entry_realtime < midpoint_entry_realtime);

        assert!(jr.step(object_file, Direction::Forward).expect("no error"));

        let entry_offset = jr.get_entry_offset().expect("A valid entry offset");
        assert_eq!(entry_offset, midpoint_entry_offset);
        let entry_realtime = jr.get_realtime_usec(object_file)?;
        assert_eq!(entry_realtime, midpoint_entry_realtime);
    }

    Ok(())
}

fn test_filter<M: MemoryMap>(object_file: &ObjectFile<M>) -> Result<()> {
    let mut jr = JournalReader::default();

    for i in 1..4 {
        let mut entry_offsets = Vec::new();

        let key = format!("SVD_{i}");
        let value = format!("svd-{i}");
        let data = format!("{key}={value}");
        jr.add_match(data.as_bytes());

        jr.set_location(Location::Head);
        while jr.step(object_file, Direction::Forward).unwrap() {
            entry_offsets.push(jr.get_entry_offset().unwrap());
            let ed = EntryData::from_offset(object_file, *entry_offsets.last().unwrap()).unwrap();
            assert_eq!(ed.get_field(key.as_str()), Some(value.as_str()));
        }

        assert_eq!(entry_offsets.len(), 20);
        assert!(entry_offsets.is_sorted());
    }

    for i in 1..4 {
        let mut entry_offsets = Vec::new();

        let key = format!("SVD_{i}");
        let value = format!("svd-{i}");
        let data = format!("{key}={value}");
        jr.add_match(data.as_bytes());

        jr.set_location(Location::Tail);
        while jr.step(object_file, Direction::Backward).unwrap() {
            entry_offsets.push(jr.get_entry_offset().unwrap());
            let ed = EntryData::from_offset(object_file, *entry_offsets.last().unwrap()).unwrap();
            assert_eq!(ed.get_field(key.as_str()), Some(value.as_str()));
        }

        assert_eq!(entry_offsets.len(), 20);
        assert!(entry_offsets.is_sorted_by(|a, b| a > b));
    }

    {
        jr.add_match(b"SVD_1=svd-1");

        jr.set_location(Location::Tail);
        assert!(!jr.step(object_file, Direction::Forward).unwrap());

        jr.set_location(Location::Head);
        assert!(!jr.step(object_file, Direction::Backward).unwrap());
    }

    {
        let mut entry_offsets = Vec::new();

        jr.add_match(b"SVD_1=svd-1");
        jr.add_match(b"SVD_1=svd-a");

        let mut num_svd_1 = 0;
        let mut num_svd_a = 0;

        jr.set_location(Location::Head);
        while jr.step(object_file, Direction::Forward).unwrap() {
            entry_offsets.push(jr.get_entry_offset().unwrap());
            let ed = EntryData::from_offset(object_file, *entry_offsets.last().unwrap()).unwrap();

            if ed.get_field("SVD_1") == Some("svd-1") {
                num_svd_1 += 1;
            } else if ed.get_field("SVD_1") == Some("svd-a") {
                num_svd_a += 1;
            } else {
                panic!("Unexpected value");
            }
        }

        assert_eq!(num_svd_1, 20);
        assert_eq!(num_svd_a, 5);
        assert_eq!(entry_offsets.len(), 20 + 5);
        assert!(entry_offsets.is_sorted());
    }

    {
        let mut entry_offsets = Vec::new();

        jr.add_match(b"SVD_1=svd-1");
        jr.add_match(b"SVD_1=svd-a");

        let mut num_svd_1 = 0;
        let mut num_svd_a = 0;

        jr.set_location(Location::Tail);
        while jr.step(object_file, Direction::Backward).unwrap() {
            entry_offsets.push(jr.get_entry_offset().unwrap());
            let ed = EntryData::from_offset(object_file, *entry_offsets.last().unwrap()).unwrap();

            if ed.get_field("SVD_1") == Some("svd-1") {
                num_svd_1 += 1;
            } else if ed.get_field("SVD_1") == Some("svd-a") {
                num_svd_a += 1;
            } else {
                panic!("Unexpected value");
            }
        }

        assert_eq!(num_svd_1, 20);
        assert_eq!(num_svd_a, 5);
        assert_eq!(entry_offsets.len(), 20 + 5);
        assert!(entry_offsets.is_sorted_by(|a, b| a > b));
    }

    {
        // get realtime of first entry with SVD_1=svd-1
        jr.add_match(b"SVD_1=svd-1");
        jr.set_location(Location::Head);
        assert!(jr.step(object_file, Direction::Forward).unwrap());

        let entry_offset = jr.get_entry_offset().unwrap();
        let ed = EntryData::from_offset(object_file, entry_offset).unwrap();
        assert_eq!(ed.get_field("SVD_1"), Some("svd-1"));
        let svd_1_rt = jr.get_realtime_usec(object_file).unwrap();

        // flush matches and make sure we end up at head
        jr.flush_matches();
        assert!(jr.step(object_file, Direction::Forward).unwrap());
        assert_eq!(
            jr.get_realtime_usec(object_file).unwrap(),
            object_file.journal_header().head_entry_realtime
        );

        // seek by realtime
        for _ in 0..5 {
            jr.add_match(b"SVD_1=svd-1");
            jr.set_location(Location::Realtime(svd_1_rt));
            assert!(jr.step(object_file, Direction::Forward).unwrap());
            assert_eq!(jr.get_realtime_usec(object_file).unwrap(), svd_1_rt);

            assert!(jr.step(object_file, Direction::Forward).unwrap());
            assert!(jr.get_realtime_usec(object_file).unwrap() > svd_1_rt);

            assert!(jr.step(object_file, Direction::Backward).unwrap());
            assert!(jr.get_realtime_usec(object_file).unwrap() == svd_1_rt);

            assert!(!jr.step(object_file, Direction::Backward).unwrap());
        }
    }

    Ok(())
}

fn test_cursor<M: MemoryMap>(object_file: &ObjectFile<M>) -> Result<()> {
    test_head(object_file)?;
    test_tail(object_file)?;
    test_midpoint_entry(object_file)?;

    test_filter(object_file)?;

    println!("All good!");
    Ok(())
}

// Example usage
fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.len() != 2 {
        eprintln!("Usage: {} <journal_file_path>", args[0]);
        std::process::exit(1);
    }

    // Add the journal reader demonstration
    const WINDOW_SIZE: u64 = 4096;
    match ObjectFile::<Mmap>::open(&args[1], WINDOW_SIZE) {
        Ok(object_file) => {
            if true {
                if let Err(e) = test_cursor(&object_file) {
                    eprintln!("Cursor tests failed: {:?}", e);
                }
            }
        }
        Err(e) => eprintln!("Failed to open journal file: {:?}", e),
    }

    if false {
        create_logs()
    }
}
