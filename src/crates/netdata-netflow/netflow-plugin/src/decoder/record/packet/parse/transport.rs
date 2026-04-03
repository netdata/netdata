use super::super::super::*;
use super::{
    parse_datalink_frame_section_record, parse_ipv4_packet_record, parse_ipv6_packet_record,
};

fn vxlan_inner_payload(proto: u8, data: &[u8]) -> Option<&[u8]> {
    if proto == 17 && data.len() >= 16 && u16::from_be_bytes([data[2], data[3]]) == VXLAN_UDP_PORT {
        Some(&data[16..])
    } else {
        None
    }
}

pub(crate) fn parse_transport_record(
    proto: u8,
    data: &[u8],
    rec: &mut FlowRecord,
    decapsulation_mode: DecapsulationMode,
) -> u64 {
    match decapsulation_mode {
        DecapsulationMode::None => {}
        DecapsulationMode::Vxlan => {
            return if let Some(inner) = vxlan_inner_payload(proto, data) {
                parse_datalink_frame_section_record(inner, rec, DecapsulationMode::None)
                    .unwrap_or(0)
            } else {
                0
            };
        }
        DecapsulationMode::Srv6 => {
            return parse_srv6_inner_record(proto, data, rec).unwrap_or(0);
        }
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn vxlan_inner_payload_accepts_minimum_header_length() {
        let packet = [
            0x12, 0x34, 0x12, 0xb5, 0, 0, 0, 0, 0x08, 0, 0, 0, 0, 0, 0, 0,
        ];

        let inner = vxlan_inner_payload(17, &packet);

        assert_eq!(inner, Some(&[][..]));
    }
}

pub(crate) fn parse_srv6_inner_record(proto: u8, data: &[u8], rec: &mut FlowRecord) -> Option<u64> {
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
