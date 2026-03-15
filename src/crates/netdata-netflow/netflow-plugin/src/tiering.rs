use crate::decoder::{FlowDirection, FlowFields, FlowRecord};
use std::collections::{BTreeMap, HashMap};
use std::hash::{Hash, Hasher};
use std::net::IpAddr;
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
    pub(crate) flows: u64,
    pub(crate) raw_bytes: u64,
    pub(crate) raw_packets: u64,
}

impl FlowMetrics {
    pub(crate) fn from_fields(fields: &FlowFields) -> Self {
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

    /// Extract metrics directly from a FlowRecord — zero-alloc, no string parsing.
    pub(crate) fn from_record(rec: &FlowRecord) -> Self {
        Self {
            bytes: rec.bytes,
            packets: rec.packets,
            flows: rec.flows.max(1),
            raw_bytes: rec.raw_bytes,
            raw_packets: rec.raw_packets,
        }
    }

    pub(crate) fn add(&mut self, other: FlowMetrics) {
        self.bytes = self.bytes.saturating_add(other.bytes);
        self.packets = self.packets.saturating_add(other.packets);
        self.flows = self.flows.saturating_add(other.flows);
        self.raw_bytes = self.raw_bytes.saturating_add(other.raw_bytes);
        self.raw_packets = self.raw_packets.saturating_add(other.raw_packets);
    }

    pub(crate) fn write_fields(self, fields: &mut FlowFields) {
        fields.insert("BYTES", self.bytes.to_string());
        fields.insert("PACKETS", self.packets.to_string());
        fields.insert("FLOWS", self.flows.to_string());
        fields.insert("RAW_BYTES", self.raw_bytes.to_string());
        fields.insert("RAW_PACKETS", self.raw_packets.to_string());
    }
}

// ---------------------------------------------------------------------------
// RollupDimensions: native-typed struct for tier aggregation.
// Only traffic-classification fields — no per-flow timestamps, MACs,
// NAT addresses, fragment IDs, TTL, prefixes, MPLS, or BGP path details.
// Hash + comparison operate on native fields directly = zero allocation.
// ---------------------------------------------------------------------------

#[derive(Debug, Clone)]
pub(crate) struct RollupDimensions {
    // Traffic classification
    pub(crate) direction: FlowDirection,
    pub(crate) protocol: u8,
    pub(crate) etype: u16,
    pub(crate) forwarding_status: u8,
    pub(crate) flow_version: &'static str,
    pub(crate) iptos: u8,
    pub(crate) tcp_flags: u8,
    pub(crate) icmpv4_type: u8,
    pub(crate) icmpv4_code: u8,
    pub(crate) icmpv6_type: u8,
    pub(crate) icmpv6_code: u8,

    // AS numbers
    pub(crate) src_as: u32,
    pub(crate) dst_as: u32,
    pub(crate) src_as_name: String,
    pub(crate) dst_as_name: String,

    // Exporter identity
    pub(crate) exporter_ip: Option<IpAddr>,
    pub(crate) exporter_port: u16,
    pub(crate) exporter_name: String,
    pub(crate) exporter_group: String,
    pub(crate) exporter_role: String,
    pub(crate) exporter_site: String,
    pub(crate) exporter_region: String,
    pub(crate) exporter_tenant: String,

    // Interfaces
    pub(crate) in_if: u32,
    pub(crate) out_if: u32,
    pub(crate) in_if_name: String,
    pub(crate) out_if_name: String,
    pub(crate) in_if_description: String,
    pub(crate) out_if_description: String,
    pub(crate) in_if_speed: u64,
    pub(crate) out_if_speed: u64,
    pub(crate) in_if_provider: String,
    pub(crate) out_if_provider: String,
    pub(crate) in_if_connectivity: String,
    pub(crate) out_if_connectivity: String,
    pub(crate) in_if_boundary: u8,
    pub(crate) out_if_boundary: u8,

