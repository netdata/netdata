use std::num::NonZeroU64;

use journal_core::file::mmap::Mmap;
use journal_core::file::{EntryDataIterator, HashableObject};
use journal_core::{Direction, JournalCursor, JournalFile, JournalReader, Location};

use crate::SessionError;

use journal_registry::repository::file::Status;

type RegistryFile = journal_registry::repository::File;

/// Instructions for replaying match filters when opening each new file.
///
/// Filters must be re-applied per file because `JournalFilter::build()` resolves
/// match expressions against that file's hash table.
#[derive(Clone)]
pub(crate) enum MatchOp {
    Match(Vec<u8>),
    Disjunction,
}

/// Builder for configuring a [`Cursor`].
pub struct CursorBuilder {
    files: Vec<RegistryFile>,
    window_size: u64,
    load_remappings: bool,
    direction: Direction,
    since: Option<u64>,
    until: Option<u64>,
    match_ops: Vec<MatchOp>,
}

impl CursorBuilder {
    pub(crate) fn new(files: Vec<RegistryFile>, window_size: u64, load_remappings: bool) -> Self {
        Self {
            files,
            window_size,
            load_remappings,
            direction: Direction::Forward,
            since: None,
            until: None,
            match_ops: Vec::new(),
        }
    }

    /// Set the iteration direction.
    pub fn direction(mut self, direction: Direction) -> Self {
        self.direction = direction;
        self
    }

    /// Only return entries with `realtime_usec >= usec` (microseconds since epoch).
    pub fn since(mut self, usec: u64) -> Self {
        self.since = Some(usec);
        self
    }

    /// Only return entries with `realtime_usec <= usec` (microseconds since epoch).
    pub fn until(mut self, usec: u64) -> Self {
        self.until = Some(usec);
        self
    }

    /// Add a match filter (e.g. `b"PRIORITY=3"`).
    ///
    /// Multiple matches on the same field are OR-ed; matches on different
    /// fields are AND-ed (same semantics as `sd_journal_add_match`).
    pub fn add_match(mut self, data: &[u8]) -> Self {
        self.match_ops.push(MatchOp::Match(data.to_vec()));
        self
    }

    /// Insert a disjunction (OR) separator between match groups.
    pub fn add_disjunction(mut self) -> Self {
        self.match_ops.push(MatchOp::Disjunction);
        self
    }

    /// Build the cursor and prepare for iteration.
    pub fn build(self) -> Result<Cursor, SessionError> {
        Cursor::new(
            self.files,
            self.direction,
            self.since,
            self.until,
            self.match_ops,
            self.window_size,
            self.load_remappings,
        )
    }
}

/// Iterates journal entries across multiple files in chronological
/// (or reverse-chronological) order.
///
/// Files are processed sequentially — archived journals first (sorted by
/// `head_realtime`), then the active journal. Each file is opened, filtered,
/// and iterated independently; when one file is exhausted the cursor
/// transparently moves to the next.
///
/// Use [`step()`](Cursor::step) to advance to the next entry, then
/// [`payloads()`](Cursor::payloads) to iterate the entry's data objects
/// as byte slices:
///
/// ```no_run
/// # use journal_session::{JournalSession, Direction};
/// # let session = JournalSession::open("/var/log/journal/id").unwrap();
/// # let mut cursor = session.cursor(Direction::Forward).unwrap();
/// while cursor.step()? {
///     let ts = cursor.realtime_usec();
///     let mut fields = cursor.payloads()?;
///     while let Some(data) = fields.next()? {
///         // data: &[u8] — raw data object payload
///     }
/// }
/// # Ok::<(), journal_session::SessionError>(())
/// ```
pub struct Cursor {
    files: Vec<RegistryFile>,
    direction: Direction,
    since: Option<u64>,
    until: Option<u64>,
    match_ops: Vec<MatchOp>,
    window_size: u64,
    load_remappings: bool,

