use super::super::*;
use super::{parse_ipv4_packet, parse_ipv6_packet};

pub(crate) fn parse_datalink_frame_section(
    data: &[u8],
    fields: &mut FlowFields,
    decapsulation_mode: DecapsulationMode,
) -> Option<u64> {
    if data.len() < 14 {
        return None;
    }

    fields.insert("DST_MAC", mac_to_string(&data[0..6]));
    fields.insert("SRC_MAC", mac_to_string(&data[6..12]));

    let mut etype = u16::from_be_bytes([data[12], data[13]]);
    let mut cursor = &data[14..];

    while etype == ETYPE_VLAN {
        if cursor.len() < 4 {
            return None;
        }
        let vlan = ((u16::from(cursor[0] & 0x0f)) << 8) | u16::from(cursor[1]);
        if vlan > 0 && !field_present_in_map(fields, "SRC_VLAN") {
            fields.insert("SRC_VLAN", vlan.to_string());
        }
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
            fields.insert("MPLS_LABELS", labels.join(","));
        }
    }

    match etype {
        0x0800 => parse_ipv4_packet(cursor, fields, decapsulation_mode),
        0x86dd => parse_ipv6_packet(cursor, fields, decapsulation_mode),
        _ => None,
    }
}

pub(crate) fn mac_to_string(bytes: &[u8]) -> String {
    if bytes.len() != 6 {
        return String::new();
    }
    format!(
        "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
        bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5]
    )
}
