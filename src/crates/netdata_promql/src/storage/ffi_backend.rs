// SPDX-License-Identifier: GPL-3.0-or-later
//
// `Backend` implementation that delegates to the C data-source shim
// (the original FFI path). This is the production backend.

use super::backend::{Backend, BackendQuery, SeriesMeta};
use super::matchers::Matcher;
use super::query::{NdQuery, ResolveError};
use super::samples::Sample;

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
    ) -> Result<Box<dyn BackendQuery + 'a>, ResolveError> {
        let q = NdQuery::resolve(host, matchers, after_s, before_s, max_series)?;
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

    fn open_samples<'q>(
        &'q self,
        i: usize,
        after_s: i64,
        before_s: i64,
        step_ms: i64,
    ) -> Option<Box<dyn Iterator<Item = Sample> + 'q>> {
        let it = self.0.open_samples(i, after_s, before_s, step_ms)?;
        Ok::<_, ()>(()).ok();
        // Returning the iterator boxed as a trait object. The lifetime is
        // tied to `&'q self`, so the borrow checker prevents the iterator
        // from outliving the query.
        Some(Box::new(it))
    }
}
