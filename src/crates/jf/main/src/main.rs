#![allow(dead_code)]

use error::Result;
use journal_file::*;
use journal_reader::{Direction, JournalReader, Location};
use std::collections::HashMap;
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
        journal_file: &JournalFile<M>,
        entry_offset: u64,
    ) -> Result<EntryData> {
        // Get the entry object
        let entry_object = journal_file.entry_ref(entry_offset)?;

        // Extract basic information from the entry header
        let realtime = entry_object.header.realtime;
        let monotonic = entry_object.header.monotonic;
        let boot_id = format_uuid_bytes(&entry_object.header.boot_id);
        let seqnum = entry_object.header.seqnum;

        drop(entry_object);

        // Create a vector to hold all fields
        let mut fields = Vec::new();

        // Iterate through all data objects for this entry
        for data_result in journal_file.entry_data_objects(entry_offset)? {
            let data_object = data_result?;
            let payload = data_object.payload_bytes();

            // Find the first '=' character to split field and value
            if let Some(equals_pos) = payload.iter().position(|&b| b == b'=') {
                let field = String::from_utf8_lossy(&payload[0..equals_pos]).to_string();
                let value = String::from_utf8_lossy(&payload[equals_pos + 1..]).to_string();

                if field.starts_with("_") {
                    continue;
                }

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

use systemd::journal;

struct JournalWrapper<'a> {
    j: journal::Journal,

    jr: JournalReader<'a, Mmap>,
}

impl<'a> JournalWrapper<'a> {
    pub fn open(path: &str) -> Result<Self> {
        let opts = journal::OpenFilesOptions::default();
        let j = opts.open_files([path])?;
        let jr = JournalReader::default();

        Ok(Self { j, jr })
    }

    pub fn match_add(&mut self, data: &str) {
        self.j.match_add(data).unwrap();
        self.jr.add_match(data.as_bytes());
    }

    pub fn match_and(&mut self, journal_file: &'a JournalFile<Mmap>) {
        self.j.match_and().unwrap();
        self.jr.add_conjunction(journal_file).unwrap();
    }

    pub fn match_or(&mut self, journal_file: &'a JournalFile<Mmap>) {
        self.j.match_or().unwrap();
        self.jr.add_disjunction(journal_file).unwrap();
    }

    pub fn match_flush(&mut self) {
        self.j.match_flush().unwrap();
        self.jr.flush_matches();
    }

    pub fn seek_head(&mut self) {
        self.j.seek_head().unwrap();
        self.jr.set_location(Location::Head);
    }

    pub fn seek_tail(&mut self) {
        self.j.seek_tail().unwrap();
        self.jr.set_location(Location::Tail);
    }

    pub fn seek_realtime(&mut self, usec: u64) {
        self.j.seek_realtime_usec(usec).unwrap();
        self.jr.set_location(Location::Realtime(usec));
    }

    pub fn next(&mut self, journal_file: &'a JournalFile<Mmap>) -> bool {
        let r1 = self.j.next().unwrap();
        let r2 = self.jr.step(journal_file, Direction::Forward).unwrap();

        if r1 > 0 {
            if r2 {
                return r2;
            } else {
                panic!("r1: {:?}, r2: {:?}", r1, r2);
            }
        } else if r1 == 0 {
            if !r2 {
                return r2;
            } else {
                panic!("r1: {:?}, r2: {:?}", r1, r2);
            }
        } else {
            println!("WTF?");
        }

        r2
    }

    pub fn previous(&mut self, journal_file: &'a JournalFile<Mmap>) -> bool {
        let r1 = self.j.previous().unwrap();
        let r2 = self.jr.step(journal_file, Direction::Backward).unwrap();

        if r1 > 0 {
            if r2 {
                return r2;
            } else {
                panic!("r1: {:?}, r2: {:?}", r1, r2);
            }
        } else if r1 == 0 {
            if !r2 {
                return r2;
            } else {
                panic!("r1: {:?}, r2: {:?}", r1, r2);
            }
        } else {
            println!("WTF?");
        }

        r2
    }

    pub fn get_realtime_usec(&mut self, journal_file: &'a JournalFile<Mmap>) -> u64 {
        let usec1 = self.j.timestamp().unwrap();
        let usec2 = self.jr.get_realtime_usec(journal_file).unwrap();

        assert_eq!(usec1, usec2);
        usec1
    }
}

fn get_terms(path: &str) -> HashMap<String, Vec<String>> {
    let window_size = 8 * 1024 * 1024;
    let journal_file = JournalFile::<Mmap>::open(path, window_size).unwrap();

    let mut terms = HashMap::new();
    let mut fields = Vec::new();
    for field in journal_file.fields() {
        let field = field.unwrap();
        let field = String::from(String::from_utf8_lossy(field.get_payload()).clone());
        fields.push(field.clone());
        terms.insert(field, Vec::new());
    }

    for field in fields {
        for data in journal_file.field_data_objects(field.as_bytes()).unwrap() {
            let data = data.unwrap();
            if data.is_compressed() {
                continue;
            }

            let data_payload = String::from(String::from_utf8_lossy(data.get_payload()).clone());

            if data_payload.len() > 200 {
                continue;
            }

            terms.get_mut(&field).unwrap().push(data_payload);
        }
    }

    terms.retain(|_, value| !value.is_empty());
    terms
}

#[derive(Debug)]
enum SeekType {
    Head,
    Tail,
    Realtime(u64),
}

fn get_timings(path: &str) -> Vec<u64> {
    let window_size = 8 * 1024 * 1024;
    let journal_file = JournalFile::<Mmap>::open(path, window_size).unwrap();
    let mut jw: JournalWrapper<'_> = JournalWrapper::open(path).unwrap();

    let mut v = Vec::new();

    jw.seek_head();
    loop {
        if !jw.next(&journal_file) {
            break;
        }

        let usec = jw.get_realtime_usec(&journal_file);
        v.push(usec);
    }
    assert!(v.is_sorted());

    v
}

use rand::{prelude::*, Rng};

#[derive(Debug, Copy, Clone)]
enum SeekOperation {
    Head,
    Tail,
    Realtime(u64),
}

fn select_seek_operation(rng: &mut ThreadRng, timings: &[u64]) -> SeekOperation {
    let duplicate_timestamps = [
        1747729025279631,
        1747729025280143,
        1747729025280247,
        1747729025358451,
        1747729025387355,
        1747729025387415,
    ];

    match rng.random_range(0..3) {
        0 => SeekOperation::Head,
        1 => SeekOperation::Tail,
        2 => {
            let rt_idx = rng.random_range(0..timings.len());

            let usec = timings[rt_idx];
            if duplicate_timestamps.contains(&usec) {
                return SeekOperation::Head;
            }

            SeekOperation::Realtime(timings[rt_idx])
        }
        _ => unreachable!(),
    }
}

#[derive(Debug)]
enum MatchOr {
    None,
    One(String),
    Two(String, String),
}

fn select_match_or(rng: &mut ThreadRng, terms: &HashMap<String, Vec<String>>) -> MatchOr {
    match rng.random_range(0..3) {
        0 => MatchOr::None,
        1 => {
            let key_index = rng.random_range(0..terms.len());
            let key = terms.keys().nth(key_index).unwrap();

            let value = terms.get(key).unwrap();
            let value_index = rng.random_range(0..value.len());

            MatchOr::One(value[value_index].clone())
        }
        2 => {
            let first_term = {
                let key_index = rng.random_range(0..terms.len());
                let key = terms.keys().nth(key_index).unwrap();

                let value = terms.get(key).unwrap();
                let value_index = rng.random_range(0..value.len());

                value[value_index].clone()
            };

            let second_term = {
                let key_index = rng.random_range(0..terms.len());
                let key = terms.keys().nth(key_index).unwrap();

                let value = terms.get(key).unwrap();
                let value_index = rng.random_range(0..value.len());

                value[value_index].clone()
            };

            MatchOr::Two(first_term, second_term)
        }
        _ => {
            unreachable!()
        }
    }
}

#[derive(Debug, Clone)]
enum MatchExpr {
    None,
    OrOne(String),
    OrTwo(String, String),
    And1(String, String),
    And2(String, (String, String)),
    And3((String, String), String),
    And4((String, String), (String, String)),
}

fn select_match_expression(rng: &mut ThreadRng, terms: &HashMap<String, Vec<String>>) -> MatchExpr {
    let mor1 = select_match_or(rng, terms);
    let mor2 = select_match_or(rng, terms);

    let expr = match (mor1, mor2) {
        (MatchOr::None, MatchOr::None) => MatchExpr::None,

        (MatchOr::None, MatchOr::One(d1)) => MatchExpr::OrOne(d1),
        (MatchOr::One(d1), MatchOr::None) => MatchExpr::OrOne(d1),

        (MatchOr::None, MatchOr::Two(d1, d2)) => MatchExpr::OrTwo(d1, d2),
        (MatchOr::Two(d1, d2), MatchOr::None) => MatchExpr::OrTwo(d1, d2),

        (MatchOr::One(d1), MatchOr::One(d2)) => MatchExpr::And1(d1, d2),

        (MatchOr::One(d1), MatchOr::Two(d2, d3)) => MatchExpr::And2(d1, (d2, d3)),
        (MatchOr::Two(d1, d2), MatchOr::One(d3)) => MatchExpr::And3((d1, d2), d3),

        (MatchOr::Two(d1, d2), MatchOr::Two(d3, d4)) => MatchExpr::And4((d1, d2), (d3, d4)),
    };

    match expr.clone() {
        MatchExpr::None | MatchExpr::OrOne(_) => expr,
        MatchExpr::OrTwo(d1, d2) => {
            if d1 == d2 {
                MatchExpr::None
            } else {
                expr
            }
        }
        MatchExpr::And1(d1, d2) => {
            if d1 == d2 {
                MatchExpr::None
            } else {
                expr
            }
        }
        MatchExpr::And2(d1, (d2, d3)) => {
            if d1 == d2 || d1 == d3 || d2 == d3 {
                MatchExpr::None
            } else {
                expr
            }
        }
        MatchExpr::And3((d1, d2), d3) => {
            if d1 == d2 || d1 == d3 || d2 == d3 {
                MatchExpr::None
            } else {
                expr
            }
        }
        MatchExpr::And4((d1, d2), (d3, d4)) => {
            if d1 == d2 || d1 == d3 || d1 == d4 || d2 == d3 || d2 == d4 || d3 == d4 {
                MatchExpr::None
            } else {
                expr
            }
        }
    }
}

#[derive(Debug, Clone, Copy)]
enum IterationOperation {
    Next,
    Previous,
}

fn select_iteration_operation(rng: &mut ThreadRng) -> IterationOperation {
    match rng.random_range(0..2) {
        0 => IterationOperation::Next,
        1 => IterationOperation::Previous,
        _ => unreachable!(),
    }
}

fn apply_seek_operation(seek_operation: SeekOperation, jw: &mut JournalWrapper) {
    match seek_operation {
        SeekOperation::Head => jw.seek_head(),
        SeekOperation::Tail => jw.seek_tail(),
        SeekOperation::Realtime(usec) => jw.seek_realtime(usec),
    }
}

fn apply_iteration_operation<'a>(
    iteration_operation: IterationOperation,
    jw: &mut JournalWrapper<'a>,
    journal_file: &'a JournalFile<Mmap>,
) -> bool {
    match iteration_operation {
        IterationOperation::Next => jw.next(journal_file),
        IterationOperation::Previous => jw.previous(journal_file),
    }
}

fn apply_match_expression<'a>(
    match_expr: MatchExpr,
    jw: &mut JournalWrapper<'a>,
    journal_file: &'a JournalFile<Mmap>,
) -> bool {
    jw.match_flush();

    match match_expr.clone() {
        MatchExpr::None => {
            return true;
        }
        MatchExpr::OrOne(d) => {
            jw.match_add(&d);
            return true;
        }
        MatchExpr::OrTwo(d1, d2) => {
            jw.match_add(&d1);
            jw.match_add(&d2);
            return true;
        }
        MatchExpr::And1(d1, d2) => {
            jw.match_add(&d1);
            jw.match_and(journal_file);
            jw.match_add(&d2);
            return true;
        }
        MatchExpr::And2(d1, (d2, d3)) => {
            jw.match_add(&d1);
            jw.match_and(journal_file);
            jw.match_add(&d2);
            jw.match_add(&d3);
            return true;
        }
        MatchExpr::And3((d1, d2), d3) => {
            jw.match_add(&d1);
            jw.match_add(&d2);
            jw.match_and(journal_file);
            jw.match_add(&d3);
            return true;
        }
        MatchExpr::And4((d1, d2), (d3, d4)) => {
            jw.match_add(&d1);
            jw.match_add(&d2);
            jw.match_or(journal_file);

            jw.match_add(&d3);
            jw.match_add(&d4);
            jw.match_or(journal_file);

            jw.match_and(journal_file);

            return true;
        }
    };
}

