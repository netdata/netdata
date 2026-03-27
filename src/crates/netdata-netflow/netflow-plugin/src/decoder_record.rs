fn base_fields(version: &str, source: SocketAddr) -> FlowFields {
    let mut fields = BTreeMap::new();
    fields.insert("FLOW_VERSION", version.to_string());
    fields.insert("EXPORTER_IP", source.ip().to_string());
    fields.insert("EXPORTER_PORT", source.port().to_string());
    fields
}

fn finalize_canonical_flow_fields(fields: &mut FlowFields) {
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
    if etype_missing {
        if let Some(inferred) = infer_etype_from_endpoints(fields) {
            fields.insert("ETYPE", inferred.to_string());
        }
    }
}

fn default_exporter_name(exporter_ip: &str) -> String {
    exporter_ip
        .chars()
        .map(|ch| if ch.is_ascii_alphanumeric() { ch } else { '_' })
        .collect()
}

fn canonical_value<'a>(canonical: &'a str, raw_value: &'a str) -> &'a str {
    if canonical == "DIRECTION" {
        normalize_direction_value(raw_value)
    } else {
        raw_value
    }
}

const USEC_PER_SECOND: u64 = 1_000_000;
const USEC_PER_MILLISECOND: u64 = 1_000;

fn unix_seconds_to_usec(seconds: u64) -> u64 {
    seconds.saturating_mul(USEC_PER_SECOND)
}

fn netflow_v9_system_init_usec(export_time_seconds: u64, sys_uptime_millis: u64) -> u64 {
    unix_seconds_to_usec(export_time_seconds)
        .saturating_sub(sys_uptime_millis.saturating_mul(USEC_PER_MILLISECOND))
}

fn netflow_v9_uptime_millis_to_absolute_usec(system_init_usec: u64, switched_millis: u64) -> u64 {
    system_init_usec.saturating_add(switched_millis.saturating_mul(USEC_PER_MILLISECOND))
}

// ---------------------------------------------------------------------------
// FlowRecord-native helpers (Phase 2: V5/V7 decode without BTreeMap)
// ---------------------------------------------------------------------------

/// Create a base FlowRecord with exporter identity populated.
fn base_record(version: &'static str, source: SocketAddr) -> FlowRecord {
    FlowRecord {
        flow_version: version,
        exporter_ip: Some(source.ip()),
        exporter_port: source.port(),
        ..Default::default()
    }
}

/// Finalize a FlowRecord: apply defaults, normalize values.
/// Equivalent of `finalize_canonical_flow_fields` for FlowRecord.
fn finalize_record(rec: &mut FlowRecord) {
    // Copy RAW_* from counters if not yet set
    if rec.raw_bytes == 0 {
        rec.raw_bytes = rec.bytes;
    }
    if rec.raw_packets == 0 {
        rec.raw_packets = rec.packets;
    }

    let sampling_rate = rec.sampling_rate.max(1);
    rec.bytes = rec.bytes.saturating_mul(sampling_rate);
    rec.packets = rec.packets.saturating_mul(sampling_rate);

    // Default flows to 1
    if rec.flows == 0 {
        rec.flows = 1;
    }

    // Default exporter name from IP if empty
    if rec.exporter_name.is_empty() {
        if let Some(ip) = rec.exporter_ip {
            rec.exporter_name = default_exporter_name(&ip.to_string());
        }
    }

    // Apply ICMP port fallback
    apply_icmp_port_fallback_record(rec);

    // Infer ETYPE from endpoints if not set
    if !rec.has_etype() {
        if let Some(src) = rec.src_addr {
            rec.set_etype(if src.is_ipv4() { 2048 } else { 34525 });
        } else if let Some(dst) = rec.dst_addr {
            rec.set_etype(if dst.is_ipv4() { 2048 } else { 34525 });
        }
    }
}

/// ICMP port fallback: when src_port is 0 and dst_port contains a combined
/// ICMP type+code value, extract the individual type/code fields from it.
/// Mirrors the original apply_icmp_port_fallback for FlowFields.
fn apply_icmp_port_fallback_record(rec: &mut FlowRecord) {
    if rec.src_port != 0 || rec.dst_port == 0 {
        return;
    }

    let icmp_type = ((rec.dst_port >> 8) & 0xff) as u8;
    let icmp_code = (rec.dst_port & 0xff) as u8;

    match rec.protocol {
        1 => {
            if !rec.has_icmpv4_type() {
                rec.set_icmpv4_type(icmp_type);
            }
            if !rec.has_icmpv4_code() {
                rec.set_icmpv4_code(icmp_code);
            }
        }
        58 => {
            if !rec.has_icmpv6_type() {
                rec.set_icmpv6_type(icmp_type);
            }
            if !rec.has_icmpv6_code() {
                rec.set_icmpv6_code(icmp_code);
            }
        }
        _ => {}
    }
}

/// Swap src/dst fields in a FlowRecord for biflow reverse direction.
fn swap_directional_record_fields(rec: &mut FlowRecord) {
    std::mem::swap(&mut rec.src_addr, &mut rec.dst_addr);
    std::mem::swap(&mut rec.src_prefix, &mut rec.dst_prefix);
    std::mem::swap(&mut rec.src_mask, &mut rec.dst_mask);
    std::mem::swap(&mut rec.src_port, &mut rec.dst_port);
    std::mem::swap(&mut rec.src_as, &mut rec.dst_as);
    std::mem::swap(&mut rec.src_net_name, &mut rec.dst_net_name);
    std::mem::swap(&mut rec.src_net_role, &mut rec.dst_net_role);
    std::mem::swap(&mut rec.src_net_site, &mut rec.dst_net_site);
    std::mem::swap(&mut rec.src_net_region, &mut rec.dst_net_region);
    std::mem::swap(&mut rec.src_net_tenant, &mut rec.dst_net_tenant);
    std::mem::swap(&mut rec.src_country, &mut rec.dst_country);
    std::mem::swap(&mut rec.src_geo_city, &mut rec.dst_geo_city);
    std::mem::swap(&mut rec.src_geo_state, &mut rec.dst_geo_state);
    std::mem::swap(&mut rec.src_addr_nat, &mut rec.dst_addr_nat);
    std::mem::swap(&mut rec.src_port_nat, &mut rec.dst_port_nat);
    std::mem::swap(&mut rec.src_vlan, &mut rec.dst_vlan);
    std::mem::swap(&mut rec.src_mac, &mut rec.dst_mac);
    std::mem::swap(&mut rec.in_if, &mut rec.out_if);
    std::mem::swap(&mut rec.in_if_name, &mut rec.out_if_name);
    std::mem::swap(&mut rec.in_if_description, &mut rec.out_if_description);
    std::mem::swap(&mut rec.in_if_speed, &mut rec.out_if_speed);
    std::mem::swap(&mut rec.in_if_provider, &mut rec.out_if_provider);
    std::mem::swap(&mut rec.in_if_connectivity, &mut rec.out_if_connectivity);
    std::mem::swap(&mut rec.in_if_boundary, &mut rec.out_if_boundary);
    rec.swap_presence_flags(FlowPresence::SRC_VLAN, FlowPresence::DST_VLAN);
    rec.swap_presence_flags(FlowPresence::IN_IF_SPEED, FlowPresence::OUT_IF_SPEED);
    rec.swap_presence_flags(FlowPresence::IN_IF_BOUNDARY, FlowPresence::OUT_IF_BOUNDARY);
}

/// Set a field on FlowRecord by canonical name (string dispatch).
/// Used by V9/IPFIX template-driven decode where field names come from template mapping.
fn set_record_field(rec: &mut FlowRecord, key: &str, value: &str) {
    if set_record_exporter_field(rec, key, value)
        || set_record_counter_field(rec, key, value)
        || set_record_network_field(rec, key, value)
        || set_record_interface_field(rec, key, value)
        || set_record_transport_field(rec, key, value)
    {
        return;
    }
}

