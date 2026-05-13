// SPDX-License-Identifier: GPL-3.0-or-later
//
// Discovery endpoints (Phase 2 of SOW-0018).
//
// Three FFI functions, one per endpoint that needs to project resolved
// series into Prometheus discovery shapes:
//
//   * `nd_promql_labels`        -> /api/v1/labels
//   * `nd_promql_label_values`  -> /api/v1/label/<name>/values
//   * `nd_promql_series`        -> /api/v1/series
//
// Each accepts an array of `match[]` selectors. The implementation parses
// each selector through `plan::lower_query`, extracts the matchers from
// the resulting Plan::VectorSelect node, and resolves them through the
// shim via `storage::NdQuery`. Results from multiple selectors are unioned
// by signature.
//
// `/api/v1/metadata` and `/api/v1/status/buildinfo` do not need Rust:
// metadata enumeration is pure-C (see `nd_pds_metadata_*`), and buildinfo
// is a static JSON response.

use std::collections::{BTreeMap, BTreeSet};
use std::ffi::{c_char, CStr};

use crate::output::{
    serialize_metadata_map, serialize_series_list, serialize_string_list, MetadataEntry,
};
use crate::plan::{lower_query, LowerError, Plan};
use crate::storage::{Matcher, NdQuery, ResolveError};
use crate::NdPromqlResponse;

/// `match[]` strings often arrive with no `__name__` predicate -- Grafana's
/// universal sentinel is `{__name__!=""}`. The shim enumerates every
/// context when no `__name__` EQ matcher is given, which is exactly what
/// we want.
const TRUNCATION_WARNING: &str =
    "result truncated at max_series=10000; tighten match[] or raise the cap";

/// `host_machine_guid` follows the shim contract:
///   - `None` -> localhost only
///   - `Some("*")` -> all known hosts
///   - `Some(name)` -> the host with that machine_guid or hostname (Phase 2 chunk 2)
fn host_from_ptr(p: *const c_char) -> Option<String> {
    if p.is_null() {
        return None;
    }
    let s = unsafe { CStr::from_ptr(p) }.to_str().ok()?;
    if s.is_empty() {
        None
    } else {
        Some(s.to_string())
    }
}

/// Collect matchers from an array of `match[]` selector strings.
///
/// Returns one `Vec<Matcher>` per selector. An empty input yields a single
/// empty matcher set (so a discovery request with no `match[]` resolves
/// every series on the host, capped by `max_series`).
fn parse_selectors(
    ptrs: *const *const c_char,
    len: usize,
) -> Result<Vec<Vec<Matcher>>, DiscoveryError> {
    if ptrs.is_null() || len == 0 {
        return Ok(vec![Vec::new()]);
    }
    let slice = unsafe { std::slice::from_raw_parts(ptrs, len) };
    let mut out = Vec::with_capacity(slice.len());
    for raw in slice {
        if raw.is_null() {
            continue;
        }
        let s = unsafe { CStr::from_ptr(*raw) }
            .to_str()
            .map_err(|_| DiscoveryError::BadSelector("non-UTF-8 selector".into()))?;
        if s.is_empty() {
            continue;
        }
        let plan = lower_query(s).map_err(DiscoveryError::Lower)?;
        let matchers = match plan {
            Plan::VectorSelect { matchers, .. } => matchers,
            _ => {
                return Err(DiscoveryError::BadSelector(format!(
                    "selector must be a vector selector, got {:?}",
                    plan.value_type()
                )));
            }
        };
        out.push(matchers);
    }
    if out.is_empty() {
        out.push(Vec::new());
    }
    Ok(out)
}

#[derive(Debug)]
enum DiscoveryError {
    Lower(LowerError),
    BadSelector(String),
}

impl DiscoveryError {
    fn into_response(self) -> NdPromqlResponse {
        match self {
            DiscoveryError::Lower(e) => NdPromqlResponse::bad_data(&format!("{e}")),
            DiscoveryError::BadSelector(m) => NdPromqlResponse::bad_data(&m),
        }
    }
}

/// Resolve every selector and aggregate results into a callback.
///
/// `visit` is called once per unique series (deduped by signature) with the
/// series view borrowed from the underlying `NdQuery`. The `NdQuery`s are
/// dropped after `resolve_all` returns, so callers must clone any data
/// they need to keep.
fn resolve_all<F>(
    host: Option<&str>,
    selector_matchers: &[Vec<Matcher>],
    max_series: usize,
    mut visit: F,
) -> bool
where
    F: FnMut(&[(&str, &str)]),
{
    let mut seen_signatures: BTreeSet<u64> = BTreeSet::new();
    let mut truncated = false;

    for matchers in selector_matchers {
        let q = match NdQuery::resolve(host, matchers, 0, 0, max_series) {
            Ok(q) => q,
            Err(ResolveError::Empty) => continue,
            Err(_) => continue,
        };
        if q.was_truncated() {
            truncated = true;
        }
        for i in 0..q.len() {
            let Some(view) = q.series(i) else { continue };
            if !seen_signatures.insert(view.signature()) {
                continue;
            }
            let labels: Vec<(&str, &str)> = view.labels().collect();
            visit(&labels);
        }
    }
    truncated
}

