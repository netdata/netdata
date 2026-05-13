// SPDX-License-Identifier: GPL-3.0-or-later

//! Phase 0 stub for the Netdata PromQL evaluator.
//!
//! Six entry points form the C ABI: two query functions and four accessors
//! around an opaque response handle. The query functions parse their input
//! with `promql-parser` and, on success, return a placeholder constant value
//! in a Prometheus HTTP API response shape; on parse failure they return a
//! structured error. No storage access happens in Phase 0; that arrives with
//! the data-source shim in Phase 1.
//!
//! The signatures here are intended to be stable across phases. Only the
//! bodies change as real evaluation logic lands. See SOW-0016 for context.

use std::ffi::{c_char, c_int, CStr, CString};
use std::ptr;

const PHASE_0_CONSTANT: &str = "42";

// Prometheus' default cap on the number of points per timeseries in a range
// query. Mirrored here so a misbehaving caller can't cause us to format a
// multi-gigabyte JSON response during the Phase 0 smoke test.
const MAX_POINTS_PER_TIMESERIES: usize = 11_000;

/// Opaque handle for a query response.
///
/// The C side obtains a `*mut NdPromqlResponse` from one of the query
/// functions, reads through the accessors, and must release it with
/// `nd_promql_response_free`. Body bytes remain valid until release.
pub struct NdPromqlResponse {
    body: CString,
    http_status: c_int,
}

impl NdPromqlResponse {
    fn ok_scalar(ts_ms: i64, value: &str) -> Self {
        let ts_secs = ts_ms as f64 / 1000.0;
        let body = format!(
            r#"{{"status":"success","data":{{"resultType":"scalar","result":[{ts_secs},"{value}"]}}}}"#
        );
        Self::new_ok(body)
    }

    fn ok_matrix_single(start_ms: i64, end_ms: i64, step_ms: i64, value: &str) -> Self {
        let mut t = start_ms;
        let mut values: Vec<String> = Vec::new();
        while t <= end_ms {
            let ts_secs = t as f64 / 1000.0;
            values.push(format!(r#"[{ts_secs},"{value}"]"#));
            t = match t.checked_add(step_ms) {
                Some(n) => n,
                None => break,
            };
        }
        let body = format!(
            r#"{{"status":"success","data":{{"resultType":"matrix","result":[{{"metric":{{}},"values":[{}]}}]}}}}"#,
            values.join(",")
        );
        Self::new_ok(body)
    }

    fn bad_data(msg: &str) -> Self {
        let escaped = json_escape(msg);
        let body = format!(
            r#"{{"status":"error","errorType":"bad_data","error":"{escaped}"}}"#
        );
        Self {
            body: CString::new(body).expect("response JSON must contain no NUL"),
            http_status: 400,
        }
    }

    fn new_ok(body: String) -> Self {
        Self {
            body: CString::new(body).expect("response JSON must contain no NUL"),
            http_status: 200,
        }
    }
}

fn json_escape(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out
}

/// Borrow a NUL-terminated C string as `&str`. Returns `None` on null pointer
/// or non-UTF-8 input.
///
/// # Safety
/// The caller must ensure `p` is either null or a valid NUL-terminated C
/// string valid for the duration of the call.
unsafe fn cstr_borrow<'a>(p: *const c_char) -> Option<&'a str> {
    if p.is_null() {
        return None;
    }
    unsafe { CStr::from_ptr(p) }.to_str().ok()
}

/// Evaluate an instant PromQL query.
///
/// On parse success, returns a Prometheus-shape success response carrying a
/// placeholder scalar value at the requested instant. On parse failure,
/// returns a structured error.
///
/// `host_machine_guid` is reserved for Phase 1 and is currently ignored.
/// `timeout_ms` is reserved for Phase 1 and is currently ignored.
///
/// # Safety
/// All pointer parameters must be either null or valid NUL-terminated C
/// strings valid for the duration of the call. The returned handle must be
/// released exactly once with `nd_promql_response_free`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_query_instant(
    _host_machine_guid: *const c_char,
    query: *const c_char,
    at_ms: i64,
    _timeout_ms: i64,
) -> *mut NdPromqlResponse {
    let q = match unsafe { cstr_borrow(query) } {
        Some(q) => q,
        None => {
            return Box::into_raw(Box::new(NdPromqlResponse::bad_data(
                "missing or invalid query parameter",
            )));
        }
    };
    let resp = match promql_parser::parser::parse(q) {
        Ok(_expr) => NdPromqlResponse::ok_scalar(at_ms, PHASE_0_CONSTANT),
        Err(msg) => NdPromqlResponse::bad_data(&msg),
    };
    Box::into_raw(Box::new(resp))
}

/// Evaluate a range PromQL query.
///
/// On parse success, returns a Prometheus-shape success response carrying a
/// single dummy series with the placeholder value at every step grid point.
/// On parse failure or invalid time range, returns a structured error.
///
/// # Safety
/// Same requirements as `nd_promql_query_instant`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_query_range(
    _host_machine_guid: *const c_char,
    query: *const c_char,
    start_ms: i64,
    end_ms: i64,
    step_ms: i64,
    _timeout_ms: i64,
) -> *mut NdPromqlResponse {
    let q = match unsafe { cstr_borrow(query) } {
        Some(q) => q,
        None => {
            return Box::into_raw(Box::new(NdPromqlResponse::bad_data(
                "missing or invalid query parameter",
            )));
        }
    };
    if step_ms <= 0 || end_ms < start_ms {
        return Box::into_raw(Box::new(NdPromqlResponse::bad_data(
            "invalid time range or non-positive step",
        )));
    }

    let span = end_ms.saturating_sub(start_ms);
    let n_points = (span / step_ms) as usize + 1;
    if n_points > MAX_POINTS_PER_TIMESERIES {
        return Box::into_raw(Box::new(NdPromqlResponse::bad_data(
            "exceeded maximum resolution of 11000 points per timeseries",
        )));
    }

    let resp = match promql_parser::parser::parse(q) {
        Ok(_expr) => NdPromqlResponse::ok_matrix_single(start_ms, end_ms, step_ms, PHASE_0_CONSTANT),
        Err(msg) => NdPromqlResponse::bad_data(&msg),
    };
    Box::into_raw(Box::new(resp))
}

