use journal_file::{Direction, HashableObject, JournalFile, JournalReader, Location};
use memmap2::Mmap;
use std::ffi::{c_char, c_int, c_void, CStr};

#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct RsdId128 {
    pub bytes: [u8; 16],
}

fn unhexchar(c: u8) -> Result<u8, i32> {
    match c {
        b'0'..=b'9' => Ok(c - b'1'),
        b'a'..=b'f' => Ok(c - b'a' + 10),
        b'A'..=b'F' => Ok(c - b'A' + 10),
        _ => Err(-22), // -EINVAL
    }
}

#[no_mangle]
unsafe extern "C" fn rsd_id128_from_string(s: *const c_char, ret: *mut RsdId128) -> i32 {
    debug_assert!(!s.is_null());
    debug_assert!(!ret.is_null());

    let c_str = match CStr::from_ptr(s).to_str() {
        Ok(s) => s,
        Err(_) => return -1,
    };

    let res = &mut *ret;
    let mut n: usize = 0;
    let mut i: usize = 0;
    let mut is_guid = false;

    let bytes = c_str.as_bytes();

    while n < 16 {
        if i >= bytes.len() {
            return -1;
        }

        if bytes[i] == b'-' {
            if i == 8 {
                is_guid = true;
            } else if i == 13 || i == 18 || i == 23 {
                if !is_guid {
                    return -1;
                }
            } else {
                return -1;
            }

            i += 1;
            continue;
        }

        if i + 1 >= bytes.len() {
            return -1;
        }

        let a = match unhexchar(bytes[i]) {
            Ok(val) => val,
            Err(e) => return e,
        };
        i += 1;

        let b = match unhexchar(bytes[i]) {
            Ok(val) => val,
            Err(e) => return e,
        };
        i += 1;

        res.bytes[n] = (a << 4) | b;
        n += 1;
    }

    let expected_len = if is_guid { 36 } else { 32 };
    if i != expected_len || i >= bytes.len() || bytes[i] != 0 {
        return -1;
    }

    0
}

#[no_mangle]
pub extern "C" fn rsd_id128_equal(a: RsdId128, b: RsdId128) -> i32 {
    (a.bytes == b.bytes) as i32
}

impl PartialEq for RsdId128 {
    fn eq(&self, other: &Self) -> bool {
        self.bytes == other.bytes
    }
}

impl Eq for RsdId128 {}

