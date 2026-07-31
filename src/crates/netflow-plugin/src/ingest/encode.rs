use std::io::Write;
use std::net::IpAddr;

const MAX_CANONICAL_JOURNAL_FIELDS: usize = crate::flow::CANONICAL_FLOW_DEFAULTS.len();

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

        // The schema-derived stack array avoids a heap allocation. The flow
        // encoder already split each canonical field, so do not make the
        // journal writer scan every `KEY=value` payload again.
        let mut fields =
            [journal_sdk_log_writer::StructuredField::new(&[], &[]); MAX_CANONICAL_JOURNAL_FIELDS];
        let n = self.refs.len();
        if n > fields.len() {
            return Err(journal_sdk_log_writer::WriterError::Serialization(format!(
                "canonical journal encoder emitted {n} fields, schema capacity is {}",
                fields.len()
            )));
        }
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

#[cfg(test)]
mod tests {
    use super::{JournalEncodeBuffer, MAX_CANONICAL_JOURNAL_FIELDS};
    use crate::flow::{CANONICAL_FLOW_DEFAULTS, FlowFields, FlowRecord};
    use journal_sdk_core::file::Mmap;
    use journal_sdk_core::repository::File as JournalRepositoryFile;
    use journal_sdk_core::{Direction, JournalFile, JournalReader, Location};
    use journal_sdk_log_writer::{
        Compression, Config, EntryTimestamps, Log, RetentionPolicy, RotationPolicy,
    };
    use journal_sdk_registry::{Origin, Source};
    use std::collections::BTreeSet;
    use std::num::NonZeroU64;
    use std::path::PathBuf;

    #[test]
    fn fully_populated_record_encodes_every_canonical_field() {
        let fields = CANONICAL_FLOW_DEFAULTS
            .iter()
            .map(|&(field, _)| (field, populated_value(field).to_string()))
            .collect::<FlowFields>();
        let record = FlowRecord::from_fields(&fields);
        let mut data = Vec::new();
        let mut refs = Vec::new();
        let mut value_starts = Vec::new();

        record.encode_to_journal_buf(&mut data, &mut refs, &mut value_starts);

        assert_eq!(MAX_CANONICAL_JOURNAL_FIELDS, 91);
        assert_eq!(refs.len(), MAX_CANONICAL_JOURNAL_FIELDS);
        assert_eq!(refs.len(), value_starts.len());

        let encoded_fields = refs
            .iter()
            .zip(&value_starts)
            .map(|(range, value_start)| {
                std::str::from_utf8(&data[range.start..(*value_start - 1)])
                    .expect("field name should be UTF-8")
            })
            .collect::<BTreeSet<_>>();
        let expected_fields = CANONICAL_FLOW_DEFAULTS
            .iter()
            .map(|&(field, _)| field)
            .collect::<BTreeSet<_>>();

        assert_eq!(encoded_fields, expected_fields);
        for field in ["MPLS_LABELS", "RAW_BYTES", "RAW_PACKETS", "SAMPLING_RATE"] {
            assert!(
                encoded_fields.contains(field),
                "{field} was previously beyond the stale 87-field write limit"
            );
        }
    }

    #[test]
    fn production_writer_persists_every_canonical_field() {
        let tmp = tempfile::tempdir().expect("create journal directory");
        let machine_id = uuid::Uuid::from_bytes([1; 16]);
        let boot_id = uuid::Uuid::from_bytes([2; 16]);
        let origin = Origin {
            machine_id: Some(machine_id),
            namespace: None,
            source: Source::System,
        };
        let mut journal = Log::new(
            tmp.path(),
            Config::new(
                origin,
                RotationPolicy::default().with_size_of_journal_file(1024 * 1024),
                RetentionPolicy::default(),
            )
            .with_boot_id(boot_id)
            .with_compact(true)
            .with_compression(Compression::None)
            .with_live_publish_every_entries(0),
        )
        .expect("create production journal writer");
        let fields = CANONICAL_FLOW_DEFAULTS
            .iter()
            .map(|&(field, _)| (field, populated_value(field).to_string()))
            .collect::<FlowFields>();
        let record = FlowRecord::from_fields(&fields);
        let mut encode_buf = JournalEncodeBuffer::new();

        encode_buf
            .encode_record_and_write(
                &record,
                &mut journal,
                EntryTimestamps::default()
                    .with_source_realtime_usec(1_000_000)
                    .with_entry_realtime_usec(1_000_000)
                    .with_entry_monotonic_usec(1),
            )
            .expect("write fully populated record through production boundary");
        journal.sync().expect("sync production journal");

        let active_path = PathBuf::from(
            journal
                .active_file()
                .expect("active journal")
                .path()
                .to_string(),
        );
        let repository_file =
            JournalRepositoryFile::from_path(&active_path).expect("parse active journal path");
        let journal_file = JournalFile::<Mmap>::open(&repository_file, 8 * 1024 * 1024)
            .expect("open production journal");
        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);
        assert!(
            reader
                .step(&journal_file, Direction::Forward)
                .expect("step production journal"),
            "production journal must contain the written entry"
        );
        let mut data_offsets = Vec::<NonZeroU64>::new();
        reader
            .entry_data_offsets(&journal_file, &mut data_offsets)
            .expect("enumerate production journal fields");
        let mut decompress_buf = Vec::new();
        let mut persisted_fields = BTreeSet::new();
        crate::query::visit_journal_payloads(
            &journal_file,
            &active_path,
            &data_offsets,
            &mut decompress_buf,
            |payload| {
                if let Some(eq_pos) = payload.iter().position(|byte| *byte == b'=') {
                    persisted_fields.insert(
                        std::str::from_utf8(&payload[..eq_pos])
                            .expect("journal field name should be UTF-8")
                            .to_string(),
                    );
                }
                Ok(())
            },
        )
        .expect("read production journal fields");

        let expected_fields = CANONICAL_FLOW_DEFAULTS
            .iter()
            .map(|&(field, _)| field)
            .collect::<BTreeSet<_>>();
        let persisted_canonical_fields = persisted_fields
            .iter()
            .map(String::as_str)
            .filter(|field| expected_fields.contains(field))
            .collect::<BTreeSet<_>>();
        assert_eq!(persisted_canonical_fields, expected_fields);
        for field in ["MPLS_LABELS", "RAW_BYTES", "RAW_PACKETS", "SAMPLING_RATE"] {
            assert!(
                persisted_fields.contains(field),
                "{field} was previously truncated at the production writer boundary"
            );
        }
    }

    fn populated_value(field: &str) -> &'static str {
        match field {
            "FLOW_VERSION" => "v9",
            "DIRECTION" => "ingress",
            "EXPORTER_IP" | "SRC_ADDR" | "DST_ADDR" | "NEXT_HOP" | "SRC_ADDR_NAT"
            | "DST_ADDR_NAT" => "192.0.2.1",
            "SRC_PREFIX" | "DST_PREFIX" => "192.0.2.0/24",
            "SRC_MAC" | "DST_MAC" => "00:00:00:00:00:01",
            _ => "1",
        }
    }
}
