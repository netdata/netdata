// SPDX-License-Identifier: GPL-3.0-or-later

//! Netdata PromQL evaluator.
//!
//! This crate implements a full PromQL query engine for the Netdata Agent.
//! It parses PromQL expressions via `promql-parser`, lowers them into a typed
//! Plan IR, evaluates against the Netdata storage engine, and serializes
//! results in the Prometheus HTTP API JSON format.
//!
//! # Architecture
//!
//! The pipeline has four stages:
//!
//! 1. **Parse** — a PromQL string is parsed into an AST via `promql-parser`.
//! 2. **Lower** — the AST is lowered into a typed [`plan::Plan`] IR that
//!    resolves operators, compiles regex matchers, and catches type errors.
//! 3. **Evaluate** — the Plan is walked against an [`eval::EvalContext`],
//!    which carries a time grid, storage backend, and query parameters.
//!    Selectors fetch data through the [`storage::Backend`] trait; operators
//!    and aggregations combine results in a single whole-range pass.
//! 4. **Serialize** — results are written as Prometheus-compatible JSON.
//!
//! # C ABI entry points
//!
//! The crate is compiled as a static library linked into the Netdata daemon.
//! The C ABI exposes two query functions and four response accessors:
//!
//! - [`nd_promql_query_instant`] and [`nd_promql_query_range`] accept a
//!   PromQL string and time parameters, returning an opaque response handle.
//! - [`nd_promql_response_body`], [`nd_promql_response_body_len`],
//!   [`nd_promql_response_http_status`] read fields from the handle.
//! - [`nd_promql_response_free`] releases the handle.
//!
//! # Supported PromQL features
//!
//! Number literals, vector and matrix selectors, scalar arithmetic,
//! comparisons (with and without `bool`), all standard aggregations
//! (`sum`/`avg`/`min`/`max`/`count`/`topk`/`bottomk`/`quantile`/`stddev`/
//! `stdvar`/`count_values`/`group`/`limitk`/`limit_ratio`) with `by`/
//! `without`, binary operators with `on`/`ignoring`/`group_left`/
//! `group_right`, set operators (`and`/`or`/`unless`), unary minus, `offset`,
//! `@` modifier, subqueries, counter functions (`rate`/`irate`/`increase`/
//! `delta`), `histogram_quantile`, and all standard `_over_time` functions.
//!
//! # Testing
//!
//! The [`testing`] module exposes an in-memory backend and evaluation helpers
//! for integration tests and the PromQL compliance corpus runner.

mod discovery;
mod eval;
mod output;
mod plan;
mod slow_log;
mod storage;

// Public testing surface for the compliance corpus runner and
// out-of-crate consumers.
pub mod testing;

use std::ffi::{CStr, CString, c_char, c_int};
use std::ptr;
use std::time::Instant;

use eval::{EvalContext, EvalError, EvalResult, Grid, Sample, Series, eval};
use output::{serialize_error, serialize_scalar_at, serialize_success};
use plan::{LowerError, ValueType, lower_query};

/// Prometheus' default cap on the number of points per timeseries in a
/// range query. Guards against pathological step values. Subquery
/// lowering reuses this cap.
pub(crate) const MAX_POINTS_PER_TIMESERIES: usize = 11_000;

/// Default lookback window for instant vector selectors.
const DEFAULT_LOOKBACK_MS: i64 = 5 * 60 * 1000;

/// Default `max_series` cardinality backstop.
const DEFAULT_MAX_SERIES: usize = 10_000;

/// Opaque handle for a query response.
///
/// Created by [`nd_promql_query_instant`] or [`nd_promql_query_range`].
/// Callers read the body and HTTP status through the accessor functions,
/// then release the handle with [`nd_promql_response_free`].
pub struct NdPromqlResponse {
    body: CString,
    http_status: c_int,
}