    // Network attributes
    pub(crate) src_net_name: String,
    pub(crate) dst_net_name: String,
    pub(crate) src_net_role: String,
    pub(crate) dst_net_role: String,
    pub(crate) src_net_site: String,
    pub(crate) dst_net_site: String,
    pub(crate) src_net_region: String,
    pub(crate) dst_net_region: String,
    pub(crate) src_net_tenant: String,
    pub(crate) dst_net_tenant: String,

    // Geo
    pub(crate) src_country: String,
    pub(crate) dst_country: String,
    pub(crate) src_geo_city: String,
    pub(crate) dst_geo_city: String,
    pub(crate) src_geo_state: String,
    pub(crate) dst_geo_state: String,

    // Routing / transport
    pub(crate) next_hop: Option<IpAddr>,
    pub(crate) src_vlan: u16,
    pub(crate) dst_vlan: u16,
    pub(crate) sampling_rate: u64,
}

impl RollupDimensions {
    /// Extract rollup dimensions from a FlowRecord. Copy types are free;
    /// String fields clone from the record (only happens on first-seen combo).
    pub(crate) fn from_record(rec: &FlowRecord) -> Self {
        Self {
            direction: rec.direction,
            protocol: rec.protocol,
            etype: rec.etype,
            forwarding_status: rec.forwarding_status,
            flow_version: rec.flow_version,
            iptos: rec.iptos,
            tcp_flags: rec.tcp_flags,
            icmpv4_type: rec.icmpv4_type,
            icmpv4_code: rec.icmpv4_code,
            icmpv6_type: rec.icmpv6_type,
            icmpv6_code: rec.icmpv6_code,
            src_as: rec.src_as,
            dst_as: rec.dst_as,
            src_as_name: rec.src_as_name.clone(),
            dst_as_name: rec.dst_as_name.clone(),
            exporter_ip: rec.exporter_ip,
            exporter_port: rec.exporter_port,
            exporter_name: rec.exporter_name.clone(),
            exporter_group: rec.exporter_group.clone(),
            exporter_role: rec.exporter_role.clone(),
            exporter_site: rec.exporter_site.clone(),
            exporter_region: rec.exporter_region.clone(),
            exporter_tenant: rec.exporter_tenant.clone(),
            in_if: rec.in_if,
            out_if: rec.out_if,
            in_if_name: rec.in_if_name.clone(),
            out_if_name: rec.out_if_name.clone(),
            in_if_description: rec.in_if_description.clone(),
            out_if_description: rec.out_if_description.clone(),
            in_if_speed: rec.in_if_speed,
            out_if_speed: rec.out_if_speed,
            in_if_provider: rec.in_if_provider.clone(),
            out_if_provider: rec.out_if_provider.clone(),
            in_if_connectivity: rec.in_if_connectivity.clone(),
            out_if_connectivity: rec.out_if_connectivity.clone(),
            in_if_boundary: rec.in_if_boundary,
            out_if_boundary: rec.out_if_boundary,
            src_net_name: rec.src_net_name.clone(),
            dst_net_name: rec.dst_net_name.clone(),
            src_net_role: rec.src_net_role.clone(),
            dst_net_role: rec.dst_net_role.clone(),
            src_net_site: rec.src_net_site.clone(),
            dst_net_site: rec.dst_net_site.clone(),
            src_net_region: rec.src_net_region.clone(),
            dst_net_region: rec.dst_net_region.clone(),
            src_net_tenant: rec.src_net_tenant.clone(),
            dst_net_tenant: rec.dst_net_tenant.clone(),
            src_country: rec.src_country.clone(),
            dst_country: rec.dst_country.clone(),
            src_geo_city: rec.src_geo_city.clone(),
            dst_geo_city: rec.dst_geo_city.clone(),
            src_geo_state: rec.src_geo_state.clone(),
            dst_geo_state: rec.dst_geo_state.clone(),
            next_hop: rec.next_hop,
            src_vlan: rec.src_vlan,
            dst_vlan: rec.dst_vlan,
            sampling_rate: rec.sampling_rate,
        }
    }

