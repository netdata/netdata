//! Seek-and-step cursor over a journal file's entry chain: the iteration
//! engine behind [`JournalReader`](crate::file::JournalReader), which
//! delegates `step` to [`JournalCursor::step`] and reads
//! [`JournalCursor::position`] for `get_realtime_usec`/`get_seqnum`/
//! `get_entry_offset` (in `file/reader.rs`). A cursor holds a
//! [`Location`] - a seek anchor or a resolved
//! entry - plus, depending on mode, a filter expression or a position in
//! the file's entry-array chain.
//!
//! `step` picks its resolver by whether a filter is set:
//! - unfiltered: walk the entry-array chain with an
//!   [`offset_array::Cursor`]. The chain lists every entry-object file
//!   offset in write order, so stepping forward/backward is later/earlier
//!   entries; entry offsets strictly increase along it (the writer
//!   appends each entry at a growing offset -
//!   [`JournalWriter::add_entry`](crate::file::JournalWriter::add_entry)).
//! - filtered: the chain position is ignored; the filter expression is
//!   asked for the first entry offset >= (or <=) a needle derived from
//!   the current location. Needles are offset+1/offset-1 to move strictly
//!   past the current entry, or 1/`u64::MAX` for the first/last match.
//!
//! `step` returns `Ok(true)` once `location` is a resolved entry and
//! `Ok(false)` at the end of iteration in that direction. On `Ok(false)`
//! all cursor state is left untouched, so a caller can switch direction
//! and continue from the same entry. Branches that need the entry array
//! report an empty journal as `Err(InvalidOffsetArrayOffset)` instead of
//! `Ok(false)`, because [`JournalFile::entry_list`] returns `None` when
//! `n_entries` is 0.
//!
//! [`Location::Monotonic`], [`Location::Seqnum`] and [`Location::XorHash`]
//! are declared seek keys but resolve through `unimplemented!()`.
//!
//! Consumers (grep-verified): the only in-repo user is `JournalReader`;
//! `file/mod.rs` and lib.rs re-export both types. Near-identical
//! twin: src/crates/jf/journal_file/src/cursor.rs (same code modulo
//! imports and a derived `Default`); netflow-plugin instead queries
//! the published `systemd-journal-sdk-core` crate carrying this same
//! design (src/crates/Cargo.toml).
use super::mmap::MemoryMap;
use crate::error::{JournalError, Result};
use crate::file::{file::JournalFile, filter::FilterExpr, offset_array, offset_array::Direction};
use std::num::NonZeroU64;

/// Where the cursor stands, or where to seek before stepping.
///
/// `Head` and `Tail` are one-past-the-end sentinels: stepping `Forward`
/// from `Head` resolves the first entry and stepping `Backward` from
/// `Tail` the last; the opposite direction has nothing to resolve.
#[derive(Debug, Copy, Clone, PartialEq, Eq)]
pub enum Location {
    /// Sentinel before the first entry; the enum's `Default`.
    Head,
    /// Sentinel after the last entry.
    Tail,
    /// Seek by wall-clock time in microseconds since the epoch, the unit
    /// of `EntryObjectHeader::realtime` ([`JournalFile::duration`]
    /// treats it as `Duration::from_micros`). Resolution snaps to the first entry
    /// whose realtime is at or after the requested time, or to the
    /// newest entry when every entry is older; the step direction is
    /// ignored. The entry-array binary search behind this assumes
    /// realtime does not decrease along the chain, which only holds
    /// while writers pass non-decreasing times (the writer stores the
    /// realtime it is given).
    Realtime(u64),
    /// Seek by monotonic time within the boot identified by the 16-byte
    /// boot id, mirroring `EntryObjectHeader`. Declared but
    /// unimplemented: resolving it panics.
    Monotonic(u64, [u8; 16]),
    /// Seek by entry sequence number, optionally scoped to a sequence
    /// number id. Declared but unimplemented: resolving it panics.
    Seqnum(u64, Option<[u8; 16]>),
    /// Seek by the entry's XOR of item hashes,
    /// `EntryObjectHeader::xor_hash`. Declared but unimplemented:
    /// resolving it panics.
    XorHash(u64),
    /// A resolved entry: the entry object's file offset, as produced by
    /// `step` and read back by `position`. Offsets are meaningful only
    /// for the journal file the cursor was last stepped against.
    ResolvedEntry(NonZeroU64),
}

impl Default for Location {
    fn default() -> Self {
        Self::Head
    }
}

/// Iteration state for one journal file.
#[derive(Debug)]
pub struct JournalCursor {
    /// The resolved entry, or the seek anchor the next `step` resolves.
    pub location: Location,
    /// Optional filter expression. When set, `step` resolves through it
    /// and `array_cursor` is ignored. The expression is stateful: it
    /// remembers its scan position between steps.
    pub filter_expr: Option<FilterExpr>,
    /// Unfiltered-path position in the entry-array chain: the slot
    /// holding `location`'s entry after a resolved step. `set_location`
    /// and `clear_filter` clear it; the filter path never touches it.
    pub array_cursor: Option<offset_array::Cursor>,
}