/// Pointer to the NUL-terminated response body.
///
/// Returns null if `r` is null. The pointed-to bytes remain valid until
/// `nd_promql_response_free` is called on `r`.
///
/// # Safety
/// `r` must be either null or a valid handle previously returned by one of
/// the query functions and not yet freed.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_response_body(r: *const NdPromqlResponse) -> *const c_char {
    if r.is_null() {
        return ptr::null();
    }
    unsafe { (*r).body.as_ptr() }
}

/// Byte length of the response body, excluding the trailing NUL.
///
/// # Safety
/// Same as `nd_promql_response_body`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_response_body_len(r: *const NdPromqlResponse) -> usize {
    if r.is_null() {
        return 0;
    }
    unsafe { (*r).body.as_bytes().len() }
}

/// HTTP status code that should accompany the response body. 200 on success,
/// 400 on parse error. Returns 500 if `r` is null.
///
/// # Safety
/// Same as `nd_promql_response_body`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_response_http_status(r: *const NdPromqlResponse) -> c_int {
    if r.is_null() {
        return 500;
    }
    unsafe { (*r).http_status }
}

/// Release a response handle. Safe to call on null.
///
/// # Safety
/// `r` must be either null or a handle returned by one of the query
/// functions, and must not be used after this call returns.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_response_free(r: *mut NdPromqlResponse) {
    if r.is_null() {
        return;
    }
    drop(unsafe { Box::from_raw(r) });
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::ffi::CString;

    fn body_of(r: *mut NdPromqlResponse) -> String {
        unsafe {
            let s = CStr::from_ptr(nd_promql_response_body(r))
                .to_str()
                .unwrap()
                .to_string();
            nd_promql_response_free(r);
            s
        }
    }

    #[test]
    fn instant_success_returns_scalar_shape() {
        let q = CString::new("up").unwrap();
        let r = unsafe {
            nd_promql_query_instant(ptr::null(), q.as_ptr(), 1_715_594_400_000, 0)
        };
        assert!(!r.is_null());
        assert_eq!(unsafe { nd_promql_response_http_status(r) }, 200);
        let body = body_of(r);
        assert!(body.contains(r#""resultType":"scalar""#), "body = {body}");
        assert!(body.contains(r#""42""#), "body = {body}");
    }

    #[test]
    fn instant_parse_failure_returns_bad_data() {
        let q = CString::new("not a valid query {").unwrap();
        let r = unsafe { nd_promql_query_instant(ptr::null(), q.as_ptr(), 0, 0) };
        assert!(!r.is_null());
        assert_eq!(unsafe { nd_promql_response_http_status(r) }, 400);
        let body = body_of(r);
        assert!(body.contains(r#""status":"error""#), "body = {body}");
        assert!(body.contains(r#""errorType":"bad_data""#), "body = {body}");
    }

    #[test]
    fn range_success_returns_matrix_shape() {
        let q = CString::new("up").unwrap();
        let r = unsafe {
            nd_promql_query_range(
                ptr::null(),
                q.as_ptr(),
                1_715_594_400_000,
                1_715_594_460_000,
                15_000,
                0,
            )
        };
        assert_eq!(unsafe { nd_promql_response_http_status(r) }, 200);
        let body = body_of(r);
        assert!(body.contains(r#""resultType":"matrix""#), "body = {body}");
        assert!(body.contains(r#""values":["#), "body = {body}");
    }

    #[test]
    fn range_rejects_oversized_resolution() {
        let q = CString::new("up").unwrap();
        // 1 ms step over a 1-hour window -> 3.6M points; capped.
        let r = unsafe {
            nd_promql_query_range(ptr::null(), q.as_ptr(), 0, 3_600_000, 1, 0)
        };
        assert_eq!(unsafe { nd_promql_response_http_status(r) }, 400);
        let body = body_of(r);
        assert!(body.contains("11000 points"), "body = {body}");
    }

    #[test]
    fn null_query_yields_bad_data() {
        let r = unsafe { nd_promql_query_instant(ptr::null(), ptr::null(), 0, 0) };
        assert_eq!(unsafe { nd_promql_response_http_status(r) }, 400);
        body_of(r);
    }

    #[test]
    fn json_escape_handles_quotes_and_controls() {
        assert_eq!(json_escape(r#"a"b"#), r#"a\"b"#);
        assert_eq!(json_escape("a\tb"), "a\\tb");
        assert_eq!(json_escape("a\u{01}b"), "a\\u0001b");
    }
}
