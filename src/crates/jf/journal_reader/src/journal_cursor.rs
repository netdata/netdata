use crate::journal_filter::FilterExpr;
use error::{JournalError, Result};
use object_file::{offset_array, offset_array::Direction, ObjectFile};
use window_manager::MemoryMap;

#[derive(Debug, Copy, Clone, PartialEq, Eq)]
pub enum Location {
    Head,
    Tail,
    Realtime(u64),
    Monotonic(u64, [u8; 16]),
    Seqnum(u64, Option<[u8; 16]>),
    XorHash(u64),
    Entry(u64),
}

impl Default for Location {
    fn default() -> Self {
        Self::Head
    }
}

#[derive(Debug)]
pub struct JournalCursor<'a, M: MemoryMap> {
    pub location: Location,
    pub filter_expr: Option<FilterExpr>,
    pub array_cursor: Option<offset_array::Cursor<'a, M>>,
}

impl<'a, M: MemoryMap> JournalCursor<'a, M> {
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

    pub fn step(&mut self, object_file: &'a ObjectFile<M>, direction: Direction) -> Result<bool> {
        let new_location = if self.filter_expr.is_some() {
            self.resolve_filter_location(object_file, direction)?
        } else {
            self.array_cursor = match (self.array_cursor.as_ref(), direction) {
                (Some(cursor), Direction::Forward) => cursor.next()?,
                (Some(cursor), Direction::Backward) => cursor.previous()?,
                (None, _) => self.resolve_array_cursor(object_file, direction)?,
            };

            self.array_cursor
                .as_ref()
                .map(|c| c.position())
                .transpose()?
                .map(Location::Entry)
        };

        if let Some(location) = new_location {
            self.location = location;
            Ok(true)
        } else {
            Ok(false)
        }
    }

    pub fn position(&self) -> Result<u64> {
        match self.location {
            Location::Entry(entry_offset) => Ok(entry_offset),
            _ => Err(JournalError::UnsetCursor),
        }
    }

    fn resolve_array_cursor(
        &self,
        object_file: &'a ObjectFile<M>,
        direction: Direction,
    ) -> Result<Option<offset_array::Cursor<'a, M>>> {
        let entry_list = object_file.entry_list()?;

        match (self.location, direction) {
            (Location::Head, Direction::Forward) => Some(entry_list.cursor_head()).transpose(),
            (Location::Head, Direction::Backward) => Ok(None),
            (Location::Tail, Direction::Forward) => Ok(None),
            (Location::Tail, Direction::Backward) => Some(entry_list.cursor_tail()).transpose(),
            (Location::Realtime(realtime), _) => {
                let predicate = |entry_offset| {
                    let entry_object = object_file.entry_object(entry_offset)?;
                    Ok(entry_object.header.realtime < realtime)
                };
                entry_list.directed_partition_point(predicate, direction)
            }
            (Location::Entry(location_offset), _) => {
                let predicate = |entry_offset| Ok(entry_offset < location_offset);
                entry_list.directed_partition_point(predicate, direction)
            }
            _ => {
                unimplemented!();
            }
        }
    }

    fn resolve_filter_location(
        &self,
        object_file: &ObjectFile<M>,
        direction: Direction,
    ) -> Result<Option<Location>> {
        let resolved_location = match (self.location, direction) {
            (Location::Head, Direction::Forward) => self
                .filter_expr
                .as_ref()
                .unwrap()
                .lookup(object_file, u64::MIN, Direction::Forward)?
                .map(Location::Entry),
            (Location::Head, Direction::Backward) => None,
            (Location::Tail, Direction::Forward) => None,
            (Location::Tail, Direction::Backward) => self
                .filter_expr
                .as_ref()
                .unwrap()
                .lookup(object_file, u64::MAX, Direction::Backward)?
                .map(Location::Entry),
            (Location::Realtime(realtime), _) => {
                let predicate = |entry_offset| {
                    let entry_object = object_file.entry_object(entry_offset)?;
                    Ok(entry_object.header.realtime < realtime)
                };

                let entry_list = object_file.entry_list()?;
                entry_list
                    .directed_partition_point(predicate, direction)?
                    .map(|c| c.position())
                    .transpose()?
                    .map(Location::Entry)
            }
            (Location::Entry(location_offset), Direction::Forward) => self
                .filter_expr
                .as_ref()
                .unwrap()
                .lookup(object_file, location_offset.saturating_add(1), direction)?
                .map(Location::Entry),
            (Location::Entry(location_offset), Direction::Backward) => self
                .filter_expr
                .as_ref()
                .unwrap()
                .lookup(object_file, location_offset.saturating_sub(1), direction)?
                .map(Location::Entry),
            _ => {
                todo!();
            }
        };

        Ok(resolved_location)
    }
}
