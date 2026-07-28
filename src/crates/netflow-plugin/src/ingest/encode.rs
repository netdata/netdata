use std::io::Write;
use std::net::IpAddr;

/// Reusable buffer for encoding flow fields into journal entries.
/// Avoids ~60 Vec<u8> allocations per flow by writing all fields into
/// a single contiguous buffer and tracking offsets.
pub(crate) struct JournalEncodeBuffer {
    data: Vec<u8>,
    refs: Vec<std::ops::Range<usize>>,
    value_starts: Vec<usize>,
    ibuf: itoa::Buffer,
}

impl JournalEncodeBuffer {
    pub(crate) fn new() -> Self {
        Self {
            data: Vec::with_capacity(4096),
            refs: Vec::with_capacity(64),
            value_starts: Vec::with_capacity(64),
            ibuf: itoa::Buffer::new(),
        }
    }

    /// Encode a FlowRecord and write to journal in one call.
    /// Uses a stack-allocated array for field slices — zero heap allocation.
    /// The borrow of self.data is contained within this method.
    pub(super) fn encode_record_and_write(
        &mut self,
        record: &crate::flow::FlowRecord,
        journal: &mut journal_sdk_log_writer::Log,
        timestamps: journal_sdk_log_writer::EntryTimestamps,
    ) -> journal_sdk_log_writer::Result<()> {
        record.encode_to_journal_buf(&mut self.data, &mut self.refs, &mut self.value_starts);
        debug_assert_eq!(self.refs.len(), self.value_starts.len());
        self.debug_assert_unique_payloads();

        // 87 canonical fields — stack array avoids heap allocation. The flow
        // encoder already split each canonical field, so do not make the
        // journal writer scan every `KEY=value` payload again.
        let mut fields = [journal_sdk_log_writer::StructuredField::new(&[], &[]); 87];
        let n = self.refs.len().min(87);
        for (i, (range, value_start)) in self.refs[..n]
            .iter()
            .zip(&self.value_starts[..n])
            .enumerate()
        {
            debug_assert!(range.start < *value_start);
            debug_assert!(*value_start <= range.end);
            fields[i] = journal_sdk_log_writer::StructuredField::new(
                &self.data[range.start..(*value_start - 1)],
                &self.data[*value_start..range.end],
            );
        }
        journal.write_fields_with_options(
            &fields[..n],
            timestamps,
            journal_sdk_log_writer::EntryWriteOptions::default().trusted_unique_payloads(true),
        )
    }

    #[allow(dead_code)]
    pub(crate) fn encode(&mut self, fields: &crate::flow::FlowFields) {
        self.clear();

        for (name, value) in fields {
            self.push_str(name, value);
        }
    }

    pub(crate) fn clear(&mut self) {
        self.data.clear();
        self.refs.clear();
        self.value_starts.clear();
    }

    pub(crate) fn push_str(&mut self, name: &str, value: &str) {
        let start = self.data.len();
        self.data.extend_from_slice(name.as_bytes());
        self.data.push(b'=');
        self.data.extend_from_slice(value.as_bytes());
        self.refs.push(start..self.data.len());
    }

    pub(crate) fn push_u8(&mut self, name: &str, value: u8) {
        self.push_number(name, value as u64);
    }

    pub(crate) fn push_u16(&mut self, name: &str, value: u16) {
        self.push_number(name, value as u64);
    }

    pub(crate) fn push_u32(&mut self, name: &str, value: u32) {
        self.push_number(name, value as u64);
    }

    pub(crate) fn push_u64(&mut self, name: &str, value: u64) {
        self.push_number(name, value);
    }

    pub(crate) fn push_ip_addr(&mut self, name: &str, value: IpAddr) {
        let start = self.data.len();
        self.data.extend_from_slice(name.as_bytes());
        self.data.push(b'=');
        let _ = write!(self.data, "{}", value);
        self.refs.push(start..self.data.len());
    }

    pub(crate) fn write_encoded(
        &self,
        journal: &mut journal_sdk_log_writer::Log,
        timestamps: journal_sdk_log_writer::EntryTimestamps,
    ) -> journal_sdk_log_writer::Result<()> {
        // Tier rows currently emit <= 73 fields; keep slack for schema growth.
        let mut slices = [&[] as &[u8]; 96];
        let n = self.refs.len().min(slices.len());
        for (index, range) in self.refs[..n].iter().enumerate() {
            slices[index] = &self.data[range.clone()];
        }
        journal.write_entry_with_timestamps(&slices[..n], timestamps)
    }

    pub(super) fn encoded_len(&self) -> u64 {
        self.data.len() as u64
    }

    #[cfg(debug_assertions)]
    fn debug_assert_unique_payloads(&self) {
        for (index, range) in self.refs.iter().enumerate() {
            let payload = &self.data[range.clone()];
            debug_assert!(
                self.refs[..index]
                    .iter()
                    .all(|previous| &self.data[previous.clone()] != payload),
                "canonical journal encoding emitted a duplicate payload"
            );
        }
    }

    #[cfg(not(debug_assertions))]
    fn debug_assert_unique_payloads(&self) {}

    #[cfg(test)]
    pub(crate) fn debug_field_slices(&self) -> Vec<&[u8]> {
        self.refs
            .iter()
            .map(|range| &self.data[range.clone()])
            .collect()
    }

    fn push_number(&mut self, name: &str, value: u64) {
        let start = self.data.len();
        self.data.extend_from_slice(name.as_bytes());
        self.data.push(b'=');
        self.data
            .extend_from_slice(self.ibuf.format(value).as_bytes());
        self.refs.push(start..self.data.len());
    }
}