fn set_record_exporter_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "FLOW_VERSION" => true,
        "EXPORTER_IP" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.exporter_ip = Some(ip);
            }
            true
        }
        "EXPORTER_PORT" => {
            rec.exporter_port = value.parse().unwrap_or(rec.exporter_port);
            true
        }
        "EXPORTER_NAME" => {
            rec.exporter_name = value.to_string();
            true
        }
        "EXPORTER_GROUP" => {
            rec.exporter_group = value.to_string();
            true
        }
        "EXPORTER_ROLE" => {
            rec.exporter_role = value.to_string();
            true
        }
        "EXPORTER_SITE" => {
            rec.exporter_site = value.to_string();
            true
        }
        "EXPORTER_REGION" => {
            rec.exporter_region = value.to_string();
            true
        }
        "EXPORTER_TENANT" => {
            rec.exporter_tenant = value.to_string();
            true
        }
        "SAMPLING_RATE" => {
            rec.set_sampling_rate(value.parse().unwrap_or(0));
            true
        }
        "ETYPE" => {
            rec.set_etype(value.parse().unwrap_or(0));
            true
        }
        "FORWARDING_STATUS" => {
            rec.set_forwarding_status(value.parse().unwrap_or(0));
            true
        }
        "DIRECTION" => {
            let normalized = FlowDirection::from_str_value(value);
            if value == DIRECTION_UNDEFINED {
                rec.clear_direction();
            } else {
                rec.set_direction(normalized);
            }
            true
        }
        _ => false,
    }
}

fn set_record_counter_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "PROTOCOL" => {
            rec.protocol = value.parse().unwrap_or(0);
            true
        }
        "BYTES" => {
            rec.bytes = value.parse().unwrap_or(0);
            true
        }
        "PACKETS" => {
            rec.packets = value.parse().unwrap_or(0);
            true
        }
        "FLOWS" => {
            rec.flows = value.parse().unwrap_or(0);
            true
        }
        "RAW_BYTES" => {
            rec.raw_bytes = value.parse().unwrap_or(0);
            true
        }
        "RAW_PACKETS" => {
            rec.raw_packets = value.parse().unwrap_or(0);
            true
        }
        _ => false,
    }
}

fn set_record_network_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "SRC_ADDR" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.src_addr = Some(ip);
            }
            true
        }
        "DST_ADDR" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.dst_addr = Some(ip);
            }
            true
        }
        "SRC_PREFIX" => {
            rec.src_prefix = parse_prefix_ip(value);
            true
        }
        "DST_PREFIX" => {
            rec.dst_prefix = parse_prefix_ip(value);
            true
        }
        "SRC_MASK" => {
            rec.src_mask = value.parse().unwrap_or(0);
            true
        }
        "DST_MASK" => {
            rec.dst_mask = value.parse().unwrap_or(0);
            true
        }
        "SRC_AS" => {
            rec.src_as = value.parse().unwrap_or(0);
            true
        }
        "DST_AS" => {
            rec.dst_as = value.parse().unwrap_or(0);
            true
        }
        "SRC_AS_NAME" => {
            rec.src_as_name = value.to_string();
            true
        }
        "DST_AS_NAME" => {
            rec.dst_as_name = value.to_string();
            true
        }
        "SRC_NET_NAME" => {
            rec.src_net_name = value.to_string();
            true
        }
        "DST_NET_NAME" => {
            rec.dst_net_name = value.to_string();
            true
        }
        "SRC_NET_ROLE" => {
            rec.src_net_role = value.to_string();
            true
        }
        "DST_NET_ROLE" => {
            rec.dst_net_role = value.to_string();
            true
        }
        "SRC_NET_SITE" => {
            rec.src_net_site = value.to_string();
            true
        }
        "DST_NET_SITE" => {
            rec.dst_net_site = value.to_string();
            true
        }
        "SRC_NET_REGION" => {
            rec.src_net_region = value.to_string();
            true
        }
        "DST_NET_REGION" => {
            rec.dst_net_region = value.to_string();
            true
        }
        "SRC_NET_TENANT" => {
            rec.src_net_tenant = value.to_string();
            true
        }
        "DST_NET_TENANT" => {
            rec.dst_net_tenant = value.to_string();
            true
        }
        "SRC_COUNTRY" => {
            rec.src_country = value.to_string();
            true
        }
        "DST_COUNTRY" => {
            rec.dst_country = value.to_string();
            true
        }
        "SRC_GEO_CITY" => {
            rec.src_geo_city = value.to_string();
            true
        }
        "DST_GEO_CITY" => {
            rec.dst_geo_city = value.to_string();
            true
        }
        "SRC_GEO_STATE" => {
            rec.src_geo_state = value.to_string();
            true
        }
        "DST_GEO_STATE" => {
            rec.dst_geo_state = value.to_string();
            true
        }
        "DST_AS_PATH" => {
            rec.dst_as_path = value.to_string();
            true
        }
        "DST_COMMUNITIES" => {
            rec.dst_communities = value.to_string();
            true
        }
        "DST_LARGE_COMMUNITIES" => {
            rec.dst_large_communities = value.to_string();
            true
        }
        "NEXT_HOP" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.next_hop = Some(ip);
            }
            true
        }
        _ => false,
    }
}

fn set_record_interface_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "IN_IF" => {
            let v: u32 = value.parse().unwrap_or(0);
            if rec.in_if == 0 {
                rec.in_if = v;
            }
            true
        }
        "OUT_IF" => {
            let v: u32 = value.parse().unwrap_or(0);
            if rec.out_if == 0 {
                rec.out_if = v;
            }
            true
        }
        "IN_IF_NAME" => {
            rec.in_if_name = value.to_string();
            true
        }
        "OUT_IF_NAME" => {
            rec.out_if_name = value.to_string();
            true
        }
        "IN_IF_DESCRIPTION" => {
            rec.in_if_description = value.to_string();
            true
        }
        "OUT_IF_DESCRIPTION" => {
            rec.out_if_description = value.to_string();
            true
        }
        "IN_IF_SPEED" => {
            rec.set_in_if_speed(value.parse().unwrap_or(0));
            true
        }
        "OUT_IF_SPEED" => {
            rec.set_out_if_speed(value.parse().unwrap_or(0));
            true
        }
        "IN_IF_PROVIDER" => {
            rec.in_if_provider = value.to_string();
            true
        }
        "OUT_IF_PROVIDER" => {
            rec.out_if_provider = value.to_string();
            true
        }
        "IN_IF_CONNECTIVITY" => {
            rec.in_if_connectivity = value.to_string();
            true
        }
        "OUT_IF_CONNECTIVITY" => {
            rec.out_if_connectivity = value.to_string();
            true
        }
        "IN_IF_BOUNDARY" => {
            rec.set_in_if_boundary(value.parse().unwrap_or(0));
            true
        }
        "OUT_IF_BOUNDARY" => {
            rec.set_out_if_boundary(value.parse().unwrap_or(0));
            true
        }
        _ => false,
    }
}

fn set_record_transport_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "SRC_PORT" => {
            rec.src_port = value.parse().unwrap_or(0);
            true
        }
        "DST_PORT" => {
            rec.dst_port = value.parse().unwrap_or(0);
            true
        }
        "FLOW_START_USEC" => {
            rec.flow_start_usec = value.parse().unwrap_or(0);
            true
        }
        "FLOW_END_USEC" => {
            rec.flow_end_usec = value.parse().unwrap_or(0);
            true
        }
        "OBSERVATION_TIME_MILLIS" => {
            rec.observation_time_millis = value.parse().unwrap_or(0);
            true
        }
        "SRC_ADDR_NAT" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.src_addr_nat = Some(ip);
            }
            true
        }
        "DST_ADDR_NAT" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.dst_addr_nat = Some(ip);
            }
            true
        }
        "SRC_PORT_NAT" => {
            rec.src_port_nat = value.parse().unwrap_or(0);
            true
        }
        "DST_PORT_NAT" => {
            rec.dst_port_nat = value.parse().unwrap_or(0);
            true
        }
        "SRC_VLAN" => {
            rec.set_src_vlan(value.parse().unwrap_or(0));
            true
        }
        "DST_VLAN" => {
            rec.set_dst_vlan(value.parse().unwrap_or(0));
            true
        }
        "SRC_MAC" => {
            rec.src_mac = parse_mac(&value.to_ascii_lowercase());
            true
        }
        "DST_MAC" => {
            rec.dst_mac = parse_mac(&value.to_ascii_lowercase());
            true
        }
        "IPTTL" => {
            rec.ipttl = value.parse().unwrap_or(0);
            true
        }
        "IPTOS" => {
            rec.set_iptos(value.parse().unwrap_or(0));
            true
        }
        "IPV6_FLOW_LABEL" => {
            rec.ipv6_flow_label = value.parse().unwrap_or(0);
            true
        }
        "TCP_FLAGS" => {
            rec.set_tcp_flags(value.parse().unwrap_or(0));
            true
        }
        "IP_FRAGMENT_ID" => {
            rec.ip_fragment_id = value.parse().unwrap_or(0);
            true
        }
        "IP_FRAGMENT_OFFSET" => {
            rec.ip_fragment_offset = value.parse().unwrap_or(0);
            true
        }
        "ICMPV4_TYPE" => {
            rec.set_icmpv4_type(value.parse().unwrap_or(0));
            true
        }
        "ICMPV4_CODE" => {
            rec.set_icmpv4_code(value.parse().unwrap_or(0));
            true
        }
        "ICMPV6_TYPE" => {
            rec.set_icmpv6_type(value.parse().unwrap_or(0));
            true
        }
        "ICMPV6_CODE" => {
            rec.set_icmpv6_code(value.parse().unwrap_or(0));
            true
        }
        "MPLS_LABELS" => {
            rec.mpls_labels = value.to_string();
            true
        }
        _ => false,
    }
}

