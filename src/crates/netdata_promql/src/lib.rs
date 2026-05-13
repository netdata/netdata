// SPDX-License-Identifier: GPL-3.0-or-later

//! Netdata PromQL evaluator.
//!
//! Six entry points form the C ABI: two query functions and four accessors
//! around an opaque response handle. Phase 1 (SOW-0017) replaces the
//! Phase 0 placeholder bodies with a real pipeline: parse via
//! `promql-parser`, lower into a typed Plan IR, evaluate against the C
//! storage shim through the `storage` module, and serialize to the
//! Prometheus HTTP API response shape.
//!
//! Chunk 3 covers number literals, vector and matrix selectors, scalar
//! arithmetic, comparisons (with and without `bool`), the five basic
//! aggregations (sum/avg/min/max/count) with `by`/`without`, unary minus,
//! and `offset`. Counter functions (`rate`/`irate`/`increase`/`delta`) and
//! `histogram_quantile` arrive in chunk 4.

mod eval;
mod output;
mod plan;
mod storage;

use std::ffi::{c_char, c_int, CStr, CString};
use std::ptr;

use eval::{eval, EvalContext, EvalError, EvalResult, Sample, Series};
use output::{serialize_error, serialize_scalar_at, serialize_success};
use plan::{lower_query, LowerError, ValueType};

/// Prometheus' default cap on the number of points per timeseries in a
/// range query. Guards against pathological step values.
const MAX_POINTS_PER_TIMESERIES: usize = 11_000;

/// Default lookback window for instant vector selectors.
const DEFAULT_LOOKBACK_MS: i64 = 5 * 60 * 1000;

/// Default `max_series` cardinality backstop.
const DEFAULT_MAX_SERIES: usize = 10_000;

/// Opaque handle for a query response.
pub struct NdPromqlResponse {
    body: CString,
    http_status: c_int,
}

impl NdPromqlResponse {
    fn ok(body: String) -> Self {
        Self {
            body: CString::new(body).unwrap_or_else(|_| {
                CString::new(r#"{"status":"error","errorType":"internal","error":"response contained NUL byte"}"#).unwrap()
            }),
            http_status: 200,
        }
    }

    fn bad_data(msg: &str) -> Self {
        Self::error(400, "bad_data", msg)
    }

    fn execution(msg: &str) -> Self {
        Self::error(422, "execution", msg)
    }

    #[allow(dead_code)]
    fn internal(msg: &str) -> Self {
        Self::error(500, "internal", msg)
    }

    fn error(status: c_int, error_type: &str, msg: &str) -> Self {
        let body = serialize_error(error_type, msg);
        Self {
            body: CString::new(body).unwrap_or_else(|_| CString::new("internal").unwrap()),
            http_status: status,
        }
    }
}

/// Borrow a NUL-terminated C string as `&str`. Returns `None` on null
/// pointer or non-UTF-8 input.
unsafe fn cstr_borrow<'a>(p: *const c_char) -> Option<&'a str> {
    if p.is_null() {
        return None;
    }
    unsafe { CStr::from_ptr(p) }.to_str().ok()
}

/// Evaluate an instant PromQL query at `at_ms`.
///
/// # Safety
/// All pointer parameters must be either null or valid NUL-terminated C
/// strings valid for the duration of the call. The returned handle must
/// be released exactly once with `nd_promql_response_free`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_query_instant(
    host_machine_guid: *const c_char,
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
    let host = unsafe { cstr_borrow(host_machine_guid) }.map(|s| s.to_string());
    let resp = match run_instant(q, at_ms, host) {
        Ok(r) => r,
        Err(e) => e.into_response(),
    };
    Box::into_raw(Box::new(resp))
}

/// Evaluate a range PromQL query over `[start_ms, end_ms]` at `step_ms`
/// granularity.
///
/// # Safety
/// Same requirements as `nd_promql_query_instant`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_query_range(
    host_machine_guid: *const c_char,
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
    let host = unsafe { cstr_borrow(host_machine_guid) }.map(|s| s.to_string());
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
    let resp = match run_range(q, start_ms, end_ms, step_ms, host) {
        Ok(r) => r,
        Err(e) => e.into_response(),
    };
    Box::into_raw(Box::new(resp))
}

