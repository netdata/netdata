use super::super::*;
use super::{parse_datalink_frame_section, parse_ipv4_packet, parse_ipv6_packet};
use crate::decoder::vxlan_inner_payload;

pub(crate) fn parse_srv6_inner(proto: u8, data: &[u8], fields: &mut FlowFields) -> Option<u64> {
    let mut next = proto;
    let mut cursor = data;

    loop {
        match next {
            4 => return parse_ipv4_packet(cursor, fields, DecapsulationMode::None),
            41 => return parse_ipv6_packet(cursor, fields, DecapsulationMode::None),
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

pub(crate) fn parse_transport(
    proto: u8,
    data: &[u8],
    fields: &mut FlowFields,
    decapsulation_mode: DecapsulationMode,
) -> u64 {
    if !decapsulation_mode.is_none() {
        return match decapsulation_mode {
            DecapsulationMode::Vxlan => {
                if let Some(inner) = vxlan_inner_payload(proto, data) {
                    parse_datalink_frame_section(inner, fields, DecapsulationMode::None)
                        .unwrap_or(0)
                } else {
                    0
                }
            }
            DecapsulationMode::Srv6 => parse_srv6_inner(proto, data, fields).unwrap_or(0),
            DecapsulationMode::None => 0,
        };
    }

    match proto {
        6 | 17 => {
            if data.len() >= 4 {
                fields.insert(
                    "SRC_PORT",
                    u16::from_be_bytes([data[0], data[1]]).to_string(),
                );
                fields.insert(
                    "DST_PORT",
                    u16::from_be_bytes([data[2], data[3]]).to_string(),
                );
            }
            if proto == 6 && data.len() >= 14 {
                fields.insert("TCP_FLAGS", data[13].to_string());
            }
        }
        1 => {
            if data.len() >= 2 {
                fields.insert("ICMPV4_TYPE", data[0].to_string());
                fields.insert("ICMPV4_CODE", data[1].to_string());
            }
        }
        58 => {
            if data.len() >= 2 {
                fields.insert("ICMPV6_TYPE", data[0].to_string());
                fields.insert("ICMPV6_CODE", data[1].to_string());
            }
        }
        _ => {}
    }

    0
}