/// Like set_record_field but always overwrites (for override_canonical_field equivalent).
fn override_record_field(rec: &mut FlowRecord, key: &str, value: &str) {
    // IN_IF/OUT_IF always overwrite in the override path (unlike set_record_field)
    match key {
        "IN_IF" => {
            rec.in_if = value.parse().unwrap_or(0);
        }
        "OUT_IF" => {
            rec.out_if = value.parse().unwrap_or(0);
        }
        _ => set_record_field(rec, key, value),
    }
}

fn sync_raw_metrics_record(rec: &mut FlowRecord) {
    rec.raw_bytes = rec.bytes;
    rec.raw_packets = rec.packets;
}

// ---------------------------------------------------------------------------
// FlowRecord-native packet parsing (mirrors FlowFields-based versions above)
// ---------------------------------------------------------------------------

fn etype_u16_from_ip_version(value: &str) -> Option<u16> {
    match value.parse::<u64>().ok() {
        Some(4) => Some(2048),
        Some(6) => Some(34525),
        _ => None,
    }
}

fn decode_type_code_raw(value: &str) -> Option<(u8, u8)> {
    let tc = value.parse::<u64>().ok()?;
    Some((((tc >> 8) & 0xff) as u8, (tc & 0xff) as u8))
}

fn append_mpls_label_record(rec: &mut FlowRecord, value: &str) {
    let raw = if let Ok(v) = value.parse::<u64>() {
        v
    } else if let Some(hex) = value
        .strip_prefix("0x")
        .or_else(|| value.strip_prefix("0X"))
    {
        u64::from_str_radix(hex, 16).ok().unwrap_or(0)
    } else if value.chars().all(|c| c.is_ascii_hexdigit()) {
        u64::from_str_radix(value, 16).ok().unwrap_or(0)
    } else {
        0
    };
    if raw == 0 {
        return;
    }
    let label = raw >> 4;
    if label == 0 {
        return;
    }
    if rec.mpls_labels.is_empty() {
        rec.mpls_labels = label.to_string();
    } else {
        rec.mpls_labels.push(',');
        rec.mpls_labels.push_str(&label.to_string());
    }
}

fn parse_datalink_frame_section_record(
    data: &[u8],
    rec: &mut FlowRecord,
    decapsulation_mode: DecapsulationMode,
) -> Option<u64> {
    if data.len() < 14 {
        return None;
    }

    rec.dst_mac.copy_from_slice(&data[0..6]);
    rec.src_mac.copy_from_slice(&data[6..12]);

    let mut etype = u16::from_be_bytes([data[12], data[13]]);
    let mut cursor = &data[14..];

    while etype == ETYPE_VLAN {
        if cursor.len() < 4 {
            return None;
        }
        // VLAN extraction from 802.1Q tags is intentionally skipped for FlowRecord.
        // The FlowFields version only extracts when SRC_VLAN was explicitly pre-set
        // to "0" (V9 special decode), which uses the FlowFields-based parse function.
        // For FlowRecord callers, VLANs come from other sources (ExtendedSwitch, etc.).
        etype = u16::from_be_bytes([cursor[2], cursor[3]]);
        cursor = &cursor[4..];
    }

    if etype == ETYPE_MPLS_UNICAST {
        let mut labels = Vec::new();
        loop {
            if cursor.len() < 4 {
                return None;
            }
            let raw =
                (u32::from(cursor[0]) << 16) | (u32::from(cursor[1]) << 8) | u32::from(cursor[2]);
            let label = raw >> 4;
            let bottom = cursor[2] & 0x01;
            cursor = &cursor[4..];
            if label > 0 {
                labels.push(label.to_string());
            }
            if bottom == 1 || label <= 15 {
                if cursor.is_empty() {
                    return None;
                }
                etype = match (cursor[0] & 0xf0) >> 4 {
                    4 => 0x0800,
                    6 => 0x86dd,
                    _ => return None,
                };
                break;
            }
        }
        if !labels.is_empty() {
            rec.mpls_labels = labels.join(",");
        }
    }

    match etype {
        0x0800 => parse_ipv4_packet_record(cursor, rec, decapsulation_mode),
        0x86dd => parse_ipv6_packet_record(cursor, rec, decapsulation_mode),
        _ => None,
    }
}

fn parse_ipv4_packet_record(
    data: &[u8],
    rec: &mut FlowRecord,
    decapsulation_mode: DecapsulationMode,
) -> Option<u64> {
    if data.len() < 20 {
        return None;
    }
    let ihl = ((data[0] & 0x0f) as usize).saturating_mul(4);
    if ihl < 20 || ihl > data.len() {
        return None;
    }

    let total_length = u16::from_be_bytes([data[2], data[3]]) as u64;
    let fragment_id = u16::from_be_bytes([data[4], data[5]]);
    let fragment_offset = u16::from_be_bytes([data[6], data[7]]) & 0x1fff;
    let proto = data[9];
    let src = Ipv4Addr::new(data[12], data[13], data[14], data[15]);
    let dst = Ipv4Addr::new(data[16], data[17], data[18], data[19]);

    if decapsulation_mode.is_none() {
        rec.set_etype(2048);
        rec.src_addr = Some(IpAddr::V4(src));
        rec.dst_addr = Some(IpAddr::V4(dst));
        rec.protocol = proto;
        rec.set_iptos(data[1]);
        rec.ipttl = data[8];
        rec.ip_fragment_id = fragment_id as u32;
        rec.ip_fragment_offset = fragment_offset;
    }

    if fragment_offset == 0 {
        let inner_l3_length = parse_transport_record(proto, &data[ihl..], rec, decapsulation_mode);
        if decapsulation_mode.is_none() {
            return Some(total_length);
        }
        return if inner_l3_length > 0 {
            Some(inner_l3_length)
        } else {
            None
        };
    }

    if decapsulation_mode.is_none() {
        Some(total_length)
    } else {
        None
    }
}

fn parse_ipv6_packet_record(
    data: &[u8],
    rec: &mut FlowRecord,
    decapsulation_mode: DecapsulationMode,
) -> Option<u64> {
    if data.len() < 40 {
        return None;
    }

    let payload_length = u16::from_be_bytes([data[4], data[5]]) as u64;
    let next_header = data[6];
    let hop_limit = data[7];
    let mut src_bytes = [0_u8; 16];
    let mut dst_bytes = [0_u8; 16];
    src_bytes.copy_from_slice(&data[8..24]);
    dst_bytes.copy_from_slice(&data[24..40]);
    let src = Ipv6Addr::from(src_bytes);
    let dst = Ipv6Addr::from(dst_bytes);

    if decapsulation_mode.is_none() {
        let traffic_class = ((u16::from_be_bytes([data[0], data[1]]) & 0x0ff0) >> 4) as u8;
        let flow_label = u32::from_be_bytes([data[0], data[1], data[2], data[3]]) & 0x000f_ffff;

        rec.set_etype(34525);
        rec.src_addr = Some(IpAddr::V6(src));
        rec.dst_addr = Some(IpAddr::V6(dst));
        rec.protocol = next_header;
        rec.set_iptos(traffic_class);
        rec.ipttl = hop_limit;
        rec.ipv6_flow_label = flow_label;
    }
    let inner_l3_length = parse_transport_record(next_header, &data[40..], rec, decapsulation_mode);
    if decapsulation_mode.is_none() {
        Some(payload_length.saturating_add(40))
    } else if inner_l3_length > 0 {
        Some(inner_l3_length)
    } else {
        None
    }
}

