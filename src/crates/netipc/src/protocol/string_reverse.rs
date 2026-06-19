//! STRING_REVERSE codec (method 3) -- variable-length payload:
//!   [0:4] u32 str_offset (from payload start, always 8)
//!   [4:8] u32 str_length (excluding NUL)
//!   [8:N+1] string data + NUL

use super::NipcError;

pub const STRING_REVERSE_HDR_SIZE: usize = 8;

/// Ephemeral view into a decoded STRING_REVERSE payload.
#[derive(Debug, Clone)]
pub struct StringReverseView<'a> {
    pub str_data: &'a [u8], // slice into payload, excludes NUL
    pub str_len: u32,
}

impl<'a> StringReverseView<'a> {
    pub fn as_str(&self) -> &'a str {
        std::str::from_utf8(self.str_data).unwrap_or("")
    }
}

pub fn string_reverse_encode(s: &[u8], buf: &mut [u8]) -> usize {
    if s.len() > u32::MAX as usize {
        return 0;
    }
    let total = STRING_REVERSE_HDR_SIZE + s.len() + 1;
    if buf.len() < total {
        return 0;
    }
    let offset: u32 = STRING_REVERSE_HDR_SIZE as u32;
    let length: u32 = s.len() as u32;
    buf[0..4].copy_from_slice(&offset.to_ne_bytes());
    buf[4..8].copy_from_slice(&length.to_ne_bytes());
    if !s.is_empty() {
        buf[8..8 + s.len()].copy_from_slice(s);
    }
    buf[8 + s.len()] = 0; // NUL
    total
}

pub fn string_reverse_decode(buf: &[u8]) -> Result<StringReverseView<'_>, NipcError> {
    if buf.len() < STRING_REVERSE_HDR_SIZE {
        return Err(NipcError::Truncated);
    }
    let str_offset = u32::from_ne_bytes(buf[0..4].try_into().unwrap()) as usize;
    if str_offset < STRING_REVERSE_HDR_SIZE {
        return Err(NipcError::BadLayout);
    }
    let str_length = u32::from_ne_bytes(buf[4..8].try_into().unwrap()) as usize;
    let end = str_offset
        .checked_add(str_length)
        .and_then(|v| v.checked_add(1))
        .ok_or(NipcError::OutOfBounds)?;
    if end > buf.len() {
        return Err(NipcError::OutOfBounds);
    }
    if buf[str_offset + str_length] != 0 {
        return Err(NipcError::MissingNul);
    }
    Ok(StringReverseView {
        str_data: &buf[str_offset..str_offset + str_length],
        str_len: str_length as u32,
    })
}

/// STRING_REVERSE dispatch: decode -> handler -> encode.
pub fn dispatch_string_reverse<F>(req: &[u8], resp: &mut [u8], handler: F) -> Option<usize>
where
    F: FnOnce(&[u8]) -> Option<Vec<u8>>,
{
    let view = string_reverse_decode(req).ok()?;
    let result = handler(view.str_data)?;
    let n = string_reverse_encode(&result, resp);
    if n == 0 {
        return None;
    }
    Some(n)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn encode_decode_roundtrip() {
        let s = b"hello world";
        let mut buf = [0u8; 64];
        let n = string_reverse_encode(s, &mut buf);
        assert_eq!(n, STRING_REVERSE_HDR_SIZE + s.len() + 1);

        let view = string_reverse_decode(&buf[..n]).unwrap();
        assert_eq!(view.str_data, s);
        assert_eq!(view.str_len, s.len() as u32);
        assert_eq!(view.as_str(), "hello world");
    }

    #[test]
    fn encode_empty() {
        let mut buf = [0u8; 64];
        let n = string_reverse_encode(b"", &mut buf);
        assert_eq!(n, STRING_REVERSE_HDR_SIZE + 1);
        let view = string_reverse_decode(&buf[..n]).unwrap();
        assert_eq!(view.str_data, b"");
        assert_eq!(view.str_len, 0);
    }

    #[test]
    fn encode_too_small() {
        let mut buf = [0u8; 4];
        assert_eq!(string_reverse_encode(b"hello", &mut buf), 0);
    }

    #[test]
    fn decode_truncated() {
        assert!(matches!(
            string_reverse_decode(&[0u8; 4]),
            Err(NipcError::Truncated)
        ));
    }

    #[test]
    fn decode_oob() {
        let mut buf = [0u8; 16];
        buf[0..4].copy_from_slice(&8u32.to_ne_bytes());
        buf[4..8].copy_from_slice(&99u32.to_ne_bytes());
        assert!(matches!(
            string_reverse_decode(&buf),
            Err(NipcError::OutOfBounds)
        ));
    }

    #[test]
    fn decode_missing_nul() {
        let mut buf = [0u8; 16];
        buf[0..4].copy_from_slice(&8u32.to_ne_bytes());
        buf[4..8].copy_from_slice(&3u32.to_ne_bytes());
        buf[8] = b'a';
        buf[9] = b'b';
        buf[10] = b'c';
        buf[11] = b'X';
        assert!(matches!(
            string_reverse_decode(&buf),
            Err(NipcError::MissingNul)
        ));
    }

    #[test]
    fn dispatch_resp_too_small() {
        let s = b"hello";
        let mut req_buf = [0u8; 64];
        string_reverse_encode(s, &mut req_buf);
        let mut resp = [0u8; 4];
        let result = dispatch_string_reverse(&req_buf, &mut resp, |data| {
            Some(data.iter().rev().copied().collect())
        });
        assert!(result.is_none());
    }

    #[test]
    fn dispatch_handler_returns_none() {
        let mut req_buf = [0u8; 64];
        string_reverse_encode(b"test", &mut req_buf);
        let mut resp = [0u8; 64];
        assert!(dispatch_string_reverse(&req_buf, &mut resp, |_| None).is_none());
    }

    #[test]
    fn dispatch_bad_request() {
        let mut resp = [0u8; 64];
        assert!(dispatch_string_reverse(&[0u8; 4], &mut resp, |_| None).is_none());
    }

    #[test]
    fn dispatch_success() {
        let mut req_buf = [0u8; 64];
        let n = string_reverse_encode(b"abc", &mut req_buf);
        let mut resp = [0u8; 64];
        let rn = dispatch_string_reverse(&req_buf[..n], &mut resp, |data| {
            Some(data.iter().rev().copied().collect())
        })
        .unwrap();
        let view = string_reverse_decode(&resp[..rn]).unwrap();
        assert_eq!(view.str_data, b"cba");
    }

    #[test]
    fn as_str_non_utf8() {
        let view = StringReverseView {
            str_data: &[0xFF, 0xFE],
            str_len: 2,
        };
        assert_eq!(view.as_str(), "");
    }
}