    file_index: usize,
    journal_file: Option<JournalFile<Mmap>>,
    cursor: JournalCursor,
    decompress_buf: Vec<u8>,
    exhausted: bool,

    // Set by step(), read by accessors and fields().
    entry_offset: Option<NonZeroU64>,
    realtime_usec: u64,
    monotonic_usec: u64,
    boot_id: [u8; 16],
}

impl Cursor {
    fn new(
        mut files: Vec<RegistryFile>,
        direction: Direction,
        since: Option<u64>,
        until: Option<u64>,
        match_ops: Vec<MatchOp>,
        window_size: u64,
        load_remappings: bool,
    ) -> Result<Self, SessionError> {
        // Drop disposed (corrupted) files.
        files.retain(|f| !f.is_disposed());

        // Sort: Archived (by head_realtime) then Active.
        files.sort();

        let file_index = Self::starting_index(&files, direction, since, until);

        Ok(Cursor {
            files,
            direction,
            since,
            until,
            match_ops,
            window_size,
            load_remappings,
            file_index,
            journal_file: None,
            cursor: JournalCursor::new(),
            decompress_buf: Vec::new(),
            exhausted: false,
            entry_offset: None,
            realtime_usec: 0,
            monotonic_usec: 0,
            boot_id: [0; 16],
        })
    }

    /// Compute the starting file index based on direction and time bounds.
    ///
    /// Files are sorted as `[Archived(t1), Archived(t2), …, Active?]` where
    /// `t1 <= t2 <= …`.  We use binary search on `head_realtime` (available
    /// from the filename without opening the file) to skip files that fall
    /// entirely outside the requested time range.
    fn starting_index(
        files: &[RegistryFile],
        direction: Direction,
        since: Option<u64>,
        until: Option<u64>,
    ) -> usize {
        match direction {
            Direction::Forward => {
                let Some(since) = since else { return 0 };
                // Find the first file with head_realtime > since.
                // The file just before it is the last one that could contain
                // entries at or after `since` (its range may span past `since`).
                let pos = files.partition_point(|f| {
                    Self::head_realtime(f).is_some_and(|t| t <= since)
                });
                pos.saturating_sub(1)
            }
            Direction::Backward => {
                let Some(until) = until else {
                    return files.len().saturating_sub(1);
                };
                // Active files (always last, no filename timestamp) are always
                // candidates — we can't determine their range without opening.
                if files.last().is_some_and(|f| f.is_active()) {
                    return files.len() - 1;
                }
                // All files are archived. Find the last with head_realtime <= until.
                let pos = files.partition_point(|f| {
                    Self::head_realtime(f).is_some_and(|t| t <= until)
                });
                pos.saturating_sub(1)
            }
        }
    }

    fn head_realtime(file: &RegistryFile) -> Option<u64> {
        match file.status() {
            Status::Archived { head_realtime, .. } => Some(*head_realtime),
            _ => None,
        }
    }

    /// Open the file at `self.file_index`.
    ///
    /// Uses a temporary `JournalReader` for setup (remappings, match filter
    /// translation, filter build), then extracts the resulting `FilterExpr`
    /// and configures the owned `JournalCursor` for direct iteration.
    fn open_current_file(&mut self) -> Result<bool, SessionError> {
        if self.file_index >= self.files.len() {
            return Ok(false);
        }

        let registry_file = &self.files[self.file_index];
        let journal_file = JournalFile::<Mmap>::open(registry_file, self.window_size)?;

        // Use a temporary reader for setup: remappings + match filter translation.
        // The reader is dropped before we store journal_file, so no lifetime issue.
        let mut reader = JournalReader::<Mmap>::default();

        if self.load_remappings {
            reader.load_remappings(&journal_file)?;
        }

        for op in &self.match_ops {
            match op {
                MatchOp::Match(data) => reader.add_match(data),
                MatchOp::Disjunction => {
                    reader.add_disjunction(&journal_file)?;
                }
            }
        }

        // Build the filter expression (resolves matches against this file's
        // hash table) and transfer it to our cursor.
        let mut cursor = JournalCursor::new();

        if let Some(filter_expr) = reader.build_filter(&journal_file)? {
            cursor.set_filter(filter_expr);
        }

        // reader is dropped here — no borrows from journal_file survive.
        drop(reader);

        // Position the cursor at the appropriate starting point.
        let location = match self.direction {
            Direction::Forward => match self.since {
                Some(usec) => Location::Realtime(usec),
                None => Location::Head,
            },
            Direction::Backward => match self.until {
                Some(usec) => Location::Realtime(usec),
                None => Location::Tail,
            },
        };
        cursor.set_location(location);

        self.journal_file = Some(journal_file);
        self.cursor = cursor;
        Ok(true)
    }