/// FFI: `/api/v1/labels`. Returns the union of label names across the
/// resolved series.
///
/// # Safety
/// `matchers` must be NULL or a pointer to `matchers_len` C strings. Each
/// non-NULL string is borrowed for the duration of the call.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_labels(
    host_machine_guid: *const c_char,
    matchers: *const *const c_char,
    matchers_len: usize,
    _start_ms: i64,
    _end_ms: i64,
    limit: usize,
) -> *mut NdPromqlResponse {
    let selectors = match parse_selectors(matchers, matchers_len) {
        Ok(s) => s,
        Err(e) => return Box::into_raw(Box::new(e.into_response())),
    };
    let host = host_from_ptr(host_machine_guid);
    let mut names: BTreeSet<String> = BTreeSet::new();
    let truncated = resolve_all(host.as_deref(), &selectors, max_series(limit), |labels| {
        for (n, _) in labels {
            names.insert((*n).to_string());
        }
    });

    let mut items: Vec<String> = names.into_iter().collect();
    if limit > 0 && items.len() > limit {
        items.truncate(limit);
    }
    let warnings = warnings_for(truncated);
    let body = serialize_string_list(&items, &warnings);
    Box::into_raw(Box::new(NdPromqlResponse::ok(body)))
}

/// FFI: `/api/v1/label/<name>/values`.
///
/// # Safety
/// `label_name` and each non-null pointer in `matchers` must be valid
/// NUL-terminated C strings for the duration of the call.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_label_values(
    host_machine_guid: *const c_char,
    label_name: *const c_char,
    matchers: *const *const c_char,
    matchers_len: usize,
    _start_ms: i64,
    _end_ms: i64,
    limit: usize,
) -> *mut NdPromqlResponse {
    let label = match (!label_name.is_null())
        .then(|| unsafe { CStr::from_ptr(label_name) }.to_str().ok())
        .flatten()
    {
        Some(s) if !s.is_empty() => s.to_string(),
        _ => {
            return Box::into_raw(Box::new(NdPromqlResponse::bad_data(
                "label name is required",
            )))
        }
    };
    let selectors = match parse_selectors(matchers, matchers_len) {
        Ok(s) => s,
        Err(e) => return Box::into_raw(Box::new(e.into_response())),
    };
    let host = host_from_ptr(host_machine_guid);
    let mut values: BTreeSet<String> = BTreeSet::new();
    let truncated = resolve_all(host.as_deref(), &selectors, max_series(limit), |labels| {
        for (n, v) in labels {
            if *n == label {
                values.insert((*v).to_string());
            }
        }
    });
    let mut items: Vec<String> = values.into_iter().collect();
    if limit > 0 && items.len() > limit {
        items.truncate(limit);
    }
    let warnings = warnings_for(truncated);
    let body = serialize_string_list(&items, &warnings);
    Box::into_raw(Box::new(NdPromqlResponse::ok(body)))
}

/// FFI: `/api/v1/series`. Returns one label map per unique series.
///
/// # Safety
/// `matchers` must be NULL or a pointer to `matchers_len` C strings.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_series(
    host_machine_guid: *const c_char,
    matchers: *const *const c_char,
    matchers_len: usize,
    _start_ms: i64,
    _end_ms: i64,
    limit: usize,
) -> *mut NdPromqlResponse {
    let selectors = match parse_selectors(matchers, matchers_len) {
        Ok(s) => s,
        Err(e) => return Box::into_raw(Box::new(e.into_response())),
    };
    let host = host_from_ptr(host_machine_guid);
    // Dedupe by signature; the signature is order-stable, so we use
    // BTreeMap with the signature as key to keep results sorted.
    let mut by_sig: BTreeMap<u64, Vec<(String, String)>> = BTreeMap::new();
    let mut signatures_in_order: Vec<u64> = Vec::new();
    {
        // Reuse the resolve loop but with a different visitor.
        let mut seen: BTreeSet<u64> = BTreeSet::new();
        let mut truncated = false;
        for matchers in &selectors {
            let q = match NdQuery::resolve(host.as_deref(), matchers, 0, 0, max_series(limit)) {
                Ok(q) => q,
                Err(ResolveError::Empty) => continue,
                Err(_) => continue,
            };
            if q.was_truncated() {
                truncated = true;
            }
            for i in 0..q.len() {
                let Some(view) = q.series(i) else { continue };
                let sig = view.signature();
                if !seen.insert(sig) {
                    continue;
                }
                let labels: Vec<(String, String)> = view
                    .labels()
                    .map(|(n, v)| (n.to_string(), v.to_string()))
                    .collect();
                by_sig.insert(sig, labels);
                signatures_in_order.push(sig);
            }
        }
        let mut items: Vec<Vec<(String, String)>> = signatures_in_order
            .into_iter()
            .filter_map(|s| by_sig.remove(&s))
            .collect();
        if limit > 0 && items.len() > limit {
            items.truncate(limit);
        }
        let warnings = warnings_for(truncated);
        let body = serialize_series_list(&items, &warnings);
        return Box::into_raw(Box::new(NdPromqlResponse::ok(body)));
    }
}

