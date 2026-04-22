use super::store::{FacetStore, FacetStoreValueRef};
use crate::facet_catalog::{FacetFieldSpec, facet_field_spec, facet_field_spec_static};
use crate::flow::{FlowFields, FlowRecord};
use crate::presentation;
use crate::query::{payload_value, split_payload_bytes};
use rustc_hash::FxHashMap;
use std::collections::{BTreeMap, BTreeSet};
use std::mem::size_of;
use std::net::IpAddr;

const HASHMAP_ENTRY_OVERHEAD_BYTES: usize = size_of::<usize>() * 2;

pub(crate) trait FacetValueSink {
    fn insert_text_static(&mut self, field: &'static str, value: &str);
    fn insert_u8_static(&mut self, field: &'static str, value: u8);
    fn insert_u16_static(&mut self, field: &'static str, value: u16);
    fn insert_u32_static(&mut self, field: &'static str, value: u32);
    fn insert_u64_static(&mut self, field: &'static str, value: u64);
    fn insert_ip_static(&mut self, field: &'static str, value: Option<IpAddr>);
}

#[derive(Debug, Clone, Default, allocative::Allocative)]
pub(crate) struct FacetFileContribution {
    fields: FxHashMap<&'static str, FacetStore>,
}

impl FacetFileContribution {
    pub(super) fn field(&self, field: &str) -> Option<&FacetStore> {
        self.fields.get(field)
    }

    pub(super) fn iter(&self) -> impl Iterator<Item = (&'static str, &FacetStore)> + '_ {
        self.fields.iter().map(|(field, store)| (*field, store))
    }

    pub(super) fn from_scanned_values(values: BTreeMap<String, BTreeSet<String>>) -> Self {
        let mut contribution = Self::default();

        for (field, stored_values) in values {
            let Some(spec) = facet_field_spec(field.as_str()) else {
                continue;
            };

            let store = contribution
                .fields
                .entry(spec.name)
                .or_insert_with(|| FacetStore::new(spec.kind));
            for value in stored_values {
                let _ = store.insert_raw(&value);
            }
        }

        contribution
    }

    #[allow(dead_code)]
    pub(super) fn insert_raw(&mut self, field: &'static str, value: &str) {
        let Some(spec) = facet_field_spec(field) else {
            return;
        };
        self.insert_raw_spec(*spec, value);
    }

    fn store_for_spec(&mut self, spec: FacetFieldSpec) -> &mut FacetStore {
        self.fields
            .entry(spec.name)
            .or_insert_with(|| FacetStore::new(spec.kind))
    }

    #[allow(dead_code)]
    fn insert_raw_spec(&mut self, spec: FacetFieldSpec, value: &str) {
        let store = self.store_for_spec(spec);
        let _ = store.insert_raw(value);
    }

    pub(crate) fn insert_text_static(&mut self, field: &'static str, value: &str) {
        if value.is_empty() {
            return;
        }
        let Some(spec) = facet_field_spec_static(field) else {
            return;
        };
        let store = self.store_for_spec(spec);
        let _ = store.insert_text(value);
    }

    pub(crate) fn insert_u8_static(&mut self, field: &'static str, value: u8) {
        if value == 0 {
            return;
        }
        self.insert_u8_present_static(field, value);
    }

    pub(crate) fn insert_u8_present_static(&mut self, field: &'static str, value: u8) {
        let Some(spec) = facet_field_spec_static(field) else {
            return;
        };
        let store = self.store_for_spec(spec);
        let _ = store.insert_u8(value);
    }

    pub(crate) fn insert_u16_static(&mut self, field: &'static str, value: u16) {
        if value == 0 {
            return;
        }
        self.insert_u16_present_static(field, value);
    }

    pub(crate) fn insert_u16_present_static(&mut self, field: &'static str, value: u16) {
        let Some(spec) = facet_field_spec_static(field) else {
            return;
        };
        let store = self.store_for_spec(spec);
        let _ = store.insert_u16(value);
    }

    pub(crate) fn insert_u32_static(&mut self, field: &'static str, value: u32) {
        if value == 0 {
            return;
        }
        self.insert_u32_present_static(field, value);
    }

    pub(crate) fn insert_u32_present_static(&mut self, field: &'static str, value: u32) {
        let Some(spec) = facet_field_spec_static(field) else {
            return;
        };
        let store = self.store_for_spec(spec);
        let _ = store.insert_u32(value);
    }

    pub(crate) fn insert_u64_static(&mut self, field: &'static str, value: u64) {
        if value == 0 {
            return;
        }
        self.insert_u64_present_static(field, value);
    }

    pub(crate) fn insert_u64_present_static(&mut self, field: &'static str, value: u64) {
        let Some(spec) = facet_field_spec_static(field) else {
            return;
        };
        let store = self.store_for_spec(spec);
        let _ = store.insert_u64(value);
    }

    pub(crate) fn insert_ip_static(&mut self, field: &'static str, value: Option<IpAddr>) {
        let Some(value) = value else {
            return;
        };
        self.insert_ip_present_static(field, value);
    }

    pub(crate) fn insert_ip_present_static(&mut self, field: &'static str, value: IpAddr) {
        let Some(spec) = facet_field_spec_static(field) else {
            return;
        };
        let store = self.store_for_spec(spec);
        let _ = store.insert_ip_addr(value);
    }

    pub(super) fn insert_value_spec(
        &mut self,
        spec: FacetFieldSpec,
        value: FacetStoreValueRef<'_>,
    ) -> bool {
        let store = self.store_for_spec(spec);
        store.insert_value_ref(value)
    }

    pub(super) fn estimated_heap_bytes(&self) -> usize {
        self.fields.len()
            * (size_of::<&'static str>() + size_of::<FacetStore>() + HASHMAP_ENTRY_OVERHEAD_BYTES)
            + self
                .fields
                .values()
                .map(FacetStore::estimated_heap_bytes)
                .sum::<usize>()
    }

    #[cfg(test)]
    pub(crate) fn debug_string_map(&self) -> BTreeMap<&'static str, Vec<String>> {
        self.fields
            .iter()
            .map(|(field, store)| (*field, store.collect_strings(None)))
            .collect()
    }
}

impl FacetValueSink for FacetFileContribution {
    fn insert_text_static(&mut self, field: &'static str, value: &str) {
        FacetFileContribution::insert_text_static(self, field, value);
    }

    fn insert_u8_static(&mut self, field: &'static str, value: u8) {
        FacetFileContribution::insert_u8_static(self, field, value);
    }

    fn insert_u16_static(&mut self, field: &'static str, value: u16) {
        FacetFileContribution::insert_u16_static(self, field, value);
    }

    fn insert_u32_static(&mut self, field: &'static str, value: u32) {
        FacetFileContribution::insert_u32_static(self, field, value);
    }

    fn insert_u64_static(&mut self, field: &'static str, value: u64) {
        FacetFileContribution::insert_u64_static(self, field, value);
    }

    fn insert_ip_static(&mut self, field: &'static str, value: Option<IpAddr>) {
        FacetFileContribution::insert_ip_static(self, field, value);
    }
}

#[allow(dead_code)]
pub(crate) fn facet_contribution_from_flow_fields(fields: &FlowFields) -> FacetFileContribution {
    let mut contribution = FacetFileContribution::default();
    let protocol = fields.get("PROTOCOL").map(String::as_str);
    let icmpv4_type = fields.get("ICMPV4_TYPE").map(String::as_str);
    let icmpv4_code = fields.get("ICMPV4_CODE").map(String::as_str);
    let icmpv6_type = fields.get("ICMPV6_TYPE").map(String::as_str);
    let icmpv6_code = fields.get("ICMPV6_CODE").map(String::as_str);

    for (field, value) in fields {
        if !value.is_empty() {
            if let Some(spec) = facet_field_spec(field) {
                contribution.insert_raw_spec(*spec, value);
            }
        }
    }

    if let Some(value) =
        presentation::icmp_virtual_value("ICMPV4", protocol, icmpv4_type, icmpv4_code)
    {
        contribution.insert_raw("ICMPV4", &value);
    }
    if let Some(value) =
        presentation::icmp_virtual_value("ICMPV6", protocol, icmpv6_type, icmpv6_code)
    {
        contribution.insert_raw("ICMPV6", &value);
    }

    contribution
}

#[allow(dead_code)]
pub(crate) fn facet_contribution_from_record(record: &FlowRecord) -> FacetFileContribution {
    let mut contribution = FacetFileContribution::default();
    append_record_facet_values(&mut contribution, record);
    contribution
}

pub(crate) fn append_record_facet_values(sink: &mut impl FacetValueSink, record: &FlowRecord) {
    append_record_core_fields(sink, record);
    append_record_network_fields(sink, record);
    append_record_interface_fields(sink, record);
    append_record_transport_fields(sink, record);
    append_record_header_fields(sink, record);
    append_record_virtual_icmp_fields(sink, record);
}

#[allow(dead_code)]
pub(crate) fn facet_contribution_from_encoded_fields<'a, I>(fields: I) -> FacetFileContribution
where
    I: IntoIterator<Item = &'a [u8]>,
{
    let mut contribution = FacetFileContribution::default();
    let mut protocol = None;
    let mut icmpv4_type = None;
    let mut icmpv4_code = None;
    let mut icmpv6_type = None;
    let mut icmpv6_code = None;

    for payload in fields {
        let Some((key_bytes, value_bytes)) = split_payload_bytes(payload) else {
            continue;
        };
        let Ok(field) = std::str::from_utf8(key_bytes) else {
            continue;
        };
        let value = payload_value(value_bytes);
        if value.is_empty() {
            continue;
        }

        match field {
            "PROTOCOL" => protocol = Some(value.into_owned()),
            "ICMPV4_TYPE" => icmpv4_type = Some(value.into_owned()),
            "ICMPV4_CODE" => icmpv4_code = Some(value.into_owned()),
            "ICMPV6_TYPE" => icmpv6_type = Some(value.into_owned()),
            "ICMPV6_CODE" => icmpv6_code = Some(value.into_owned()),
            _ => {
                if let Some(spec) = facet_field_spec(field) {
                    contribution.insert_raw_spec(*spec, value.as_ref());
                }
            }
        }
    }

    if let Some(value) = presentation::icmp_virtual_value(
        "ICMPV4",
        protocol.as_deref(),
        icmpv4_type.as_deref(),
        icmpv4_code.as_deref(),
    ) {
        contribution.insert_raw("ICMPV4", &value);
    }
    if let Some(value) = presentation::icmp_virtual_value(
        "ICMPV6",
        protocol.as_deref(),
        icmpv6_type.as_deref(),
        icmpv6_code.as_deref(),
    ) {
        contribution.insert_raw("ICMPV6", &value);
    }

    contribution
}

fn append_record_core_fields(sink: &mut impl FacetValueSink, record: &FlowRecord) {
    sink.insert_text_static("FLOW_VERSION", record.flow_version);
    sink.insert_ip_static("EXPORTER_IP", record.exporter_ip);
    sink.insert_u16_static("EXPORTER_PORT", record.exporter_port);
    sink.insert_text_static("EXPORTER_NAME", &record.exporter_name);
    sink.insert_text_static("EXPORTER_GROUP", &record.exporter_group);
    sink.insert_text_static("EXPORTER_ROLE", &record.exporter_role);
    sink.insert_text_static("EXPORTER_SITE", &record.exporter_site);
    sink.insert_text_static("EXPORTER_REGION", &record.exporter_region);
    sink.insert_text_static("EXPORTER_TENANT", &record.exporter_tenant);
    if record.has_etype() {
        sink.insert_u16_static("ETYPE", record.etype);
    }
    if record.has_forwarding_status() {
        sink.insert_u8_static("FORWARDING_STATUS", record.forwarding_status);
    }
    if record.has_direction() {
        sink.insert_text_static("DIRECTION", record.direction.as_str());
    }
}

fn append_record_network_fields(sink: &mut impl FacetValueSink, record: &FlowRecord) {
    sink.insert_ip_static("SRC_ADDR", record.src_addr);
    sink.insert_ip_static("DST_ADDR", record.dst_addr);
    if let Some(prefix) = format_prefix(record.src_prefix, record.src_mask) {
        sink.insert_text_static("SRC_PREFIX", &prefix);
    }
    if let Some(prefix) = format_prefix(record.dst_prefix, record.dst_mask) {
        sink.insert_text_static("DST_PREFIX", &prefix);
    }
    sink.insert_u8_static("SRC_MASK", record.src_mask);
    sink.insert_u8_static("DST_MASK", record.dst_mask);
    sink.insert_u32_static("SRC_AS", record.src_as);
    sink.insert_u32_static("DST_AS", record.dst_as);
    sink.insert_text_static("SRC_AS_NAME", &record.src_as_name);
    sink.insert_text_static("DST_AS_NAME", &record.dst_as_name);
    sink.insert_text_static("SRC_NET_NAME", &record.src_net_name);
    sink.insert_text_static("DST_NET_NAME", &record.dst_net_name);
    sink.insert_text_static("SRC_NET_ROLE", &record.src_net_role);
    sink.insert_text_static("DST_NET_ROLE", &record.dst_net_role);
    sink.insert_text_static("SRC_NET_SITE", &record.src_net_site);
    sink.insert_text_static("DST_NET_SITE", &record.dst_net_site);
    sink.insert_text_static("SRC_NET_REGION", &record.src_net_region);
    sink.insert_text_static("DST_NET_REGION", &record.dst_net_region);
    sink.insert_text_static("SRC_NET_TENANT", &record.src_net_tenant);
    sink.insert_text_static("DST_NET_TENANT", &record.dst_net_tenant);
    sink.insert_text_static("SRC_COUNTRY", &record.src_country);
    sink.insert_text_static("DST_COUNTRY", &record.dst_country);
    sink.insert_text_static("SRC_GEO_CITY", &record.src_geo_city);
    sink.insert_text_static("DST_GEO_CITY", &record.dst_geo_city);
    sink.insert_text_static("SRC_GEO_STATE", &record.src_geo_state);
    sink.insert_text_static("DST_GEO_STATE", &record.dst_geo_state);
    sink.insert_text_static("DST_AS_PATH", &record.dst_as_path);
    sink.insert_text_static("DST_COMMUNITIES", &record.dst_communities);
    sink.insert_text_static("DST_LARGE_COMMUNITIES", &record.dst_large_communities);
}

fn append_record_interface_fields(sink: &mut impl FacetValueSink, record: &FlowRecord) {
    sink.insert_u32_static("IN_IF", record.in_if);
    sink.insert_u32_static("OUT_IF", record.out_if);
    sink.insert_text_static("IN_IF_NAME", &record.in_if_name);
    sink.insert_text_static("OUT_IF_NAME", &record.out_if_name);
    sink.insert_text_static("IN_IF_DESCRIPTION", &record.in_if_description);
    sink.insert_text_static("OUT_IF_DESCRIPTION", &record.out_if_description);
    if record.has_in_if_speed() {
        sink.insert_u64_static("IN_IF_SPEED", record.in_if_speed);
    }
    if record.has_out_if_speed() {
        sink.insert_u64_static("OUT_IF_SPEED", record.out_if_speed);
    }
    sink.insert_text_static("IN_IF_PROVIDER", &record.in_if_provider);
    sink.insert_text_static("OUT_IF_PROVIDER", &record.out_if_provider);
    sink.insert_text_static("IN_IF_CONNECTIVITY", &record.in_if_connectivity);
    sink.insert_text_static("OUT_IF_CONNECTIVITY", &record.out_if_connectivity);
    if record.has_in_if_boundary() {
        sink.insert_u8_static("IN_IF_BOUNDARY", record.in_if_boundary);
    }
    if record.has_out_if_boundary() {
        sink.insert_u8_static("OUT_IF_BOUNDARY", record.out_if_boundary);
    }
}

fn append_record_transport_fields(sink: &mut impl FacetValueSink, record: &FlowRecord) {
    sink.insert_ip_static("NEXT_HOP", record.next_hop);
    sink.insert_u16_static("SRC_PORT", record.src_port);
    sink.insert_u16_static("DST_PORT", record.dst_port);
    sink.insert_ip_static("SRC_ADDR_NAT", record.src_addr_nat);
    sink.insert_ip_static("DST_ADDR_NAT", record.dst_addr_nat);
    sink.insert_u16_static("SRC_PORT_NAT", record.src_port_nat);
    sink.insert_u16_static("DST_PORT_NAT", record.dst_port_nat);
    if record.has_src_vlan() {
        sink.insert_u16_static("SRC_VLAN", record.src_vlan);
    }
    if record.has_dst_vlan() {
        sink.insert_u16_static("DST_VLAN", record.dst_vlan);
    }
    if let Some(mac) = format_mac(record.src_mac) {
        sink.insert_text_static("SRC_MAC", &mac);
    }
    if let Some(mac) = format_mac(record.dst_mac) {
        sink.insert_text_static("DST_MAC", &mac);
    }
}

fn append_record_header_fields(sink: &mut impl FacetValueSink, record: &FlowRecord) {
    sink.insert_u8_static("IPTTL", record.ipttl);
    if record.has_iptos() {
        sink.insert_u8_static("IPTOS", record.iptos);
    }
    sink.insert_u32_static("IPV6_FLOW_LABEL", record.ipv6_flow_label);
    if record.has_tcp_flags() {
        sink.insert_u8_static("TCP_FLAGS", record.tcp_flags);
    }
    sink.insert_u32_static("IP_FRAGMENT_ID", record.ip_fragment_id);
    sink.insert_u16_static("IP_FRAGMENT_OFFSET", record.ip_fragment_offset);
    sink.insert_text_static("MPLS_LABELS", &record.mpls_labels);
}

fn append_record_virtual_icmp_fields(sink: &mut impl FacetValueSink, record: &FlowRecord) {
    let protocol = (record.protocol != 0).then_some(record.protocol.to_string());
    let icmpv4_type = record
        .has_icmpv4_type()
        .then_some(record.icmpv4_type.to_string());
    let icmpv4_code = record
        .has_icmpv4_code()
        .then_some(record.icmpv4_code.to_string());
    let icmpv6_type = record
        .has_icmpv6_type()
        .then_some(record.icmpv6_type.to_string());
    let icmpv6_code = record
        .has_icmpv6_code()
        .then_some(record.icmpv6_code.to_string());

    if let Some(value) = presentation::icmp_virtual_value(
        "ICMPV4",
        protocol.as_deref(),
        icmpv4_type.as_deref(),
        icmpv4_code.as_deref(),
    ) {
        sink.insert_text_static("ICMPV4", &value);
    }
    if let Some(value) = presentation::icmp_virtual_value(
        "ICMPV6",
        protocol.as_deref(),
        icmpv6_type.as_deref(),
        icmpv6_code.as_deref(),
    ) {
        sink.insert_text_static("ICMPV6", &value);
    }
}

fn format_prefix(ip: Option<IpAddr>, mask: u8) -> Option<String> {
    let ip = ip?;
    if mask > 0 {
        return Some(format!("{ip}/{mask}"));
    }
    Some(ip.to_string())
}

fn format_mac(mac: [u8; 6]) -> Option<String> {
    if mac == [0u8; 6] {
        return None;
    }
    Some(format!(
        "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
        mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::BTreeMap;

    fn contribution_strings(
        contribution: &FacetFileContribution,
    ) -> BTreeMap<&'static str, Vec<String>> {
        contribution
            .iter()
            .map(|(field, store)| (field, store.collect_strings(None)))
            .collect()
    }

    #[test]
    fn record_and_encoded_contributions_match() {
        let mut record = FlowRecord {
            flow_version: "v9",
            protocol: 6,
            exporter_ip: Some("192.0.2.10".parse().unwrap()),
            exporter_port: 2055,
            exporter_name: "edge-a".to_string(),
            exporter_site: "ath".to_string(),
            src_addr: Some("10.0.0.1".parse().unwrap()),
            dst_addr: Some("203.0.113.8".parse().unwrap()),
            src_prefix: Some("10.0.0.0".parse().unwrap()),
            dst_prefix: Some("203.0.113.0".parse().unwrap()),
            src_mask: 24,
            dst_mask: 24,
            src_as: 64512,
            dst_as: 15169,
            src_as_name: "AS64512 EXAMPLE".to_string(),
            dst_as_name: "AS15169 GOOGLE".to_string(),
            src_country: "US".to_string(),
            dst_country: "DE".to_string(),
            in_if: 10,
            out_if: 20,
            in_if_name: "xe-0/0/0".to_string(),
            out_if_name: "xe-0/0/1".to_string(),
            next_hop: Some("198.51.100.1".parse().unwrap()),
            src_port: 12345,
            dst_port: 443,
            ipttl: 64,
            ipv6_flow_label: 1234,
            mpls_labels: "100-200".to_string(),
            ..FlowRecord::default()
        };
        record.set_direction(crate::flow::FlowDirection::Ingress);
        record.set_etype(2048);
        record.set_forwarding_status(64);
        record.set_in_if_speed(1_000_000_000);
        record.set_out_if_speed(1_000_000_000);
        record.set_in_if_boundary(1);
        record.set_out_if_boundary(1);
        record.set_src_vlan(100);
        record.set_dst_vlan(200);
        record.set_iptos(16);
        record.set_tcp_flags(0x12);
        record.set_icmpv4_type(8);
        record.set_icmpv4_code(0);

        let mut data = Vec::new();
        let mut refs = Vec::new();
        record.encode_to_journal_buf(&mut data, &mut refs);

        let encoded = facet_contribution_from_encoded_fields(refs.iter().map(|r| &data[r.clone()]));
        let direct = facet_contribution_from_record(&record);

        assert_eq!(
            contribution_strings(&direct),
            contribution_strings(&encoded)
        );
    }

}
