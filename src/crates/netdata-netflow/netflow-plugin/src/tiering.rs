use crate::decoder::{FlowDirection, FlowFields, FlowRecord};
use anyhow::{Context, Result, anyhow};
use netdata_flow_index::{
    FieldKind as IndexFieldKind, FieldSpec as IndexFieldSpec, FieldValue as IndexFieldValue,
    FlowId as IndexedFlowId, FlowIndex, FlowIndexError,
};
use std::collections::{BTreeMap, BTreeSet, HashMap};
use std::net::{IpAddr, Ipv4Addr};
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

    pub(crate) fn write_fields(self, fields: &mut FlowFields) {
        fields.insert("BYTES", self.bytes.to_string());
        fields.insert("PACKETS", self.packets.to_string());
    }
}

const HOUR_BUCKET_USEC: u64 = 60 * 60 * 1_000_000;
const INTERNAL_EXPORTER_IP_PRESENT: &str = "_EXPORTER_IP_PRESENT";
const INTERNAL_NEXT_HOP_PRESENT: &str = "_NEXT_HOP_PRESENT";
const INTERNAL_DIRECTION_PRESENT: &str = "_DIRECTION_PRESENT";
const INTERNAL_ETYPE_PRESENT: &str = "_ETYPE_PRESENT";
const INTERNAL_FORWARDING_STATUS_PRESENT: &str = "_FORWARDING_STATUS_PRESENT";
const INTERNAL_IPTOS_PRESENT: &str = "_IPTOS_PRESENT";
const INTERNAL_TCP_FLAGS_PRESENT: &str = "_TCP_FLAGS_PRESENT";
const INTERNAL_ICMPV4_TYPE_PRESENT: &str = "_ICMPV4_TYPE_PRESENT";
const INTERNAL_ICMPV4_CODE_PRESENT: &str = "_ICMPV4_CODE_PRESENT";
const INTERNAL_ICMPV6_TYPE_PRESENT: &str = "_ICMPV6_TYPE_PRESENT";
const INTERNAL_ICMPV6_CODE_PRESENT: &str = "_ICMPV6_CODE_PRESENT";
const INTERNAL_IN_IF_SPEED_PRESENT: &str = "_IN_IF_SPEED_PRESENT";
const INTERNAL_OUT_IF_SPEED_PRESENT: &str = "_OUT_IF_SPEED_PRESENT";
const INTERNAL_IN_IF_BOUNDARY_PRESENT: &str = "_IN_IF_BOUNDARY_PRESENT";
const INTERNAL_OUT_IF_BOUNDARY_PRESENT: &str = "_OUT_IF_BOUNDARY_PRESENT";
const INTERNAL_SRC_VLAN_PRESENT: &str = "_SRC_VLAN_PRESENT";
const INTERNAL_DST_VLAN_PRESENT: &str = "_DST_VLAN_PRESENT";
const ROLLUP_PRESENCE_FIELDS: &[(&str, &str)] = &[
    ("DIRECTION", INTERNAL_DIRECTION_PRESENT),
    ("ETYPE", INTERNAL_ETYPE_PRESENT),
    ("FORWARDING_STATUS", INTERNAL_FORWARDING_STATUS_PRESENT),
    ("IPTOS", INTERNAL_IPTOS_PRESENT),
    ("TCP_FLAGS", INTERNAL_TCP_FLAGS_PRESENT),
    ("ICMPV4_TYPE", INTERNAL_ICMPV4_TYPE_PRESENT),
    ("ICMPV4_CODE", INTERNAL_ICMPV4_CODE_PRESENT),
    ("ICMPV6_TYPE", INTERNAL_ICMPV6_TYPE_PRESENT),
    ("ICMPV6_CODE", INTERNAL_ICMPV6_CODE_PRESENT),
    ("IN_IF_SPEED", INTERNAL_IN_IF_SPEED_PRESENT),
    ("OUT_IF_SPEED", INTERNAL_OUT_IF_SPEED_PRESENT),
    ("IN_IF_BOUNDARY", INTERNAL_IN_IF_BOUNDARY_PRESENT),
    ("OUT_IF_BOUNDARY", INTERNAL_OUT_IF_BOUNDARY_PRESENT),
    ("SRC_VLAN", INTERNAL_SRC_VLAN_PRESENT),
    ("DST_VLAN", INTERNAL_DST_VLAN_PRESENT),
];