impl JournalCursor {
    /// Starts at `Head` with no filter and no chain position.
    #[allow(clippy::new_without_default)]
    pub fn new() -> Self {
        Self {
            location: Location::Head,
            filter_expr: None,
            array_cursor: None,
        }
    }

    /// Re-anchors the cursor, discarding the chain position so the next
    /// `step` re-establishes it from the new anchor.
    ///
    /// Note: `Location::ResolvedEntry` here leaves `array_cursor` unset,
    /// so an unfiltered step panics at the `array_cursor.unwrap()` in
    /// `resolve_array_cursor`. With a filter set the filter path resolves
    /// from the offset directly, and `position` works.
    pub fn set_location(&mut self, location: Location) {
        self.location = location;
        self.array_cursor = None;
    }

    /// Installs a filter expression without moving the cursor: the next
    /// `step` continues from the current position under the new filter,
    /// because the filter path reads `location` only. That is what lets
    /// [`JournalReader::step`](crate::file::JournalReader::step) add
    /// matches mid-iteration.
    pub fn set_filter(&mut self, filter_expr: FilterExpr) {
        self.filter_expr = Some(filter_expr);
        // TODO: decide whether the cursor state should be reset instead
        // of keeping the current position as the stepping anchor.
    }

    /// Drops the filter and rewinds to `Head`, clearing the chain
    /// position - unlike `set_filter`, this moves the cursor.
    /// [`JournalReader::flush_matches`](crate::file::JournalReader::flush_matches)
    /// resets through it.
    pub fn clear_filter(&mut self) {
        self.filter_expr = None;
        self.array_cursor = None;
        self.set_location(Location::Head);
    }

