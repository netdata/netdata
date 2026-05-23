// SPDX-License-Identifier: GPL-3.0-or-later
//
// Storage adapter: safe wrappers around the C data-source shim
// (`src/database/contexts/promql-data-source.{c,h}`).
//
// Two public abstractions:
//   - `Matcher`  -- a single label predicate
//   - `NdQuery`  -- the resolved series set for one query, with Drop.
//                   Samples are drained per series into caller-provided
//                   `(Vec<i64>, Vec<f64>)` column buffers.
//
// `raw` holds bindgen output and is not re-exported.

//! Storage backend abstraction.
//!
//! The PromQL evaluator's leaf selectors reach into storage through the
//! [`Backend`] trait. Two implementations exist:
//!
//! - [`FfiBackend`] — wraps the C shim's query interface. Used by the
//!   daemon in production.
//! - [`MemBackend`] — holds synthetic series in memory. Used by the
//!   compliance corpus runner and unit tests.
//!
//! Label matchers ([`Matcher`]) are compiled at query-lowering time and
//! used both for shim-side filtering (EQ/NE) and Rust-side post-filtering
//! (RE/NRE via a process-wide regex cache).

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
pub use matchers::{Matcher, MatcherError, compile_regex};
#[allow(unused_imports)]
pub use mem_backend::{MemBackend, MemSeries};
#[allow(unused_imports)]
pub use query::{NdQuery, ResolveError, SeriesView};
