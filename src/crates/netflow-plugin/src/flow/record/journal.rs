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
    /// Encode fields into a byte buffer for journal writing. Fields at their
    /// default value (0, empty string, `None`) are skipped, and presence-tracked
    /// fields are emitted only when their flag is set; ports are the exception,
    /// emitted by value alone, so a present-but-zero port loses its presence
    /// flag on the `from_fields` round trip. `PROTOCOL` is retained even when
    /// zero, and missing fields are read back by `from_fields` at these same
    /// defaults, so field values round-trip losslessly. This reduces typical
    /// per-entry item counts from 91 to ~20-25.
    pub(crate) fn encode_to_journal_buf(
        &self,
        data: &mut Vec<u8>,
        refs: &mut Vec<std::ops::Range<usize>>,
        value_starts: &mut Vec<usize>,
    ) {
        data.clear();
        refs.clear();
        value_starts.clear();

        let mut writer = JournalBufWriter::new(data, refs, value_starts);
        encode_core_journal_fields(self, &mut writer);
        encode_network_journal_fields(self, &mut writer);
        encode_interface_journal_fields(self, &mut writer);
        encode_transport_journal_fields(self, &mut writer);
        encode_header_journal_fields(self, &mut writer);
    }
}
