use crate::{
    cursor::{JournalCursor, Location},
    file::{FieldDataIterator, FieldIterator, JournalFile},
    filter::{FilterExpr, JournalFilter, LogicalOp},
    object::{DataObject, FieldObject},
    offset_array::Direction,
    value_guard::ValueGuard,
};
use error::Result;
use std::num::NonZeroU64;
use window_manager::MemoryMap;

pub struct JournalReader<'a, M: MemoryMap> {
    cursor: JournalCursor,

    filter: Option<JournalFilter>,
    field_iterator: Option<FieldIterator<'a, M>>,
    field_data_iterator: Option<FieldDataIterator<'a, M>>,
    entry_data_offsets: Vec<NonZeroU64>,
    entry_data_index: usize,
    entry_data_ready: bool,

    field_guard: Option<ValueGuard<'a, FieldObject<&'a [u8]>>>,
    data_guard: Option<ValueGuard<'a, DataObject<&'a [u8]>>>,
}

impl<M: MemoryMap> std::fmt::Debug for JournalReader<'_, M> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("JournalReader")
            // .field("cursor", &self.cursor)
            .field("field_guard", &self.field_guard)
            .field("data_guard", &self.data_guard)
            .finish()
    }
}

impl<M: MemoryMap> Default for JournalReader<'_, M> {
    fn default() -> Self {
        Self {
            cursor: JournalCursor::new(),
            filter: None,
            field_iterator: None,
            field_data_iterator: None,
            entry_data_offsets: Vec::new(),
            entry_data_index: 0,
            entry_data_ready: false,
            field_guard: None,
            data_guard: None,
        }
    }
}

impl<'a, M: MemoryMap> JournalReader<'a, M> {
    pub fn dump(&self, journal_file: &'a JournalFile<M>) -> Result<String> {
        if let Some(filter_expr) = self.cursor.filter_expr.as_ref() {
            filter_expr.dump(journal_file)
        } else {
            Ok(String::from("no filter expr"))
        }
    }

    pub fn set_location(&mut self, location: Location) {
        self.cursor.set_location(location)
    }

    pub fn step(&mut self, journal_file: &'a JournalFile<M>, direction: Direction) -> Result<bool> {
        self.drop_guards();

        if let Some(filter) = self.filter.as_mut() {
            let filter_expr = filter.build(journal_file)?;
            self.cursor.set_filter(filter_expr);
            self.filter = None;
        }

        self.cursor.step(journal_file, direction)
    }

    /// Build the pending filter expression (if any) and return it.
    ///
    /// This consumes the unresolved filter state so callers that drive
    /// [`JournalCursor`] directly can resolve per-file matches once and
    /// reuse the resulting expression.
    pub fn build_filter(&mut self, journal_file: &JournalFile<M>) -> Result<Option<FilterExpr>> {
        if let Some(filter) = self.filter.as_mut() {
            let expr = filter.build(journal_file)?;
            self.cursor.set_filter(expr.clone());
            self.filter = None;
            Ok(Some(expr))
        } else {
            Ok(None)
        }
    }

    pub fn add_match(&mut self, data: &[u8]) {
        self.filter.get_or_insert_default().add_match(data);
    }

    pub fn add_conjunction(&mut self, journal_file: &'a JournalFile<M>) -> Result<()> {
        self.filter
            .get_or_insert_default()
            .set_operation(journal_file, LogicalOp::Conjunction)
    }

    pub fn add_disjunction(&mut self, journal_file: &'a JournalFile<M>) -> Result<()> {
        self.filter
            .get_or_insert_default()
            .set_operation(journal_file, LogicalOp::Disjunction)
    }

    pub fn flush_matches(&mut self) {
        self.cursor.clear_filter();
        self.filter = None;
    }

    pub fn get_realtime_usec(&self, journal_file: &'a JournalFile<M>) -> Result<u64> {
        let entry_offset = self.cursor.position()?;
        let entry_object = journal_file.entry_ref(entry_offset)?;
        Ok(entry_object.header.realtime)
    }

    pub fn get_seqnum(&self, journal_file: &'a JournalFile<M>) -> Result<(u64, [u8; 16])> {
        let entry_offset = self.cursor.position()?;
        let entry_object = journal_file.entry_ref(entry_offset)?;
        Ok((
            entry_object.header.seqnum,
            journal_file.journal_header_ref().seqnum_id,
        ))
    }

    pub fn get_entry_offset(&self) -> Result<NonZeroU64> {
        self.cursor.position()
    }

    fn drop_guards(&mut self) {
        self.field_guard.take();
        self.data_guard.take();
    }

