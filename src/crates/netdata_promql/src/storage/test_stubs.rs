// SPDX-License-Identifier: GPL-3.0-or-later
//
// Linker stubs for the C data-source shim, used only by `cargo test`.
//
// The shim's real implementation lives in `src/database/contexts/promql-data-source.c`
// and links into the netdata daemon binary. When we build the test target,
// however, there is no daemon -- so we provide minimal stubs that satisfy
// the linker and return shapes equivalent to "no series found." Integration
// tests against the live shim land in chunk 5 via the curl-based harness.

#![cfg(test)]

use std::os::raw::c_char;

#[unsafe(no_mangle)]
pub extern "C" fn nd_pds_resolve(
    _host: *const c_char,
    _matchers: *const super::raw::nd_pds_matcher,
    _matchers_len: usize,
    _after_s: i64,
    _before_s: i64,
    _max_series: usize,
    _err: *mut c_char,
    _err_len: usize,
) -> *mut super::raw::nd_pds_query {
    std::ptr::null_mut()
}

#[unsafe(no_mangle)]
pub extern "C" fn nd_pds_series_count(_q: *const super::raw::nd_pds_query) -> usize {
    0
}

#[unsafe(no_mangle)]
pub extern "C" fn nd_pds_series_metadata(
    _q: *const super::raw::nd_pds_query,
    _i: usize,
    _labels: *mut *const super::raw::nd_pds_label,
    _labels_len: *mut usize,
    _signature: *mut u64,
) {
}

#[unsafe(no_mangle)]
pub extern "C" fn nd_pds_open_samples(
    _q: *const super::raw::nd_pds_query,
    _i: usize,
    _after_s: i64,
    _before_s: i64,
    _step_ms: i64,
) -> *mut super::raw::nd_pds_samples {
    std::ptr::null_mut()
}

#[unsafe(no_mangle)]
pub extern "C" fn nd_pds_samples_next(
    _it: *mut super::raw::nd_pds_samples,
    _ts: *mut i64,
    _value: *mut f64,
    _flags: *mut u32,
) -> i32 {
    0
}

#[unsafe(no_mangle)]
pub extern "C" fn nd_pds_samples_close(_it: *mut super::raw::nd_pds_samples) {}

#[unsafe(no_mangle)]
pub extern "C" fn nd_pds_free(_q: *mut super::raw::nd_pds_query) {}
