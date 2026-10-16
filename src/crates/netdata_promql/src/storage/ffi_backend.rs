// SPDX-License-Identifier: GPL-3.0-or-later
//
// `Backend` implementation that delegates to the C data-source shim
// (the original FFI path). This is the production backend.

use super::backend::{Backend, BackendQuery, SeriesMeta};
use super::matchers::Matcher;
use super::query::{NdQuery, ResolveError};

/// Zero-size marker. The shim is process-global; no per-backend state.
pub struct FfiBackend;

impl Backend for FfiBackend {
    fn resolve<'a>(
        &'a self,
        host: Option<&str>,
        matchers: &[Matcher],
        after_s: i64,
        before_s: i64,
        max_series: usize,
        points_wanted: i64,
        tier_hint: i32,
    ) -> Result<Box<dyn BackendQuery + 'a>, ResolveError> {
        let q = NdQuery::resolve(
            host,
            matchers,
            after_s,
            before_s,
            max_series,
            points_wanted,
            tier_hint,
        )?;
        Ok(Box::new(FfiQuery(q)))
    }
}

struct FfiQuery(NdQuery);

impl BackendQuery for FfiQuery {
    fn len(&self) -> usize {
        self.0.len()
    }

    fn was_truncated(&self) -> bool {
        self.0.was_truncated()
    }

    fn series_meta(&self, i: usize) -> Option<SeriesMeta> {
        let view = self.0.series(i)?;
        let labels: Vec<(String, String)> = view
            .labels()
            .map(|(n, v)| (n.to_string(), v.to_string()))
            .collect();
        Some(SeriesMeta {
            labels,
            signature: view.signature(),
        })
    }

    fn drain_samples(
        &self,
        i: usize,
        after_s: i64,
        before_s: i64,
        step_ms: i64,
        out_ts: &mut Vec<i64>,
        out_vals: &mut Vec<f64>,
    ) {
        self.0
            .drain_samples(i, after_s, before_s, step_ms, out_ts, out_vals);
    }
}
