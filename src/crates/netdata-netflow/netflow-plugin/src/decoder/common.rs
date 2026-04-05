use super::*;

pub(crate) fn vxlan_inner_payload(proto: u8, data: &[u8]) -> Option<&[u8]> {
    if proto == 17 && data.len() >= 16 && u16::from_be_bytes([data[2], data[3]]) == VXLAN_UDP_PORT {
        Some(&data[16..])
    } else {
        None
    }
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
