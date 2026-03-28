use super::super::super::*;
use super::parse_transport_record;

pub(crate) fn parse_ipv4_packet_record(
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

pub(crate) fn parse_ipv6_packet_record(
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