fn parse_transport_record(
    proto: u8,
    data: &[u8],
    rec: &mut FlowRecord,
    decapsulation_mode: DecapsulationMode,
) -> u64 {
    if !decapsulation_mode.is_none() {
        return match decapsulation_mode {
            DecapsulationMode::Vxlan => {
                if proto == 17
                    && data.len() > 16
                    && u16::from_be_bytes([data[2], data[3]]) == VXLAN_UDP_PORT
                {
                    parse_datalink_frame_section_record(&data[16..], rec, DecapsulationMode::None)
                        .unwrap_or(0)
                } else {
                    0
                }
            }
            DecapsulationMode::Srv6 => parse_srv6_inner_record(proto, data, rec).unwrap_or(0),
            DecapsulationMode::None => 0,
        };
    }

    match proto {
        6 | 17 => {
            if data.len() >= 4 {
                rec.src_port = u16::from_be_bytes([data[0], data[1]]);
                rec.dst_port = u16::from_be_bytes([data[2], data[3]]);
            }
            if proto == 6 && data.len() >= 14 {
                rec.set_tcp_flags(data[13]);
            }
        }
        1 => {
            if data.len() >= 2 {
                rec.set_icmpv4_type(data[0]);
                rec.set_icmpv4_code(data[1]);
            }
        }
        58 => {
            if data.len() >= 2 {
                rec.set_icmpv6_type(data[0]);
                rec.set_icmpv6_code(data[1]);
            }
        }
        _ => {}
    }

    0
}

fn parse_srv6_inner_record(proto: u8, data: &[u8], rec: &mut FlowRecord) -> Option<u64> {
    let mut next = proto;
    let mut cursor = data;

    loop {
        match next {
            4 => return parse_ipv4_packet_record(cursor, rec, DecapsulationMode::None),
            41 => return parse_ipv6_packet_record(cursor, rec, DecapsulationMode::None),
            43 => {
                if cursor.len() < 8 || cursor[2] != 4 {
                    return None;
                }
                let skip = 8_usize.saturating_add((cursor[1] as usize).saturating_mul(8));
                if cursor.len() < skip {
                    return None;
                }
                next = cursor[0];
                cursor = &cursor[skip..];
            }
            _ => return None,
        }
    }
}

fn apply_v9_special_mappings_record(rec: &mut FlowRecord, field: V9Field, value: &str) {
    match field {
        V9Field::IpProtocolVersion => {
            if let Some(etype) = etype_u16_from_ip_version(value) {
                rec.set_etype(etype);
            }
        }
        V9Field::IcmpType => {
            if let Some((icmp_type, icmp_code)) = decode_type_code_raw(value) {
                if !rec.has_icmpv4_type() {
                    rec.set_icmpv4_type(icmp_type);
                }
                if !rec.has_icmpv4_code() {
                    rec.set_icmpv4_code(icmp_code);
                }
                if !rec.has_icmpv6_type() {
                    rec.set_icmpv6_type(
                        value
                            .parse::<u64>()
                            .ok()
                            .map(|v| ((v >> 8) & 0xff) as u8)
                            .unwrap_or(0),
                    );
                }
                if !rec.has_icmpv6_code() {
                    rec.set_icmpv6_code(
                        value
                            .parse::<u64>()
                            .ok()
                            .map(|v| (v & 0xff) as u8)
                            .unwrap_or(0),
                    );
                }
            }
        }
        V9Field::IcmpTypeValue => {
            if !rec.has_icmpv4_type() {
                rec.set_icmpv4_type(value.parse().unwrap_or(0));
            }
        }
        V9Field::IcmpCodeValue => {
            if !rec.has_icmpv4_code() {
                rec.set_icmpv4_code(value.parse().unwrap_or(0));
            }
        }
        V9Field::IcmpIpv6TypeValue => {
            if !rec.has_icmpv6_type() {
                rec.set_icmpv6_type(value.parse().unwrap_or(0));
            }
        }
        V9Field::ImpIpv6CodeValue => {
            if !rec.has_icmpv6_code() {
                rec.set_icmpv6_code(value.parse().unwrap_or(0));
            }
        }
        V9Field::MplsLabel1
        | V9Field::MplsLabel2
        | V9Field::MplsLabel3
        | V9Field::MplsLabel4
        | V9Field::MplsLabel5
        | V9Field::MplsLabel6
        | V9Field::MplsLabel7
        | V9Field::MplsLabel8
        | V9Field::MplsLabel9
        | V9Field::MplsLabel10 => {
            append_mpls_label_record(rec, value);
        }
        _ => {}
    }
}

fn apply_ipfix_special_mappings_record(rec: &mut FlowRecord, field: &IPFixField, value: &str) {
    match field {
        IPFixField::IANA(IANAIPFixField::IpVersion) => {
            if let Some(etype) = etype_u16_from_ip_version(value) {
                rec.set_etype(etype);
            }
        }
        IPFixField::IANA(IANAIPFixField::IcmpTypeCodeIpv4) => {
            if let Some((icmp_type, icmp_code)) = decode_type_code_raw(value) {
                if !rec.has_icmpv4_type() {
                    rec.set_icmpv4_type(icmp_type);
                }
                if !rec.has_icmpv4_code() {
                    rec.set_icmpv4_code(icmp_code);
                }
            }
        }
        IPFixField::IANA(IANAIPFixField::IcmpTypeCodeIpv6) => {
            if let Some((icmp_type, icmp_code)) = decode_type_code_raw(value) {
                if !rec.has_icmpv6_type() {
                    rec.set_icmpv6_type(icmp_type);
                }
                if !rec.has_icmpv6_code() {
                    rec.set_icmpv6_code(icmp_code);
                }
            }
        }
        IPFixField::IANA(IANAIPFixField::MplsTopLabelStackSection)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection2)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection3)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection4)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection5)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection6)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection7)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection8)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection9)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection10) => {
            append_mpls_label_record(rec, value);
        }
        _ => {}
    }
}

fn apply_sampling_state_record(
    rec: &mut FlowRecord,
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampler_id: Option<u64>,
    observed_sampling_rate: Option<u64>,
    sampling: &SamplingState,
) {
    if let Some(rate) = observed_sampling_rate.filter(|rate| *rate > 0) {
        rec.set_sampling_rate(rate);
        return;
    }

    if !rec.has_sampling_rate() {
        if let Some(id) = sampler_id
            && let Some(rate) = sampling.get(exporter_ip, version, observation_domain_id, id)
        {
            rec.set_sampling_rate(rate);
            return;
        }

        if let Some(rate) = sampling.get(exporter_ip, version, observation_domain_id, 0) {
            rec.set_sampling_rate(rate);
        }
    }
}

fn apply_sampling_state_fields(
    fields: &mut FlowFields,
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampler_id: Option<u64>,
    observed_sampling_rate: Option<u64>,
    sampling: &SamplingState,
) {
    if let Some(rate) = observed_sampling_rate.filter(|rate| *rate > 0) {
        fields.insert("SAMPLING_RATE", rate.to_string());
        return;
    }

    if fields.contains_key("SAMPLING_RATE") {
        return;
    }

    if let Some(id) = sampler_id
        && let Some(rate) = sampling.get(exporter_ip, version, observation_domain_id, id)
    {
        fields.insert("SAMPLING_RATE", rate.to_string());
        return;
    }

    if let Some(rate) = sampling.get(exporter_ip, version, observation_domain_id, 0) {
        fields.insert("SAMPLING_RATE", rate.to_string());
    }
}

fn looks_like_sampling_option_record_from_rec(
    rec: &FlowRecord,
    observed_sampling_rate: Option<u64>,
) -> bool {
    if observed_sampling_rate.unwrap_or(0) == 0 {
        return false;
    }
    // No endpoints = likely a sampling option record, not a data flow
    rec.src_addr.is_none() && rec.dst_addr.is_none()
}

fn sflow_agent_ip_addr(address: &Address) -> Option<IpAddr> {
    match address {
        Address::IPv4(ip) => Some(IpAddr::V4(*ip)),
        Address::IPv6(ip) => Some(IpAddr::V6(*ip)),
        Address::Unknown => None,
    }
}

fn normalize_direction_value(value: &str) -> &str {
    match value.parse::<u64>().ok() {
        // Akvorado parity: IPFIX/NetFlow flowDirection 0=ingress, 1=egress.
        Some(0) => DIRECTION_INGRESS,
        Some(1) => DIRECTION_EGRESS,
        _ => value,
    }
}

