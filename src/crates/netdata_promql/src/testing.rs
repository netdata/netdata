// SPDX-License-Identifier: GPL-3.0-or-later
//
// Public testing surface. Re-exports the in-memory backend and a
// convenience `eval_instant_against` entry point so integration tests
// and external consumers can drive the evaluator without the C shim.

//! Testing utilities for the PromQL evaluator.
//!
//! This module re-exports the in-memory [`MemBackend`] and provides
//! [`eval_instant_against`], a convenience function that parses a PromQL
//! string, builds an evaluation context around a caller-supplied backend,
//! and returns a [`TestResult`]. This is the primary entry point for the
//! PromQL compliance corpus runner and for unit tests that need known-good
//! inputs.

pub use crate::storage::{Backend, MemBackend, MemSeries};

use std::sync::Arc;

use crate::eval::{EvalContext, EvalError, EvalResult, Grid, eval};
use crate::plan::lower_query;

/// One series in an instant-vector result. Owns its label pairs and
/// scalar value for ergonomic consumption in test assertions.
#[derive(Debug)]
pub struct TestSeries {
    pub labels: Vec<(String, String)>,
    pub value: f64,
}

/// Result of an instant evaluation. Scalars and instant vectors are the
/// only valid top-level results for instant queries.
#[derive(Debug)]
pub enum TestResult {
    Scalar(f64),
    InstantVector(Vec<TestSeries>),
}

/// Parse and evaluate a PromQL string at a single timestamp against a
/// custom [`Backend`].
///
/// This is the main entry point for the compliance corpus runner. It
/// performs lowering, builds a default [`EvalContext`](crate::eval::EvalContext)
/// around the supplied backend, runs evaluation, and converts the result
/// into the convenient [`TestResult`] enum.
///
/// # Errors
///
/// Returns a string describing the error if lowering or evaluation fails.
pub fn eval_instant_against(
    backend: Arc<dyn Backend>,
    query: &str,
    at_ms: i64,
) -> Result<TestResult, String> {
    let plan = lower_query(query).map_err(|e| format!("lower: {e}"))?;
    let ctx = EvalContext {
        grid: Arc::new(Grid::instant(at_ms)),
        lookback_ms: 5 * 60 * 1000,
        host_machine_guid: None,
        max_series: 10_000,
        outer_start_ms: at_ms,
        outer_end_ms: at_ms,
        backend,
        // Compliance harness uses MemBackend which ignores tier hints.
        tier_hint: -1,
    };
    match eval(&ctx, &plan).map_err(|e: EvalError| format!("eval: {e}"))? {
        EvalResult::Scalar(v) => Ok(TestResult::Scalar(v)),
        EvalResult::InstantVector(series) => {
            let out = series
                .into_iter()
                .map(|s| TestSeries {
                    labels: s.labels,
                    value: s.values.first().copied().unwrap_or(f64::NAN),
                })
                .collect();
            Ok(TestResult::InstantVector(out))
        }
        EvalResult::RangeVector(_) => Err("instant query produced a range vector".into()),
    }
}
