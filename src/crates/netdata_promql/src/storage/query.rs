// SPDX-License-Identifier: GPL-3.0-or-later
//
// Safe wrapper around `nd_pds_query`.
//
// `NdQuery` owns the shim's opaque pointer and releases it on Drop. Series
// rejected by RE/NRE matchers are tracked in a Rust-side filter index --
// the shim's series array is read-only from our perspective. Borrowing
// rules: a `NdQuery` reference can hand out `SeriesView` borrows and
// drain per-series samples into caller-provided column buffers (SOW-0040).

use std::ffi::{CStr, CString};
use std::os::raw::c_char;
use std::ptr::NonNull;
use std::slice;

use super::matchers::{Matcher, MatcherError, MatchersFfi};
use super::raw;

/// Result of a shim-side query resolution, including any RE/NRE post-filter.
pub struct NdQuery {
    handle: NonNull<raw::nd_pds_query>,
    // Indices into the underlying FFI series array that survived the
    // RE/NRE post-filter. None means "no filter applied" (all series pass).
    filter: Option<Vec<usize>>,
}

unsafe impl Send for NdQuery {}
unsafe impl Sync for NdQuery {}

/// Read-only view of one resolved series's labels.
pub struct SeriesView<'q> {
    labels: &'q [raw::nd_pds_label],
    signature: u64,
}

impl<'q> SeriesView<'q> {
    pub fn signature(&self) -> u64 {
        self.signature
    }

    /// Iterate `(name, value)` for every label on this series.
    pub fn labels(&self) -> impl Iterator<Item = (&'q str, &'q str)> + '_ {
        self.labels.iter().filter_map(|l| {
            let name = unsafe { cstr_to_str(l.name) }?;
            let value = unsafe { cstr_to_str(l.value) }?;
            Some((name, value))
        })
    }

    /// Look up one label by name. `None` if not present.
    pub fn get(&self, name: &str) -> Option<&'q str> {
        self.labels.iter().find_map(|l| {
            let n = unsafe { cstr_to_str(l.name) }?;
            if n == name {
                unsafe { cstr_to_str(l.value) }
            } else {
                None
            }
        })
    }
}

unsafe fn cstr_to_str<'a>(p: *const c_char) -> Option<&'a str> {
    if p.is_null() {
        return None;
    }
    unsafe { CStr::from_ptr(p) }.to_str().ok()
}

/// Reasons a resolve call can fail or yield nothing.
#[derive(Debug, thiserror::Error)]
pub enum ResolveError {
    #[error("matcher translation failed: {0}")]
    Matcher(#[from] MatcherError),

    #[error("host_machine_guid contains an interior NUL")]
    InvalidHost,

    #[error("shim error: {0}")]
    Shim(String),

    /// Resolution returned no series. Not strictly an error, but represented
    /// in the same error type because the shim's NULL return means either.
    /// PromQL evaluation typically treats "no series" as a valid empty
    /// result; the caller decides.
    #[error("no matching series")]
    Empty,
}

impl NdQuery {
    /// Resolve a query against the shim.
    ///
    /// `host`:
    ///   - `None`  -> localhost only
    ///   - `Some("*")` -> all known hosts
    ///   - `Some(name)` -> the host with that machine_guid or hostname
    ///
    /// The RE/NRE matchers are applied post-resolution against the
    /// candidate label sets.
    pub fn resolve(
        host: Option<&str>,
        matchers: &[Matcher],
        after_s: i64,
        before_s: i64,
        max_series: usize,
        points_wanted: i64,
        tier_hint: i32,
    ) -> Result<Self, ResolveError> {
        let host_c = host
            .map(CString::new)
            .transpose()
            .map_err(|_| ResolveError::InvalidHost)?;

        let ffi = MatchersFfi::from_slice(matchers)?;
        let mut err = vec![0i8; 256];

        let host_ptr = host_c
            .as_ref()
            .map(|c| c.as_ptr())
            .unwrap_or(std::ptr::null());

        let raw_q = unsafe {
            raw::nd_pds_resolve(
                host_ptr,
                ffi.as_ptr(),
                ffi.len(),
                after_s,
                before_s,
                max_series,
                points_wanted,
                tier_hint,
                err.as_mut_ptr(),
                err.len(),
            )
        };

        let handle = match NonNull::new(raw_q) {
            Some(h) => h,
            None => {
                // Distinguish "error" from "no matches": the shim writes to
                // the err buffer on error and leaves it untouched on empty.
                let msg = unsafe { CStr::from_ptr(err.as_ptr()) }
                    .to_string_lossy()
                    .into_owned();
                if msg.is_empty() {
                    return Err(ResolveError::Empty);
                }
                return Err(ResolveError::Shim(msg));
            }
        };

        let mut q = NdQuery {
            handle,
            filter: None,
        };

        // Apply RE/NRE post-filter if any regex matchers are present.
        if matchers.iter().any(|m| m.is_regex()) {
            q.apply_regex_filter(matchers);
        }

        Ok(q)
    }

