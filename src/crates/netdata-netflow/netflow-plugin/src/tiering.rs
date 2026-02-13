use crate::rollup::{self, RollupKey};
use std::collections::{BTreeMap, HashMap};
use std::time::Duration;

const INTERNAL_FIELDS: &[&str] = &["_BOOT_ID", "_SOURCE_REALTIME_TIMESTAMP"];
const METRIC_FIELDS: &[&str] = &["BYTES", "PACKETS", "FLOWS", "RAW_BYTES", "RAW_PACKETS"];
const PROTOCOL_DEBUG_PREFIXES: &[&str] = &["V9_", "IPFIX_"];

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
    pub(crate) flows: u64,
    pub(crate) raw_bytes: u64,
    pub(crate) raw_packets: u64,
}

impl FlowMetrics {
    pub(crate) fn from_fields(fields: &BTreeMap<String, String>) -> Self {
        let bytes = parse_u64(fields.get("BYTES"));
        let packets = parse_u64(fields.get("PACKETS"));
        let flows = parse_u64(fields.get("FLOWS")).max(1);
        let raw_bytes = parse_u64(fields.get("RAW_BYTES"));
        let raw_packets = parse_u64(fields.get("RAW_PACKETS"));

        Self {
            bytes,
            packets,
            flows,
            raw_bytes,
            raw_packets,
        }
    }

    pub(crate) fn add(&mut self, other: FlowMetrics) {
        self.bytes = self.bytes.saturating_add(other.bytes);
        self.packets = self.packets.saturating_add(other.packets);
        self.flows = self.flows.saturating_add(other.flows);
        self.raw_bytes = self.raw_bytes.saturating_add(other.raw_bytes);
        self.raw_packets = self.raw_packets.saturating_add(other.raw_packets);
    }

    pub(crate) fn write_fields(self, fields: &mut BTreeMap<String, String>) {
        fields.insert("BYTES".to_string(), self.bytes.to_string());
        fields.insert("PACKETS".to_string(), self.packets.to_string());
        fields.insert("FLOWS".to_string(), self.flows.to_string());
        fields.insert("RAW_BYTES".to_string(), self.raw_bytes.to_string());
        fields.insert("RAW_PACKETS".to_string(), self.raw_packets.to_string());
    }
}

#[derive(Debug, Clone)]
pub(crate) struct OpenTierRow {
    pub(crate) timestamp_usec: u64,
    pub(crate) fields: BTreeMap<String, String>,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct OpenTierState {
    pub(crate) minute_1: Vec<OpenTierRow>,
    pub(crate) minute_5: Vec<OpenTierRow>,
    pub(crate) hour_1: Vec<OpenTierRow>,
}

impl OpenTierState {
    pub(crate) fn rows_for_tier(&self, tier: TierKind) -> &[OpenTierRow] {
        match tier {
            TierKind::Raw => &[],
            TierKind::Minute1 => &self.minute_1,
            TierKind::Minute5 => &self.minute_5,
            TierKind::Hour1 => &self.hour_1,
        }
    }
}

#[derive(Debug, Clone)]
struct AggregateEntry {
    labels: BTreeMap<String, String>,
    metrics: FlowMetrics,
}

#[derive(Debug)]
pub(crate) struct TierAccumulator {
    bucket_usec: u64,
    buckets: BTreeMap<u64, HashMap<RollupKey, AggregateEntry>>,
}

impl TierAccumulator {
    pub(crate) fn new(tier: TierKind) -> Self {
        let duration = tier
            .bucket_duration()
            .expect("materialized tier must have fixed bucket duration");
        Self {
            bucket_usec: duration.as_micros() as u64,
            buckets: BTreeMap::new(),
        }
    }

    pub(crate) fn observe_flow(&mut self, timestamp_usec: u64, fields: &BTreeMap<String, String>) {
        if timestamp_usec == 0 {
            return;
        }

        let bucket_start = bucket_start_usec(timestamp_usec, self.bucket_usec);
        let dimensions = dimensions_for_rollup(fields);
        let rollup_key = rollup::build_rollup_key(&dimensions);
        let labels: BTreeMap<String, String> = rollup_key.0.iter().cloned().collect();
        let metrics = FlowMetrics::from_fields(fields);

        let entry = self
            .buckets
            .entry(bucket_start)
            .or_default()
            .entry(rollup_key)
            .or_insert_with(|| AggregateEntry {
                labels,
                metrics: FlowMetrics::default(),
            });
        entry.metrics.add(metrics);
    }

