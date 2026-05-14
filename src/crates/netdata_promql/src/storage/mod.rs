// SPDX-License-Identifier: GPL-3.0-or-later
//
// Storage adapter: safe wrappers around the C data-source shim
// (`src/database/contexts/promql-data-source.{c,h}`).
//
// Two public abstractions:
//   - `Matcher`  -- a single label predicate
//   - `NdQuery`  -- the resolved series set for one query, with Drop.
//                   Samples are drained per series into caller-provided
//                   `(Vec<i64>, Vec<f64>)` column buffers (SOW-0040).
//
// `raw` holds bindgen output and is not re-exported.

#![allow(dead_code)] // Some methods are reserved for chunks 4/5.

mod backend;
mod ffi_backend;
mod matchers;
mod mem_backend;
mod query;
pub(crate) mod raw;

#[cfg(test)]
mod test_stubs;

// Some items are exposed for use by later chunks (eval needs Matcher, plan
// needs compile_regex). Mark the unused ones as expected-dead.
#[allow(unused_imports)]
pub use backend::{Backend, BackendQuery, SeriesMeta};
#[allow(unused_imports)]
pub use ffi_backend::FfiBackend;
#[allow(unused_imports)]
pub use matchers::{compile_regex, Matcher, MatcherError};
#[allow(unused_imports)]
pub use mem_backend::{MemBackend, MemSeries};
#[allow(unused_imports)]
pub use query::{NdQuery, ResolveError, SeriesView};