fn filtered_test() {
    let path = "/tmp/foo.journal";
    let window_size = 8 * 1024 * 1024;
    let journal_file = JournalFile::<Mmap>::open(path, window_size).unwrap();
    println!(
        "num entries: {:?}",
        journal_file.journal_header_ref().n_entries
    );
    let mut jw = JournalWrapper::open(path).unwrap();

    let terms = get_terms(path);
    let timings = get_timings(path);

    let mut rng = rand::rng();

    let mut counter = 0;
    loop {
        let match_expr = select_match_expression(&mut rng, &terms);
        let applied = apply_match_expression(match_expr.clone(), &mut jw, &journal_file);
        if !applied {
            continue;
        }

        println!("[{}] match_expr: {:?}", counter, match_expr);

        let seek_operation = select_seek_operation(&mut rng, &timings);
        println!("[{}] seek: {:?}", counter, seek_operation);
        apply_seek_operation(seek_operation, &mut jw);

        let mut num_matches = 0;
        let iteration_operation = select_iteration_operation(&mut rng);
        println!("[{}] iteration: {:?}", counter, iteration_operation);

        for _ in 0..rng.random_range(0..2 * timings.len()) {
            let found = apply_iteration_operation(iteration_operation, &mut jw, &journal_file);
            if found {
                jw.get_realtime_usec(&journal_file);
                num_matches += 1;
            }
        }

        println!("[{}] num matches: {:?}\n", counter, num_matches);
        counter += 1;
    }
}

