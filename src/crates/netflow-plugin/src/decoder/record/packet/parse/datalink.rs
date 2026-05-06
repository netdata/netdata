use super::super::super::*;
use super::{parse_ipv4_packet_record, parse_ipv6_packet_record};

pub(crate) fn parse_datalink_frame_section_record(
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

    while is_vlan_ethertype(etype) {
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
            if bottom == 1 {
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn qinq_datalink_record_keeps_inner_ip_payload() {
        let mut frame = vec![0_u8; 14 + 4 + 4 + 20];
        frame[12] = 0x88;
        frame[13] = 0xa8;
        frame[16] = 0x81;
        frame[17] = 0x00;
        frame[20] = 0x08;
        frame[21] = 0x00;
        frame[22] = 0x45;
        frame[24] = 0;
        frame[25] = 20;
        frame[30] = 64;
        frame[31] = 17;
        frame[34..38].copy_from_slice(&[10, 0, 0, 1]);
        frame[38..42].copy_from_slice(&[10, 0, 0, 2]);

        let mut rec = FlowRecord::default();
        let parsed = parse_datalink_frame_section_record(&frame, &mut rec, DecapsulationMode::None);

        assert_eq!(parsed, Some(20));
        assert_eq!(rec.src_addr, Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1))));
        assert_eq!(rec.dst_addr, Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2))));
    }
}