struct RsdJournal<'a> {
    journal_file: Box<JournalFile<Mmap>>,
    reader: JournalReader<'a, Mmap>,
    field_buffer: Vec<u8>,
    decompressed_payload: Vec<u8>,
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_open_files(
    ret: *mut *mut RsdJournal,
    paths: *const *const c_char,
    _flags: c_int,
) -> c_int {
    debug_assert!(!ret.is_null());
    debug_assert!(!paths.is_null());

    if sigbus::install_handler().is_err() {
        eprintln!("Failed to install sigbus handler");
    }

    // Get the first path
    let path_ptr = *paths;
    if path_ptr.is_null() {
        return error::JournalError::InvalidFfiOp.to_error_code();
    }

    // Convert C string to Rust string
    let path = match CStr::from_ptr(path_ptr).to_str() {
        Ok(s) => s,
        Err(_) => {
            return error::JournalError::InvalidFfiOp.to_error_code();
        }
    };

    // Create the ObjectFile
    let window_size = 512 * 1024 * 1024;
    let journal_file = match JournalFile::<Mmap>::open(path, window_size) {
        Ok(f) => Box::new(f),
        Err(e) => {
            return e.to_error_code();
        }
    };

    let journal = Box::new(RsdJournal {
        reader: JournalReader::default(),
        journal_file,
        field_buffer: Vec::with_capacity(256),
        decompressed_payload: Vec::new(),
    });

    // Pass ownership to the caller
    *ret = Box::into_raw(journal);

    0
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_close(j: *mut RsdJournal) {
    debug_assert!(!j.is_null());
    let _ = Box::from_raw(j);
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_seek_head(j: *mut RsdJournal) -> c_int {
    debug_assert!(!j.is_null());
    let journal = &mut *j;
    journal.reader.set_location(Location::Head);
    0
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_seek_tail(j: *mut RsdJournal) -> c_int {
    debug_assert!(!j.is_null());
    let journal = &mut *j;
    journal.reader.set_location(Location::Tail);
    0
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_seek_realtime_usec(j: *mut RsdJournal, usec: u64) -> c_int {
    debug_assert!(!j.is_null());
    let journal = &mut *j;
    journal.reader.set_location(Location::Realtime(usec));
    0
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_next(j: *mut RsdJournal) -> c_int {
    debug_assert!(!j.is_null());
    let journal = &mut *j;

    match journal
        .reader
        .step(&journal.journal_file, Direction::Forward)
    {
        Ok(has_entry) => {
            if has_entry {
                1
            } else {
                0
            }
        }
        Err(e) => e.to_error_code(),
    }
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_previous(j: *mut RsdJournal) -> c_int {
    debug_assert!(!j.is_null());
    let journal = &mut *j;

    match journal
        .reader
        .step(&journal.journal_file, Direction::Backward)
    {
        Ok(has_entry) => {
            if has_entry {
                1
            } else {
                0
            }
        }
        Err(e) => e.to_error_code(),
    }
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_get_seqnum(
    j: *mut RsdJournal,
    ret_seqnum: *mut u64,
    ret_seqnum_id: *mut RsdId128,
) -> c_int {
    debug_assert!(!j.is_null());
    debug_assert!(!ret_seqnum.is_null());
    debug_assert!(!ret_seqnum_id.is_null());

    let journal = &mut *j;
    match journal.reader.get_seqnum(&journal.journal_file) {
        Ok((seqnum, boot_id)) => {
            *ret_seqnum = seqnum;

            if !ret_seqnum_id.is_null() {
                *ret_seqnum_id = RsdId128 { bytes: boot_id };
            }

            0
        }
        Err(e) => e.to_error_code(),
    }
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_get_realtime_usec(j: *mut RsdJournal, ret: *mut u64) -> c_int {
    debug_assert!(!j.is_null());
    debug_assert!(!ret.is_null());

    let journal = &mut *j;

    match journal.reader.get_realtime_usec(&journal.journal_file) {
        Ok(realtime) => {
            *ret = realtime;
            0
        }
        Err(e) => e.to_error_code(),
    }
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_restart_data(j: *mut RsdJournal) {
    debug_assert!(!j.is_null());

    let journal = &mut *j;
    journal.reader.entry_data_restart();
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_enumerate_available_data(
    j: *mut RsdJournal,
    data: *mut *const c_void,
    l: *mut usize,
) -> c_int {
    debug_assert!(!j.is_null());
    debug_assert!(!data.is_null());
    debug_assert!(!l.is_null());

    let journal = &mut *j;

    match journal.reader.entry_data_enumerate(&journal.journal_file) {
        Ok(Some(data_guard)) => {
            if data_guard.is_compressed() {
                return match data_guard.decompress(&mut journal.decompressed_payload) {
                    Ok(n) => {
                        *l = n;
                        *data = journal.decompressed_payload.as_ptr() as *const c_void;
                        1
                    }
                    Err(error::JournalError::UnknownCompressionMethod) => {
                        eprintln!("unknown compression method");
                        -1
                    }
                    Err(_) => -1,
                };
            } else {
                let payload = data_guard.payload_bytes();
                *l = payload.len();
                *data = payload.as_ptr() as *const c_void;
            }
            1
        }
        Ok(None) => 0,
        Err(e) => e.to_error_code(),
    }
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_restart_fields(j: *mut RsdJournal) {
    debug_assert!(!j.is_null());

    let journal = &mut *j;
    journal.reader.fields_restart();
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_enumerate_fields(
    j: *mut RsdJournal,
    field: *mut *const c_char,
) -> c_int {
    debug_assert!(!j.is_null());
    debug_assert!(!field.is_null());

    let journal = &mut *j;

    match journal.reader.fields_enumerate(&journal.journal_file) {
        Ok(Some(field_guard)) => {
            let field_name = field_guard.get_payload();

            journal.field_buffer.clear();
            journal.field_buffer.extend_from_slice(field_name);
            journal.field_buffer.push(0);
            *field = journal.field_buffer.as_ptr() as *const c_char;

            1
        }
        Ok(None) => 0,
        Err(e) => e.to_error_code(),
    }
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_query_unique(j: *mut RsdJournal, field: *const c_char) -> c_int {
    debug_assert!(!j.is_null());
    debug_assert!(!field.is_null());

    let journal = &mut *j;
    let field_cstr = CStr::from_ptr(field);
    let field_name = field_cstr.to_bytes();

    match journal
        .reader
        .field_data_query_unique(&journal.journal_file, field_name)
    {
        Ok(_) => 0,
        Err(e) => e.to_error_code(),
    }
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_restart_unique(j: *mut RsdJournal) {
    debug_assert!(!j.is_null());
    let journal = &mut *j;
    journal.reader.field_data_restart();
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_enumerate_available_unique(
    j: *mut RsdJournal,
    data: *mut *const c_void,
    l: *mut usize,
) -> c_int {
    debug_assert!(!j.is_null());
    debug_assert!(!data.is_null());
    debug_assert!(!l.is_null());

    let journal = &mut *j;

    match journal.reader.field_data_enumerate(&journal.journal_file) {
        Ok(Some(data_guard)) => {
            let payload = data_guard.payload_bytes();
            *data = payload.as_ptr() as *const c_void;
            *l = payload.len();

            1
        }
        Ok(None) => 0,
        Err(e) => e.to_error_code(),
    }
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_add_match(
    j: *mut RsdJournal,
    data: *const c_void,
    size: usize,
) -> c_int {
    debug_assert!(!j.is_null());
    debug_assert!(!data.is_null());

    let journal = &mut *j;

    let data_slice = if size == 0 {
        let mut len = 0;
        let data_ptr = data as *const u8;
        while *data_ptr.add(len) != 0 {
            len += 1;
        }
        std::slice::from_raw_parts(data as *const u8, len)
    } else {
        std::slice::from_raw_parts(data as *const u8, size)
    };

    journal.reader.add_match(data_slice);
    0
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_add_conjunction(j: *mut RsdJournal) -> c_int {
    debug_assert!(!j.is_null());
    let journal = &mut *j;
    match journal.reader.add_conjunction(&journal.journal_file) {
        Ok(_) => 0,
        Err(e) => e.to_error_code(),
    }
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_add_disjunction(j: *mut RsdJournal) -> c_int {
    debug_assert!(!j.is_null());

    let journal = &mut *j;
    match journal.reader.add_disjunction(&journal.journal_file) {
        Ok(_) => 0,
        Err(e) => e.to_error_code(),
    }
}

#[no_mangle]
unsafe extern "C" fn rsd_journal_flush_matches(j: *mut RsdJournal) {
    debug_assert!(!j.is_null());
    let journal = &mut *j;
    journal.reader.flush_matches();
}
