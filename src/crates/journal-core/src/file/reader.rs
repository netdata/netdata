use super::mmap::MemoryMap;
use crate::error::Result;
use crate::field_map::{FieldMap, REMAPPING_MARKER, extract_field_name};
use crate::file::{
    EntryItemsType,
    cursor::{JournalCursor, Location},
    file::{EntryDataIterator, FieldDataIterator, FieldIterator, JournalFile},
    filter::{JournalFilter, LogicalOp},
    object::{DataObject, FieldObject, HashableObject},
    offset_array::Direction,
    value_guard::ValueGuard,
};
use std::num::NonZeroU64;

pub struct JournalReader<'a, M: MemoryMap> {
    cursor: JournalCursor,

    filter: Option<JournalFilter>,
    field_iterator: Option<FieldIterator<'a, M>>,
    field_data_iterator: Option<FieldDataIterator<'a, M>>,
    entry_data_iterator: Option<EntryDataIterator<'a, M>>,

    field_guard: Option<ValueGuard<'a, FieldObject<&'a [u8]>>>,
    data_guard: Option<ValueGuard<'a, DataObject<&'a [u8]>>>,

    // Field name remapping support
    remapping_registry: FieldMap,
    translated_payload: Vec<u8>,
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
            remapping_registry: FieldMap::new(),
            translated_payload: Vec::new(),
        }
    }
}

impl<'a, M: MemoryMap> JournalReader<'a, M> {
    pub fn dump(&self, _journal_file: &'a JournalFile<M>) -> Result<String> {
        if let Some(_filter_expr) = self.cursor.filter_expr.as_ref() {
            todo!();
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

    /// Adds a match filter for the given field=value pair.
    ///
    /// If the field name is an original (otel) name that has been remapped,
    /// this automatically translates it to the systemd-compatible name before
    /// applying the filter.
    ///
    /// # Example
    ///
    /// ```no_run
    /// # use journal_core::{JournalReader, JournalFile};
    /// # use journal_core::file::Mmap;
    /// # fn example(reader: &mut JournalReader<Mmap>, file: &JournalFile<Mmap>) {
    /// // Even if "my.field.name" was remapped to "ND_ABC123...", this works:
    /// reader.add_match(b"my.field.name=some_value");
    /// # }
    /// ```
    pub fn add_match(&mut self, data: &[u8]) {
        // Check if the field name needs translation
        if let Some(field_name) = extract_field_name(data) {
            if let Some(systemd_name) = self.remapping_registry.get_systemd_name(field_name) {
                // Field has been remapped - translate the query
                let eq_pos = data.iter().position(|&b| b == b'=').unwrap();
                let value = &data[eq_pos..]; // includes '='

                let mut translated_query = Vec::with_capacity(systemd_name.len() + value.len());
                translated_query.extend_from_slice(systemd_name.as_bytes());
                translated_query.extend_from_slice(value);

                self.filter
                    .get_or_insert_default()
                    .add_match(&translated_query);
                return;
            }
        }

        // No translation needed - use original
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
        self.entry_data_iterator = None;
    }

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

            // Translate field name if needed
            if let Some(data_guard) = &self.data_guard {
                let payload = data_guard.get_payload();

                // Check if this field needs translation
                if let Some(field_name) = extract_field_name(payload) {
                    if field_name.starts_with(b"ND_") {
                        // This looks like a remapped field
                        if let Ok(systemd_name) = std::str::from_utf8(field_name) {
                            if let Some(otel_name) =
                                self.remapping_registry.get_otel_name(systemd_name)
                            {
                                // Translate: build new payload with original field name
                                let eq_pos = payload.iter().position(|&b| b == b'=').unwrap();
                                let value = &payload[eq_pos..]; // includes '='

                                self.translated_payload.clear();
                                self.translated_payload.extend_from_slice(otel_name);
                                self.translated_payload.extend_from_slice(value);
                            } else {
                                // No mapping found - clear buffer
                                self.translated_payload.clear();
                            }
                        } else {
                            // Invalid UTF-8 - clear buffer
                            self.translated_payload.clear();
                        }
                    } else {
                        // Not a remapped field - clear buffer
                        self.translated_payload.clear();
                    }
                } else {
                    // No '=' found - clear buffer
                    self.translated_payload.clear();
                }
            }

            Ok(self.data_guard.as_ref())
        } else {
            Ok(None)
        }
    }

    pub fn entry_data_offsets(
        &self,
        journal_file: &'a JournalFile<M>,
        data_offsets: &mut Vec<NonZeroU64>,
    ) -> Result<()> {
        let entry_offset = self.cursor.position()?;
        let entry_guard = journal_file.entry_ref(entry_offset)?;

        match &entry_guard.items {
            EntryItemsType::Regular(items) => {
                for item in items.iter() {
                    if let Some(offset) = NonZeroU64::new(item.object_offset) {
                        data_offsets.push(offset);
                    }
                }
            }
            EntryItemsType::Compact(items) => {
                for item in items.iter() {
                    if let Some(offset) = NonZeroU64::new(item.object_offset as u64) {
                        data_offsets.push(offset);
                    }
                }
            }
        }

        Ok(())
    }

