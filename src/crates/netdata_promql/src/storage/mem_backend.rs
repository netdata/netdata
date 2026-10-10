// SPDX-License-Identifier: GPL-3.0-or-later
//
// In-memory storage backend.
//
// Used by the compliance-corpus runner and by unit tests that need
// synthetic series. Not wired into the daemon; lives in the crate so
// the evaluator can be exercised without the C shim.

use std::sync::RwLock;

use super::backend::{Backend, BackendQuery, SeriesMeta};
use super::matchers::Matcher;
use super::query::ResolveError;
use crate::eval::labels_signature;

/// One synthetic series.
///
/// Samples are stored as parallel `(timestamps_ms, values)` columns,
/// matching the shape `drain_samples` produces and `eval` consumes.
/// Values may be NaN to represent "missing"; promqltest's `_` token
/// loads as NaN.
#[derive(Clone, Debug)]
pub struct MemSeries {
    /// Labels in sorted-by-name order. The first entry is normally
    /// `("__name__", "<metric>")`.
    pub labels: Vec<(String, String)>,
    /// Sample timestamps in milliseconds, ascending. Length equal to
    /// `values`.
    pub timestamps_ms: Vec<i64>,
    /// Sample values, parallel to `timestamps_ms`.
    pub values: Vec<f64>,
}

impl MemSeries {
    /// Construct from labels and a `(ts, value)` pair list. Used by
    /// the compliance corpus runner and unit tests.
    pub fn new(labels: Vec<(String, String)>, samples: Vec<(i64, f64)>) -> Self {
        let mut labels = labels;
        labels.sort_by(|a, b| a.0.cmp(&b.0));
        let mut timestamps_ms = Vec::with_capacity(samples.len());
        let mut values = Vec::with_capacity(samples.len());
        for (t, v) in samples {
            timestamps_ms.push(t);
            values.push(v);
        }
        Self {
            labels,
            timestamps_ms,
            values,
        }
    }

    pub fn signature(&self) -> u64 {
        labels_signature(&self.labels)
    }
}

/// The in-memory backend. Thread-safe so the daemon-shaped
/// `Arc<dyn Backend>` API works in tests that may parallelise.
pub struct MemBackend {
    inner: RwLock<Vec<MemSeries>>,
}

impl MemBackend {
    pub fn new() -> Self {
        Self {
            inner: RwLock::new(Vec::new()),
        }
    }

    /// Append a series. The compliance runner calls this for each
    /// row of a `load` block.
    pub fn add_series(&self, s: MemSeries) {
        self.inner.write().expect("mem backend poisoned").push(s);
    }

    /// Remove all series. Called by the `.test` `clear` command.
    pub fn clear(&self) {
        self.inner.write().expect("mem backend poisoned").clear();
    }

    /// Series count for diagnostics.
    pub fn len(&self) -> usize {
        self.inner.read().expect("mem backend poisoned").len()
    }
}

impl Default for MemBackend {
    fn default() -> Self {
        Self::new()
    }
}

impl Backend for MemBackend {
    fn resolve<'a>(
        &'a self,
        _host: Option<&str>,
        matchers: &[Matcher],
        _after_s: i64,
        _before_s: i64,
        max_series: usize,
        _points_wanted: i64,
        _tier_hint: i32,
    ) -> Result<Box<dyn BackendQuery + 'a>, ResolveError> {
        let series = self.inner.read().expect("mem backend poisoned");

        // Walk all series, keep those that satisfy every matcher.
        // The mem backend ignores the retention window (samples are
        // explicit) and applies every matcher including regex --
        // there's no two-stage chart-then-dim pre-filter to mirror.
        let mut indices: Vec<usize> = Vec::with_capacity(series.len());
        for (i, s) in series.iter().enumerate() {
            if series_matches(&s.labels, matchers) {
                indices.push(i);
                if indices.len() > max_series {
                    return Ok(Box::new(MemQuery {
                        series: series.clone(),
                        indices,
                        truncated: true,
                    }));
                }
            }
        }

        if indices.is_empty() {
            return Err(ResolveError::Empty);
        }

        Ok(Box::new(MemQuery {
            // Clone the snapshot the resolver saw so reads through the
            // query don't need to re-acquire the lock. For a test
            // backend that's fine.
            series: series.clone(),
            indices,
            truncated: false,
        }))
    }
}