/// Pointer to the NUL-terminated response body.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_response_body(r: *const NdPromqlResponse) -> *const c_char {
    if r.is_null() {
        return ptr::null();
    }
    unsafe { (*r).body.as_ptr() }
}

/// Byte length of the response body, excluding the trailing NUL.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_response_body_len(r: *const NdPromqlResponse) -> usize {
    if r.is_null() {
        return 0;
    }
    unsafe { (*r).body.as_bytes().len() }
}

/// HTTP status code that should accompany the response body.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_response_http_status(r: *const NdPromqlResponse) -> c_int {
    if r.is_null() {
        return 500;
    }
    unsafe { (*r).http_status }
}

/// Release a response handle. Safe to call on null.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_response_free(r: *mut NdPromqlResponse) {
    if r.is_null() {
        return;
    }
    drop(unsafe { Box::from_raw(r) });
}

// ---------------------------------------------------------------------------
// Pipeline
// ---------------------------------------------------------------------------

#[derive(Debug)]
enum QueryError {
    Lower(LowerError),
    Eval(EvalError),
    UnsupportedTopLevel(ValueType),
}

impl From<LowerError> for QueryError {
    fn from(e: LowerError) -> Self {
        QueryError::Lower(e)
    }
}

impl From<EvalError> for QueryError {
    fn from(e: EvalError) -> Self {
        QueryError::Eval(e)
    }
}

impl QueryError {
    fn into_response(self) -> NdPromqlResponse {
        match self {
            QueryError::Lower(e) => NdPromqlResponse::bad_data(&format!("{e}")),
            QueryError::Eval(EvalError::NotYetImplemented(msg)) => {
                NdPromqlResponse::execution(&format!("not yet implemented: {msg}"))
            }
            QueryError::Eval(e) => NdPromqlResponse::execution(&format!("{e}")),
            QueryError::UnsupportedTopLevel(t) => NdPromqlResponse::bad_data(&format!(
                "range query top-level expression must be scalar or instant-vector; got {t:?}"
            )),
        }
    }
}

fn run_instant(
    query: &str,
    at_ms: i64,
    host: Option<String>,
) -> Result<NdPromqlResponse, QueryError> {
    let plan = lower_query(query)?;
    let ctx = EvalContext {
        at_ms,
        lookback_ms: DEFAULT_LOOKBACK_MS,
        host_machine_guid: host,
        max_series: DEFAULT_MAX_SERIES,
    };
    let result = eval(&ctx, &plan)?;
    let body = match result {
        EvalResult::Scalar(v) => serialize_scalar_at(v, at_ms),
        EvalResult::InstantVector(_) | EvalResult::RangeVector(_) => serialize_success(&result),
    };
    Ok(NdPromqlResponse::ok(body))
}

