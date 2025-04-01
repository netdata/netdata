use error::Result;
use object_file::{
    offset_array, DataObject, EntryDataIterator, FieldDataIterator, FieldIterator, FieldObject,
    ObjectFile, ValueGuard,
};
use window_manager::MemoryMap;

pub mod journal_filter;
use journal_filter::{JournalFilter, LogicalOp};
pub mod journal_cursor;
use journal_cursor::JournalCursor;
pub use journal_cursor::Location;

pub use offset_array::Direction;
use tracing::{error, instrument};

pub struct JournalReader<'a, M: MemoryMap> {
    cursor: JournalCursor<'a, M>,

    filter: Option<JournalFilter>,
    field_iterator: Option<FieldIterator<'a, M>>,
    field_data_iterator: Option<FieldDataIterator<'a, M>>,
    entry_data_iterator: Option<EntryDataIterator<'a, M>>,

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
            entry_data_iterator: None,
            field_guard: None,
            data_guard: None,
        }
    }
}

impl<'a, M: MemoryMap> JournalReader<'a, M> {
    pub fn set_location(&mut self, location: Location) {
        self.cursor.set_location(location)
    }

    pub fn step(&mut self, object_file: &'a ObjectFile<M>, direction: Direction) -> Result<bool> {
        self.drop_guards();

        if let Some(filter) = self.filter.as_mut() {
            let filter_expr = filter.build(object_file)?;
            self.cursor.set_filter(filter_expr);
            self.filter = None;
        }

        self.cursor.step(object_file, direction)
    }

    pub fn add_match(&mut self, data: &[u8]) {
        self.filter.get_or_insert_default().add_match(data);
    }

    pub fn add_conjunction(&mut self, object_file: &'a ObjectFile<M>) -> Result<()> {
        self.filter
            .get_or_insert_default()
            .set_operation(object_file, LogicalOp::Conjunction)
    }

    pub fn add_disjunction(&mut self, object_file: &'a ObjectFile<M>) -> Result<()> {
        self.filter
            .get_or_insert_default()
            .set_operation(object_file, LogicalOp::Disjunction)
    }

    pub fn flush_matches(&mut self) {
        self.cursor.clear_filter();
        self.filter = None;
    }

    pub fn get_realtime_usec(&self, object_file: &'a ObjectFile<M>) -> Result<u64> {
        let entry_offset = self.cursor.position()?;
        let entry_object = object_file.entry_object(entry_offset)?;
        Ok(entry_object.header.realtime)
    }

    pub fn get_seqnum(&self, object_file: &'a ObjectFile<M>) -> Result<(u64, [u8; 16])> {
        let entry_offset = self.cursor.position()?;
        let entry_object = object_file.entry_object(entry_offset)?;
        Ok((entry_object.header.seqnum, entry_object.header.boot_id))
    }

    pub fn get_entry_offset(&self) -> Result<u64> {
        self.cursor.position()
    }

    fn drop_guards(&mut self) {
        if let Some(field_guard) = self.field_guard.take() {
            drop(field_guard);
        }

        if let Some(data_guard) = self.data_guard.take() {
            drop(data_guard);
        }
    }

    pub fn fields_restart(&mut self) {
        self.drop_guards();
        self.field_iterator = None;
    }

    pub fn fields_enumerate(
        &mut self,
        object_file: &'a ObjectFile<M>,
    ) -> Result<Option<&ValueGuard<FieldObject<&'a [u8]>>>> {
        self.drop_guards();

        if self.field_iterator.is_none() {
            self.field_iterator = Some(object_file.fields());
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
        object_file: &'a ObjectFile<M>,
        field_name: &'a [u8],
    ) -> Result<()> {
        self.drop_guards();

        self.field_data_iterator = Some(object_file.field_data_objects(field_name)?);
        Ok(())
    }

    pub fn field_data_restart(&mut self) {
        self.drop_guards();
    }

    pub fn field_data_enumerate(
        &mut self,
        _: &'a ObjectFile<M>,
    ) -> Result<Option<&ValueGuard<DataObject<&'a [u8]>>>> {
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
        self.entry_data_iterator = None;
    }

    pub fn entry_data_enumerate(
        &mut self,
        object_file: &'a ObjectFile<M>,
    ) -> Result<Option<&ValueGuard<DataObject<&'a [u8]>>>> {
        self.drop_guards();

        if self.entry_data_iterator.is_none() {
            let entry_offset = self.cursor.position()?;
            self.entry_data_iterator = Some(object_file.entry_data_objects(entry_offset)?);
        }

        if let Some(iter) = &mut self.entry_data_iterator {
            self.data_guard = iter.next().transpose()?;
            Ok(self.data_guard.as_ref())
        } else {
            Ok(None)
        }
    }

    #[instrument(skip_all)]
    pub fn log(&mut self, _object_file: &'a ObjectFile<M>, filename: &str) {
        error!(
            filename = filename,
            location = format!("{:?}", self.cursor.location),
            filter_expr = format!("{:?}", self.cursor.filter_expr),
        );
    }
}