struct MemQuery {
    series: Vec<MemSeries>,
    indices: Vec<usize>,
    truncated: bool,
}

impl BackendQuery for MemQuery {
    fn len(&self) -> usize {
        self.indices.len()
    }

    fn was_truncated(&self) -> bool {
        self.truncated
    }

    fn series_meta(&self, i: usize) -> Option<SeriesMeta> {
        let idx = *self.indices.get(i)?;
        let s = self.series.get(idx)?;
        Some(SeriesMeta {
            labels: s.labels.clone(),
            signature: s.signature(),
        })
    }

    fn drain_samples(
        &self,
        i: usize,
        after_s: i64,
        before_s: i64,
        _step_ms: i64,
        out_ts: &mut Vec<i64>,
        out_vals: &mut Vec<f64>,
    ) {
        out_ts.clear();
        out_vals.clear();
        let Some(&idx) = self.indices.get(i) else {
            return;
        };
        let Some(s) = self.series.get(idx) else {
            return;
        };
        // Mirror the FFI backend's second-resolution boundaries.
        let lo_ms = after_s.saturating_mul(1000);
        let hi_ms = before_s.saturating_mul(1000);
        for (k, &t) in s.timestamps_ms.iter().enumerate() {
            if t < lo_ms {
                continue;
            }
            if t > hi_ms {
                break;
            }
            out_ts.push(t);
            out_vals.push(s.values[k]);
        }
    }
}

fn series_matches(labels: &[(String, String)], matchers: &[Matcher]) -> bool {
    for m in matchers {
        let value = labels
            .iter()
            .find(|(n, _)| n == m.name())
            .map(|(_, v)| v.as_str())
            .unwrap_or("");
        if !m.matches(value) {
            return false;
        }
    }
    true
}

#[cfg(test)]
mod tests {
    use super::*;

    fn s(labels: &[(&str, &str)], samples: &[(i64, f64)]) -> MemSeries {
        MemSeries::new(
            labels
                .iter()
                .map(|(n, v)| (n.to_string(), v.to_string()))
                .collect(),
            samples.to_vec(),
        )
    }

    #[test]
    fn resolve_filters_by_eq_matcher() {
        let b = MemBackend::new();
        b.add_series(s(&[("__name__", "m"), ("job", "api")], &[(0, 1.0)]));
        b.add_series(s(&[("__name__", "m"), ("job", "web")], &[(0, 2.0)]));
        let q = b
            .resolve(
                None,
                &[Matcher::eq("__name__", "m"), Matcher::eq("job", "api")],
                0,
                10,
                100,
                0,
                -1,
            )
            .unwrap();
        assert_eq!(q.len(), 1);
        let meta = q.series_meta(0).unwrap();
        assert!(meta.labels.iter().any(|(n, v)| n == "job" && v == "api"));
    }

    #[test]
    fn resolve_returns_empty_for_no_match() {
        let b = MemBackend::new();
        b.add_series(s(&[("__name__", "m")], &[(0, 1.0)]));
        let r = b.resolve(None, &[Matcher::eq("__name__", "other")], 0, 10, 100, 0, -1);
        assert!(matches!(r, Err(ResolveError::Empty)));
    }

    #[test]
    fn drain_samples_respects_window() {
        let b = MemBackend::new();
        b.add_series(s(
            &[("__name__", "m")],
            &[(0, 1.0), (5000, 2.0), (10_000, 3.0), (15_000, 4.0)],
        ));
        let q = b
            .resolve(None, &[Matcher::eq("__name__", "m")], 0, 100, 100, 0, -1)
            .unwrap();
        let mut ts = Vec::new();
        let mut vals = Vec::new();
        q.drain_samples(0, 3, 12, 0, &mut ts, &mut vals);
        assert_eq!(ts, vec![5000, 10_000]);
        assert_eq!(vals, vec![2.0, 3.0]);
    }
}
