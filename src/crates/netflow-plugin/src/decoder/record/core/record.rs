use crate::decoder::{FlowPresence, FlowRecord, canonicalize_ip_addr, default_exporter_name};
use std::net::SocketAddr;

/// Create a base FlowRecord with exporter identity populated.
pub(crate) fn base_record(version: &'static str, source: SocketAddr) -> FlowRecord {
    FlowRecord {
        flow_version: version,
        exporter_ip: Some(canonicalize_ip_addr(source.ip())),
        exporter_port: source.port(),
        ..Default::default()
    }
}

/// Finalize a FlowRecord: apply defaults, normalize values.
/// Equivalent of `finalize_canonical_flow_fields` for FlowRecord.
pub(crate) fn finalize_record(rec: &mut FlowRecord) {
    if rec.raw_bytes == 0 {
        rec.raw_bytes = rec.bytes;
    }
    if rec.raw_packets == 0 {
        rec.raw_packets = rec.packets;
    }

    let sampling_rate = rec.sampling_rate.max(1);
    rec.bytes = rec.bytes.saturating_mul(sampling_rate);
    rec.packets = rec.packets.saturating_mul(sampling_rate);

    if rec.flows == 0 {
        rec.flows = 1;
    }

    if rec.exporter_name.is_empty()
        && let Some(ip) = rec.exporter_ip
    {
        rec.exporter_name = default_exporter_name(&ip.to_string());
    }

    apply_icmp_port_fallback_record(rec);

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
pub(crate) fn apply_icmp_port_fallback_record(rec: &mut FlowRecord) {
    if (rec.has_src_port() && rec.src_port != 0) || !rec.has_dst_port() {
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
pub(crate) fn swap_directional_record_fields(rec: &mut FlowRecord) {
    std::mem::swap(&mut rec.src_addr, &mut rec.dst_addr);
    std::mem::swap(&mut rec.src_prefix, &mut rec.dst_prefix);
    std::mem::swap(&mut rec.src_mask, &mut rec.dst_mask);
    std::mem::swap(&mut rec.src_port, &mut rec.dst_port);
    std::mem::swap(&mut rec.src_as, &mut rec.dst_as);
    std::mem::swap(&mut rec.src_as_name, &mut rec.dst_as_name);
    std::mem::swap(&mut rec.src_net_name, &mut rec.dst_net_name);
    std::mem::swap(&mut rec.src_net_role, &mut rec.dst_net_role);
    std::mem::swap(&mut rec.src_net_site, &mut rec.dst_net_site);
    std::mem::swap(&mut rec.src_net_region, &mut rec.dst_net_region);
    std::mem::swap(&mut rec.src_net_tenant, &mut rec.dst_net_tenant);
    std::mem::swap(&mut rec.src_country, &mut rec.dst_country);
    std::mem::swap(&mut rec.src_geo_city, &mut rec.dst_geo_city);
    std::mem::swap(&mut rec.src_geo_state, &mut rec.dst_geo_state);
    std::mem::swap(&mut rec.src_geo_latitude, &mut rec.dst_geo_latitude);
    std::mem::swap(&mut rec.src_geo_longitude, &mut rec.dst_geo_longitude);
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