impl NdPromqlResponse {
    pub(crate) fn ok(body: String) -> Self {
        Self {
            body: CString::new(body).unwrap_or_else(|_| {
                CString::new(r#"{"status":"error","errorType":"internal","error":"response contained NUL byte"}"#).unwrap()
            }),
            http_status: 200,
        }
    }

    pub(crate) fn bad_data(msg: &str) -> Self {
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
    tier_hint: i32,
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
    let resp = match run_instant(q, at_ms, host, tier_hint) {
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
    tier_hint: i32,
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
    let resp = match run_range(q, start_ms, end_ms, step_ms, host, tier_hint) {
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
    tier_hint: i32,
) -> Result<NdPromqlResponse, QueryError> {
    let start = Instant::now();
    let host_for_log = host.clone();
    let inner = run_instant_inner(query, at_ms, host, tier_hint);
    let (final_resp, series_count_opt) = match inner {
        Ok((resp, n)) => (Ok(resp), Some(n)),
        Err(e) => (Err(e), None),
    };
    log_query(
        slow_log::Kind::Instant,
        query,
        host_for_log.as_deref(),
        None,
        None,
        None,
        start,
        &final_resp,
        series_count_opt,
    );
    final_resp
}

fn run_instant_inner(
    query: &str,
    at_ms: i64,
    host: Option<String>,
    tier_hint: i32,
) -> Result<(NdPromqlResponse, usize), QueryError> {
    let plan = lower_query(query)?;
    // Operators that need exact distribution shape force tier 0 because
    // higher-tier bucket aggregates don't preserve enough information.
    let effective_tier_hint = if plan.requires_tier_zero() {
        0
    } else {
        tier_hint
    };
    // For instant queries, the outer "range" is a single point.
    let ctx = EvalContext {
        grid: std::sync::Arc::new(Grid::instant(at_ms)),
        lookback_ms: DEFAULT_LOOKBACK_MS,
        host_machine_guid: host,
        max_series: DEFAULT_MAX_SERIES,
        outer_start_ms: at_ms,
        outer_end_ms: at_ms,
        backend: std::sync::Arc::new(storage::FfiBackend),
        tier_hint: effective_tier_hint,
    };
    let result = eval(&ctx, &plan)?;
    let series_count = match &result {
        EvalResult::Scalar(_) => 1,
        EvalResult::InstantVector(v) | EvalResult::RangeVector(v) => v.len(),
    };
    let body = match result {
        EvalResult::Scalar(v) => serialize_scalar_at(v, at_ms),
        EvalResult::InstantVector(_) | EvalResult::RangeVector(_) => serialize_success(&result),
    };
    Ok((NdPromqlResponse::ok(body), series_count))
}

fn run_range(
    query: &str,
    start_ms: i64,
    end_ms: i64,
    step_ms: i64,
    host: Option<String>,
    tier_hint: i32,
) -> Result<NdPromqlResponse, QueryError> {
    let start = Instant::now();
    let host_for_log = host.clone();
    let inner = run_range_inner(query, start_ms, end_ms, step_ms, host, tier_hint);
    let (final_resp, series_count_opt) = match inner {
        Ok((resp, n)) => (Ok(resp), Some(n)),
        Err(e) => (Err(e), None),
    };
    log_query(
        slow_log::Kind::Range,
        query,
        host_for_log.as_deref(),
        Some(start_ms),
        Some(end_ms),
        Some(step_ms),
        start,
        &final_resp,
        series_count_opt,
    );
    final_resp
}

fn run_range_inner(
    query: &str,
    start_ms: i64,
    end_ms: i64,
    step_ms: i64,
    host: Option<String>,
    tier_hint: i32,
) -> Result<(NdPromqlResponse, usize), QueryError> {
    let plan = lower_query(query)?;

    // Range queries require the top-level expression to evaluate to a
    // scalar or instant vector at each step. A range vector (matrix
    // selector at top level) is meaningless here.
    let tt = plan.value_type();
    if matches!(tt, ValueType::RangeVector | ValueType::String) {
        return Err(QueryError::UnsupportedTopLevel(tt));
    }

    // Operators that need exact distribution shape force tier 0.
    // See [`Plan::requires_tier_zero`] for the full list.
    let effective_tier_hint = if plan.requires_tier_zero() {
        0
    } else {
        tier_hint
    };

    // Every operator is grid-aware. Build one Grid::range, call eval
    // once, serialize. Selectors and per-position operators emit grid-
    // aligned series; the serializer reads them as a matrix.
    run_range_single_pass(plan, start_ms, end_ms, step_ms, host, effective_tier_hint)
}

/// Single-pass range query path. Builds a [`Grid::range`] once, calls
/// [`eval`] once. Selectors and per-position operators emit grid-aligned
/// series; the serializer reads them as a matrix.
fn run_range_single_pass(
    plan: plan::Plan,
    start_ms: i64,
    end_ms: i64,
    step_ms: i64,
    host: Option<String>,
    tier_hint: i32,
) -> Result<(NdPromqlResponse, usize), QueryError> {
    let grid = std::sync::Arc::new(Grid::range(start_ms, end_ms, step_ms));
    let ctx = EvalContext {
        grid: std::sync::Arc::clone(&grid),
        lookback_ms: DEFAULT_LOOKBACK_MS,
        host_machine_guid: host,
        max_series: DEFAULT_MAX_SERIES,
        outer_start_ms: start_ms,
        outer_end_ms: end_ms,
        backend: std::sync::Arc::new(storage::FfiBackend),
        tier_hint,
    };
    let r = eval(&ctx, &plan)?;
    let result = match r {
        EvalResult::Scalar(v) => {
            // Broadcast the scalar across the grid.
            let values: Vec<f64> = vec![v; grid.len()];
            EvalResult::RangeVector(vec![Series::new(
                Vec::new(),
                std::sync::Arc::clone(&grid.timestamps),
                values,
            )])
        }
        EvalResult::InstantVector(series) => EvalResult::RangeVector(series),
        EvalResult::RangeVector(_) => {
            return Err(QueryError::UnsupportedTopLevel(ValueType::RangeVector));
        }
    };
    let series_count = match &result {
        EvalResult::RangeVector(v) => v.len(),
        _ => 0,
    };
    Ok((
        NdPromqlResponse::ok(serialize_success(&result)),
        series_count,
    ))
}

/// Emit one slow-query log record. Status, error type, and count are
/// extracted from whichever arm the result took.
#[allow(clippy::too_many_arguments)]
fn log_query(
    kind: slow_log::Kind,
    query: &str,
    host: Option<&str>,
    start_ms: Option<i64>,
    end_ms: Option<i64>,
    step_ms: Option<i64>,
    started: Instant,
    result: &Result<NdPromqlResponse, QueryError>,
    series_count: Option<usize>,
) {
    let elapsed_ms = started.elapsed().as_millis() as u64;
    let (status, error_type, error_message): (i32, Option<&str>, Option<String>) = match result {
        Ok(resp) => (resp.http_status, None, None),
        Err(QueryError::Lower(e)) => (400, Some("bad_data"), Some(format!("{e}"))),
        Err(QueryError::Eval(EvalError::NotYetImplemented(msg))) => (
            422,
            Some("execution"),
            Some(format!("not yet implemented: {msg}")),
        ),
        Err(QueryError::Eval(e)) => (422, Some("execution"), Some(format!("{e}"))),
        Err(QueryError::UnsupportedTopLevel(t)) => (
            400,
            Some("bad_data"),
            Some(format!(
                "range query top-level expression must be scalar or instant-vector; got {t:?}"
            )),
        ),
    };
    slow_log::record(slow_log::Record {
        kind,
        query,
        host,
        start_ms,
        end_ms,
        step_ms,
        elapsed_ms,
        http_status: status,
        series_count,
        error_type,
        error_message: error_message.as_deref(),
    });
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
        let r = unsafe { nd_promql_query_instant(ptr::null(), q.as_ptr(), 0, 0, -1) };
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
        let r = unsafe { nd_promql_query_instant(ptr::null(), ptr::null(), 0, 0, -1) };
        assert_eq!(unsafe { nd_promql_response_http_status(r) }, 400);
        unsafe { nd_promql_response_free(r) };
    }

    #[test]
    fn range_rejects_oversized_resolution() {
        let q = CString::new("foo").unwrap();
        let r = unsafe { nd_promql_query_range(ptr::null(), q.as_ptr(), 0, 3_600_000, 1, 0, -1) };
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
        let r = unsafe { nd_promql_query_range(ptr::null(), q.as_ptr(), 100, 50, 1000, 0, -1) };
        assert_eq!(unsafe { nd_promql_response_http_status(r) }, 400);
        unsafe { nd_promql_response_free(r) };
    }
}
