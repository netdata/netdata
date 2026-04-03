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

    let total_length = u16::from_be_bytes([data[2], data[3]]) as usize;
    if total_length < ihl {
        return None;
    }
    let captured_length = total_length.min(data.len());
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
        let inner_l3_length =
            parse_transport_record(proto, &data[ihl..captured_length], rec, decapsulation_mode);
        if decapsulation_mode.is_none() {
            return Some(captured_length as u64);
        }
        return if inner_l3_length > 0 {
            Some(inner_l3_length)
        } else {
            None
        };
    }

    if decapsulation_mode.is_none() {
        Some(captured_length as u64)
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

    let payload_length = u16::from_be_bytes([data[4], data[5]]) as usize;
    let captured_length = data.len().min(40_usize.saturating_add(payload_length));
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
    let inner_l3_length = parse_transport_record(
        next_header,
        &data[40..captured_length],
        rec,
        decapsulation_mode,
    );
    if decapsulation_mode.is_none() {
        Some(captured_length as u64)
    } else if inner_l3_length > 0 {
        Some(inner_l3_length)
    } else {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ipv4_record_transport_parse_respects_total_length() {
        let mut packet = vec![0_u8; 24];
        packet[0] = 0x45;
        packet[2] = 0;
        packet[3] = 20;
        packet[9] = 17;
        packet[12..16].copy_from_slice(&[10, 0, 0, 1]);
        packet[16..20].copy_from_slice(&[10, 0, 0, 2]);
        packet[20..24].copy_from_slice(&[0x12, 0x34, 0x56, 0x78]);

        let mut rec = FlowRecord::default();
        let len = parse_ipv4_packet_record(&packet, &mut rec, DecapsulationMode::None);

        assert_eq!(len, Some(20));
        assert_eq!(rec.src_port, 0);
        assert_eq!(rec.dst_port, 0);
    }

    #[test]
    fn ipv4_record_rejects_total_length_shorter_than_header() {
        let mut packet = vec![0_u8; 24];
        packet[0] = 0x46;
        packet[2] = 0;
        packet[3] = 20;

        let mut rec = FlowRecord::default();
        let len = parse_ipv4_packet_record(&packet, &mut rec, DecapsulationMode::None);

        assert_eq!(len, None);
    }

    #[test]
    fn ipv4_record_clamps_accounted_length_to_captured_bytes() {
        let mut packet = vec![0_u8; 22];
        packet[0] = 0x45;
        packet[2] = 0;
        packet[3] = 40;
        packet[9] = 17;
        packet[12..16].copy_from_slice(&[10, 0, 0, 1]);
        packet[16..20].copy_from_slice(&[10, 0, 0, 2]);
        packet[20..22].copy_from_slice(&[0x12, 0x34]);

        let mut rec = FlowRecord::default();
        let len = parse_ipv4_packet_record(&packet, &mut rec, DecapsulationMode::None);

        assert_eq!(len, Some(packet.len() as u64));
        assert_eq!(rec.src_port, 0);
        assert_eq!(rec.dst_port, 0);
    }

    #[test]
    fn ipv6_record_transport_parse_respects_payload_length() {
        let mut packet = vec![0_u8; 44];
        packet[0] = 0x60;
        packet[6] = 17;
        packet[7] = 64;
        packet[8..24]
            .copy_from_slice(&[0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]);
        packet[24..40]
            .copy_from_slice(&[0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2]);
        packet[40..44].copy_from_slice(&[0x12, 0x34, 0x56, 0x78]);

        let mut rec = FlowRecord::default();
        let len = parse_ipv6_packet_record(&packet, &mut rec, DecapsulationMode::None);

        assert_eq!(len, Some(40));
        assert_eq!(rec.src_port, 0);
        assert_eq!(rec.dst_port, 0);
    }

    #[test]
    fn ipv6_record_clamps_accounted_length_to_captured_bytes() {
        let mut packet = vec![0_u8; 42];
        packet[0] = 0x60;
        packet[4] = 0;
        packet[5] = 16;
        packet[6] = 17;
        packet[7] = 64;
        packet[8..24]
            .copy_from_slice(&[0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]);
        packet[24..40]
            .copy_from_slice(&[0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2]);
        packet[40..42].copy_from_slice(&[0x12, 0x34]);

        let mut rec = FlowRecord::default();
        let len = parse_ipv6_packet_record(&packet, &mut rec, DecapsulationMode::None);

        assert_eq!(len, Some(packet.len() as u64));
        assert_eq!(rec.src_port, 0);
        assert_eq!(rec.dst_port, 0);
    }
}
