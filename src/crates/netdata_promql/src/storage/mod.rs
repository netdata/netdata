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

mod matchers;
mod query;
mod raw;
mod samples;

pub use matchers::{compile_regex, Matcher, MatcherError};
pub use query::{NdQuery, ResolveError, SeriesView};
pub use samples::{NdSamples, Sample};
pub(crate) use samples::flags;
