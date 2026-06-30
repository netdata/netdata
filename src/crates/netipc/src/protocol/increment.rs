//! INCREMENT codec (method 1) -- 8-byte payload: { u64 value }

use super::NipcError;

pub const INCREMENT_PAYLOAD_SIZE: usize = 8;

pub fn increment_encode(value: u64, buf: &mut [u8]) -> usize {
    if buf.len() < INCREMENT_PAYLOAD_SIZE {
        return 0;
    }
    buf[..8].copy_from_slice(&value.to_ne_bytes());
    INCREMENT_PAYLOAD_SIZE
}

pub fn increment_decode(buf: &[u8]) -> Result<u64, NipcError> {
    if buf.len() < INCREMENT_PAYLOAD_SIZE {
        return Err(NipcError::Truncated);
    }
    Ok(u64::from_ne_bytes(buf[..8].try_into().unwrap()))
}

/// INCREMENT dispatch: decode -> handler -> encode.
pub fn dispatch_increment<F>(req: &[u8], resp: &mut [u8], handler: F) -> Option<usize>
where
    F: FnOnce(u64) -> Option<u64>,
{
    let value = increment_decode(req).ok()?;
    let result = handler(value)?;
    let n = increment_encode(result, resp);
    if n == 0 {
        return None;
    }
    Some(n)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn encode_too_small() {
        let mut buf = [0u8; 4];
        assert_eq!(increment_encode(42, &mut buf), 0);
    }

    #[test]
    fn decode_too_small() {
        let buf = [0u8; 7];
        assert_eq!(increment_decode(&buf), Err(NipcError::Truncated));
    }

    #[test]
    fn roundtrip() {
        let mut buf = [0u8; 8];
        let n = increment_encode(0xDEAD_BEEF, &mut buf);
        assert_eq!(n, 8);
        let val = increment_decode(&buf).unwrap();
        assert_eq!(val, 0xDEAD_BEEF);
    }

    #[test]
    fn dispatch_resp_too_small() {
        let mut req = [0u8; 8];
        increment_encode(42, &mut req);
        let mut resp = [0u8; 4];
        assert!(dispatch_increment(&req, &mut resp, |v| Some(v + 1)).is_none());
    }

    #[test]
    fn dispatch_bad_request() {
        let mut resp = [0u8; 8];
        assert!(dispatch_increment(&[0u8; 4], &mut resp, |v| Some(v)).is_none());
    }

    #[test]
    fn dispatch_handler_returns_none() {
        let mut req = [0u8; 8];
        increment_encode(42, &mut req);
        let mut resp = [0u8; 8];
        assert!(dispatch_increment(&req, &mut resp, |_| None).is_none());
    }

    #[test]
    fn dispatch_success() {
        let mut req = [0u8; 8];
        increment_encode(99, &mut req);
        let mut resp = [0u8; 8];
        let n = dispatch_increment(&req, &mut resp, |v| Some(v + 1)).unwrap();
        assert_eq!(n, 8);
        assert_eq!(increment_decode(&resp).unwrap(), 100);
    }
}