    /// Check if this dimensions entry matches a FlowRecord. Zero-alloc:
    /// compares native fields directly without string conversion.
    pub(crate) fn matches_record(&self, rec: &FlowRecord) -> bool {
        self.direction == rec.direction
            && self.protocol == rec.protocol
            && self.etype == rec.etype
            && self.forwarding_status == rec.forwarding_status
            && self.flow_version == rec.flow_version
            && self.iptos == rec.iptos
            && self.tcp_flags == rec.tcp_flags
            && self.icmpv4_type == rec.icmpv4_type
            && self.icmpv4_code == rec.icmpv4_code
            && self.icmpv6_type == rec.icmpv6_type
            && self.icmpv6_code == rec.icmpv6_code
            && self.src_as == rec.src_as
            && self.dst_as == rec.dst_as
            && self.src_as_name == rec.src_as_name
            && self.dst_as_name == rec.dst_as_name
            && self.exporter_ip == rec.exporter_ip
            && self.exporter_port == rec.exporter_port
            && self.exporter_name == rec.exporter_name
            && self.exporter_group == rec.exporter_group
            && self.exporter_role == rec.exporter_role
            && self.exporter_site == rec.exporter_site
            && self.exporter_region == rec.exporter_region
            && self.exporter_tenant == rec.exporter_tenant
            && self.in_if == rec.in_if
            && self.out_if == rec.out_if
            && self.in_if_name == rec.in_if_name
            && self.out_if_name == rec.out_if_name
            && self.in_if_description == rec.in_if_description
            && self.out_if_description == rec.out_if_description
            && self.in_if_speed == rec.in_if_speed
            && self.out_if_speed == rec.out_if_speed
            && self.in_if_provider == rec.in_if_provider
            && self.out_if_provider == rec.out_if_provider
            && self.in_if_connectivity == rec.in_if_connectivity
            && self.out_if_connectivity == rec.out_if_connectivity
            && self.in_if_boundary == rec.in_if_boundary
            && self.out_if_boundary == rec.out_if_boundary
            && self.src_net_name == rec.src_net_name
            && self.dst_net_name == rec.dst_net_name
            && self.src_net_role == rec.src_net_role
            && self.dst_net_role == rec.dst_net_role
            && self.src_net_site == rec.src_net_site
            && self.dst_net_site == rec.dst_net_site
            && self.src_net_region == rec.src_net_region
            && self.dst_net_region == rec.dst_net_region
            && self.src_net_tenant == rec.src_net_tenant
            && self.dst_net_tenant == rec.dst_net_tenant
            && self.src_country == rec.src_country
            && self.dst_country == rec.dst_country
            && self.src_geo_city == rec.src_geo_city
            && self.dst_geo_city == rec.dst_geo_city
            && self.src_geo_state == rec.src_geo_state
            && self.dst_geo_state == rec.dst_geo_state
            && self.next_hop == rec.next_hop
            && self.src_vlan == rec.src_vlan
            && self.dst_vlan == rec.dst_vlan
            && self.sampling_rate == rec.sampling_rate
    }