/// FFI: `/api/v1/metadata`. Returns per-metric TYPE/HELP/unit, optionally
/// filtered to a single metric name.
///
/// # Safety
/// `metric_filter` must be NULL or a valid NUL-terminated C string.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn nd_promql_metadata(
    host_machine_guid: *const c_char,
    metric_filter: *const c_char,
    limit: usize,
) -> *mut NdPromqlResponse {
    use crate::storage::raw;

    let max_entries = if limit == 0 { 10_000 } else { limit };

    let m = unsafe { raw::nd_pds_metadata_collect(host_machine_guid, metric_filter, max_entries) };
    if m.is_null() {
        let body = serialize_metadata_map(&[], &[]);
        return Box::into_raw(Box::new(NdPromqlResponse::ok(body)));
    }

    // Pull every entry out as owned Rust strings first, then serialize.
    // This sidesteps the C-string lifetime puzzle (the metadata set's
    // strings are only valid until `nd_pds_metadata_free`).
    let count = unsafe { raw::nd_pds_metadata_count(m) };
    let mut owned: Vec<(String, String, String, String)> = Vec::with_capacity(count);
    for i in 0..count {
        let mut e = raw::nd_pds_metadata_entry {
            metric_name: std::ptr::null(),
            type_: std::ptr::null(),
            help: std::ptr::null(),
            unit: std::ptr::null(),
        };
        unsafe { raw::nd_pds_metadata_get(m, i, &mut e) };
        if e.metric_name.is_null() {
            continue;
        }
        let name = unsafe { CStr::from_ptr(e.metric_name) }
            .to_string_lossy()
            .into_owned();
        let ty = if e.type_.is_null() {
            String::new()
        } else {
            unsafe { CStr::from_ptr(e.type_) }.to_string_lossy().into_owned()
        };
        let help = if e.help.is_null() {
            String::new()
        } else {
            unsafe { CStr::from_ptr(e.help) }.to_string_lossy().into_owned()
        };
        let unit = if e.unit.is_null() {
            String::new()
        } else {
            unsafe { CStr::from_ptr(e.unit) }.to_string_lossy().into_owned()
        };
        owned.push((name, ty, help, unit));
    }
    unsafe { raw::nd_pds_metadata_free(m) };

    let entries: Vec<MetadataEntry<'_>> = owned
        .iter()
        .map(|(n, t, h, u)| MetadataEntry {
            metric_name: n,
            type_: t,
            help: h,
            unit: u,
        })
        .collect();
    let body = serialize_metadata_map(&entries, &[]);
    Box::into_raw(Box::new(NdPromqlResponse::ok(body)))
}

fn max_series(limit: usize) -> usize {
    // The shim's max_series cap is 10000; we honor whichever is smaller.
    // Grafana's default limit=40000 will therefore cap at the shim
    // ceiling, and truncation surfaces via the warnings field.
    if limit == 0 {
        crate::DEFAULT_MAX_SERIES
    } else {
        limit.min(crate::DEFAULT_MAX_SERIES)
    }
}

fn warnings_for(truncated: bool) -> Vec<String> {
    if truncated {
        vec![TRUNCATION_WARNING.to_string()]
    } else {
        Vec::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::ffi::CString;
    use std::ptr;

    // These tests cover the FFI argument plumbing and JSON shaping. Tests
    // that exercise the shim against a live RRDHOST live in the integration
    // smoke harness.

    #[test]
    fn null_matchers_pointer_is_treated_as_empty() {
        // With no matchers, parse_selectors returns one empty matcher set.
        // The resolve loop then asks the shim for "all series on the host"
        // which in the unit-test environment is empty.
        let r = unsafe { nd_promql_labels(ptr::null(), ptr::null(), 0, 0, 0, 0) };
        assert!(!r.is_null());
        let status = unsafe { crate::nd_promql_response_http_status(r) };
        assert_eq!(status, 200);
        let body = unsafe { CStr::from_ptr(crate::nd_promql_response_body(r)) }
            .to_str()
            .unwrap()
            .to_string();
        assert!(body.contains(r#""status":"success""#), "body = {body}");
        unsafe { crate::nd_promql_response_free(r) };
    }

    #[test]
    fn invalid_selector_yields_bad_data() {
        let bad = CString::new("not a valid {").unwrap();
        let ptrs = [bad.as_ptr()];
        let r = unsafe { nd_promql_labels(ptr::null(), ptrs.as_ptr(), 1, 0, 0, 0) };
        let status = unsafe { crate::nd_promql_response_http_status(r) };
        assert_eq!(status, 400);
        unsafe { crate::nd_promql_response_free(r) };
    }

    #[test]
    fn missing_label_name_yields_bad_data() {
        let r = unsafe { nd_promql_label_values(ptr::null(), ptr::null(), ptr::null(), 0, 0, 0, 0) };
        let status = unsafe { crate::nd_promql_response_http_status(r) };
        assert_eq!(status, 400);
        unsafe { crate::nd_promql_response_free(r) };
    }
}