    /// Close the current file and advance the file index.
    /// Returns `true` if there is another file to try.
    fn advance_to_next_file(&mut self) -> bool {
        self.journal_file = None;
        self.entry_offset = None;

        match self.direction {
            Direction::Forward => {
                self.file_index += 1;
                self.file_index < self.files.len()
            }
            Direction::Backward => {
                if self.file_index == 0 {
                    false
                } else {
                    self.file_index -= 1;
                    true
                }
            }
        }
    }

    /// Advance past the current file on error.
    fn skip_file_on_error(&mut self, err: SessionError) -> SessionError {
        if !self.advance_to_next_file() {
            self.exhausted = true;
        }
        err
    }

    /// Advance to the next matching entry, crossing file boundaries as needed.
    ///
    /// Returns `true` if positioned on an entry, `false` when exhausted.
    /// After a successful step, use [`realtime_usec()`](Cursor::realtime_usec),
    /// [`monotonic_usec()`](Cursor::monotonic_usec), [`boot_id()`](Cursor::boot_id)
    /// for entry metadata, and [`payloads()`](Cursor::payloads) to iterate the
    /// entry's data objects as raw byte slices.
    ///
    /// On error the cursor advances past the problematic file so the caller
    /// can retry:
    ///
    /// ```no_run
    /// # use journal_session::{JournalSession, Direction};
    /// # let session = JournalSession::open("/var/log/journal/id").unwrap();
    /// # let mut cursor = session.cursor(Direction::Forward).unwrap();
    /// loop {
    ///     match cursor.step() {
    ///         Ok(true) => {
    ///             let mut fields = cursor.payloads().unwrap();
    ///             while let Some(data) = fields.next().unwrap() {
    ///                 // data: &[u8] — raw data object payload
    ///             }
    ///         }
    ///         Ok(false) => break,
    ///         Err(e) => {
    ///             eprintln!("skipping file: {e}");
    ///             continue;
    ///         }
    ///     }
    /// }
    /// ```
    pub fn step(&mut self) -> Result<bool, SessionError> {
        if self.exhausted {
            return Ok(false);
        }

        self.entry_offset = None;

        loop {
            // Ensure a file is open.
            if self.journal_file.is_none() {
                match self.open_current_file() {
                    Ok(true) => {}
                    Ok(false) => {
                        self.exhausted = true;
                        return Ok(false);
                    }
                    Err(e) => return Err(self.skip_file_on_error(e)),
                }
            }

            // Each block that touches self.journal_file is scoped so the
            // immutable borrow ends before any skip_file_on_error() call.

            // Step to next entry in current file.
            let stepped = {
                let jf = self.journal_file.as_ref().unwrap();
                self.cursor.step(jf, self.direction)
            };
            // TODO: JournalCursor should adopt skip-on-error semantics so that
            // corruption inside an offset array or filter traversal is treated as
            // "file exhausted" rather than a hard error.  Until then, we recover
            // at the file level here.
            let stepped = match stepped {
                Ok(s) => s,
                Err(e) => return Err(self.skip_file_on_error(e.into())),
            };

            if !stepped {
                if !self.advance_to_next_file() {
                    self.exhausted = true;
                    return Ok(false);
                }
                continue;
            }

            let entry_offset = match self.cursor.position() {
                Ok(o) => o,
                Err(e) => return Err(self.skip_file_on_error(e.into())),
            };

            // Read timestamps from entry header.
            let header = {
                let jf = self.journal_file.as_ref().unwrap();
                jf.entry_ref(entry_offset)
                    .map(|r| (r.header.realtime, r.header.monotonic, r.header.boot_id))
            };
            let (realtime_usec, monotonic_usec, boot_id) = match header {
                Ok(h) => h,
                Err(e) => return Err(self.skip_file_on_error(e.into())),
            };

            // Time bound checks.
            match self.direction {
                Direction::Forward => {
                    if let Some(until) = self.until {
                        if realtime_usec > until {
                            self.exhausted = true;
                            return Ok(false);
                        }
                    }
                    if let Some(since) = self.since {
                        if realtime_usec < since {
                            continue;
                        }
                    }
                }
                Direction::Backward => {
                    if let Some(since) = self.since {
                        if realtime_usec < since {
                            self.exhausted = true;
                            return Ok(false);
                        }
                    }
                    if let Some(until) = self.until {
                        if realtime_usec > until {
                            continue;
                        }
                    }
                }
            }

            self.entry_offset = Some(entry_offset);
            self.realtime_usec = realtime_usec;
            self.monotonic_usec = monotonic_usec;
            self.boot_id = boot_id;
            return Ok(true);
        }
    }

