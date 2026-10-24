//! C API for reading systemd journal files.
//!
//! The exported `rsd_journal_*` functions mirror the systemd `sd_journal_*`
//! API: a handle obtained through `rsd_journal_open_files` is passed as `j` to
//! the other calls and released with `rsd_journal_close`. Functions return `0`
//! on success, `1` when a value or entry was produced, and a negative
//! `JournalError::to_error_code` code on failure.
//!
//! Pointer arguments are generally only validated by `debug_assert!` in debug
//! builds; `j` must always be a live handle from `rsd_journal_open_files`.
use journal_file::{Direction, HashableObject, JournalFile, JournalReader, Location};
use memmap2::Mmap;
use std::ffi::{CStr, c_char, c_int, c_void};

#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct RsdId128 {
    pub bytes: [u8; 16],
}

fn unhexchar(c: u8) -> Result<u8, i32> {
    match c {
        b'0'..=b'9' => Ok(c - b'0'),
        b'a'..=b'f' => Ok(c - b'a' + 10),
        b'A'..=b'F' => Ok(c - b'A' + 10),
        _ => Err(-22), // -EINVAL
    }
}

#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_id128_from_string(s: *const c_char, ret: *mut RsdId128) -> i32 {
    unsafe {
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
}

#[unsafe(no_mangle)]
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

/// Opens the first path in `paths` for reading; the remaining paths and
/// `flags` are accepted for `sd_journal_open_files` compatibility and ignored.
///
/// On success `*ret` receives a handle that the caller must release with
/// `rsd_journal_close`. Opening also installs a process-wide SIGBUS handler so
/// faults in the memory-mapped windows do not abort the process (see the
/// `sigbus` crate).
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_open_files(
    ret: *mut *mut RsdJournal,
    paths: *const *const c_char,
    _flags: c_int,
) -> c_int {
    unsafe {
        debug_assert!(!ret.is_null());
        debug_assert!(!paths.is_null());

        if sigbus::install_handler().is_err() {
            eprintln!("Failed to install sigbus handler");
        }

        let path_ptr = *paths;
        if path_ptr.is_null() {
            return error::JournalError::InvalidFfiOp.to_error_code();
        }

        let path = match CStr::from_ptr(path_ptr).to_str() {
            Ok(s) => s,
            Err(_) => {
                return error::JournalError::InvalidFfiOp.to_error_code();
            }
        };

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

        *ret = Box::into_raw(journal);

        0
    }
}

/// Releases a handle returned by `rsd_journal_open_files`.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_close(j: *mut RsdJournal) {
    unsafe {
        debug_assert!(!j.is_null());
        let _ = Box::from_raw(j);
    }
}

#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_seek_head(j: *mut RsdJournal) -> c_int {
    unsafe {
        debug_assert!(!j.is_null());
        let journal = &mut *j;
        journal.reader.set_location(Location::Head);
        0
    }
}

#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_seek_tail(j: *mut RsdJournal) -> c_int {
    unsafe {
        debug_assert!(!j.is_null());
        let journal = &mut *j;
        journal.reader.set_location(Location::Tail);
        0
    }
}

#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_seek_realtime_usec(j: *mut RsdJournal, usec: u64) -> c_int {
    unsafe {
        debug_assert!(!j.is_null());
        let journal = &mut *j;
        journal.reader.set_location(Location::Realtime(usec));
        0
    }
}

/// Steps to the next entry, honoring the installed matches. Returns `1` if an
/// entry is available, `0` when the end of the journal is reached, negative on
/// error.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_next(j: *mut RsdJournal) -> c_int {
    unsafe {
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
}

/// Steps to the previous entry, honoring the installed matches. Returns `1` if
/// an entry is available, `0` when the start of the journal is reached,
/// negative on error.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_previous(j: *mut RsdJournal) -> c_int {
    unsafe {
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
}

#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_get_seqnum(
    j: *mut RsdJournal,
    ret_seqnum: *mut u64,
    ret_seqnum_id: *mut RsdId128,
) -> c_int {
    unsafe {
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
}

#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_get_realtime_usec(j: *mut RsdJournal, ret: *mut u64) -> c_int {
    unsafe {
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
}

/// Restarts data-field enumeration for the current entry, so the next
/// `rsd_journal_enumerate_available_data` call starts from the first field
/// again.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_restart_data(j: *mut RsdJournal) {
    unsafe {
        debug_assert!(!j.is_null());

        let journal = &mut *j;
        journal.reader.entry_data_restart();
    }
}

