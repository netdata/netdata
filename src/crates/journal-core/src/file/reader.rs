//! The public read side of a journal file: [`JournalReader`] holds one
//! private [`JournalCursor`] and exposes it as a seek/step/enumerate
//! surface - filtered entry iteration (`step` plus the `add_match`
//! family), current-entry metadata reads (`get_realtime_usec`,
//! `get_seqnum`, `get_entry_offset`), and field/data enumeration
//! (`fields_enumerate`, `field_data_*`, `entry_data_offsets`). The file
//! is borrowed per call and never stored: a reader carries no
//! open/attach state and no `JournalState`; it starts empty from
//! `Default` and is driven against one `&JournalFile` at a time.
//!
//! Filter flow: `add_match`/`add_conjunction`/`add_disjunction` queue a
//! pending [`JournalFilter`]; the first [`JournalReader::step`] - or
//! [`JournalReader::build_filter`] - resolves it against the file's
//! hash tables into a [`FilterExpr`] (file/filter.rs). `step` installs
//! the result on the cursor and consumes the pending filter; a filter
//! that matches nothing simply ends iteration with `Ok(false)`.
//!
//! Field-name remapping (field_map.rs):
//! [`JournalReader::load_remappings`] rebuilds the reader's
//! [`FieldMap`] from the file's `ND_REMAPPING=1` entries, and
//! [`JournalReader::add_match`] rewrites query-side original names to
//! their stored journal names before filtering. Nothing translates
//! names back on read - entries expose the stored names as written.
//!
//! Consumers (grep-verified): no caller outside this crate yet - the
//! type is re-exported in `file/mod.rs` and lib.rs, and only the
//! tests below call it. The same surface lives on as the near twin
//! src/crates/jf/journal_file/src/reader.rs (which adds
//! `entry_data_enumerate` and a working `dump`, and has no remapping
//! support), wrapped by src/crates/jf/journal_reader_ffi/src/lib.rs
//! into the `rsd_journal_*` C API; netflow-plugin instead uses the
//! published systemd-journal-sdk-core crate with the same design
//! (src/crates/Cargo.toml). journal-engine and journal-index read
//! through [`JournalFile`] directly (journal-engine/src/logs/query.rs,
//! journal-index/src/file_index.rs), not through this reader.
use super::mmap::MemoryMap;
use crate::error::Result;
use crate::field_map::{FieldMap, REMAPPING_MARKER, extract_field_name};
use crate::file::{
    EntryItemsType,
    cursor::{JournalCursor, Location},
    file::{EntryDataIterator, FieldDataIterator, FieldIterator, JournalFile},
    filter::{FilterExpr, JournalFilter, LogicalOp},
    object::{DataObject, FieldObject, HashableObject},
    offset_array::Direction,
    value_guard::ValueGuard,
};
use std::num::NonZeroU64;