fn apply_icmp_port_fallback(fields: &mut FlowFields) {
    let protocol = fields
        .get("PROTOCOL")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);
    let src_port_present = scalar_field_present_in_map(fields, "SRC_PORT");
    let dst_port_present = scalar_field_present_in_map(fields, "DST_PORT");
    let src_port = fields
        .get("SRC_PORT")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);
    let dst_port = fields
        .get("DST_PORT")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);

    if (src_port_present && src_port != 0) || !dst_port_present {
        return;
    }

    let icmp_type = ((dst_port >> 8) & 0xff).to_string();
    let icmp_code = (dst_port & 0xff).to_string();

    match protocol {
        1 => {
            set_if_missing_or_empty(fields, "ICMPV4_TYPE", &icmp_type);
            set_if_missing_or_empty(fields, "ICMPV4_CODE", &icmp_code);
        }
        58 => {
            set_if_missing_or_empty(fields, "ICMPV6_TYPE", &icmp_type);
            set_if_missing_or_empty(fields, "ICMPV6_CODE", &icmp_code);
        }
        _ => {}
    }
}

fn set_if_missing_or_empty(fields: &mut FlowFields, key: &'static str, value: &str) {
    let current = fields.get(key).map(String::as_str).unwrap_or_default();
    if current.is_empty() {
        fields.insert(key, value.to_string());
    }
}

fn is_zero_ip_value(value: &str) -> bool {
    matches!(value, "0.0.0.0" | "::" | "::ffff:0.0.0.0")
}

fn should_skip_zero_ip(canonical: &str, value: &str) -> bool {
    matches!(
        canonical,
        "SRC_ADDR" | "DST_ADDR" | "NEXT_HOP" | "SRC_ADDR_NAT" | "DST_ADDR_NAT"
    ) && is_zero_ip_value(value)
}

fn append_mpls_label(fields: &mut FlowFields, value: &str) {
    let raw = if let Ok(v) = value.parse::<u64>() {
        v
    } else if let Some(hex) = value
        .strip_prefix("0x")
        .or_else(|| value.strip_prefix("0X"))
    {
        u64::from_str_radix(hex, 16).ok().unwrap_or(0)
    } else if value.chars().all(|c| c.is_ascii_hexdigit()) {
        u64::from_str_radix(value, 16).ok().unwrap_or(0)
    } else {
        0
    };
    if raw == 0 {
        return;
    }
    let label = raw >> 4;
    if label == 0 {
        return;
    }

    let labels = fields.entry("MPLS_LABELS").or_default();
    if labels.is_empty() {
        *labels = label.to_string();
    } else {
        labels.push(',');
        labels.push_str(&label.to_string());
    }
}

fn append_mpls_label_value(fields: &mut FlowFields, label: u64) {
    let labels = fields.entry("MPLS_LABELS").or_default();
    if labels.is_empty() {
        *labels = label.to_string();
    } else {
        labels.push(',');
        labels.push_str(&label.to_string());
    }
}

fn parse_ip_value(raw_value: &[u8]) -> Option<String> {
    match raw_value.len() {
        4 => {
            Some(Ipv4Addr::new(raw_value[0], raw_value[1], raw_value[2], raw_value[3]).to_string())
        }
        16 => {
            let mut octets = [0_u8; 16];
            octets.copy_from_slice(raw_value);
            Some(Ipv6Addr::from(octets).to_string())
        }
        _ => None,
    }
}

fn observe_v9_sampling_options(
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampling: &mut SamplingState,
    options_data: V9OptionsData,
) {
    for record in options_data.fields {
        let mut sampler_id = 0_u64;
        let mut rate: Option<u64> = None;

        for fields in record.options_fields {
            for (field, value) in fields {
                let value_str = field_value_to_string(&value);
                match field {
                    V9Field::FlowSamplerId => {
                        if let Ok(parsed) = value_str.parse::<u64>() {
                            sampler_id = parsed;
                        }
                    }
                    V9Field::SamplingInterval | V9Field::FlowSamplerRandomInterval => {
                        rate = value_str.parse::<u64>().ok();
                    }
                    _ => {}
                }
            }
        }

        if let Some(rate) = rate.filter(|rate| *rate > 0) {
            sampling.set(
                exporter_ip,
                version,
                observation_domain_id,
                sampler_id,
                rate,
            );
        }
    }
}

fn observe_ipfix_sampling_options(
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampling: &mut SamplingState,
    options_data: IPFixOptionsData,
) {
    for record in options_data.fields {
        let mut sampler_id = 0_u64;
        let mut rate: Option<u64> = None;
        let mut packet_interval: Option<u64> = None;
        let mut packet_space: Option<u64> = None;

        for (field, value) in record {
            let value_str = field_value_to_string(&value);
            match field {
                IPFixField::IANA(IANAIPFixField::SamplerId)
                | IPFixField::IANA(IANAIPFixField::SelectorId) => {
                    if let Ok(parsed) = value_str.parse::<u64>() {
                        sampler_id = parsed;
                    }
                }
                IPFixField::IANA(IANAIPFixField::SamplingInterval)
                | IPFixField::IANA(IANAIPFixField::SamplerRandomInterval) => {
                    rate = value_str.parse::<u64>().ok();
                }
                IPFixField::IANA(IANAIPFixField::SamplingPacketInterval) => {
                    packet_interval = value_str.parse::<u64>().ok();
                }
                IPFixField::IANA(IANAIPFixField::SamplingPacketSpace) => {
                    packet_space = value_str.parse::<u64>().ok();
                }
                _ => {}
            }
        }

        if let (Some(interval), Some(space)) = (packet_interval, packet_space) {
            if interval > 0 {
                rate = Some((interval.saturating_add(space)) / interval);
            }
        }

        if let Some(rate) = rate.filter(|rate| *rate > 0) {
            sampling.set(
                exporter_ip,
                version,
                observation_domain_id,
                sampler_id,
                rate,
            );
        }
    }
}

fn infer_etype_from_endpoints(fields: &FlowFields) -> Option<&'static str> {
    let src_addr = fields
        .get("SRC_ADDR")
        .map(String::as_str)
        .unwrap_or_default();
    let dst_addr = fields
        .get("DST_ADDR")
        .map(String::as_str)
        .unwrap_or_default();
    let probe = if !src_addr.is_empty() {
        src_addr
    } else {
        dst_addr
    };
    if probe.is_empty() {
        return None;
    }

    if probe.contains(':') {
        Some(ETYPE_IPV6)
    } else if probe.contains('.') {
        Some(ETYPE_IPV4)
    } else {
        None
    }
}

fn decode_type_code(value: &str) -> Option<(String, String)> {
    let type_code = value.parse::<u64>().ok()?;
    let icmp_type = ((type_code >> 8) & 0xff).to_string();
    let icmp_code = (type_code & 0xff).to_string();
    Some((icmp_type, icmp_code))
}

fn etype_from_ip_version(value: &str) -> Option<&'static str> {
    match value.parse::<u64>().ok() {
        Some(4) => Some(ETYPE_IPV4),
        Some(6) => Some(ETYPE_IPV6),
        _ => None,
    }
}

fn apply_v9_special_mappings(fields: &mut FlowFields, field: V9Field, value: &str) {
    match field {
        V9Field::IpProtocolVersion => {
            if let Some(etype) = etype_from_ip_version(value) {
                fields.insert("ETYPE", etype.to_string());
            }
        }
        V9Field::IcmpType => {
            if let Some((icmp_type, icmp_code)) = decode_type_code(value) {
                // Field 32 appears in both v4/v6 paths in some exporters.
                fields.entry("ICMPV4_TYPE").or_insert(icmp_type);
                fields.entry("ICMPV4_CODE").or_insert(icmp_code);
                fields.entry("ICMPV6_TYPE").or_insert_with(|| {
                    value
                        .parse::<u64>()
                        .ok()
                        .map(|v| (v >> 8).to_string())
                        .unwrap_or_default()
                });
                fields.entry("ICMPV6_CODE").or_insert_with(|| {
                    value
                        .parse::<u64>()
                        .ok()
                        .map(|v| (v & 0xff).to_string())
                        .unwrap_or_default()
                });
            }
        }
        V9Field::IcmpTypeValue => {
            fields
                .entry("ICMPV4_TYPE")
                .or_insert_with(|| value.to_string());
        }
        V9Field::IcmpCodeValue => {
            fields
                .entry("ICMPV4_CODE")
                .or_insert_with(|| value.to_string());
        }
        V9Field::IcmpIpv6TypeValue => {
            fields
                .entry("ICMPV6_TYPE")
                .or_insert_with(|| value.to_string());
        }
        V9Field::ImpIpv6CodeValue => {
            fields
                .entry("ICMPV6_CODE")
                .or_insert_with(|| value.to_string());
        }
        V9Field::MplsLabel1
        | V9Field::MplsLabel2
        | V9Field::MplsLabel3
        | V9Field::MplsLabel4
        | V9Field::MplsLabel5
        | V9Field::MplsLabel6
        | V9Field::MplsLabel7
        | V9Field::MplsLabel8
        | V9Field::MplsLabel9
        | V9Field::MplsLabel10 => {
            append_mpls_label(fields, value);
        }
        _ => {}
    }
}

