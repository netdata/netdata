//! Non-`q` request-boundary translation: Tempo's unix-SECONDS time
//! params → the engine's nanosecond [`TimeWindow`], and the trace-by-id
//! URL path id → [`sfst::TraceId`]. Both mirror Tempo's own rules
//! (`tempo/pkg/api/http.go`): start/end are optional, `0` means absent,
//! a window needs `end > start`.

use sfsq::traces::TimeWindow;

/// A non-`q` request-parameter defect (HTTP 400).
#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum RequestError {
    #[error("http parameter start must be before end (received start={start} end={end})")]
    WindowOrder { start: i64, end: i64 },
    #[error("time parameter out of range: {value}")]
    TimeOutOfRange { value: i64 },
    #[error("invalid trace id {got:?}: expected 32 hex characters")]
    BadTraceId { got: String },
    #[error("invalid trace id: the all-zero id is not a valid W3C trace id")]
    ZeroTraceId,
}

/// Unix-seconds `start`/`end` (0 = absent, Tempo convention) → an
/// optional engine window. Both absent → `None` (whole retention);
/// otherwise `end > start` is required, matching Tempo's check.
pub fn window_from_unix_seconds(start: i64, end: i64) -> Result<Option<TimeWindow>, RequestError> {
    if start == 0 && end == 0 {
        return Ok(None);
    }
    if end <= start {
        return Err(RequestError::WindowOrder { start, end });
    }
    const NS: i64 = 1_000_000_000;
    let to_ns = |secs: i64| {
        secs.checked_mul(NS)
            .filter(|_| secs >= 0)
            .ok_or(RequestError::TimeOutOfRange { value: secs })
    };
    let window = TimeWindow::new(to_ns(start)?, to_ns(end)?)
        .expect("end > start checked above, ns conversion is monotone");
    Ok(Some(window))
}

/// The trace-by-id URL path segment: exactly 32 hex chars (the W3C
/// 128-bit rendering the plugin round-trips from search results); the
/// all-zero id is rejected up front (the engine treats it as UNSET).
pub fn parse_trace_id_hex(s: &str) -> Result<sfst::TraceId, RequestError> {
    if s.len() != 32 || !s.bytes().all(|b| b.is_ascii_hexdigit()) {
        return Err(RequestError::BadTraceId { got: s.to_string() });
    }
    let mut bytes = [0u8; 16];
    for (i, chunk) in s.as_bytes().chunks_exact(2).enumerate() {
        let hex = std::str::from_utf8(chunk).expect("ascii hex checked above");
        bytes[i] = u8::from_str_radix(hex, 16).expect("ascii hex checked above");
    }
    if bytes == [0u8; 16] {
        return Err(RequestError::ZeroTraceId);
    }
    Ok(sfst::TraceId::from(bytes))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn window_rules() {
        assert_eq!(window_from_unix_seconds(0, 0), Ok(None));
        let w = window_from_unix_seconds(100, 200).unwrap().unwrap();
        assert_eq!(w, TimeWindow::new(100_000_000_000, 200_000_000_000).unwrap());
        // start=0 with a real end is a valid [0, end) window (Tempo's
        // check only rejects end <= start).
        assert!(window_from_unix_seconds(0, 200).unwrap().is_some());
        assert_eq!(
            window_from_unix_seconds(200, 100),
            Err(RequestError::WindowOrder { start: 200, end: 100 })
        );
        assert_eq!(
            window_from_unix_seconds(100, 100),
            Err(RequestError::WindowOrder { start: 100, end: 100 })
        );
        assert!(matches!(
            window_from_unix_seconds(-5, 100),
            Err(RequestError::TimeOutOfRange { .. })
        ));
        assert!(matches!(
            window_from_unix_seconds(1, i64::MAX),
            Err(RequestError::TimeOutOfRange { .. })
        ));
    }

    #[test]
    fn trace_id_rules() {
        let id = parse_trace_id_hex("0102030405060708090a0b0c0d0e0f10").unwrap();
        assert_eq!(
            id,
            sfst::TraceId::from([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16])
        );
        // Case-insensitive hex.
        assert!(parse_trace_id_hex("ABCDEF00000000000000000000000001").is_ok());
        assert!(matches!(
            parse_trace_id_hex("abc"),
            Err(RequestError::BadTraceId { .. })
        ));
        assert!(matches!(
            parse_trace_id_hex("zz02030405060708090a0b0c0d0e0f10"),
            Err(RequestError::BadTraceId { .. })
        ));
        assert_eq!(
            parse_trace_id_hex("00000000000000000000000000000000"),
            Err(RequestError::ZeroTraceId)
        );
    }
}
