use super::super::*;
use super::{parse_datalink_frame_section, parse_ipv4_packet, parse_ipv6_packet};
use crate::decoder::{srv6_inner_payload, vxlan_inner_payload};

pub(crate) fn parse_srv6_inner(proto: u8, data: &[u8], fields: &mut FlowFields) -> Option<u64> {
    let (next, cursor) = srv6_inner_payload(proto, data)?;

    match next {
        4 => parse_ipv4_packet(cursor, fields, DecapsulationMode::None),
        41 => parse_ipv6_packet(cursor, fields, DecapsulationMode::None),
        _ => None,
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