    /// Convert to FlowFields for tier journal output (cold path).
    pub(crate) fn to_fields(&self) -> FlowFields {
        let mut f = BTreeMap::new();
        let mut ibuf = itoa::Buffer::new();

        f.insert("DIRECTION", self.direction.as_str().to_string());
        f.insert("PROTOCOL", ibuf.format(self.protocol as u64).to_string());
        f.insert("ETYPE", ibuf.format(self.etype as u64).to_string());
        f.insert("FORWARDING_STATUS", ibuf.format(self.forwarding_status as u64).to_string());
        f.insert("FLOW_VERSION", self.flow_version.to_string());
        f.insert("IPTOS", ibuf.format(self.iptos as u64).to_string());
        f.insert("TCP_FLAGS", ibuf.format(self.tcp_flags as u64).to_string());
        f.insert("ICMPV4_TYPE", ibuf.format(self.icmpv4_type as u64).to_string());
        f.insert("ICMPV4_CODE", ibuf.format(self.icmpv4_code as u64).to_string());
        f.insert("ICMPV6_TYPE", ibuf.format(self.icmpv6_type as u64).to_string());
        f.insert("ICMPV6_CODE", ibuf.format(self.icmpv6_code as u64).to_string());
        f.insert("SRC_AS", ibuf.format(self.src_as as u64).to_string());
        f.insert("DST_AS", ibuf.format(self.dst_as as u64).to_string());
        f.insert("SRC_AS_NAME", self.src_as_name.clone());
        f.insert("DST_AS_NAME", self.dst_as_name.clone());
        f.insert("EXPORTER_IP", self.exporter_ip.map(|ip| ip.to_string()).unwrap_or_default());
        f.insert("EXPORTER_PORT", ibuf.format(self.exporter_port as u64).to_string());
        f.insert("EXPORTER_NAME", self.exporter_name.clone());
        f.insert("EXPORTER_GROUP", self.exporter_group.clone());
        f.insert("EXPORTER_ROLE", self.exporter_role.clone());
        f.insert("EXPORTER_SITE", self.exporter_site.clone());
        f.insert("EXPORTER_REGION", self.exporter_region.clone());
        f.insert("EXPORTER_TENANT", self.exporter_tenant.clone());
        f.insert("IN_IF", ibuf.format(self.in_if as u64).to_string());
        f.insert("OUT_IF", ibuf.format(self.out_if as u64).to_string());
        f.insert("IN_IF_NAME", self.in_if_name.clone());
        f.insert("OUT_IF_NAME", self.out_if_name.clone());
        f.insert("IN_IF_DESCRIPTION", self.in_if_description.clone());
        f.insert("OUT_IF_DESCRIPTION", self.out_if_description.clone());
        f.insert("IN_IF_SPEED", ibuf.format(self.in_if_speed).to_string());
        f.insert("OUT_IF_SPEED", ibuf.format(self.out_if_speed).to_string());
        f.insert("IN_IF_PROVIDER", self.in_if_provider.clone());
        f.insert("OUT_IF_PROVIDER", self.out_if_provider.clone());
        f.insert("IN_IF_CONNECTIVITY", self.in_if_connectivity.clone());
        f.insert("OUT_IF_CONNECTIVITY", self.out_if_connectivity.clone());
        f.insert("IN_IF_BOUNDARY", ibuf.format(self.in_if_boundary as u64).to_string());
        f.insert("OUT_IF_BOUNDARY", ibuf.format(self.out_if_boundary as u64).to_string());
        f.insert("SRC_NET_NAME", self.src_net_name.clone());
        f.insert("DST_NET_NAME", self.dst_net_name.clone());
        f.insert("SRC_NET_ROLE", self.src_net_role.clone());
        f.insert("DST_NET_ROLE", self.dst_net_role.clone());
        f.insert("SRC_NET_SITE", self.src_net_site.clone());
        f.insert("DST_NET_SITE", self.dst_net_site.clone());
        f.insert("SRC_NET_REGION", self.src_net_region.clone());
        f.insert("DST_NET_REGION", self.dst_net_region.clone());
        f.insert("SRC_NET_TENANT", self.src_net_tenant.clone());
        f.insert("DST_NET_TENANT", self.dst_net_tenant.clone());
        f.insert("SRC_COUNTRY", self.src_country.clone());
        f.insert("DST_COUNTRY", self.dst_country.clone());
        f.insert("SRC_GEO_CITY", self.src_geo_city.clone());
        f.insert("DST_GEO_CITY", self.dst_geo_city.clone());
        f.insert("SRC_GEO_STATE", self.src_geo_state.clone());
        f.insert("DST_GEO_STATE", self.dst_geo_state.clone());
        f.insert("NEXT_HOP", self.next_hop.map(|ip| ip.to_string()).unwrap_or_default());
        f.insert("SRC_VLAN", ibuf.format(self.src_vlan as u64).to_string());
        f.insert("DST_VLAN", ibuf.format(self.dst_vlan as u64).to_string());
        f.insert("SAMPLING_RATE", ibuf.format(self.sampling_rate).to_string());

        f
    }
}

/// Hash rollup dimension fields directly from a FlowRecord — zero allocation.
pub(crate) fn rollup_dimension_hash(rec: &FlowRecord) -> u64 {
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    rec.direction.hash(&mut hasher);
    rec.protocol.hash(&mut hasher);
    rec.etype.hash(&mut hasher);
    rec.forwarding_status.hash(&mut hasher);
    rec.flow_version.hash(&mut hasher);
    rec.iptos.hash(&mut hasher);
    rec.tcp_flags.hash(&mut hasher);
    rec.icmpv4_type.hash(&mut hasher);
    rec.icmpv4_code.hash(&mut hasher);
    rec.icmpv6_type.hash(&mut hasher);
    rec.icmpv6_code.hash(&mut hasher);
    rec.src_as.hash(&mut hasher);
    rec.dst_as.hash(&mut hasher);
    rec.src_as_name.hash(&mut hasher);
    rec.dst_as_name.hash(&mut hasher);
    rec.exporter_ip.hash(&mut hasher);
    rec.exporter_port.hash(&mut hasher);
    rec.exporter_name.hash(&mut hasher);
    rec.exporter_group.hash(&mut hasher);
    rec.exporter_role.hash(&mut hasher);
    rec.exporter_site.hash(&mut hasher);
    rec.exporter_region.hash(&mut hasher);
    rec.exporter_tenant.hash(&mut hasher);
    rec.in_if.hash(&mut hasher);
    rec.out_if.hash(&mut hasher);
    rec.in_if_name.hash(&mut hasher);
    rec.out_if_name.hash(&mut hasher);
    rec.in_if_description.hash(&mut hasher);
    rec.out_if_description.hash(&mut hasher);
    rec.in_if_speed.hash(&mut hasher);
    rec.out_if_speed.hash(&mut hasher);
    rec.in_if_provider.hash(&mut hasher);
    rec.out_if_provider.hash(&mut hasher);
    rec.in_if_connectivity.hash(&mut hasher);
    rec.out_if_connectivity.hash(&mut hasher);
    rec.in_if_boundary.hash(&mut hasher);
    rec.out_if_boundary.hash(&mut hasher);
    rec.src_net_name.hash(&mut hasher);
    rec.dst_net_name.hash(&mut hasher);
    rec.src_net_role.hash(&mut hasher);
    rec.dst_net_role.hash(&mut hasher);
    rec.src_net_site.hash(&mut hasher);
    rec.dst_net_site.hash(&mut hasher);
    rec.src_net_region.hash(&mut hasher);
    rec.dst_net_region.hash(&mut hasher);
    rec.src_net_tenant.hash(&mut hasher);
    rec.dst_net_tenant.hash(&mut hasher);
    rec.src_country.hash(&mut hasher);
    rec.dst_country.hash(&mut hasher);
    rec.src_geo_city.hash(&mut hasher);
    rec.dst_geo_city.hash(&mut hasher);
    rec.src_geo_state.hash(&mut hasher);
    rec.dst_geo_state.hash(&mut hasher);
    rec.next_hop.hash(&mut hasher);
    rec.src_vlan.hash(&mut hasher);
    rec.dst_vlan.hash(&mut hasher);
    rec.sampling_rate.hash(&mut hasher);
    hasher.finish()
}

// ---------------------------------------------------------------------------
// Tier accumulator
// ---------------------------------------------------------------------------

#[derive(Debug, Clone)]
pub(crate) struct OpenTierRow {
    pub(crate) timestamp_usec: u64,
    pub(crate) fields: FlowFields,
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

/// Per-bucket storage: dimension hash → collision chain of (dims, metrics).
/// Collisions are vanishingly rare with 64-bit SipHash over ~50 fields.
type DimensionBucket = HashMap<u64, Vec<(RollupDimensions, FlowMetrics)>>;

#[derive(Debug)]
pub(crate) struct TierAccumulator {
    bucket_usec: u64,
    buckets: BTreeMap<u64, DimensionBucket>,
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