    /// Loads field name remappings from the journal file.
    ///
    /// This method scans the journal for entries tagged with `ND_REMAPPING=1`,
    /// extracts the field name mappings, and builds the internal remapping registry.
    ///
    /// This is automatically called by convenience wrappers, but can be called
    /// explicitly if needed for manual reader setup.
    ///
    /// # Performance
    ///
    /// This uses the field indexing optimization (O(k) where k = number of mapping entries)
    /// rather than scanning all entries (O(n)).
    pub fn load_remappings(&mut self, journal_file: &'a JournalFile<M>) -> Result<()> {
        // Look up all entries with ND_REMAPPING=1 using field indexing
        let marker_field_name = {
            let marker_str = std::str::from_utf8(REMAPPING_MARKER)
                .map_err(|_| crate::error::JournalError::InvalidField)?;
            let eq_pos = marker_str
                .find('=')
                .ok_or(crate::error::JournalError::InvalidField)?;
            &marker_str.as_bytes()[..eq_pos]
        };

        // Collect entry information first to avoid iterator conflicts
        let mut entry_info: Vec<(Option<NonZeroU64>, Option<(NonZeroU64, u64)>)> = Vec::new();

        {
            // Get iterator for all data objects with this field
            let Ok(mut data_iter) = journal_file.field_data_objects(marker_field_name) else {
                // No remapping entries found - this is fine
                return Ok(());
            };

            // Process each data object (should all have value "1")
            while let Some(data_guard) = data_iter.next().transpose()? {
                // Get all entries that contain this data object
                let n_entries = data_guard.header.n_entries;

                if let Some(entry_count) = n_entries {
                    match entry_count.get() {
                        0 => {
                            // Should not happen
                            continue;
                        }
                        1 => {
                            // Single entry - stored directly
                            if let Some(entry_offset) = data_guard.header.entry_offset {
                                entry_info.push((Some(entry_offset), None));
                            }
                        }
                        n => {
                            // Multiple entries - first is inlined, rest in entry array
                            // Process the first entry (inlined)
                            if let Some(entry_offset) = data_guard.header.entry_offset {
                                entry_info.push((Some(entry_offset), None));
                            }
                            // Process remaining entries from array
                            if let Some(array_offset) = data_guard.header.entry_array_offset {
                                entry_info.push((None, Some((array_offset, n))));
                            }
                        }
                    }
                }
            }
        }

        // Now parse the collected entries
        for (single_entry, array_entry) in entry_info {
            if let Some(entry_offset) = single_entry {
                self.parse_remapping_entry(journal_file, entry_offset)?;
            } else if let Some((array_offset, n_entries)) = array_entry {
                self.parse_remapping_entries_from_array(journal_file, array_offset, n_entries)?;
            }
        }

        Ok(())
    }

    fn parse_remapping_entry(
        &mut self,
        journal_file: &'a JournalFile<M>,
        entry_offset: NonZeroU64,
    ) -> Result<()> {
        // Collect all payloads first to avoid guard lifetime issues
        let mut payloads: Vec<Vec<u8>> = Vec::new();

        {
            let data_iter = journal_file.entry_data_objects(entry_offset)?;
            for data_result in data_iter {
                let data_guard = data_result?;
                let payload = data_guard.get_payload();
                payloads.push(payload.to_vec());
            }
        }

        // Now parse the collected payloads
        for payload in payloads {
            // Skip the marker field itself
            if payload == REMAPPING_MARKER {
                continue;
            }

            // Parse ND_<md5>=<original_name>
            if let Some(field_name) = extract_field_name(&payload) {
                if field_name.starts_with(b"ND_") && field_name.len() == 35 {
                    // This is a remapping field
                    let eq_pos = payload.iter().position(|&b| b == b'=').unwrap();
                    let systemd_name = std::str::from_utf8(field_name)
                        .map_err(|_| crate::error::JournalError::InvalidField)?
                        .to_string();
                    let otel_name = payload[eq_pos + 1..].to_vec();

                    self.remapping_registry
                        .add_otel_mapping(otel_name, systemd_name);
                }
            }
        }

        Ok(())
    }

    fn parse_remapping_entries_from_array(
        &mut self,
        journal_file: &'a JournalFile<M>,
        array_offset: NonZeroU64,
        n_entries: u64,
    ) -> Result<()> {
        // Get all entry offsets from the array chain
        let mut entry_offsets = Vec::new();

        // Create list and collect all offsets
        // n_entries includes the inlined entry (count=1) plus array entries (count=n-1)
        // So the array has n-1 entries
        let array_count = n_entries.saturating_sub(1);

        if let Some(total_items_nz) = std::num::NonZeroUsize::new(array_count as usize) {
            use crate::file::offset_array::List;
            let list = List::new(array_offset, total_items_nz);
            list.collect_offsets(journal_file, &mut entry_offsets)?;
        }

        // Parse each remapping entry
        for entry_offset in entry_offsets {
            self.parse_remapping_entry(journal_file, entry_offset)?;
        }

        Ok(())
    }

    /// Gets the current entry data payload, translating remapped field names if applicable.
    ///
    /// This should be called after `entry_data_enumerate()` to get the translated version
    /// of the field name. If the field name doesn't need translation, returns the original.
    pub fn get_entry_data_payload(&self) -> &[u8] {
        if !self.translated_payload.is_empty() {
            &self.translated_payload
        } else if let Some(data_guard) = &self.data_guard {
            data_guard.get_payload()
        } else {
            &[]
        }
    }
}
