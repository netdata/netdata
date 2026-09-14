use super::*;

pub(crate) fn vxlan_inner_payload(proto: u8, data: &[u8]) -> Option<&[u8]> {
    let valid_flags = data.get(8).copied() == Some(0x08)
        && data.get(9).copied() == Some(0)
        && data.get(10).copied() == Some(0)
        && data.get(11).copied() == Some(0)
        && data.get(15).copied() == Some(0);
    if proto == 17
        && data.len() >= 16
        && u16::from_be_bytes([data[2], data[3]]) == VXLAN_UDP_PORT
        && valid_flags
    {
        Some(&data[16..])
    } else {
        None
    }
}

fn srv6_extension_header_len(next: u8, data: &[u8]) -> Option<usize> {
    match next {
        0 | 43 | 60 | 135 | 139 | 140 => {
            let extra = usize::from(*data.get(1)?);
            Some(8_usize.saturating_add(extra.saturating_mul(8)))
        }
        44 => Some(8),
        51 => {
            let extra = usize::from(*data.get(1)?);
            Some(extra.saturating_add(2).saturating_mul(4))
        }
        _ => None,
    }
}

pub(crate) fn srv6_inner_payload(proto: u8, data: &[u8]) -> Option<(u8, &[u8])> {
    let mut next = proto;
    let mut cursor = data;

    loop {
        match next {
            4 | 41 => return Some((next, cursor)),
            43 => {
                if cursor.len() < 8 || cursor[2] != 4 {
                    return None;
                }
                let skip = srv6_extension_header_len(next, cursor)?;
                if cursor.len() < skip {
                    return None;
                }
                next = cursor[0];
                cursor = &cursor[skip..];
            }
            _ => {
                let skip = srv6_extension_header_len(next, cursor)?;
                if cursor.len() < skip {
                    return None;
                }
                next = cursor[0];
                cursor = &cursor[skip..];
            }
        }
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

    #[test]
    fn vxlan_inner_payload_rejects_invalid_flags() {
        let packet = [
            0x12, 0x34, 0x12, 0xb5, 0, 0, 0, 0, 0x00, 0, 0, 0, 0, 0, 0, 0,
        ];

        assert_eq!(vxlan_inner_payload(17, &packet), None);
    }

    #[test]
    fn srv6_inner_payload_skips_extension_headers_before_srh() {
        let mut packet = Vec::new();
        packet.extend_from_slice(&[43, 0, 0, 0, 0, 0, 0, 0]);
        packet.extend_from_slice(&[4, 0, 4, 0, 0, 0, 0, 0]);
        packet.extend_from_slice(&[
            0x45, 0, 0, 20, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2,
        ]);

        let inner = srv6_inner_payload(60, &packet).expect("inner payload");

        assert_eq!(inner.0, 4);
        assert_eq!(inner.1[0], 0x45);
    }
}