/// Returns the next data field of the current entry in `FIELD=value` form:
/// `1` with `*data` and `*l` set, `0` when the entry is exhausted, negative on
/// error.
///
/// The pointer is owned by the journal handle and stays valid only until the
/// next call on the same handle; compressed fields are decompressed into a
/// buffer that is reused between calls.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_enumerate_available_data(
    j: *mut RsdJournal,
    data: *mut *const c_void,
    l: *mut usize,
) -> c_int {
    unsafe {
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
                        Err(e) => e.to_error_code(),
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
}

/// Restarts field-name enumeration, so the next `rsd_journal_enumerate_fields`
/// call starts from the first field again.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_restart_fields(j: *mut RsdJournal) {
    unsafe {
        debug_assert!(!j.is_null());

        let journal = &mut *j;
        journal.reader.fields_restart();
    }
}

/// Returns the next field name defined in the journal as a NUL-terminated
/// string: `1` with `*field` set, `0` when the fields are exhausted, negative
/// on error.
///
/// The string is owned by the journal handle and stays valid only until the
/// next call on the same handle.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_enumerate_fields(
    j: *mut RsdJournal,
    field: *mut *const c_char,
) -> c_int {
    unsafe {
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
}

/// Prepares enumeration of the distinct values stored for `field`; iterate
/// them with `rsd_journal_restart_unique` and
/// `rsd_journal_enumerate_available_unique`.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_query_unique(j: *mut RsdJournal, field: *const c_char) -> c_int {
    unsafe {
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
}

/// Drops the value returned by the last `rsd_journal_enumerate_available_unique`
/// call, invalidating its pointer.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_restart_unique(j: *mut RsdJournal) {
    unsafe {
        debug_assert!(!j.is_null());
        let journal = &mut *j;
        journal.reader.field_data_restart();
    }
}

/// Returns the next distinct value stored for the field given to
/// `rsd_journal_query_unique`: `1` with `*data` and `*l` set, `0` when the
/// values are exhausted, negative on error.
///
/// The pointer is owned by the journal handle and stays valid only until the
/// next call on the same handle.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_enumerate_available_unique(
    j: *mut RsdJournal,
    data: *mut *const c_void,
    l: *mut usize,
) -> c_int {
    unsafe {
        debug_assert!(!j.is_null());
        debug_assert!(!data.is_null());
        debug_assert!(!l.is_null());

        let journal = &mut *j;

        match journal.reader.field_data_enumerate(&journal.journal_file) {
            Ok(Some(data_guard)) => {
                if data_guard.is_compressed() {
                    return match data_guard.decompress(&mut journal.decompressed_payload) {
                        Ok(n) => {
                            *l = n;
                            *data = journal.decompressed_payload.as_ptr() as *const c_void;
                            1
                        }
                        Err(e) => e.to_error_code(),
                    };
                } else {
                    let payload = data_guard.payload_bytes();
                    *data = payload.as_ptr() as *const c_void;
                    *l = payload.len();
                }

                1
            }
            Ok(None) => 0,
            Err(e) => e.to_error_code(),
        }
    }
}

/// Adds a match in `FIELD=value` form. If `size` is zero, `data` is read as a
/// NUL-terminated string; otherwise exactly `size` bytes are used. Matches
/// without a `=` are silently ignored.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_add_match(
    j: *mut RsdJournal,
    data: *const c_void,
    size: usize,
) -> c_int {
    unsafe {
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
}

/// Marks the following matches to be combined with the accumulated filter
/// through a logical AND.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_add_conjunction(j: *mut RsdJournal) -> c_int {
    unsafe {
        debug_assert!(!j.is_null());
        let journal = &mut *j;
        match journal.reader.add_conjunction(&journal.journal_file) {
            Ok(_) => 0,
            Err(e) => e.to_error_code(),
        }
    }
}

/// Marks the following matches to be combined with the accumulated filter
/// through a logical OR.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_add_disjunction(j: *mut RsdJournal) -> c_int {
    unsafe {
        debug_assert!(!j.is_null());

        let journal = &mut *j;
        match journal.reader.add_disjunction(&journal.journal_file) {
            Ok(_) => 0,
            Err(e) => e.to_error_code(),
        }
    }
}

/// Discards all installed matches.
#[unsafe(no_mangle)]
unsafe extern "C" fn rsd_journal_flush_matches(j: *mut RsdJournal) {
    unsafe {
        debug_assert!(!j.is_null());
        let journal = &mut *j;
        journal.reader.flush_matches();
    }
}
