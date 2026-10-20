use super::super::super::*;
use super::{
    parse_datalink_frame_section_record, parse_ipv4_packet_record, parse_ipv6_packet_record,
};
use crate::decoder::{srv6_inner_payload, vxlan_inner_payload};

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
                rec.set_src_port(u16::from_be_bytes([data[0], data[1]]));
                rec.set_dst_port(u16::from_be_bytes([data[2], data[3]]));
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
pub(crate) fn parse_srv6_inner_record(proto: u8, data: &[u8], rec: &mut FlowRecord) -> Option<u64> {
    let (next, cursor) = srv6_inner_payload(proto, data)?;

    match next {
        4 => parse_ipv4_packet_record(cursor, rec, DecapsulationMode::None),
        41 => parse_ipv6_packet_record(cursor, rec, DecapsulationMode::None),
        _ => None,
    }
}
