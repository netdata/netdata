use super::*;

pub(crate) struct QueryFlowRecord {
    pub(crate) timestamp_usec: u64,
    pub(crate) fields: BTreeMap<String, String>,
}

impl QueryFlowRecord {
    pub(crate) fn new(timestamp_usec: u64, mut fields: BTreeMap<String, String>) -> Self {
        populate_virtual_fields(&mut fields);
        Self {
            timestamp_usec,
            fields,
        }
    }
}

#[derive(Debug, Clone, Copy, Default)]
pub(crate) struct QueryFlowMetrics {
    pub(crate) bytes: u64,
    pub(crate) packets: u64,
}

impl QueryFlowMetrics {
    pub(crate) fn add(&mut self, other: QueryFlowMetrics) {
        self.bytes = self.bytes.saturating_add(other.bytes);
        self.packets = self.packets.saturating_add(other.packets);
    }

    pub(crate) fn to_value(self) -> Value {
        json!({
            "bytes": self.bytes,
            "packets": self.packets,
        })
    }

    pub(crate) fn to_map(self) -> HashMap<String, u64> {
        let mut m = HashMap::new();
        m.insert("bytes".to_string(), self.bytes);
        m.insert("packets".to_string(), self.packets);
        m
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RecordHandle {
    JournalRealtime { tier: TierKind, timestamp_usec: u64 },
}