/// Stateful read side of one journal file; the file is borrowed per
/// call and never stored.
///
/// State, all empty under `Default`:
/// - `cursor` + `filter`: entry iteration - the cursor's position and
///   installed filter, plus the pending (unresolved) [`JournalFilter`]
///   fed by the `add_match` family and resolved by [`Self::step`] or
///   [`Self::build_filter`].
/// - the `*_iterator` + `*_guard` slots: enumeration positions and the
///   guard the last advance left behind - at most one guard is live at
///   a time, and every advance drops it first because the file's
///   window manager admits one object at a time (its `GuardedCell` flag).
/// - `remapping_registry`: original -> stored field names, read by
///   [`Self::add_match`], rebuilt by [`Self::load_remappings`].
pub struct JournalReader<'a, M: MemoryMap> {
    cursor: JournalCursor,

    filter: Option<JournalFilter>,
    field_iterator: Option<FieldIterator<'a, M>>,
    field_data_iterator: Option<FieldDataIterator<'a, M>>,
    entry_data_iterator: Option<EntryDataIterator<'a, M>>,

    field_guard: Option<ValueGuard<'a, FieldObject<&'a [u8]>>>,
    data_guard: Option<ValueGuard<'a, DataObject<&'a [u8]>>>,

    // Original -> stored field names; rebuilt by load_remappings,
    // read by add_match.
    remapping_registry: FieldMap,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::file::{JournalFileOptions, JournalWriter, MmapMut};
    use tempfile::TempDir;

    fn test_uuid(seed: u8) -> uuid::Uuid {
        uuid::Uuid::from_bytes([seed; 16])
    }

    fn create_test_journal() -> (TempDir, JournalFile<MmapMut>) {
        let dir = TempDir::new().expect("create temp dir");
        let journal_dir = dir.path().join("journals");
        std::fs::create_dir_all(&journal_dir).expect("create journal dir");
        let path = journal_dir.join("system.journal");
        let repo_file =
            crate::repository::File::from_path(&path).expect("test journal path should parse");

        let mut journal_file = JournalFile::create(
            &repo_file,
            JournalFileOptions::new(test_uuid(1), test_uuid(2), test_uuid(3)),
        )
        .expect("create journal");
        let mut writer =
            JournalWriter::new(&mut journal_file, 1, test_uuid(4)).expect("create writer");
        let payloads = [b"MESSAGE=test".as_slice(), b"PRIORITY=6".as_slice()];
        writer
            .add_entry(&mut journal_file, &payloads, 1_000_000, 100)
            .expect("write entry");

        (dir, journal_file)
    }

    #[test]
    fn build_filter_returns_expr_and_consumes_pending_filter() {
        let (_dir, journal_file) = create_test_journal();
        let mut reader = JournalReader::<MmapMut>::default();
        reader.add_match(b"MESSAGE=test");

        let expr = reader
            .build_filter(&journal_file)
            .expect("build filter")
            .expect("resolved filter expr");

        assert!(!matches!(expr, FilterExpr::None));
        assert!(reader.filter.is_none(), "pending filter should be consumed");
        assert!(
            reader
                .build_filter(&journal_file)
                .expect("second build")
                .is_none()
        );
    }

    #[test]
    fn build_filter_failure_keeps_pending_filter() {
        let (_dir, journal_file) = create_test_journal();
        let mut reader = JournalReader::<MmapMut>::default();
        reader.filter = Some(JournalFilter::default());

        assert!(reader.build_filter(&journal_file).is_err());
        assert!(
            reader.filter.is_some(),
            "pending filter should remain after build failure"
        );
        assert!(reader.build_filter(&journal_file).is_err());
    }
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
        }
    }
}

impl<'a, M: MemoryMap> JournalReader<'a, M> {
    /// Placeholder dump of the installed filter: returns the literal
    /// "no filter expr" while the cursor has none, panics with
    /// `todo!()` once it has. No in-repo caller (grep-verified); the
    /// near twin implements it via `FilterExpr::dump`
    /// (src/crates/jf/journal_file/src/reader.rs).
    pub fn dump(&self, _journal_file: &'a JournalFile<M>) -> Result<String> {
        if let Some(_filter_expr) = self.cursor.filter_expr.as_ref() {
            todo!();
        } else {
            Ok(String::from("no filter expr"))
        }
    }

    /// Re-anchors the cursor for the next [`Self::step`] without
    /// touching any filter; discards the entry-array chain position,
    /// so the next step re-establishes it from the new anchor
    /// ([`JournalCursor::set_location`]).
    pub fn set_location(&mut self, location: Location) {
        self.cursor.set_location(location)
    }

    /// Advances to the next entry in `direction`, installing a pending
    /// filter first if one is queued.
    ///
    /// Drops the held guards, then - when `add_match`/
    /// `add_conjunction`/`add_disjunction` have queued a pending
    /// filter - resolves it against `journal_file` and installs the
    /// resulting expression on the cursor (replacing any earlier one;
    /// [`JournalCursor::set_filter`] keeps the cursor's position as the
    /// stepping anchor) and consumes the pending filter. A
    /// resolution error propagates without stepping and leaves the
    /// pending filter queued for a retry.
    ///
    /// Then delegates to [`JournalCursor::step`]:
    /// `Ok(true)` resolves an entry - read it through
    /// [`Self::get_entry_offset`] and friends; `Ok(false)` ends
    /// iteration in that direction with the cursor untouched, so the
    /// direction can be switched.
    pub fn step(&mut self, journal_file: &'a JournalFile<M>, direction: Direction) -> Result<bool> {
        self.drop_guards();

        if let Some(filter) = self.filter.as_mut() {
            let filter_expr = filter.build(journal_file)?;
            self.cursor.set_filter(filter_expr);
            self.filter = None;
        }

        self.cursor.step(journal_file, direction)
    }