fn apply_reverse_ipfix_special_mappings(
    fields: &mut FlowFields,
    field: &ReverseInformationElement,
    value: &str,
) {
    match field {
        ReverseInformationElement::ReverseIpVersion => {
            if let Some(etype) = etype_from_ip_version(value) {
                fields.insert("ETYPE", etype.to_string());
            }
        }
        ReverseInformationElement::ReverseIcmpTypeCodeIPv4 => {
            if let Some((icmp_type, icmp_code)) = decode_type_code(value) {
                fields.entry("ICMPV4_TYPE").or_insert(icmp_type);
                fields.entry("ICMPV4_CODE").or_insert(icmp_code);
            }
        }
        ReverseInformationElement::ReverseIcmpTypeCodeIPv6 => {
            if let Some((icmp_type, icmp_code)) = decode_type_code(value) {
                fields.entry("ICMPV6_TYPE").or_insert(icmp_type);
                fields.entry("ICMPV6_CODE").or_insert(icmp_code);
            }
        }
        ReverseInformationElement::ReverseMplsTopLabelStackSection
        | ReverseInformationElement::ReverseMplsLabelStackSection2
        | ReverseInformationElement::ReverseMplsLabelStackSection3
        | ReverseInformationElement::ReverseMplsLabelStackSection4
        | ReverseInformationElement::ReverseMplsLabelStackSection5
        | ReverseInformationElement::ReverseMplsLabelStackSection6
        | ReverseInformationElement::ReverseMplsLabelStackSection7
        | ReverseInformationElement::ReverseMplsLabelStackSection8
        | ReverseInformationElement::ReverseMplsLabelStackSection9
        | ReverseInformationElement::ReverseMplsLabelStackSection10 => {
            append_mpls_label(fields, value);
        }
        _ => {}
    }
}

fn v9_canonical_key(field: V9Field) -> Option<&'static str> {
    match field {
        V9Field::InBytes => Some("BYTES"),
        V9Field::InPkts => Some("PACKETS"),
        V9Field::Flows => Some("FLOWS"),
        V9Field::IpProtocolVersion => Some("ETYPE"),
        V9Field::Protocol => Some("PROTOCOL"),
        V9Field::SrcTos | V9Field::DstTos => Some("IPTOS"),
        V9Field::TcpFlags => Some("TCP_FLAGS"),
        V9Field::L4SrcPort => Some("SRC_PORT"),
        V9Field::L4DstPort => Some("DST_PORT"),
        V9Field::Ipv4SrcAddr | V9Field::Ipv6SrcAddr => Some("SRC_ADDR"),
        V9Field::Ipv4DstAddr | V9Field::Ipv6DstAddr => Some("DST_ADDR"),
        V9Field::Ipv4NextHop
        | V9Field::BgpIpv4NextHop
        | V9Field::Ipv6NextHop
        | V9Field::BpgIpv6NextHop => Some("NEXT_HOP"),
        V9Field::SrcAs => Some("SRC_AS"),
        V9Field::DstAs => Some("DST_AS"),
        V9Field::InputSnmp => Some("IN_IF"),
        V9Field::OutputSnmp => Some("OUT_IF"),
        V9Field::SrcMask | V9Field::Ipv6SrcMask => Some("SRC_MASK"),
        V9Field::DstMask | V9Field::Ipv6DstMask => Some("DST_MASK"),
        V9Field::Ipv4SrcPrefix => Some("SRC_PREFIX"),
        V9Field::Ipv4DstPrefix => Some("DST_PREFIX"),
        V9Field::SrcVlan => Some("SRC_VLAN"),
        V9Field::DstVlan => Some("DST_VLAN"),
        V9Field::ForwardingStatus => Some("FORWARDING_STATUS"),
        V9Field::SamplingInterval => Some("SAMPLING_RATE"),
        V9Field::Direction => Some("DIRECTION"),
        V9Field::MinTtl | V9Field::MaxTtl => Some("IPTTL"),
        V9Field::Ipv6FlowLabel => Some("IPV6_FLOW_LABEL"),
        V9Field::Ipv4Ident => Some("IP_FRAGMENT_ID"),
        V9Field::FragmentOffset => Some("IP_FRAGMENT_OFFSET"),
        V9Field::ObservationTimeMilliseconds => Some("OBSERVATION_TIME_MILLIS"),
        V9Field::InSrcMac | V9Field::OutSrcMac => Some("SRC_MAC"),
        V9Field::InDstMac | V9Field::OutDstMac => Some("DST_MAC"),
        V9Field::PostNATSourceIPv4Address | V9Field::PostNATSourceIpv6Address => {
            Some("SRC_ADDR_NAT")
        }
        V9Field::PostNATDestinationIPv4Address | V9Field::PostNATDestinationIpv6Address => {
            Some("DST_ADDR_NAT")
        }
        V9Field::PostNATTSourceTransportPort => Some("SRC_PORT_NAT"),
        V9Field::PostNATTDestinationTransportPort => Some("DST_PORT_NAT"),
        _ => None,
    }
}