    pub fn fields_restart(&mut self) {
        self.drop_guards();
        self.field_iterator = None;
    }

    pub fn fields_enumerate(
        &mut self,
        journal_file: &'a JournalFile<M>,
    ) -> Result<Option<&ValueGuard<'_, FieldObject<&'a [u8]>>>> {
        self.drop_guards();

        if self.field_iterator.is_none() {
            self.field_iterator = Some(journal_file.fields());
        }

        if let Some(iter) = &mut self.field_iterator {
            self.field_guard = iter.next().transpose()?;
            Ok(self.field_guard.as_ref())
        } else {
            Ok(None)
        }
    }

    pub fn field_data_query_unique(
        &mut self,
        journal_file: &'a JournalFile<M>,
        field_name: &'a [u8],
    ) -> Result<()> {
        self.drop_guards();

        self.field_data_iterator = Some(journal_file.field_data_objects(field_name)?);
        Ok(())
    }

    pub fn field_data_restart(&mut self) {
        self.drop_guards();
    }

    pub fn field_data_enumerate(
        &mut self,
        _: &'a JournalFile<M>,
    ) -> Result<Option<&ValueGuard<'_, DataObject<&'a [u8]>>>> {
        self.drop_guards();

        if let Some(iter) = &mut self.field_data_iterator {
            self.data_guard = iter.next().transpose()?;
            Ok(self.data_guard.as_ref())
        } else {
            Ok(None)
        }
    }

    pub fn entry_data_restart(&mut self) {
        self.drop_guards();
        self.entry_data_offsets.clear();
        self.entry_data_index = 0;
        self.entry_data_ready = false;
    }

    pub fn entry_data_enumerate(
        &mut self,
        journal_file: &'a JournalFile<M>,
    ) -> Result<Option<&ValueGuard<'_, DataObject<&'a [u8]>>>> {
        self.drop_guards();

        if !self.entry_data_ready {
            let entry_offset = self.cursor.position()?;
            self.entry_data_offsets.clear();
            journal_file.entry_data_object_offsets(entry_offset, &mut self.entry_data_offsets)?;
            self.entry_data_index = 0;
            self.entry_data_ready = true;
        }

        if self.entry_data_index >= self.entry_data_offsets.len() {
            return Ok(None);
        }

        let data_offset = self.entry_data_offsets[self.entry_data_index];
        self.entry_data_index += 1;

        self.data_guard = Some(journal_file.data_ref(data_offset)?);
        Ok(self.data_guard.as_ref())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{
        Direction, JournalFile, JournalFileOptions, JournalReader, JournalWriter, Location, Mmap,
    };
    use tempfile::NamedTempFile;

    fn test_uuid(seed: u8) -> [u8; 16] {
        [seed; 16]
    }

    #[test]
    fn build_filter_keeps_cursor_filter_in_sync() -> Result<()> {
        let temp_file = NamedTempFile::new().map_err(error::JournalError::Io)?;
        let journal_path = temp_file.path();
        let boot_id = test_uuid(9);

        {
            let options =
                JournalFileOptions::new(test_uuid(1), test_uuid(2), test_uuid(3), test_uuid(4));
            let mut journal_file = JournalFile::create(&journal_path, options)?;
            let mut writer = JournalWriter::new(&mut journal_file)?;
            writer.add_entry(
                &mut journal_file,
                &[
                    b"MESSAGE=first".as_slice(),
                    b"_SYSTEMD_UNIT=first.service".as_slice(),
                ],
                1_000_000,
                500_000,
                boot_id,
            )?;
            writer.add_entry(
                &mut journal_file,
                &[
                    b"MESSAGE=second".as_slice(),
                    b"_SYSTEMD_UNIT=second.service".as_slice(),
                ],
                2_000_000,
                600_000,
                boot_id,
            )?;
        }

        let journal_file = JournalFile::<Mmap>::open(&journal_path, 8 * 1024)?;
        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);
        reader.add_match(b"MESSAGE=first");
        assert!(reader.step(&journal_file, Direction::Forward)?);
        assert_eq!(reader.get_realtime_usec(&journal_file)?, 1_000_000);

        reader.set_location(Location::Head);
        reader.add_match(b"MESSAGE=second");
        let built = reader
            .build_filter(&journal_file)?
            .expect("expected a rebuilt filter expression");
        assert!(matches!(built, crate::FilterExpr::Match(..)));
        assert!(reader.step(&journal_file, Direction::Forward)?);
        assert_eq!(reader.get_realtime_usec(&journal_file)?, 2_000_000);

        Ok(())
    }
}
