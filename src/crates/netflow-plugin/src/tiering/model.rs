use crate::flow::{FlowFields, FlowRecord};
use crate::flow_index::FlowId as IndexedFlowId;
use std::time::Duration;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub(crate) enum TierKind {
    Raw,
    Minute1,
    Minute5,
    Hour1,
}

impl TierKind {
    pub(crate) fn dir_name(self) -> &'static str {
        match self {
            Self::Raw => "raw",
            Self::Minute1 => "1m",
            Self::Minute5 => "5m",
            Self::Hour1 => "1h",
        }
    }

    pub(crate) fn bucket_duration(self) -> Option<Duration> {
        match self {
            Self::Raw => None,
            Self::Minute1 => Some(Duration::from_secs(60)),
            Self::Minute5 => Some(Duration::from_secs(5 * 60)),
            Self::Hour1 => Some(Duration::from_secs(60 * 60)),
        }
    }
}

pub(crate) const MATERIALIZED_TIERS: [TierKind; 3] =
    [TierKind::Minute1, TierKind::Minute5, TierKind::Hour1];

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) struct FlowMetrics {
    pub(crate) bytes: u64,
    pub(crate) packets: u64,
}

impl FlowMetrics {
    pub(crate) fn from_fields(fields: &FlowFields) -> Self {
        let bytes = parse_u64(fields.get("BYTES"));
        let packets = parse_u64(fields.get("PACKETS"));

        Self { bytes, packets }
    }

    pub(crate) fn from_record(rec: &FlowRecord) -> Self {
        Self {
            bytes: rec.bytes,
            packets: rec.packets,
        }
    }

    pub(crate) fn add(&mut self, other: FlowMetrics) {
        self.bytes = self.bytes.saturating_add(other.bytes);
        self.packets = self.packets.saturating_add(other.packets);
    }

    #[allow(dead_code)]
    pub(crate) fn write_fields(self, fields: &mut FlowFields) {
        fields.insert("BYTES", self.bytes.to_string());
        fields.insert("PACKETS", self.packets.to_string());
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub(crate) struct TierFlowRef {
    pub(crate) hour_start_usec: u64,
    pub(crate) flow_id: IndexedFlowId,
}

#[derive(Debug, Clone, Copy)]
#[allow(dead_code)]
pub(crate) struct OpenTierRow {
    pub(crate) timestamp_usec: u64,
    pub(crate) flow_ref: TierFlowRef,
    pub(crate) metrics: FlowMetrics,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct OpenTierState {
    pub(crate) generation: u64,
    pub(crate) minute_1_rows: u64,
    pub(crate) minute_5_rows: u64,
    pub(crate) hour_1_rows: u64,
    pub(crate) accumulator_heap_bytes: usize,
}

impl OpenTierState {
    pub(crate) fn replace_snapshot(
        &mut self,
        generation: u64,
        minute_1_rows: u64,
        minute_5_rows: u64,
        hour_1_rows: u64,
        accumulator_heap_bytes: usize,
    ) {
        self.generation = generation;
        self.minute_1_rows = minute_1_rows;
        self.minute_5_rows = minute_5_rows;
        self.hour_1_rows = hour_1_rows;
        self.accumulator_heap_bytes = accumulator_heap_bytes;
    }

    pub(crate) fn counts(&self) -> (u64, u64, u64) {
        (self.minute_1_rows, self.minute_5_rows, self.hour_1_rows)
    }

    pub(crate) const fn estimated_heap_bytes(&self) -> usize {
        self.accumulator_heap_bytes
    }
}

fn parse_u64(value: Option<&String>) -> u64 {
    value.and_then(|v| v.parse::<u64>().ok()).unwrap_or(0)
}