    /// Hot path: observe a flow from a FlowRecord. Zero allocations when
    /// the dimension combination already exists in the current time bucket.
    pub(crate) fn observe_record(
        &mut self,
        timestamp_usec: u64,
        rec: &FlowRecord,
        dim_hash: u64,
        metrics: FlowMetrics,
    ) {
        if timestamp_usec == 0 {
            return;
        }

        let bucket_start = bucket_start_usec(timestamp_usec, self.bucket_usec);
        let bucket = self.buckets.entry(bucket_start).or_default();

        if let Some(chain) = bucket.get_mut(&dim_hash) {
            for (dims, existing_metrics) in chain.iter_mut() {
                if dims.matches_record(rec) {
                    existing_metrics.add(metrics);
                    return;
                }
            }
            // Hash collision with different dimensions (extremely rare).
            chain.push((RollupDimensions::from_record(rec), metrics));
        } else {
            // First time seeing this dimension combination in this bucket.
            bucket.insert(dim_hash, vec![(RollupDimensions::from_record(rec), metrics)]);
        }
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
                for (_hash, chain) in entries {
                    for (dims, metrics) in chain {
                        let mut fields = dims.to_fields();
                        metrics.write_fields(&mut fields);
                        rows.push(OpenTierRow {
                            timestamp_usec: end.saturating_sub(1),
                            fields,
                        });
                    }
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

            for (_hash, chain) in entries {
                for (dims, metrics) in chain {
                    let mut fields = dims.to_fields();
                    metrics.write_fields(&mut fields);
                    rows.push(OpenTierRow {
                        timestamp_usec: now_usec.min(end.saturating_sub(1)),
                        fields,
                    });
                }
            }
        }
        rows
    }
}

fn parse_u64(value: Option<&String>) -> u64 {
    value.and_then(|v| v.parse::<u64>().ok()).unwrap_or(0)
}

fn bucket_start_usec(timestamp_usec: u64, bucket_usec: u64) -> u64 {
    (timestamp_usec / bucket_usec).saturating_mul(bucket_usec)
}

/// Extract dimension fields from FlowFields by removing internal (_*),
/// debug (V9_*, IPFIX_*), and metric fields. Used by query tests only.
#[cfg(test)]
pub(crate) fn dimensions_for_rollup(fields: &FlowFields) -> FlowFields {
    const METRIC_FIELDS: [&str; 5] = ["BYTES", "PACKETS", "FLOWS", "RAW_BYTES", "RAW_PACKETS"];
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
    use crate::decoder::FlowRecord;
    use std::collections::BTreeMap;

    #[test]
    fn accumulator_flushes_closed_bucket() {
        let mut acc = TierAccumulator::new(TierKind::Minute1);
        let ts = 120_000_000; // minute 2
        let mut rec = FlowRecord::default();
        rec.protocol = 6;
        rec.src_addr = Some("10.0.0.1".parse().unwrap());
        rec.dst_addr = Some("10.0.0.2".parse().unwrap());
        rec.src_port = 12345;
        rec.dst_port = 443;
        rec.bytes = 100;
        rec.packets = 2;
        rec.flows = 1;

        let hash = rollup_dimension_hash(&rec);
        let metrics = FlowMetrics::from_record(&rec);
        acc.observe_record(ts, &rec, hash, metrics);

        let rows = acc.flush_closed_rows(180_000_000);
        assert_eq!(rows.len(), 1);
        assert_eq!(
            rows[0].fields.get("PROTOCOL").map(String::as_str),
            Some("6")
        );
        assert_eq!(rows[0].fields.get("BYTES").map(String::as_str), Some("100"));
        // SRC_ADDR, DST_ADDR, SRC_PORT, DST_PORT are not in RollupDimensions
        assert!(rows[0].fields.get("SRC_ADDR").is_none());
        assert!(rows[0].fields.get("DST_ADDR").is_none());
        assert!(rows[0].fields.get("SRC_PORT").is_none());
        assert!(rows[0].fields.get("DST_PORT").is_none());
    }

    #[test]
    fn metrics_defaults_flows_to_one() {
        let fields = BTreeMap::new();
        let m = FlowMetrics::from_fields(&fields);
        assert_eq!(m.flows, 1);
    }

    #[test]
    fn metrics_from_record_matches_from_fields() {
        let mut rec = FlowRecord::default();
        rec.bytes = 12345;
        rec.packets = 67;
        rec.flows = 3;
        rec.raw_bytes = 12345;
        rec.raw_packets = 67;

        let fields = rec.to_fields();
        let m_fields = FlowMetrics::from_fields(&fields);
        let m_record = FlowMetrics::from_record(&rec);

        assert_eq!(m_fields, m_record);
    }

    #[test]
    fn rollup_dimensions_round_trip() {
        let mut rec = FlowRecord::default();
        rec.flow_version = "v9";
        rec.exporter_ip = Some("192.0.2.10".parse().unwrap());
        rec.exporter_port = 12345;
        rec.protocol = 6;
        rec.etype = 2048;
        rec.src_as = 64512;
        rec.dst_as = 15169;
        rec.src_as_name = "Example Transit".to_string();
        rec.dst_as_name = "Google LLC".to_string();
        rec.in_if = 10;
        rec.out_if = 20;
        rec.sampling_rate = 100;
        rec.direction = crate::decoder::FlowDirection::Ingress;
        rec.src_country = "US".to_string();
        rec.dst_country = "DE".to_string();

        let dims = RollupDimensions::from_record(&rec);
        assert!(dims.matches_record(&rec));

        let fields = dims.to_fields();
        assert_eq!(fields.get("PROTOCOL").map(String::as_str), Some("6"));
        assert_eq!(fields.get("ETYPE").map(String::as_str), Some("2048"));
        assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64512"));
        assert_eq!(fields.get("DST_AS").map(String::as_str), Some("15169"));
        assert_eq!(
            fields.get("SRC_AS_NAME").map(String::as_str),
            Some("Example Transit")
        );
        assert_eq!(
            fields.get("DST_AS_NAME").map(String::as_str),
            Some("Google LLC")
        );
        assert_eq!(fields.get("SRC_COUNTRY").map(String::as_str), Some("US"));
        assert_eq!(fields.get("DST_COUNTRY").map(String::as_str), Some("DE"));
        assert_eq!(fields.get("DIRECTION").map(String::as_str), Some("ingress"));
        assert_eq!(fields.get("SAMPLING_RATE").map(String::as_str), Some("100"));
    }

    #[test]
    fn same_dimensions_aggregate() {
        let mut acc = TierAccumulator::new(TierKind::Minute1);
        let ts = 120_000_000;

        let mut rec = FlowRecord::default();
        rec.protocol = 6;
        rec.bytes = 100;
        rec.packets = 2;

        let hash = rollup_dimension_hash(&rec);
        acc.observe_record(ts, &rec, hash, FlowMetrics::from_record(&rec));

        // Same dimensions, different metrics
        rec.bytes = 200;
        rec.packets = 3;
        acc.observe_record(ts, &rec, hash, FlowMetrics::from_record(&rec));

        let rows = acc.flush_closed_rows(180_000_000);
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].fields.get("BYTES").map(String::as_str), Some("300"));
        assert_eq!(rows[0].fields.get("PACKETS").map(String::as_str), Some("5"));
    }

    #[test]
    fn different_dimensions_separate() {
        let mut acc = TierAccumulator::new(TierKind::Minute1);
        let ts = 120_000_000;

        let mut rec1 = FlowRecord::default();
        rec1.protocol = 6;
        rec1.bytes = 100;
        rec1.packets = 2;

        let mut rec2 = FlowRecord::default();
        rec2.protocol = 17; // UDP vs TCP
        rec2.bytes = 200;
        rec2.packets = 3;

        let hash1 = rollup_dimension_hash(&rec1);
        let hash2 = rollup_dimension_hash(&rec2);
        acc.observe_record(ts, &rec1, hash1, FlowMetrics::from_record(&rec1));
        acc.observe_record(ts, &rec2, hash2, FlowMetrics::from_record(&rec2));

        let rows = acc.flush_closed_rows(180_000_000);
        assert_eq!(rows.len(), 2);
    }
}
