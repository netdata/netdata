use super::super::*;
use super::parse_transport;

pub(crate) fn parse_ipv4_packet(
    data: &[u8],
    fields: &mut FlowFields,
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
    let wire_length = total_length as u64;
    let fragment_id = u16::from_be_bytes([data[4], data[5]]);
    let fragment_offset = u16::from_be_bytes([data[6], data[7]]) & 0x1fff;
    let proto = data[9];
    let src = Ipv4Addr::new(data[12], data[13], data[14], data[15]);
    let dst = Ipv4Addr::new(data[16], data[17], data[18], data[19]);

    if decapsulation_mode.is_none() {
        fields.insert("ETYPE", ETYPE_IPV4.to_string());
        fields.insert("SRC_ADDR", src.to_string());
        fields.insert("DST_ADDR", dst.to_string());
        fields.insert("PROTOCOL", proto.to_string());
        fields.insert("IPTOS", data[1].to_string());
        fields.insert("IPTTL", data[8].to_string());
        fields.insert("IP_FRAGMENT_ID", fragment_id.to_string());
        fields.insert("IP_FRAGMENT_OFFSET", fragment_offset.to_string());
    }

    if fragment_offset == 0 {
        let payload_end = captured_length;
        let inner_l3_length =
            parse_transport(proto, &data[ihl..payload_end], fields, decapsulation_mode);
        if decapsulation_mode.is_none() {
            return Some(wire_length);
        }
        return if inner_l3_length > 0 {
            Some(inner_l3_length)
        } else {
            None
        };
    }

    if decapsulation_mode.is_none() {
        Some(wire_length)
    } else {
        None
    }
}

pub(crate) fn parse_ipv6_packet(
    data: &[u8],
    fields: &mut FlowFields,
    decapsulation_mode: DecapsulationMode,
) -> Option<u64> {
    if data.len() < 40 {
        return None;
    }

    let payload_length = u16::from_be_bytes([data[4], data[5]]) as usize;
    let wire_length = 40_u64.saturating_add(payload_length as u64);
    let next_header = data[6];
    let hop_limit = data[7];
    let mut src_bytes = [0_u8; 16];
    let mut dst_bytes = [0_u8; 16];
    src_bytes.copy_from_slice(&data[8..24]);
    dst_bytes.copy_from_slice(&data[24..40]);
    let src = Ipv6Addr::from(src_bytes);
    let dst = Ipv6Addr::from(dst_bytes);

    if decapsulation_mode.is_none() {
        let traffic_class = (u16::from_be_bytes([data[0], data[1]]) & 0x0ff0) >> 4;
        let flow_label = u32::from_be_bytes([data[0], data[1], data[2], data[3]]) & 0x000f_ffff;

        fields.insert("ETYPE", ETYPE_IPV6.to_string());
        fields.insert("SRC_ADDR", src.to_string());
        fields.insert("DST_ADDR", dst.to_string());
        fields.insert("PROTOCOL", next_header.to_string());
        fields.insert("IPTOS", traffic_class.to_string());
        fields.insert("IPTTL", hop_limit.to_string());
        fields.insert("IPV6_FLOW_LABEL", flow_label.to_string());
    }
    let payload_end = data.len().min(40_usize.saturating_add(payload_length));
    let inner_l3_length = parse_transport(
        next_header,
        &data[40..payload_end],
        fields,
        decapsulation_mode,
    );
    if decapsulation_mode.is_none() {
        Some(wire_length)
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
    fn ipv4_transport_parse_respects_total_length() {
        let mut packet = vec![0_u8; 24];
        packet[0] = 0x45;
        packet[2] = 0;
        packet[3] = 20;
        packet[9] = 17;
        packet[12..16].copy_from_slice(&[10, 0, 0, 1]);
        packet[16..20].copy_from_slice(&[10, 0, 0, 2]);
        packet[20..24].copy_from_slice(&[0x12, 0x34, 0x56, 0x78]);

        let mut fields = FlowFields::default();
        let len = parse_ipv4_packet(&packet, &mut fields, DecapsulationMode::None);

        assert_eq!(len, Some(20));
        assert!(!fields.contains_key("SRC_PORT"));
        assert!(!fields.contains_key("DST_PORT"));
    }

    #[test]
    fn ipv4_parse_rejects_total_length_shorter_than_header() {
        let mut packet = vec![0_u8; 24];
        packet[0] = 0x46;
        packet[2] = 0;
        packet[3] = 20;

        let mut fields = FlowFields::default();
        let len = parse_ipv4_packet(&packet, &mut fields, DecapsulationMode::None);

        assert_eq!(len, None);
    }

    #[test]
    fn ipv4_parse_uses_header_declared_length_when_capture_is_truncated() {
        let mut packet = vec![0_u8; 22];
        packet[0] = 0x45;
        packet[2] = 0;
        packet[3] = 40;
        packet[9] = 17;
        packet[12..16].copy_from_slice(&[10, 0, 0, 1]);
        packet[16..20].copy_from_slice(&[10, 0, 0, 2]);
        packet[20..22].copy_from_slice(&[0x12, 0x34]);

        let mut fields = FlowFields::default();
        let len = parse_ipv4_packet(&packet, &mut fields, DecapsulationMode::None);

        assert_eq!(len, Some(40));
        assert!(!fields.contains_key("SRC_PORT"));
        assert!(!fields.contains_key("DST_PORT"));
    }

    #[test]
    fn ipv6_transport_parse_respects_payload_length() {
        let mut packet = vec![0_u8; 44];
        packet[0] = 0x60;
        packet[6] = 17;
        packet[7] = 64;
        packet[8..24]
            .copy_from_slice(&[0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1]);
        packet[24..40]
            .copy_from_slice(&[0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2]);
        packet[40..44].copy_from_slice(&[0x12, 0x34, 0x56, 0x78]);

        let mut fields = FlowFields::default();
        let len = parse_ipv6_packet(&packet, &mut fields, DecapsulationMode::None);

        assert_eq!(len, Some(40));
        assert!(!fields.contains_key("SRC_PORT"));
        assert!(!fields.contains_key("DST_PORT"));
    }

    #[test]
    fn ipv6_parse_uses_header_declared_length_when_capture_is_truncated() {
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

        let mut fields = FlowFields::default();
        let len = parse_ipv6_packet(&packet, &mut fields, DecapsulationMode::None);

        assert_eq!(len, Some(56));
        assert!(!fields.contains_key("SRC_PORT"));
        assert!(!fields.contains_key("DST_PORT"));
    }
}