fn ipfix_canonical_key(field: &IPFixField) -> Option<&'static str> {
    match field {
        IPFixField::IANA(IANAIPFixField::OctetDeltaCount) => Some("BYTES"),
        IPFixField::IANA(IANAIPFixField::PostOctetDeltaCount) => Some("BYTES"),
        IPFixField::IANA(IANAIPFixField::InitiatorOctets) => Some("BYTES"),
        IPFixField::IANA(IANAIPFixField::PacketDeltaCount) => Some("PACKETS"),
        IPFixField::IANA(IANAIPFixField::PostPacketDeltaCount) => Some("PACKETS"),
        IPFixField::IANA(IANAIPFixField::InitiatorPackets) => Some("PACKETS"),
        IPFixField::IANA(IANAIPFixField::IpVersion) => Some("ETYPE"),
        IPFixField::IANA(IANAIPFixField::ProtocolIdentifier) => Some("PROTOCOL"),
        IPFixField::IANA(IANAIPFixField::IpClassOfService)
        | IPFixField::IANA(IANAIPFixField::PostIpClassOfService) => Some("IPTOS"),
        IPFixField::IANA(IANAIPFixField::TcpControlBits) => Some("TCP_FLAGS"),
        IPFixField::IANA(IANAIPFixField::SourceTransportPort) => Some("SRC_PORT"),
        IPFixField::IANA(IANAIPFixField::DestinationTransportPort) => Some("DST_PORT"),
        IPFixField::IANA(IANAIPFixField::SourceIpv4address)
        | IPFixField::IANA(IANAIPFixField::SourceIpv6address) => Some("SRC_ADDR"),
        IPFixField::IANA(IANAIPFixField::DestinationIpv4address)
        | IPFixField::IANA(IANAIPFixField::DestinationIpv6address) => Some("DST_ADDR"),
        IPFixField::IANA(IANAIPFixField::SourceIpv4prefixLength)
        | IPFixField::IANA(IANAIPFixField::SourceIpv6prefixLength) => Some("SRC_MASK"),
        IPFixField::IANA(IANAIPFixField::DestinationIpv4prefixLength)
        | IPFixField::IANA(IANAIPFixField::DestinationIpv6prefixLength) => Some("DST_MASK"),
        IPFixField::IANA(IANAIPFixField::SourceIpv4prefix) => Some("SRC_PREFIX"),
        IPFixField::IANA(IANAIPFixField::DestinationIpv4prefix) => Some("DST_PREFIX"),
        IPFixField::IANA(IANAIPFixField::IpNextHopIpv4address)
        | IPFixField::IANA(IANAIPFixField::BgpNextHopIpv4address)
        | IPFixField::IANA(IANAIPFixField::IpNextHopIpv6address)
        | IPFixField::IANA(IANAIPFixField::BgpNextHopIpv6address) => Some("NEXT_HOP"),
        IPFixField::IANA(IANAIPFixField::BgpSourceAsNumber) => Some("SRC_AS"),
        IPFixField::IANA(IANAIPFixField::BgpDestinationAsNumber) => Some("DST_AS"),
        IPFixField::IANA(IANAIPFixField::IngressInterface) => Some("IN_IF"),
        IPFixField::IANA(IANAIPFixField::IngressPhysicalInterface) => Some("IN_IF"),
        IPFixField::IANA(IANAIPFixField::EgressInterface) => Some("OUT_IF"),
        IPFixField::IANA(IANAIPFixField::EgressPhysicalInterface) => Some("OUT_IF"),
        IPFixField::IANA(IANAIPFixField::VlanId)
        | IPFixField::IANA(IANAIPFixField::Dot1qVlanId) => Some("SRC_VLAN"),
        IPFixField::IANA(IANAIPFixField::PostVlanId)
        | IPFixField::IANA(IANAIPFixField::PostDot1qVlanId) => Some("DST_VLAN"),
        IPFixField::IANA(IANAIPFixField::ForwardingStatus) => Some("FORWARDING_STATUS"),
        IPFixField::IANA(IANAIPFixField::SamplingInterval) => Some("SAMPLING_RATE"),
        IPFixField::IANA(IANAIPFixField::SamplerRandomInterval) => Some("SAMPLING_RATE"),
        IPFixField::IANA(IANAIPFixField::FlowDirection)
        | IPFixField::IANA(IANAIPFixField::BiflowDirection) => Some("DIRECTION"),
        IPFixField::IANA(IANAIPFixField::MinimumTtl) | IPFixField::IANA(IANAIPFixField::IpTtl) => {
            Some("IPTTL")
        }
        IPFixField::IANA(IANAIPFixField::FlowLabelIpv6) => Some("IPV6_FLOW_LABEL"),
        IPFixField::IANA(IANAIPFixField::FragmentIdentification) => Some("IP_FRAGMENT_ID"),
        IPFixField::IANA(IANAIPFixField::FragmentOffset) => Some("IP_FRAGMENT_OFFSET"),
        IPFixField::IANA(IANAIPFixField::IcmpTypeIpv4) => Some("ICMPV4_TYPE"),
        IPFixField::IANA(IANAIPFixField::IcmpCodeIpv4) => Some("ICMPV4_CODE"),
        IPFixField::IANA(IANAIPFixField::IcmpTypeIpv6) => Some("ICMPV6_TYPE"),
        IPFixField::IANA(IANAIPFixField::IcmpCodeIpv6) => Some("ICMPV6_CODE"),
        IPFixField::IANA(IANAIPFixField::SourceMacaddress)
        | IPFixField::IANA(IANAIPFixField::PostSourceMacaddress) => Some("SRC_MAC"),
        IPFixField::IANA(IANAIPFixField::DestinationMacaddress)
        | IPFixField::IANA(IANAIPFixField::PostDestinationMacaddress) => Some("DST_MAC"),
        IPFixField::IANA(IANAIPFixField::PostNatsourceIpv4address)
        | IPFixField::IANA(IANAIPFixField::PostNatsourceIpv6address) => Some("SRC_ADDR_NAT"),
        IPFixField::IANA(IANAIPFixField::PostNatdestinationIpv4address)
        | IPFixField::IANA(IANAIPFixField::PostNatdestinationIpv6address) => Some("DST_ADDR_NAT"),
        IPFixField::IANA(IANAIPFixField::PostNaptsourceTransportPort) => Some("SRC_PORT_NAT"),
        IPFixField::IANA(IANAIPFixField::PostNaptdestinationTransportPort) => Some("DST_PORT_NAT"),
        _ => None,
    }
}

fn reverse_ipfix_canonical_key(field: &ReverseInformationElement) -> Option<&'static str> {
    match field {
        ReverseInformationElement::ReverseOctetDeltaCount
        | ReverseInformationElement::ReversePostOctetDeltaCount
        | ReverseInformationElement::ReverseInitiatorOctets
        | ReverseInformationElement::ReverseResponderOctets => Some("BYTES"),
        ReverseInformationElement::ReversePacketDeltaCount
        | ReverseInformationElement::ReversePostPacketDeltaCount
        | ReverseInformationElement::ReverseInitiatorPackets
        | ReverseInformationElement::ReverseResponderPackets => Some("PACKETS"),
        ReverseInformationElement::ReverseProtocolIdentifier => Some("PROTOCOL"),
        ReverseInformationElement::ReverseIpClassOfService
        | ReverseInformationElement::ReversePostIpClassOfService => Some("IPTOS"),
        ReverseInformationElement::ReverseTcpControlBits => Some("TCP_FLAGS"),
        ReverseInformationElement::ReverseSourceTransportPort
        | ReverseInformationElement::ReverseUdpSourcePort
        | ReverseInformationElement::ReverseTcpSourcePort => Some("SRC_PORT"),
        ReverseInformationElement::ReverseDestinationTransportPort
        | ReverseInformationElement::ReverseUdpDestinationPort
        | ReverseInformationElement::ReverseTcpDestinationPort => Some("DST_PORT"),
        ReverseInformationElement::ReverseSourceIPv4Address
        | ReverseInformationElement::ReverseSourceIPv6Address => Some("SRC_ADDR"),
        ReverseInformationElement::ReverseDestinationIPv4Address
        | ReverseInformationElement::ReverseDestinationIPv6Address => Some("DST_ADDR"),
        ReverseInformationElement::ReverseSourceIPv4PrefixLength
        | ReverseInformationElement::ReverseSourceIPv6PrefixLength => Some("SRC_MASK"),
        ReverseInformationElement::ReverseDestinationIPv4PrefixLength
        | ReverseInformationElement::ReverseDestinationIPv6PrefixLength => Some("DST_MASK"),
        ReverseInformationElement::ReverseIpNextHopIPv4Address
        | ReverseInformationElement::ReverseIpNextHopIPv6Address
        | ReverseInformationElement::ReverseBgpNextHopIPv4Address
        | ReverseInformationElement::ReverseBgpNextHopIPv6Address => Some("NEXT_HOP"),
        ReverseInformationElement::ReverseBgpSourceAsNumber => Some("SRC_AS"),
        ReverseInformationElement::ReverseBgpDestinationAsNumber => Some("DST_AS"),
        ReverseInformationElement::ReverseIngressInterface => Some("IN_IF"),
        ReverseInformationElement::ReverseEgressInterface => Some("OUT_IF"),
        ReverseInformationElement::ReverseVlanId => Some("SRC_VLAN"),
        ReverseInformationElement::ReversePostVlanId => Some("DST_VLAN"),
        ReverseInformationElement::ReverseSourceMacAddress
        | ReverseInformationElement::ReversePostSourceMacAddress => Some("SRC_MAC"),
        ReverseInformationElement::ReverseDestinationMacAddress
        | ReverseInformationElement::ReversePostDestinationMacAddress => Some("DST_MAC"),
        ReverseInformationElement::ReverseForwardingStatus => Some("FORWARDING_STATUS"),
        ReverseInformationElement::ReverseSamplingInterval
        | ReverseInformationElement::ReverseSamplerRandomInterval => Some("SAMPLING_RATE"),
        ReverseInformationElement::ReverseFlowDirection => Some("DIRECTION"),
        ReverseInformationElement::ReverseMinimumTTL
        | ReverseInformationElement::ReverseMaximumTTL => Some("IPTTL"),
        ReverseInformationElement::ReverseFlowLabelIPv6 => Some("IPV6_FLOW_LABEL"),
        ReverseInformationElement::ReverseFragmentIdentification => Some("IP_FRAGMENT_ID"),
        ReverseInformationElement::ReverseFragmentOffset => Some("IP_FRAGMENT_OFFSET"),
        ReverseInformationElement::ReverseIcmpTypeIPv4 => Some("ICMPV4_TYPE"),
        ReverseInformationElement::ReverseIcmpCodeIPv4 => Some("ICMPV4_CODE"),
        ReverseInformationElement::ReverseIcmpTypeIPv6 => Some("ICMPV6_TYPE"),
        ReverseInformationElement::ReverseIcmpCodeIPv6 => Some("ICMPV6_CODE"),
        ReverseInformationElement::ReverseIpVersion => Some("ETYPE"),
        _ => None,
    }
}