    /// Advance to the next entry in `direction`.
    ///
    /// Returns `Ok(true)` when an entry was resolved - `position` then
    /// returns its entry-object offset - and `Ok(false)` at the end of
    /// iteration in that direction, leaving all cursor state untouched
    /// so the direction can be switched. `Err` covers format and
    /// mmap-layer failures raised while reading the file.
    pub fn step<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        direction: Direction,
    ) -> Result<bool> {
        let new_location = if self.filter_expr.is_some() {
            self.resolve_filter_location(journal_file, direction)?
        } else {
            self.resolve_array_cursor(journal_file, direction)?
        };

        if let Some(location) = new_location {
            self.location = location;
            Ok(true)
        } else {
            Ok(false)
        }
    }

    /// The current entry's object offset, to be resolved against the
    /// file with `JournalFile::entry_ref`. Errors with `UnsetCursor`
    /// unless `location` is a resolved entry - set by `step`, or
    /// directly through `set_location`.
    pub fn position(&self) -> Result<NonZeroU64> {
        match self.location {
            Location::ResolvedEntry(entry_offset) => Ok(entry_offset),
            _ => Err(JournalError::UnsetCursor),
        }
    }

    /// Resolve the next location on the entry-array chain (no filter).
    ///
    /// Stepping from `ResolvedEntry` requires the chain position of a
    /// previous step: a location set directly through `set_location` has
    /// none, and the `array_cursor.unwrap()` below panics. An unset
    /// (zero) chain slot reads back as `Ok(None)` from `Cursor::value`
    /// (`OffsetArrayObject::get`) and ends iteration; `Head`+`Backward` and
    /// `Tail`+`Forward` have nothing to resolve.
    fn resolve_array_cursor<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        direction: Direction,
    ) -> Result<Option<Location>> {
        let new_location = match (self.location, direction) {
            (Location::Head, Direction::Forward) => {
                let entry_list = journal_file
                    .entry_list()
                    .ok_or(JournalError::InvalidOffsetArrayOffset)?;

                let cursor = entry_list.cursor_head();
                if let Some(offset) = cursor.value(journal_file)? {
                    self.array_cursor = Some(cursor);
                    Some(Location::ResolvedEntry(offset))
                } else {
                    None
                }
            }
            (Location::Head, Direction::Backward) => None,
            (Location::Tail, Direction::Forward) => None,
            (Location::Tail, Direction::Backward) => {
                let entry_list = journal_file
                    .entry_list()
                    .ok_or(JournalError::InvalidOffsetArrayOffset)?;

                let cursor = entry_list.cursor_tail(journal_file)?;
                if let Some(offset) = cursor.value(journal_file)? {
                    self.array_cursor = Some(cursor);
                    Some(Location::ResolvedEntry(offset))
                } else {
                    None
                }
            }
            (Location::Realtime(realtime), _) => {
                let entry_list = journal_file
                    .entry_list()
                    .ok_or(JournalError::InvalidOffsetArrayOffset)?;

                // First entry whose realtime is at or after the requested
                // time; the step direction is ignored.
                let predicate = |entry_offset| {
                    let entry_object = journal_file.entry_ref(entry_offset)?;
                    Ok(entry_object.header.realtime < realtime)
                };

                // Every entry older: land on the newest entry
                // (`cursor_tail` walks the chain, hence the nested Result).
                let cursor = entry_list
                    .directed_partition_point(journal_file, predicate, Direction::Forward)?
                    .map(Ok)
                    .unwrap_or_else(|| entry_list.cursor_tail(journal_file))?;

                if let Some(offset) = cursor.value(journal_file)? {
                    self.array_cursor = Some(cursor);
                    Some(Location::ResolvedEntry(offset))
                } else {
                    None
                }
            }
            (Location::ResolvedEntry(_), Direction::Forward) => {
                let Some(cursor) = self.array_cursor.unwrap().next(journal_file)? else {
                    return Ok(None);
                };

                if let Some(offset) = cursor.value(journal_file)? {
                    self.array_cursor = Some(cursor);
                    Some(Location::ResolvedEntry(offset))
                } else {
                    None
                }
            }
            (Location::ResolvedEntry(_), Direction::Backward) => {
                let Some(cursor) = self.array_cursor.unwrap().previous(journal_file)? else {
                    return Ok(None);
                };

                if let Some(offset) = cursor.value(journal_file)? {
                    self.array_cursor = Some(cursor);
                    Some(Location::ResolvedEntry(offset))
                } else {
                    None
                }
            }
            // Monotonic/Seqnum/XorHash locations are not implemented.
            _ => {
                unimplemented!()
            }
        };

        Ok(new_location)
    }

    /// Resolve the next location through the filter expression. Only
    /// called with a filter set (`step` dispatches on that), so the
    /// `unwrap` below cannot panic.
    ///
    /// `FilterExpr::next`/`previous` continue from the expression's
    /// internal scan position and clamp to offsets >= / <= the needle,
    /// so fresh-resolution branches rewind with `head()`/`tail()` first;
    /// without the rewind, `next`/`previous` would return the
    /// already-current entry. Needles 1/`u64::MAX` select the first/last
    /// match overall; the +1/-1 needles at the `ResolvedEntry` arms move
    /// strictly past the current entry, relying on entry offsets
    /// strictly increasing along the chain. A filter that built to
    /// [`FilterExpr::None`] - no match survived construction - matches
    /// nothing: every step reports the end of iteration.
    fn resolve_filter_location<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        direction: Direction,
    ) -> Result<Option<Location>> {
        let filter_expr = self.filter_expr.as_mut().unwrap();

        let resolved_location = match (self.location, direction) {
            (Location::Head, Direction::Forward) => filter_expr
                .head()
                .next(journal_file, NonZeroU64::MIN)?
                .map(Location::ResolvedEntry),
            (Location::Head, Direction::Backward) => None,
            (Location::Tail, Direction::Forward) => None,
            (Location::Tail, Direction::Backward) => filter_expr
                .tail(journal_file)?
                .previous(journal_file, NonZeroU64::MAX)?
                .map(Location::ResolvedEntry),
            (Location::Realtime(realtime), direction) => {
                let entry_list = journal_file
                    .entry_list()
                    .ok_or(JournalError::InvalidOffsetArrayOffset)?;

                // Same entry-array anchor as the unfiltered path, always
                // searching forward; the filter then picks the first
                // match at/after (forward) or at/before (backward) that
                // entry.
                let predicate = |entry_offset| {
                    let entry_object = journal_file.entry_ref(entry_offset)?;
                    Ok(entry_object.header.realtime < realtime)
                };

                let cursor = entry_list
                    .directed_partition_point(journal_file, predicate, Direction::Forward)?
                    .map(Ok)
                    .unwrap_or_else(|| entry_list.cursor_tail(journal_file))?;

                if let Some(entry_offset) = cursor.value(journal_file)? {
                    match direction {
                        Direction::Forward => filter_expr
                            .head()
                            .next(journal_file, entry_offset)?
                            .map(Location::ResolvedEntry),
                        Direction::Backward => filter_expr
                            .tail(journal_file)?
                            .previous(journal_file, entry_offset)?
                            .map(Location::ResolvedEntry),
                    }
                } else {
                    None
                }
            }
            (Location::ResolvedEntry(location_offset), Direction::Forward) => filter_expr
                .next(journal_file, location_offset.saturating_add(1))?
                .map(Location::ResolvedEntry),
            (Location::ResolvedEntry(location_offset), Direction::Backward) => {
                // No entry offset is lower than 1: past the first entry.
                if let Some(needle_offset) = NonZeroU64::new(location_offset.get() - 1) {
                    filter_expr
                        .previous(journal_file, needle_offset)?
                        .map(Location::ResolvedEntry)
                } else {
                    None
                }
            }
            // Monotonic/Seqnum/XorHash locations are not implemented.
            _ => {
                unimplemented!();
            }
        };

        Ok(resolved_location)
    }
}
