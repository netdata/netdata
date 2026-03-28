use super::*;

mod core;
mod headers;
mod interfaces;
mod network;
mod transport;
mod writer;

use core::encode_core_journal_fields;
use headers::encode_header_journal_fields;
use interfaces::encode_interface_journal_fields;
use network::encode_network_journal_fields;
use transport::encode_transport_journal_fields;
use writer::JournalBufWriter;

impl FlowRecord {
    /// Encode non-default fields into a byte buffer for journal writing.
    /// Skips fields at their default value (0, empty string, None) — the reader
    /// (`from_fields`) defaults missing fields to the same values, so the
    /// round-trip is lossless. Reduces per-entry item count from 87 to ~20-25
    /// for typical flows, proportionally cutting journal write overhead.
    pub(crate) fn encode_to_journal_buf(
        &self,
        data: &mut Vec<u8>,
        refs: &mut Vec<std::ops::Range<usize>>,
    ) {
        data.clear();
        refs.clear();

        let mut writer = JournalBufWriter::new(data, refs);
        encode_core_journal_fields(self, &mut writer);
        encode_network_journal_fields(self, &mut writer);
        encode_interface_journal_fields(self, &mut writer);
        encode_transport_journal_fields(self, &mut writer);
        encode_header_journal_fields(self, &mut writer);
    }
}
