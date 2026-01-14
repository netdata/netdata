use crate::file::{file::JournalFile, filter::FilterExpr, offset_array, offset_array::Direction};
use crate::error::{JournalError, Result};
use std::num::NonZeroU64;
use super::mmap::MemoryMap;

#[derive(Debug, Copy, Clone, PartialEq, Eq)]
pub enum Location {
    Head,
    Tail,
    Realtime(u64),
    Monotonic(u64, [u8; 16]),
    Seqnum(u64, Option<[u8; 16]>),
    XorHash(u64),
    ResolvedEntry(NonZeroU64),
}

impl Default for Location {
    fn default() -> Self {
        Self::Head
    }
}

#[derive(Debug)]
pub struct JournalCursor {
    pub location: Location,
    pub filter_expr: Option<FilterExpr>,
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

    pub fn set_location(&mut self, location: Location) {
        self.location = location;
        self.array_cursor = None;
    }

    pub fn set_filter(&mut self, filter_expr: FilterExpr) {
        self.filter_expr = Some(filter_expr);
        // FIXME: should we set cursor to None?
    }

    pub fn clear_filter(&mut self) {
        self.filter_expr = None;
        self.array_cursor = None;
        self.set_location(Location::Head);
    }

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
