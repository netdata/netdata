use crate::{file::JournalFile, filter::FilterExpr, offset_array, offset_array::Direction};
use error::{JournalError, Result};
use std::num::NonZeroU64;
use window_manager::MemoryMap;

/// Where a [`JournalCursor`] sits, or a seek target waiting to be resolved.
///
/// [`Location::Head`] and [`Location::Tail`] are virtual positions before the
/// first and after the last entry. `Realtime`, `Monotonic`, `Seqnum` and
/// `XorHash` carry seek keys: [`JournalCursor::step`] resolves `Realtime`
/// against the entry list and panics on the other three. Only
/// [`Location::ResolvedEntry`] is a real position; [`JournalCursor::position`]
/// fails for every other variant.
#[derive(Debug, Copy, Clone, PartialEq, Eq, Default)]
pub enum Location {
    #[default]
    Head,
    Tail,
    Realtime(u64),
    Monotonic(u64, [u8; 16]),
    Seqnum(u64, Option<[u8; 16]>),
    XorHash(u64),
    ResolvedEntry(NonZeroU64),
}

/// Iteration state over a journal file: where the cursor sits, which filter
/// applies, and its position in the entry offset-array chain.
#[derive(Debug)]
pub struct JournalCursor {
    /// Current position, or a pending seek target.
    pub location: Location,
    /// When set, [`JournalCursor::step`] visits only entries matching this
    /// expression.
    pub filter_expr: Option<FilterExpr>,
    /// Position in the entry offset-array chain, used to continue unfiltered
    /// iteration from [`Location::ResolvedEntry`]; reset by `set_location` and
    /// `clear_filter`.
    pub array_cursor: Option<offset_array::Cursor>,
}

impl JournalCursor {
    #[allow(clippy::new_without_default)]
    pub fn new() -> Self {
        Self {
            location: Location::Head,
            filter_expr: None,
            array_cursor: None,
        }
    }

    /// Set the location; it is resolved to an entry by the next
    /// [`JournalCursor::step`]. Any array-cursor position is discarded, so
    /// an unfiltered `step` from a directly set [`Location::ResolvedEntry`]
    /// panics unwrapping the missing array cursor; set a filter first.
    pub fn set_location(&mut self, location: Location) {
        self.location = location;
        self.array_cursor = None;
    }

    /// Restrict iteration to entries matching `filter_expr`.
    pub fn set_filter(&mut self, filter_expr: FilterExpr) {
        self.filter_expr = Some(filter_expr);
        // The array cursor is left as-is: the filtered path never reads it,
        // and `clear_filter` resets it.
    }

    /// Drop the filter and rewind to [`Location::Head`].
    pub fn clear_filter(&mut self) {
        self.filter_expr = None;
        self.array_cursor = None;
        self.set_location(Location::Head);
    }

    /// Advance to the next entry in `direction`, honoring the filter when one
    /// is set.
    ///
    /// Returns `true` when the cursor landed on an entry and `false` when
    /// iteration is exhausted in that direction; the location is left
    /// unchanged in the latter case.
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

    /// Entry offset the cursor is parked on.
    ///
    /// Errors with `JournalError::UnsetCursor` unless the location is
    /// [`Location::ResolvedEntry`].
    pub fn position(&self) -> Result<NonZeroU64> {
        match self.location {
            Location::ResolvedEntry(entry_offset) => Ok(entry_offset),
            _ => Err(JournalError::UnsetCursor),
        }
    }

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
                // First entry at or after `realtime`, falling back to the
                // tail entry when every entry is older; the step direction
                // is ignored in this arm.
                let entry_list = journal_file
                    .entry_list()
                    .ok_or(JournalError::InvalidOffsetArrayOffset)?;

                let predicate = |entry_offset| {
                    let entry_object = journal_file.entry_ref(entry_offset)?;
                    Ok(entry_object.header.realtime < realtime)
                };

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
            _ => {
                unimplemented!()
            }
        };

        Ok(new_location)
    }

    fn resolve_filter_location<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        direction: Direction,
    ) -> Result<Option<Location>> {
        // Only reachable with a filter set (see `step`).
        let filter_expr = self.filter_expr.as_mut().unwrap();

        let resolved_location = match (self.location, direction) {
            (Location::Head, Direction::Forward) => filter_expr
                // Rewind each match's entry cursor, then take the first
                // matching entry anywhere in the file.
                .head()
                .next(journal_file, NonZeroU64::MIN)?
                .map(Location::ResolvedEntry),
            (Location::Head, Direction::Backward) => None,
            (Location::Tail, Direction::Forward) => None,
            (Location::Tail, Direction::Backward) => filter_expr
                // Seek each match's entry cursor to its tail, then take the
                // last matching entry anywhere in the file.
                .tail(journal_file)?
                .previous(journal_file, NonZeroU64::MAX)?
                .map(Location::ResolvedEntry),
            (Location::Realtime(realtime), direction) => {
                // Seek to the first entry at or after `realtime` (the tail
                // entry when all are older), then find the first match at or
                // after it, or the last match at or before it, per
                // `direction`.
                let entry_list = journal_file
                    .entry_list()
                    .ok_or(JournalError::InvalidOffsetArrayOffset)?;

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
                // First matching entry strictly after the current one.
                .next(journal_file, location_offset.saturating_add(1))?
                .map(Location::ResolvedEntry),
            (Location::ResolvedEntry(location_offset), Direction::Backward) => {
                // Last matching entry strictly before the current one; none
                // at offset 1, since offsets are nonzero.
                if let Some(needle_offset) = NonZeroU64::new(location_offset.get() - 1) {
                    filter_expr
                        .previous(journal_file, needle_offset)?
                        .map(Location::ResolvedEntry)
                } else {
                    None
                }
            }
            _ => {
                unimplemented!();
            }
        };

        Ok(resolved_location)
    }
}