fn test_case() {
    let path = "/tmp/foo.journal";

    let window_size = 8 * 1024 * 1024;
    let journal_file = JournalFile::<Mmap>::open(path, window_size).unwrap();
    let mut jw = JournalWrapper::open(path).unwrap();

    let timings = get_timings(path);
    println!("timings: {:#?}", timings);

    jw.seek_realtime(u64::MAX);
    if jw.previous(&journal_file) {
        let value = jw.get_realtime_usec(&journal_file);
        println!("first value: {:?}", value);
    }
    // return;

    // jw.next(&journal_file);
    // let value = jw.get_realtime_usec(&journal_file);
    // println!("second value: {:?}", value);
}

fn main() {
    // {
    //     let mut jf = JournalFile::<MmapMut>::create("/tmp/muh.journal", 4096).unwrap();

    //     let dht = jf.data_hash_table_mut().unwrap();
    //     let mut items = dht.items;
    //     println!("dht items: {:?}", items.len());
    //     items[0].head_hash_offset = 0xdeadbeef;
    //     items[0].tail_hash_offset = 0xbeefdead;

    //     let fht = jf.field_hash_table_mut().unwrap();
    //     let mut items = fht.items;
    //     println!("fht items: {:?}", items.len());
    //     items[0].head_hash_offset = 0xaaaabbbb;
    //     items[0].tail_hash_offset = 0xccccdddd;

    //     let mut offset_array = jf.offset_array_mut(1024 * 1024, Some(8)).unwrap();

    //     for i in 0..4 {
    //         let offset = std::num::NonZeroU64::new(0xdead0000 + i).unwrap();
    //         offset_array.set(i as usize, offset).unwrap();
    //     }
    // }

    // let jf = JournalFile::<Mmap>::open("/tmp/muh.journal", 4096).unwrap();

    // let offset_array = jf.offset_array_ref(1024 * 1024).unwrap();

    // println!(
    //     "tail object offset: 0x{:x?}",
    //     jf.journal_header_ref().tail_object_offset
    // );
    // println!(
    //     "fht offset: 0x{:x?}",
    //     jf.journal_header_ref().field_hash_table_offset
    // );
    // println!(
    //     "hash table header size: 0x{:x?}",
    //     std::mem::size_of::<ObjectHeader>()
    // );

    // println!("offset_array: {:?}", offset_array);

    // for i in 0..4 {
    //     let offset = offset_array.get(i, 8).unwrap();
    //     println!("offset[{}]: 0x{:x?}", i, offset);
    // }

    filtered_test();
    // test_case()

    //     altime();

    //     let args: Vec<String> = std::env::args().collect();
    //     if args.len() != 2 {
    //         eprintln!("Usage: {} <journal_file_path>", args[0]);
    //         std::process::exit(1);
    //     }

    //     if false {
    //         create_logs();
    //         return;
    //     }

    //     const WINDOW_SIZE: u64 = 4096;
    //     match JournalFile::<Mmap>::open(&args[1], WINDOW_SIZE) {
    //         Ok(journal_file) => {
    //             if true {
    //                 if let Err(e) = test_cursor(&journal_file) {
    //                     panic!("Cursor tests failed: {:?}", e);
    //                 }
    //             }

    //             if true {
    //                 if let Err(e) = test_filter_expr(&journal_file) {
    //                     panic!("Filter expression tests failed: {:?}", e);
    //                 }

    //                 println!("Overall stat: {:?}", journal_file.stats());
    //             }
    //         }
    //         Err(e) => panic!("Failed to open journal file: {:?}", e),
    //     }
}
