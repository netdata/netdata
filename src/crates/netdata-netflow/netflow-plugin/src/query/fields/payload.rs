use memchr::memchr;
use std::borrow::Cow;

pub(crate) fn split_payload_bytes(payload: &[u8]) -> Option<(&[u8], &[u8])> {
    let eq_pos = memchr(b'=', payload)?;
    Some((&payload[..eq_pos], &payload[eq_pos + 1..]))
}

pub(crate) fn parse_u64_ascii(bytes: &[u8]) -> Option<u64> {
    std::str::from_utf8(bytes).ok()?.parse::<u64>().ok()
}

pub(crate) fn payload_value(value_bytes: &[u8]) -> Cow<'_, str> {
    match std::str::from_utf8(value_bytes) {
        Ok(value) => Cow::Borrowed(value),
        Err(_) => String::from_utf8_lossy(value_bytes),
    }
}