    pub(crate) fn flush_closed_rows(&mut self, now_usec: u64) -> Vec<OpenTierRow> {
        let mut closable = Vec::new();
        for start in self.buckets.keys().copied() {
            let end = start.saturating_add(self.bucket_usec);
            if end <= now_usec {
                closable.push(start);
            }
        }

        let mut rows = Vec::new();
        for start in closable {
            let end = start.saturating_add(self.bucket_usec);
            if let Some(entries) = self.buckets.remove(&start) {
                for (_key, entry) in entries {
                    rows.push(OpenTierRow {
                        timestamp_usec: end.saturating_sub(1),
                        fields: build_row_fields(entry),
                    });
                }
            }
        }
        rows
    }

    pub(crate) fn snapshot_open_rows(&self, now_usec: u64) -> Vec<OpenTierRow> {
        let mut rows = Vec::new();
        for (start, entries) in &self.buckets {
            let end = start.saturating_add(self.bucket_usec);
            if end <= now_usec {
                continue;
            }

            for entry in entries.values() {
                rows.push(OpenTierRow {
                    timestamp_usec: now_usec.min(end.saturating_sub(1)),
                    fields: build_row_fields(entry.clone()),
                });
            }
        }
        rows
    }
}

pub(crate) fn dimensions_for_rollup(fields: &BTreeMap<String, String>) -> BTreeMap<String, String> {
    let mut dimensions = BTreeMap::new();
    for (name, value) in fields {
        if INTERNAL_FIELDS.contains(&name.as_str()) {
            continue;
        }
        if METRIC_FIELDS.contains(&name.as_str()) {
            continue;
        }
        if PROTOCOL_DEBUG_PREFIXES
            .iter()
            .any(|prefix| name.starts_with(prefix))
        {
            continue;
        }
        dimensions.insert(name.clone(), value.clone());
    }
    dimensions
}

fn build_row_fields(entry: AggregateEntry) -> BTreeMap<String, String> {
    let mut fields = entry.labels;
    entry.metrics.write_fields(&mut fields);
    fields
}

fn parse_u64(value: Option<&String>) -> u64 {
    value.and_then(|v| v.parse::<u64>().ok()).unwrap_or(0)
}

fn bucket_start_usec(timestamp_usec: u64, bucket_usec: u64) -> u64 {
    (timestamp_usec / bucket_usec).saturating_mul(bucket_usec)
}

#[cfg(test)]
mod tests {
    use super::{FlowMetrics, TierAccumulator, TierKind};
    use std::collections::BTreeMap;

    #[test]
    fn accumulator_flushes_closed_bucket() {
        let mut acc = TierAccumulator::new(TierKind::Minute1);
        let ts = 120_000_000; // minute 2
        let mut fields = BTreeMap::new();
        fields.insert("PROTOCOL".to_string(), "6".to_string());
        fields.insert("SRC_ADDR".to_string(), "10.0.0.1".to_string());
        fields.insert("DST_ADDR".to_string(), "10.0.0.2".to_string());
        fields.insert("SRC_PORT".to_string(), "12345".to_string());
        fields.insert("DST_PORT".to_string(), "443".to_string());
        fields.insert("BYTES".to_string(), "100".to_string());
        fields.insert("PACKETS".to_string(), "2".to_string());
        fields.insert("FLOWS".to_string(), "1".to_string());
        fields.insert("V9_IN_BYTES".to_string(), "100".to_string());
        acc.observe_flow(ts, &fields);

        let rows = acc.flush_closed_rows(180_000_000);
        assert_eq!(rows.len(), 1);
        assert_eq!(
            rows[0].fields.get("PROTOCOL").map(String::as_str),
            Some("6")
        );
        assert_eq!(rows[0].fields.get("BYTES").map(String::as_str), Some("100"));
        assert!(rows[0].fields.get("SRC_ADDR").is_none());
        assert!(rows[0].fields.get("DST_ADDR").is_none());
        assert!(rows[0].fields.get("SRC_PORT").is_none());
        assert!(rows[0].fields.get("DST_PORT").is_none());
        assert!(rows[0].fields.get("V9_IN_BYTES").is_none());
    }

    #[test]
    fn metrics_defaults_flows_to_one() {
        let fields = BTreeMap::new();
        let m = FlowMetrics::from_fields(&fields);
        assert_eq!(m.flows, 1);
    }
}
