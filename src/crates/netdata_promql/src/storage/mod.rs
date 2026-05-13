// SPDX-License-Identifier: GPL-3.0-or-later
//
// Storage adapter: safe wrappers around the C data-source shim
// (`src/database/contexts/promql-data-source.{c,h}`).
//
// Three public types:
//   - `Matcher`  -- a single label predicate
//   - `NdQuery`  -- the resolved series set for one query, with Drop
//   - `NdSamples` -- a sample iterator borrowed from an `NdQuery`
//
// `raw` holds bindgen output and is not re-exported.

#![allow(dead_code)] // Some methods are reserved for chunks 4/5.

mod matchers;
mod query;
pub(crate) mod raw;
mod samples;

#[cfg(test)]
mod test_stubs;

// Some items are exposed for use by later chunks (eval needs Matcher, plan
// needs compile_regex). Mark the unused ones as expected-dead.
#[allow(unused_imports)]
pub use matchers::{compile_regex, Matcher, MatcherError};
#[allow(unused_imports)]
pub use query::{NdQuery, ResolveError, SeriesView};
#[allow(unused_imports)]
pub use samples::{NdSamples, Sample};
#[allow(unused_imports)]
pub(crate) use samples::flags;
