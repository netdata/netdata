use crate::{
    cursor::{JournalCursor, Location},
    file::{EntryDataIterator, FieldDataIterator, FieldIterator, JournalFile},
    filter::{JournalFilter, LogicalOp},
    object::{DataObject, FieldObject},
    offset_array::Direction,
    value_guard::ValueGuard,
};
use error::Result;
use std::num::NonZeroU64;
use window_manager::MemoryMap;

/// Reads entries from a [`JournalFile`].
///
/// Iteration state lives in a [`JournalCursor`]: [`Self::set_location`]
/// seeks, [`Self::step`] advances and compiles matches queued through
/// [`Self::add_match`], [`Self::add_conjunction`], and
/// [`Self::add_disjunction`] into the cursor's filter. Fields, field data,
/// and entry data are enumerated through independent iterators, each with
/// its own `*_restart` method.
pub struct JournalReader<'a, M: MemoryMap> {
    cursor: JournalCursor,

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
            // cursor deliberately left out of the debug output
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
    /// Render the cursor's active filter expression, for debugging; a
    /// placeholder string when no filter is set.
    pub fn dump(&self, journal_file: &'a JournalFile<M>) -> Result<String> {
        if let Some(filter_expr) = self.cursor.filter_expr.as_ref() {
            filter_expr.dump(journal_file)
        } else {
            Ok(String::from("no filter expr"))
        }
    }

    /// Seek; the location is resolved to an entry by the next [`Self::step`].
    ///
    /// Only [`Location::Head`], [`Location::Tail`], [`Location::Realtime`],
    /// and [`Location::ResolvedEntry`] are supported by `step`; the other
    /// variants panic there. A sought [`Location::ResolvedEntry`] also
    /// needs an active filter; without one, `step` panics.
    pub fn set_location(&mut self, location: Location) {
        self.cursor.set_location(location)
    }

    /// Advance to the next entry in `direction`, honoring the active filter.
    ///
    /// Matches queued since the last call are compiled into the cursor's
    /// filter first, so they take effect from this step on. Returns `true`
    /// when the cursor landed on an entry and `false` when iteration is
    /// exhausted in that direction.
    pub fn step(&mut self, journal_file: &'a JournalFile<M>, direction: Direction) -> Result<bool> {
        self.drop_guards();

        if let Some(filter) = self.filter.as_mut() {
            let filter_expr = filter.build(journal_file)?;
            self.cursor.set_filter(filter_expr);
            self.filter = None;
        }

        self.cursor.step(journal_file, direction)
    }

    /// Queue a `FIELD=value` match; it takes effect at the next
    /// [`Self::step`]. Bytes without an `=` are ignored.
    pub fn add_match(&mut self, data: &[u8]) {
        self.filter.get_or_insert_default().add_match(data);
    }

    /// Fold the matches queued so far into the filter and mark the next
    /// group to be joined by conjunction; takes effect at the next
    /// [`Self::step`].
    pub fn add_conjunction(&mut self, journal_file: &'a JournalFile<M>) -> Result<()> {
        self.filter
            .get_or_insert_default()
            .set_operation(journal_file, LogicalOp::Conjunction)
    }

    /// Fold the matches queued so far into the filter and mark the next
    /// group to be joined by disjunction; takes effect at the next
    /// [`Self::step`].
    pub fn add_disjunction(&mut self, journal_file: &'a JournalFile<M>) -> Result<()> {
        self.filter
            .get_or_insert_default()
            .set_operation(journal_file, LogicalOp::Disjunction)
    }

    /// Discard queued matches and the cursor's active filter, rewinding to
    /// [`Location::Head`].
    pub fn flush_matches(&mut self) {
        self.cursor.clear_filter();
        self.filter = None;
    }

    /// Realtime timestamp of the entry the cursor is parked on, in
    /// microseconds; fails when the cursor is unset.
    pub fn get_realtime_usec(&self, journal_file: &'a JournalFile<M>) -> Result<u64> {
        let entry_offset = self.cursor.position()?;
        let entry_object = journal_file.entry_ref(entry_offset)?;
        Ok(entry_object.header.realtime)
    }

    /// Sequence number of the entry the cursor is parked on, together with
    /// the file's sequence number ID; fails when the cursor is unset.
    pub fn get_seqnum(&self, journal_file: &'a JournalFile<M>) -> Result<(u64, [u8; 16])> {
        let entry_offset = self.cursor.position()?;
        let entry_object = journal_file.entry_ref(entry_offset)?;
        Ok((
            entry_object.header.seqnum,
            journal_file.journal_header_ref().seqnum_id,
        ))
    }

    /// Entry offset the cursor is parked on; fails when the cursor is unset.
    pub fn get_entry_offset(&self) -> Result<NonZeroU64> {
        self.cursor.position()
    }

    /// Drop the guards of the most recently returned objects. New objects
    /// must not be created while a guard is alive: objects share a limited
    /// set of memory-mapped windows, and creating one can invalidate the
    /// memory a previously returned guard points at.
    fn drop_guards(&mut self) {
        self.field_guard.take();
        self.data_guard.take();
    }

    /// Restart field enumeration: the next [`Self::fields_enumerate`] starts
    /// from the first field again.
    pub fn fields_restart(&mut self) {
        self.drop_guards();
        self.field_iterator = None;
    }

    /// Return the next field, or `None` when fields are exhausted. The
    /// iterator is created on the first call and reused until
    /// [`Self::fields_restart`].
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

    /// Replace the field-data iterator with one over all data objects of
    /// `field_name`; a field that does not exist enumerates as empty.
    pub fn field_data_query_unique(
        &mut self,
        journal_file: &'a JournalFile<M>,
        field_name: &'a [u8],
    ) -> Result<()> {
        self.drop_guards();

        self.field_data_iterator = Some(journal_file.field_data_objects(field_name)?);
        Ok(())
    }

    /// Drop the current data guard. Unlike [`Self::fields_restart`] and
    /// [`Self::entry_data_restart`], the field-data iterator is not reset:
    /// [`Self::field_data_enumerate`] continues where it left off.
    pub fn field_data_restart(&mut self) {
        self.drop_guards();
    }

    /// Return the next data object of the field selected by
    /// [`Self::field_data_query_unique`], or `None` when exhausted.
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

    /// Restart entry-data enumeration: the next
    /// [`Self::entry_data_enumerate`] re-creates the iterator for the entry
    /// the cursor is parked on.
    pub fn entry_data_restart(&mut self) {
        self.drop_guards();
        self.entry_data_iterator = None;
    }

    /// Return the next data object of the entry being enumerated, or `None`
    /// when exhausted.
    ///
    /// The iterator is pinned to the entry the cursor was parked on when it
    /// was created; [`Self::step`] does not reset it, so call
    /// [`Self::entry_data_restart`] after moving the cursor to enumerate the
    /// new entry.
    pub fn entry_data_enumerate(
        &mut self,
        journal_file: &'a JournalFile<M>,
    ) -> Result<Option<&ValueGuard<'_, DataObject<&'a [u8]>>>> {
        self.drop_guards();

        if self.entry_data_iterator.is_none() {
            let entry_offset = self.cursor.position()?;
            self.entry_data_iterator = Some(journal_file.entry_data_objects(entry_offset)?);
        }

        if let Some(iter) = &mut self.entry_data_iterator {
            self.data_guard = iter.next().transpose()?;
            Ok(self.data_guard.as_ref())
        } else {
            Ok(None)
        }
    }
}
