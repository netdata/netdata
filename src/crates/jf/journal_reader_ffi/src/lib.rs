use journal_reader::{Direction, JournalReader, Location};
use memmap2::Mmap;
use object_file::{HashableObject, ObjectFile};
use std::ffi::{c_char, c_int, c_void, CStr};

use std::sync::Once;
use tracing::{debug, error, info, instrument};
use tracing_subscriber::{fmt, prelude::*, EnvFilter};

// Initialize tracing once
static INIT_TRACING: Once = Once::new();

// Initialize tracing with file output
fn init_tracing() {
    INIT_TRACING.call_once(|| {
        // Create log directory if it doesn't exist
        let log_dir = std::env::temp_dir();
        let log_path = log_dir.join("ffi.log");

        // Open the file with truncate option
        let file = match std::fs::File::create(&log_path) {
            Ok(file) => file,
            Err(e) => {
                eprintln!("Failed to create log file: {}", e);
                return;
            }
        };

        tracing_subscriber::registry()
            .with(fmt::layer().with_writer(file))
            .with(
                EnvFilter::from_default_env()
                    .add_directive("journal_reader_ffi=error".parse().unwrap()),
            )
            .init();

        info!(
            "Tracing initialized for journal_reader_ffi to {}",
            log_path.display()
        );
    });
}

#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct SdId128 {
    pub bytes: [u8; 16],
}

/// Convert a hex character to its numeric value
fn unhexchar(c: u8) -> Result<u8, i32> {
    match c {
        b'0'..=b'9' => Ok(c - b'0'),
        b'a'..=b'f' => Ok(c - b'a' + 10),
        b'A'..=b'F' => Ok(c - b'A' + 10),
        _ => Err(-22), // -EINVAL
    }
}

/// Parse a string into an sd_id128_t
#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_id128_from_string(s: *const c_char, ret: *mut SdId128) -> i32 {
    debug!("sd_id128_from_string called");

    if s.is_null() || ret.is_null() {
        return -1;
    }

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

/// Compare two sd_id128_t values for equality
#[no_mangle]
pub extern "C" fn sd_id128_equal(a: SdId128, b: SdId128) -> i32 {
    (a.bytes == b.bytes) as i32
}

/// For better Rust integration, also implement the PartialEq trait
impl PartialEq for SdId128 {
    fn eq(&self, other: &Self) -> bool {
        self.bytes == other.bytes
    }
}

impl Eq for SdId128 {}