fn rollup_presence_field(field: &str) -> Option<&'static str> {
    ROLLUP_PRESENCE_FIELDS
        .iter()
        .find_map(|(actual, internal)| (*actual == field).then_some(*internal))
}

fn is_internal_rollup_presence_field(field: &str) -> bool {
    ROLLUP_PRESENCE_FIELDS
        .iter()
        .any(|(_, internal)| *internal == field)
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub(crate) struct TierFlowRef {
    pub(crate) hour_start_usec: u64,
    pub(crate) flow_id: IndexedFlowId,
}

#[derive(Debug, Clone, Copy)]
pub(crate) struct OpenTierRow {
    pub(crate) timestamp_usec: u64,
    pub(crate) flow_ref: TierFlowRef,
    pub(crate) metrics: FlowMetrics,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct OpenTierState {
    pub(crate) generation: u64,
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

type MetricBucket = HashMap<TierFlowRef, FlowMetrics>;

#[derive(Default)]
pub(crate) struct TierFlowIndexStore {
    generation: u64,
    indexes: BTreeMap<u64, FlowIndex>,
    scratch_field_ids: Vec<u32>,
}

#[derive(Clone, Copy)]
struct RollupFieldDef {
    name: &'static str,
    kind: IndexFieldKind,
}

const ROLLUP_FIELD_DEFS: &[RollupFieldDef] = &[
    RollupFieldDef {
        name: "DIRECTION",
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "PROTOCOL",
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "ETYPE",
        kind: IndexFieldKind::U16,
    },
    RollupFieldDef {
        name: "FORWARDING_STATUS",
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "FLOW_VERSION",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "IPTOS",
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "TCP_FLAGS",
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "ICMPV4_TYPE",
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "ICMPV4_CODE",
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "ICMPV6_TYPE",
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "ICMPV6_CODE",
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "SRC_AS",
        kind: IndexFieldKind::U32,
    },
    RollupFieldDef {
        name: "DST_AS",
        kind: IndexFieldKind::U32,
    },
    RollupFieldDef {
        name: "SRC_AS_NAME",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "DST_AS_NAME",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: INTERNAL_EXPORTER_IP_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "EXPORTER_IP",
        kind: IndexFieldKind::IpAddr,
    },
    RollupFieldDef {
        name: "EXPORTER_PORT",
        kind: IndexFieldKind::U16,
    },
    RollupFieldDef {
        name: "EXPORTER_NAME",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "EXPORTER_GROUP",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "EXPORTER_ROLE",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "EXPORTER_SITE",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "EXPORTER_REGION",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "EXPORTER_TENANT",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "IN_IF",
        kind: IndexFieldKind::U32,
    },
    RollupFieldDef {
        name: "OUT_IF",
        kind: IndexFieldKind::U32,
    },
    RollupFieldDef {
        name: "IN_IF_NAME",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "OUT_IF_NAME",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "IN_IF_DESCRIPTION",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "OUT_IF_DESCRIPTION",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "IN_IF_SPEED",
        kind: IndexFieldKind::U64,
    },
    RollupFieldDef {
        name: "OUT_IF_SPEED",
        kind: IndexFieldKind::U64,
    },
    RollupFieldDef {
        name: "IN_IF_PROVIDER",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "OUT_IF_PROVIDER",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "IN_IF_CONNECTIVITY",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "OUT_IF_CONNECTIVITY",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "IN_IF_BOUNDARY",
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "OUT_IF_BOUNDARY",
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "SRC_NET_NAME",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "DST_NET_NAME",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "SRC_NET_ROLE",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "DST_NET_ROLE",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "SRC_NET_SITE",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "DST_NET_SITE",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "SRC_NET_REGION",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "DST_NET_REGION",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "SRC_NET_TENANT",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "DST_NET_TENANT",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "SRC_COUNTRY",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "DST_COUNTRY",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "SRC_GEO_CITY",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "DST_GEO_CITY",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "SRC_GEO_STATE",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: "DST_GEO_STATE",
        kind: IndexFieldKind::Text,
    },
    RollupFieldDef {
        name: INTERNAL_NEXT_HOP_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: "NEXT_HOP",
        kind: IndexFieldKind::IpAddr,
    },
    RollupFieldDef {
        name: "SRC_VLAN",
        kind: IndexFieldKind::U16,
    },
    RollupFieldDef {
        name: "DST_VLAN",
        kind: IndexFieldKind::U16,
    },
    RollupFieldDef {
        name: INTERNAL_DIRECTION_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_ETYPE_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_FORWARDING_STATUS_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_IPTOS_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_TCP_FLAGS_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_ICMPV4_TYPE_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_ICMPV4_CODE_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_ICMPV6_TYPE_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_ICMPV6_CODE_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_IN_IF_SPEED_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_OUT_IF_SPEED_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_IN_IF_BOUNDARY_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_OUT_IF_BOUNDARY_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_SRC_VLAN_PRESENT,
        kind: IndexFieldKind::U8,
    },
    RollupFieldDef {
        name: INTERNAL_DST_VLAN_PRESENT,
        kind: IndexFieldKind::U8,
    },
];

impl TierFlowIndexStore {
    pub(crate) fn generation(&self) -> u64 {
        self.generation
    }

    pub(crate) fn get_or_insert_record_flow(
        &mut self,
        timestamp_usec: u64,
        record: &FlowRecord,
    ) -> Result<TierFlowRef> {
        if timestamp_usec == 0 {
            return Err(anyhow!("tier timestamp cannot be zero"));
        }

        let hour_start_usec = bucket_start_usec(timestamp_usec, HOUR_BUCKET_USEC);
        if !self.indexes.contains_key(&hour_start_usec) {
            self.indexes.insert(
                hour_start_usec,
                build_rollup_flow_index().expect("rollup schema should be valid"),
            );
            self.generation = self.generation.saturating_add(1);
        }

        let index = self
            .indexes
            .get_mut(&hour_start_usec)
            .expect("hour index should exist after insertion");

        self.scratch_field_ids.clear();
        push_rollup_field_ids(index, record, &mut self.scratch_field_ids)
            .context("failed to intern tier flow dimensions")?;

        let flow_id = if let Some(existing) = index
            .find_flow_by_field_ids(&self.scratch_field_ids)
            .context("failed to find tier flow dimensions")?
        {
            existing
        } else {
            index
                .insert_flow_by_field_ids(&self.scratch_field_ids)
                .context("failed to insert tier flow dimensions")?
        };

        Ok(TierFlowRef {
            hour_start_usec,
            flow_id,
        })
    }

    pub(crate) fn materialize_fields(&self, flow_ref: TierFlowRef) -> Option<FlowFields> {
        let index = self.indexes.get(&flow_ref.hour_start_usec)?;
        materialize_rollup_fields(index, flow_ref.flow_id)
    }

    pub(crate) fn field_value_string(&self, flow_ref: TierFlowRef, field: &str) -> Option<String> {
        let normalized = field.to_ascii_uppercase();
        let index = self.indexes.get(&flow_ref.hour_start_usec)?;
        let field_ids = index.flow_field_ids(flow_ref.flow_id)?;

        match normalized.as_str() {
            "EXPORTER_IP" => {
                let present = rollup_field_value(index, field_ids, INTERNAL_EXPORTER_IP_PRESENT)
                    .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                if !present {
                    return Some(String::new());
                }
                rollup_field_value(index, field_ids, "EXPORTER_IP")
                    .map(compact_index_value_to_string)
            }
            "NEXT_HOP" => {
                let present = rollup_field_value(index, field_ids, INTERNAL_NEXT_HOP_PRESENT)
                    .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                if !present {
                    return Some(String::new());
                }
                rollup_field_value(index, field_ids, "NEXT_HOP").map(compact_index_value_to_string)
            }
            "DIRECTION" => {
                let present = rollup_field_value(index, field_ids, INTERNAL_DIRECTION_PRESENT)
                    .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                if !present {
                    return Some(String::new());
                }
                let value = rollup_field_value(index, field_ids, "DIRECTION")?;
                match value {
                    IndexFieldValue::U8(direction) => {
                        Some(direction_from_u8(direction).as_str().to_string())
                    }
                    _ => None,
                }
            }
            _ => {
                if let Some(internal_field) = rollup_presence_field(normalized.as_str()) {
                    let present = rollup_field_value(index, field_ids, internal_field)
                        .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                    if !present {
                        return Some(String::new());
                    }
                }
                rollup_field_value(index, field_ids, normalized.as_str())
                    .map(compact_index_value_to_string)
            }
        }
    }

    pub(crate) fn prune_unused_hours(&mut self, active_hours: &BTreeSet<u64>) {
        let before = self.indexes.len();
        self.indexes
            .retain(|hour_start_usec, _| active_hours.contains(hour_start_usec));
        if self.indexes.len() != before {
            self.generation = self.generation.saturating_add(1);
        }
    }
}

#[derive(Debug)]
pub(crate) struct TierAccumulator {
    bucket_usec: u64,
    buckets: BTreeMap<u64, MetricBucket>,
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

    pub(crate) fn observe_flow(
        &mut self,
        timestamp_usec: u64,
        flow_ref: TierFlowRef,
        metrics: FlowMetrics,
    ) {
        if timestamp_usec == 0 {
            return;
        }

        let bucket_start = bucket_start_usec(timestamp_usec, self.bucket_usec);
        let bucket = self.buckets.entry(bucket_start).or_default();
        bucket
            .entry(flow_ref)
            .and_modify(|existing| existing.add(metrics))
            .or_insert(metrics);
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
                for (flow_ref, metrics) in entries {
                    rows.push(OpenTierRow {
                        timestamp_usec: end.saturating_sub(1),
                        flow_ref,
                        metrics,
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

            for (&flow_ref, &metrics) in entries {
                rows.push(OpenTierRow {
                    timestamp_usec: now_usec.min(end.saturating_sub(1)),
                    flow_ref,
                    metrics,
                });
            }
        }
        rows
    }

    pub(crate) fn active_hours(&self) -> BTreeSet<u64> {
        let mut hours = BTreeSet::new();
        for entries in self.buckets.values() {
            for flow_ref in entries.keys() {
                hours.insert(flow_ref.hour_start_usec);
            }
        }
        hours
    }
}

fn build_rollup_flow_index() -> Result<FlowIndex, FlowIndexError> {
    FlowIndex::new(
        ROLLUP_FIELD_DEFS
            .iter()
            .map(|field| IndexFieldSpec::new(field.name, field.kind)),
    )
}

fn direction_to_u8(direction: FlowDirection) -> u8 {
    match direction {
        FlowDirection::Ingress => 0,
        FlowDirection::Egress => 1,
        FlowDirection::Undefined => 2,
    }
}

fn direction_from_u8(value: u8) -> FlowDirection {
    match value {
        0 => FlowDirection::Ingress,
        1 => FlowDirection::Egress,
        _ => FlowDirection::Undefined,
    }
}

fn push_rollup_field_ids(
    index: &mut FlowIndex,
    rec: &FlowRecord,
    scratch_field_ids: &mut Vec<u32>,
) -> Result<(), FlowIndexError> {
    let missing_ip = IpAddr::V4(Ipv4Addr::UNSPECIFIED);

    scratch_field_ids.push(
        index.get_or_insert_field_value(0, IndexFieldValue::U8(direction_to_u8(rec.direction)))?,
    );
    scratch_field_ids.push(index.get_or_insert_field_value(1, IndexFieldValue::U8(rec.protocol))?);
    scratch_field_ids.push(index.get_or_insert_field_value(2, IndexFieldValue::U16(rec.etype))?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(3, IndexFieldValue::U8(rec.forwarding_status))?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(4, IndexFieldValue::Text(rec.flow_version))?);
    scratch_field_ids.push(index.get_or_insert_field_value(5, IndexFieldValue::U8(rec.iptos))?);
    scratch_field_ids.push(index.get_or_insert_field_value(6, IndexFieldValue::U8(rec.tcp_flags))?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(7, IndexFieldValue::U8(rec.icmpv4_type))?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(8, IndexFieldValue::U8(rec.icmpv4_code))?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(9, IndexFieldValue::U8(rec.icmpv6_type))?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(10, IndexFieldValue::U8(rec.icmpv6_code))?);
    scratch_field_ids.push(index.get_or_insert_field_value(11, IndexFieldValue::U32(rec.src_as))?);
    scratch_field_ids.push(index.get_or_insert_field_value(12, IndexFieldValue::U32(rec.dst_as))?);
    scratch_field_ids.push(
        index.get_or_insert_field_value(13, IndexFieldValue::Text(rec.src_as_name.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(14, IndexFieldValue::Text(rec.dst_as_name.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(
            15,
            IndexFieldValue::U8(u8::from(rec.exporter_ip.is_some())),
        )?,
    );
    scratch_field_ids.push(index.get_or_insert_field_value(
        16,
        IndexFieldValue::IpAddr(rec.exporter_ip.unwrap_or(missing_ip)),
    )?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(17, IndexFieldValue::U16(rec.exporter_port))?);
    scratch_field_ids.push(
        index.get_or_insert_field_value(18, IndexFieldValue::Text(rec.exporter_name.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(19, IndexFieldValue::Text(rec.exporter_group.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(20, IndexFieldValue::Text(rec.exporter_role.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(21, IndexFieldValue::Text(rec.exporter_site.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(22, IndexFieldValue::Text(rec.exporter_region.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(23, IndexFieldValue::Text(rec.exporter_tenant.as_str()))?,
    );
    scratch_field_ids.push(index.get_or_insert_field_value(24, IndexFieldValue::U32(rec.in_if))?);
    scratch_field_ids.push(index.get_or_insert_field_value(25, IndexFieldValue::U32(rec.out_if))?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(26, IndexFieldValue::Text(rec.in_if_name.as_str()))?);
    scratch_field_ids.push(
        index.get_or_insert_field_value(27, IndexFieldValue::Text(rec.out_if_name.as_str()))?,
    );
    scratch_field_ids.push(
        index
            .get_or_insert_field_value(28, IndexFieldValue::Text(rec.in_if_description.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(
            29,
            IndexFieldValue::Text(rec.out_if_description.as_str()),
        )?,
    );
    scratch_field_ids
        .push(index.get_or_insert_field_value(30, IndexFieldValue::U64(rec.in_if_speed))?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(31, IndexFieldValue::U64(rec.out_if_speed))?);
    scratch_field_ids.push(
        index.get_or_insert_field_value(32, IndexFieldValue::Text(rec.in_if_provider.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(33, IndexFieldValue::Text(rec.out_if_provider.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(
            34,
            IndexFieldValue::Text(rec.in_if_connectivity.as_str()),
        )?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(
            35,
            IndexFieldValue::Text(rec.out_if_connectivity.as_str()),
        )?,
    );
    scratch_field_ids
        .push(index.get_or_insert_field_value(36, IndexFieldValue::U8(rec.in_if_boundary))?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(37, IndexFieldValue::U8(rec.out_if_boundary))?);
    scratch_field_ids.push(
        index.get_or_insert_field_value(38, IndexFieldValue::Text(rec.src_net_name.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(39, IndexFieldValue::Text(rec.dst_net_name.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(40, IndexFieldValue::Text(rec.src_net_role.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(41, IndexFieldValue::Text(rec.dst_net_role.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(42, IndexFieldValue::Text(rec.src_net_site.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(43, IndexFieldValue::Text(rec.dst_net_site.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(44, IndexFieldValue::Text(rec.src_net_region.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(45, IndexFieldValue::Text(rec.dst_net_region.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(46, IndexFieldValue::Text(rec.src_net_tenant.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(47, IndexFieldValue::Text(rec.dst_net_tenant.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(48, IndexFieldValue::Text(rec.src_country.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(49, IndexFieldValue::Text(rec.dst_country.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(50, IndexFieldValue::Text(rec.src_geo_city.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(51, IndexFieldValue::Text(rec.dst_geo_city.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(52, IndexFieldValue::Text(rec.src_geo_state.as_str()))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(53, IndexFieldValue::Text(rec.dst_geo_state.as_str()))?,
    );
    scratch_field_ids.push(
        index
            .get_or_insert_field_value(54, IndexFieldValue::U8(u8::from(rec.next_hop.is_some())))?,
    );
    scratch_field_ids.push(index.get_or_insert_field_value(
        55,
        IndexFieldValue::IpAddr(rec.next_hop.unwrap_or(missing_ip)),
    )?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(56, IndexFieldValue::U16(rec.src_vlan))?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(57, IndexFieldValue::U16(rec.dst_vlan))?);
    scratch_field_ids.push(
        index.get_or_insert_field_value(58, IndexFieldValue::U8(u8::from(rec.has_direction())))?,
    );
    scratch_field_ids
        .push(index.get_or_insert_field_value(59, IndexFieldValue::U8(u8::from(rec.has_etype())))?);
    scratch_field_ids.push(index.get_or_insert_field_value(
        60,
        IndexFieldValue::U8(u8::from(rec.has_forwarding_status())),
    )?);
    scratch_field_ids
        .push(index.get_or_insert_field_value(61, IndexFieldValue::U8(u8::from(rec.has_iptos())))?);
    scratch_field_ids.push(
        index.get_or_insert_field_value(62, IndexFieldValue::U8(u8::from(rec.has_tcp_flags())))?,
    );
    scratch_field_ids.push(
        index
            .get_or_insert_field_value(63, IndexFieldValue::U8(u8::from(rec.has_icmpv4_type())))?,
    );
    scratch_field_ids.push(
        index
            .get_or_insert_field_value(64, IndexFieldValue::U8(u8::from(rec.has_icmpv4_code())))?,
    );
    scratch_field_ids.push(
        index
            .get_or_insert_field_value(65, IndexFieldValue::U8(u8::from(rec.has_icmpv6_type())))?,
    );
    scratch_field_ids.push(
        index
            .get_or_insert_field_value(66, IndexFieldValue::U8(u8::from(rec.has_icmpv6_code())))?,
    );
    scratch_field_ids.push(
        index
            .get_or_insert_field_value(67, IndexFieldValue::U8(u8::from(rec.has_in_if_speed())))?,
    );
    scratch_field_ids.push(
        index
            .get_or_insert_field_value(68, IndexFieldValue::U8(u8::from(rec.has_out_if_speed())))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(
            69,
            IndexFieldValue::U8(u8::from(rec.has_in_if_boundary())),
        )?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(
            70,
            IndexFieldValue::U8(u8::from(rec.has_out_if_boundary())),
        )?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(71, IndexFieldValue::U8(u8::from(rec.has_src_vlan())))?,
    );
    scratch_field_ids.push(
        index.get_or_insert_field_value(72, IndexFieldValue::U8(u8::from(rec.has_dst_vlan())))?,
    );
    Ok(())
}

fn materialize_rollup_fields(index: &FlowIndex, flow_id: IndexedFlowId) -> Option<FlowFields> {
    let field_ids = index.flow_field_ids(flow_id)?;
    let mut fields = FlowFields::new();
    let mut exporter_ip_present = false;
    let mut next_hop_present = false;
    let mut exporter_ip = IpAddr::V4(Ipv4Addr::UNSPECIFIED);
    let mut next_hop = IpAddr::V4(Ipv4Addr::UNSPECIFIED);

    for (field_index, field_id) in field_ids.iter().copied().enumerate() {
        let name = ROLLUP_FIELD_DEFS.get(field_index)?.name;
        let value = index.field_value(field_index, field_id)?;
        match name {
            INTERNAL_EXPORTER_IP_PRESENT => {
                exporter_ip_present = matches!(value, IndexFieldValue::U8(1));
            }
            INTERNAL_NEXT_HOP_PRESENT => {
                next_hop_present = matches!(value, IndexFieldValue::U8(1));
            }
            name if is_internal_rollup_presence_field(name) => {}
            "EXPORTER_IP" => {
                if let IndexFieldValue::IpAddr(ip) = value {
                    exporter_ip = ip;
                }
            }
            "NEXT_HOP" => {
                if let IndexFieldValue::IpAddr(ip) = value {
                    next_hop = ip;
                }
            }
            "DIRECTION" => {
                if let IndexFieldValue::U8(direction) = value {
                    fields.insert(name, direction_from_u8(direction).as_str().to_string());
                }
            }
            _ => {
                fields.insert(name, compact_index_value_to_string(value));
            }
        }
    }

    fields.insert(
        "EXPORTER_IP",
        if exporter_ip_present {
            exporter_ip.to_string()
        } else {
            String::new()
        },
    );
    fields.insert(
        "NEXT_HOP",
        if next_hop_present {
            next_hop.to_string()
        } else {
            String::new()
        },
    );
    for &(field, internal_field) in ROLLUP_PRESENCE_FIELDS {
        let present = rollup_field_value(index, field_ids, internal_field)
            .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
        if !present {
            fields.insert(field, String::new());
        }
    }

    Some(fields)
}

pub(crate) fn rollup_field_supported(field: &str) -> bool {
    rollup_field_index(field).is_some()
}

fn rollup_field_index(field: &str) -> Option<usize> {
    ROLLUP_FIELD_DEFS
        .iter()
        .position(|def| def.name.eq_ignore_ascii_case(field))
}

fn rollup_field_value<'a>(
    index: &'a FlowIndex,
    field_ids: &[u32],
    field: &str,
) -> Option<IndexFieldValue<'a>> {
    let field_index = rollup_field_index(field)?;
    let field_id = *field_ids.get(field_index)?;
    index.field_value(field_index, field_id)
}

fn compact_index_value_to_string(value: IndexFieldValue<'_>) -> String {
    match value {
        IndexFieldValue::Text(text) => text.to_string(),
        IndexFieldValue::U8(number) => number.to_string(),
        IndexFieldValue::U16(number) => number.to_string(),
        IndexFieldValue::U32(number) => number.to_string(),
        IndexFieldValue::U64(number) => number.to_string(),
        IndexFieldValue::IpAddr(ip) => ip.to_string(),
    }
}

fn parse_u64(value: Option<&String>) -> u64 {
    value.and_then(|v| v.parse::<u64>().ok()).unwrap_or(0)
}

fn bucket_start_usec(timestamp_usec: u64, bucket_usec: u64) -> u64 {
    (timestamp_usec / bucket_usec).saturating_mul(bucket_usec)
}

#[cfg(test)]
pub(crate) fn dimensions_for_rollup(fields: &FlowFields) -> FlowFields {
    const METRIC_FIELDS: [&str; 4] = ["BYTES", "PACKETS", "RAW_BYTES", "RAW_PACKETS"];
    const DEBUG_PREFIXES: [&str; 2] = ["V9_", "IPFIX_"];

    fields
        .iter()
        .filter(|&(&name, _)| {
            !name.starts_with('_')
                && !METRIC_FIELDS.contains(&name)
                && !DEBUG_PREFIXES.iter().any(|p| name.starts_with(p))
        })
        .map(|(&name, value)| (name, value.clone()))
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::BTreeMap;

    fn materialize_row_fields(store: &TierFlowIndexStore, row: &OpenTierRow) -> FlowFields {
        let mut fields = store
            .materialize_fields(row.flow_ref)
            .expect("materialize row fields");
        row.metrics.write_fields(&mut fields);
        fields
    }

    #[test]
    fn accumulator_flushes_closed_bucket() {
        let mut acc = TierAccumulator::new(TierKind::Minute1);
        let mut store = TierFlowIndexStore::default();
        let ts = 120_000_000;

        let mut rec = FlowRecord::default();
        rec.protocol = 6;
        rec.src_addr = Some("10.0.0.1".parse().unwrap());
        rec.dst_addr = Some("10.0.0.2".parse().unwrap());
        rec.src_port = 12345;
        rec.dst_port = 443;
        rec.bytes = 100;
        rec.packets = 2;
        rec.flows = 1;

        let flow_ref = store
            .get_or_insert_record_flow(ts, &rec)
            .expect("intern tier flow");
        acc.observe_flow(ts, flow_ref, FlowMetrics::from_record(&rec));

        let rows = acc.flush_closed_rows(180_000_000);
        assert_eq!(rows.len(), 1);
        let fields = materialize_row_fields(&store, &rows[0]);
        assert_eq!(fields.get("PROTOCOL").map(String::as_str), Some("6"));
        assert_eq!(fields.get("BYTES").map(String::as_str), Some("100"));
        assert!(fields.get("SRC_ADDR").is_none());
        assert!(fields.get("DST_ADDR").is_none());
        assert!(fields.get("SRC_PORT").is_none());
        assert!(fields.get("DST_PORT").is_none());
    }

    #[test]
    fn metrics_defaults_to_zero() {
        let fields = BTreeMap::new();
        let m = FlowMetrics::from_fields(&fields);
        assert_eq!(m.bytes, 0);
        assert_eq!(m.packets, 0);
    }

    #[test]
    fn metrics_from_record_matches_from_fields() {
        let mut rec = FlowRecord::default();
        rec.bytes = 12345;
        rec.packets = 67;

        let fields = rec.to_fields();
        let m_fields = FlowMetrics::from_fields(&fields);
        let m_record = FlowMetrics::from_record(&rec);

        assert_eq!(m_fields, m_record);
    }

    #[test]
    fn rollup_dimensions_round_trip() {
        let mut store = TierFlowIndexStore::default();
        let mut rec = FlowRecord::default();
        rec.flow_version = "v9";
        rec.exporter_ip = Some("192.0.2.10".parse().unwrap());
        rec.exporter_port = 12345;
        rec.protocol = 6;
        rec.set_etype(2048);
        rec.src_as = 64512;
        rec.dst_as = 15169;
        rec.src_as_name = "AS64512 Example Transit".to_string();
        rec.dst_as_name = "AS15169 Google LLC".to_string();
        rec.in_if = 10;
        rec.out_if = 20;
        rec.set_sampling_rate(100);
        rec.set_direction(FlowDirection::Ingress);
        rec.src_country = "US".to_string();
        rec.dst_country = "DE".to_string();

        let flow_ref = store
            .get_or_insert_record_flow(120_000_000, &rec)
            .expect("intern tier flow");
        let fields = store
            .materialize_fields(flow_ref)
            .expect("materialize fields");
        assert_eq!(fields.get("PROTOCOL").map(String::as_str), Some("6"));
        assert_eq!(fields.get("ETYPE").map(String::as_str), Some("2048"));
        assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64512"));
        assert_eq!(fields.get("DST_AS").map(String::as_str), Some("15169"));
        assert_eq!(
            fields.get("SRC_AS_NAME").map(String::as_str),
            Some("AS64512 Example Transit")
        );
        assert_eq!(
            fields.get("DST_AS_NAME").map(String::as_str),
            Some("AS15169 Google LLC")
        );
        assert_eq!(fields.get("SRC_COUNTRY").map(String::as_str), Some("US"));
        assert_eq!(fields.get("DST_COUNTRY").map(String::as_str), Some("DE"));
        assert_eq!(fields.get("DIRECTION").map(String::as_str), Some("ingress"));
        assert!(fields.get("SAMPLING_RATE").is_none());
    }

    #[test]
    fn indexed_field_lookup_matches_rollup_materialization_semantics() {
        let mut store = TierFlowIndexStore::default();
        let mut rec = FlowRecord::default();
        rec.set_direction(FlowDirection::Ingress);
        rec.protocol = 6;
        rec.src_as_name = "AS0 Private IP Address Space".to_string();

        let flow_ref = store
            .get_or_insert_record_flow(120_000_000, &rec)
            .expect("intern tier flow");

        assert_eq!(
            store.field_value_string(flow_ref, "DIRECTION").as_deref(),
            Some("ingress")
        );
        assert_eq!(
            store.field_value_string(flow_ref, "PROTOCOL").as_deref(),
            Some("6")
        );
        assert_eq!(
            store.field_value_string(flow_ref, "SRC_AS_NAME").as_deref(),
            Some("AS0 Private IP Address Space")
        );
        assert_eq!(
            store.field_value_string(flow_ref, "EXPORTER_IP").as_deref(),
            Some("")
        );
        assert_eq!(
            store.field_value_string(flow_ref, "NEXT_HOP").as_deref(),
            Some("")
        );
    }

    #[test]
    fn same_dimensions_aggregate() {
        let mut acc = TierAccumulator::new(TierKind::Minute1);
        let mut store = TierFlowIndexStore::default();
        let ts = 120_000_000;

        let mut rec = FlowRecord::default();
        rec.protocol = 6;
        rec.bytes = 100;
        rec.packets = 2;

        let flow_ref = store
            .get_or_insert_record_flow(ts, &rec)
            .expect("intern tier flow");
        acc.observe_flow(ts, flow_ref, FlowMetrics::from_record(&rec));

        rec.bytes = 200;
        rec.packets = 3;
        let same_flow_ref = store
            .get_or_insert_record_flow(ts, &rec)
            .expect("reuse tier flow");
        assert_eq!(flow_ref, same_flow_ref);
        acc.observe_flow(ts, same_flow_ref, FlowMetrics::from_record(&rec));

        let rows = acc.flush_closed_rows(180_000_000);
        assert_eq!(rows.len(), 1);
        let fields = materialize_row_fields(&store, &rows[0]);
        assert_eq!(fields.get("BYTES").map(String::as_str), Some("300"));
        assert_eq!(fields.get("PACKETS").map(String::as_str), Some("5"));
    }

    #[test]
    fn different_dimensions_separate() {
        let mut acc = TierAccumulator::new(TierKind::Minute1);
        let mut store = TierFlowIndexStore::default();
        let ts = 120_000_000;

        let mut rec1 = FlowRecord::default();
        rec1.protocol = 6;
        rec1.bytes = 100;
        rec1.packets = 2;

        let mut rec2 = FlowRecord::default();
        rec2.protocol = 17;
        rec2.bytes = 200;
        rec2.packets = 3;

        let flow_ref_1 = store
            .get_or_insert_record_flow(ts, &rec1)
            .expect("intern first flow");
        let flow_ref_2 = store
            .get_or_insert_record_flow(ts, &rec2)
            .expect("intern second flow");
        acc.observe_flow(ts, flow_ref_1, FlowMetrics::from_record(&rec1));
        acc.observe_flow(ts, flow_ref_2, FlowMetrics::from_record(&rec2));

        let rows = acc.flush_closed_rows(180_000_000);
        assert_eq!(rows.len(), 2);
    }
}