fn field_value_to_string(value: &FieldValue) -> String {
    match value {
        FieldValue::ApplicationId(app) => {
            format!(
                "{}:{}",
                app.classification_engine_id,
                data_number_to_string(&app.selector_id)
            )
        }
        FieldValue::String(v) => v.clone(),
        FieldValue::DataNumber(v) => data_number_to_string(v),
        FieldValue::Float64(v) => v.to_string(),
        FieldValue::Duration(v) => v.as_millis().to_string(),
        FieldValue::Ip4Addr(v) => v.to_string(),
        FieldValue::Ip6Addr(v) => v.to_string(),
        FieldValue::MacAddr(v) => v.to_string(),
        FieldValue::Vec(v) | FieldValue::Unknown(v) => bytes_to_hex(v),
        FieldValue::ProtocolType(v) => u8::from(*v).to_string(),
    }
}

fn field_value_unsigned(value: &FieldValue) -> Option<u64> {
    match value {
        FieldValue::DataNumber(number) => match number {
            DataNumber::U8(v) => Some(u64::from(*v)),
            DataNumber::U16(v) => Some(u64::from(*v)),
            DataNumber::U24(v) => Some(u64::from(*v)),
            DataNumber::U32(v) => Some(u64::from(*v)),
            DataNumber::U64(v) => Some(*v),
            DataNumber::I8(v) if *v >= 0 => Some(*v as u64),
            DataNumber::I16(v) if *v >= 0 => Some(*v as u64),
            DataNumber::I24(v) if *v >= 0 => Some(*v as u64),
            DataNumber::I32(v) if *v >= 0 => Some(*v as u64),
            DataNumber::I64(v) if *v >= 0 => Some(*v as u64),
            DataNumber::U128(v) => u64::try_from(*v).ok(),
            DataNumber::I128(v) if *v >= 0 => u64::try_from(*v).ok(),
            _ => None,
        },
        _ => None,
    }
}

fn field_value_duration_usec(value: &FieldValue) -> Option<u64> {
    match value {
        FieldValue::Duration(duration) => u64::try_from(duration.as_micros()).ok(),
        _ => None,
    }
}

fn data_number_to_string(value: &DataNumber) -> String {
    match value {
        DataNumber::U8(v) => v.to_string(),
        DataNumber::I8(v) => v.to_string(),
        DataNumber::U16(v) => v.to_string(),
        DataNumber::I16(v) => v.to_string(),
        DataNumber::U24(v) => v.to_string(),
        DataNumber::I24(v) => v.to_string(),
        DataNumber::U32(v) => v.to_string(),
        DataNumber::U64(v) => v.to_string(),
        DataNumber::I64(v) => v.to_string(),
        DataNumber::U128(v) => v.to_string(),
        DataNumber::I128(v) => v.to_string(),
        DataNumber::I32(v) => v.to_string(),
    }
}

fn bytes_to_hex(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}

fn reverse_ipfix_timestamp_to_usec(
    field: &ReverseInformationElement,
    value: &FieldValue,
    export_usec: u64,
    system_init_millis: Option<u64>,
) -> Option<u64> {
    match field {
        ReverseInformationElement::ReverseFlowStartSeconds
        | ReverseInformationElement::ReverseFlowEndSeconds => {
            field_value_unsigned(value).map(unix_seconds_to_usec)
        }
        ReverseInformationElement::ReverseFlowStartMilliseconds
        | ReverseInformationElement::ReverseFlowEndMilliseconds => {
            field_value_unsigned(value).map(|v| v.saturating_mul(USEC_PER_MILLISECOND))
        }
        ReverseInformationElement::ReverseFlowStartMicroseconds
        | ReverseInformationElement::ReverseFlowEndMicroseconds
        | ReverseInformationElement::ReverseMinFlowStartMicroseconds
        | ReverseInformationElement::ReverseMaxFlowEndMicroseconds => {
            field_value_duration_usec(value)
        }
        ReverseInformationElement::ReverseFlowStartNanoseconds
        | ReverseInformationElement::ReverseFlowEndNanoseconds
        | ReverseInformationElement::ReverseMinFlowStartNanoseconds
        | ReverseInformationElement::ReverseMaxFlowEndNanoseconds => {
            field_value_duration_usec(value)
        }
        ReverseInformationElement::ReverseFlowStartDeltaMicroseconds
        | ReverseInformationElement::ReverseFlowEndDeltaMicroseconds => {
            field_value_unsigned(value).map(|delta| export_usec.saturating_sub(delta))
        }
        ReverseInformationElement::ReverseFlowStartSysUpTime
        | ReverseInformationElement::ReverseFlowEndSysUpTime => {
            let system_init_usec = system_init_millis?.saturating_mul(USEC_PER_MILLISECOND);
            field_value_unsigned(value).map(|uptime_millis| {
                system_init_usec.saturating_add(uptime_millis.saturating_mul(USEC_PER_MILLISECOND))
            })
        }
        _ => None,
    }
}

fn resolve_ipfix_time_usec(
    seconds: Option<u64>,
    millis: Option<u64>,
    micros: Option<u64>,
    nanos: Option<u64>,
    delta_micros: Option<u64>,
    sys_uptime_millis: Option<u64>,
    system_init_millis: Option<u64>,
    export_usec: u64,
) -> Option<u64> {
    seconds
        .map(unix_seconds_to_usec)
        .or_else(|| millis.map(|value| value.saturating_mul(USEC_PER_MILLISECOND)))
        .or(micros)
        .or(nanos)
        .or_else(|| delta_micros.map(|value| export_usec.saturating_sub(value)))
        .or_else(|| {
            let system_init_usec = system_init_millis?.saturating_mul(USEC_PER_MILLISECOND);
            let uptime_millis = sys_uptime_millis?;
            Some(
                system_init_usec.saturating_add(uptime_millis.saturating_mul(USEC_PER_MILLISECOND)),
            )
        })
}

fn decode_sampling_interval(raw: u16) -> u32 {
    let interval = raw & 0x3fff;
    if interval == 0 { 1 } else { interval as u32 }
}

fn template_scope(payload: &[u8]) -> Option<(u16, u32)> {
    if payload.len() < 2 {
        return None;
    }
    let version = u16::from_be_bytes([payload[0], payload[1]]);
    match version {
        9 => {
            if payload.len() < 20 {
                return None;
            }
            let source_id =
                u32::from_be_bytes([payload[16], payload[17], payload[18], payload[19]]);
            Some((version, source_id))
        }
        10 => {
            if payload.len() < 16 {
                return None;
            }
            let observation_domain_id =
                u32::from_be_bytes([payload[12], payload[13], payload[14], payload[15]]);
            Some((version, observation_domain_id))
        }
        _ => None,
    }
}

fn unix_timestamp_to_usec(seconds: u64, nanos: u64) -> u64 {
    seconds
        .saturating_mul(1_000_000)
        .saturating_add(nanos / 1_000)
}

fn now_usec() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_micros() as u64)
        .unwrap_or(0)
}

#[cfg(test)]
fn to_field_token(name: &str) -> String {
    let mut out = String::with_capacity(name.len() + 8);
    let mut prev_is_sep = true;
    let mut prev_is_lower_or_digit = false;

    for ch in name.chars() {
        if ch.is_ascii_alphanumeric() {
            if ch.is_ascii_uppercase() && prev_is_lower_or_digit && !out.ends_with('_') {
                out.push('_');
            }
            if ch.is_ascii_digit() && prev_is_lower_or_digit && !out.ends_with('_') {
                out.push('_');
            }
            out.push(ch.to_ascii_uppercase());
            prev_is_sep = false;
            prev_is_lower_or_digit = ch.is_ascii_lowercase() || ch.is_ascii_digit();
        } else {
            if !prev_is_sep && !out.ends_with('_') {
                out.push('_');
            }
            prev_is_sep = true;
            prev_is_lower_or_digit = false;
        }
    }

    while out.ends_with('_') {
        out.pop();
    }

    if out.is_empty() {
        "UNKNOWN".to_string()
    } else {
        out
    }
}