struct SdJournal<'a> {
    object_file: Box<ObjectFile<Mmap>>,
    reader: JournalReader<'a, Mmap>,
    path: String,
    field_buffer: Vec<u8>,
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_open_files(
    ret: *mut *mut SdJournal,
    paths: *const *const c_char,
    _flags: c_int,
) -> c_int {
    init_tracing();

    if ret.is_null() || paths.is_null() {
        error!("wrong arguments");
        return -1;
    }

    // Get the first path
    let path_ptr = *paths;
    if path_ptr.is_null() {
        error!("wrong arguments");
        return -1;
    }

    // Convert C string to Rust string
    let path = match CStr::from_ptr(path_ptr).to_str() {
        Ok(s) => s,
        Err(e) => {
            error!("failed to convert path: {:?}", e);
            return -1;
        }
    };

    // Create the ObjectFile
    let window_size = 128 * 1024 * 1024;
    let object_file = match ObjectFile::<Mmap>::open(path, window_size) {
        Ok(f) => Box::new(f),
        Err(e) => {
            error!("failed to create object file: {:?}", e);
            return -1;
        }
    };

    let journal = Box::new(SdJournal {
        reader: JournalReader::default(),
        object_file,
        path: String::from(path),
        field_buffer: Vec::with_capacity(256),
    });
    info!(path);

    // Pass ownership to the caller
    *ret = Box::into_raw(journal);

    0
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_close(j: *mut SdJournal) {
    let j = Box::from_raw(j);
    info!(path = j.path);
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_seek_head(j: *mut SdJournal) -> c_int {
    let journal = &mut *j;
    debug!(path = journal.path);

    journal.reader.set_location(Location::Head);
    0
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_seek_tail(j: *mut SdJournal) -> c_int {
    let journal = &mut *j;
    debug!(path = journal.path);
    journal.reader.set_location(Location::Tail);
    0
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_seek_realtime_usec(j: *mut SdJournal, usec: u64) -> c_int {
    let journal = &mut *j;
    debug!(path = journal.path, usec = usec);
    journal.reader.set_location(Location::Realtime(usec));
    0
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_next(j: *mut SdJournal) -> c_int {
    let journal = &mut *j;

    match journal
        .reader
        .step(&journal.object_file, Direction::Forward)
    {
        Ok(has_entry) => {
            if has_entry {
                debug!(path = journal.path, rc = 1);
                1
            } else {
                debug!(path = journal.path, rc = 0);
                0
            }
        }
        Err(e) => {
            debug!(path = journal.path, error = format!("{:?}", e));
            -1
        }
    }
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_previous(j: *mut SdJournal) -> c_int {
    let journal = &mut *j;
    debug!(path = journal.path);

    match journal
        .reader
        .step(&journal.object_file, Direction::Backward)
    {
        Ok(has_entry) => {
            if has_entry {
                debug!(path = journal.path, rc = 1);
                1
            } else {
                debug!(path = journal.path, rc = 0);
                0
            }
        }
        Err(e) => {
            debug!(path = journal.path, error = format!("{:?}", e));
            -1
        }
    }
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_get_seqnum(
    j: *mut SdJournal,
    ret_seqnum: *mut u64,
    ret_seqnum_id: *mut SdId128,
) -> c_int {
    if j.is_null() || ret_seqnum.is_null() {
        return -1;
    }

    let journal = &mut *j;
    debug!(path = journal.path);

    match journal.reader.get_seqnum(&journal.object_file) {
        Ok((seqnum, boot_id)) => {
            debug!(path = journal.path, seqnum, "_boot_id={:?}", boot_id);
            *ret_seqnum = seqnum;

            if !ret_seqnum_id.is_null() {
                *ret_seqnum_id = SdId128 { bytes: boot_id };
            }

            0
        }
        Err(_) => -1,
    }
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_get_realtime_usec(j: *mut SdJournal, ret: *mut u64) -> c_int {
    if j.is_null() || ret.is_null() {
        return -1;
    }

    let journal = &mut *j;

    match journal.reader.get_realtime_usec(&journal.object_file) {
        Ok(realtime) => {
            debug!(path = journal.path, ret = realtime);
            *ret = realtime;
            0
        }
        Err(_) => -1,
    }
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_restart_data(j: *mut SdJournal) {
    if j.is_null() {
        return;
    }

    let journal = &mut *j;
    debug!(path = journal.path);
    journal.reader.entry_data_restart();
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_enumerate_available_data(
    j: *mut SdJournal,
    data: *mut *const c_void,
    l: *mut usize,
) -> c_int {
    if j.is_null() || data.is_null() || l.is_null() {
        return -1;
    }

    let journal = &mut *j;
    debug!(path = journal.path);

    match journal.reader.entry_data_enumerate(&journal.object_file) {
        Ok(Some(data_guard)) => {
            let payload = data_guard.payload_bytes();

            *l = payload.len();
            *data = payload.as_ptr() as *const c_void;

            1
        }
        Ok(None) => 0,
        Err(_) => -1,
    }
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_restart_fields(j: *mut SdJournal) {
    if j.is_null() {
        return;
    }

    let journal = &mut *j;
    debug!(path = journal.path);
    journal.reader.fields_restart();
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_enumerate_fields(
    j: *mut SdJournal,
    field: *mut *const c_char,
) -> c_int {
    if j.is_null() || field.is_null() {
        return -1;
    }

    let journal = &mut *j;

    match journal.reader.fields_enumerate(&journal.object_file) {
        Ok(Some(field_guard)) => {
            let field_name = field_guard.get_payload();

            journal.field_buffer.clear();
            journal.field_buffer.extend_from_slice(field_name);
            journal.field_buffer.push(0);
            *field = journal.field_buffer.as_ptr() as *const c_char;
            debug!(path = journal.path, field = ?String::from_utf8_lossy(field_name));

            1
        }
        Ok(None) => 0,
        Err(_) => -1,
    }
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_query_unique(j: *mut SdJournal, field: *const c_char) -> c_int {
    if j.is_null() || field.is_null() {
        return -1;
    }

    let journal = &mut *j;
    let field_cstr = CStr::from_ptr(field);
    let field_name = field_cstr.to_bytes();

    match journal
        .reader
        .field_data_query_unique(&journal.object_file, field_name)
    {
        Ok(_) => {
            debug!(
                path = journal.path,
                field = ?field_cstr.to_string_lossy()
            );

            0
        }
        Err(e) => {
            error!(error=?e);
            -1
        }
    }
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_restart_unique(j: *mut SdJournal) {
    if j.is_null() {
        return;
    }

    let journal = &mut *j;
    journal.reader.field_data_restart();
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_enumerate_available_unique(
    j: *mut SdJournal,
    data: *mut *const c_void,
    l: *mut usize,
) -> c_int {
    if j.is_null() || data.is_null() || l.is_null() {
        return -1;
    }

    let journal = &mut *j;

    match journal.reader.field_data_enumerate(&journal.object_file) {
        Ok(Some(data_guard)) => {
            let payload = data_guard.get_payload();
            debug!(path = journal.path, payload= ?String::from_utf8_lossy(payload));

            *data = payload.as_ptr() as *const c_void;
            *l = payload.len();

            1
        }
        Ok(None) => {
            debug!(path = journal.path, "No more data entries for field");
            0
        }
        Err(e) => {
            error!(path = journal.path, error = ?e);

            -1
        }
    }
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_add_match(
    j: *mut SdJournal,
    data: *const c_void,
    size: usize,
) -> c_int {
    if j.is_null() || data.is_null() {
        return -1;
    }

    let journal = &mut *j;
    let data_slice = std::slice::from_raw_parts(data as *const u8, size);

    debug!(
        path = journal.path,
        data = format!("{}", String::from_utf8_lossy(data_slice))
    );

    journal.reader.add_match(data_slice);
    0
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_add_conjunction(j: *mut SdJournal) -> c_int {
    if j.is_null() {
        return -1;
    }

    let journal = &mut *j;
    debug!(path = journal.path);

    match journal.reader.add_conjunction(&journal.object_file) {
        Ok(_) => 0,
        Err(e) => {
            error!(path = journal.path, error = ?e, "Failed to add conjunction");
            -1
        }
    }
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_add_disjunction(j: *mut SdJournal) -> c_int {
    if j.is_null() {
        return -1;
    }

    let journal = &mut *j;
    debug!(path = journal.path);

    match journal.reader.add_disjunction(&journal.object_file) {
        Ok(_) => 0,
        Err(e) => {
            error!(path = journal.path, error = ?e, "Failed to add disjunction");
            -1
        }
    }
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_flush_matches(j: *mut SdJournal) {
    if j.is_null() {
        return;
    }

    let journal = &mut *j;
    debug!(path = journal.path);

    journal.reader.flush_matches();
}

#[no_mangle]
#[instrument(skip_all)]
unsafe extern "C" fn sd_journal_log(j: *mut SdJournal) {
    if j.is_null() {
        return;
    }

    let journal = &mut *j;

    journal.reader.log(&journal.object_file, &journal.path);
}