    /// Realtime timestamp (microseconds since epoch) of the current entry.
    ///
    /// Only valid after [`step()`](Cursor::step) returns `true`.
    pub fn realtime_usec(&self) -> u64 {
        self.realtime_usec
    }

    /// Monotonic timestamp (microseconds since boot) of the current entry.
    ///
    /// Only valid after [`step()`](Cursor::step) returns `true`.
    pub fn monotonic_usec(&self) -> u64 {
        self.monotonic_usec
    }

    /// Boot ID of the current entry.
    ///
    /// Only valid after [`step()`](Cursor::step) returns `true`.
    pub fn boot_id(&self) -> [u8; 16] {
        self.boot_id
    }

    /// Return a lending iterator over the current entry's data objects.
    ///
    /// Each call to [`Payloads::next()`] yields the raw payload of a data
    /// object as `&[u8]`. Each `next()` call may overwrite the previous
    /// slice (the internal buffer is reused).
    ///
    /// Must be called after [`step()`](Cursor::step) returns `true`.
    pub fn payloads(&mut self) -> Result<Payloads<'_>, SessionError> {
        let entry_offset = self.entry_offset.expect("fields() called without step()");
        let jf = self.journal_file.as_ref().unwrap();
        let iter = jf.entry_data_objects(entry_offset)?;
        Ok(Payloads {
            iter,
            buf: &mut self.decompress_buf,
        })
    }
}

/// Lending iterator over a journal entry's data objects.
///
/// Each [`next()`](Payloads::next) call yields the raw payload of a data
/// object as `&[u8]`. The returned slice borrows from the iterator and
/// is invalidated by the next `next()` call (the internal buffer is reused).
///
/// This cannot implement [`Iterator`] because the yielded reference
/// borrows from `self` (lending iterator pattern).
pub struct Payloads<'a> {
    iter: EntryDataIterator<'a, Mmap>,
    buf: &'a mut Vec<u8>,
}

impl Payloads<'_> {
    /// Advance to the next data object.
    ///
    /// Returns `Ok(Some(payload))` for each data object, `Ok(None)` when done.
    pub fn next(&mut self) -> Result<Option<&[u8]>, SessionError> {
        let guard = match self.iter.next() {
            Some(result) => result?,
            None => return Ok(None),
        };

        if guard.is_compressed() {
            guard.decompress(self.buf)?;
        } else {
            self.buf.clear();
            self.buf.extend_from_slice(guard.raw_payload());
        }

        Ok(Some(self.buf))
    }
}