fn run_range(
    query: &str,
    start_ms: i64,
    end_ms: i64,
    step_ms: i64,
    host: Option<String>,
) -> Result<NdPromqlResponse, QueryError> {
    let plan = lower_query(query)?;

    // Range queries require the top-level expression to evaluate to a
    // scalar or instant vector at each step. A range vector (matrix
    // selector at top level) is meaningless here.
    let tt = plan.value_type();
    if matches!(tt, ValueType::RangeVector | ValueType::String) {
        return Err(QueryError::UnsupportedTopLevel(tt));
    }

    use std::collections::BTreeMap;
    // Accumulate per-signature label sets + sample sequence across steps.
    let mut accum: BTreeMap<u64, (Vec<(String, String)>, Vec<Sample>)> = BTreeMap::new();
    // For top-level scalars, every step contributes the same scalar value
    // to a single "metric: {}" series.
    let mut scalar_samples: Option<Vec<Sample>> = None;

    let mut t = start_ms;
    loop {
        let ctx = EvalContext {
            at_ms: t,
            lookback_ms: DEFAULT_LOOKBACK_MS,
            host_machine_guid: host.clone(),
            max_series: DEFAULT_MAX_SERIES,
        };
        let r = eval(&ctx, &plan)?;
        match r {
            EvalResult::Scalar(v) => {
                scalar_samples
                    .get_or_insert_with(Vec::new)
                    .push(Sample {
                        timestamp_ms: t,
                        value: v,
                    });
            }
            EvalResult::InstantVector(series) => {
                for s in series {
                    let sample = s.samples.first().copied().unwrap_or(Sample {
                        timestamp_ms: t,
                        value: f64::NAN,
                    });
                    accum
                        .entry(s.signature)
                        .or_insert_with(|| (s.labels, Vec::new()))
                        .1
                        .push(Sample {
                            timestamp_ms: t,
                            value: sample.value,
                        });
                }
            }
            EvalResult::RangeVector(_) => {
                return Err(QueryError::UnsupportedTopLevel(ValueType::RangeVector));
            }
        }
        let next = match t.checked_add(step_ms) {
            Some(n) => n,
            None => break,
        };
        if next > end_ms {
            break;
        }
        t = next;
    }

    let result = if let Some(samples) = scalar_samples {
        EvalResult::RangeVector(vec![Series {
            labels: Vec::new(),
            signature: 0,
            samples,
        }])
    } else {
        let mut series: Vec<Series> = accum
            .into_iter()
            .map(|(sig, (labels, samples))| Series {
                labels,
                signature: sig,
                samples,
            })
            .collect();
        series.sort_by_key(|s| s.signature);
        EvalResult::RangeVector(series)
    };
    Ok(NdPromqlResponse::ok(serialize_success(&result)))
}

#[cfg(test)]
mod tests {
    use super::*;

    // These tests cover the FFI surface that does not depend on a live
    // Netdata daemon -- parse-error reporting and null/argument handling.
    // Tests that exercise the storage layer (real queries against real
    // contexts) live in the integration smoke harness (`tests/`) and run
    // against an installed binary in chunk 5.

    #[test]
    fn parse_failure_yields_bad_data_envelope() {
        let q = CString::new("not a valid query {").unwrap();
        let r = unsafe { nd_promql_query_instant(ptr::null(), q.as_ptr(), 0, 0) };
        assert!(!r.is_null());
        assert_eq!(unsafe { nd_promql_response_http_status(r) }, 400);
        let body = unsafe { CStr::from_ptr(nd_promql_response_body(r)) }
            .to_str()
            .unwrap()
            .to_string();
        assert!(body.contains(r#""errorType":"bad_data""#), "body = {body}");
        unsafe { nd_promql_response_free(r) };
    }

    #[test]
    fn null_query_yields_bad_data() {
        let r = unsafe { nd_promql_query_instant(ptr::null(), ptr::null(), 0, 0) };
        assert_eq!(unsafe { nd_promql_response_http_status(r) }, 400);
        unsafe { nd_promql_response_free(r) };
    }

    #[test]
    fn range_rejects_oversized_resolution() {
        let q = CString::new("foo").unwrap();
        let r = unsafe { nd_promql_query_range(ptr::null(), q.as_ptr(), 0, 3_600_000, 1, 0) };
        assert_eq!(unsafe { nd_promql_response_http_status(r) }, 400);
        let body = unsafe { CStr::from_ptr(nd_promql_response_body(r)) }
            .to_str()
            .unwrap()
            .to_string();
        assert!(body.contains("11000 points"), "body = {body}");
        unsafe { nd_promql_response_free(r) };
    }

    #[test]
    fn range_rejects_bad_window() {
        let q = CString::new("foo").unwrap();
        let r = unsafe { nd_promql_query_range(ptr::null(), q.as_ptr(), 100, 50, 1000, 0) };
        assert_eq!(unsafe { nd_promql_response_http_status(r) }, 400);
        unsafe { nd_promql_response_free(r) };
    }

}
