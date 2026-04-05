pub(crate) fn decode_akvorado_unsigned(bytes: &[u8]) -> u64 {
    match bytes.len() {
        1 => u64::from(bytes[0]),
        2 => u64::from(bytes[1]) | (u64::from(bytes[0]) << 8),
        3 => u64::from(bytes[2]) | (u64::from(bytes[1]) << 8) | (u64::from(bytes[0]) << 16),
        4 => {
            u64::from(bytes[3])
                | (u64::from(bytes[2]) << 8)
                | (u64::from(bytes[1]) << 16)
                | (u64::from(bytes[0]) << 24)
        }
        5 => {
            u64::from(bytes[4])
                | (u64::from(bytes[3]) << 8)
                | (u64::from(bytes[2]) << 16)
                | (u64::from(bytes[1]) << 24)
                | (u64::from(bytes[0]) << 32)
        }
        6 => {
            u64::from(bytes[5])
                | (u64::from(bytes[4]) << 8)
                | (u64::from(bytes[3]) << 16)
                | (u64::from(bytes[2]) << 24)
                | (u64::from(bytes[1]) << 32)
                | (u64::from(bytes[0]) << 40)
        }
        7 => {
            u64::from(bytes[6])
                | (u64::from(bytes[5]) << 8)
                | (u64::from(bytes[4]) << 16)
                | (u64::from(bytes[3]) << 24)
                | (u64::from(bytes[2]) << 32)
                | (u64::from(bytes[1]) << 40)
                | (u64::from(bytes[0]) << 48)
        }
        8 => u64::from_be_bytes([
            bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5], bytes[6], bytes[7],
        ]),
        _ => 0,
    }
}

pub(crate) fn is_template_error(message: &str) -> bool {
    let msg = message.to_ascii_lowercase();
    msg.contains("template") && msg.contains("not found")
}