    fn apply_regex_filter(&mut self, matchers: &[Matcher]) {
        let total = unsafe { raw::nd_pds_series_count(self.handle.as_ptr()) };
        let mut kept = Vec::with_capacity(total);
        'series: for i in 0..total {
            let view = self.series_view_raw(i);
            for m in matchers.iter().filter(|m| m.is_regex()) {
                let value = view.get(m.name()).unwrap_or("");
                if !m.matches(value) {
                    continue 'series;
                }
            }
            kept.push(i);
        }
        self.filter = Some(kept);
    }

    /// True if the shim hit `max_series` during resolution and returned a
    /// prefix of the candidate set. Phase 1's query path turns this into a
    /// `bad_data` error; Phase 2's discovery path converts it to a
    /// `warnings` envelope field.
    pub fn was_truncated(&self) -> bool {
        unsafe { raw::nd_pds_was_truncated(self.handle.as_ptr()) }
    }

    /// Number of series after RE/NRE post-filter.
    pub fn len(&self) -> usize {
        match &self.filter {
            Some(f) => f.len(),
            None => unsafe { raw::nd_pds_series_count(self.handle.as_ptr()) },
        }
    }

    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    fn resolved_index(&self, i: usize) -> Option<usize> {
        match &self.filter {
            Some(f) => f.get(i).copied(),
            None => {
                let total = unsafe { raw::nd_pds_series_count(self.handle.as_ptr()) };
                if i < total {
                    Some(i)
                } else {
                    None
                }
            }
        }
    }

    fn series_view_raw(&self, raw_index: usize) -> SeriesView<'_> {
        let mut labels_ptr: *const raw::nd_pds_label = std::ptr::null();
        let mut labels_len: usize = 0;
        let mut signature: u64 = 0;
        unsafe {
            raw::nd_pds_series_metadata(
                self.handle.as_ptr(),
                raw_index,
                &mut labels_ptr,
                &mut labels_len,
                &mut signature,
            )
        };
        let labels = if labels_ptr.is_null() || labels_len == 0 {
            &[]
        } else {
            unsafe { slice::from_raw_parts(labels_ptr, labels_len) }
        };
        SeriesView { labels, signature }
    }

    /// Read-only access to the labels and signature of the i-th series
    /// (after RE/NRE filter).
    pub fn series(&self, i: usize) -> Option<SeriesView<'_>> {
        let raw_i = self.resolved_index(i)?;
        Some(self.series_view_raw(raw_i))
    }

    /// Drain samples for the i-th series (after RE/NRE filter) into the
    /// caller's column buffers. The buffers are cleared first; on
    /// return they hold parallel `(timestamp_ms, value)` columns in
    /// ascending timestamp order. SOW-0040 replaces the prior
    /// iterator-returning shape so the per-sample fetch does not pay a
    /// per-call indirect-dispatch cost.
    pub fn drain_samples(
        &self,
        i: usize,
        after_s: i64,
        before_s: i64,
        step_ms: i64,
        out_ts: &mut Vec<i64>,
        out_vals: &mut Vec<f64>,
    ) {
        out_ts.clear();
        out_vals.clear();
        let Some(raw_i) = self.resolved_index(i) else {
            return;
        };
        let p = unsafe {
            raw::nd_pds_open_samples(self.handle.as_ptr(), raw_i, after_s, before_s, step_ms)
        };
        let Some(handle) = NonNull::new(p) else {
            return;
        };
        // Tight pull loop. The shim's per-sample call returns >0 on a
        // valid sample, 0 on EOF, <0 on error. We treat both 0 and <0 as
        // end-of-stream (matches the previous iterator-shape behavior).
        loop {
            let mut ts: i64 = 0;
            let mut value: f64 = 0.0;
            let mut flags: u32 = 0;
            let r = unsafe {
                raw::nd_pds_samples_next(handle.as_ptr(), &mut ts, &mut value, &mut flags)
            };
            if r <= 0 {
                break;
            }
            out_ts.push(ts);
            out_vals.push(value);
        }
        unsafe { raw::nd_pds_samples_close(handle.as_ptr()) };
    }
}

impl Drop for NdQuery {
    fn drop(&mut self) {
        unsafe { raw::nd_pds_free(self.handle.as_ptr()) }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // These tests do not exercise the shim itself (which requires a live
    // RRDHOST). They exercise the Rust-side wrapper logic that does not
    // depend on FFI return values. Functional tests against the shim land
    // in chunk 3 alongside the evaluator.

    #[test]
    fn empty_matchers_translate_cleanly() {
        let ffi = MatchersFfi::from_slice(&[]).unwrap();
        assert_eq!(ffi.len(), 0);
    }

    #[test]
    fn resolve_error_display_includes_shim_message() {
        let e = ResolveError::Shim("oops".into());
        let s = format!("{e}");
        assert!(s.contains("oops"));
    }
}
