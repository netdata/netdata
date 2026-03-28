/// Reusable buffer for encoding flow fields into journal entries.
/// Avoids ~60 Vec<u8> allocations per flow by writing all fields into
/// a single contiguous buffer and tracking offsets.
pub(super) struct JournalEncodeBuffer {
    data: Vec<u8>,
    refs: Vec<std::ops::Range<usize>>,
}

impl JournalEncodeBuffer {
    pub(super) fn new() -> Self {
        Self {
            data: Vec::with_capacity(4096),
            refs: Vec::with_capacity(64),
        }
    }

    /// Encode a FlowRecord and write to journal in one call.
    /// Uses a stack-allocated array for field slices — zero heap allocation.
    /// The borrow of self.data is contained within this method.
    pub(super) fn encode_record_and_write(
        &mut self,
        record: &crate::flow::FlowRecord,
        journal: &mut journal_log_writer::Log,
        timestamps: journal_log_writer::EntryTimestamps,
    ) -> journal_log_writer::Result<()> {
        record.encode_to_journal_buf(&mut self.data, &mut self.refs);
        // 87 canonical fields — stack array avoids heap allocation.
        let mut slices = [&[] as &[u8]; 87];
        let n = self.refs.len().min(87);
        for (i, r) in self.refs[..n].iter().enumerate() {
            slices[i] = &self.data[r.clone()];
        }
        journal.write_entry_with_timestamps(&slices[..n], timestamps)
    }

    pub(super) fn encode(&mut self, fields: &crate::flow::FlowFields) {
        self.data.clear();
        self.refs.clear();

        for (name, value) in fields {
            let start = self.data.len();
            self.data.extend_from_slice(name.as_bytes());
            self.data.push(b'=');
            self.data.extend_from_slice(value.as_bytes());
            self.refs.push(start..self.data.len());
        }
    }

    pub(super) fn field_slices(&self) -> Vec<&[u8]> {
        self.refs.iter().map(|r| &self.data[r.clone()]).collect()
    }

    pub(super) fn facet_contribution(&self) -> crate::facet_runtime::FacetFileContribution {
        crate::facet_runtime::facet_contribution_from_encoded_fields(
            self.refs.iter().map(|r| &self.data[r.clone()]),
        )
    }

    pub(super) fn encoded_len(&self) -> u64 {
        self.data.len() as u64
    }
}
