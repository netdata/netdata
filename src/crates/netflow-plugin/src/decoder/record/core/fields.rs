use crate::decoder::{
    CANONICAL_FLOW_DEFAULTS, FlowFields, apply_icmp_port_fallback, canonicalize_ip_addr,
    default_exporter_name, field_tracks_presence, infer_etype_from_endpoints,
    normalize_direction_value,
};
use std::collections::BTreeMap;
use std::net::SocketAddr;

pub(crate) fn base_fields(version: &str, source: SocketAddr) -> FlowFields {
    let mut fields = BTreeMap::new();
    fields.insert("FLOW_VERSION", version.to_string());
    fields.insert("EXPORTER_IP", canonicalize_ip_addr(source.ip()).to_string());
    fields.insert("EXPORTER_PORT", source.port().to_string());
    fields
}

pub(crate) fn finalize_canonical_flow_fields(fields: &mut FlowFields) {
    // Akvorado-style contract: protocol-specific fields are not part of the canonical record.
    fields.retain(|name, _| !name.starts_with("V9_") && !name.starts_with("IPFIX_"));

    if !fields.contains_key("RAW_BYTES") {
        let bytes = fields
            .get("BYTES")
            .cloned()
            .unwrap_or_else(|| "0".to_string());
        fields.insert("RAW_BYTES", bytes);
    }
    if !fields.contains_key("RAW_PACKETS") {
        let packets = fields
            .get("PACKETS")
            .cloned()
            .unwrap_or_else(|| "0".to_string());
        fields.insert("RAW_PACKETS", packets);
    }

    for &(name, default_value) in CANONICAL_FLOW_DEFAULTS {
        if field_tracks_presence(name) {
            continue;
        }
        fields
            .entry(name)
            .or_insert_with(|| default_value.to_string());
    }

    let sampling_rate = fields
        .get("SAMPLING_RATE")
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
        .max(1);
    if let Some(bytes) = fields.get_mut("BYTES") {
        let scaled = bytes
            .parse::<u64>()
            .unwrap_or(0)
            .saturating_mul(sampling_rate);
        *bytes = scaled.to_string();
    }
    if let Some(packets) = fields.get_mut("PACKETS") {
        let scaled = packets
            .parse::<u64>()
            .unwrap_or(0)
            .saturating_mul(sampling_rate);
        *packets = scaled.to_string();
    }

    let exporter_name_missing = fields
        .get("EXPORTER_NAME")
        .map(String::is_empty)
        .unwrap_or(true);
    if exporter_name_missing && let Some(exporter_ip) = fields.get("EXPORTER_IP") {
        fields.insert("EXPORTER_NAME", default_exporter_name(exporter_ip));
    }

    apply_icmp_port_fallback(fields);

    if let Some(direction) = fields.get_mut("DIRECTION") {
        *direction = normalize_direction_value(direction).to_string();
    }

    let etype_missing = fields
        .get("ETYPE")
        .map(|v| v.is_empty() || v == "0")
        .unwrap_or(true);
    if etype_missing && let Some(inferred) = infer_etype_from_endpoints(fields) {
        fields.insert("ETYPE", inferred.to_string());
    }
}