    /// Resolves the pending filter (if any) against `journal_file` and
    /// returns the resulting [`FilterExpr`] - without installing it on
    /// the reader's cursor, unlike [`Self::step`], which installs what
    /// it builds.
    ///
    /// Returns `Ok(None)` when nothing is pending. The pending filter
    /// is consumed on success and kept on resolution failure (asserted
    /// by the tests below), so the same filter can be rebuilt after
    /// fixing the cause. Meant for callers that drive [`JournalCursor`]
    /// themselves.
    pub fn build_filter(&mut self, journal_file: &JournalFile<M>) -> Result<Option<FilterExpr>> {
        self.drop_guards();
        if let Some(filter) = self.filter.as_mut() {
            let expr = filter.build(journal_file)?;
            self.filter = None;
            Ok(Some(expr))
        } else {
            Ok(None)
        }
    }

    /// Queues one `FIELD=VALUE` match into the pending filter (items
    /// without `=` are ignored by [`JournalFilter::add_match`]).
    ///
    /// When the field name is an original name that
    /// [`Self::load_remappings`] has mapped to a stored journal name,
    /// the match is rewritten to the stored name first (see the body) -
    /// so queries written against original names work against remapped
    /// data. The registry is read as-is: with an empty registry (before
    /// `load_remappings` ran) no rewrite happens.
    ///
    /// # Example
    ///
    /// ```no_run
    /// # use journal_core::{JournalReader, JournalFile};
    /// # use journal_core::file::Mmap;
    /// # fn example(reader: &mut JournalReader<Mmap>, file: &JournalFile<Mmap>) {
    /// // Works even when "my.field.name" is stored under an
    /// // rdp-encoded name:
    /// reader.add_match(b"my.field.name=some_value");
    /// # }
    /// ```
    pub fn add_match(&mut self, data: &[u8]) {
        // Rewrite the query when the field name is one load_remappings
        // knows.
        if let Some(field_name) = extract_field_name(data) {
            if let Some(systemd_name) = self.remapping_registry.get_systemd_name(field_name) {
                // Remapped: rebuild the item under the stored name.
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

        // Unmapped (or no '=' in the item) - queue as written.
        self.filter.get_or_insert_default().add_match(data);
    }

    /// Folds the matches accumulated so far into the pending expression
    /// under `Conjunction` ([`JournalFilter::set_operation`]): matches
    /// are resolved against `journal_file` right away, so lookup errors
    /// surface here. With nothing accumulated, only the operation is
    /// recorded for the next resolution ([`JournalFilter::set_operation`]).
    pub fn add_conjunction(&mut self, journal_file: &'a JournalFile<M>) -> Result<()> {
        self.filter
            .get_or_insert_default()
            .set_operation(journal_file, LogicalOp::Conjunction)
    }

    /// Same as [`Self::add_conjunction`] with `LogicalOp::Disjunction`.
    pub fn add_disjunction(&mut self, journal_file: &'a JournalFile<M>) -> Result<()> {
        self.filter
            .get_or_insert_default()
            .set_operation(journal_file, LogicalOp::Disjunction)
    }

    /// Clears both filter states - the pending filter queued by the
    /// `add_match` family and the filter installed on the cursor - and
    /// rewinds the cursor to `Head`, forgetting its chain position
    /// ([`JournalCursor::clear_filter`]).
    pub fn flush_matches(&mut self) {
        self.cursor.clear_filter();
        self.filter = None;
    }

    /// Wall-clock time (microseconds since the epoch) of the current
    /// entry, read from its entry object at the cursor position. Fails
    /// with `UnsetCursor` until a [`Self::step`] has resolved an entry
    /// ([`JournalCursor::position`]).
    pub fn get_realtime_usec(&self, journal_file: &'a JournalFile<M>) -> Result<u64> {
        let entry_offset = self.cursor.position()?;
        let entry_object = journal_file.entry_ref(entry_offset)?;
        Ok(entry_object.header.realtime)
    }

    /// Sequence number of the current entry, plus the file's
    /// sequence-number ID from the journal header. Same `UnsetCursor`
    /// rule as [`Self::get_realtime_usec`].
    pub fn get_seqnum(&self, journal_file: &'a JournalFile<M>) -> Result<(u64, [u8; 16])> {
        let entry_offset = self.cursor.position()?;
        let entry_object = journal_file.entry_ref(entry_offset)?;
        Ok((
            entry_object.header.seqnum,
            journal_file.journal_header_ref().seqnum_id,
        ))
    }

    /// File offset of the current entry object - the cursor's resolved
    /// position. Same `UnsetCursor` rule as [`Self::get_realtime_usec`].
    pub fn get_entry_offset(&self) -> Result<NonZeroU64> {
        self.cursor.position()
    }

    fn drop_guards(&mut self) {
        self.field_guard.take();
        self.data_guard.take();
    }

    /// Resets the field enumeration: the next [`Self::fields_enumerate`]
    /// starts from the first field again, and any held guard is dropped.
    pub fn fields_restart(&mut self) {
        self.drop_guards();
        self.field_iterator = None;
    }

    /// Returns the next field object of the field hash table, or `None`
    /// after the last one. Creates the iterator on the first call after
    /// [`Self::fields_restart`] (or on a fresh reader); once exhausted
    /// it keeps returning `None` until restarted. Order is bucket
    /// order, then hash-chain order within each bucket
    /// ([`JournalFile::fields`]).
    ///
    /// The returned guard borrows the reader and holds the file's
    /// one-object-at-a-time access slot (the window manager's in-use
    /// flag); every call first drops the previous guard.
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

    /// Binds the data enumeration to `field_name`: creates a fresh
    /// iterator over every DATA object carrying that field. Hash-table
    /// lookup; a field the file does not have yields an iterator that
    /// ends immediately ([`JournalFile::field_data_objects`]). This is the
    /// only way to
    /// start (or restart) the enumeration -
    /// [`Self::field_data_restart`] does not reset it.
    pub fn field_data_query_unique(
        &mut self,
        journal_file: &'a JournalFile<M>,
        field_name: &'a [u8],
    ) -> Result<()> {
        self.drop_guards();

        self.field_data_iterator = Some(journal_file.field_data_objects(field_name)?);
        Ok(())
    }

    /// Drops the held data guard. Unlike [`Self::fields_restart`] and
    /// [`Self::entry_data_restart`], this does NOT reset the
    /// enumeration position: the next [`Self::field_data_enumerate`]
    /// continues where it stopped. To restart, call
    /// [`Self::field_data_query_unique`] again.
    pub fn field_data_restart(&mut self) {
        self.drop_guards();
    }

    /// Returns the next DATA object of the field bound by
    /// [`Self::field_data_query_unique`], or `None` when unbound or
    /// exhausted. The `journal_file` argument is unused - the
    /// enumeration follows the bound iterator, not the cursor.
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

    /// Drops the held guards and clears `entry_data_iterator` - a field
    /// no method in this file ever sets (the near twin's
    /// `entry_data_enumerate` does,
    /// src/crates/jf/journal_file/src/reader.rs). Per-entry data
    /// access here goes through [`Self::entry_data_offsets`].
    pub fn entry_data_restart(&mut self) {
        self.drop_guards();
        self.entry_data_iterator = None;
    }

    /// Appends the current entry's data-object offsets to
    /// `data_offsets` (without clearing it first), covering both
    /// entry-item layouts, `Regular` and `Compact`. Items with a zero
    /// object offset are skipped. Requires a resolved entry
    /// (`UnsetCursor` otherwise); resolve the offsets against the file
    /// with `JournalFile::data_ref`. Unlike the enumerators, this does
    /// not drop the reader's guards first - with one still live, the
    /// internal `entry_ref` fails with `ValueGuardInUse`.
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

    /// Rebuilds the reader's remapping registry from this file's
    /// remapping entries - the read-side mirror of what
    /// journal-log-writer writes (journal-log-writer/src/log/mod.rs).
    /// Finds the `ND_REMAPPING` field (the name part of
    /// `REMAPPING_MARKER`) through the field hash table, collects the
    /// entries referencing its data objects, and parses each entry's
    /// other payloads as `stored-name=original-name` into the registry.
    ///
    /// Name-shape gate: only stored names shaped like
    /// `rdp::encode_full`'s MD5 fallback are accepted (see the body) -
    /// normal-shape names are silently dropped, while
    /// [`JournalFile::load_fields`] parses the same
    /// entries without any gate and loads both shapes. The two paths
    /// disagree.
    ///
    /// Skipped without error: the marker payload itself, and every
    /// payload the gate rejects - including the `_BOOT_ID=...` item the
    /// writer adds to every remapping entry
    /// (journal-log-writer/src/log/mod.rs). Errors: a failing
    /// marker lookup is swallowed and treated as "no remappings in this
    /// file"; failures while reading marker data objects, entry chains
    /// or payloads propagate. First mapping per original name wins
    /// ([`FieldMap::add_otel_mapping`]).
    ///
    /// No caller in this workspace yet (grep-verified): run it before
    /// [`Self::add_match`] for query-side name translation to have any
    /// effect.
    ///
    /// # Performance
    ///
    /// Field-index lookup plus one pass over the marker's referencing
    /// entries - not a scan over all entries.
    pub fn load_remappings(&mut self, journal_file: &'a JournalFile<M>) -> Result<()> {
        // Field-name part of REMAPPING_MARKER, for the hash-table lookup.
        let marker_field_name = {
            let marker_str = std::str::from_utf8(REMAPPING_MARKER)
                .map_err(|_| crate::error::JournalError::InvalidField)?;
            let eq_pos = marker_str
                .find('=')
                .ok_or(crate::error::JournalError::InvalidField)?;
            &marker_str.as_bytes()[..eq_pos]
        };

        // Two-phase: collect the entry references first, then parse
        // below - the window guards held while walking the marker's
        // data objects must be released before the parse re-enters the
        // file.
        let mut entry_info: Vec<(Option<NonZeroU64>, Option<(NonZeroU64, u64)>)> = Vec::new();

        {
            // All DATA objects carrying the marker field.
            let Ok(mut data_iter) = journal_file.field_data_objects(marker_field_name) else {
                // Lookup error (e.g. no field hash table) is swallowed:
                // treated as "no remappings in this file".
                return Ok(());
            };

            // Each marker DATA object; the mappings live in the
            // payloads of the entries referencing it.
            while let Some(data_guard) = data_iter.next().transpose()? {
                // Referencing entries: the first is inlined in the
                // header (`DataObjectHeader::entry_offset`), the rest hang
                // off the entry-array chain.
                let n_entries = data_guard.header.n_entries;

                if let Some(entry_count) = n_entries {
                    match entry_count.get() {
                        0 => {
                            // Unreachable: n_entries is NonZeroU64
                            // (`DataObjectHeader::n_entries`).
                            continue;
                        }
                        1 => {
                            // One referencing entry, inlined in the header.
                            if let Some(entry_offset) = data_guard.header.entry_offset {
                                entry_info.push((Some(entry_offset), None));
                            }
                        }
                        n => {
                            // First referencing entry inlined; walk the
                            // array for the remaining n-1.
                            if let Some(entry_offset) = data_guard.header.entry_offset {
                                entry_info.push((Some(entry_offset), None));
                            }
                            // The chain lists one offset per remaining
                            // entry.
                            if let Some(array_offset) = data_guard.header.entry_array_offset {
                                entry_info.push((None, Some((array_offset, n))));
                            }
                        }
                    }
                }
            }
        }

        // Phase 2: parse the collected references.
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
        // Copy the payloads out while the guard scope is alive;
        // parsing happens after it ends.
        let mut payloads: Vec<Vec<u8>> = Vec::new();

        {
            let data_iter = journal_file.entry_data_objects(entry_offset)?;
            for data_result in data_iter {
                let data_guard = data_result?;
                let payload = data_guard.raw_payload();
                payloads.push(payload.to_vec());
            }
        }

        for payload in payloads {
            // Skip the marker field itself
            if payload == REMAPPING_MARKER {
                continue;
            }

            // Parse "<stored-name>=<original-name>" (name before the
            // first '=').
            if let Some(field_name) = extract_field_name(&payload) {
                if field_name.starts_with(b"ND_") && field_name.len() == 35 {
                    // Gate: only rdp::encode_full's MD5-fallback shape
                    // passes - "ND_" + 32 hex chars = 35 bytes
                    // (rdp/src/lib.rs). Normal-shape names
                    // (ND<checksum><structure>_<NAME>) never start with
                    // "ND_": chars after "ND" come from the checksum
                    // (A-Z/0-9) or the structure alphabet (A-X/0-9 after
                    // uppercasing), never '_' (rdp's structure alphabet) -
                    // so such mappings are silently dropped here, while
                    // JournalFile::load_fields has no
                    // gate.
                    //
                    // '=' exists: extract_field_name found it.
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
        // All entry offsets on the array chain.
        let mut entry_offsets = Vec::new();

        // The array chain holds n_entries - 1 offsets: n_entries counts
        // all referencing entries, one of which is inlined in the
        // header.
        let array_count = n_entries.saturating_sub(1);

        if let Some(total_items_nz) = std::num::NonZeroUsize::new(array_count as usize) {
            use crate::file::offset_array::List;
            let list = List::new(array_offset, total_items_nz);
            list.collect_offsets(journal_file, &mut entry_offsets)?;
        }

        for entry_offset in entry_offsets {
            self.parse_remapping_entry(journal_file, entry_offset)?;
        }

        Ok(())
    }
}
