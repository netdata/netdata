use std::io::Write;
use std::net::IpAddr;

/// Reusable buffer for encoding flow fields into journal entries.
/// Avoids ~60 Vec<u8> allocations per flow by writing all fields into
/// a single contiguous buffer and tracking offsets.
pub(crate) struct JournalEncodeBuffer {
    data: Vec<u8>,
    refs: Vec<std::ops::Range<usize>>,
    ibuf: itoa::Buffer,
}

impl JournalEncodeBuffer {
    pub(crate) fn new() -> Self {
        Self {
            data: Vec::with_capacity(4096),
            refs: Vec::with_capacity(64),
            ibuf: itoa::Buffer::new(),
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
        journal: &mut journal_log_writer::Log,
        timestamps: journal_log_writer::EntryTimestamps,
    ) -> journal_log_writer::Result<()> {
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

    #[cfg(test)]
    pub(crate) fn debug_field_slices(&self) -> Vec<&[u8]> {
        self.refs.iter().map(|range| &self.data[range.clone()]).collect()
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
